package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/wangling-miao/aroute/sdk/interfaces"
)

// JWTManager handles JWT token generation and verification.
type JWTManager struct {
	secretKey           []byte
	algorithm           string
	accessTokenTTL      time.Duration
	refreshTokenTTL     time.Duration
	rotateRefreshTokens bool
}

// jwtClaims extends the standard JWT claims with user information.
type jwtClaims struct {
	jwt.RegisteredClaims
	Email    string   `json:"email"`
	Username string   `json:"username"`
	Roles    []string `json:"roles"`
}

// NewJWTManager creates a new JWTManager from configuration.
// It reads auth.jwt.* config keys for customization.
func NewJWTManager(config authConfig) *JWTManager {
	return &JWTManager{
		secretKey:           []byte(config.jwtSecret),
		algorithm:           config.jwtAlgorithm,
		accessTokenTTL:      config.accessTokenTTL,
		refreshTokenTTL:     config.refreshTokenTTL,
		rotateRefreshTokens: config.rotateRefreshTokens,
	}
}

// GenerateAccessToken creates a new JWT access token for the given user.
// Returns the signed token string, the token ID (jti), and any error.
func (m *JWTManager) GenerateAccessToken(_ context.Context, user *interfaces.User, roleNames []string) (string, string, error) {
	now := time.Now().UTC()
	jti := uuid.New().String()
	expiresAt := now.Add(m.accessTokenTTL)

	claims := jwtClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   user.ID,
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(now),
			ID:        jti,
		},
		Email:    user.Email,
		Username: user.Username,
		Roles:    roleNames,
	}

	// TODO(future): Support RS256 signing by checking config.algorithm and loading
	// a private key from file. Currently only HS256 is supported.
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(m.secretKey)
	if err != nil {
		return "", "", fmt.Errorf("sign access token: %w", err)
	}

	return tokenString, jti, nil
}

// GenerateRefreshToken creates a new JWT refresh token for the given user.
// Returns the signed token string, the token ID (jti), and any error.
func (m *JWTManager) GenerateRefreshToken(_ context.Context, user *interfaces.User) (string, string, error) {
	now := time.Now().UTC()
	jti := uuid.New().String()
	expiresAt := now.Add(m.refreshTokenTTL)

	claims := jwtClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   user.ID,
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(now),
			ID:        jti,
		},
		Email:    user.Email,
		Username: user.Username,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(m.secretKey)
	if err != nil {
		return "", "", fmt.Errorf("sign refresh token: %w", err)
	}

	return tokenString, jti, nil
}

// VerifyToken parses and verifies a JWT token string, returning the user claims.
func (m *JWTManager) VerifyToken(tokenString string) (*interfaces.UserClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &jwtClaims{}, func(token *jwt.Token) (interface{}, error) {
		// Verify the signing method matches expectations.
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return m.secretKey, nil
	})
	if err != nil {
		return nil, fmt.Errorf("parse token: %w", err)
	}

	claims, ok := token.Claims.(*jwtClaims)
	if !ok || !token.Valid {
		return nil, interfaces.ErrUnauthorized
	}

	return &interfaces.UserClaims{
		UserID:    claims.Subject,
		Email:     claims.Email,
		Roles:     claims.Roles,
		ExpiresAt: claims.ExpiresAt.Unix(),
		IssuedAt:  claims.IssuedAt.Unix(),
		TokenID:   claims.ID,
	}, nil
}

// AccessTTL returns the configured access token TTL.
func (m *JWTManager) AccessTTL() time.Duration {
	return m.accessTokenTTL
}

// RotateRefreshTokens returns whether refresh tokens should be rotated.
func (m *JWTManager) RotateRefreshTokens() bool {
	return m.rotateRefreshTokens
}

// RefreshTTL returns the configured refresh token TTL.
func (m *JWTManager) RefreshTTL() time.Duration {
	return m.refreshTokenTTL
}
