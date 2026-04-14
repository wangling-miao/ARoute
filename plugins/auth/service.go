package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/mail"
	"strings"
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
	store       *Store
	jwt         *JWTManager
	rbac        *RBACManager
	rateLimiter *RateLimiter
	logger      *slog.Logger
	config      core.ConfigProvider
	bcryptCost  int
	adminEmail  string
	adminPass   string
}

// NewService creates a new auth Service.
func NewService(store *Store, jwt *JWTManager, rbac *RBACManager, rateLimiter *RateLimiter, logger *slog.Logger, config core.ConfigProvider, authCfg authConfig) *Service {
	cost := authCfg.bcryptCost
	if cost < 10 {
		logger.Warn("bcrypt cost below minimum, using 10", "configured", cost)
		cost = 10
	}
	return &Service{
		store:       store,
		jwt:         jwt,
		rbac:        rbac,
		rateLimiter: rateLimiter,
		logger:      logger,
		config:      config,
		bcryptCost:  cost,
		adminEmail:  authCfg.adminEmail,
		adminPass:   authCfg.adminPassword,
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
		if err == interfaces.ErrNotFound {
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
	if err != nil && err != interfaces.ErrNotFound {
		return nil, fmt.Errorf("check email: %w", err)
	}
	if existing != nil {
		return nil, fmt.Errorf("email already exists: %w", interfaces.ErrConflict)
	}

	// Check for duplicate username.
	existing, err = s.store.GetUserByUsername(ctx, req.Username)
	if err != nil && err != interfaces.ErrNotFound {
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
			user.Roles, _ = s.store.GetUserRoleNames(ctx, user.ID)
			return user, nil
		}
		if err != interfaces.ErrNotFound {
			return nil, fmt.Errorf("get user by id: %w", err)
		}
	}

	// Try as email.
	if strings.Contains(identifier, "@") {
		user, err := s.store.GetUserByEmail(ctx, identifier)
		if err == nil {
			user.PasswordHash = ""
			user.Roles, _ = s.store.GetUserRoleNames(ctx, user.ID)
			return user, nil
		}
		if err != interfaces.ErrNotFound {
			return nil, fmt.Errorf("get user by email: %w", err)
		}
	}

	// Try as username.
	user, err := s.store.GetUserByUsername(ctx, identifier)
	if err != nil {
		if err == interfaces.ErrNotFound {
			return nil, interfaces.ErrNotFound
		}
		return nil, fmt.Errorf("get user by username: %w", err)
	}
	user.PasswordHash = ""
	user.Roles, _ = s.store.GetUserRoleNames(ctx, user.ID)
	return user, nil
}

// HasPermission checks if a user has permission for a resource/action.
func (s *Service) HasPermission(ctx context.Context, userID, resource, action string) (bool, error) {
	return s.rbac.HasPermission(ctx, userID, resource, action)
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

	// Hash the token for storage.
	h := sha256.Sum256([]byte(tokenStr))
	tokenHash := hex.EncodeToString(h[:])

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
	// Hash the token to look it up.
	h := sha256.Sum256([]byte(tokenString))
	tokenHash := hex.EncodeToString(h[:])

	apiToken, err := s.store.GetAPITokenByHash(ctx, tokenHash)
	if err != nil {
		if err == interfaces.ErrNotFound {
			return nil, interfaces.ErrUnauthorized
		}
		return nil, fmt.Errorf("lookup api token: %w", err)
	}

	// Check expiration.
	if apiToken.ExpiresAt != nil && time.Now().UTC().After(*apiToken.ExpiresAt) {
		return nil, fmt.Errorf("api token expired: %w", interfaces.ErrUnauthorized)
	}

	// Update last used timestamp asynchronously.
	go func() {
		bgCtx := context.Background()
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

// extractIP extracts an IP address from context or returns a default.
func extractIP(_ context.Context) string {
	// In a real implementation, extract from request context.
	// For now, return a placeholder — the middleware will set the real IP.
	return "default"
}
