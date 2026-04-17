// Package engine provides plugin execution engines for Aroute CMS.
// It implements multiple execution backends (L1 native, L3 Wasm) with different
// isolation levels and performance characteristics.
package engine

import (
	"context"
	"fmt"
	"sync"

	"github.com/wangling-miao/aroute/core"
)

// Dispatcher manages multiple plugin execution engines and routes
// plugin execution requests to the appropriate engine based on the plugin's
// declared engine type (L1 native or L3 Wasm).
//
// Thread safety: Dispatcher methods are safe for concurrent use.
type Dispatcher interface {
	// RegisterEngine registers an execution engine for a specific engine type.
	// Only one engine per type is allowed. Returns error if type already registered.
	RegisterEngine(engineType core.EngineType, engine Engine) error

	// GetEngine retrieves the engine for a given type.
	// Returns error if no engine is registered for the type.
	GetEngine(engineType core.EngineType) (Engine, error)

	// Execute runs a plugin using the appropriate engine based on its manifest
	// and core context. Returns error if engine selection fails or execution fails.
	Execute(ctx context.Context, plugin core.Plugin, manifest *core.Manifest, coreCtx core.CoreContext) error

	// Close shuts down all registered engines and releases resources.
	// Should be called during graceful shutdown.
	Close() error
}

// Engine defines the interface for plugin execution backends.
// Each engine type (L1, L3) implements this interface with different
// isolation and performance characteristics.
type Engine interface {
	// Type returns the engine type identifier (L1Native or L3Wasm).
	Type() core.EngineType

	// Initialize prepares the engine for executing plugins.
	// For L1: validates plugin implements required interfaces.
	// For L3: initializes WebAssembly runtime (wazero).
	Initialize(ctx context.Context) error

	// ExecuteLifecycle runs the plugin lifecycle (Init → Start).
	// The plugin is loaded, dependencies resolved, and started.
	// Returns error if any lifecycle step fails.
	ExecuteLifecycle(ctx context.Context, plugin core.Plugin, ctx2 core.CoreContext) error

	// Close shuts down the engine and releases all resources.
	// After Close returns, no more ExecuteLifecycle calls should be made.
	Close() error
}

// DispatcherError represents errors from the Dispatcher operations.
type DispatcherError struct {
	Op     string // Operation that failed (register, execute, close)
	Engine string // Engine type involved (optional)
	Plugin string // Plugin name involved (optional)
	Err    error  // Underlying error
}

func (e *DispatcherError) Error() string {
	if e.Plugin != "" {
		return fmt.Sprintf("dispatcher %s error for plugin %s: %v", e.Op, e.Plugin, e.Err)
	}
	if e.Engine != "" {
		return fmt.Sprintf("dispatcher %s error for engine %s: %v", e.Op, e.Engine, e.Err)
	}
	return fmt.Sprintf("dispatcher %s error: %v", e.Op, e.Err)
}

func (e *DispatcherError) Unwrap() error {
	return e.Err
}

// dispatcherImpl is the concrete implementation of Dispatcher.
type dispatcherImpl struct {
	mu      sync.RWMutex
	engines map[core.EngineType]Engine
}

// NewDispatcher creates a new Dispatcher instance.
func NewDispatcher() Dispatcher {
	return &dispatcherImpl{
		engines: make(map[core.EngineType]Engine),
	}
}

// RegisterEngine registers an execution engine for a specific type.
func (d *dispatcherImpl) RegisterEngine(engineType core.EngineType, engine Engine) error {
	if engine == nil {
		return &DispatcherError{
			Op:     "register",
			Engine: engineType.String(),
			Err:    fmt.Errorf("engine cannot be nil"),
		}
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	if _, exists := d.engines[engineType]; exists {
		return &DispatcherError{
			Op:     "register",
			Engine: engineType.String(),
			Err:    fmt.Errorf("engine type already registered"),
		}
	}

	d.engines[engineType] = engine
	return nil
}

// GetEngine retrieves the engine for a given type.
func (d *dispatcherImpl) GetEngine(engineType core.EngineType) (Engine, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	engine, ok := d.engines[engineType]
	if !ok {
		return nil, &DispatcherError{
			Op:     "get",
			Engine: engineType.String(),
			Err:    fmt.Errorf("engine not registered"),
		}
	}
	return engine, nil
}

// Execute runs a plugin using the appropriate engine.
// Engine selection strategy:
//  1. Parse manifest.Engine field (e.g., "native", "wasm", "l1", "l3")
//  2. Map string to EngineType enum
//  3. Retrieve registered engine for that type
//  4. Execute lifecycle through the engine
func (d *dispatcherImpl) Execute(ctx context.Context, plugin core.Plugin, manifest *core.Manifest, coreCtx core.CoreContext) error {
	engineType, err := core.ParseEngine(manifest.Engine)
	if err != nil {
		return &DispatcherError{Op: "execute", Plugin: manifest.Name, Err: fmt.Errorf("invalid engine type: %w", err)}
	}

	engine, err := d.GetEngine(engineType)
	if err != nil {
		return &DispatcherError{Op: "execute", Plugin: manifest.Name, Err: fmt.Errorf("engine not available: %w", err)}
	}

	d.mu.RLock()
	defer d.mu.RUnlock()

	if engine.Type() != engineType {
		return &DispatcherError{Op: "execute", Plugin: manifest.Name, Err: fmt.Errorf("plugin engine mismatch: manifest=%v, actual=%v", engineType, engine.Type())}
	}

	return engine.ExecuteLifecycle(ctx, plugin, coreCtx)
}

// Close shuts down all engines.
func (d *dispatcherImpl) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	var errs []error

	for engineType, engine := range d.engines {
		if err := engine.Close(); err != nil {
			errs = append(errs, &DispatcherError{
				Op:     "close",
				Engine: engineType.String(),
				Err:    err,
			})
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("dispatcher close errors: %v", errs)
	}

	return nil
}
