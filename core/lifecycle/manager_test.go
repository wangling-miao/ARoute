package lifecycle

import (
	"context"
	"errors"
	"testing"

	"github.com/wangling-miao/aroute/core"
	"github.com/wangling-miao/aroute/core/events"
)

// mockPlugin implements core.Plugin for testing.
type mockPlugin struct {
	name        string
	version     string
	manifest    *core.Manifest
	initErr     error
	startErr    error
	stopErr     error
	initCalled  bool
	startCalled bool
	stopCalled  bool
}

func newMockPlugin(name, version string) *mockPlugin {
	return &mockPlugin{
		name:    name,
		version: version,
		manifest: &core.Manifest{
			Name:    name,
			Version: version,
			Engine:  "native",
		},
	}
}

func (p *mockPlugin) Name() string             { return p.name }
func (p *mockPlugin) Version() string          { return p.version }
func (p *mockPlugin) Manifest() *core.Manifest { return p.manifest }
func (p *mockPlugin) Init(ctx core.CoreContext) error {
	p.initCalled = true
	return p.initErr
}
func (p *mockPlugin) Start() error {
	p.startCalled = true
	return p.startErr
}
func (p *mockPlugin) Stop() error {
	p.stopCalled = true
	return p.stopErr
}

// mockRegistry implements PluginRegistry for testing.
type mockRegistry struct {
	plugins map[string]core.Manifest
	enabled map[string]bool
}

func newMockRegistry() *mockRegistry {
	return &mockRegistry{
		plugins: make(map[string]core.Manifest),
		enabled: make(map[string]bool),
	}
}

func (r *mockRegistry) List() ([]core.Manifest, error) {
	var result []core.Manifest
	for _, m := range r.plugins {
		result = append(result, m)
	}
	return result, nil
}

func (r *mockRegistry) Get(name string) (core.Manifest, error) {
	m, ok := r.plugins[name]
	if !ok {
		return core.Manifest{}, errors.New("not found")
	}
	return m, nil
}

func (r *mockRegistry) IsEnabled(name string) (bool, error) {
	return r.enabled[name], nil
}

func (r *mockRegistry) Enable(name string) error {
	r.enabled[name] = true
	return nil
}

func (r *mockRegistry) Disable(name string) error {
	r.enabled[name] = false
	return nil
}

// mockLoader implements PluginLoader for testing.
type mockLoader struct {
	factory map[string]core.Plugin
}

func newMockLoader() *mockLoader {
	return &mockLoader{
		factory: make(map[string]core.Plugin),
	}
}

func (l *mockLoader) Load(manifest core.Manifest) (core.Plugin, error) {
	if p, ok := l.factory[manifest.Name]; ok {
		return p, nil
	}
	return newMockPlugin(manifest.Name, manifest.Version), nil
}

func newTestManager(registry PluginRegistry, loader PluginLoader) *ManagerImpl {
	return NewManager(registry, loader, nil, nil, nil)
}

// TestLoadAll tests loading all plugins from the registry.
func TestLoadAll(t *testing.T) {
	t.Run("loads enabled plugins successfully", func(t *testing.T) {
		registry := newMockRegistry()
		loader := newMockLoader()

		registry.plugins["plugin-a"] = core.Manifest{Name: "plugin-a", Version: "1.0.0", Engine: "native"}
		registry.plugins["plugin-b"] = core.Manifest{Name: "plugin-b", Version: "1.0.0", Engine: "native"}
		registry.enabled["plugin-a"] = true
		registry.enabled["plugin-b"] = true

		manager := newTestManager(registry, loader)
		err := manager.LoadAll(context.Background())
		if err != nil {
			t.Fatalf("LoadAll failed: %v", err)
		}

		plugins := manager.ListPlugins()
		if len(plugins) != 2 {
			t.Errorf("expected 2 plugins, got %d", len(plugins))
		}

		state, err := manager.GetState("plugin-a")
		if err != nil {
			t.Errorf("GetState failed: %v", err)
		}
		if state != core.StateRegistered {
			t.Errorf("plugin-a state = %v, want %v", state, core.StateRegistered)
		}
	})

	t.Run("skips disabled plugins", func(t *testing.T) {
		registry := newMockRegistry()
		loader := newMockLoader()

		registry.plugins["plugin-a"] = core.Manifest{Name: "plugin-a", Version: "1.0.0", Engine: "native"}
		registry.plugins["plugin-b"] = core.Manifest{Name: "plugin-b", Version: "1.0.0", Engine: "native"}
		registry.enabled["plugin-a"] = true
		registry.enabled["plugin-b"] = false

		manager := newTestManager(registry, loader)
		err := manager.LoadAll(context.Background())
		if err != nil {
			t.Fatalf("LoadAll failed: %v", err)
		}

		plugins := manager.ListPlugins()
		if len(plugins) != 1 {
			t.Errorf("expected 1 plugin, got %d", len(plugins))
		}

		_, err = manager.GetState("plugin-b")
		if err == nil {
			t.Error("expected error for disabled plugin")
		}
	})
}

// TestStateTransitions tests valid and invalid state transitions.
func TestStateTransitions(t *testing.T) {
	tests := []struct {
		name  string
		from  core.PluginState
		to    core.PluginState
		valid bool
	}{
		{"Registered to Resolved", core.StateRegistered, core.StateResolved, true},
		{"Registered to Starting", core.StateRegistered, core.StateStarting, false},
		{"Resolved to Starting", core.StateResolved, core.StateStarting, true},
		{"Starting to Active", core.StateStarting, core.StateActive, true},
		{"Active to Stopping", core.StateActive, core.StateStopping, true},
		{"Stopping to Stopped", core.StateStopping, core.StateStopped, true},
		{"Stopped to Starting", core.StateStopped, core.StateStarting, true},
		{"Active to Active", core.StateActive, core.StateActive, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := canTransition(tt.from, tt.to)
			if result != tt.valid {
				t.Errorf("canTransition(%v, %v) = %v, want %v", tt.from, tt.to, result, tt.valid)
			}
		})
	}
}

// TestDependencyOrder tests topological sorting with dependencies.
func TestDependencyOrder(t *testing.T) {
	t.Run("simple dependency chain", func(t *testing.T) {
		registry := newMockRegistry()
		loader := newMockLoader()

		pluginA := newMockPlugin("plugin-a", "1.0.0")
		pluginB := newMockPlugin("plugin-b", "1.0.0")
		pluginB.manifest.Requires = []string{"plugin-a"}

		loader.factory["plugin-a"] = pluginA
		loader.factory["plugin-b"] = pluginB

		registry.plugins["plugin-a"] = *pluginA.manifest
		registry.plugins["plugin-b"] = *pluginB.manifest
		registry.enabled["plugin-a"] = true
		registry.enabled["plugin-b"] = true

		manager := newTestManager(registry, loader)
		err := manager.LoadAll(context.Background())
		if err != nil {
			t.Fatalf("LoadAll failed: %v", err)
		}

		err = manager.Start(context.Background())
		if err != nil {
			t.Fatalf("Start failed: %v", err)
		}

		order := manager.order
		aIndex := indexOf(order, "plugin-a")
		bIndex := indexOf(order, "plugin-b")

		if aIndex >= bIndex {
			t.Errorf("plugin-a (index %d) should come before plugin-b (index %d)", aIndex, bIndex)
		}
	})

	t.Run("cycle detection", func(t *testing.T) {
		registry := newMockRegistry()
		loader := newMockLoader()

		pluginA := newMockPlugin("plugin-a", "1.0.0")
		pluginB := newMockPlugin("plugin-b", "1.0.0")
		pluginA.manifest.Requires = []string{"plugin-b"}
		pluginB.manifest.Requires = []string{"plugin-a"}

		loader.factory["plugin-a"] = pluginA
		loader.factory["plugin-b"] = pluginB

		registry.plugins["plugin-a"] = *pluginA.manifest
		registry.plugins["plugin-b"] = *pluginB.manifest
		registry.enabled["plugin-a"] = true
		registry.enabled["plugin-b"] = true

		manager := newTestManager(registry, loader)
		err := manager.LoadAll(context.Background())
		if err != nil {
			t.Fatalf("LoadAll failed: %v", err)
		}

		err = manager.Start(context.Background())
		if err == nil {
			t.Error("expected cycle detection error")
		}

		var depErr *DependencyError
		if !errors.As(err, &depErr) {
			t.Errorf("expected DependencyError, got %T", err)
		}

		if len(depErr.CyclePath) == 0 {
			t.Error("expected cycle path in error")
		}
	})
}

// TestStartupFailure tests handling of startup failures.
func TestStartupFailure(t *testing.T) {
	t.Run("plugin init failure", func(t *testing.T) {
		registry := newMockRegistry()
		loader := newMockLoader()

		pluginA := newMockPlugin("plugin-a", "1.0.0")
		pluginA.initErr = errors.New("init failed")

		loader.factory["plugin-a"] = pluginA
		registry.plugins["plugin-a"] = *pluginA.manifest
		registry.enabled["plugin-a"] = true

		manager := newTestManager(registry, loader)
		_ = manager.LoadAll(context.Background())
		err := manager.Start(context.Background())

		if err == nil {
			t.Error("expected startup error")
		}

		state, _ := manager.GetState("plugin-a")
		if state != core.StateFailed {
			t.Errorf("plugin-a state = %v, want %v", state, core.StateFailed)
		}
	})

	t.Run("startup failure skips dependents", func(t *testing.T) {
		registry := newMockRegistry()
		loader := newMockLoader()

		pluginA := newMockPlugin("plugin-a", "1.0.0")
		pluginA.initErr = errors.New("init failed")

		pluginB := newMockPlugin("plugin-b", "1.0.0")
		pluginB.manifest.Requires = []string{"plugin-a"}

		loader.factory["plugin-a"] = pluginA
		loader.factory["plugin-b"] = pluginB

		registry.plugins["plugin-a"] = *pluginA.manifest
		registry.plugins["plugin-b"] = *pluginB.manifest
		registry.enabled["plugin-a"] = true
		registry.enabled["plugin-b"] = true

		manager := newTestManager(registry, loader)
		_ = manager.LoadAll(context.Background())
		_ = manager.Start(context.Background())

		stateB, _ := manager.GetState("plugin-b")
		if stateB != core.StateFailed {
			t.Errorf("plugin-b state = %v, want %v", stateB, core.StateFailed)
		}
	})
}

// TestHotPlug tests enable/disable at runtime.
func TestHotPlug(t *testing.T) {
	t.Run("enable disabled plugin", func(t *testing.T) {
		registry := newMockRegistry()
		loader := newMockLoader()

		pluginA := newMockPlugin("plugin-a", "1.0.0")
		loader.factory["plugin-a"] = pluginA
		registry.plugins["plugin-a"] = *pluginA.manifest
		registry.enabled["plugin-a"] = true

		manager := newTestManager(registry, loader)
		_ = manager.LoadAll(context.Background())
		_ = manager.Start(context.Background())

		_ = manager.Disable(context.Background(), "plugin-a")

		state, _ := manager.GetState("plugin-a")
		if state != core.StateStopped {
			t.Errorf("plugin-a state after disable = %v, want %v", state, core.StateStopped)
		}

		err := manager.Enable(context.Background(), "plugin-a")
		if err != nil {
			t.Errorf("Enable failed: %v", err)
		}

		state, _ = manager.GetState("plugin-a")
		if state != core.StateActive {
			t.Errorf("plugin-a state after enable = %v, want %v", state, core.StateActive)
		}
	})

	t.Run("disable with active dependents fails", func(t *testing.T) {
		registry := newMockRegistry()
		loader := newMockLoader()

		pluginA := newMockPlugin("plugin-a", "1.0.0")
		pluginB := newMockPlugin("plugin-b", "1.0.0")
		pluginB.manifest.Requires = []string{"plugin-a"}

		loader.factory["plugin-a"] = pluginA
		loader.factory["plugin-b"] = pluginB

		registry.plugins["plugin-a"] = *pluginA.manifest
		registry.plugins["plugin-b"] = *pluginB.manifest
		registry.enabled["plugin-a"] = true
		registry.enabled["plugin-b"] = true

		manager := newTestManager(registry, loader)
		_ = manager.LoadAll(context.Background())
		_ = manager.Start(context.Background())

		err := manager.Disable(context.Background(), "plugin-a")
		if err == nil {
			t.Error("expected error when disabling plugin with active dependents")
		}
	})
}

// TestShutdownOrder tests reverse topological order shutdown.
func TestShutdownOrder(t *testing.T) {
	registry := newMockRegistry()
	loader := newMockLoader()

	pluginA := newMockPlugin("plugin-a", "1.0.0")
	pluginB := newMockPlugin("plugin-b", "1.0.0")
	pluginB.manifest.Requires = []string{"plugin-a"}

	loader.factory["plugin-a"] = pluginA
	loader.factory["plugin-b"] = pluginB

	registry.plugins["plugin-a"] = *pluginA.manifest
	registry.plugins["plugin-b"] = *pluginB.manifest
	registry.enabled["plugin-a"] = true
	registry.enabled["plugin-b"] = true

	manager := newTestManager(registry, loader)
	_ = manager.LoadAll(context.Background())
	_ = manager.Start(context.Background())

	_ = manager.Stop(context.Background())

	if !pluginB.stopCalled {
		t.Error("plugin-b should be stopped first")
	}
	if !pluginA.stopCalled {
		t.Error("plugin-a should be stopped second")
	}
}

// Helper function to find index of element in slice.
func indexOf(slice []string, item string) int {
	for i, s := range slice {
		if s == item {
			return i
		}
	}
	return -1
}

// mockEventBus implements core.EventBus for testing.
type mockEventBus struct {
	emitted []events.Event
}

func (m *mockEventBus) SubscribeFilter(topic string, priority int, handler events.FilterHandler) string {
	return ""
}

func (m *mockEventBus) SubscribeBroadcast(topic string, handler events.BroadcastHandler) string {
	return ""
}

func (m *mockEventBus) Emit(ctx context.Context, event events.Event) {
	m.emitted = append(m.emitted, event)
}

func (m *mockEventBus) DispatchFilter(ctx context.Context, event *events.Event) (*events.Event, error) {
	return nil, nil
}

func (m *mockEventBus) Unsubscribe(handlerID string) {
}

// TestRetry tests the Retry method for recovering from Failed state.
func TestRetry(t *testing.T) {
	t.Run("retry failed plugin success", func(t *testing.T) {
		registry := newMockRegistry()
		loader := newMockLoader()

		pluginA := newMockPlugin("plugin-a", "1.0.0")
		pluginA.initErr = errors.New("init failed")
		loader.factory["plugin-a"] = pluginA
		registry.plugins["plugin-a"] = *pluginA.manifest
		registry.enabled["plugin-a"] = true

		manager := newTestManager(registry, loader)
		_ = manager.LoadAll(context.Background())
		_ = manager.Start(context.Background())

		state, _ := manager.GetState("plugin-a")
		if state != core.StateFailed {
			t.Fatalf("plugin-a state = %v, want %v", state, core.StateFailed)
		}

		pluginA.initErr = nil
		err := manager.Retry(context.Background(), "plugin-a")
		if err != nil {
			t.Errorf("Retry failed: %v", err)
		}

		state, _ = manager.GetState("plugin-a")
		if state != core.StateActive {
			t.Errorf("plugin-a state after retry = %v, want %v", state, core.StateActive)
		}
	})

	t.Run("retry non-failed plugin fails", func(t *testing.T) {
		registry := newMockRegistry()
		loader := newMockLoader()

		pluginA := newMockPlugin("plugin-a", "1.0.0")
		loader.factory["plugin-a"] = pluginA
		registry.plugins["plugin-a"] = *pluginA.manifest
		registry.enabled["plugin-a"] = true

		manager := newTestManager(registry, loader)
		_ = manager.LoadAll(context.Background())
		_ = manager.Start(context.Background())

		err := manager.Retry(context.Background(), "plugin-a")
		if err == nil {
			t.Error("expected error when retrying non-failed plugin")
		}
	})
}

// TestStateChangeEvents tests that state transitions emit events to EventBus.
func TestStateChangeEvents(t *testing.T) {
	t.Run("state changes emit events", func(t *testing.T) {
		registry := newMockRegistry()
		loader := newMockLoader()

		pluginA := newMockPlugin("plugin-a", "1.0.0")
		loader.factory["plugin-a"] = pluginA
		registry.plugins["plugin-a"] = *pluginA.manifest
		registry.enabled["plugin-a"] = true

		eventBus := &mockEventBus{}
		manager := NewManager(registry, loader, eventBus, nil, nil)

		_ = manager.LoadAll(context.Background())
		_ = manager.Start(context.Background())

		var stateChanges []StateChangeEventData
		for _, e := range eventBus.emitted {
			data, ok := e.Data["data"].(StateChangeEventData)
			if ok {
				stateChanges = append(stateChanges, data)
			}
		}

		if len(stateChanges) == 0 {
			t.Error("expected state change events to be emitted")
		}

		state, _ := manager.GetState("plugin-a")
		if state != core.StateActive {
			t.Errorf("plugin-a state = %v, want %v", state, core.StateActive)
		}
	})

	t.Run("nil eventBus does not panic", func(t *testing.T) {
		registry := newMockRegistry()
		loader := newMockLoader()

		pluginA := newMockPlugin("plugin-a", "1.0.0")
		loader.factory["plugin-a"] = pluginA
		registry.plugins["plugin-a"] = *pluginA.manifest
		registry.enabled["plugin-a"] = true

		manager := newTestManager(registry, loader)

		err := manager.LoadAll(context.Background())
		if err != nil {
			t.Fatalf("LoadAll failed: %v", err)
		}

		err = manager.Start(context.Background())
		if err != nil {
			t.Fatalf("Start failed: %v", err)
		}

		state, _ := manager.GetState("plugin-a")
		if state != core.StateActive {
			t.Errorf("plugin-a state = %v, want %v", state, core.StateActive)
		}
	})
}

// TestCoreContextPassing tests that CoreContext is passed to Init.
func TestCoreContextPassing(t *testing.T) {
	t.Run("context factory is called during init", func(t *testing.T) {
		registry := newMockRegistry()
		loader := newMockLoader()

		pluginA := newMockPlugin("plugin-a", "1.0.0")
		loader.factory["plugin-a"] = pluginA
		registry.plugins["plugin-a"] = *pluginA.manifest
		registry.enabled["plugin-a"] = true

		contextCalled := false
		ctxFactory := func(ctx context.Context, pluginName string) core.CoreContext {
			contextCalled = true
			if pluginName != "plugin-a" {
				t.Errorf("context factory called with wrong plugin name: %s", pluginName)
			}
			return nil
		}

		manager := NewManager(registry, loader, nil, nil, ctxFactory)
		_ = manager.LoadAll(context.Background())
		_ = manager.Start(context.Background())

		if !contextCalled {
			t.Error("expected context factory to be called")
		}
		if !pluginA.initCalled {
			t.Error("expected Init to be called")
		}
	})
}
