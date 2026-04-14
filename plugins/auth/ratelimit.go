package auth

import (
	"sync"
	"time"
)

// RateLimiter provides IP-based rate limiting for authentication attempts.
type RateLimiter struct {
	maxAttempts int
	window      time.Duration
	entries     sync.Map // map[string]*rateLimitEntry
	done        chan struct{}
}

// rateLimitEntry tracks failed authentication attempts for a single IP.
type rateLimitEntry struct {
	mu       sync.Mutex
	attempts []time.Time
}

// NewRateLimiter creates a new RateLimiter with the given parameters.
func NewRateLimiter(maxAttempts int, window time.Duration) *RateLimiter {
	rl := &RateLimiter{
		maxAttempts: maxAttempts,
		window:      window,
		done:        make(chan struct{}),
	}
	go rl.cleanup()
	return rl
}

// Check returns whether the given IP is allowed to make another attempt.
// Returns (allowed, retryAfterSeconds).
func (r *RateLimiter) Check(ip string) (bool, int) {
	entry := r.getOrCreate(ip)
	entry.mu.Lock()
	defer entry.mu.Unlock()

	now := time.Now()
	r.evictOld(entry, now)

	if len(entry.attempts) >= r.maxAttempts {
		oldest := entry.attempts[0]
		retryAfter := int(oldest.Add(r.window).Sub(now).Seconds())
		if retryAfter < 1 {
			retryAfter = 1
		}
		return false, retryAfter
	}
	return true, 0
}

// RecordFailure records a failed authentication attempt for the given IP.
func (r *RateLimiter) RecordFailure(ip string) {
	entry := r.getOrCreate(ip)
	entry.mu.Lock()
	defer entry.mu.Unlock()

	entry.attempts = append(entry.attempts, time.Now())
}

// Reset clears all rate limit entries for the given IP (called on successful auth).
func (r *RateLimiter) Reset(ip string) {
	r.entries.Delete(ip)
}

// Stop terminates the background cleanup goroutine.
func (r *RateLimiter) Stop() {
	close(r.done)
}

// getOrCreate returns the rate limit entry for an IP, creating one if needed.
func (r *RateLimiter) getOrCreate(ip string) *rateLimitEntry {
	val, _ := r.entries.LoadOrStore(ip, &rateLimitEntry{
		attempts: make([]time.Time, 0, r.maxAttempts),
	})
	return val.(*rateLimitEntry)
}

// evictOld removes attempt timestamps outside the current window.
func (r *RateLimiter) evictOld(entry *rateLimitEntry, now time.Time) {
	cutoff := now.Add(-r.window)
	idx := 0
	for i, t := range entry.attempts {
		if t.After(cutoff) {
			idx = i
			break
		}
		if i == len(entry.attempts)-1 {
			// All entries are old.
			entry.attempts = entry.attempts[:0]
			return
		}
	}
	if idx > 0 {
		entry.attempts = entry.attempts[idx:]
	}
}

// cleanup periodically removes stale entries from the rate limiter.
func (r *RateLimiter) cleanup() {
	ticker := time.NewTicker(r.window)
	defer ticker.Stop()

	for {
		select {
		case <-r.done:
			return
		case <-ticker.C:
			now := time.Now()
			r.entries.Range(func(key, value interface{}) bool {
				entry := value.(*rateLimitEntry)
				entry.mu.Lock()
				r.evictOld(entry, now)
				isEmpty := len(entry.attempts) == 0
				entry.mu.Unlock()

				if isEmpty {
					r.entries.Delete(key)
				}
				return true
			})
		}
	}
}
