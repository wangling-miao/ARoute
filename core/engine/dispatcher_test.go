package engine

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/wangling-miao/aroute/core"
)

type mockPlugin struct {
	name     string
	version  string
	initErr  error
	startErr error
}

func (m *mockPlugin) Name() string                    { return m.name }
func (m *mockPlugin) Version() string                 { return m.version }
func (m *mockPlugin) Manifest() *core.Manifest        { return nil }
func (m *mockPlugin) Init(ctx core.CoreContext) error { return m.initErr }
func (m *mockPlugin) Start() error                    { return m.startErr }
func (m *mockPlugin) Stop() error                     { return nil }

func TestNativeEngine_Type(t *testing.T) {
	engine := NewNativeEngine()
	if engine.Type() != core.EngineL1Native {
		t.Errorf("Expected EngineL1Native, got %v", engine.Type())
	}
}

func TestNativeEngine_Initialize(t *testing.T) {
	engine := NewNativeEngine()
	ctx := context.Background()

	if err := engine.Initialize(ctx); err != nil {
		t.Errorf("Initialize failed: %v", err)
	}

	if !engine.initialized {
		t.Error("Engine should be initialized")
	}

	if err := engine.Initialize(ctx); err != nil {
		t.Errorf("Second Initialize should be idempotent: %v", err)
	}
}

func TestNativeEngine_ExecuteLifecycle(t *testing.T) {
	ctx := context.Background()
	coreCtx := core.NewCoreContext(ctx, nil, nil, nil, nil, "/data", "/plugins")

	t.Run("successful lifecycle", func(t *testing.T) {
		engine := NewNativeEngine()
		_ = engine.Initialize(ctx)

		plugin := &mockPlugin{
			name:    "test-plugin",
			version: "1.0.0",
		}

		err := engine.ExecuteLifecycle(ctx, plugin, coreCtx)
		if err != nil {
			t.Errorf("ExecuteLifecycle failed: %v", err)
		}
	})

	t.Run("init failure", func(t *testing.T) {
		engine := NewNativeEngine()
		_ = engine.Initialize(ctx)

		plugin := &mockPlugin{
			name:    "test-plugin",
			version: "1.0.0",
			initErr: errors.New("init failed"),
		}

		err := engine.ExecuteLifecycle(ctx, plugin, coreCtx)
		if err == nil {
			t.Error("Expected error on init failure")
		}
	})

	t.Run("start failure", func(t *testing.T) {
		engine := NewNativeEngine()
		_ = engine.Initialize(ctx)

		plugin := &mockPlugin{
			name:     "test-plugin",
			version:  "1.0.0",
			startErr: errors.New("start failed"),
		}

		err := engine.ExecuteLifecycle(ctx, plugin, coreCtx)
		if err == nil {
			t.Error("Expected error on start failure")
		}
	})

	t.Run("not initialized", func(t *testing.T) {
		engine := NewNativeEngine()

		plugin := &mockPlugin{
			name:    "test-plugin",
			version: "1.0.0",
		}

		err := engine.ExecuteLifecycle(ctx, plugin, coreCtx)
		if err == nil {
			t.Error("Expected error when engine not initialized")
		}
	})

	t.Run("nil plugin", func(t *testing.T) {
		engine := NewNativeEngine()
		_ = engine.Initialize(ctx)

		err := engine.ExecuteLifecycle(ctx, nil, coreCtx)
		if err == nil {
			t.Error("Expected error for nil plugin")
		}
	})

	t.Run("nil core context", func(t *testing.T) {
		engine := NewNativeEngine()
		_ = engine.Initialize(ctx)

		plugin := &mockPlugin{
			name:    "test-plugin",
			version: "1.0.0",
		}

		err := engine.ExecuteLifecycle(ctx, plugin, nil)
		if err == nil {
			t.Error("Expected error for nil core context")
		}
	})
}

func TestNativeEngine_Close(t *testing.T) {
	engine := NewNativeEngine()
	ctx := context.Background()

	_ = engine.Initialize(ctx)

	if err := engine.Close(); err != nil {
		t.Errorf("Close failed: %v", err)
	}

	if engine.initialized {
		t.Error("Engine should not be initialized after close")
	}

	if err := engine.Close(); err != nil {
		t.Errorf("Second Close should be idempotent: %v", err)
	}
}

func TestNativeEngine_PanicRecovery(t *testing.T) {
	ctx := context.Background()
	coreCtx := core.NewCoreContext(ctx, nil, nil, nil, nil, "/data", "/plugins")

	t.Run("panic in Init is recovered", func(t *testing.T) {
		engine := NewNativeEngine()
		_ = engine.Initialize(ctx)

		plugin := &panicPlugin{panicAt: "init"}

		err := engine.ExecuteLifecycle(ctx, plugin, coreCtx)
		if err == nil {
			t.Error("Expected error from panic recovery")
		}
	})

	t.Run("panic in Start is recovered", func(t *testing.T) {
		engine := NewNativeEngine()
		_ = engine.Initialize(ctx)

		plugin := &panicPlugin{panicAt: "start"}

		err := engine.ExecuteLifecycle(ctx, plugin, coreCtx)
		if err == nil {
			t.Error("Expected error from panic recovery")
		}
	})
}

type panicPlugin struct {
	panicAt string
}

func (p *panicPlugin) Name() string             { return "panic-plugin" }
func (p *panicPlugin) Version() string          { return "1.0.0" }
func (p *panicPlugin) Manifest() *core.Manifest { return nil }
func (p *panicPlugin) Init(ctx core.CoreContext) error {
	if p.panicAt == "init" {
		panic("init panic")
	}
	return nil
}
func (p *panicPlugin) Start() error {
	if p.panicAt == "start" {
		panic("start panic")
	}
	return nil
}
func (p *panicPlugin) Stop() error { return nil }

func TestDispatcher_RegisterEngine(t *testing.T) {
	dispatcher := NewDispatcher()

	t.Run("register native engine", func(t *testing.T) {
		nativeEngine := NewNativeEngine()
		err := dispatcher.RegisterEngine(core.EngineL1Native, nativeEngine)
		if err != nil {
			t.Errorf("RegisterEngine failed: %v", err)
		}
	})

	t.Run("register duplicate engine", func(t *testing.T) {
		nativeEngine1 := NewNativeEngine()
		nativeEngine2 := NewNativeEngine()

		_ = dispatcher.RegisterEngine(core.EngineL1Native, nativeEngine1)
		err := dispatcher.RegisterEngine(core.EngineL1Native, nativeEngine2)
		if err == nil {
			t.Error("Expected error for duplicate engine registration")
		}
	})

	t.Run("register nil engine", func(t *testing.T) {
		err := dispatcher.RegisterEngine(core.EngineL1Native, nil)
		if err == nil {
			t.Error("Expected error for nil engine")
		}
	})

	t.Run("register wasm engine", func(t *testing.T) {
		wasmEngine := NewWasmEngine()
		err := dispatcher.RegisterEngine(core.EngineL3Wasm, wasmEngine)
		if err != nil {
			t.Errorf("RegisterEngine failed: %v", err)
		}
	})
}

func TestDispatcher_GetEngine(t *testing.T) {
	dispatcher := NewDispatcher()

	t.Run("get registered engine", func(t *testing.T) {
		nativeEngine := NewNativeEngine()
		_ = dispatcher.RegisterEngine(core.EngineL1Native, nativeEngine)

		engine, err := dispatcher.GetEngine(core.EngineL1Native)
		if err != nil {
			t.Errorf("GetEngine failed: %v", err)
		}

		if engine.Type() != core.EngineL1Native {
			t.Errorf("Expected EngineL1Native, got %v", engine.Type())
		}
	})

	t.Run("get unregistered engine", func(t *testing.T) {
		_, err := dispatcher.GetEngine(core.EngineL3Wasm)
		if err == nil {
			t.Error("Expected error for unregistered engine")
		}
	})
}

func TestDispatcher_Execute(t *testing.T) {
	dispatcher := NewDispatcher()
	ctx := context.Background()
	coreCtx := core.NewCoreContext(ctx, nil, nil, nil, nil, "/data", "/plugins")

	nativeEngine := NewNativeEngine()
	_ = nativeEngine.Initialize(ctx)
	_ = dispatcher.RegisterEngine(core.EngineL1Native, nativeEngine)

	t.Run("execute with valid manifest", func(t *testing.T) {
		plugin := &mockPlugin{
			name:    "test-plugin",
			version: "1.0.0",
		}

		manifest := &core.Manifest{
			Name:    "test-plugin",
			Version: "1.0.0",
			Engine:  "native",
		}

		err := dispatcher.Execute(ctx, plugin, manifest, coreCtx)
		if err != nil {
			t.Errorf("Execute failed: %v", err)
		}
	})

	t.Run("execute with invalid engine", func(t *testing.T) {
		plugin := &mockPlugin{
			name:    "test-plugin",
			version: "1.0.0",
		}

		manifest := &core.Manifest{
			Name:    "test-plugin",
			Version: "1.0.0",
			Engine:  "invalid",
		}

		err := dispatcher.Execute(ctx, plugin, manifest, coreCtx)
		if err == nil {
			t.Error("Expected error for invalid engine")
		}
	})

	t.Run("execute with unregistered engine", func(t *testing.T) {
		plugin := &mockPlugin{
			name:    "test-plugin",
			version: "1.0.0",
		}

		manifest := &core.Manifest{
			Name:    "test-plugin",
			Version: "1.0.0",
			Engine:  "wasm",
		}

		err := dispatcher.Execute(ctx, plugin, manifest, coreCtx)
		if err == nil {
			t.Error("Expected error for unregistered engine")
		}
	})
}

func TestDispatcher_Close(t *testing.T) {
	dispatcher := NewDispatcher()
	ctx := context.Background()

	nativeEngine := NewNativeEngine()
	_ = nativeEngine.Initialize(ctx)
	_ = dispatcher.RegisterEngine(core.EngineL1Native, nativeEngine)

	err := dispatcher.Close()
	if err != nil {
		t.Errorf("Close failed: %v", err)
	}
}

func TestWasmEngine_Type(t *testing.T) {
	engine := NewWasmEngine()
	if engine.Type() != core.EngineL3Wasm {
		t.Errorf("Expected EngineL3Wasm, got %v", engine.Type())
	}
}

func TestWasmEngine_Initialize(t *testing.T) {
	engine := NewWasmEngine()
	ctx := context.Background()

	if err := engine.Initialize(ctx); err != nil {
		t.Errorf("Initialize failed: %v", err)
	}

	if !engine.initialized {
		t.Error("Engine should be initialized")
	}

	if err := engine.Initialize(ctx); err != nil {
		t.Errorf("Second Initialize should be idempotent: %v", err)
	}
}

func TestWasmEngine_Initialize_WithConfig(t *testing.T) {
	config := WasmConfig{
		MaxMemoryPages:   256,
		ExecutionTimeout: 10 * time.Second,
	}
	engine := NewWasmEngineWithConfig(config)
	ctx := context.Background()

	if err := engine.Initialize(ctx); err != nil {
		t.Errorf("Initialize failed: %v", err)
	}

	if engine.config.MaxMemoryPages != 256 {
		t.Errorf("Expected MaxMemoryPages 256, got %d", engine.config.MaxMemoryPages)
	}

	if engine.config.ExecutionTimeout != 10*time.Second {
		t.Errorf("Expected ExecutionTimeout 10s, got %v", engine.config.ExecutionTimeout)
	}
}

func TestDefaultWasmConfig(t *testing.T) {
	config := DefaultWasmConfig()

	if config.MaxMemoryPages != 512 {
		t.Errorf("Expected default MaxMemoryPages 512, got %d", config.MaxMemoryPages)
	}

	if config.ExecutionTimeout != 30*time.Second {
		t.Errorf("Expected default ExecutionTimeout 30s, got %v", config.ExecutionTimeout)
	}
}

func TestWasmEngine_Close(t *testing.T) {
	engine := NewWasmEngine()
	ctx := context.Background()

	_ = engine.Initialize(ctx)

	if err := engine.Close(); err != nil {
		t.Errorf("Close failed: %v", err)
	}

	if engine.initialized {
		t.Error("Engine should not be initialized after close")
	}

	if err := engine.Close(); err != nil {
		t.Errorf("Second Close should be idempotent: %v", err)
	}
}

func TestWasmEngine_ExecuteLifecycle_NonWasmPlugin(t *testing.T) {
	engine := NewWasmEngine()
	ctx := context.Background()
	coreCtx := core.NewCoreContext(ctx, nil, nil, nil, nil, "/data", "/plugins")

	_ = engine.Initialize(ctx)

	plugin := &mockPlugin{
		name:    "test-plugin",
		version: "1.0.0",
	}

	err := engine.ExecuteLifecycle(ctx, plugin, coreCtx)
	if err == nil {
		t.Error("Expected error for non-WasmPlugin")
	}
}

func TestEngineType_String(t *testing.T) {
	tests := []struct {
		engineType core.EngineType
		expected   string
	}{
		{core.EngineL1Native, "native"},
		{core.EngineL3Wasm, "wasm"},
		{core.EngineType(999), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.engineType.String(); got != tt.expected {
				t.Errorf("EngineType.String() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestParseEngine(t *testing.T) {
	tests := []struct {
		input    string
		expected core.EngineType
		hasError bool
	}{
		{"native", core.EngineL1Native, false},
		{"l1", core.EngineL1Native, false},
		{"wasm", core.EngineL3Wasm, false},
		{"l3", core.EngineL3Wasm, false},
		{"invalid", core.EngineType(0), true},
		{"", core.EngineType(0), true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, err := core.ParseEngine(tt.input)
			if tt.hasError {
				if err == nil {
					t.Error("Expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if result != tt.expected {
					t.Errorf("ParseEngine(%v) = %v, want %v", tt.input, result, tt.expected)
				}
			}
		})
	}
}

func TestDispatcherError_Error(t *testing.T) {
	t.Run("with Plugin", func(t *testing.T) {
		err := &DispatcherError{
			Op:     "execute",
			Plugin: "my-plugin",
			Err:    errors.New("some error"),
		}
		msg := err.Error()
		if msg != "dispatcher execute error for plugin my-plugin: some error" {
			t.Errorf("unexpected message: %s", msg)
		}
	})

	t.Run("with Engine only", func(t *testing.T) {
		err := &DispatcherError{
			Op:     "register",
			Engine: "wasm",
			Err:    errors.New("already registered"),
		}
		msg := err.Error()
		if msg != "dispatcher register error for engine wasm: already registered" {
			t.Errorf("unexpected message: %s", msg)
		}
	})

	t.Run("with neither Plugin nor Engine", func(t *testing.T) {
		err := &DispatcherError{
			Op:  "close",
			Err: errors.New("failed"),
		}
		msg := err.Error()
		if msg != "dispatcher close error: failed" {
			t.Errorf("unexpected message: %s", msg)
		}
	})
}

func TestDispatcherError_Unwrap(t *testing.T) {
	inner := errors.New("inner error")
	err := &DispatcherError{
		Op:  "execute",
		Err: inner,
	}

	if unwrapped := err.Unwrap(); unwrapped != inner {
		t.Errorf("Unwrap() = %v, want %v", unwrapped, inner)
	}
}

type typeMismatchEngine struct{}

func (e *typeMismatchEngine) Type() core.EngineType              { return core.EngineL3Wasm }
func (e *typeMismatchEngine) Initialize(_ context.Context) error { return nil }
func (e *typeMismatchEngine) ExecuteLifecycle(_ context.Context, _ core.Plugin, _ core.CoreContext) error {
	return nil
}
func (e *typeMismatchEngine) Close() error { return nil }

func TestDispatcher_Execute_TypeMismatch(t *testing.T) {
	dispatcher := NewDispatcher()
	ctx := context.Background()
	coreCtx := core.NewCoreContext(ctx, nil, nil, nil, nil, "/data", "/plugins")

	_ = dispatcher.RegisterEngine(core.EngineL1Native, &typeMismatchEngine{})

	plugin := &mockPlugin{name: "test-plugin", version: "1.0.0"}
	manifest := &core.Manifest{
		Name:    "test-plugin",
		Version: "1.0.0",
		Engine:  "native",
	}

	err := dispatcher.Execute(ctx, plugin, manifest, coreCtx)
	if err == nil {
		t.Error("Expected error for type mismatch")
	}
}

type closeErrorEngine struct{}

func (e *closeErrorEngine) Type() core.EngineType              { return core.EngineL1Native }
func (e *closeErrorEngine) Initialize(_ context.Context) error { return nil }
func (e *closeErrorEngine) ExecuteLifecycle(_ context.Context, _ core.Plugin, _ core.CoreContext) error {
	return nil
}
func (e *closeErrorEngine) Close() error { return errors.New("close failed") }

func TestDispatcher_CloseWithEngineError(t *testing.T) {
	dispatcher := NewDispatcher()
	_ = dispatcher.RegisterEngine(core.EngineL1Native, &closeErrorEngine{})

	err := dispatcher.Close()
	if err == nil {
		t.Error("Expected error when engine Close fails")
	}
}
