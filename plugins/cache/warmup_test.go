package cache

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wangling-miao/aroute/core"
	"github.com/wangling-miao/aroute/core/events"
	"github.com/wangling-miao/aroute/sdk/interfaces"
)

type mockContentService struct {
	mu    sync.Mutex
	pages map[string]*interfaces.Page
	errs  map[string]error
	calls []string
}

func newMockContentService() *mockContentService {
	return &mockContentService{
		pages: make(map[string]*interfaces.Page),
		errs:  make(map[string]error),
	}
}

func (m *mockContentService) addItems(contentType string, items []*interfaces.Content) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pages[contentType] = &interfaces.Page{
		Data: items,
		Meta: interfaces.PageMeta{Total: int64(len(items))},
	}
}

func (m *mockContentService) setError(contentType string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.errs[contentType] = err
}

func (m *mockContentService) List(ctx context.Context, contentType string, query *interfaces.ListQuery) (*interfaces.Page, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, contentType)
	if err, ok := m.errs[contentType]; ok {
		return nil, err
	}
	if page, ok := m.pages[contentType]; ok {
		return page, nil
	}
	return &interfaces.Page{Data: []*interfaces.Content{}, Meta: interfaces.PageMeta{}}, nil
}

func (m *mockContentService) Create(ctx context.Context, contentType string, data map[string]interface{}) (*interfaces.Content, error) {
	return nil, nil
}
func (m *mockContentService) GetByID(ctx context.Context, id string) (*interfaces.Content, error) {
	return nil, nil
}
func (m *mockContentService) Update(ctx context.Context, id string, data map[string]interface{}) (*interfaces.Content, error) {
	return nil, nil
}
func (m *mockContentService) Delete(ctx context.Context, id string) error { return nil }
func (m *mockContentService) GetContentType(ctx context.Context, name string) (*interfaces.ContentType, error) {
	return nil, nil
}
func (m *mockContentService) CreateContentType(ctx context.Context, ct *interfaces.ContentType) (*interfaces.ContentType, error) {
	return nil, nil
}
func (m *mockContentService) UpdateContentType(ctx context.Context, name string, ct *interfaces.ContentType) (*interfaces.ContentType, error) {
	return nil, nil
}
func (m *mockContentService) DeleteContentType(ctx context.Context, name string) error {
	return nil
}
func (m *mockContentService) ListContentTypes(ctx context.Context) ([]*interfaces.ContentType, error) {
	return nil, nil
}

func newWarmupService(t *testing.T, warmupCfg WarmUpConfig) *Service {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	eb := events.NewEventBus()
	svc, err := NewService(Config{
		NumCounters: 10000,
		MaxCost:     10000,
		BufferItems: 64,
		DefaultTTL:  5 * time.Minute,
		WarmUp:      warmupCfg,
	}, eb, logger)
	require.NoError(t, err)
	t.Cleanup(func() { svc.Close() })
	return svc
}

func makeContent(id, contentType string) *interfaces.Content {
	return &interfaces.Content{
		ID:          id,
		ContentType: contentType,
		Title:       fmt.Sprintf("Test %s %s", contentType, id),
		Data:        map[string]interface{}{"body": "content"},
	}
}

func TestWarmUp_Disabled(t *testing.T) {
	svc := newWarmupService(t, WarmUpConfig{Enabled: false})
	contentSvc := newMockContentService()

	err := svc.WarmUp(context.Background(), contentSvc)
	assert.NoError(t, err)
	assert.Empty(t, contentSvc.calls)
}

func TestWarmUp_NilContentService(t *testing.T) {
	svc := newWarmupService(t, WarmUpConfig{
		Enabled:      true,
		ContentTypes: []string{"article"},
		MaxItems:     10,
	})

	err := svc.WarmUp(context.Background(), nil)
	assert.NoError(t, err)
}

func TestWarmUp_EmptyContentTypes(t *testing.T) {
	svc := newWarmupService(t, WarmUpConfig{
		Enabled:      true,
		ContentTypes: []string{},
		MaxItems:     10,
	})
	contentSvc := newMockContentService()

	err := svc.WarmUp(context.Background(), contentSvc)
	assert.NoError(t, err)
	assert.Empty(t, contentSvc.calls)
}

func TestWarmUp_SingleContentType(t *testing.T) {
	contentSvc := newMockContentService()
	items := []*interfaces.Content{
		makeContent("1", "article"),
		makeContent("2", "article"),
		makeContent("3", "article"),
	}
	contentSvc.addItems("article", items)

	svc := newWarmupService(t, WarmUpConfig{
		Enabled:      true,
		ContentTypes: []string{"article"},
		MaxItems:     100,
	})

	err := svc.WarmUp(context.Background(), contentSvc)
	require.NoError(t, err)

	assert.Equal(t, []string{"article"}, contentSvc.calls)

	for _, item := range items {
		key := svc.BuildContentKey("article", item.ID, nil)
		val, found := svc.Get(context.Background(), key)
		assert.True(t, found, "item %s should be in cache", item.ID)
		assert.Equal(t, item, val)
	}
}

func TestWarmUp_MultipleContentTypes(t *testing.T) {
	contentSvc := newMockContentService()
	articles := []*interfaces.Content{makeContent("a1", "article"), makeContent("a2", "article")}
	pages := []*interfaces.Content{makeContent("p1", "page"), makeContent("p2", "page"), makeContent("p3", "page")}
	contentSvc.addItems("article", articles)
	contentSvc.addItems("page", pages)

	svc := newWarmupService(t, WarmUpConfig{
		Enabled:      true,
		ContentTypes: []string{"article", "page"},
		MaxItems:     100,
	})

	err := svc.WarmUp(context.Background(), contentSvc)
	require.NoError(t, err)

	assert.Equal(t, []string{"article", "page"}, contentSvc.calls)

	for _, item := range articles {
		key := svc.BuildContentKey("article", item.ID, nil)
		_, found := svc.Get(context.Background(), key)
		assert.True(t, found, "article %s should be cached", item.ID)
	}
	for _, item := range pages {
		key := svc.BuildContentKey("page", item.ID, nil)
		_, found := svc.Get(context.Background(), key)
		assert.True(t, found, "page %s should be cached", item.ID)
	}
}

func TestWarmUp_MaxItemsPassedToQuery(t *testing.T) {
	contentSvc := newMockContentService()
	var items []*interfaces.Content
	for i := 0; i < 200; i++ {
		items = append(items, makeContent(fmt.Sprintf("%d", i), "article"))
	}
	contentSvc.addItems("article", items)

	svc := newWarmupService(t, WarmUpConfig{
		Enabled:      true,
		ContentTypes: []string{"article"},
		MaxItems:     50,
	})

	err := svc.WarmUp(context.Background(), contentSvc)
	require.NoError(t, err)
}

func TestWarmUp_DefaultMaxItems(t *testing.T) {
	contentSvc := newMockContentService()
	contentSvc.addItems("article", []*interfaces.Content{makeContent("1", "article")})

	svc := newWarmupService(t, WarmUpConfig{
		Enabled:      true,
		ContentTypes: []string{"article"},
		MaxItems:     0,
	})

	err := svc.WarmUp(context.Background(), contentSvc)
	require.NoError(t, err)

	_, found := svc.Get(context.Background(), svc.BuildContentKey("article", "1", nil))
	assert.True(t, found)
}

func TestWarmUp_DBError_NonFatal(t *testing.T) {
	contentSvc := newMockContentService()
	contentSvc.addItems("article", []*interfaces.Content{makeContent("1", "article")})
	contentSvc.setError("page", fmt.Errorf("database connection lost"))

	svc := newWarmupService(t, WarmUpConfig{
		Enabled:      true,
		ContentTypes: []string{"article", "page"},
		MaxItems:     100,
	})

	err := svc.WarmUp(context.Background(), contentSvc)
	require.NoError(t, err)

	assert.Equal(t, []string{"article", "page"}, contentSvc.calls)

	key := svc.BuildContentKey("article", "1", nil)
	_, found := svc.Get(context.Background(), key)
	assert.True(t, found, "articles should still be cached despite page failure")
}

func TestWarmUp_AllDBErrors_NonFatal(t *testing.T) {
	contentSvc := newMockContentService()
	contentSvc.setError("article", fmt.Errorf("db down"))
	contentSvc.setError("page", fmt.Errorf("db down"))

	svc := newWarmupService(t, WarmUpConfig{
		Enabled:      true,
		ContentTypes: []string{"article", "page"},
		MaxItems:     100,
	})

	err := svc.WarmUp(context.Background(), contentSvc)
	require.NoError(t, err)

	stats := svc.Stats(context.Background())
	assert.Equal(t, int64(0), stats.Size)
}

func TestWarmUp_CancelledContext(t *testing.T) {
	contentSvc := newMockContentService()
	contentSvc.addItems("article", []*interfaces.Content{makeContent("1", "article")})

	svc := newWarmupService(t, WarmUpConfig{
		Enabled:      true,
		ContentTypes: []string{"article"},
		MaxItems:     100,
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := svc.WarmUp(ctx, contentSvc)
	assert.NoError(t, err)
}

func TestWarmUp_ConcurrentSafety(t *testing.T) {
	contentSvc := newMockContentService()
	var items []*interfaces.Content
	for i := 0; i < 50; i++ {
		items = append(items, makeContent(fmt.Sprintf("%d", i), "article"))
	}
	contentSvc.addItems("article", items)

	svc := newWarmupService(t, WarmUpConfig{
		Enabled:      true,
		ContentTypes: []string{"article"},
		MaxItems:     100,
	})

	err := svc.WarmUp(context.Background(), contentSvc)
	require.NoError(t, err)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			key := svc.BuildContentKey("article", fmt.Sprintf("%d", idx), nil)
			svc.Get(context.Background(), key)
		}(i)
	}
	wg.Wait()
}

func TestWarmUp_StatsReflectWarmedItems(t *testing.T) {
	contentSvc := newMockContentService()
	items := []*interfaces.Content{
		makeContent("1", "article"),
		makeContent("2", "article"),
		makeContent("3", "article"),
	}
	contentSvc.addItems("article", items)

	svc := newWarmupService(t, WarmUpConfig{
		Enabled:      true,
		ContentTypes: []string{"article"},
		MaxItems:     100,
	})

	err := svc.WarmUp(context.Background(), contentSvc)
	require.NoError(t, err)

	stats := svc.Stats(context.Background())
	assert.Equal(t, int64(3), stats.Size)
}

func TestWarmUp_ExistingCachePreserved(t *testing.T) {
	contentSvc := newMockContentService()
	contentSvc.addItems("article", []*interfaces.Content{makeContent("warm1", "article")})

	svc := newWarmupService(t, WarmUpConfig{
		Enabled:      true,
		ContentTypes: []string{"article"},
		MaxItems:     100,
	})

	err := svc.Set(context.Background(), "preexisting-key", "preexisting-value", 0)
	require.NoError(t, err)

	err = svc.WarmUp(context.Background(), contentSvc)
	require.NoError(t, err)

	val, found := svc.Get(context.Background(), "preexisting-key")
	assert.True(t, found, "pre-existing cache entry should be preserved")
	assert.Equal(t, "preexisting-value", val)

	warmKey := svc.BuildContentKey("article", "warm1", nil)
	_, found = svc.Get(context.Background(), warmKey)
	assert.True(t, found, "warmed item should be in cache")
}

func TestPlugin_Init_WarmUpDisabled(t *testing.T) {
	p := New()
	ctx := newMockCoreContext(&mockConfigProvider{data: map[string]interface{}{}})
	t.Cleanup(func() { p.Stop() })

	err := p.Init(ctx)
	require.NoError(t, err)

	assert.False(t, p.service.config.WarmUp.Enabled)
	assert.Equal(t, int64(0), p.service.Stats(context.Background()).Size)
}

func TestPlugin_Init_WarmUpEnabled_NoContentService(t *testing.T) {
	p := New()
	ctx := newMockCoreContext(&mockConfigProvider{data: map[string]interface{}{
		"warmup_enabled":       true,
		"warmup_content_types": []string{"article"},
		"warmup_max_items":     50,
	}})
	t.Cleanup(func() { p.Stop() })

	err := p.Init(ctx)
	require.NoError(t, err)

	assert.True(t, p.service.config.WarmUp.Enabled)
	assert.Equal(t, []string{"article"}, p.service.config.WarmUp.ContentTypes)
	assert.Equal(t, 50, p.service.config.WarmUp.MaxItems)
}

func TestPlugin_Init_WarmUpEnabled_WithContentService(t *testing.T) {
	p := New()
	contentSvc := newMockContentService()
	contentSvc.addItems("article", []*interfaces.Content{
		makeContent("1", "article"),
		makeContent("2", "article"),
	})

	ctx := newMockCoreContext(&mockConfigProvider{data: map[string]interface{}{
		"warmup_enabled":       true,
		"warmup_content_types": []string{"article"},
		"warmup_max_items":     100,
	}})

	err := ctx.Services().Provide(func(c core.ServiceContainer) (interfaces.ContentService, error) {
		return contentSvc, nil
	})
	require.NoError(t, err)
	t.Cleanup(func() { p.Stop() })

	err = p.Init(ctx)
	require.NoError(t, err)

	key := p.service.BuildContentKey("article", "1", nil)
	_, found := p.service.Get(context.Background(), key)
	assert.True(t, found, "article 1 should be warmed via plugin Init")
}

func TestPlugin_Init_WarmUpConfigDefaults(t *testing.T) {
	p := New()
	ctx := newMockCoreContext(&mockConfigProvider{data: map[string]interface{}{
		"warmup_enabled": false,
	}})
	t.Cleanup(func() { p.Stop() })

	err := p.Init(ctx)
	require.NoError(t, err)

	assert.False(t, p.service.config.WarmUp.Enabled)
	assert.Nil(t, p.service.config.WarmUp.ContentTypes)
	assert.Equal(t, 0, p.service.config.WarmUp.MaxItems)
}
