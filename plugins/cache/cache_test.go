package cache

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wangling-miao/aroute/core/events"
	"github.com/wangling-miao/aroute/sdk/interfaces"
)

func newTestService(t *testing.T) *Service {
	t.Helper()
	return newTestServiceWithTTL(t, 5*time.Minute)
}

func newTestServiceWithTTL(t *testing.T, defaultTTL time.Duration) *Service {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	eb := events.NewEventBus()

	svc, err := NewService(Config{
		NumCounters: 10000,
		MaxCost:     10000,
		BufferItems: 64,
		DefaultTTL:  defaultTTL,
	}, eb, logger)
	require.NoError(t, err)
	t.Cleanup(func() { svc.Close() })
	return svc
}

func TestNewService_Defaults(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	eb := events.NewEventBus()

	svc, err := NewService(Config{}, eb, logger)
	require.NoError(t, err)
	defer svc.Close()

	assert.Equal(t, int64(1_000_000), svc.config.NumCounters)
	assert.Equal(t, int64(64*1024*1024), svc.config.MaxCost)
	assert.Equal(t, int64(64), svc.config.BufferItems)
	assert.Equal(t, 5*time.Minute, svc.config.DefaultTTL)
}

func TestNewService_CustomConfig(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	eb := events.NewEventBus()

	cfg := Config{
		NumCounters: 2_000_000,
		MaxCost:     128 * 1024 * 1024,
		BufferItems: 128,
		DefaultTTL:  10 * time.Minute,
	}
	svc, err := NewService(cfg, eb, logger)
	require.NoError(t, err)
	defer svc.Close()

	assert.Equal(t, int64(2_000_000), svc.config.NumCounters)
	assert.Equal(t, int64(128*1024*1024), svc.config.MaxCost)
	assert.Equal(t, int64(128), svc.config.BufferItems)
	assert.Equal(t, 10*time.Minute, svc.config.DefaultTTL)
}

func TestService_SetGet(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)

	// Cache miss on non-existent key
	val, found := svc.Get(ctx, "nonexistent")
	assert.Nil(t, val)
	assert.False(t, found)

	// Set and Get
	err := svc.Set(ctx, "key1", "value1", 0)
	require.NoError(t, err)

	val, found = svc.Get(ctx, "key1")
	assert.Equal(t, "value1", val)
	assert.True(t, found)

	// Overwrite
	err = svc.Set(ctx, "key1", "value2", 0)
	require.NoError(t, err)

	val, found = svc.Get(ctx, "key1")
	assert.Equal(t, "value2", val)
	assert.True(t, found)
}

func TestService_SetWithTTL(t *testing.T) {
	ctx := context.Background()
	svc := newTestServiceWithTTL(t, 5*time.Minute)

	// Set with short TTL
	err := svc.Set(ctx, "short-lived", "data", 200*time.Millisecond)
	require.NoError(t, err)

	val, found := svc.Get(ctx, "short-lived")
	assert.Equal(t, "data", val)
	assert.True(t, found)

	// Wait for expiry — ristretto uses bucket-based cleanup,
	// so we wait a bit longer than the TTL.
	time.Sleep(300 * time.Millisecond)

	val, found = svc.Get(ctx, "short-lived")
	assert.Nil(t, val)
	assert.False(t, found)
}

func TestService_SetDefaultTTL(t *testing.T) {
	ctx := context.Background()
	svc := newTestServiceWithTTL(t, 200*time.Millisecond)

	// Set with TTL=0 should use default TTL
	err := svc.Set(ctx, "default-ttl", "data", 0)
	require.NoError(t, err)

	val, found := svc.Get(ctx, "default-ttl")
	assert.Equal(t, "data", val)
	assert.True(t, found)

	time.Sleep(300 * time.Millisecond)

	val, found = svc.Get(ctx, "default-ttl")
	assert.Nil(t, val)
	assert.False(t, found)
}

func TestService_Delete(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)

	err := svc.Set(ctx, "del-key", "value", 0)
	require.NoError(t, err)

	err = svc.Delete(ctx, "del-key")
	require.NoError(t, err)

	val, found := svc.Get(ctx, "del-key")
	assert.Nil(t, val)
	assert.False(t, found)

	// Delete non-existent key is a no-op
	err = svc.Delete(ctx, "nonexistent")
	assert.NoError(t, err)
}

func TestService_Invalidate_Prefix(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)

	_ = svc.Set(ctx, "content:article:1:abc", "v1", 0)
	_ = svc.Set(ctx, "content:article:2:def", "v2", 0)
	_ = svc.Set(ctx, "content:page:1:abc", "v3", 0)
	_ = svc.Set(ctx, "list:article:nofilter:date:1:10", "v4", 0)

	err := svc.Invalidate(ctx, "content:article:*")
	require.NoError(t, err)

	_, found := svc.Get(ctx, "content:article:1:abc")
	assert.False(t, found, "content:article:1:abc should be invalidated")

	_, found = svc.Get(ctx, "content:article:2:def")
	assert.False(t, found, "content:article:2:def should be invalidated")

	_, found = svc.Get(ctx, "content:page:1:abc")
	assert.True(t, found, "content:page:1:abc should be preserved")

	_, found = svc.Get(ctx, "list:article:nofilter:date:1:10")
	assert.True(t, found, "list:article keys should be preserved")
}

func TestService_Invalidate_Exact(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)

	_ = svc.Set(ctx, "exact-key", "v1", 0)
	_ = svc.Set(ctx, "exact-key-other", "v2", 0)

	err := svc.Invalidate(ctx, "exact-key")
	require.NoError(t, err)

	_, found := svc.Get(ctx, "exact-key")
	assert.False(t, found)

	_, found = svc.Get(ctx, "exact-key-other")
	assert.True(t, found, "non-matching key should be preserved")
}

func TestService_Invalidate_Wildcard(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)

	_ = svc.Set(ctx, "key1", "v1", 0)
	_ = svc.Set(ctx, "key2", "v2", 0)

	err := svc.Invalidate(ctx, "*")
	require.NoError(t, err)

	_, found := svc.Get(ctx, "key1")
	assert.False(t, found)
	_, found = svc.Get(ctx, "key2")
	assert.False(t, found)
}

func TestService_Stats(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)

	_ = svc.Set(ctx, "stats-key1", "v1", 0)
	_ = svc.Set(ctx, "stats-key2", "v2", 0)

	svc.Get(ctx, "stats-key1")
	svc.Get(ctx, "stats-key1")
	svc.Get(ctx, "nonexistent")

	stats := svc.Stats(ctx)
	assert.Equal(t, int64(2), stats.Size, "size should match number of tracked keys")
	assert.Equal(t, int64(10000), stats.Capacity)
	assert.True(t, stats.Hits >= 2, "hits should be at least 2")
	assert.True(t, stats.Misses >= 1, "misses should be at least 1")
}

func TestService_Flush(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)

	_ = svc.Set(ctx, "flush-key1", "v1", 0)
	_ = svc.Set(ctx, "flush-key2", "v2", 0)

	err := svc.Flush(ctx)
	require.NoError(t, err)

	_, found := svc.Get(ctx, "flush-key1")
	assert.False(t, found)
	_, found = svc.Get(ctx, "flush-key2")
	assert.False(t, found)

	stats := svc.Stats(ctx)
	assert.Equal(t, int64(0), stats.Size)
}

func TestBuildContentKey(t *testing.T) {
	svc := newTestService(t)

	key1 := svc.BuildContentKey("article", "42", []string{"title", "body"})
	key2 := svc.BuildContentKey("article", "42", []string{"title", "body"})
	assert.Equal(t, key1, key2, "same inputs should produce same key")

	key3 := svc.BuildContentKey("article", "42", []string{"body", "title"})
	assert.Equal(t, key1, key3, "field order should not affect key")

	key4 := svc.BuildContentKey("article", "42", []string{"title"})
	assert.NotEqual(t, key1, key4, "different fields should produce different key")

	key5 := svc.BuildContentKey("article", "42", nil)
	assert.Contains(t, key5, "content:article:42:")

	key6 := svc.BuildContentKey("page", "42", []string{"title"})
	assert.NotEqual(t, key4, key6, "different content type should produce different key")
}

func TestBuildListKey(t *testing.T) {
	svc := newTestService(t)

	key1 := svc.BuildListKey("article", map[string]interface{}{"status": "published"}, "date", 1, 10)
	key2 := svc.BuildListKey("article", map[string]interface{}{"status": "published"}, "date", 1, 10)
	assert.Equal(t, key1, key2, "same inputs should produce same key")

	key3 := svc.BuildListKey("article", map[string]interface{}{"status": "draft"}, "date", 1, 10)
	assert.NotEqual(t, key1, key3, "different filter values should produce different key")

	key4 := svc.BuildListKey("article", nil, "date", 1, 10)
	assert.Contains(t, key4, "nofilter", "nil filters should use nofilter")

	key5 := svc.BuildListKey("article", map[string]interface{}{}, "date", 1, 10)
	assert.Contains(t, key5, "nofilter", "empty filters should use nofilter")

	key6 := svc.BuildListKey("article", map[string]interface{}{"status": "published"}, "title", 1, 10)
	assert.NotEqual(t, key1, key6, "different sort should produce different key")

	key7 := svc.BuildListKey("article", map[string]interface{}{"status": "published"}, "date", 2, 10)
	assert.NotEqual(t, key1, key7, "different page should produce different key")
}

func TestBuildListKey_DeterministicFilterOrder(t *testing.T) {
	svc := newTestService(t)

	filters1 := map[string]interface{}{"a": 1, "b": 2, "c": 3}
	filters2 := map[string]interface{}{"c": 3, "a": 1, "b": 2}

	key1 := svc.BuildListKey("article", filters1, "date", 1, 10)
	key2 := svc.BuildListKey("article", filters2, "date", 1, 10)
	assert.Equal(t, key1, key2, "filter order should not affect key")
}

func TestHandleContentEvent_Created(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)

	_ = svc.Set(ctx, "list:article:nofilter:date:1:10", "listing", 0)
	_ = svc.Set(ctx, "content:article:99:abc", "individual", 0)

	svc.HandleContentEvent(ctx, events.Event{
		Topic: "content.article.created",
		Data:  map[string]interface{}{"content_type": "article", "id": "100"},
	})

	_, found := svc.Get(ctx, "list:article:nofilter:date:1:10")
	assert.False(t, found, "listing cache should be invalidated on create")

	_, found = svc.Get(ctx, "content:article:99:abc")
	assert.True(t, found, "unrelated individual item should be preserved")
}

func TestHandleContentEvent_Updated(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)

	_ = svc.Set(ctx, "content:article:42:abc", "item", 0)
	_ = svc.Set(ctx, "list:article:nofilter:date:1:10", "listing", 0)
	_ = svc.Set(ctx, "content:page:42:abc", "other-type", 0)

	svc.HandleContentEvent(ctx, events.Event{
		Topic: "content.article.updated",
		Data:  map[string]interface{}{"content_type": "article", "id": "42"},
	})

	_, found := svc.Get(ctx, "content:article:42:abc")
	assert.False(t, found, "individual item cache should be invalidated on update")

	_, found = svc.Get(ctx, "list:article:nofilter:date:1:10")
	assert.False(t, found, "listing cache should be invalidated on update")

	_, found = svc.Get(ctx, "content:page:42:abc")
	assert.True(t, found, "different content type should be preserved")
}

func TestHandleContentEvent_Deleted(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)

	_ = svc.Set(ctx, "content:article:42:abc", "item", 0)
	_ = svc.Set(ctx, "list:article:nofilter:date:1:10", "listing", 0)

	svc.HandleContentEvent(ctx, events.Event{
		Topic: "content.article.deleted",
		Data:  map[string]interface{}{"content_type": "article", "id": "42"},
	})

	_, found := svc.Get(ctx, "content:article:42:abc")
	assert.False(t, found, "individual item should be invalidated on delete")

	_, found = svc.Get(ctx, "list:article:nofilter:date:1:10")
	assert.False(t, found, "listing cache should be invalidated on delete")
}

func TestHandleContentEvent_ShortTopic(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)

	_ = svc.Set(ctx, "some-key", "value", 0)

	// Should not panic on short topic
	svc.HandleContentEvent(ctx, events.Event{
		Topic: "content",
		Data:  nil,
	})

	_, found := svc.Get(ctx, "some-key")
	assert.True(t, found, "no invalidation should happen on malformed topic")
}

func TestHandleContentTypeEvent(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)

	_ = svc.Set(ctx, "content:article:1:abc", "v1", 0)
	_ = svc.Set(ctx, "content:article:2:def", "v2", 0)
	_ = svc.Set(ctx, "list:article:nofilter:date:1:10", "listing", 0)
	_ = svc.Set(ctx, "content:page:1:abc", "page-v", 0)

	svc.HandleContentTypeEvent(ctx, events.Event{
		Topic: "content_type.article.updated",
		Data:  map[string]interface{}{"content_type": "article"},
	})

	_, found := svc.Get(ctx, "content:article:1:abc")
	assert.False(t, found, "all article content should be invalidated")
	_, found = svc.Get(ctx, "content:article:2:def")
	assert.False(t, found, "all article content should be invalidated")
	_, found = svc.Get(ctx, "list:article:nofilter:date:1:10")
	assert.False(t, found, "article listing should be invalidated")

	_, found = svc.Get(ctx, "content:page:1:abc")
	assert.True(t, found, "page content should be preserved")
}

func TestConcurrentSetGet(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)

	var wg sync.WaitGroup
	const numGoroutines = 100

	// Concurrent writes
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			key := fmt.Sprintf("concurrent:key:%d", idx)
			_ = svc.Set(ctx, key, idx, 0)
		}(i)
	}
	wg.Wait()

	// Concurrent reads
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			key := fmt.Sprintf("concurrent:key:%d", idx)
			val, found := svc.Get(ctx, key)
			if found {
				assert.Equal(t, idx, val)
			}
		}(i)
	}
	wg.Wait()
}

func TestConcurrentDeleteGet(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)

	_ = svc.Set(ctx, "race-key", "value", 0)

	var wg sync.WaitGroup
	const numOps = 50

	for i := 0; i < numOps; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			svc.Delete(ctx, "race-key")
		}()
		go func() {
			defer wg.Done()
			svc.Get(ctx, "race-key")
		}()
	}
	wg.Wait()
}

func TestConcurrentInvalidateSet(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)

	var wg sync.WaitGroup
	const numOps = 50

	for i := 0; i < numOps; i++ {
		wg.Add(2)
		go func(idx int) {
			defer wg.Done()
			_ = svc.Set(ctx, fmt.Sprintf("content:article:%d:h", idx), idx, 0)
		}(i)
		go func() {
			defer wg.Done()
			_ = svc.Invalidate(ctx, "content:article:*")
		}()
	}
	wg.Wait()
}

// Compile-time interface check
func TestServiceImplementsCacheService(t *testing.T) {
	var _ interface {
		Get(context.Context, string) (interface{}, bool)
		Set(context.Context, string, interface{}, time.Duration) error
		Delete(context.Context, string) error
		Invalidate(context.Context, string) error
		Stats(context.Context) *interfaces.CacheStats
		Flush(context.Context) error
	} = (*Service)(nil)
}

// Verify sort.Strings determinism used in key generation
func TestKeyGeneration_FieldOrder(t *testing.T) {
	fields := []string{"z_field", "a_field", "m_field"}
	sorted := make([]string, len(fields))
	copy(sorted, fields)
	sort.Strings(sorted)
	assert.Equal(t, []string{"a_field", "m_field", "z_field"}, sorted)
}
