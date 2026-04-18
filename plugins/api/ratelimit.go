package api

import (
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/wangling-miao/aroute/core"
)

// rateLimitEntry tracks request counts within a sliding one-minute window.
type rateLimitEntry struct {
	mu        sync.Mutex
	count     int
	windowEnd time.Time
}

// rateLimiter provides thread-safe per-key rate limiting with a fixed window.
type rateLimiter struct {
	mu      sync.RWMutex
	entries map[string]*rateLimitEntry
	limit   int
	window  time.Duration
}

func newRateLimiter(limit int, window time.Duration) *rateLimiter {
	return &rateLimiter{
		entries: make(map[string]*rateLimitEntry),
		limit:   limit,
		window:  window,
	}
}

// allow checks whether a request with the given key is allowed.
// It returns (allowed bool, remaining int, resetTime time.Time).
func (rl *rateLimiter) allow(key string) (bool, int, time.Time) {
	now := time.Now()
	windowEnd := now.Truncate(rl.window).Add(rl.window)

	rl.mu.Lock()
	entry, exists := rl.entries[key]
	if !exists {
		entry = &rateLimitEntry{windowEnd: windowEnd}
		rl.entries[key] = entry
	}
	rl.mu.Unlock()

	entry.mu.Lock()
	defer entry.mu.Unlock()

	// Reset the window if it has expired.
	if now.After(entry.windowEnd) || now.Equal(entry.windowEnd) {
		entry.count = 0
		entry.windowEnd = now.Truncate(rl.window).Add(rl.window)
	}

	entry.count++
	remaining := rl.limit - entry.count
	if remaining < 0 {
		remaining = 0
	}

	if entry.count > rl.limit {
		return false, 0, entry.windowEnd
	}
	return true, remaining, entry.windowEnd
}

// rateLimitMiddleware creates middleware that limits requests per client
// (by user ID for authenticated requests, by IP for unauthenticated).
//
// Config key: api.rate_limit.requests_per_minute (default: 120).
func rateLimitMiddleware(config core.ConfigProvider) func(http.Handler) http.Handler {
	rpm := 120
	if config != nil {
		rpm = config.GetInt("api.rate_limit.requests_per_minute")
		if rpm <= 0 {
			rpm = 120
		}
	}

	limiter := newRateLimiter(rpm, time.Minute)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := clientKey(r)

			allowed, remaining, resetTime := limiter.allow(key)

			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(rpm))
			w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
			w.Header().Set("X-RateLimit-Reset", strconv.Itoa(int(resetTime.Unix())))

			if !allowed {
				retryAfter := int(resetTime.Sub(time.Now()).Seconds())
				if retryAfter < 1 {
					retryAfter = 1
				}
				w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
				writeError(w, http.StatusTooManyRequests, "RATE_LIMITED", "rate limit exceeded")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// clientKey returns a unique identifier for the request origin:
// user ID for authenticated requests, client IP otherwise.
func clientKey(r *http.Request) string {
	claims := userClaimsFromRequest(r)
	if claims != nil && claims.UserID != "" {
		return "user:" + claims.UserID
	}
	return "ip:" + stripPort(r.RemoteAddr)
}

// stripPort removes the port from an address string (e.g. "1.2.3.4:1234" → "1.2.3.4").
func stripPort(addr string) string {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	return host
}
