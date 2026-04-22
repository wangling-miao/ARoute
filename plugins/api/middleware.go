package api

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/wangling-miao/aroute/core"
	"github.com/wangling-miao/aroute/sdk/interfaces"
)

type contextKey string

const (
	claimsContextKey contextKey = "user_claims"
	authHeaderPrefix            = "Bearer "
)

// userClaimsFromRequest extracts UserClaims from request context.
func userClaimsFromRequest(r *http.Request) *interfaces.UserClaims {
	val := r.Context().Value(claimsContextKey)
	if val == nil {
		return nil
	}
	claims, ok := val.(*interfaces.UserClaims)
	if !ok {
		return nil
	}
	return claims
}

// authMiddleware creates JWT authentication middleware using AuthService.
// If authSvc is nil, the middleware is a no-op (public API mode).
// publicPaths are routes that skip authentication entirely.
var publicPaths = map[string]bool{
	"POST /api/v1/auth/login":    true,
	"POST /api/v1/auth/refresh":  true,
	"POST /api/v1/users":         true, // user registration
}

func authMiddleware(authSvc interfaces.AuthService) func(http.Handler) http.Handler {
	if authSvc == nil {
		return func(next http.Handler) http.Handler {
			return next
		}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if publicPaths[r.Method+" "+r.URL.Path] {
				next.ServeHTTP(w, r)
				return
			}

			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "missing authorization header")
				return
			}

			if !strings.HasPrefix(authHeader, authHeaderPrefix) {
				writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "invalid authorization header format")
				return
			}

			token := strings.TrimPrefix(authHeader, authHeaderPrefix)
			if token == "" {
				writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "empty bearer token")
				return
			}

			claims, err := authSvc.VerifyToken(r.Context(), token)
			if err != nil {
				if errors.Is(err, interfaces.ErrUnauthorized) {
					writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "invalid or expired token")
					return
				}
				writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "token verification failed")
				return
			}

			ctx := context.WithValue(r.Context(), claimsContextKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// requirePermission creates middleware that checks a specific permission.
func requirePermission(authSvc interfaces.AuthService, resource, action string) func(http.Handler) http.Handler {
	if authSvc == nil {
		return func(next http.Handler) http.Handler {
			return next
		}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims := userClaimsFromRequest(r)
			if claims == nil {
				writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
				return
			}

			allowed, err := authSvc.HasPermission(r.Context(), claims.UserID, resource, action)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "permission check failed")
				return
			}

			if !allowed {
				writeError(w, http.StatusForbidden, "FORBIDDEN", "insufficient permissions")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// authMiddlewareWithPublicRead wraps authMiddleware to skip authentication
// for GET requests when publicRead is true. When publicRead is false,
// it behaves identically to authMiddleware (all methods require auth).
func authMiddlewareWithPublicRead(authSvc interfaces.AuthService, publicRead bool) func(http.Handler) http.Handler {
	if !publicRead {
		return authMiddleware(authSvc)
	}

	// When publicRead is true but authSvc is nil, all requests pass through
	// (public API mode). authMiddleware(nil) is already a no-op.
	if authSvc == nil {
		return authMiddleware(nil)
	}

	// publicRead=true with a real auth service: skip auth for GET, enforce for others.
	inner := authMiddleware(authSvc)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet {
				next.ServeHTTP(w, r)
				return
			}
			inner(next).ServeHTTP(w, r)
		})
	}
}

// contentNegotiationMiddleware enforces Accept: application/json and
// Content-Type: application/json on write requests per the spec.
func contentNegotiationMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			accept := r.Header.Get("Accept")
			if accept != "" && accept != "application/json" && !strings.Contains(accept, "*/*") {
				writeError(w, http.StatusNotAcceptable, "NOT_ACCEPTABLE", "only application/json is supported")
				return
			}

			if r.Method == http.MethodPost || r.Method == http.MethodPut {
				ct := r.Header.Get("Content-Type")
				if ct != "" && !strings.HasPrefix(ct, "application/json") {
					writeError(w, http.StatusUnsupportedMediaType, "UNSUPPORTED_MEDIA_TYPE", "Content-Type must be application/json")
					return
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}

// perContentTypeAuthMiddleware checks per-content-type auth overrides.
// If a content type has auth disabled via config (content_types.{slug}.auth_required = false),
// all methods skip auth. Otherwise, the global publicRead policy applies.
func perContentTypeAuthMiddleware(authSvc interfaces.AuthService, publicRead bool, config core.ConfigProvider) func(http.Handler) http.Handler {
	globalAuth := authMiddlewareWithPublicRead(authSvc, publicRead)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Only apply auth to /api/ routes — skip admin UI and others.
			if !strings.HasPrefix(r.URL.Path, "/api/") {
				next.ServeHTTP(w, r)
				return
			}

			contentType := chi.URLParam(r, "contentType")
			if contentType == "" {
				globalAuth(next).ServeHTTP(w, r)
				return
			}

			if config != nil {
				key := "content_types." + contentType + ".auth_required"
				if v, ok := config.Get(key).(bool); ok && !v {
					next.ServeHTTP(w, r)
					return
				}
			}

			globalAuth(next).ServeHTTP(w, r)
		})
	}
}
