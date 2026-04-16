package auth

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/wangling-miao/aroute/sdk/interfaces"
)

// contextKey is used for storing auth context values in request context.
type contextKey string

const (
	// ContextKeyClaims stores UserClaims in the request context.
	ContextKeyClaims contextKey = "auth_claims"
	// ContextKeyUserID stores the user ID in the request context.
	ContextKeyUserID contextKey = "auth_user_id"
	// ContextKeyRemoteIP stores the client IP address in the request context.
	ContextKeyRemoteIP contextKey = "auth_remote_ip"
)

// RBACMiddleware returns a chi-compatible middleware that authenticates requests
// via JWT or API token and checks RBAC permissions.
func RBACMiddleware(svc *Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, err := authenticateRequest(r, svc)
			if err != nil {
				http.Error(w, `{"error":"unauthorized","message":"invalid or missing token"}`, http.StatusUnauthorized)
				return
			}

			remoteIP := r.RemoteAddr
			if rip := r.Header.Get("X-Real-IP"); rip != "" {
				remoteIP = rip
			} else if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
				if idx := strings.Index(xff, ","); idx != -1 {
					remoteIP = strings.TrimSpace(xff[:idx])
				} else {
					remoteIP = strings.TrimSpace(xff)
				}
			}
			if idx := strings.LastIndex(remoteIP, ":"); idx != -1 {
				remoteIP = remoteIP[:idx]
			}

			ctx := context.WithValue(r.Context(), ContextKeyClaims, claims)
			ctx = context.WithValue(ctx, ContextKeyUserID, claims.UserID)
			ctx = context.WithValue(ctx, ContextKeyRemoteIP, remoteIP)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequirePermission returns middleware that checks if the authenticated user has
// the specified permission. Must be used after RBACMiddleware.
func RequirePermission(svc *Service, resource, action string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := r.Context().Value(ContextKeyClaims).(*interfaces.UserClaims)
			if !ok || claims == nil {
				http.Error(w, `{"error":"unauthorized","message":"authentication required"}`, http.StatusUnauthorized)
				return
			}

			allowed, err := svc.HasPermission(r.Context(), claims.UserID, resource, action)
			if err != nil {
				http.Error(w, `{"error":"internal_error","message":"permission check failed"}`, http.StatusInternalServerError)
				return
			}
			if !allowed {
				http.Error(w, `{"error":"forbidden","message":"insufficient permissions"}`, http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// authenticateRequest extracts and validates the token from the Authorization header.
// It supports both Bearer tokens (JWT) and API tokens.
func authenticateRequest(r *http.Request, svc *Service) (*interfaces.UserClaims, error) {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return nil, interfaces.ErrUnauthorized
	}

	// Check for Bearer token.
	if strings.HasPrefix(authHeader, "Bearer ") {
		tokenStr := strings.TrimPrefix(authHeader, "Bearer ")

		// First try API token authentication (longer, opaque tokens).
		if len(tokenStr) > 40 && strings.HasPrefix(tokenStr, "aroute_") {
			return authenticateAPIToken(r.Context(), svc, tokenStr)
		}

		// Then try JWT authentication.
		return svc.VerifyToken(r.Context(), tokenStr)
	}

	return nil, interfaces.ErrUnauthorized
}

// authenticateAPIToken validates an API token and returns synthetic claims.
func authenticateAPIToken(ctx context.Context, svc *Service, tokenStr string) (*interfaces.UserClaims, error) {
	claims, err := svc.VerifyAPIToken(ctx, tokenStr)
	if err != nil {
		return nil, err
	}
	return claims, nil
}

// GetClaimsFromContext extracts UserClaims from the request context.
func GetClaimsFromContext(ctx context.Context) *interfaces.UserClaims {
	claims, _ := ctx.Value(ContextKeyClaims).(*interfaces.UserClaims)
	return claims
}

// GetUserIDFromContext extracts the user ID from the request context.
func GetUserIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(ContextKeyUserID).(string)
	return id
}

// IsRateLimitError checks if an error is a rate limit error and returns the retry-after duration.
func IsRateLimitError(err error) (bool, int) {
	if err == nil {
		return false, 0
	}
	msg := err.Error()
	if !strings.Contains(msg, "rate limit exceeded") {
		return false, 0
	}
	// Extract retry-after seconds from error message.
	// Expected format: "rate limit exceeded, retry after X seconds..."
	parts := strings.Split(msg, "retry after ")
	if len(parts) >= 2 {
		var seconds int
		fmt.Sscanf(parts[1], "%d", &seconds)
		if seconds > 0 {
			return true, seconds
		}
	}
	return true, 60 // default retry after
}

// WriteRateLimitError writes a 429 Too Many Requests response with Retry-After header.
func WriteRateLimitError(w http.ResponseWriter, retryAfterSeconds int) {
	w.Header().Set("Retry-After", fmt.Sprintf("%d", retryAfterSeconds))
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusTooManyRequests)
	fmt.Fprintf(w, `{"error":"rate_limit_exceeded","message":"too many attempts, retry after %d seconds"}`, retryAfterSeconds)
}
