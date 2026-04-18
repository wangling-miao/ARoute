package cache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/dgraph-io/ristretto/v2"
	"github.com/wangling-miao/aroute/core"
	"github.com/wangling-miao/aroute/core/events"
	"github.com/wangling-miao/aroute/sdk/interfaces"
)

// Service implements interfaces.CacheService backed by Ristretto in-memory cache.
type Service struct {
	mu     sync.RWMutex
	cache  *ristretto.Cache[string, interface{}]
	events core.EventBus
	logger *slog.Logger
	keySet map[string]bool
	config Config
}

type WarmUpConfig struct {
	Enabled      bool
	ContentTypes []string
	MaxItems     int
}

// Config holds ristretto cache initialization parameters.
type Config struct {
	NumCounters int64
	MaxCost     int64
	BufferItems int64
	DefaultTTL  time.Duration
	WarmUp      WarmUpConfig
}

// NewService creates a cache service with the given configuration.
// Applies sensible defaults for any zero-valued config fields.
func NewService(cfg Config, events core.EventBus, logger *slog.Logger) (*Service, error) {
	if cfg.NumCounters <= 0 {
		cfg.NumCounters = 1_000_000
	}
	if cfg.MaxCost <= 0 {
		cfg.MaxCost = 64 * 1024 * 1024
	}
	if cfg.BufferItems <= 0 {
		cfg.BufferItems = 64
	}
	if cfg.DefaultTTL == 0 {
		cfg.DefaultTTL = 5 * time.Minute
	}

	cache, err := ristretto.NewCache[string, interface{}](&ristretto.Config[string, interface{}]{
		NumCounters: cfg.NumCounters,
		MaxCost:     cfg.MaxCost,
		BufferItems: cfg.BufferItems,
		Metrics:     true,
	})
	if err != nil {
		return nil, fmt.Errorf("create ristretto cache: %w", err)
	}

	return &Service{
		cache:  cache,
		events: events,
		logger: logger,
		keySet: make(map[string]bool),
		config: cfg,
	}, nil
}

// Get retrieves a cached value by key. Returns (nil, false) on miss or expiry.
func (s *Service) Get(ctx context.Context, key string) (interface{}, bool) {
	return s.cache.Get(key)
}

// Set stores a value with the given TTL. Uses configured default TTL when ttl is 0.
func (s *Service) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	if ttl == 0 {
		ttl = s.config.DefaultTTL
	}

	// Ristretto SetWithTTL is asynchronous; wait for the write buffer
	// to drain by calling Wait() to ensure the item is visible immediately.
	s.cache.SetWithTTL(key, value, 1, ttl)
	s.cache.Wait()

	s.mu.Lock()
	s.keySet[key] = true
	s.mu.Unlock()

	return nil
}

// Delete removes a cached value by key. No-op if the key doesn't exist.
func (s *Service) Delete(ctx context.Context, key string) error {
	s.cache.Del(key)

	s.mu.Lock()
	delete(s.keySet, key)
	s.mu.Unlock()

	return nil
}

// Invalidate removes all cached values matching a prefix pattern.
// Patterns ending with "*" are treated as prefix matches;
// all other patterns are treated as exact key matches.
func (s *Service) Invalidate(ctx context.Context, pattern string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var keysToDelete []string

	isPrefix := strings.HasSuffix(pattern, "*")
	prefix := strings.TrimSuffix(pattern, "*")

	for key := range s.keySet {
		if isPrefix && strings.HasPrefix(key, prefix) {
			keysToDelete = append(keysToDelete, key)
		} else if !isPrefix && key == pattern {
			keysToDelete = append(keysToDelete, key)
		}
	}

	for _, key := range keysToDelete {
		s.cache.Del(key)
		delete(s.keySet, key)
	}

	return nil
}

// Stats returns cache performance metrics.
func (s *Service) Stats(ctx context.Context) *interfaces.CacheStats {
	m := s.cache.Metrics

	s.mu.RLock()
	size := int64(len(s.keySet))
	s.mu.RUnlock()

	var hits, misses, evictions int64
	var hitRate float64
	if m != nil {
		hits = int64(m.Hits())
		misses = int64(m.Misses())
		evictions = int64(m.KeysEvicted())
		total := hits + misses
		if total > 0 {
			hitRate = float64(hits) / float64(total)
		}
	}

	return &interfaces.CacheStats{
		Hits:      hits,
		Misses:    misses,
		Evictions: evictions,
		HitRate:   hitRate,
		Size:      size,
		Capacity:  s.config.MaxCost,
	}
}

// Flush removes all cached entries and resets the key tracker.
func (s *Service) Flush(ctx context.Context) error {
	s.cache.Clear()

	s.mu.Lock()
	s.keySet = make(map[string]bool)
	s.mu.Unlock()

	return nil
}

// BuildContentKey generates a deterministic cache key for a single content item.
// Format: content:{contentType}:{id}:{fields_hash}
func (s *Service) BuildContentKey(contentType, id string, fields []string) string {
	fieldsHash := ""
	if len(fields) > 0 {
		sorted := make([]string, len(fields))
		copy(sorted, fields)
		sort.Strings(sorted)
		hash := sha256.Sum256([]byte(strings.Join(sorted, ",")))
		fieldsHash = hex.EncodeToString(hash[:4])
	}
	return fmt.Sprintf("content:%s:%s:%s", contentType, id, fieldsHash)
}

// BuildListKey generates a deterministic cache key for a content list query.
// Format: list:{contentType}:{filter_hash}:{sortBy}:{page}:{perPage}
func (s *Service) BuildListKey(contentType string, filters map[string]interface{}, sortBy string, page, perPage int) string {
	filterHash := "nofilter"
	if len(filters) > 0 {
		keys := make([]string, 0, len(filters))
		for k := range filters {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		sortedFilters := make(map[string]interface{}, len(keys))
		for _, k := range keys {
			sortedFilters[k] = filters[k]
		}

		if data, err := json.Marshal(sortedFilters); err == nil {
			hash := sha256.Sum256(data)
			filterHash = hex.EncodeToString(hash[:4])
		}
	}

	return fmt.Sprintf("list:%s:%s:%s:%d:%d", contentType, filterHash, sortBy, page, perPage)
}

// HandleContentEvent handles content.* events for automatic cache invalidation.
// Event topics: content.{type}.created, content.{type}.updated, content.{type}.deleted
func (s *Service) HandleContentEvent(ctx context.Context, event events.Event) {
	parts := strings.Split(event.Topic, ".")
	if len(parts) < 3 {
		return
	}

	if event.Data == nil {
		return
	}

	contentType, _ := event.Data["content_type"].(string)
	id, _ := event.Data["id"].(string)
	action := parts[len(parts)-1]

	switch action {
	case "created":
		if contentType != "" {
			s.Invalidate(ctx, fmt.Sprintf("list:%s:*", contentType))
		}
	case "updated":
		if contentType != "" && id != "" {
			s.Invalidate(ctx, fmt.Sprintf("content:%s:%s:*", contentType, id))
		}
		if contentType != "" {
			s.Invalidate(ctx, fmt.Sprintf("list:%s:*", contentType))
		}
	case "deleted":
		if contentType != "" && id != "" {
			s.Invalidate(ctx, fmt.Sprintf("content:%s:%s:*", contentType, id))
		}
		if contentType != "" {
			s.Invalidate(ctx, fmt.Sprintf("list:%s:*", contentType))
		}
	}
}

// HandleContentTypeEvent handles content_type.* events for full cache invalidation.
// When a content type schema changes, all cached items for that type are invalidated.
func (s *Service) HandleContentTypeEvent(ctx context.Context, event events.Event) {
	parts := strings.Split(event.Topic, ".")
	if len(parts) < 2 {
		return
	}

	if event.Data == nil {
		return
	}

	contentType, _ := event.Data["content_type"].(string)
	if contentType == "" {
		// Fallback: try to extract from topic "content_type.{type}.{action}"
		if len(parts) >= 2 {
			contentType = parts[1]
		}
	}

	if contentType != "" {
		s.Invalidate(ctx, fmt.Sprintf("content:%s:*", contentType))
		s.Invalidate(ctx, fmt.Sprintf("list:%s:*", contentType))
	}
}

// Close shuts down the cache and releases resources.
func (s *Service) Close() {
	s.cache.Close()
}

func (s *Service) WarmUp(ctx context.Context, contentSvc interfaces.ContentService) error {
	if !s.config.WarmUp.Enabled {
		return nil
	}
	if contentSvc == nil {
		s.logger.Warn("cache warm-up skipped: ContentService is nil")
		return nil
	}

	maxItems := s.config.WarmUp.MaxItems
	if maxItems <= 0 {
		maxItems = 100
	}

	for _, ct := range s.config.WarmUp.ContentTypes {
		page, err := contentSvc.List(ctx, ct, &interfaces.ListQuery{
			Page:    1,
			PerPage: maxItems,
			Sort:    "updated_at",
			Order:   "desc",
		})
		if err != nil {
			s.logger.Warn("cache warm-up failed for content type, skipping",
				"content_type", ct,
				"error", fmt.Errorf("list content: %w", err).Error(),
			)
			continue
		}

		items, ok := page.Data.([]*interfaces.Content)
		if !ok {
			s.logger.Warn("cache warm-up: unexpected page data type for content type, skipping",
				"content_type", ct,
			)
			continue
		}

		var warmed int
		for _, item := range items {
			key := s.BuildContentKey(ct, item.ID, nil)
			if setErr := s.Set(ctx, key, item, 0); setErr != nil {
				continue
			}
			warmed++
		}
		s.logger.Info("cache warm-up completed",
			"content_type", ct,
			"items_warmed", warmed,
		)
	}

	return nil
}
