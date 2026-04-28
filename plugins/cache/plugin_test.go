package cache

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wangling-miao/aroute/core"
	"github.com/wangling-miao/aroute/core/events"
	"github.com/wangling-miao/aroute/core/services"
	"github.com/wangling-miao/aroute/sdk/interfaces"
)

// --- Mock types (following http/plugin_test.go pattern) ---

// mockCoreContext implements core.CoreContext for testing
type mockCoreContext struct {
	services  core.ServiceContainer
	events    core.EventBus
	config    core.ConfigProvider
	logger    *slog.Logger
	dataDir   string
	pluginDir string
	ctx       context.Context
}

func newMockCoreContext(config core.ConfigProvider) *mockCoreContext {
	return &mockCoreContext{
		services:  services.NewContainer(),
		events:    events.NewEventBus(),
		config:    config,
		logger:    slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn})),
		dataDir:   os.TempDir(),
		pluginDir: os.TempDir(),
		ctx:       context.Background(),
	}
}

func (m *mockCoreContext) Services() core.ServiceContainer { return m.services }
func (m *mockCoreContext) Events() core.EventBus           { return m.events }
func (m *mockCoreContext) Config() core.ConfigProvider     { return m.config }
func (m *mockCoreContext) Logger() *slog.Logger            { return m.logger }
func (m *mockCoreContext) DataDir() string                 { return m.dataDir }
func (m *mockCoreContext) PluginDir() string               { return m.pluginDir }
func (m *mockCoreContext) Context() context.Context        { return m.ctx }

// mockConfigProvider implements core.ConfigProvider for testing
type mockConfigProvider struct {
	data map[string]interface{}
}

func (m *mockConfigProvider) GetString(key string) string {
	if v, ok := m.data[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func (m *mockConfigProvider) GetInt(key string) int {
	if v, ok := m.data[key]; ok {
		if i, ok := v.(int); ok {
			return i
		}
	}
	return 0
}

func (m *mockConfigProvider) GetBool(key string) bool {
	if v, ok := m.data[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}

func (m *mockConfigProvider) GetStringSlice(key string) []string {
	if v, ok := m.data[key]; ok {
		if s, ok := v.([]string); ok {
			return s
		}
	}
	return nil
}

func (m *mockConfigProvider) Get(key string) interface{} {
	return m.data[key]
}

func (m *mockConfigProvider) Unmarshal(key string, target interface{}) error {
	return nil
}
func (m *mockConfigProvider) Set(key string, value interface{})              { m.data[key] = value }
func (m *mockConfigProvider) Save() error                                    { return nil }

// --- Plugin lifecycle tests ---

func TestPlugin_New(t *testing.T) {
	p := New()

	assert.NotNil(t, p, "New() should return non-nil plugin")
	assert.Equal(t, "cache", p.Name(), "plugin name should be 'cache'")
	assert.Equal(t, "1.0.0", p.Version(), "plugin version should be '1.0.0'")

	manifest := p.Manifest()
	require.NotNil(t, manifest, "Manifest() should return non-nil")
	assert.Equal(t, "cache", manifest.Name)
	assert.Equal(t, "native", manifest.Engine)
}

func TestPlugin_Init_DefaultConfig(t *testing.T) {
	p := New()
	ctx := newMockCoreContext(&mockConfigProvider{data: map[string]interface{}{}})
	t.Cleanup(func() { p.Stop() })

	err := p.Init(ctx)
	require.NoError(t, err, "Init with default config should succeed")

	// Verify the CacheService was registered
	var cacheSvc interfaces.CacheService
	err = ctx.Services().Get(&cacheSvc)
	require.NoError(t, err, "CacheService should be registered in container")
	assert.NotNil(t, cacheSvc, "CacheService should be non-nil")

	// Verify defaults were applied
	svc := p.service
	require.NotNil(t, svc)
	assert.Equal(t, int64(1_000_000), svc.config.NumCounters, "default NumCounters should be 1_000_000")
	assert.Equal(t, int64(64*1024*1024), svc.config.MaxCost, "default MaxCost should be 64MB")
	assert.Equal(t, int64(64), svc.config.BufferItems, "default BufferItems should be 64")
	assert.Equal(t, 5*time.Minute, svc.config.DefaultTTL, "default TTL should be 5 minutes")
}

func TestPlugin_Init_CustomConfig(t *testing.T) {
	p := New()
	cfg := &mockConfigProvider{
		data: map[string]interface{}{
			"num_counters":        5000,
			"max_cost":            1024,
			"buffer_items":        32,
			"default_ttl_seconds": 60,
		},
	}
	ctx := newMockCoreContext(cfg)
	t.Cleanup(func() { p.Stop() })

	err := p.Init(ctx)
	require.NoError(t, err, "Init with custom config should succeed")

	svc := p.service
	require.NotNil(t, svc)
	assert.Equal(t, int64(5000), svc.config.NumCounters)
	assert.Equal(t, int64(1024), svc.config.MaxCost)
	assert.Equal(t, int64(32), svc.config.BufferItems)
	assert.Equal(t, 60*time.Second, svc.config.DefaultTTL)
}

func TestPlugin_Start(t *testing.T) {
	p := New()
	ctx := newMockCoreContext(&mockConfigProvider{data: map[string]interface{}{}})
	t.Cleanup(func() { p.Stop() })

	err := p.Init(ctx)
	require.NoError(t, err)

	err = p.Start()
	require.NoError(t, err, "Start should succeed after Init")
	assert.True(t, p.running, "plugin should be running after Start")
}

func TestPlugin_Stop(t *testing.T) {
	p := New()
	ctx := newMockCoreContext(&mockConfigProvider{data: map[string]interface{}{}})

	err := p.Init(ctx)
	require.NoError(t, err)

	err = p.Start()
	require.NoError(t, err)

	err = p.Stop()
	require.NoError(t, err, "Stop should succeed")
	assert.False(t, p.running, "plugin should not be running after Stop")
}

func TestPlugin_StartStop_Idempotent(t *testing.T) {
	p := New()
	ctx := newMockCoreContext(&mockConfigProvider{data: map[string]interface{}{}})

	err := p.Init(ctx)
	require.NoError(t, err)

	// Start twice — second should be no-op
	err = p.Start()
	require.NoError(t, err)
	err = p.Start()
	require.NoError(t, err, "second Start should be no-op")
	assert.True(t, p.running)

	// Stop twice — second should be no-op
	err = p.Stop()
	require.NoError(t, err)
	err = p.Stop()
	require.NoError(t, err, "second Stop should be no-op")
	assert.False(t, p.running)
}

func TestPlugin_Init_AndUse(t *testing.T) {
	p := New()
	ctx := newMockCoreContext(&mockConfigProvider{data: map[string]interface{}{}})
	t.Cleanup(func() { p.Stop() })

	err := p.Init(ctx)
	require.NoError(t, err)

	err = p.Start()
	require.NoError(t, err)

	// Retrieve the CacheService from the container and use it
	var cacheSvc interfaces.CacheService
	err = ctx.Services().Get(&cacheSvc)
	require.NoError(t, err)

	bgCtx := context.Background()

	// Set a value
	err = cacheSvc.Set(bgCtx, "plugin-test-key", "plugin-test-value", 0)
	require.NoError(t, err)

	// Get the value back
	val, found := cacheSvc.Get(bgCtx, "plugin-test-key")
	assert.True(t, found, "key should be found after Set")
	assert.Equal(t, "plugin-test-value", val)

	// Delete
	err = cacheSvc.Delete(bgCtx, "plugin-test-key")
	require.NoError(t, err)

	_, found = cacheSvc.Get(bgCtx, "plugin-test-key")
	assert.False(t, found, "key should be gone after Delete")
}

func TestHandleContentTypeEvent_NilData(t *testing.T) {
	p := New()
	ctx := newMockCoreContext(&mockConfigProvider{data: map[string]interface{}{}})
	t.Cleanup(func() { p.Stop() })

	err := p.Init(ctx)
	require.NoError(t, err)

	bgCtx := context.Background()

	// Seed some cache entries
	_ = p.service.Set(bgCtx, "content:article:1:abc", "v1", 0)
	_ = p.service.Set(bgCtx, "list:article:nofilter:date:1:10", "listing", 0)

	// Emit event with nil Data — should early-return and NOT invalidate anything
	ctx.Events().Emit(bgCtx, events.Event{
		Topic: "content_type.article.updated",
		Data:  nil,
	})

	// Give event bus time to process
	time.Sleep(50 * time.Millisecond)

	_, found := p.service.Get(bgCtx, "content:article:1:abc")
	assert.True(t, found, "nil Data should not cause any invalidation")

	_, found = p.service.Get(bgCtx, "list:article:nofilter:date:1:10")
	assert.True(t, found, "nil Data should not cause any invalidation")
}
