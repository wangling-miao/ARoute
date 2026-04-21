package registry

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/wangling-miao/aroute/core"
)

// FSDiscovery implements Discovery by scanning filesystem directories.
// It looks for manifest.yaml or manifest.json files in each subdirectory.
type FSDiscovery struct {
	// RootDir is the base directory to scan for plugins.
	RootDir string
}

// NewFSDiscovery creates a new filesystem-based discovery implementation.
func NewFSDiscovery(rootDir string) *FSDiscovery {
	return &FSDiscovery{RootDir: rootDir}
}

// Discover scans the plugins directory for manifest files.
// Returns a map of plugin name to absolute manifest path.
//
// Directory structure expected:
//
//	plugins/
//	  ├── http/
//	  │   └── manifest.yaml
//	  ├── database/
//	  │   └── manifest.json
//	  └── auth/
//	      └── manifest.yaml
//
// Each directory should contain exactly one manifest file (YAML or JSON).
func (d *FSDiscovery) Discover() (map[string]string, error) {
	plugins := make(map[string]string)

	// Check if root directory exists
	info, err := os.Stat(d.RootDir)
	if err != nil {
		if os.IsNotExist(err) {
			// Directory doesn't exist yet, return empty map
			return plugins, nil
		}
		return nil, fmt.Errorf("stat plugins directory: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", d.RootDir)
	}

	// Read directory entries
	entries, err := os.ReadDir(d.RootDir)
	if err != nil {
		return nil, fmt.Errorf("read plugins directory: %w", err)
	}

	// Scan each subdirectory for manifest files
	for _, entry := range entries {
		if !entry.IsDir() {
			// Skip non-directory entries (files in root)
			continue
		}

		pluginName := entry.Name()
		pluginDir := filepath.Join(d.RootDir, pluginName)

		manifestPath, err := d.findManifest(pluginDir)
		if err != nil {
			// Log warning but continue discovering other plugins
			// In production, use structured logging
			continue
		}

		if manifestPath != "" {
			plugins[pluginName] = manifestPath
		}
	}

	return plugins, nil
}

// findManifest searches for a manifest file in the given directory.
// Returns the absolute path to the manifest file, or empty string if not found.
// Prefers manifest.yaml over manifest.json if both exist.
func (d *FSDiscovery) findManifest(dir string) (string, error) {
	// Try YAML first (preferred)
	yamlPath := filepath.Join(dir, "manifest.yaml")
	if _, err := os.Stat(yamlPath); err == nil {
		return filepath.Abs(yamlPath)
	}

	ymlPath := filepath.Join(dir, "manifest.yml")
	if _, err := os.Stat(ymlPath); err == nil {
		return filepath.Abs(ymlPath)
	}

	// Try JSON
	jsonPath := filepath.Join(dir, "manifest.json")
	if _, err := os.Stat(jsonPath); err == nil {
		return filepath.Abs(jsonPath)
	}

	// No manifest found
	return "", fmt.Errorf("no manifest file found in %s", dir)
}

// LoadAndRegister discovers plugins from filesystem and registers them with the registry.
// For each discovered manifest, it loads the manifest, creates a PluginEntry,
// and registers it with the provided registry.
//
// If a plugin already exists in the registry with the same name:
// - If manifests match: skip (already registered)
// - If manifests differ: update existing entry (plugin upgrade scenario)
//
// Returns the number of plugins registered and any errors encountered.
func LoadAndRegister(registry Registry, discovery Discovery) (int, error) {
	paths, err := discovery.Discover()
	if err != nil {
		return 0, fmt.Errorf("discover plugins: %w", err)
	}

	registered := 0

	for name, manifestPath := range paths {
		// Load manifest
		manifest, err := core.LoadManifest(manifestPath)
		if err != nil {
			// Log warning and continue
			continue
		}

		// Verify manifest name matches directory name
		if manifest.Name != name {
			// Manifest name should match directory name
			// This is a validation error, skip this plugin
			continue
		}

		// Check if plugin already exists
		existing, err := registry.Get(name)
		if err == nil {
			// Plugin already exists, check if we need to update
			if manifestsEqual(&existing.Manifest, manifest) {
				// Same version, skip
				registered++
				continue
			}

			// Different version, update
			if err := registry.Update(name, *manifest); err != nil {
				continue
			}
			registered++
			continue
		}

		// New plugin, register it
		entry := &PluginEntry{
			Manifest:       *manifest,
			Enabled:        true, // Enable by default
			DiscoveredPath: manifestPath,
		}

		if err := registry.Register(entry); err != nil {
			continue
		}
		registered++
	}

	return registered, nil
}

// manifestsEqual checks if two manifests are identical (same version and content).
func manifestsEqual(a, b *core.Manifest) bool {
	if a.Name != b.Name {
		return false
	}
	if a.Version != b.Version {
		return false
	}
	if a.Description != b.Description {
		return false
	}
	if a.Author != b.Author {
		return false
	}
	if a.License != b.License {
		return false
	}
	if a.Engine != b.Engine {
		return false
	}
	if len(a.Requires) != len(b.Requires) {
		return false
	}
	if len(a.Provides) != len(b.Provides) {
		return false
	}
	// Compare requires
	for i, req := range a.Requires {
		if b.Requires[i] != req {
			return false
		}
	}
	// Compare provides
	for i, prov := range a.Provides {
		if b.Provides[i] != prov {
			return false
		}
	}
	return true
}

// DiscoverOne discovers a single plugin by name.
// Returns the manifest path if found, or an error if not found.
func (d *FSDiscovery) DiscoverOne(name string) (string, error) {
	pluginDir := filepath.Join(d.RootDir, name)

	// Check if directory exists
	info, err := os.Stat(pluginDir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("plugin directory %s not found", name)
		}
		return "", fmt.Errorf("stat plugin directory: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s is not a directory", name)
	}

	// Find manifest
	manifestPath, err := d.findManifest(pluginDir)
	if err != nil {
		return "", fmt.Errorf("find manifest: %w", err)
	}

	return manifestPath, nil
}

// ValidateManifest validates a plugin manifest.
// Returns an error if the manifest is invalid.
func ValidateManifest(manifestPath string) error {
	_, err := core.LoadManifest(manifestPath)
	if err != nil {
		return fmt.Errorf("load manifest: %w", err)
	}
	return nil
}

// CleanupMissingPlugins removes registry entries for L2/L3 (non-native) plugins
// whose files are no longer present on disk. Returns the number of entries removed.
//
// For each registry entry with engine "wasm" or "grpc", it checks whether the
// discovered path (manifest file) still exists. For wasm plugins, it also checks
// that the plugin.wasm binary is present. If either is missing, the entry is removed.
func CleanupMissingPlugins(registry Registry) (int, error) {
	entries, err := registry.List()
	if err != nil {
		return 0, fmt.Errorf("list registry: %w", err)
	}

	removed := 0
	for _, entry := range entries {
		// Only cleanup non-native (L2/L3) plugins
		if entry.Manifest.Engine == "" || entry.Manifest.Engine == "native" || entry.Manifest.Engine == "l1" {
			continue
		}

		// Check if the manifest file still exists
		if entry.DiscoveredPath != "" {
			if _, err := os.Stat(entry.DiscoveredPath); os.IsNotExist(err) {
				if err := registry.Remove(entry.Manifest.Name); err != nil {
					continue
				}
				removed++
				continue
			}
		}

		// For L3 (wasm) plugins, also check the wasm binary exists
		if entry.Manifest.Engine == "wasm" || entry.Manifest.Engine == "l3" {
			pluginDir := filepath.Dir(entry.DiscoveredPath)
			if pluginDir != "" && entry.DiscoveredPath != "" {
				wasmFile := filepath.Join(pluginDir, "plugin.wasm")
				if _, err := os.Stat(wasmFile); os.IsNotExist(err) {
					if err := registry.Remove(entry.Manifest.Name); err != nil {
						continue
					}
					removed++
				}
			}
		}
	}

	return removed, nil
}
