package core

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"
)

// mockPlugin implements Plugin for testing.
type mockPlugin struct {
	name       string
	version    string
	manifest   *Manifest
	initCalls  int
	startCalls int
	stopCalls  int
	initError  error
	startError error
	stopError  error
	mu         sync.Mutex
}

func newMockPlugin(name, version string) *mockPlugin {
	return &mockPlugin{
		name:    name,
		version: version,
		manifest: &Manifest{
			Name:    name,
			Version: version,
			Engine:  "native",
		},
	}
}

func (p *mockPlugin) Name() string        { return p.name }
func (p *mockPlugin) Version() string     { return p.version }
func (p *mockPlugin) Manifest() *Manifest { return p.manifest }

func (p *mockPlugin) Init(ctx CoreContext) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.initCalls++
	return p.initError
}

func (p *mockPlugin) Start() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.startCalls++
	return p.startError
}

func (p *mockPlugin) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stopCalls++
	return p.stopError
}

func (p *mockPlugin) getCallCounts() (int, int, int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.initCalls, p.startCalls, p.stopCalls
}

// mockServiceContainer implements ServiceContainer for testing.
type mockServiceContainer struct {
	services map[string]interface{}
	mu       sync.RWMutex
}

func newMockServiceContainer() *mockServiceContainer {
	return &mockServiceContainer{
		services: make(map[string]interface{}),
	}
}

func (c *mockServiceContainer) Provide(provider interface{}) error {
	return nil
}

func (c *mockServiceContainer) Get(target interface{}) error {
	return nil
}

func (c *mockServiceContainer) GetNamed(name string, target interface{}) error {
	return nil
}

func (c *mockServiceContainer) Unregister(target interface{}) error {
	return nil
}

func (c *mockServiceContainer) Has(target interface{}) bool {
	return false
}

func (c *mockServiceContainer) Keys() []string {
	return nil
}

// mockEventBus implements EventBus for testing.
type mockEventBus struct {
	handlers map[string][]interface{}
	mu       sync.RWMutex
}

func newMockEventBus() *mockEventBus {
	return &mockEventBus{
		handlers: make(map[string][]interface{}),
	}
}

func (b *mockEventBus) SubscribeFilter(event string, priority int, handler FilterHandler) string {
	return "filter-handler-id"
}

func (b *mockEventBus) SubscribeBroadcast(event string, handler BroadcastHandler) string {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers[event] = append(b.handlers[event], handler)
	return "broadcast-handler-id"
}

func (b *mockEventBus) Emit(ctx context.Context, event string, data interface{}) {
	b.mu.RLock()
	handlers := b.handlers[event]
	b.mu.RUnlock()
	for _, h := range handlers {
		if handler, ok := h.(BroadcastHandler); ok {
			go handler(ctx, event, data)
		}
	}
}

func (b *mockEventBus) DispatchFilter(ctx context.Context, event string, data interface{}) (interface{}, error) {
	return data, nil
}

func (b *mockEventBus) Unsubscribe(handlerID string) error {
	return nil
}

// mockPluginRegistry implements PluginRegistry for testing.
type mockPluginRegistry struct {
	plugins map[string]*PluginEntry
	mu      sync.RWMutex
	closed  bool
}

func newMockPluginRegistry() *mockPluginRegistry {
	return &mockPluginRegistry{
		plugins: make(map[string]*PluginEntry),
	}
}

func (r *mockPluginRegistry) Register(entry *PluginEntry) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return fmt.Errorf("registry closed")
	}
	r.plugins[entry.Manifest.Name] = entry
	return nil
}

func (r *mockPluginRegistry) Get(name string) (*PluginEntry, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.closed {
		return nil, fmt.Errorf("registry closed")
	}
	entry, ok := r.plugins[name]
	if !ok {
		return nil, fmt.Errorf("plugin not found: %s", name)
	}
	return entry, nil
}

func (r *mockPluginRegistry) List() ([]*PluginEntry, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.closed {
		return nil, fmt.Errorf("registry closed")
	}
	entries := make([]*PluginEntry, 0, len(r.plugins))
	for _, entry := range r.plugins {
		entries = append(entries, entry)
	}
	return entries, nil
}

func (r *mockPluginRegistry) Update(name string, manifest Manifest) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return fmt.Errorf("registry closed")
	}
	entry, ok := r.plugins[name]
	if !ok {
		return fmt.Errorf("plugin not found: %s", name)
	}
	entry.Manifest = manifest
	return nil
}

func (r *mockPluginRegistry) Remove(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return fmt.Errorf("registry closed")
	}
	delete(r.plugins, name)
	return nil
}

func (r *mockPluginRegistry) Enable(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return fmt.Errorf("registry closed")
	}
	entry, ok := r.plugins[name]
	if !ok {
		return fmt.Errorf("plugin not found: %s", name)
	}
	entry.Enabled = true
	return nil
}

func (r *mockPluginRegistry) Disable(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return fmt.Errorf("registry closed")
	}
	entry, ok := r.plugins[name]
	if !ok {
		return fmt.Errorf("plugin not found: %s", name)
	}
	entry.Enabled = false
	return nil
}

func (r *mockPluginRegistry) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closed = true
	return nil
}

// mockLifecycleManager implements LifecycleManager for testing.
type mockLifecycleManager struct {
	plugins    map[string]Plugin
	pluginInfo map[string]*mockPluginInfo
	mu         sync.RWMutex
	started    bool
}

type mockPluginInfo struct {
	state  PluginState
	plugin Plugin
}

func newMockLifecycleManager() *mockLifecycleManager {
	return &mockLifecycleManager{
		plugins:    make(map[string]Plugin),
		pluginInfo: make(map[string]*mockPluginInfo),
	}
}

func (m *mockLifecycleManager) LoadAll(ctx context.Context) error {
	return nil
}

func (m *mockLifecycleManager) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.started = true
	for name, info := range m.pluginInfo {
		info.state = StateStarting
		if p, ok := m.plugins[name]; ok {
			pluginCtx := &mockCoreContext{}
			if err := p.Init(pluginCtx); err != nil {
				info.state = StateFailed
				continue
			}
			if err := p.Start(); err != nil {
				info.state = StateFailed
				continue
			}
			info.state = StateActive
		}
	}
	return nil
}

func (m *mockLifecycleManager) Stop(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for name, p := range m.plugins {
		p.Stop()
		if info, ok := m.pluginInfo[name]; ok {
			info.state = StateStopped
		}
	}
	m.started = false
	return nil
}

func (m *mockLifecycleManager) Enable(ctx context.Context, pluginName string) error {
	return nil
}

func (m *mockLifecycleManager) Disable(ctx context.Context, pluginName string) error {
	return nil
}

func (m *mockLifecycleManager) GetState(pluginName string) (PluginState, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if info, ok := m.pluginInfo[pluginName]; ok {
		return info.state, nil
	}
	return StateRegistered, fmt.Errorf("plugin not found")
}

func (m *mockLifecycleManager) GetPlugin(pluginName string) Plugin {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.plugins[pluginName]
}

func (m *mockLifecycleManager) ListPlugins() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	names := make([]string, 0, len(m.plugins))
	for name := range m.plugins {
		names = append(names, name)
	}
	return names
}

func (m *mockLifecycleManager) registerPlugin(p Plugin) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.plugins[p.Name()] = p
	m.pluginInfo[p.Name()] = &mockPluginInfo{
		state:  StateRegistered,
		plugin: p,
	}
}

// mockEngineDispatcher implements EngineDispatcher for testing.
type mockEngineDispatcher struct {
	engines map[EngineType]EngineExecutor
	closed  bool
}

func newMockEngineDispatcher() *mockEngineDispatcher {
	return &mockEngineDispatcher{
		engines: make(map[EngineType]EngineExecutor),
	}
}

func (d *mockEngineDispatcher) RegisterEngine(engineType EngineType, engine EngineExecutor) error {
	d.engines[engineType] = engine
	return nil
}

func (d *mockEngineDispatcher) GetEngine(engineType EngineType) (EngineExecutor, error) {
	engine, ok := d.engines[engineType]
	if !ok {
		return nil, fmt.Errorf("engine not found")
	}
	return engine, nil
}

func (d *mockEngineDispatcher) Execute(ctx context.Context, plugin Plugin, manifest *Manifest, coreCtx CoreContext) error {
	return nil
}

func (d *mockEngineDispatcher) Close() error {
	d.closed = true
	return nil
}

// mockLicenseValidator implements LicenseValidator for testing.
type mockLicenseValidator struct {
	tier     LicenseTier
	features map[string]bool
	expired  bool
}

func newMockLicenseValidator() *mockLicenseValidator {
	return &mockLicenseValidator{
		tier:     LicenseTierOpen,
		features: make(map[string]bool),
	}
}

func (v *mockLicenseValidator) Tier() LicenseTier {
	return v.tier
}

func (v *mockLicenseValidator) IsFeatureAllowed(feature string) bool {
	return v.features[feature]
}

func (v *mockLicenseValidator) IsExpired() bool {
	return v.expired
}

func (v *mockLicenseValidator) Validate() error {
	return nil
}

func (v *mockLicenseValidator) LicenseInfo() LicenseInfoResult {
	return LicenseInfoResult{
		Tier:     v.tier,
		Features: []string{},
	}
}

// mockCoreContext implements CoreContext for testing.
type mockCoreContext struct{}

func (c *mockCoreContext) Services() ServiceContainer { return nil }
func (c *mockCoreContext) Events() EventBus           { return nil }
func (c *mockCoreContext) Config() ConfigProvider     { return nil }
func (c *mockCoreContext) Logger() *slog.Logger       { return nil }
func (c *mockCoreContext) DataDir() string            { return "" }
func (c *mockCoreContext) PluginDir() string          { return "" }
func (c *mockCoreContext) Context() context.Context   { return context.Background() }

// Tests

func TestAroute_New(t *testing.T) {
	ctx := context.Background()

	container := newMockServiceContainer()
	eventBus := newMockEventBus()
	registry := newMockPluginRegistry()
	lifecycle := newMockLifecycleManager()
	dispatcher := newMockEngineDispatcher()
	license := newMockLicenseValidator()

	// Create temp directories
	tmpDir, err := os.MkdirTemp("", "aroute-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	aroute, err := New(ctx, container, eventBus, registry, lifecycle, dispatcher, license,
		WithDataDir(tmpDir+"/data"),
		WithPluginDir(tmpDir+"/plugins"),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if aroute == nil {
		t.Fatal("New() returned nil")
	}

	if aroute.IsStarted() {
		t.Error("new engine should not be started")
	}

	if aroute.Services() != container {
		t.Error("Services() returned wrong container")
	}

	if aroute.Events() != eventBus {
		t.Error("Events() returned wrong event bus")
	}
}

func TestAroute_StartStop(t *testing.T) {
	ctx := context.Background()

	container := newMockServiceContainer()
	eventBus := newMockEventBus()
	registry := newMockPluginRegistry()
	lifecycle := newMockLifecycleManager()
	dispatcher := newMockEngineDispatcher()
	license := newMockLicenseValidator()

	tmpDir, err := os.MkdirTemp("", "aroute-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	aroute, err := New(ctx, container, eventBus, registry, lifecycle, dispatcher, license,
		WithDataDir(tmpDir+"/data"),
		WithPluginDir(tmpDir+"/plugins"),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	// Start
	if err := aroute.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if !aroute.IsStarted() {
		t.Error("engine should be started")
	}

	// Start again should fail
	if err := aroute.Start(ctx); err == nil {
		t.Error("Start() should fail when already started")
	}

	// Stop
	if err := aroute.Stop(ctx); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}

	if aroute.IsStarted() {
		t.Error("engine should be stopped")
	}

	// Stop again should be no-op
	if err := aroute.Stop(ctx); err != nil {
		t.Fatalf("Stop() error on stopped engine = %v", err)
	}
}

func TestAroute_RegisterPlugin(t *testing.T) {
	ctx := context.Background()

	container := newMockServiceContainer()
	eventBus := newMockEventBus()
	registry := newMockPluginRegistry()
	lifecycle := newMockLifecycleManager()
	dispatcher := newMockEngineDispatcher()
	license := newMockLicenseValidator()

	tmpDir, err := os.MkdirTemp("", "aroute-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	aroute, err := New(ctx, container, eventBus, registry, lifecycle, dispatcher, license,
		WithDataDir(tmpDir+"/data"),
		WithPluginDir(tmpDir+"/plugins"),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	plugin := newMockPlugin("test-plugin", "1.0.0")

	if err := aroute.RegisterPlugin(plugin); err != nil {
		t.Fatalf("RegisterPlugin() error = %v", err)
	}

	// Verify plugin was registered
	entry, err := registry.Get("test-plugin")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if entry.Manifest.Name != "test-plugin" {
		t.Errorf("expected plugin name 'test-plugin', got %q", entry.Manifest.Name)
	}

	if !entry.Enabled {
		t.Error("plugin should be enabled by default")
	}
}

func TestAroute_EventDispatch(t *testing.T) {
	ctx := context.Background()

	container := newMockServiceContainer()
	eventBus := newMockEventBus()
	registry := newMockPluginRegistry()
	lifecycle := newMockLifecycleManager()
	dispatcher := newMockEngineDispatcher()
	license := newMockLicenseValidator()

	tmpDir, err := os.MkdirTemp("", "aroute-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	aroute, err := New(ctx, container, eventBus, registry, lifecycle, dispatcher, license,
		WithDataDir(tmpDir+"/data"),
		WithPluginDir(tmpDir+"/plugins"),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	// Subscribe to an event
	received := make(chan bool, 1)
	eventBus.SubscribeBroadcast("test.event", func(ctx context.Context, event string, data interface{}) error {
		received <- true
		return nil
	})

	// Emit event
	aroute.Events().Emit(ctx, "test.event", map[string]interface{}{"key": "value"})

	// Wait for event
	select {
	case <-received:
		// Success
	case <-time.After(time.Second):
		t.Error("event was not received")
	}
}

func TestBuilder_Build(t *testing.T) {
	ctx := context.Background()

	tmpDir, err := os.MkdirTemp("", "aroute-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	container := newMockServiceContainer()
	eventBus := newMockEventBus()
	registry := newMockPluginRegistry()
	lifecycle := newMockLifecycleManager()
	dispatcher := newMockEngineDispatcher()
	license := newMockLicenseValidator()

	aroute, err := NewBuilder().
		WithDataDir(tmpDir + "/data").
		WithPluginDir(tmpDir + "/plugins").
		WithContainer(container).
		WithEventBus(eventBus).
		WithRegistry(registry).
		WithLifecycle(lifecycle).
		WithDispatcher(dispatcher).
		WithLicenseValidator(license).
		Build(ctx)

	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	if aroute == nil {
		t.Fatal("Build() returned nil")
	}

	if aroute.DataDir() != tmpDir+"/data" {
		t.Errorf("DataDir() = %q, want %q", aroute.DataDir(), tmpDir+"/data")
	}
}
