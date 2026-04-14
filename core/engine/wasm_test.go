package engine

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/wangling-miao/aroute/core"
	"github.com/wangling-miao/aroute/core/events"
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
	emitEvent  events.Event
}

func (m *mockEventBus) SubscribeFilter(topic string, priority int, handler events.FilterHandler) string {
	return "handler-1"
}
func (m *mockEventBus) SubscribeBroadcast(topic string, handler events.BroadcastHandler) string {
	return "handler-1"
}
func (m *mockEventBus) Emit(ctx context.Context, event events.Event) {
	m.emitCalled = true
	m.emitEvent = event
}
func (m *mockEventBus) DispatchFilter(ctx context.Context, event *events.Event) (*events.Event, error) {
	return event, nil
}
func (m *mockEventBus) Unsubscribe(handlerID string) {}

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

func setupHostModuleTest(t *testing.T, container core.ServiceContainer, events core.EventBus) (*HostModuleBuilder, wazero.Runtime, api.Module, context.Context) {
	t.Helper()
	ctx := context.Background()
	runtime := wazero.NewRuntime(ctx)

	builder := NewHostModuleBuilder(container, events)
	_, err := builder.BuildCMSHostModule(ctx, runtime)
	if err != nil {
		runtime.Close(ctx)
		t.Fatalf("BuildCMSHostModule failed: %v", err)
	}

	mod, err := runtime.Instantiate(ctx, minimalWasmModule())
	if err != nil {
		runtime.Close(ctx)
		t.Fatalf("Instantiate failed: %v", err)
	}

	t.Cleanup(func() {
		runtime.Close(ctx)
	})

	return builder, runtime, mod, ctx
}

func TestHostModuleBuilder_ServiceGet(t *testing.T) {
	t.Run("valid serviceID returns offset", func(t *testing.T) {
		container := &mockServiceContainer{keys: []string{"service-a", "service-b"}}
		builder, _, mod, ctx := setupHostModuleTest(t, container, &mockEventBus{})

		result := builder.serviceGet(ctx, mod, 0)
		if result != 1 {
			t.Errorf("serviceGet(0) = %d, want 1", result)
		}

		result = builder.serviceGet(ctx, mod, 1)
		if result != 2 {
			t.Errorf("serviceGet(1) = %d, want 2", result)
		}
	})

	t.Run("out-of-range serviceID returns 0", func(t *testing.T) {
		container := &mockServiceContainer{keys: []string{"service-a"}}
		builder, _, mod, ctx := setupHostModuleTest(t, container, &mockEventBus{})

		result := builder.serviceGet(ctx, mod, 5)
		if result != 0 {
			t.Errorf("serviceGet(5) = %d, want 0", result)
		}
	})

	t.Run("nil container returns 0", func(t *testing.T) {
		builder, _, mod, ctx := setupHostModuleTest(t, nil, &mockEventBus{})

		result := builder.serviceGet(ctx, mod, 0)
		if result != 0 {
			t.Errorf("serviceGet with nil container = %d, want 0", result)
		}
	})
}

func TestHostModuleBuilder_ServiceHas(t *testing.T) {
	t.Run("valid serviceID returns 1", func(t *testing.T) {
		container := &mockServiceContainer{keys: []string{"service-a", "service-b"}}
		builder, _, mod, ctx := setupHostModuleTest(t, container, &mockEventBus{})

		result := builder.serviceHas(ctx, mod, 0)
		if result != 1 {
			t.Errorf("serviceHas(0) = %d, want 1", result)
		}
	})

	t.Run("out-of-range serviceID returns 0", func(t *testing.T) {
		container := &mockServiceContainer{keys: []string{"service-a"}}
		builder, _, mod, ctx := setupHostModuleTest(t, container, &mockEventBus{})

		result := builder.serviceHas(ctx, mod, 99)
		if result != 0 {
			t.Errorf("serviceHas(99) = %d, want 0", result)
		}
	})

	t.Run("nil container returns 0", func(t *testing.T) {
		builder, _, mod, ctx := setupHostModuleTest(t, nil, &mockEventBus{})

		result := builder.serviceHas(ctx, mod, 0)
		if result != 0 {
			t.Errorf("serviceHas with nil container = %d, want 0", result)
		}
	})
}

func TestHostModuleBuilder_EventSubscribe(t *testing.T) {
	t.Run("valid topic bytes", func(t *testing.T) {
		container := &mockServiceContainer{keys: []string{}}
		events := &mockEventBus{}
		builder, _, mod, ctx := setupHostModuleTest(t, container, events)

		mem := mod.Memory()
		mem.Write(0, []byte("test-topic"))

		result := builder.eventSubscribe(ctx, mod, 0, 10)
		if result != 0 {
			t.Errorf("eventSubscribe = %d, want 0", result)
		}
	})

	t.Run("nil events returns 0", func(t *testing.T) {
		container := &mockServiceContainer{keys: []string{}}
		builder, _, mod, ctx := setupHostModuleTest(t, container, nil)

		result := builder.eventSubscribe(ctx, mod, 0, 5)
		if result != 0 {
			t.Errorf("eventSubscribe with nil events = %d, want 0", result)
		}
	})

	t.Run("failed read returns 0", func(t *testing.T) {
		container := &mockServiceContainer{keys: []string{}}
		events := &mockEventBus{}
		builder, _, mod, ctx := setupHostModuleTest(t, container, events)

		result := builder.eventSubscribe(ctx, mod, 65535, 1000)
		if result != 0 {
			t.Errorf("eventSubscribe with bad read = %d, want 0", result)
		}
	})
}

func TestHostModuleBuilder_EventPublish(t *testing.T) {
	t.Run("valid data emits event", func(t *testing.T) {
		container := &mockServiceContainer{keys: []string{}}
		events := &mockEventBus{}
		builder, _, mod, ctx := setupHostModuleTest(t, container, events)

		mem := mod.Memory()
		mem.Write(0, []byte("my-topic"))
		mem.Write(20, []byte("event-data"))

		builder.eventPublish(ctx, mod, 0, 8, 20, 10)

		if !events.emitCalled {
			t.Error("expected Emit to be called")
		}
		if events.emitEvent.Topic != "my-topic" {
			t.Errorf("emitEvent.Topic = %q, want %q", events.emitEvent.Topic, "my-topic")
		}
	})

	t.Run("nil events does nothing", func(t *testing.T) {
		container := &mockServiceContainer{keys: []string{}}
		builder, _, mod, ctx := setupHostModuleTest(t, container, nil)

		builder.eventPublish(ctx, mod, 0, 5, 10, 5)
	})

	t.Run("failed topic read does nothing", func(t *testing.T) {
		container := &mockServiceContainer{keys: []string{}}
		events := &mockEventBus{}
		builder, _, mod, ctx := setupHostModuleTest(t, container, events)

		builder.eventPublish(ctx, mod, 65535, 1000, 0, 1)

		if events.emitCalled {
			t.Error("should not emit on bad topic read")
		}
	})

	t.Run("failed data read does nothing", func(t *testing.T) {
		container := &mockServiceContainer{keys: []string{}}
		events := &mockEventBus{}
		builder, _, mod, ctx := setupHostModuleTest(t, container, events)

		mem := mod.Memory()
		mem.Write(0, []byte("topic"))

		builder.eventPublish(ctx, mod, 0, 5, 65535, 1000)

		if events.emitCalled {
			t.Error("should not emit on bad data read")
		}
	})
}

func TestHostModuleBuilder_MemoryAlloc(t *testing.T) {
	t.Run("no allocate_memory export returns 0", func(t *testing.T) {
		container := &mockServiceContainer{keys: []string{}}
		builder, _, mod, ctx := setupHostModuleTest(t, container, &mockEventBus{})

		result := builder.memoryAlloc(ctx, mod, 64)
		if result != 0 {
			t.Errorf("memoryAlloc without export = %d, want 0", result)
		}
	})

	t.Run("success with allocate_memory export", func(t *testing.T) {
		ctx := context.Background()
		runtime := wazero.NewRuntime(ctx)
		defer runtime.Close(ctx)

		container := &mockServiceContainer{keys: []string{}}
		builder := NewHostModuleBuilder(container, &mockEventBus{})
		_, err := builder.BuildCMSHostModule(ctx, runtime)
		if err != nil {
			t.Fatalf("BuildCMSHostModule failed: %v", err)
		}

		mod, err := runtime.Instantiate(ctx, wasmModuleWithAllocator())
		if err != nil {
			t.Fatalf("Instantiate failed: %v", err)
		}
		defer mod.Close(ctx)

		result := builder.memoryAlloc(ctx, mod, 128)
		if result == 0 {
			t.Error("memoryAlloc should return non-zero with allocator export")
		}
	})
}

func TestHostModuleBuilder_MemoryFree(t *testing.T) {
	t.Run("no free_memory export does nothing", func(t *testing.T) {
		container := &mockServiceContainer{keys: []string{}}
		builder, _, mod, ctx := setupHostModuleTest(t, container, &mockEventBus{})

		builder.memoryFree(ctx, mod, 100)
	})

	t.Run("with free_memory export calls it", func(t *testing.T) {
		ctx := context.Background()
		runtime := wazero.NewRuntime(ctx)
		defer runtime.Close(ctx)

		container := &mockServiceContainer{keys: []string{}}
		builder := NewHostModuleBuilder(container, &mockEventBus{})
		_, err := builder.BuildCMSHostModule(ctx, runtime)
		if err != nil {
			t.Fatalf("BuildCMSHostModule failed: %v", err)
		}

		mod, err := runtime.Instantiate(ctx, wasmModuleWithFree())
		if err != nil {
			t.Fatalf("Instantiate failed: %v", err)
		}
		defer mod.Close(ctx)

		builder.memoryFree(ctx, mod, 42)
	})
}

func TestWasmEngine_CloseError(t *testing.T) {
	t.Run("close on uninitialized engine returns nil", func(t *testing.T) {
		engine := NewWasmEngine()
		if err := engine.Close(); err != nil {
			t.Errorf("Close on uninitialized should return nil, got: %v", err)
		}
	})
}

func wasmModuleWithAllocator() []byte {
	return []byte{
		0x00, 0x61, 0x73, 0x6D,
		0x01, 0x00, 0x00, 0x00,
		0x01, 0x06, 0x01, 0x60, 0x01, 0x7F, 0x01, 0x7F,
		0x03, 0x02, 0x01, 0x00,
		0x05, 0x03, 0x01, 0x00, 0x01,
		0x07, 0x13, 0x01,
		0x0F, 'a', 'l', 'l', 'o', 'c', 'a', 't', 'e', '_', 'm', 'e', 'm', 'o', 'r', 'y',
		0x00, 0x00,
		0x0A, 0x06, 0x01,
		0x04, 0x00,
		0x20, 0x00,
		0x0B,
	}
}

func wasmModuleWithFree() []byte {
	return []byte{
		0x00, 0x61, 0x73, 0x6D,
		0x01, 0x00, 0x00, 0x00,
		0x01, 0x05, 0x01, 0x60, 0x01, 0x7F, 0x00,
		0x03, 0x02, 0x01, 0x00,
		0x05, 0x03, 0x01, 0x00, 0x01,
		0x07, 0x0F, 0x01,
		0x0B, 'f', 'r', 'e', 'e', '_', 'm', 'e', 'm', 'o', 'r', 'y',
		0x00, 0x00,
		0x0A, 0x04, 0x01,
		0x02, 0x00,
		0x0B,
	}
}
