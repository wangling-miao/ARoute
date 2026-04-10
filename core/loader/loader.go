// Package loader provides plugin loading mechanisms for Aroute CMS.
// It implements the PluginLoader interface used by the lifecycle manager
// to instantiate plugins from their manifests.
package loader

import (
	"fmt"
	"sync"

	"github.com/wangling-miao/aroute/core"
)

// PluginFactory is a function that creates a new Plugin instance.
// Factories are registered with the loader and called when a plugin
// needs to be instantiated.
type PluginFactory func() core.Plugin

// NativePluginLoader implements the PluginLoader interface for native Go plugins.
// It maintains a registry of plugin factories that can be called to create
// plugin instances at runtime.
//
// Thread safety: All methods are safe for concurrent use.
type NativePluginLoader struct {
	mu        sync.RWMutex
	factories map[string]PluginFactory
}

// NewNativePluginLoader creates a new loader with an empty registry.
func NewNativePluginLoader() *NativePluginLoader {
	return &NativePluginLoader{
		factories: make(map[string]PluginFactory),
	}
}

// Register adds a plugin factory to the loader's registry.
// The factory will be called when Load() is invoked for a plugin with
// the matching name.
//
// If a factory is already registered for the name, it will be replaced.
// This allows for hot-replacement during development.
func (l *NativePluginLoader) Register(name string, factory PluginFactory) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.factories[name] = factory
}

// Unregister removes a plugin factory from the registry.
// After unregistration, Load() will fail for that plugin name.
func (l *NativePluginLoader) Unregister(name string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.factories, name)
}

// Has checks if a factory is registered for the given plugin name.
func (l *NativePluginLoader) Has(name string) bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.factories[name] != nil
}

// List returns all registered plugin names.
func (l *NativePluginLoader) List() []string {
	l.mu.RLock()
	defer l.mu.RUnlock()

	names := make([]string, 0, len(l.factories))
	for name := range l.factories {
		names = append(names, name)
	}
	return names
}

// Load creates a new Plugin instance from the manifest.
// It looks up the registered factory for the plugin name and calls it.
//
// Load only supports native plugins (Engine: "native").
// For Wasm plugins, the WasmLoader should be used instead.
//
// Returns error if:
//   - No factory is registered for the plugin name
//   - Plugin engine is not "native"
func (l *NativePluginLoader) Load(manifest core.Manifest) (core.Plugin, error) {
	// Check engine type - only native plugins supported
	engineType, err := core.ParseEngine(manifest.Engine)
	if err != nil {
		return nil, fmt.Errorf("loader: invalid engine type '%s': %w", manifest.Engine, err)
	}

	if engineType != core.EngineL1Native {
		return nil, fmt.Errorf("loader: plugin '%s' has engine '%s', only 'native' is supported by NativePluginLoader", manifest.Name, manifest.Engine)
	}

	// Look up factory
	l.mu.RLock()
	factory, exists := l.factories[manifest.Name]
	l.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("loader: no factory registered for plugin '%s'", manifest.Name)
	}

	// Create plugin instance
	plugin := factory()
	if plugin == nil {
		return nil, fmt.Errorf("loader: factory for '%s' returned nil", manifest.Name)
	}

	return plugin, nil
}

// Compile-time interface check
var _ interface {
	Load(core.Manifest) (core.Plugin, error)
} = (*NativePluginLoader)(nil)
