package cache

import (
	"context"
	"net/http"
	"strings"
	"time"
)

// contextKey is an unexported type for context keys defined in this package.
type contextKey string

const bypassKey contextKey = "cache_bypass"

// BypassMiddleware returns HTTP middleware that checks for Cache-Control: no-cache
// header. When present, it sets a context value indicating cache should be bypassed.
// The middleware works with cache-aware handlers that check IsBypassed().
func BypassMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isNoCacheRequest(r) {
			ctx := context.WithValue(r.Context(), bypassKey, true)
			r = r.WithContext(ctx)
		}
		next.ServeHTTP(w, r)
	})
}

// IsBypassed checks if the cache should be bypassed for this request context.
func IsBypassed(ctx context.Context) bool {
	val, ok := ctx.Value(bypassKey).(bool)
	return ok && val
}

// isNoCacheRequest checks if the request has Cache-Control: no-cache header.
func isNoCacheRequest(r *http.Request) bool {
	cc := r.Header.Get("Cache-Control")
	if cc == "" {
		return false
	}
	for _, directive := range strings.Split(cc, ",") {
		if strings.TrimSpace(directive) == "no-cache" {
			return true
		}
	}
	return false
}

// SetCachedValue stores a value in the cache with TTL, respecting bypass.
// If bypass is active, it STILL stores the value (spec: fresh response stored
// for subsequent requests).
func (s *Service) SetCachedValue(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	return s.Set(ctx, key, value, ttl)
}

// GetCachedValue retrieves from cache unless bypass is active.
// Returns (nil, false) when bypassed, forcing origin fetch.
func (s *Service) GetCachedValue(ctx context.Context, key string) (interface{}, bool) {
	if IsBypassed(ctx) {
		return nil, false
	}
	return s.Get(ctx, key)
}
