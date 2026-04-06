package interfaces

import (
	"context"
	"time"
)

// CacheService defines in-memory caching operations with TTL support.
// It wraps the ristretto cache with Aroute-specific key strategies
// and automatic invalidation on content changes.
type CacheService interface {
	// Get retrieves a cached value by key.
	// Returns nil and false if the key doesn't exist or has expired.
	Get(ctx context.Context, key string) (interface{}, bool)

	// Set stores a value in the cache with the specified TTL.
	// If TTL is 0, uses default TTL from configuration.
	Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error

	// Delete removes a cached value by key.
	Delete(ctx context.Context, key string) error

	// Invalidate removes all cached values matching a pattern.
	// Pattern supports wildcards: "content:*" matches all content-related keys.
	Invalidate(ctx context.Context, pattern string) error

	// Stats returns cache statistics (hit rate, miss rate, eviction count).
	Stats(ctx context.Context) *CacheStats
}

// CacheStats contains cache performance statistics.
type CacheStats struct {
	// Hits is the number of cache hits.
	Hits int64 `json:"hits"`

	// Misses is the number of cache misses.
	Misses int64 `json:"misses"`

	// Evictions is the number of cache evictions.
	Evictions int64 `json:"evictions"`

	// HitRate is the cache hit rate (0.0 to 1.0).
	HitRate float64 `json:"hit_rate"`

	// Size is the current number of items in the cache.
	Size int64 `json:"size"`

	// Capacity is the maximum number of items.
	Capacity int64 `json:"capacity"`
}
