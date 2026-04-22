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
	"sync"

	"github.com/wangling-miao/aroute/core"
)

// SDKVersion is the current semantic version of the Plugin SDK.
const SDKVersion = "1.0.0"

// Version returns the current SDK version string in SemVer format.
func Version() string {
	return SDKVersion
}

// BasePlugin provides a default implementation of core.Plugin with manifest
// auto-loading and no-op lifecycle hooks. Plugin authors embed this struct
// to avoid implementing all interface methods, overriding only the hooks they need.
//
// BasePlugin handles:
//   - Manifest loading from "plugin.yaml", "plugin.yml", "plugin.json", "manifest.yaml", or "manifest.json"
//   - Default no-op Init, Start, and Stop methods
//   - Access to the plugin's manifest metadata
//
// Thread safety: All methods are safe for concurrent use.
type BasePlugin struct {
	mu       sync.RWMutex
	name     string
	version  string
	manifest *core.Manifest
	logger   *slog.Logger
	ctx      core.CoreContext
}

// NewBasePlugin creates a BasePlugin with the given name and version.
// The manifest is created inline from the provided parameters.
// For manifest auto-loading from file, use NewBasePluginFromFile instead.
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
// Searches for "plugin.yaml", "plugin.yml", "plugin.json", "manifest.yaml",
// or "manifest.json" in the given directory.
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
// panicking if loading fails.
func MustNewBasePluginFromFile(dir string) *BasePlugin {
	bp, err := NewBasePluginFromFile(dir)
	if err != nil {
		panic(fmt.Sprintf("sdk: failed to load plugin manifest: %v", err))
	}
	return bp
}

// Name returns the plugin's unique identifier.
func (p *BasePlugin) Name() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.name
}

// Version returns the plugin's semantic version string.
func (p *BasePlugin) Version() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.version
}

// Manifest returns the plugin's metadata and dependency declarations.
// Returns nil if the plugin was created with NewBasePlugin (no file loaded).
func (p *BasePlugin) Manifest() *core.Manifest {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.manifest
}

// Description returns the plugin description from the manifest.
func (p *BasePlugin) Description() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.manifest == nil {
		return ""
	}
	return p.manifest.Description
}

// Author returns the plugin author from the manifest.
func (p *BasePlugin) Author() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.manifest == nil {
		return ""
	}
	return p.manifest.Author
}

// Init initializes the plugin with the Core context.
// Default implementation stores the context for later use by helper methods.
// Override this method to add custom initialization logic.
func (p *BasePlugin) Init(ctx core.CoreContext) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.ctx = ctx
	p.logger = ctx.Logger()
	return nil
}

// Start activates the plugin after all dependencies are resolved and initialized.
// Default implementation is a no-op.
func (p *BasePlugin) Start() error { return nil }

// Stop gracefully shuts down the plugin.
// Default implementation is a no-op.
func (p *BasePlugin) Stop() error { return nil }

// Context returns the CoreContext stored during Init.
// Returns nil if Init has not been called yet.
func (p *BasePlugin) Context() core.CoreContext {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.ctx
}

// Logger returns the plugin's structured logger.
// Returns nil if Init has not been called yet.
func (p *BasePlugin) Logger() *slog.Logger {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.logger
}

// SetManifest allows setting the manifest after construction.
func (p *BasePlugin) SetManifest(m *core.Manifest) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.manifest = m
	p.name = m.Name
	p.version = m.Version
}

// validate checks that the plugin name and version meet Core requirements.
func (p *BasePlugin) validate() error {
	m := &core.Manifest{
		Name:    p.name,
		Version: p.version,
		Engine:  "native",
	}
	if p.manifest != nil {
		m = p.manifest
	}
	return m.Validate()
}

// Compile-time interface check: BasePlugin satisfies core.Plugin.
var _ core.Plugin = (*BasePlugin)(nil)

// findManifest searches for a manifest file in the given directory.
// Checks plugin.yaml, plugin.yml, plugin.json, manifest.yaml, manifest.json in order.
func findManifest(dir string) (string, error) {
	candidates := []string{
		filepath.Join(dir, "plugin.yaml"),
		filepath.Join(dir, "plugin.yml"),
		filepath.Join(dir, "plugin.json"),
		filepath.Join(dir, "manifest.yaml"),
		filepath.Join(dir, "manifest.json"),
	}

	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}

	return "", fmt.Errorf("sdk: no manifest file found in %s (checked plugin.yaml, plugin.yml, plugin.json, manifest.yaml, manifest.json)", dir)
}

// ContextAware is an optional interface that plugins can implement to indicate
// they accept a context.Context in their lifecycle methods.
type ContextAware interface {
	StartWithContext(ctx context.Context) error
	StopWithContext(ctx context.Context) error
}
