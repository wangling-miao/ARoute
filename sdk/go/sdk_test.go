package sdk

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wangling-miao/aroute/core"
	"github.com/wangling-miao/aroute/core/events"
	"github.com/wangling-miao/aroute/core/services"
	"github.com/wangling-miao/aroute/sdk/interfaces"
)

// --- Mock implementations for testing ---

// mockConfigProvider implements core.ConfigProvider for testing.
type mockConfigProvider struct {
	data map[string]interface{}
}

func newMockConfig() *mockConfigProvider {
	return &mockConfigProvider{data: make(map[string]interface{})}
}

func (m *mockConfigProvider) GetString(key string) string    { v, _ := m.data[key].(string); return v }
func (m *mockConfigProvider) GetInt(key string) int          { v, _ := m.data[key].(int); return v }
func (m *mockConfigProvider) GetBool(key string) bool        { v, _ := m.data[key].(bool); return v }
func (m *mockConfigProvider) GetStringSlice(key string) []string {
	v, _ := m.data[key].([]string); return v
}
func (m *mockConfigProvider) Get(key string) interface{}     { return m.data[key] }
func (m *mockConfigProvider) Unmarshal(key string, target interface{}) error { return nil }

// mockEventBus wraps events.EventBus for test assertions.
type mockEventBus struct {
	*events.EventBus
	subscribedTopics []string
}

func newMockEventBus() *mockEventBus {
	return &mockEventBus{EventBus: events.NewEventBus()}
}

func (m *mockEventBus) SubscribeBroadcast(topic string, handler events.BroadcastHandler) string {
	m.subscribedTopics = append(m.subscribedTopics, topic)
	return m.EventBus.SubscribeBroadcast(topic, handler)
}

// mockCoreContext implements core.CoreContext for testing.
type mockCoreContext struct {
	services  core.ServiceContainer
	eventBus  core.EventBus
	config    core.ConfigProvider
	logger    *slog.Logger
	dataDir   string
	pluginDir string
	ctx       context.Context
}

func newMockCoreContext(container core.ServiceContainer) *mockCoreContext {
	return &mockCoreContext{
		services:  container,
		eventBus:  events.NewEventBus(),
		config:    newMockConfig(),
		logger:    slog.Default(),
		dataDir:   "/tmp/aroute-test/data",
		pluginDir: "/tmp/aroute-test/plugins",
		ctx:       context.Background(),
	}
}

func (m *mockCoreContext) Services() core.ServiceContainer { return m.services }
func (m *mockCoreContext) Events() core.EventBus          { return m.eventBus }
func (m *mockCoreContext) Config() core.ConfigProvider     { return m.config }
func (m *mockCoreContext) Logger() *slog.Logger            { return m.logger }
func (m *mockCoreContext) DataDir() string                 { return m.dataDir }
func (m *mockCoreContext) PluginDir() string               { return m.pluginDir }
func (m *mockCoreContext) Context() context.Context        { return m.ctx }

// --- Version tests ---

func TestVersion(t *testing.T) {
	v := Version()
	if v == "" {
		t.Fatal("Version() returned empty string")
	}
	if v != SDKVersion {
		t.Fatalf("Version() = %q, want SDKVersion %q", v, SDKVersion)
	}
	// Verify semver format (MAJOR.MINOR.PATCH)
	parts := strings.Split(v, ".")
	if len(parts) < 3 {
		t.Fatalf("Version %q is not semver format", v)
	}
}

// --- BasePlugin constructor tests ---

func TestNewBasePlugin(t *testing.T) {
	bp := NewBasePlugin("test-plugin", "1.0.0")

	if bp.Name() != "test-plugin" {
		t.Errorf("Name() = %q, want %q", bp.Name(), "test-plugin")
	}
	if bp.Version() != "1.0.0" {
		t.Errorf("Version() = %q, want %q", bp.Version(), "1.0.0")
	}
	if bp.Manifest() != nil {
		t.Error("Manifest() should be nil when created with NewBasePlugin")
	}
	if bp.Description() != "" {
		t.Error("Description() should be empty without manifest")
	}
	if bp.Author() != "" {
		t.Error("Author() should be empty without manifest")
	}
}

func TestMustNewBasePlugin(t *testing.T) {
	bp := MustNewBasePlugin("valid-plugin", "2.0.0")
	if bp.Name() != "valid-plugin" {
		t.Errorf("Name() = %q, want %q", bp.Name(), "valid-plugin")
	}
}

func TestMustNewBasePlugin_PanicsOnEmptyName(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("Expected panic for empty name")
		}
		if !strings.Contains(r.(string), "name is required") {
			t.Fatalf("Unexpected panic message: %v", r)
		}
	}()
	MustNewBasePlugin("", "1.0.0")
}

func TestMustNewBasePlugin_PanicsOnEmptyVersion(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("Expected panic for empty version")
		}
	}()
	MustNewBasePlugin("test", "")
}

// --- BasePlugin lifecycle tests ---

func TestBasePlugin_Init(t *testing.T) {
	bp := NewBasePlugin("test", "1.0.0")
	container := services.NewContainer()
	ctx := newMockCoreContext(container)

	if err := bp.Init(ctx); err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	if bp.Context() != ctx {
		t.Error("Context() should return the CoreContext passed to Init")
	}
	if bp.Logger() == nil {
		t.Error("Logger() should not be nil after Init")
	}
}

func TestBasePlugin_Start(t *testing.T) {
	bp := NewBasePlugin("test", "1.0.0")
	if err := bp.Start(); err != nil {
		t.Fatalf("Start() error: %v", err)
	}
}

func TestBasePlugin_Stop(t *testing.T) {
	bp := NewBasePlugin("test", "1.0.0")
	if err := bp.Stop(); err != nil {
		t.Fatalf("Stop() error: %v", err)
	}
}

func TestBasePlugin_InterfaceCompliance(t *testing.T) {
	// Verify BasePlugin satisfies core.Plugin
	var _ core.Plugin = (*BasePlugin)(nil)

	// Verify it works as a core.Plugin
	var p core.Plugin = NewBasePlugin("test", "1.0.0")
	if p.Name() != "test" {
		t.Errorf("Name via interface = %q, want %q", p.Name(), "test")
	}
}

func TestBasePlugin_SetManifest(t *testing.T) {
	bp := NewBasePlugin("old", "0.0.0")
	manifest := &core.Manifest{
		Name:        "new-plugin",
		Version:     "2.0.0",
		Description: "A test plugin",
		Author:      "Test Author",
	}

	bp.SetManifest(manifest)

	if bp.Name() != "new-plugin" {
		t.Errorf("Name() after SetManifest = %q, want %q", bp.Name(), "new-plugin")
	}
	if bp.Version() != "2.0.0" {
		t.Errorf("Version() after SetManifest = %q, want %q", bp.Version(), "2.0.0")
	}
	if bp.Description() != "A test plugin" {
		t.Errorf("Description() = %q, want %q", bp.Description(), "A test plugin")
	}
	if bp.Author() != "Test Author" {
		t.Errorf("Author() = %q, want %q", bp.Author(), "Test Author")
	}
}

// --- BasePlugin lifecycle with embedding ---

type testPlugin struct {
	*BasePlugin
	initCalled  bool
	startCalled bool
	stopCalled  bool
}

func (p *testPlugin) Init(ctx core.CoreContext) error {
	p.initCalled = true
	return p.BasePlugin.Init(ctx)
}

func (p *testPlugin) Start() error {
	p.startCalled = true
	return nil
}

func (p *testPlugin) Stop() error {
	p.stopCalled = true
	return nil
}

func TestEmbeddedBasePlugin_Lifecycle(t *testing.T) {
	tp := &testPlugin{
		BasePlugin: NewBasePlugin("embedded-test", "1.0.0"),
	}
	container := services.NewContainer()
	ctx := newMockCoreContext(container)

	// Init
	if err := tp.Init(ctx); err != nil {
		t.Fatalf("Init error: %v", err)
	}
	if !tp.initCalled {
		t.Error("Custom Init was not called")
	}
	if tp.Context() != ctx {
		t.Error("Context not stored via BasePlugin.Init")
	}

	// Start
	if err := tp.Start(); err != nil {
		t.Fatalf("Start error: %v", err)
	}
	if !tp.startCalled {
		t.Error("Custom Start was not called")
	}

	// Stop
	if err := tp.Stop(); err != nil {
		t.Fatalf("Stop error: %v", err)
	}
	if !tp.stopCalled {
		t.Error("Custom Stop was not called")
	}
}

// --- Manifest file loading tests ---

func TestNewBasePluginFromFile(t *testing.T) {
	dir := t.TempDir()
	manifestContent := `
name: file-plugin
version: 1.2.3
description: A plugin loaded from file
author: Test Author
engine: native
`
	err := os.WriteFile(filepath.Join(dir, "plugin.yaml"), []byte(manifestContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write test manifest: %v", err)
	}

	bp, err := NewBasePluginFromFile(dir)
	if err != nil {
		t.Fatalf("NewBasePluginFromFile error: %v", err)
	}

	if bp.Name() != "file-plugin" {
		t.Errorf("Name() = %q, want %q", bp.Name(), "file-plugin")
	}
	if bp.Version() != "1.2.3" {
		t.Errorf("Version() = %q, want %q", bp.Version(), "1.2.3")
	}
	if bp.Description() != "A plugin loaded from file" {
		t.Errorf("Description() = %q, want %q", bp.Description(), "A plugin loaded from file")
	}
	if bp.Author() != "Test Author" {
		t.Errorf("Author() = %q, want %q", bp.Author(), "Test Author")
	}
	if bp.Manifest() == nil {
		t.Error("Manifest() should not be nil after loading from file")
	}
}

func TestNewBasePluginFromFile_ManifestYml(t *testing.T) {
	dir := t.TempDir()
	manifestContent := `
name: yml-plugin
version: 0.1.0
engine: native
`
	err := os.WriteFile(filepath.Join(dir, "plugin.yml"), []byte(manifestContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write test manifest: %v", err)
	}

	bp, err := NewBasePluginFromFile(dir)
	if err != nil {
		t.Fatalf("NewBasePluginFromFile error: %v", err)
	}
	if bp.Name() != "yml-plugin" {
		t.Errorf("Name() = %q, want %q", bp.Name(), "yml-plugin")
	}
}

func TestNewBasePluginFromFile_ManifestYaml(t *testing.T) {
	dir := t.TempDir()
	manifestContent := `
name: manifest-yaml-plugin
version: 0.1.0
engine: native
`
	err := os.WriteFile(filepath.Join(dir, "manifest.yaml"), []byte(manifestContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write test manifest: %v", err)
	}

	bp, err := NewBasePluginFromFile(dir)
	if err != nil {
		t.Fatalf("NewBasePluginFromFile error: %v", err)
	}
	if bp.Name() != "manifest-yaml-plugin" {
		t.Errorf("Name() = %q, want %q", bp.Name(), "manifest-yaml-plugin")
	}
}

func TestNewBasePluginFromFile_NotFound(t *testing.T) {
	dir := t.TempDir()
	_, err := NewBasePluginFromFile(dir)
	if err == nil {
		t.Fatal("Expected error when no manifest file exists")
	}
	if !strings.Contains(err.Error(), "no manifest file found") {
		t.Errorf("Error = %q, should mention no manifest file found", err.Error())
	}
}

func TestMustNewBasePluginFromFile(t *testing.T) {
	dir := t.TempDir()
	manifestContent := `
name: must-plugin
version: 1.0.0
engine: native
`
	os.WriteFile(filepath.Join(dir, "plugin.yaml"), []byte(manifestContent), 0644)

	bp := MustNewBasePluginFromFile(dir)
	if bp.Name() != "must-plugin" {
		t.Errorf("Name() = %q, want %q", bp.Name(), "must-plugin")
	}
}

func TestMustNewBasePluginFromFile_PanicsOnMissing(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("Expected panic for missing manifest")
		}
	}()
	MustNewBasePluginFromFile(t.TempDir())
}

// --- findManifest tests ---

func TestFindManifest(t *testing.T) {
	dir := t.TempDir()

	// No files → error
	_, err := findManifest(dir)
	if err == nil {
		t.Fatal("Expected error for empty directory")
	}

	// plugin.yaml takes priority
	os.WriteFile(filepath.Join(dir, "plugin.yaml"), []byte("test"), 0644)
	path, err := findManifest(dir)
	if err != nil {
		t.Fatalf("findManifest error: %v", err)
	}
	if filepath.Base(path) != "plugin.yaml" {
		t.Errorf("Expected plugin.yaml, got %s", filepath.Base(path))
	}
}

// --- Helper function tests ---

func TestGetDB_ServiceNotRegistered(t *testing.T) {
	container := services.NewContainer()
	_, err := GetDB(container)
	if err == nil {
		t.Fatal("Expected error when database service not registered")
	}
	if !strings.Contains(err.Error(), "database service not available") {
		t.Errorf("Error = %q, should mention database service not available", err.Error())
	}
}

func TestGetAuth_ServiceNotRegistered(t *testing.T) {
	container := services.NewContainer()
	_, err := GetAuth(container)
	if err == nil {
		t.Fatal("Expected error when auth service not registered")
	}
}

func TestGetContent_ServiceNotRegistered(t *testing.T) {
	container := services.NewContainer()
	_, err := GetContent(container)
	if err == nil {
		t.Fatal("Expected error when content service not registered")
	}
}

func TestGetMedia_ServiceNotRegistered(t *testing.T) {
	container := services.NewContainer()
	_, err := GetMedia(container)
	if err == nil {
		t.Fatal("Expected error when media service not registered")
	}
}

func TestGetSearch_ServiceNotRegistered(t *testing.T) {
	container := services.NewContainer()
	_, err := GetSearch(container)
	if err == nil {
		t.Fatal("Expected error when search service not registered")
	}
}

func TestGetCache_ServiceNotRegistered(t *testing.T) {
	container := services.NewContainer()
	_, err := GetCache(container)
	if err == nil {
		t.Fatal("Expected error when cache service not registered")
	}
}

func TestGetQueue_ServiceNotRegistered(t *testing.T) {
	container := services.NewContainer()
	_, err := GetQueue(container)
	if err == nil {
		t.Fatal("Expected error when queue service not registered")
	}
}

func TestGetTheme_ServiceNotRegistered(t *testing.T) {
	container := services.NewContainer()
	_, err := GetTheme(container)
	if err == nil {
		t.Fatal("Expected error when theme service not registered")
	}
}

func TestGetRouter_ServiceNotRegistered(t *testing.T) {
	container := services.NewContainer()
	_, err := GetRouter(container)
	if err == nil {
		t.Fatal("Expected error when router not registered")
	}
}

// --- MustGet helpers panic tests ---

func TestMustGetDB_Panics(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("Expected MustGetDB to panic")
		}
	}()
	container := services.NewContainer()
	MustGetDB(container)
}

func TestMustGetAuth_Panics(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("Expected MustGetAuth to panic")
		}
	}()
	container := services.NewContainer()
	MustGetAuth(container)
}

func TestMustGetContent_Panics(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("Expected MustGetContent to panic")
		}
	}()
	container := services.NewContainer()
	MustGetContent(container)
}

func TestMustGetMedia_Panics(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("Expected MustGetMedia to panic")
		}
	}()
	container := services.NewContainer()
	MustGetMedia(container)
}

func TestMustGetSearch_Panics(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("Expected MustGetSearch to panic")
		}
	}()
	container := services.NewContainer()
	MustGetSearch(container)
}

func TestMustGetCache_Panics(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("Expected MustGetCache to panic")
		}
	}()
	container := services.NewContainer()
	MustGetCache(container)
}

func TestMustGetQueue_Panics(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("Expected MustGetQueue to panic")
		}
	}()
	container := services.NewContainer()
	MustGetQueue(container)
}

func TestMustGetTheme_Panics(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("Expected MustGetTheme to panic")
		}
	}()
	container := services.NewContainer()
	MustGetTheme(container)
}

func TestMustGetRouter_Panics(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("Expected MustGetRouter to panic")
		}
	}()
	container := services.NewContainer()
	MustGetRouter(container)
}

// --- Event subscription helper tests ---

func TestSubscribeEvent(t *testing.T) {
	bus := events.NewEventBus()
	mockCtx := &mockCoreContext{
		services: services.NewContainer(),
		eventBus: bus,
		config:   newMockConfig(),
		logger:   slog.Default(),
		ctx:      context.Background(),
	}

	done := make(chan struct{})
	handlerID := SubscribeEvent(mockCtx, "test.event", func(ctx context.Context, event events.Event) {
		close(done)
	})

	if handlerID == "" {
		t.Fatal("SubscribeEvent returned empty handler ID")
	}

	// Emit the event
	bus.Emit(context.Background(), events.Event{Topic: "test.event", Data: map[string]interface{}{"key": "value"}})

	// Wait for async handler with timeout
	select {
	case <-done:
		// success
	case <-time.After(2 * time.Second):
		t.Fatal("Event handler was not called within timeout")
	}
}

func TestOnContentCreated_AllTypes(t *testing.T) {
	bus := events.NewEventBus()
	mockCtx := &mockCoreContext{
		services: services.NewContainer(),
		eventBus: bus,
		config:   newMockConfig(),
		logger:   slog.Default(),
		ctx:      context.Background(),
	}

	done := make(chan struct{})
	handlerID := OnContentCreated(mockCtx, "", func(ctx context.Context, event events.Event) {
		close(done)
	})

	if handlerID == "" {
		t.Fatal("OnContentCreated should return non-empty handler ID")
	}

	bus.Emit(context.Background(), events.Event{
		Topic: "content.post.created",
		Data:  map[string]interface{}{"id": "123"},
	})

	select {
	case <-done:
		// success
	case <-time.After(2 * time.Second):
		t.Fatal("OnContentCreated handler not called for wildcard topic within timeout")
	}
}

func TestOnContentCreated_SpecificType(t *testing.T) {
	bus := events.NewEventBus()
	mockCtx := &mockCoreContext{
		services: services.NewContainer(),
		eventBus: bus,
		config:   newMockConfig(),
		logger:   slog.Default(),
		ctx:      context.Background(),
	}

	done := make(chan struct{})
	handlerID := OnContentCreated(mockCtx, "post", func(ctx context.Context, event events.Event) {
		close(done)
	})

	if handlerID == "" {
		t.Fatal("OnContentCreated should return non-empty handler ID")
	}

	bus.Emit(context.Background(), events.Event{
		Topic: "content.post.created",
		Data:  map[string]interface{}{"id": "123"},
	})

	select {
	case <-done:
		// success
	case <-time.After(2 * time.Second):
		t.Fatal("OnContentCreated handler not called for specific content type within timeout")
	}
}

// --- Integration: full lifecycle with service access ---

func TestFullLifecycle_WithServices(t *testing.T) {
	container := services.NewContainer()

	// Register a mock cache service
	err := container.Provide(func(c *services.Container) (interfaces.CacheService, error) {
		return &mockCacheService{}, nil
	})
	if err != nil {
		t.Fatalf("Failed to register cache service: %v", err)
	}

	bp := NewBasePlugin("lifecycle-test", "1.0.0")
	ctx := newMockCoreContext(container)

	// Init
	if err := bp.Init(ctx); err != nil {
		t.Fatalf("Init error: %v", err)
	}

	// Access service via helper
	cache, err := GetCache(ctx.Services())
	if err != nil {
		t.Fatalf("GetCache error: %v", err)
	}
	if cache == nil {
		t.Fatal("GetCache returned nil")
	}

	// Start
	if err := bp.Start(); err != nil {
		t.Fatalf("Start error: %v", err)
	}

	// Stop
	if err := bp.Stop(); err != nil {
		t.Fatalf("Stop error: %v", err)
	}
}

// mockCacheService implements interfaces.CacheService for testing.
type mockCacheService struct{}

func (m *mockCacheService) Get(ctx context.Context, key string) (interface{}, bool) {
	return nil, false
}
func (m *mockCacheService) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	return nil
}
func (m *mockCacheService) Delete(ctx context.Context, key string) error { return nil }
func (m *mockCacheService) Invalidate(ctx context.Context, pattern string) error {
	return nil
}
func (m *mockCacheService) Stats(ctx context.Context) *interfaces.CacheStats {
	return &interfaces.CacheStats{}
}
func (m *mockCacheService) Flush(ctx context.Context) error { return nil }

// --- Helper success path tests using registered services ---

func registerAllMockServices(t *testing.T, container *services.Container) {
	t.Helper()

	// DatabaseService
	err := container.Provide(func(c *services.Container) (interfaces.DatabaseService, error) {
		return &mockDBSvc{}, nil
	})
	if err != nil {
		t.Fatalf("register db: %v", err)
	}

	// AuthService
	err = container.Provide(func(c *services.Container) (interfaces.AuthService, error) {
		return &mockAuthSvc{}, nil
	})
	if err != nil {
		t.Fatalf("register auth: %v", err)
	}

	// ContentService
	err = container.Provide(func(c *services.Container) (interfaces.ContentService, error) {
		return &mockContentSvc{}, nil
	})
	if err != nil {
		t.Fatalf("register content: %v", err)
	}

	// MediaService
	err = container.Provide(func(c *services.Container) (interfaces.MediaService, error) {
		return &mockMediaSvc{}, nil
	})
	if err != nil {
		t.Fatalf("register media: %v", err)
	}

	// SearchService
	err = container.Provide(func(c *services.Container) (interfaces.SearchService, error) {
		return &mockSearchSvc{}, nil
	})
	if err != nil {
		t.Fatalf("register search: %v", err)
	}

	// CacheService
	err = container.Provide(func(c *services.Container) (interfaces.CacheService, error) {
		return &mockCacheService{}, nil
	})
	if err != nil {
		t.Fatalf("register cache: %v", err)
	}

	// QueueService
	err = container.Provide(func(c *services.Container) (interfaces.QueueService, error) {
		return &mockQueueSvc{}, nil
	})
	if err != nil {
		t.Fatalf("register queue: %v", err)
	}

	// ThemeService
	err = container.Provide(func(c *services.Container) (interfaces.ThemeService, error) {
		return &mockThemeSvc{}, nil
	})
	if err != nil {
		t.Fatalf("register theme: %v", err)
	}

	// RouteRegistrar
	err = container.Provide(func(c *services.Container) (interfaces.RouteRegistrar, error) {
		return &mockRouteRegistrar{}, nil
	})
	if err != nil {
		t.Fatalf("register router: %v", err)
	}
}

func TestGetHelpers_SuccessPaths(t *testing.T) {
	container := services.NewContainer()
	registerAllMockServices(t, container)

	db, err := GetDB(container)
	if err != nil || db == nil {
		t.Fatalf("GetDB: err=%v, db=%v", err, db)
	}
	MustGetDB(container)

	auth, err := GetAuth(container)
	if err != nil || auth == nil {
		t.Fatalf("GetAuth: err=%v", err)
	}
	MustGetAuth(container)

	content, err := GetContent(container)
	if err != nil || content == nil {
		t.Fatalf("GetContent: err=%v", err)
	}
	MustGetContent(container)

	media, err := GetMedia(container)
	if err != nil || media == nil {
		t.Fatalf("GetMedia: err=%v", err)
	}
	MustGetMedia(container)

	search, err := GetSearch(container)
	if err != nil || search == nil {
		t.Fatalf("GetSearch: err=%v", err)
	}
	MustGetSearch(container)

	cache, err := GetCache(container)
	if err != nil || cache == nil {
		t.Fatalf("GetCache: err=%v", err)
	}
	MustGetCache(container)

	queue, err := GetQueue(container)
	if err != nil || queue == nil {
		t.Fatalf("GetQueue: err=%v", err)
	}
	MustGetQueue(container)

	theme, err := GetTheme(container)
	if err != nil || theme == nil {
		t.Fatalf("GetTheme: err=%v", err)
	}
	MustGetTheme(container)

	router, err := GetRouter(container)
	if err != nil || router == nil {
		t.Fatalf("GetRouter: err=%v", err)
	}
	MustGetRouter(container)
}

// Minimal mock service stubs for each interface.
// Only need to satisfy the interface; methods are not called in these tests.

type mockDBSvc struct{}

func (m *mockDBSvc) Query(ctx context.Context, q string, args ...interface{}) (*sql.Rows, error) {
	return nil, nil
}
func (m *mockDBSvc) QueryRow(ctx context.Context, q string, args ...interface{}) *sql.Row {
	return nil
}
func (m *mockDBSvc) Exec(ctx context.Context, q string, args ...interface{}) (sql.Result, error) {
	return nil, nil
}
func (m *mockDBSvc) BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
	return nil, nil
}
func (m *mockDBSvc) Ping(ctx context.Context) error                          { return nil }
func (m *mockDBSvc) Close() error                                            { return nil }
func (m *mockDBSvc) SchemaIntrospect(ctx context.Context) (*interfaces.DatabaseSchema, error) {
	return nil, nil
}
func (m *mockDBSvc) Prepare(ctx context.Context, q string) (*sql.Stmt, error) {
	return nil, nil
}

type mockAuthSvc struct{}

func (m *mockAuthSvc) Authenticate(ctx context.Context, req *interfaces.AuthRequest) (*interfaces.AuthResult, error) {
	return nil, nil
}
func (m *mockAuthSvc) VerifyToken(ctx context.Context, token string) (*interfaces.UserClaims, error) {
	return nil, nil
}
func (m *mockAuthSvc) RefreshToken(ctx context.Context, token string) (*interfaces.TokenPair, error) {
	return nil, nil
}
func (m *mockAuthSvc) CreateUser(ctx context.Context, req *interfaces.CreateUserRequest) (*interfaces.User, error) {
	return nil, nil
}
func (m *mockAuthSvc) GetUser(ctx context.Context, id string) (*interfaces.User, error) {
	return nil, nil
}
func (m *mockAuthSvc) HasPermission(ctx context.Context, uid, res, act string) (bool, error) {
	return false, nil
}
func (m *mockAuthSvc) CreateAPIToken(ctx context.Context, uid, name string, exp *time.Time) (*interfaces.APIToken, error) {
	return nil, nil
}
func (m *mockAuthSvc) RevokeAPIToken(ctx context.Context, id string) error { return nil }
func (m *mockAuthSvc) UpdateUser(ctx context.Context, id string, req *interfaces.UpdateUserRequest) (*interfaces.User, error) {
	return nil, nil
}
func (m *mockAuthSvc) DeleteUser(ctx context.Context, id string) error      { return nil }
func (m *mockAuthSvc) ListUsers(ctx context.Context, q *interfaces.UserQuery) (*interfaces.Page, error) {
	return nil, nil
}

type mockContentSvc struct{}

func (m *mockContentSvc) Create(ctx context.Context, ct string, data map[string]interface{}) (*interfaces.Content, error) {
	return nil, nil
}
func (m *mockContentSvc) GetByID(ctx context.Context, id string) (*interfaces.Content, error) {
	return nil, nil
}
func (m *mockContentSvc) Update(ctx context.Context, id string, data map[string]interface{}) (*interfaces.Content, error) {
	return nil, nil
}
func (m *mockContentSvc) Delete(ctx context.Context, id string) error { return nil }
func (m *mockContentSvc) List(ctx context.Context, ct string, q *interfaces.ListQuery) (*interfaces.Page, error) {
	return nil, nil
}
func (m *mockContentSvc) GetContentType(ctx context.Context, name string) (*interfaces.ContentType, error) {
	return nil, nil
}
func (m *mockContentSvc) CreateContentType(ctx context.Context, ct *interfaces.ContentType) (*interfaces.ContentType, error) {
	return nil, nil
}
func (m *mockContentSvc) UpdateContentType(ctx context.Context, name string, ct *interfaces.ContentType) (*interfaces.ContentType, error) {
	return nil, nil
}
func (m *mockContentSvc) DeleteContentType(ctx context.Context, name string) error { return nil }
func (m *mockContentSvc) ListContentTypes(ctx context.Context) ([]*interfaces.ContentType, error) {
	return nil, nil
}

type mockMediaSvc struct{}

func (m *mockMediaSvc) Upload(ctx context.Context, r io.Reader, filename string, contentType string, size int64, uid string) (*interfaces.MediaFile, error) {
	return nil, nil
}
func (m *mockMediaSvc) GetByID(ctx context.Context, id string) (*interfaces.MediaFile, error) {
	return nil, nil
}
func (m *mockMediaSvc) Delete(ctx context.Context, id string) error { return nil }
func (m *mockMediaSvc) List(ctx context.Context, q *interfaces.ListQuery) (*interfaces.Page, error) {
	return nil, nil
}
func (m *mockMediaSvc) GetURL(ctx context.Context, id string) (string, error) { return "", nil }
func (m *mockMediaSvc) GenerateThumbnail(ctx context.Context, id string, w, h int) (string, error) {
	return "", nil
}

type mockSearchSvc struct{}

func (m *mockSearchSvc) Index(ctx context.Context, ct string, c *interfaces.Content) error {
	return nil
}
func (m *mockSearchSvc) Remove(ctx context.Context, id string) error { return nil }
func (m *mockSearchSvc) Search(ctx context.Context, q *interfaces.SearchQuery) (*interfaces.SearchResponse, error) {
	return nil, nil
}
func (m *mockSearchSvc) Rebuild(ctx context.Context) error { return nil }
func (m *mockSearchSvc) GetFacets(ctx context.Context, ct string, fields []string) (map[string]map[string]int64, error) {
	return nil, nil
}

type mockQueueSvc struct{}

func (m *mockQueueSvc) RegisterTask(ctx context.Context, name string, handler interfaces.TaskHandler) error {
	return nil
}
func (m *mockQueueSvc) Enqueue(ctx context.Context, name string, payload interface{}, opts *interfaces.TaskOptions) (string, error) {
	return "", nil
}
func (m *mockQueueSvc) GetStatus(ctx context.Context, id string) (*interfaces.TaskStatus, error) {
	return nil, nil
}
func (m *mockQueueSvc) Close(ctx context.Context) error { return nil }
func (m *mockQueueSvc) ListDeadLetters(ctx context.Context, page, size int) ([]*interfaces.DeadLetterEntry, int, error) {
	return nil, 0, nil
}
func (m *mockQueueSvc) RetryDeadLetter(ctx context.Context, id string) error { return nil }
func (m *mockQueueSvc) DeleteDeadLetter(ctx context.Context, id string) error { return nil }
func (m *mockQueueSvc) WorkerCount() int                                  { return 0 }

type mockThemeSvc struct{}

func (m *mockThemeSvc) Render(ctx context.Context, tpl string, data map[string]interface{}) (string, error) {
	return "", nil
}
func (m *mockThemeSvc) GetActiveTheme(ctx context.Context) (string, error) { return "", nil }
func (m *mockThemeSvc) SetActiveTheme(ctx context.Context, name string) error {
	return nil
}
func (m *mockThemeSvc) ListThemes(ctx context.Context) ([]string, error) { return nil, nil }
func (m *mockThemeSvc) InstallTheme(ctx context.Context, path string) error {
	return nil
}

type mockRouteRegistrar struct{}

func (m *mockRouteRegistrar) Handle(pattern string, handler http.Handler)                   {}
func (m *mockRouteRegistrar) HandleFunc(pattern string, handler http.HandlerFunc)            {}
func (m *mockRouteRegistrar) Use(mw ...func(http.Handler) http.Handler)                     {}
func (m *mockRouteRegistrar) Middlewares() []func(http.Handler) http.Handler { return nil }
