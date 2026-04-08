// Package core provides the microkernel infrastructure for Aroute CMS.
// It implements the pure microkernel architecture where Core only manages
// plugin lifecycle, service discovery, and event distribution.
package core

// Plugin defines the interface that all Aroute plugins must implement.
// Plugins are the fundamental building blocks of the CMS - all functionality
// (including HTTP server, database, authentication) is provided through plugins.
//
// Plugin lifecycle:
//
//	Registered → Resolved → Starting → Active → Stopping → Stopped
//	                    ↓
//	                  Failed
//
// Custom implementations should embed BasePlugin to avoid implementing all methods.
//
// Thread safety: Implementations must be safe for concurrent use across goroutines.
type Plugin interface {
	// Name returns the unique identifier for this plugin.
	// Names must be lowercase, alphanumeric with hyphens allowed (e.g., "http-server", "content").
	// The name should match the manifest name exactly.
	Name() string

	// Version returns the plugin's semantic version.
	// Must be a valid semver string (e.g., "1.0.0", "2.3.4-beta.1").
	Version() string

	// Manifest returns the plugin's metadata and dependency declarations.
	// The manifest is loaded from manifest.yaml/manifest.json during discovery.
	Manifest() *Manifest

	// Init initializes the plugin with the Core context.
	// Called once during plugin startup. Should validate configuration,
	// register services, and subscribe to events.
	//
	// Init MUST be idempotent - calling it multiple times should be safe.
	// Implementations should return errors rather than panic.
	//
	// ctx provides access to:
	//   - Services: ServiceContainer for dependency injection
	//   - Events: EventBus for plugin communication
	//   - Config: Configuration values
	//   - Logger: Structured logging
	//   - DataDir: Plugin-specific data directory
	//   - PluginDir: Plugin installation directory
	Init(ctx CoreContext) error

	// Start activates the plugin after successful initialization.
	// Called after all dependencies have been resolved and initialized.
	// For example, an HTTP plugin would start the server here.
	//
	// Start should block until the plugin is ready to serve requests.
	// If the plugin needs background goroutines, start them here.
	// Return an error if startup fails.
	Start() error

	// Stop gracefully shuts down the plugin.
	// Called during graceful shutdown or when disabling the plugin.
	// Should clean up resources, stop goroutines, and close connections.
	//
	// After Stop returns, no plugin methods will be called.
	Stop() error
}

// PluginLifecycleHooks defines optional lifecycle hooks for plugins.
// Plugins can implement this interface to receive load/unload notifications.
// These hooks are NOT part of the main Plugin interface.
//
// OnLoad is called before Init, OnUnload is called after Stop.
// Use for setup/cleanup that doesn't require Core context.
type PluginLifecycleHooks interface {
	// OnLoad is called after the plugin binary/module is loaded into memory
	// but before Init. Use for one-time setup that doesn't depend on Core
	// context or other plugins.
	//
	// This is useful for:
	//   - Registering plugin-specific flags
	//   - Setting up package-level state
	//   - Validating build-time dependencies
	OnLoad() error

	// OnUnload is called after Stop during plugin unload.
	// Use for cleanup that can happen after the plugin is fully stopped.
	//
	// This is useful for:
	//   - Releasing package-level resources
	//   - Clearing global state
	//   - Cleanup that doesn't require Core context
	OnUnload() error
}

// PluginState represents the current state of a plugin in its lifecycle.
type PluginState int

const (
	// StateRegistered indicates the plugin is discovered and registered
	// but not yet initialized.
	StateRegistered PluginState = iota

	// StateResolved indicates all dependencies are satisfied.
	// The plugin is ready for initialization.
	StateResolved

	// StateStarting indicates the plugin is currently initializing.
	StateStarting

	// StateActive indicates the plugin is fully initialized and running.
	StateActive

	// StateStopping indicates the plugin is shutting down.
	StateStopping

	// StateStopped indicates the plugin has been stopped.
	StateStopped

	// StateFailed indicates the plugin encountered an error during
	// initialization, startup, or runtime.
	StateFailed
)

// String returns the string representation of the plugin state.
func (s PluginState) String() string {
	switch s {
	case StateRegistered:
		return "registered"
	case StateResolved:
		return "resolved"
	case StateStarting:
		return "starting"
	case StateActive:
		return "active"
	case StateStopping:
		return "stopping"
	case StateStopped:
		return "stopped"
	case StateFailed:
		return "failed"
	default:
		return "unknown"
	}
}

// EngineType defines the execution engine for a plugin.
// Aroute supports multiple plugin engines with different isolation levels.
type EngineType int

const (
	// EngineL1Native indicates a native Go plugin running in the same process.
	// L1 plugins have direct access to Core APIs via Go interfaces.
	// Maximum performance, minimum isolation - trusted plugins only.
	EngineL1Native EngineType = iota

	// EngineL3Wasm indicates a WebAssembly plugin running in a sandbox.
	// L3 plugins are isolated via wazero runtime with memory limits.
	// Zero trust isolation - suitable for untrusted plugins.
	EngineL3Wasm

	// Note: L2 gRPC plugins are reserved for Pro/Enterprise versions
	// and provide process isolation with gRPC communication.
)

func (e EngineType) String() string {
	switch e {
	case EngineL1Native:
		return "native"
	case EngineL3Wasm:
		return "wasm"
	default:
		return "unknown"
	}
}
