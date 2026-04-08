// Package engine provides plugin execution engines for Aroute CMS.
package engine

import (
	"context"
	"fmt"

	"github.com/wangling-miao/aroute/core"
)

// NativeEngine implements the L1 execution backend for trusted Go plugins.
// L1 native plugins run in the same process with direct Go interface calls,
// providing maximum performance and access to all Core APIs.
//
// Security: L1 plugins are trusted - they have full access to Core APIs
// and can directly call Go methods without sandboxing. Only use L1 for
// plugins you control and trust completely.
type NativeEngine struct {
	initialized bool
}

// NewNativeEngine creates a new L1 native engine instance.
func NewNativeEngine() *NativeEngine {
	return &NativeEngine{}
}

// Type returns EngineL1Native.
func (e *NativeEngine) Type() core.EngineType {
	return core.EngineL1Native
}

// Initialize validates the native engine is ready for use.
// For L1 native, this is a no-op since there's no runtime to initialize.
func (e *NativeEngine) Initialize(ctx context.Context) error {
	if e.initialized {
		return nil
	}

	e.initialized = true
	return nil
}

// ExecuteLifecycle runs the plugin lifecycle (Init → Start) via direct Go method calls.
// The plugin's Init() method receives CoreContext for dependency injection and
// event subscription. After successful Init, Start() is called to activate the plugin.
// Panics during lifecycle methods are recovered and returned as errors.
func (e *NativeEngine) ExecuteLifecycle(ctx context.Context, plugin core.Plugin, coreCtx core.CoreContext) (err error) {
	if !e.initialized {
		return fmt.Errorf("native engine not initialized")
	}

	if plugin == nil {
		return fmt.Errorf("plugin cannot be nil")
	}

	if coreCtx == nil {
		return fmt.Errorf("core context cannot be nil")
	}

	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("plugin panic recovered: %v", r)
		}
	}()

	if err := plugin.Init(coreCtx); err != nil {
		return fmt.Errorf("plugin Init() failed: %w", err)
	}

	if err := plugin.Start(); err != nil {
		_ = plugin.Stop()
		return fmt.Errorf("plugin Start() failed: %w", err)
	}

	return nil
}

// Close shuts down the native engine.
// For L1 native, this is a no-op since there's no runtime state to clean up.
func (e *NativeEngine) Close() error {
	e.initialized = false
	return nil
}

// Ensure NativeEngine implements Engine interface at compile time.
var _ Engine = (*NativeEngine)(nil)
