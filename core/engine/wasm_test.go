package engine

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/wangling-miao/aroute/core"
)

type mockServiceContainer struct {
	keys   []string
	hasVal bool
}

func (m *mockServiceContainer) Provide(provider interface{}) error             { return nil }
func (m *mockServiceContainer) Get(target interface{}) error                   { return nil }
func (m *mockServiceContainer) GetNamed(name string, target interface{}) error { return nil }
func (m *mockServiceContainer) Unregister(target interface{}) error            { return nil }
func (m *mockServiceContainer) Has(target interface{}) bool                    { return m.hasVal }
func (m *mockServiceContainer) Keys() []string                                 { return m.keys }

type mockEventBus struct {
	emitCalled bool
	emitTopic  string
	emitData   interface{}
}

func (m *mockEventBus) SubscribeFilter(event string, priority int, handler core.FilterHandler) string {
	return "handler-1"
}
func (m *mockEventBus) SubscribeBroadcast(event string, handler core.BroadcastHandler) string {
	return "handler-1"
}
func (m *mockEventBus) Emit(ctx context.Context, event string, data interface{}) {
	m.emitCalled = true
	m.emitTopic = event
	m.emitData = data
}
func (m *mockEventBus) DispatchFilter(ctx context.Context, event string, data interface{}) (interface{}, error) {
	return data, nil
}
func (m *mockEventBus) Unsubscribe(handlerID string) error { return nil }

type mockWasmPlugin struct {
	name         string
	version      string
	wasmBytes    []byte
	wasmBytesErr error
}

func (m *mockWasmPlugin) Name() string                    { return m.name }
func (m *mockWasmPlugin) Version() string                 { return m.version }
func (m *mockWasmPlugin) Manifest() *core.Manifest        { return nil }
func (m *mockWasmPlugin) Init(ctx core.CoreContext) error { return nil }
func (m *mockWasmPlugin) Start() error                    { return nil }
func (m *mockWasmPlugin) Stop() error                     { return nil }
func (m *mockWasmPlugin) WasmBytes() ([]byte, error) {
	return m.wasmBytes, m.wasmBytesErr
}

func TestNewHostModuleBuilder(t *testing.T) {
	t.Run("with valid container and events", func(t *testing.T) {
		builder := NewHostModuleBuilder(&mockServiceContainer{keys: []string{"s1"}}, &mockEventBus{})
		if builder.container == nil || builder.events == nil {
			t.Error("Should not be nil")
		}
	})

	t.Run("with nil container", func(t *testing.T) {
		builder := NewHostModuleBuilder(nil, &mockEventBus{})
		if builder.container != nil {
			t.Error("Should be nil")
		}
	})

	t.Run("with nil events", func(t *testing.T) {
		builder := NewHostModuleBuilder(&mockServiceContainer{}, nil)
		if builder.events != nil {
			t.Error("Should be nil")
		}
	})
}

func TestHostModuleBuilder_refreshServiceRegistry(t *testing.T) {
	t.Run("with valid container", func(t *testing.T) {
		container := &mockServiceContainer{keys: []string{"s1", "s2"}}
		builder := NewHostModuleBuilder(container, &mockEventBus{})
		builder.refreshServiceRegistry()

		builder.registryMu.RLock()
		registry := builder.serviceRegistry
		builder.registryMu.RUnlock()

		if len(registry) != 2 {
			t.Errorf("Expected 2, got %d", len(registry))
		}
	})

	t.Run("with nil container", func(t *testing.T) {
		builder := NewHostModuleBuilder(nil, &mockEventBus{})
		builder.refreshServiceRegistry()

		builder.registryMu.RLock()
		if len(builder.serviceRegistry) > 0 {
			t.Error("Should be empty")
		}
		builder.registryMu.RUnlock()
	})
}

func TestHostModuleBuilder_BuildCMSHostModule(t *testing.T) {
	ctx := context.Background()
	runtime := wazero.NewRuntime(ctx)

	t.Run("successful build", func(t *testing.T) {
		builder := NewHostModuleBuilder(&mockServiceContainer{keys: []string{"s1"}}, &mockEventBus{})
		module, err := builder.BuildCMSHostModule(ctx, runtime)
		if err != nil || module == nil {
			t.Errorf("Failed: %v", err)
		}
		if builder.module == nil {
			t.Error("Module should be set")
		}
		module.Close(ctx)
	})

	t.Run("with nil container", func(t *testing.T) {
		builder := NewHostModuleBuilder(nil, &mockEventBus{})
		module, err := builder.BuildCMSHostModule(ctx, runtime)
		if err != nil {
			t.Errorf("Should succeed: %v", err)
		}
		if module != nil {
			module.Close(ctx)
		}
	})

	t.Run("with nil events", func(t *testing.T) {
		builder := NewHostModuleBuilder(&mockServiceContainer{}, nil)
		module, err := builder.BuildCMSHostModule(ctx, runtime)
		if err != nil {
			t.Errorf("Should succeed: %v", err)
		}
		if module != nil {
			module.Close(ctx)
		}
	})

	runtime.Close(ctx)
}

func TestHostModuleBuilder_ServiceRegistry(t *testing.T) {
	t.Run("nil container has empty registry", func(t *testing.T) {
		builder := NewHostModuleBuilder(nil, &mockEventBus{})
		builder.refreshServiceRegistry()

		builder.registryMu.RLock()
		if len(builder.serviceRegistry) != 0 {
			t.Error("Should be empty")
		}
		builder.registryMu.RUnlock()
	})

	t.Run("valid container populates registry", func(t *testing.T) {
		container := &mockServiceContainer{keys: []string{"svc1", "svc2"}}
		builder := NewHostModuleBuilder(container, &mockEventBus{})
		builder.refreshServiceRegistry()

		builder.registryMu.RLock()
		if len(builder.serviceRegistry) != 2 {
			t.Errorf("Expected 2, got %d", len(builder.serviceRegistry))
		}
		builder.registryMu.RUnlock()
	})

	t.Run("empty container has empty registry", func(t *testing.T) {
		container := &mockServiceContainer{keys: []string{}}
		builder := NewHostModuleBuilder(container, &mockEventBus{})
		builder.refreshServiceRegistry()

		builder.registryMu.RLock()
		if len(builder.serviceRegistry) != 0 {
			t.Error("Should be empty")
		}
		builder.registryMu.RUnlock()
	})
}

func TestWasmEngine_ExecuteLifecycle(t *testing.T) {
	ctx := context.Background()
	coreCtx := core.NewCoreContext(ctx, nil, nil, nil, nil, "/data", "/plugins")

	t.Run("not initialized", func(t *testing.T) {
		engine := NewWasmEngine()
		plugin := &mockWasmPlugin{name: "test", wasmBytes: []byte{}}

		err := engine.ExecuteLifecycle(ctx, plugin, coreCtx)
		if err == nil {
			t.Error("Expected error")
		}
		if err.Error() != "wasm engine not initialized" {
			t.Errorf("Wrong error: %v", err)
		}
	})

	t.Run("non-WasmPlugin", func(t *testing.T) {
		engine := NewWasmEngine()
		_ = engine.Initialize(ctx)

		err := engine.ExecuteLifecycle(ctx, &mockPlugin{name: "test"}, coreCtx)
		if err == nil {
			t.Error("Expected error")
		}
		engine.Close()
	})

	t.Run("WasmBytes error", func(t *testing.T) {
		engine := NewWasmEngine()
		_ = engine.Initialize(ctx)

		plugin := &mockWasmPlugin{name: "test", wasmBytesErr: errors.New("failed")}
		err := engine.ExecuteLifecycle(ctx, plugin, coreCtx)
		if err == nil {
			t.Error("Expected error")
		}
		engine.Close()
	})

	t.Run("invalid wasm bytes", func(t *testing.T) {
		engine := NewWasmEngine()
		_ = engine.Initialize(ctx)

		plugin := &mockWasmPlugin{name: "test", wasmBytes: []byte{0x00}}
		err := engine.ExecuteLifecycle(ctx, plugin, coreCtx)
		if err == nil {
			t.Error("Expected error")
		}
		engine.Close()
	})

	t.Run("valid wasm module", func(t *testing.T) {
		engine := NewWasmEngine()
		_ = engine.Initialize(ctx)

		plugin := &mockWasmPlugin{name: "test", wasmBytes: minimalWasmModule()}
		err := engine.ExecuteLifecycle(ctx, plugin, coreCtx)
		if err != nil {
			t.Errorf("Failed: %v", err)
		}
		engine.Close()
	})

	t.Run("wasm with init", func(t *testing.T) {
		engine := NewWasmEngine()
		_ = engine.Initialize(ctx)

		plugin := &mockWasmPlugin{name: "test", wasmBytes: wasmModuleWithInit()}
		err := engine.ExecuteLifecycle(ctx, plugin, coreCtx)
		if err != nil {
			t.Errorf("Failed: %v", err)
		}
		engine.Close()
	})

	t.Run("wasm with start", func(t *testing.T) {
		engine := NewWasmEngine()
		_ = engine.Initialize(ctx)

		plugin := &mockWasmPlugin{name: "test", wasmBytes: wasmModuleWithStart()}
		err := engine.ExecuteLifecycle(ctx, plugin, coreCtx)
		if err != nil {
			t.Errorf("Failed: %v", err)
		}
		engine.Close()
	})
}

func TestWasmEngine_ExecuteLifecycle_WithTimeout(t *testing.T) {
	ctx := context.Background()
	coreCtx := core.NewCoreContext(ctx, nil, nil, nil, nil, "/data", "/plugins")

	config := WasmConfig{
		MaxMemoryPages:   512,
		ExecutionTimeout: 100 * time.Millisecond,
	}
	engine := NewWasmEngineWithConfig(config)
	_ = engine.Initialize(ctx)

	plugin := &mockWasmPlugin{name: "test", wasmBytes: minimalWasmModule()}
	err := engine.ExecuteLifecycle(ctx, plugin, coreCtx)
	if err != nil {
		t.Errorf("Failed: %v", err)
	}
	engine.Close()
}

func TestWasmPlugin_Interface(t *testing.T) {
	var _ WasmPlugin = &mockWasmPlugin{}
	var _ core.Plugin = &mockWasmPlugin{}
}

func TestWasmPlugin_Methods(t *testing.T) {
	plugin := &mockWasmPlugin{name: "test-plugin", version: "1.0.0", wasmBytes: []byte{0x00}}

	if plugin.Name() != "test-plugin" {
		t.Errorf("Wrong: %s", plugin.Name())
	}
	if plugin.Version() != "1.0.0" {
		t.Errorf("Wrong: %s", plugin.Version())
	}

	bytes, err := plugin.WasmBytes()
	if err != nil || len(bytes) != 1 {
		t.Errorf("Wrong: %v, %d", err, len(bytes))
	}

	pluginErr := &mockWasmPlugin{wasmBytesErr: errors.New("err")}
	_, err = pluginErr.WasmBytes()
	if err == nil {
		t.Error("Expected error")
	}
}

func TestHostModuleBuilder_ConcurrentAccess(t *testing.T) {
	container := &mockServiceContainer{keys: []string{"s1", "s2"}}
	builder := NewHostModuleBuilder(container, &mockEventBus{})
	done := make(chan bool)

	for range 10 {
		go func() {
			builder.refreshServiceRegistry()
			builder.registryMu.RLock()
			_ = builder.serviceRegistry
			builder.registryMu.RUnlock()
			done <- true
		}()
	}

	for range 10 {
		<-done
	}
}

func TestSafeReadBytes(t *testing.T) {
	ctx := context.Background()
	runtime := wazero.NewRuntime(ctx)

	wasmBytes := minimalWasmModule()
	module, err := runtime.Instantiate(ctx, wasmBytes)
	if err != nil {
		runtime.Close(ctx)
		t.Fatalf("Failed: %v", err)
	}
	defer module.Close(ctx)

	mem := module.Memory()
	if mem == nil {
		runtime.Close(ctx)
		t.Fatal("Memory nil")
	}

	mem.Write(0, []byte("testdata"))

	t.Run("successful read", func(t *testing.T) {
		buf, ok := safeReadBytes(module, 0, 8)
		if !ok || string(buf) != "testdata" {
			t.Errorf("Wrong: %s, %v", string(buf), ok)
		}
	})

	t.Run("zero length", func(t *testing.T) {
		buf, ok := safeReadBytes(module, 0, 0)
		if !ok || len(buf) != 0 {
			t.Error("Zero should succeed with empty buffer")
		}
	})

	t.Run("read beyond memory", func(t *testing.T) {
		buf, ok := safeReadBytes(module, 65535, 1000)
		if ok || buf != nil {
			t.Error("Should fail with nil buffer")
		}
	})

	runtime.Close(ctx)
}

func minimalWasmModule() []byte {
	return []byte{0x00, 0x61, 0x73, 0x6D, 0x01, 0x00, 0x00, 0x00, 0x05, 0x03, 0x01, 0x00, 0x01}
}

func wasmModuleWithInit() []byte {
	return []byte{
		0x00, 0x61, 0x73, 0x6D, 0x01, 0x00, 0x00, 0x00,
		0x01, 0x04, 0x01, 0x60, 0x00, 0x00,
		0x03, 0x02, 0x01, 0x00,
		0x05, 0x03, 0x01, 0x00, 0x01,
		0x07, 0x08, 0x01, 0x04, 'i', 'n', 'i', 't', 0x00, 0x00,
		0x0A, 0x04, 0x01, 0x02, 0x00, 0x0B,
	}
}

func wasmModuleWithStart() []byte {
	return []byte{
		0x00, 0x61, 0x73, 0x6D, 0x01, 0x00, 0x00, 0x00,
		0x01, 0x04, 0x01, 0x60, 0x00, 0x00,
		0x03, 0x02, 0x01, 0x00,
		0x05, 0x03, 0x01, 0x00, 0x01,
		0x07, 0x09, 0x01, 0x05, 's', 't', 'a', 'r', 't', 0x00, 0x00,
		0x0A, 0x04, 0x01, 0x02, 0x00, 0x0B,
	}
}
