// Package engine provides plugin execution engines for Aroute CMS.
package engine

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/wangling-miao/aroute/core"
	"github.com/wangling-miao/aroute/core/events"
)

// WasmConfig holds configuration for the Wasm engine.
type WasmConfig struct {
	// MaxMemoryPages is the maximum memory pages (1 page = 64KB) for Wasm modules.
	// Default is 512 pages (32 MB).
	MaxMemoryPages uint32

	// ExecutionTimeout is the maximum duration for plugin lifecycle execution.
	// Default is 30 seconds.
	ExecutionTimeout time.Duration
}

// DefaultWasmConfig returns the default Wasm configuration.
func DefaultWasmConfig() WasmConfig {
	return WasmConfig{
		MaxMemoryPages:   512, // 32 MB (512 * 64KB)
		ExecutionTimeout: 30 * time.Second,
	}
}

// WasmEngine implements the L3 execution backend for untrusted plugins.
// L3 Wasm plugins run in a sandboxed WebAssembly runtime (wazero) with
// memory limits and restricted host function access.
//
// Security: L3 plugins are isolated - they can only access host functions
// explicitly exposed by the Core (ServiceContainer, EventBus operations).
// Suitable for untrusted third-party plugins.
type WasmEngine struct {
	runtime     wazero.Runtime
	config      WasmConfig
	initialized bool
}

// NewWasmEngine creates a new L3 Wasm engine instance with default configuration.
func NewWasmEngine() *WasmEngine {
	return NewWasmEngineWithConfig(DefaultWasmConfig())
}

// NewWasmEngineWithConfig creates a new L3 Wasm engine instance with custom configuration.
func NewWasmEngineWithConfig(config WasmConfig) *WasmEngine {
	return &WasmEngine{
		config: config,
	}
}

// Type returns EngineL3Wasm.
func (e *WasmEngine) Type() core.EngineType {
	return core.EngineL3Wasm
}

// Initialize creates the wazero runtime with compiler mode for optimal performance.
// Applies configured memory limits and prepares the engine for plugin execution.
func (e *WasmEngine) Initialize(ctx context.Context) error {
	if e.initialized {
		return nil
	}

	runtimeConfig := wazero.NewRuntimeConfig().
		WithMemoryLimitPages(e.config.MaxMemoryPages)

	e.runtime = wazero.NewRuntimeWithConfig(ctx, runtimeConfig)
	e.initialized = true

	return nil
}

// ExecuteLifecycle loads a Wasm plugin and executes its lifecycle.
func (e *WasmEngine) ExecuteLifecycle(ctx context.Context, plugin core.Plugin, coreCtx core.CoreContext) error {
	if !e.initialized {
		return fmt.Errorf("wasm engine not initialized")
	}

	wasmPlugin, ok := plugin.(WasmPlugin)
	if !ok {
		return fmt.Errorf("plugin does not implement WasmPlugin interface for L3 engine")
	}

	wasmBytes, err := wasmPlugin.WasmBytes()
	if err != nil {
		return fmt.Errorf("loading wasm bytes: %w", err)
	}

	moduleConfig := wazero.NewModuleConfig().WithName(plugin.Name())

	if e.config.ExecutionTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, e.config.ExecutionTimeout)
		defer cancel()
	}

	mod, err := e.runtime.InstantiateWithConfig(ctx, wasmBytes, moduleConfig)
	if err != nil {
		return fmt.Errorf("instantiating wasm module: %w", err)
	}
	defer mod.Close(ctx)

	if initFn := mod.ExportedFunction("init"); initFn != nil {
		if _, err := initFn.Call(ctx); err != nil {
			return fmt.Errorf("calling wasm init function: %w", err)
		}
	}

	if startFn := mod.ExportedFunction("start"); startFn != nil {
		if _, err := startFn.Call(ctx); err != nil {
			return fmt.Errorf("calling wasm start function: %w", err)
		}
	}

	return nil
}

// Close shuts down the wazero runtime and releases all resources.
func (e *WasmEngine) Close() error {
	if !e.initialized {
		return nil
	}

	if e.runtime != nil {
		if err := e.runtime.Close(context.Background()); err != nil {
			return fmt.Errorf("closing wazero runtime: %w", err)
		}
	}

	e.initialized = false
	return nil
}

// WasmPlugin defines the interface for WebAssembly plugins.
// Unlike native plugins, Wasm plugins must provide their .wasm binary
// and export specific lifecycle functions.
type WasmPlugin interface {
	core.Plugin
	WasmBytes() ([]byte, error)
}

// HostModuleBuilder helps construct host modules for exposing Core APIs to Wasm plugins.
type HostModuleBuilder struct {
	container       core.ServiceContainer
	events          core.EventBus
	logger          interface{ Info(string, ...any) }
	module          api.Module
	serviceRegistry []string
	registryMu      sync.RWMutex
}

// NewHostModuleBuilder creates a new host module builder with Core context.
func NewHostModuleBuilder(container core.ServiceContainer, events core.EventBus, logger ...interface{ Info(string, ...any) }) *HostModuleBuilder {
	var l interface{ Info(string, ...any) }
	if len(logger) > 0 {
		l = logger[0]
	} else {
		l = slog.Default()
	}
	b := &HostModuleBuilder{
		container:       container,
		events:          events,
		logger:          l,
		serviceRegistry: []string{},
	}
	b.refreshServiceRegistry()
	return b
}

func (b *HostModuleBuilder) refreshServiceRegistry() {
	if b.container == nil {
		return
	}
	b.registryMu.Lock()
	defer b.registryMu.Unlock()
	b.serviceRegistry = b.container.Keys()
}

// BuildCMSHostModule creates the "cms" host module with Core API functions.
func (b *HostModuleBuilder) BuildCMSHostModule(ctx context.Context, runtime wazero.Runtime) (api.Module, error) {
	module, err := runtime.NewHostModuleBuilder("cms").
		NewFunctionBuilder().WithFunc(b.serviceGet).Export("service_get").
		NewFunctionBuilder().WithFunc(b.serviceHas).Export("service_has").
		NewFunctionBuilder().WithFunc(b.eventSubscribe).Export("event_subscribe").
		NewFunctionBuilder().WithFunc(b.eventPublish).Export("event_publish").
		NewFunctionBuilder().WithFunc(b.hostLog).Export("host_log").
		NewFunctionBuilder().WithFunc(b.memoryAlloc).Export("memory_alloc").
		NewFunctionBuilder().WithFunc(b.memoryFree).Export("memory_free").
		Instantiate(ctx)
	if err != nil {
		return nil, err
	}
	b.module = module
	return module, nil
}

func (b *HostModuleBuilder) serviceGet(ctx context.Context, module api.Module, serviceID uint32) uint32 {
	if b.container == nil {
		return 0
	}

	b.refreshServiceRegistry()
	b.registryMu.RLock()
	defer b.registryMu.RUnlock()

	if serviceID >= uint32(len(b.serviceRegistry)) {
		return 0
	}

	return serviceID + 1
}

func (b *HostModuleBuilder) serviceHas(ctx context.Context, module api.Module, serviceID uint32) uint32 {
	if b.container == nil {
		return 0
	}

	b.refreshServiceRegistry()
	b.registryMu.RLock()
	defer b.registryMu.RUnlock()

	if serviceID >= uint32(len(b.serviceRegistry)) {
		return 0
	}

	return 1
}

func (b *HostModuleBuilder) eventSubscribe(ctx context.Context, module api.Module, topicPtr, topicLen, callbackPtr, callbackLen uint32) uint32 {
	if b.events == nil {
		return 0
	}

	topicBytes, ok := safeReadBytes(module, topicPtr, topicLen)
	if !ok {
		return 0
	}

	topic := string(topicBytes)
	if topic == "" {
		return 0
	}

	// Event subscription is not supported in the Wasm sandbox.
	// Wasm plugins should poll for events via host functions instead.
	// Return 0 to indicate subscription was not created.
	return 0
}

func (b *HostModuleBuilder) eventPublish(ctx context.Context, module api.Module, topicPtr, topicLen, dataPtr, dataLen uint32) {
	if b.events == nil {
		return
	}

	topicBytes, ok := safeReadBytes(module, topicPtr, topicLen)
	if !ok {
		return
	}

	dataBytes, ok := safeReadBytes(module, dataPtr, dataLen)
	if !ok {
		return
	}

	b.events.Emit(ctx, events.Event{
		Topic: string(topicBytes),
		Data:  map[string]interface{}{"payload": dataBytes},
	})
}

func (b *HostModuleBuilder) hostLog(ctx context.Context, module api.Module, msgPtr, msgLen uint32) {
	msg := safeReadWasmString(module, msgPtr, msgLen)
	if b.logger != nil {
		b.logger.Info("wasm plugin log", "message", msg)
	}
}

func (b *HostModuleBuilder) memoryAlloc(ctx context.Context, module api.Module, size uint32) uint32 {
	mem := module.Memory()
	if mem == nil {
		return 0
	}

	allocFn := module.ExportedFunction("allocate_memory")
	if allocFn == nil {
		return 0
	}

	results, err := allocFn.Call(ctx, uint64(size))
	if err != nil {
		return 0
	}

	return uint32(results[0])
}

func (b *HostModuleBuilder) memoryFree(ctx context.Context, module api.Module, ptr uint32) {
	freeFn := module.ExportedFunction("free_memory")
	if freeFn == nil {
		return
	}
	_, _ = freeFn.Call(ctx, uint64(ptr))
}

func safeReadBytes(module api.Module, ptr, length uint32) ([]byte, bool) {
	if length == 0 {
		return []byte{}, true
	}

	mem := module.Memory()
	if mem == nil {
		return nil, false
	}

	buf, ok := mem.Read(ptr, length)
	if !ok {
		return nil, false
	}
	return buf, true
}

func safeReadWasmString(module api.Module, ptr, length uint32) string {
	buf, ok := safeReadBytes(module, ptr, length)
	if !ok {
		return ""
	}
	return string(buf)
}

var _ Engine = (*WasmEngine)(nil)
