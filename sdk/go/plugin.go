// Package sdk provides the Plugin Development Kit for building Aroute CMS plugins.
//
// The SDK offers a high-level API on top of the Core microkernel, including:
//   - BasePlugin: a ready-to-use plugin skeleton with manifest auto-loading
//   - Service accessor helpers (GetDB, GetAuth, GetContent, etc.)
//   - SDK versioning for compatibility checks
//
// Quick start:
//
//	package main
//
//	import (
//	    "github.com/wangling-miao/aroute/sdk/go"
//	    "github.com/wangling-miao/aroute/core"
//	)
//
//	type MyPlugin struct {
//	    *sdk.BasePlugin
//	}
//
//	func New() *MyPlugin {
//	    return &MyPlugin{
//	        BasePlugin: sdk.MustNewBasePlugin("my-plugin", "1.0.0"),
//	    }
//	}
//
//	func (p *MyPlugin) Init(ctx core.CoreContext) error {
//	    db, err := sdk.GetDB(ctx.Services())
//	    if err != nil {
//	        return err
//	    }
//	    _ = db // use database service
//	    return nil
//	}
//
// Plugin lifecycle follows the Core state machine:
//
//	Registered → Resolved → Starting → Active → Stopping → Stopped
//	                     ↓
//	                   Failed
//
// Thread safety: All SDK types are safe for concurrent use.
package sdk

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/wangling-miao/aroute/core"
)

// SDKVersion is the current semantic version of the Plugin SDK.
// Increment this when releasing a new SDK version.
// Follows SemVer: MAJOR.MINOR.PATCH
const SDKVersion = "1.0.0"

// Version returns the current SDK version string in SemVer format.
// Plugins can use this to check SDK compatibility at startup.
func Version() string {
	return SDKVersion
}

// BasePlugin provides a default implementation of core.Plugin with manifest
// auto-loading and no-op lifecycle hooks. Plugin authors embed this struct
// to avoid implementing all interface methods, overriding only the hooks they need.
//
// BasePlugin handles:
//   - Manifest loading from "plugin.yaml" or "manifest.yaml" in the plugin directory
//   - Default no-op Init, Start, and Stop methods
//   - Access to the plugin's manifest metadata
//
// Embedding example:
//
//	type MyPlugin struct {
//	    *sdk.BasePlugin
//	    // custom fields...
//	}
//
//	func New() *MyPlugin {
//	    return &MyPlugin{
//	        BasePlugin: sdk.MustNewBasePlugin("my-plugin", "1.0.0"),
//	    }
//	}
//
// Override only the hooks you need:
//
//	func (p *MyPlugin) Init(ctx core.CoreContext) error {
//	    // custom initialization
//	    return nil
//	}
type BasePlugin struct {
	name    string
	version string
	manifest *core.Manifest
	logger   *slog.Logger
	ctx      core.CoreContext
}

// NewBasePlugin creates a BasePlugin with the given name and version.
// The manifest is created inline from the provided parameters.
// For manifest auto-loading from file, use NewBasePluginFromFile instead.
//
// Parameters:
//   - name: unique plugin identifier (lowercase alphanumeric + hyphens)
//   - version: semantic version string (e.g., "1.0.0")
//
// Returns a *BasePlugin ready for embedding.
func NewBasePlugin(name, version string) *BasePlugin {
	return &BasePlugin{
		name:    name,
		version: version,
	}
}

// MustNewBasePlugin creates a BasePlugin, panicking on validation errors.
// Use this when the plugin name and version are hardcoded constants
// and a failure indicates a programming error.
func MustNewBasePlugin(name, version string) *BasePlugin {
	bp := NewBasePlugin(name, version)
	if err := bp.validate(); err != nil {
		panic(fmt.Sprintf("sdk: invalid plugin definition: %v", err))
	}
	return bp
}

// NewBasePluginFromFile creates a BasePlugin by loading a manifest file.
// Searches for "plugin.yaml", "plugin.yml", or "manifest.yaml" in the
// given directory.
//
// Parameters:
//   - dir: directory containing the manifest file
//
// Returns a *BasePlugin populated from the manifest, or an error if loading fails.
func NewBasePluginFromFile(dir string) (*BasePlugin, error) {
	manifestPath, err := findManifest(dir)
	if err != nil {
		return nil, err
	}

	manifest, err := core.LoadManifest(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("sdk: load manifest from %s: %w", manifestPath, err)
	}

	return &BasePlugin{
		name:     manifest.Name,
		version:  manifest.Version,
		manifest: manifest,
	}, nil
}

// MustNewBasePluginFromFile creates a BasePlugin from a manifest file,
// panicking if loading fails. Use this when the manifest file is expected
// to always be present (e.g., embedded via go:embed).
func MustNewBasePluginFromFile(dir string) *BasePlugin {
	bp, err := NewBasePluginFromFile(dir)
	if err != nil {
		panic(fmt.Sprintf("sdk: failed to load plugin manifest: %v", err))
	}
	return bp
}

// Name returns the plugin's unique identifier.
// This value comes from the manifest or constructor parameter.
func (p *BasePlugin) Name() string { return p.name }

// Version returns the plugin's semantic version string.
func (p *BasePlugin) Version() string { return p.version }

// Manifest returns the plugin's metadata and dependency declarations.
// Returns nil if the plugin was created with NewBasePlugin (no file loaded).
func (p *BasePlugin) Manifest() *core.Manifest { return p.manifest }

// Description returns the plugin description from the manifest.
// Returns empty string if no manifest was loaded.
func (p *BasePlugin) Description() string {
	if p.manifest == nil {
		return ""
	}
	return p.manifest.Description
}

// Author returns the plugin author from the manifest.
// Returns empty string if no manifest was loaded.
func (p *BasePlugin) Author() string {
	if p.manifest == nil {
		return ""
	}
	return p.manifest.Author
}

// Init initializes the plugin with the Core context.
// Default implementation stores the context for later use by helper methods.
// Override this method to add custom initialization logic.
//
// The ctx parameter provides access to:
//   - Services: ServiceContainer for dependency injection
//   - Events: EventBus for plugin communication
//   - Config: Configuration values
//   - Logger: Structured logging
//   - DataDir: Plugin-specific data directory
//   - PluginDir: Plugin installation directory
func (p *BasePlugin) Init(ctx core.CoreContext) error {
	p.ctx = ctx
	p.logger = ctx.Logger()
	return nil
}

// Start activates the plugin after all dependencies are resolved and initialized.
// Default implementation is a no-op. Override to add startup logic (e.g., start
// HTTP listeners, begin background processing).
func (p *BasePlugin) Start() error { return nil }

// Stop gracefully shuts down the plugin.
// Default implementation is a no-op. Override to clean up resources,
// stop goroutines, and close connections.
func (p *BasePlugin) Stop() error { return nil }

// Context returns the CoreContext stored during Init.
// Returns nil if Init has not been called yet.
// Use this to access services, events, config, and logger
// in methods other than Init.
func (p *BasePlugin) Context() core.CoreContext { return p.ctx }

// Logger returns the plugin's structured logger.
// Returns nil if Init has not been called yet.
func (p *BasePlugin) Logger() *slog.Logger { return p.logger }

// SetManifest allows setting the manifest after construction.
// Useful for plugins that load manifest data from embedded resources.
func (p *BasePlugin) SetManifest(m *core.Manifest) {
	p.manifest = m
	p.name = m.Name
	p.version = m.Version
}

// validate checks that the plugin name and version are valid.
func (p *BasePlugin) validate() error {
	if p.name == "" {
		return fmt.Errorf("plugin name is required")
	}
	if p.version == "" {
		return fmt.Errorf("plugin version is required")
	}
	return nil
}

// Compile-time interface check: BasePlugin satisfies core.Plugin.
var _ core.Plugin = (*BasePlugin)(nil)

// findManifest searches for a manifest file in the given directory.
// Checks for plugin.yaml, plugin.yml, and manifest.yaml in order.
func findManifest(dir string) (string, error) {
	candidates := []string{
		filepath.Join(dir, "plugin.yaml"),
		filepath.Join(dir, "plugin.yml"),
		filepath.Join(dir, "manifest.yaml"),
	}

	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}

	return "", fmt.Errorf("sdk: no manifest file found in %s (checked plugin.yaml, plugin.yml, manifest.yaml)", dir)
}

// ContextAware is an optional interface that plugins can implement to indicate
// they accept a context.Context in their lifecycle methods. The Core runtime
// will pass request-scoped context values during lifecycle transitions.
type ContextAware interface {
	// StartWithContext starts the plugin with a cancellable context.
	// Use for plugins that need to monitor shutdown signals.
	StartWithContext(ctx context.Context) error

	// StopWithContext stops the plugin with a timeout context.
	// Use for plugins that need bounded shutdown time.
	StopWithContext(ctx context.Context) error
}
