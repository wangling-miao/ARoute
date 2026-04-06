package interfaces

import (
	"context"
	"time"
)

// AuthService defines authentication and authorization operations.
// It handles user authentication, JWT token management, RBAC permissions,
// and API token lifecycle.
type AuthService interface {
	// Authenticate validates user credentials and returns access/refresh tokens.
	Authenticate(ctx context.Context, req *AuthRequest) (*AuthResult, error)

	// VerifyToken validates a JWT access token and returns the user claims.
	VerifyToken(ctx context.Context, token string) (*UserClaims, error)

	// RefreshToken exchanges a valid refresh token for a new access token.
	RefreshToken(ctx context.Context, refreshToken string) (*TokenPair, error)

	// CreateUser creates a new user account with hashed password.
	CreateUser(ctx context.Context, req *CreateUserRequest) (*User, error)

	// GetUser retrieves a user by ID or email.
	GetUser(ctx context.Context, identifier string) (*User, error)

	// HasPermission checks if a user has permission to perform an action on a resource.
	HasPermission(ctx context.Context, userID string, resource, action string) (bool, error)

	// CreateAPIToken creates a long-lived API token for programmatic access.
	CreateAPIToken(ctx context.Context, userID string, name string, expiresAt *time.Time) (*APIToken, error)

	// RevokeAPIToken revokes an API token by ID.
	RevokeAPIToken(ctx context.Context, tokenID string) error
}

// AuthRequest contains authentication credentials.
type AuthRequest struct {
	// Email is the user's email address.
	Email string `json:"email"`

	// Password is the user's password (plaintext, will be hashed during verification).
	Password string `json:"password"`
}

// AuthResult contains the result of a successful authentication.
type AuthResult struct {
	// AccessToken is the JWT access token for API requests.
	AccessToken string `json:"access_token"`

	// RefreshToken is used to obtain new access tokens.
	RefreshToken string `json:"refresh_token"`

	// TokenType is typically "Bearer".
	TokenType string `json:"token_type"`

	// ExpiresIn is the access token lifetime in seconds.
	ExpiresIn int64 `json:"expires_in"`

	// User contains basic user information.
	User *User `json:"user"`
}

// TokenPair represents an access/refresh token pair.
type TokenPair struct {
	// AccessToken is the JWT access token.
	AccessToken string `json:"access_token"`

	// RefreshToken is the refresh token.
	RefreshToken string `json:"refresh_token"`

	// ExpiresIn is the access token expiration in seconds.
	ExpiresIn int64 `json:"expires_in"`
}

// UserClaims contains the claims extracted from a JWT token.
type UserClaims struct {
	// UserID is the unique user identifier.
	UserID string `json:"sub"`

	// Email is the user's email address.
	Email string `json:"email"`

	// Roles is the list of role names assigned to the user.
	Roles []string `json:"roles"`

	// ExpiresAt is the token expiration timestamp.
	ExpiresAt int64 `json:"exp"`

	// IssuedAt is the token issuance timestamp.
	IssuedAt int64 `json:"iat"`

	// TokenID is a unique identifier for this token (jti claim).
	TokenID string `json:"jti"`
}

// CreateUserRequest contains parameters for creating a new user.
type CreateUserRequest struct {
	// Email is the user's email address (required, unique).
	Email string `json:"email"`

	// Username is the user's display name (required, unique).
	Username string `json:"username"`

	// Password is the user's password (will be hashed).
	Password string `json:"password"`

	// Roles is the list of role names to assign (optional).
	Roles []string `json:"roles,omitempty"`
}

// APIToken represents a long-lived API token.
type APIToken struct {
	// ID is the unique token identifier.
	ID string `json:"id"`

	// UserID is the owner of this token.
	UserID string `json:"user_id"`

	// Name is a human-readable token name.
	Name string `json:"name"`

	// TokenHash is the hashed token value (for storage).
	// The actual token is only shown once when created.
	TokenHash string `json:"-"`

	// CreatedAt is when the token was created.
	CreatedAt time.Time `json:"created_at"`

	// ExpiresAt is the optional token expiration time.
	ExpiresAt *time.Time `json:"expires_at,omitempty"`

	// LastUsedAt is when the token was last used.
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
}
