package auth

import (
	"context"
	"crypto/rsa"
	"fmt"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/wangling-miao/aroute/sdk/interfaces"
)

// JWTManager handles JWT token generation and verification.
type JWTManager struct {
	secretKey           []byte
	privateKey          *rsa.PrivateKey
	publicKey           *rsa.PublicKey
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
// For RSA algorithms (RS256/RS384/RS512), it loads the key pair from PEM files.
func NewJWTManager(config authConfig) (*JWTManager, error) {
	m := &JWTManager{
		secretKey:           []byte(config.jwtSecret),
		algorithm:           config.jwtAlgorithm,
		accessTokenTTL:      config.accessTokenTTL,
		refreshTokenTTL:     config.refreshTokenTTL,
		rotateRefreshTokens: config.rotateRefreshTokens,
	}

	if isRSAAlgorithm(config.jwtAlgorithm) {
		if config.jwtPrivateKeyPath == "" {
			return nil, fmt.Errorf("auth: jwt_private_key_path is required for algorithm %q", config.jwtAlgorithm)
		}
		if config.jwtPublicKeyPath == "" {
			return nil, fmt.Errorf("auth: jwt_public_key_path is required for algorithm %q", config.jwtAlgorithm)
		}

		privKey, err := loadPrivateKey(config.jwtPrivateKeyPath)
		if err != nil {
			return nil, fmt.Errorf("auth: load private key: %w", err)
		}
		m.privateKey = privKey

		pubKey, err := loadPublicKey(config.jwtPublicKeyPath)
		if err != nil {
			return nil, fmt.Errorf("auth: load public key: %w", err)
		}
		m.publicKey = pubKey
	}

	return m, nil
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

	token := jwt.NewWithClaims(m.signingMethod(), claims)
	tokenString, err := token.SignedString(m.signKey())
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

	token := jwt.NewWithClaims(m.signingMethod(), claims)
	tokenString, err := token.SignedString(m.signKey())
	if err != nil {
		return "", "", fmt.Errorf("sign refresh token: %w", err)
	}

	return tokenString, jti, nil
}

// VerifyToken parses and verifies a JWT token string, returning the user claims.
func (m *JWTManager) VerifyToken(tokenString string) (*interfaces.UserClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &jwtClaims{}, func(token *jwt.Token) (interface{}, error) {
		switch token.Method.(type) {
		case *jwt.SigningMethodRSA:
			if m.publicKey == nil {
				return nil, fmt.Errorf("RSA public key not configured")
			}
			return m.publicKey, nil
		case *jwt.SigningMethodHMAC:
			return m.secretKey, nil
		default:
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
	})
	if err != nil {
		return nil, fmt.Errorf("parse token: %w: %w", err, interfaces.ErrUnauthorized)
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

// signingMethod returns the jwt.SigningMethod matching the configured algorithm.
func (m *JWTManager) signingMethod() jwt.SigningMethod {
	switch m.algorithm {
	case "RS256":
		return jwt.SigningMethodRS256
	case "RS384":
		return jwt.SigningMethodRS384
	case "RS512":
		return jwt.SigningMethodRS512
	case "HS384":
		return jwt.SigningMethodHS384
	case "HS512":
		return jwt.SigningMethodHS512
	default:
		return jwt.SigningMethodHS256
	}
}

// signKey returns the key used for signing tokens.
func (m *JWTManager) signKey() interface{} {
	if m.privateKey != nil {
		return m.privateKey
	}
	return m.secretKey
}

func isRSAAlgorithm(alg string) bool {
	return alg == "RS256" || alg == "RS384" || alg == "RS512"
}

func loadPrivateKey(path string) (*rsa.PrivateKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	key, err := jwt.ParseRSAPrivateKeyFromPEM(data)
	if err != nil {
		return nil, fmt.Errorf("parse RSA private key from %s: %w", path, err)
	}
	return key, nil
}

func loadPublicKey(path string) (*rsa.PublicKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	key, err := jwt.ParseRSAPublicKeyFromPEM(data)
	if err != nil {
		return nil, fmt.Errorf("parse RSA public key from %s: %w", path, err)
	}
	return key, nil
}
