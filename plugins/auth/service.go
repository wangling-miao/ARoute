package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/mail"
	"strings"
	"sync"
	"time"

	"github.com/wangling-miao/aroute/core"
	"github.com/wangling-miao/aroute/sdk/interfaces"
	"golang.org/x/crypto/bcrypt"
)

const (
	defaultBCryptCost = 10
	minPasswordLength = 8
	apiTokenPrefix    = "aroute_"
	apiTokenBytes     = 32
)

// Service implements interfaces.AuthService.
type Service struct {
	store          *Store
	jwt            *JWTManager
	rbac           *RBACManager
	rateLimiter    *RateLimiter
	logger         *slog.Logger
	config         core.ConfigProvider
	bcryptCost     int
	adminEmail     string
	adminPass      string
	hmacKey        [32]byte
	bgWg           sync.WaitGroup
	trustedProxies []*net.IPNet
}

// NewService creates a new auth Service.
func NewService(store *Store, jwt *JWTManager, rbac *RBACManager, rateLimiter *RateLimiter, logger *slog.Logger, config core.ConfigProvider, authCfg authConfig) *Service {
	cost := authCfg.bcryptCost
	if cost < 10 {
		logger.Warn("bcrypt cost below minimum, using 10", "configured", cost)
		cost = 10
	}
	hmacKey := sha256.Sum256([]byte("aroute-hmac-" + authCfg.jwtSecret))
	return &Service{
		store:          store,
		jwt:            jwt,
		rbac:           rbac,
		rateLimiter:    rateLimiter,
		logger:         logger,
		config:         config,
		bcryptCost:     cost,
		adminEmail:     authCfg.adminEmail,
		adminPass:      authCfg.adminPassword,
		hmacKey:        hmacKey,
		trustedProxies: authCfg.trustedProxies,
	}
}

// Authenticate validates user credentials and returns access/refresh tokens.
func (s *Service) Authenticate(ctx context.Context, req *interfaces.AuthRequest) (*interfaces.AuthResult, error) {
	// Rate limit check by IP (extracted from context or use a default).
	ip := extractIP(ctx)
	allowed, retryAfter := s.rateLimiter.Check(ip)
	if !allowed {
		return nil, fmt.Errorf("rate limit exceeded, retry after %d seconds", retryAfter)
	}

	// Validate input.
	if req.Email == "" || req.Password == "" {
		return nil, fmt.Errorf("email and password are required: %w", interfaces.ErrValidation)
	}

	// Look up user by email.
	user, err := s.store.GetUserByEmail(ctx, req.Email)
	if err != nil {
		if errors.Is(err, interfaces.ErrNotFound) {
			s.rateLimiter.RecordFailure(ip)
			return nil, fmt.Errorf("invalid credentials: %w", interfaces.ErrUnauthorized)
		}
		return nil, fmt.Errorf("get user: %w", err)
	}

	// Check user status.
	if user.Status != "active" {
		return nil, fmt.Errorf("account is %s: %w", user.Status, interfaces.ErrForbidden)
	}

	// Verify password.
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		s.rateLimiter.RecordFailure(ip)
		return nil, fmt.Errorf("invalid credentials: %w", interfaces.ErrUnauthorized)
	}

	// Get role names for the user.
	roleNames, err := s.store.GetUserRoleNames(ctx, user.ID)
	if err != nil {
		return nil, fmt.Errorf("get user roles: %w", err)
	}

	// Viewer-only accounts cannot access the admin panel.
	if isViewerOnly(roleNames) {
		return nil, fmt.Errorf("viewer accounts cannot access the admin panel: %w", interfaces.ErrForbidden)
	}

	// Generate tokens.
	accessToken, _, err := s.jwt.GenerateAccessToken(ctx, user, roleNames)
	if err != nil {
		return nil, fmt.Errorf("generate access token: %w", err)
	}

	refreshToken, _, err := s.jwt.GenerateRefreshToken(ctx, user)
	if err != nil {
		return nil, fmt.Errorf("generate refresh token: %w", err)
	}

	// Record login.
	if err := s.store.UpdateLastLogin(ctx, user.ID); err != nil {
		s.logger.Warn("failed to update last login", "user_id", user.ID, "error", err)
	}

	// Reset rate limit on successful auth.
	s.rateLimiter.Reset(ip)

	// Strip password hash from response.
	user.PasswordHash = ""
	user.Roles = roleNames

	return &interfaces.AuthResult{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    int64(s.jwt.AccessTTL().Seconds()),
		User:         user,
	}, nil
}

// VerifyToken validates a JWT access token and returns user claims.
func (s *Service) VerifyToken(ctx context.Context, token string) (*interfaces.UserClaims, error) {
	claims, err := s.jwt.VerifyToken(token)
	if err != nil {
		return nil, fmt.Errorf("verify token: %w", err)
	}

	// Check if token is blacklisted.
	blacklisted, err := s.store.IsTokenBlacklisted(ctx, claims.TokenID)
	if err != nil {
		return nil, fmt.Errorf("check token blacklist: %w", err)
	}
	if blacklisted {
		return nil, fmt.Errorf("token has been revoked: %w", interfaces.ErrUnauthorized)
	}

	// Check user-level token revocation.
	revokedBefore, err := s.store.GetUserRevocation(ctx, claims.UserID)
	if err != nil {
		return nil, fmt.Errorf("check user revocation: %w", err)
	}
	if revokedBefore != nil {
		issuedAt := time.Unix(claims.IssuedAt, 0)
		if issuedAt.Before(*revokedBefore) {
			return nil, fmt.Errorf("token revoked by user: %w", interfaces.ErrUnauthorized)
		}
	}

	return claims, nil
}

// RefreshToken exchanges a valid refresh token for a new access token.
func (s *Service) RefreshToken(ctx context.Context, refreshToken string) (*interfaces.TokenPair, error) {
	claims, err := s.jwt.VerifyToken(refreshToken)
	if err != nil {
		return nil, fmt.Errorf("verify refresh token: %w", err)
	}

	// Check if refresh token is blacklisted.
	blacklisted, err := s.store.IsTokenBlacklisted(ctx, claims.TokenID)
	if err != nil {
		return nil, fmt.Errorf("check refresh token blacklist: %w", err)
	}
	if blacklisted {
		return nil, fmt.Errorf("refresh token has been revoked: %w", interfaces.ErrUnauthorized)
	}

	// Get user and role names.
	user, err := s.store.GetUserByID(ctx, claims.UserID)
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}
	if user.Status != "active" {
		return nil, fmt.Errorf("account is %s: %w", user.Status, interfaces.ErrForbidden)
	}

	roleNames, err := s.store.GetUserRoleNames(ctx, user.ID)
	if err != nil {
		return nil, fmt.Errorf("get user roles: %w", err)
	}

	// Generate new access token.
	accessToken, _, err := s.jwt.GenerateAccessToken(ctx, user, roleNames)
	if err != nil {
		return nil, fmt.Errorf("generate access token: %w", err)
	}

	// Optionally rotate refresh token.
	newRefreshToken := refreshToken
	if s.jwt.RotateRefreshTokens() {
		// Blacklist old refresh token.
		expiresAt := time.Unix(claims.ExpiresAt, 0)
		if err := s.store.BlacklistToken(ctx, claims.TokenID, claims.UserID, expiresAt); err != nil {
			s.logger.Warn("failed to blacklist old refresh token", "error", err)
		}

		newRefreshToken, _, err = s.jwt.GenerateRefreshToken(ctx, user)
		if err != nil {
			return nil, fmt.Errorf("generate refresh token: %w", err)
		}
	}

	return &interfaces.TokenPair{
		AccessToken:  accessToken,
		RefreshToken: newRefreshToken,
		ExpiresIn:    int64(s.jwt.AccessTTL().Seconds()),
	}, nil
}

// CreateUser creates a new user account with hashed password.
func (s *Service) CreateUser(ctx context.Context, req *interfaces.CreateUserRequest) (*interfaces.User, error) {
	// Rate limit check by IP.
	ip := extractIP(ctx)
	allowed, retryAfter := s.rateLimiter.Check(ip)
	if !allowed {
		return nil, fmt.Errorf("rate limit exceeded, retry after %d seconds: %w", retryAfter, interfaces.ErrValidation)
	}

	// Validate input.
	if err := validateCreateUser(req); err != nil {
		return nil, err
	}

	// Check for duplicate email.
	existing, err := s.store.GetUserByEmail(ctx, req.Email)
	if err != nil && !errors.Is(err, interfaces.ErrNotFound) {
		return nil, fmt.Errorf("check email: %w", err)
	}
	if existing != nil {
		return nil, fmt.Errorf("email already exists: %w", interfaces.ErrConflict)
	}

	// Check for duplicate username.
	existing, err = s.store.GetUserByUsername(ctx, req.Username)
	if err != nil && !errors.Is(err, interfaces.ErrNotFound) {
		return nil, fmt.Errorf("check username: %w", err)
	}
	if existing != nil {
		return nil, fmt.Errorf("username already exists: %w", interfaces.ErrConflict)
	}

	// Hash password.
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), s.bcryptCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	now := time.Now().UTC()
	user := &interfaces.User{
		ID:           newUUID(),
		Email:        strings.ToLower(strings.TrimSpace(req.Email)),
		Username:     strings.TrimSpace(req.Username),
		PasswordHash: string(hash),
		Status:       "active",
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := s.store.CreateUser(ctx, user); err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}

	// Assign roles.
	rolesToAssign := req.Roles
	if len(rolesToAssign) == 0 {
		rolesToAssign = []string{"viewer"}
	}
	for _, roleName := range rolesToAssign {
		if err := s.rbac.AssignRole(ctx, user.ID, roleName); err != nil {
			s.logger.Warn("failed to assign role",
				"user_id", user.ID, "role", roleName, "error", err,
			)
		}
	}

	// Reset rate limit on successful creation.
	s.rateLimiter.Reset(ip)

	// Strip password hash from response.
	user.PasswordHash = ""
	user.Roles = rolesToAssign

	s.logger.Info("user created", "user_id", user.ID, "username", user.Username)
	return user, nil
}

// GetUser retrieves a user by ID, email, or username.
func (s *Service) GetUser(ctx context.Context, identifier string) (*interfaces.User, error) {
	if identifier == "" {
		return nil, fmt.Errorf("identifier is required: %w", interfaces.ErrValidation)
	}

	// Try as UUID (ID).
	if len(identifier) == 36 && strings.Contains(identifier, "-") {
		user, err := s.store.GetUserByID(ctx, identifier)
		if err == nil {
			user.PasswordHash = ""
			if roles, rerr := s.store.GetUserRoleNames(ctx, user.ID); rerr != nil {
				s.logger.Error("failed to get user roles", "user_id", user.ID, "error", rerr)
			} else {
				user.Roles = roles
			}
			return user, nil
		}
		if !errors.Is(err, interfaces.ErrNotFound) {
			return nil, fmt.Errorf("get user by id: %w", err)
		}
	}

	// Try as email.
	if strings.Contains(identifier, "@") {
		user, err := s.store.GetUserByEmail(ctx, identifier)
		if err == nil {
			user.PasswordHash = ""
			if roles, rerr := s.store.GetUserRoleNames(ctx, user.ID); rerr != nil {
				s.logger.Error("failed to get user roles", "user_id", user.ID, "error", rerr)
			} else {
				user.Roles = roles
			}
			return user, nil
		}
		if !errors.Is(err, interfaces.ErrNotFound) {
			return nil, fmt.Errorf("get user by email: %w", err)
		}
	}

	// Try as username.
	user, err := s.store.GetUserByUsername(ctx, identifier)
	if err != nil {
		if errors.Is(err, interfaces.ErrNotFound) {
			return nil, interfaces.ErrNotFound
		}
		return nil, fmt.Errorf("get user by username: %w", err)
	}
	user.PasswordHash = ""
	if roles, rerr := s.store.GetUserRoleNames(ctx, user.ID); rerr != nil {
		s.logger.Error("failed to get user roles", "user_id", user.ID, "error", rerr)
	} else {
		user.Roles = roles
	}
	return user, nil
}

// HasPermission checks if a user has permission for a resource/action.
func (s *Service) HasPermission(ctx context.Context, userID, resource, action string) (bool, error) {
	return s.rbac.HasPermission(ctx, userID, resource, action)
}

// GetUserPermissions returns all permissions assigned to a user through their roles.
func (s *Service) GetUserPermissions(ctx context.Context, userID string) ([]*interfaces.Permission, error) {
	return s.rbac.store.GetUserPermissions(ctx, userID)
}

// GetUserRoleNames returns the role names assigned to a user.
func (s *Service) GetUserRoleNames(ctx context.Context, userID string) ([]string, error) {
	return s.store.GetUserRoleNames(ctx, userID)
}

// GetRoleByID returns a role by its ID.
func (s *Service) GetRoleByID(ctx context.Context, id string) (*interfaces.Role, error) {
	return s.store.GetRoleByID(ctx, id)
}

// CreateAPIToken creates a new long-lived API token.
func (s *Service) CreateAPIToken(ctx context.Context, userID, name string, expiresAt *time.Time) (*interfaces.APIToken, error) {
	if name == "" {
		return nil, fmt.Errorf("token name is required: %w", interfaces.ErrValidation)
	}

	// Generate a random token.
	tokenBytes := make([]byte, apiTokenBytes)
	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, fmt.Errorf("generate token: %w", err)
	}
	tokenStr := apiTokenPrefix + hex.EncodeToString(tokenBytes)

	// Hash the token for storage using HMAC-SHA256.
	mac := hmac.New(sha256.New, s.hmacKey[:])
	mac.Write([]byte(tokenStr))
	tokenHash := hex.EncodeToString(mac.Sum(nil))

	now := time.Now().UTC()
	apiToken := &interfaces.APIToken{
		ID:        newUUID(),
		UserID:    userID,
		Name:      name,
		TokenHash: tokenHash,
		CreatedAt: now,
		ExpiresAt: expiresAt,
	}

	if err := s.store.CreateAPIToken(ctx, apiToken); err != nil {
		return nil, fmt.Errorf("store api token: %w", err)
	}

	s.logger.Info("api token created", "token_id", apiToken.ID, "user_id", userID, "name", name)

	// Return plaintext token in TokenHash field (only shown once to caller).
	apiToken.TokenHash = tokenStr

	return apiToken, nil
}

// RevokeAPIToken revokes an API token by ID.
func (s *Service) RevokeAPIToken(ctx context.Context, tokenID string) error {
	if err := s.store.RevokeAPIToken(ctx, tokenID); err != nil {
		return fmt.Errorf("revoke api token: %w", err)
	}
	s.logger.Info("api token revoked", "token_id", tokenID)
	return nil
}

// ChangePassword changes a user's password after verifying the current one.
func (s *Service) ChangePassword(ctx context.Context, userID, currentPassword, newPassword string) error {
	user, err := s.store.GetUserByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("get user: %w", err)
	}

	// Verify current password.
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(currentPassword)); err != nil {
		return fmt.Errorf("current password incorrect: %w", interfaces.ErrUnauthorized)
	}

	// Validate new password.
	if len(newPassword) < minPasswordLength {
		return fmt.Errorf("password must be at least %d characters: %w", minPasswordLength, interfaces.ErrValidation)
	}

	// Check new password is different from current.
	if currentPassword == newPassword {
		return fmt.Errorf("new password must be different from current password: %w", interfaces.ErrValidation)
	}

	// Hash new password.
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), s.bcryptCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	user.PasswordHash = string(hash)
	user.UpdatedAt = time.Now().UTC()
	if err := s.store.UpdateUser(ctx, user); err != nil {
		return fmt.Errorf("update user: %w", err)
	}

	// Revoke all existing tokens by setting user revocation timestamp.
	now := time.Now().UTC()
	if err := s.store.SetUserRevocation(ctx, userID, now); err != nil {
		s.logger.Warn("failed to set user token revocation", "user_id", userID, "error", err)
	}

	s.logger.Info("password changed", "user_id", userID)
	return nil
}

// CreateDefaultAdmin creates the default admin user if no users exist.
func (s *Service) CreateDefaultAdmin(ctx context.Context) error {
	count, err := s.store.CountUsers(ctx)
	if err != nil {
		return fmt.Errorf("count users: %w", err)
	}
	if count > 0 {
		return nil
	}

	adminReq := &interfaces.CreateUserRequest{
		Email:    s.adminEmail,
		Username: "admin",
		Password: s.adminPass,
		Roles:    []string{"admin"},
	}

	user, err := s.CreateUser(ctx, adminReq)
	if err != nil {
		return fmt.Errorf("create default admin: %w", err)
	}

	s.logger.Info("default admin user created",
		"user_id", user.ID,
		"email", adminReq.Email,
		"message", "please change the default password immediately",
	)
	return nil
}

// VerifyAPIToken verifies an API token string and returns synthetic user claims.
func (s *Service) VerifyAPIToken(ctx context.Context, tokenString string) (*interfaces.UserClaims, error) {
	// Hash the token using HMAC-SHA256 to look it up.
	mac := hmac.New(sha256.New, s.hmacKey[:])
	mac.Write([]byte(tokenString))
	tokenHash := hex.EncodeToString(mac.Sum(nil))

	apiToken, err := s.store.GetAPITokenByHash(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, interfaces.ErrNotFound) {
			return nil, interfaces.ErrUnauthorized
		}
		return nil, fmt.Errorf("lookup api token: %w", err)
	}

	// Check expiration.
	if apiToken.ExpiresAt != nil && time.Now().UTC().After(*apiToken.ExpiresAt) {
		return nil, fmt.Errorf("api token expired: %w", interfaces.ErrUnauthorized)
	}

	// Update last used timestamp asynchronously.
	s.bgWg.Add(1)
	go func() {
		defer s.bgWg.Done()
		bgCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.store.UpdateAPITokenLastUsed(bgCtx, apiToken.ID); err != nil {
			s.logger.Warn("failed to update api token last used", "token_id", apiToken.ID, "error", err)
		}
	}()

	// Get user roles for claims.
	roleNames, err := s.store.GetUserRoleNames(ctx, apiToken.UserID)
	if err != nil {
		return nil, fmt.Errorf("get user roles: %w", err)
	}

	now := time.Now().UTC()
	return &interfaces.UserClaims{
		UserID:    apiToken.UserID,
		Roles:     roleNames,
		ExpiresAt: now.Add(1 * time.Hour).Unix(),
		IssuedAt:  now.Unix(),
		TokenID:   apiToken.ID,
	}, nil
}

// validateCreateUser validates the fields of a CreateUserRequest.
func validateCreateUser(req *interfaces.CreateUserRequest) error {
	if req.Email == "" {
		return fmt.Errorf("email is required: %w", interfaces.ErrValidation)
	}
	if _, err := mail.ParseAddress(req.Email); err != nil {
		return fmt.Errorf("invalid email format: %w", interfaces.ErrValidation)
	}
	if req.Username == "" {
		return fmt.Errorf("username is required: %w", interfaces.ErrValidation)
	}
	if len(req.Username) < 3 {
		return fmt.Errorf("username must be at least 3 characters: %w", interfaces.ErrValidation)
	}
	if req.Password == "" {
		return fmt.Errorf("password is required: %w", interfaces.ErrValidation)
	}
	if len(req.Password) < minPasswordLength {
		return fmt.Errorf("password must be at least %d characters: %w", minPasswordLength, interfaces.ErrValidation)
	}
	return nil
}

func (s *Service) UpdateUser(ctx context.Context, id string, req *interfaces.UpdateUserRequest) (*interfaces.User, error) {
	user, err := s.store.GetUserByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get user for update: %w", err)
	}

	if req.Email != nil {
		if _, err := mail.ParseAddress(*req.Email); err != nil {
			return nil, fmt.Errorf("invalid email format: %w", interfaces.ErrValidation)
		}
		user.Email = *req.Email
	}
	if req.Username != nil {
		if len(*req.Username) < 3 {
			return nil, fmt.Errorf("username must be at least 3 characters: %w", interfaces.ErrValidation)
		}
		user.Username = *req.Username
	}
	if req.Password != nil {
		if len(*req.Password) < minPasswordLength {
			return nil, fmt.Errorf("password must be at least %d characters: %w", minPasswordLength, interfaces.ErrValidation)
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(*req.Password), defaultBCryptCost)
		if err != nil {
			return nil, fmt.Errorf("hash password: %w", err)
		}
		user.PasswordHash = string(hash)
	}
	if req.Status != nil {
		user.Status = *req.Status
	}
	user.UpdatedAt = time.Now().UTC()

	if err := s.store.UpdateUser(ctx, user); err != nil {
		return nil, fmt.Errorf("update user: %w", err)
	}

	if len(req.Roles) > 0 {
		existingRoles, err := s.store.GetUserRoleNames(ctx, id)
		if err != nil {
			s.logger.Error("failed to get user roles for update", "error", err)
		} else {
			// Remove roles that are no longer in the request.
			newRoleSet := make(map[string]bool, len(req.Roles))
			for _, r := range req.Roles {
				newRoleSet[r] = true
			}
			for _, r := range existingRoles {
				if !newRoleSet[r] {
					if rmErr := s.rbac.RemoveRole(ctx, id, r); rmErr != nil {
						s.logger.Error("failed to remove user role", "role", r, "error", rmErr)
					}
				}
			}
			// Add roles that are new.
			existingSet := make(map[string]bool, len(existingRoles))
			for _, r := range existingRoles {
				existingSet[r] = true
			}
			for _, r := range req.Roles {
				if !existingSet[r] {
					role, err := s.store.GetRoleByName(ctx, r)
					if err == nil {
						if addErr := s.store.AddUserRole(ctx, id, role.ID); addErr != nil {
							s.logger.Error("failed to add user role", "role", r, "error", addErr)
						}
					}
				}
			}
		}
	}

	user.Roles, err = s.store.GetUserRoleNames(ctx, id)
	if err != nil {
		s.logger.Error("failed to get user roles after update", "error", err)
	}

	return user, nil
}

func (s *Service) DeleteUser(ctx context.Context, id string) error {
	now := time.Now().UTC()
	if err := s.store.SetUserRevocation(ctx, id, now); err != nil {
		return fmt.Errorf("revoke user: %w", err)
	}
	return nil
}

func (s *Service) ListUsers(ctx context.Context, query *interfaces.UserQuery) (*interfaces.Page, error) {
	if query == nil {
		query = &interfaces.UserQuery{Page: 1, PerPage: 20}
	}
	if query.Page < 1 {
		query.Page = 1
	}
	if query.PerPage < 1 || query.PerPage > 100 {
		query.PerPage = 20
	}
	return s.store.ListUsers(ctx, query)
}

// ListRoles returns all roles in the system.
func (s *Service) ListRoles(ctx context.Context) ([]*interfaces.Role, error) {
	return s.rbac.ListAllRoles(ctx)
}

// UpdateRole updates a role's description and permissions by ID.
func (s *Service) UpdateRole(ctx context.Context, id string, description string, permissionNames []string) (*interfaces.Role, error) {
	role, err := s.store.GetRoleByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get role: %w", err)
	}

	if description != "" {
		role.Description = description
	}
	role.UpdatedAt = time.Now().UTC()

	if err := s.store.UpdateRole(ctx, role); err != nil {
		return nil, fmt.Errorf("update role: %w", err)
	}

	if len(permissionNames) > 0 {
		existingPerms, err := s.store.GetPermissionsByRoleID(ctx, id)
		if err != nil {
			s.logger.Warn("failed to get existing permissions for role update", "role_id", id, "error", err)
		}
		for _, p := range existingPerms {
			if err := s.store.RemoveRolePermission(ctx, id, p.ID); err != nil {
				s.logger.Warn("failed to remove permission from role", "role_id", id, "perm_id", p.ID, "error", err)
			}
		}

		for _, permName := range permissionNames {
			if err := s.rbac.AssignPermission(ctx, role.Name, parsePermissionResource(permName), parsePermissionAction(permName)); err != nil {
				s.logger.Warn("failed to assign permission to role", "role", role.Name, "perm", permName, "error", err)
			}
		}
	}

	perms, err := s.store.GetPermissionsByRoleID(ctx, id)
	if err != nil {
		s.logger.Warn("failed to get permissions for role response", "role_id", id, "error", err)
	}
	role.Permissions = make([]string, 0, len(perms))
	for _, p := range perms {
		role.Permissions = append(role.Permissions, p.Name)
	}

	return role, nil
}

// ListAPITokens returns all API tokens for a user.
func (s *Service) ListAPITokens(ctx context.Context, userID string) ([]*interfaces.APIToken, error) {
	return s.store.ListAPITokensByUser(ctx, userID)
}

// parsePermissionResource extracts the resource part from "resource.action".
func parsePermissionResource(name string) string {
	parts := strings.SplitN(name, ".", 2)
	if len(parts) == 2 {
		return parts[0]
	}
	return name
}

// parsePermissionAction extracts the action part from "resource.action".
func parsePermissionAction(name string) string {
	parts := strings.SplitN(name, ".", 2)
	if len(parts) == 2 {
		return parts[1]
	}
	return "*"
}

func extractIP(ctx context.Context) string {
	if ip, ok := ctx.Value(ContextKeyRemoteIP).(string); ok && ip != "" {
		return ip
	}
	return "unknown"
}

// isViewerOnly returns true if the user's only role is "viewer".
func isViewerOnly(roles []string) bool {
	if len(roles) != 1 {
		return false
	}
	return roles[0] == "viewer"
}
