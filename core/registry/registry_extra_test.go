package registry

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/wangling-miao/aroute/core"
	bolt "go.etcd.io/bbolt"
)

// =============================================================================
// FSDiscovery Tests
// =============================================================================

func TestFSDiscovery_EmptyDirectory(t *testing.T) {
	// Create empty root directory
	rootDir := t.TempDir()

	discovery := NewFSDiscovery(rootDir)
	plugins, err := discovery.Discover()
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}

	if len(plugins) != 0 {
		t.Errorf("Discover() should return empty map for empty directory, got %d", len(plugins))
	}
}

func TestFSDiscovery_NonExistentDirectory(t *testing.T) {
	rootDir := filepath.Join(t.TempDir(), "nonexistent")

	discovery := NewFSDiscovery(rootDir)
	plugins, err := discovery.Discover()
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}

	// Non-existent directory should return empty map, not error
	if len(plugins) != 0 {
		t.Errorf("Discover() should return empty map for non-existent directory, got %d", len(plugins))
	}
}

func TestFSDiscovery_DirectoryIsFile(t *testing.T) {
	// Create a file instead of directory
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "notadir")
	if err := os.WriteFile(filePath, []byte("test"), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	discovery := NewFSDiscovery(filePath)
	_, err := discovery.Discover()
	if err == nil {
		t.Error("Discover() should fail when path is a file, not directory")
	}
}

func TestFSDiscovery_MissingManifest(t *testing.T) {
	rootDir := t.TempDir()

	// Create plugin directory without manifest
	pluginDir := filepath.Join(rootDir, "plugin-no-manifest")
	if err := os.Mkdir(pluginDir, 0755); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}

	// Create another plugin directory with manifest (should be discovered)
	pluginDir2 := filepath.Join(rootDir, "plugin-with-manifest")
	if err := os.Mkdir(pluginDir2, 0755); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	manifestPath := filepath.Join(pluginDir2, "manifest.yaml")
	if err := os.WriteFile(manifestPath, []byte("name: plugin-with-manifest\nversion: 1.0.0\nengine: native"), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	discovery := NewFSDiscovery(rootDir)
	plugins, err := discovery.Discover()
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}

	// Only plugin-with-manifest should be discovered
	if len(plugins) != 1 {
		t.Errorf("Discover() should find 1 plugin, got %d", len(plugins))
	}

	if _, ok := plugins["plugin-with-manifest"]; !ok {
		t.Error("Discover() should find plugin-with-manifest")
	}

	if _, ok := plugins["plugin-no-manifest"]; ok {
		t.Error("Discover() should not find plugin without manifest")
	}
}

func TestFSDiscovery_YAMLManifest(t *testing.T) {
	rootDir := t.TempDir()

	// Create plugin directory with YAML manifest
	pluginDir := filepath.Join(rootDir, "yaml-plugin")
	if err := os.Mkdir(pluginDir, 0755); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}

	manifestContent := `
name: yaml-plugin
version: 1.0.0
description: A YAML plugin
author: test
license: MIT
engine: native
`
	manifestPath := filepath.Join(pluginDir, "manifest.yaml")
	if err := os.WriteFile(manifestPath, []byte(manifestContent), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	discovery := NewFSDiscovery(rootDir)
	plugins, err := discovery.Discover()
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}

	if len(plugins) != 1 {
		t.Errorf("Discover() should find 1 plugin, got %d", len(plugins))
	}

	// Verify path is absolute
	path := plugins["yaml-plugin"]
	if !filepath.IsAbs(path) {
		t.Errorf("Discover() should return absolute path, got %s", path)
	}
}

func TestFSDiscovery_YMLExtension(t *testing.T) {
	rootDir := t.TempDir()

	// Create plugin directory with .yml manifest
	pluginDir := filepath.Join(rootDir, "yml-plugin")
	if err := os.Mkdir(pluginDir, 0755); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}

	manifestContent := `name: yml-plugin
version: 1.0.0
engine: native`
	manifestPath := filepath.Join(pluginDir, "manifest.yml")
	if err := os.WriteFile(manifestPath, []byte(manifestContent), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	discovery := NewFSDiscovery(rootDir)
	plugins, err := discovery.Discover()
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}

	if len(plugins) != 1 {
		t.Errorf("Discover() should find 1 plugin with .yml extension, got %d", len(plugins))
	}
}

func TestFSDiscovery_JSONManifest(t *testing.T) {
	rootDir := t.TempDir()

	// Create plugin directory with JSON manifest
	pluginDir := filepath.Join(rootDir, "json-plugin")
	if err := os.Mkdir(pluginDir, 0755); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}

	manifestContent := `{
		"name": "json-plugin",
		"version": "1.0.0",
		"description": "A JSON plugin",
		"author": "test",
		"license": "MIT",
		"engine": "native"
	}`
	manifestPath := filepath.Join(pluginDir, "manifest.json")
	if err := os.WriteFile(manifestPath, []byte(manifestContent), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	discovery := NewFSDiscovery(rootDir)
	plugins, err := discovery.Discover()
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}

	if len(plugins) != 1 {
		t.Errorf("Discover() should find 1 plugin, got %d", len(plugins))
	}

	if _, ok := plugins["json-plugin"]; !ok {
		t.Error("Discover() should find json-plugin")
	}
}

func TestFSDiscovery_PrefersYAMLOverJSON(t *testing.T) {
	rootDir := t.TempDir()

	// Create plugin directory with both YAML and JSON manifests
	pluginDir := filepath.Join(rootDir, "mixed-plugin")
	if err := os.Mkdir(pluginDir, 0755); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}

	// Create both manifest files
	yamlPath := filepath.Join(pluginDir, "manifest.yaml")
	if err := os.WriteFile(yamlPath, []byte("name: mixed-plugin\nversion: 1.0.0\nengine: native"), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	jsonPath := filepath.Join(pluginDir, "manifest.json")
	if err := os.WriteFile(jsonPath, []byte(`{"name": "mixed-plugin", "version": "2.0.0", "engine": "native"}`), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	discovery := NewFSDiscovery(rootDir)
	plugins, err := discovery.Discover()
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}

	// Should return YAML path (preferred)
	path := plugins["mixed-plugin"]
	if filepath.Ext(path) != ".yaml" {
		t.Errorf("Discover() should prefer YAML manifest, got %s", path)
	}
}

func TestFSDiscovery_NestedDirectories(t *testing.T) {
	rootDir := t.TempDir()

	// Create nested plugin directories
	for _, name := range []string{"plugin-a", "plugin-b", "plugin-c"} {
		pluginDir := filepath.Join(rootDir, name)
		if err := os.Mkdir(pluginDir, 0755); err != nil {
			t.Fatalf("Mkdir() error = %v", err)
		}
		manifestPath := filepath.Join(pluginDir, "manifest.yaml")
		content := "name: " + name + "\nversion: 1.0.0\nengine: native"
		if err := os.WriteFile(manifestPath, []byte(content), 0644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
	}

	// Also create a nested subdirectory within a plugin (should not be treated as plugin)
	nestedDir := filepath.Join(rootDir, "plugin-a", "nested")
	if err := os.Mkdir(nestedDir, 0755); err != nil {
		t.Fatalf("Mkdir() nested error = %v", err)
	}

	// Create a file in root (should be skipped)
	filePath := filepath.Join(rootDir, "somefile.txt")
	if err := os.WriteFile(filePath, []byte("not a plugin"), 0644); err != nil {
		t.Fatalf("WriteFile() file error = %v", err)
	}

	discovery := NewFSDiscovery(rootDir)
	plugins, err := discovery.Discover()
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}

	// Should find 3 plugins
	if len(plugins) != 3 {
		t.Errorf("Discover() should find 3 plugins, got %d: %v", len(plugins), plugins)
	}

	for _, name := range []string{"plugin-a", "plugin-b", "plugin-c"} {
		if _, ok := plugins[name]; !ok {
			t.Errorf("Discover() should find %s", name)
		}
	}
}

func TestFSDiscovery_DiscoverOne(t *testing.T) {
	rootDir := t.TempDir()

	// Create plugin directory
	pluginDir := filepath.Join(rootDir, "single-plugin")
	if err := os.Mkdir(pluginDir, 0755); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	manifestPath := filepath.Join(pluginDir, "manifest.yaml")
	if err := os.WriteFile(manifestPath, []byte("name: single-plugin\nversion: 1.0.0\nengine: native"), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	discovery := NewFSDiscovery(rootDir)

	// Test successful discovery
	path, err := discovery.DiscoverOne("single-plugin")
	if err != nil {
		t.Fatalf("DiscoverOne() error = %v", err)
	}

	if !filepath.IsAbs(path) {
		t.Errorf("DiscoverOne() should return absolute path, got %s", path)
	}

	// Test non-existent plugin
	_, err = discovery.DiscoverOne("nonexistent")
	if err == nil {
		t.Error("DiscoverOne() should fail for non-existent plugin")
	}
}

func TestFSDiscover_DiscoverOne_DirectoryIsFile(t *testing.T) {
	rootDir := t.TempDir()

	// Create a file named like a plugin
	filePath := filepath.Join(rootDir, "file-plugin")
	if err := os.WriteFile(filePath, []byte("not a directory"), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	discovery := NewFSDiscovery(rootDir)
	_, err := discovery.DiscoverOne("file-plugin")
	if err == nil {
		t.Error("DiscoverOne() should fail when plugin path is a file")
	}
}

func TestFSDiscover_DiscoverOne_MissingManifest(t *testing.T) {
	rootDir := t.TempDir()

	// Create plugin directory without manifest
	pluginDir := filepath.Join(rootDir, "empty-plugin")
	if err := os.Mkdir(pluginDir, 0755); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}

	discovery := NewFSDiscovery(rootDir)
	_, err := discovery.DiscoverOne("empty-plugin")
	if err == nil {
		t.Error("DiscoverOne() should fail when manifest is missing")
	}
}

// =============================================================================
// LoadAndRegister Tests
// =============================================================================

func TestLoadAndRegister_NewPlugins(t *testing.T) {
	rootDir := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "test.db")

	// Create discovery directory with plugins
	for _, name := range []string{"plugin-alpha", "plugin-beta"} {
		pluginDir := filepath.Join(rootDir, name)
		if err := os.Mkdir(pluginDir, 0755); err != nil {
			t.Fatalf("Mkdir() error = %v", err)
		}
		manifestPath := filepath.Join(pluginDir, "manifest.yaml")
		content := "name: " + name + "\nversion: 1.0.0\nengine: native"
		if err := os.WriteFile(manifestPath, []byte(content), 0644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
	}

	reg, err := NewBoltRegistry(dbPath)
	if err != nil {
		t.Fatalf("NewBoltRegistry() error = %v", err)
	}
	defer reg.Close()

	discovery := NewFSDiscovery(rootDir)

	count, err := LoadAndRegister(reg, discovery)
	if err != nil {
		t.Fatalf("LoadAndRegister() error = %v", err)
	}

	if count != 2 {
		t.Errorf("LoadAndRegister() should register 2 plugins, got %d", count)
	}

	// Verify plugins are registered
	for _, name := range []string{"plugin-alpha", "plugin-beta"} {
		entry, err := reg.Get(name)
		if err != nil {
			t.Errorf("Get(%s) error = %v", name, err)
			continue
		}
		if !entry.Enabled {
			t.Errorf("Plugin %s should be enabled", name)
		}
		if entry.Manifest.Version != "1.0.0" {
			t.Errorf("Plugin %s version = %s, want 1.0.0", name, entry.Manifest.Version)
		}
	}
}

func TestLoadAndRegister_NameMismatch(t *testing.T) {
	rootDir := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "test.db")

	// Create plugin with mismatched name
	pluginDir := filepath.Join(rootDir, "plugin-dir-name")
	if err := os.Mkdir(pluginDir, 0755); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}

	// Manifest name differs from directory name
	manifestContent := "name: different-plugin-name\nversion: 1.0.0\nengine: native"
	manifestPath := filepath.Join(pluginDir, "manifest.yaml")
	if err := os.WriteFile(manifestPath, []byte(manifestContent), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	reg, err := NewBoltRegistry(dbPath)
	if err != nil {
		t.Fatalf("NewBoltRegistry() error = %v", err)
	}
	defer reg.Close()

	discovery := NewFSDiscovery(rootDir)

	count, err := LoadAndRegister(reg, discovery)
	if err != nil {
		t.Fatalf("LoadAndRegister() error = %v", err)
	}

	// Plugin should not be registered due to name mismatch
	if count != 0 {
		t.Errorf("LoadAndRegister() should register 0 plugins with name mismatch, got %d", count)
	}

	// Verify plugin is not registered
	_, err = reg.Get("different-plugin-name")
	if err == nil {
		t.Error("Plugin with mismatched name should not be registered")
	}
}

func TestLoadAndRegister_InvalidManifest(t *testing.T) {
	rootDir := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "test.db")

	// Create plugin with invalid manifest (missing version)
	pluginDir := filepath.Join(rootDir, "invalid-plugin")
	if err := os.Mkdir(pluginDir, 0755); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}

	manifestContent := "name: invalid-plugin\nengine: native" // missing version
	manifestPath := filepath.Join(pluginDir, "manifest.yaml")
	if err := os.WriteFile(manifestPath, []byte(manifestContent), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	reg, err := NewBoltRegistry(dbPath)
	if err != nil {
		t.Fatalf("NewBoltRegistry() error = %v", err)
	}
	defer reg.Close()

	discovery := NewFSDiscovery(rootDir)

	count, err := LoadAndRegister(reg, discovery)
	if err != nil {
		t.Fatalf("LoadAndRegister() error = %v", err)
	}

	// Invalid manifest should be skipped
	if count != 0 {
		t.Errorf("LoadAndRegister() should register 0 plugins with invalid manifest, got %d", count)
	}
}

func TestLoadAndRegister_ExistingPluginSameVersion(t *testing.T) {
	rootDir := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "test.db")

	// Create plugin
	pluginDir := filepath.Join(rootDir, "existing-plugin")
	if err := os.Mkdir(pluginDir, 0755); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	manifestPath := filepath.Join(pluginDir, "manifest.yaml")
	if err := os.WriteFile(manifestPath, []byte("name: existing-plugin\nversion: 1.0.0\nengine: native"), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	reg, err := NewBoltRegistry(dbPath)
	if err != nil {
		t.Fatalf("NewBoltRegistry() error = %v", err)
	}
	defer reg.Close()

	// Pre-register the plugin
	entry := &PluginEntry{
		Manifest: testManifest("existing-plugin", "1.0.0"),
		Enabled:  true,
	}
	if err := reg.Register(entry); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	// Disable it to test state preservation
	if err := reg.Disable("existing-plugin"); err != nil {
		t.Fatalf("Disable() error = %v", err)
	}

	discovery := NewFSDiscovery(rootDir)

	count, err := LoadAndRegister(reg, discovery)
	if err != nil {
		t.Fatalf("LoadAndRegister() error = %v", err)
	}

	if count != 1 {
		t.Errorf("LoadAndRegister() should count existing plugin, got %d", count)
	}

	// Verify state is preserved (disabled)
	retrieved, err := reg.Get("existing-plugin")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if retrieved.Enabled {
		t.Error("Existing disabled plugin state should be preserved")
	}
}

func TestLoadAndRegister_ExistingPluginUpgrade(t *testing.T) {
	rootDir := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "test.db")

	// Create plugin with new version
	pluginDir := filepath.Join(rootDir, "upgrade-plugin")
	if err := os.Mkdir(pluginDir, 0755); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	manifestPath := filepath.Join(pluginDir, "manifest.yaml")
	if err := os.WriteFile(manifestPath, []byte("name: upgrade-plugin\nversion: 2.0.0\nengine: native"), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	reg, err := NewBoltRegistry(dbPath)
	if err != nil {
		t.Fatalf("NewBoltRegistry() error = %v", err)
	}
	defer reg.Close()

	// Pre-register with old version
	entry := &PluginEntry{
		Manifest: testManifest("upgrade-plugin", "1.0.0"),
		Enabled:  true,
	}
	if err := reg.Register(entry); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	// Disable it to test state preservation during upgrade
	if err := reg.Disable("upgrade-plugin"); err != nil {
		t.Fatalf("Disable() error = %v", err)
	}

	discovery := NewFSDiscovery(rootDir)

	count, err := LoadAndRegister(reg, discovery)
	if err != nil {
		t.Fatalf("LoadAndRegister() error = %v", err)
	}

	if count != 1 {
		t.Errorf("LoadAndRegister() should count upgraded plugin, got %d", count)
	}

	// Verify version was upgraded
	retrieved, err := reg.Get("upgrade-plugin")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if retrieved.Manifest.Version != "2.0.0" {
		t.Errorf("Plugin version should be upgraded to 2.0.0, got %s", retrieved.Manifest.Version)
	}

	// State should be preserved (disabled)
	if retrieved.Enabled {
		t.Error("Disabled state should be preserved during upgrade")
	}
}

func TestLoadAndRegister_DiscoveredPathStored(t *testing.T) {
	rootDir := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "test.db")

	pluginDir := filepath.Join(rootDir, "path-plugin")
	if err := os.Mkdir(pluginDir, 0755); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	manifestPath := filepath.Join(pluginDir, "manifest.yaml")
	if err := os.WriteFile(manifestPath, []byte("name: path-plugin\nversion: 1.0.0\nengine: native"), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	reg, err := NewBoltRegistry(dbPath)
	if err != nil {
		t.Fatalf("NewBoltRegistry() error = %v", err)
	}
	defer reg.Close()

	discovery := NewFSDiscovery(rootDir)

	count, err := LoadAndRegister(reg, discovery)
	if err != nil {
		t.Fatalf("LoadAndRegister() error = %v", err)
	}

	if count != 1 {
		t.Errorf("LoadAndRegister() should register 1 plugin, got %d", count)
	}

	retrieved, err := reg.Get("path-plugin")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if retrieved.DiscoveredPath == "" {
		t.Error("DiscoveredPath should be stored")
	}

	if !filepath.IsAbs(retrieved.DiscoveredPath) {
		t.Errorf("DiscoveredPath should be absolute, got %s", retrieved.DiscoveredPath)
	}
}

// =============================================================================
// Concurrent Access Tests (Additional)
// =============================================================================

func TestBoltRegistry_ConcurrentReadWrite(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	reg, err := NewBoltRegistry(dbPath)
	if err != nil {
		t.Fatalf("NewBoltRegistry() error = %v", err)
	}
	defer reg.Close()

	// Pre-register some plugins
	for i := 0; i < 5; i++ {
		entry := &PluginEntry{
			Manifest: testManifest("initial-"+string(rune('0'+i)), "1.0.0"),
			Enabled:  true,
		}
		if err := reg.Register(entry); err != nil {
			t.Fatalf("Register() error = %v", err)
		}
	}

	var wg sync.WaitGroup
	errCh := make(chan error, 50)

	// Concurrent reads and writes
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			switch idx % 5 {
			case 0: // Register new
				entry := &PluginEntry{
					Manifest: testManifest("new-"+string(rune('a'+idx%26)), "1.0.0"),
					Enabled:  true,
				}
				reg.Register(entry) // May fail if duplicate, that's okay
			case 1: // Get
				_, err := reg.Get("initial-0")
				if err != nil {
					errCh <- err
				}
			case 2: // List
				_, err := reg.List()
				if err != nil {
					errCh <- err
				}
			case 3: // Enable/Disable
				if err := reg.Disable("initial-1"); err != nil {
					errCh <- err
				}
				if err := reg.Enable("initial-1"); err != nil {
					errCh <- err
				}
			case 4: // Update
				reg.Update("initial-2", testManifest("initial-2", "2.0.0"))
			}
		}(i)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Errorf("Concurrent operation error = %v", err)
	}
}

func TestBoltRegistry_ConcurrentEnableDisable(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	reg, err := NewBoltRegistry(dbPath)
	if err != nil {
		t.Fatalf("NewBoltRegistry() error = %v", err)
	}
	defer reg.Close()

	// Register plugin
	entry := &PluginEntry{
		Manifest: testManifest("concurrent-state", "1.0.0"),
		Enabled:  true,
	}
	if err := reg.Register(entry); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	var wg sync.WaitGroup
	errCh := make(chan error, 100)

	// Concurrent Enable/Disable on same plugin
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			if idx%2 == 0 {
				if err := reg.Enable("concurrent-state"); err != nil {
					errCh <- err
				}
			} else {
				if err := reg.Disable("concurrent-state"); err != nil {
					errCh <- err
				}
			}
		}(i)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Errorf("Concurrent Enable/Disable error = %v", err)
	}

	// Verify final state is consistent (either enabled or disabled)
	retrieved, err := reg.Get("concurrent-state")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	// State should be valid (true or false, not corrupted)
	if retrieved.Manifest.Name != "concurrent-state" {
		t.Error("Plugin state corrupted after concurrent operations")
	}
}

// =============================================================================
// Plugin Entry Validation Tests
// =============================================================================

func TestPluginEntry_InvalidName(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	reg, err := NewBoltRegistry(dbPath)
	if err != nil {
		t.Fatalf("NewBoltRegistry() error = %v", err)
	}
	defer reg.Close()

	// Test various invalid names
	invalidNames := []string{
		"",            // empty
		"1plugin",     // starts with number
		"PLUGIN",      // uppercase
		"plugin_name", // underscore
		"plugin.name", // dot
		"-plugin",     // starts with hyphen
	}

	for _, name := range invalidNames {
		entry := &PluginEntry{
			Manifest: core.Manifest{
				Name:    name,
				Version: "1.0.0",
				Engine:  "native",
			},
			Enabled: true,
		}

		if err := reg.Register(entry); err == nil {
			t.Errorf("Register() should fail for invalid name %q", name)
		}
	}
}

func TestPluginEntry_InvalidVersion(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	reg, err := NewBoltRegistry(dbPath)
	if err != nil {
		t.Fatalf("NewBoltRegistry() error = %v", err)
	}
	defer reg.Close()

	invalidVersions := []string{
		"",       // empty
		"1",      // incomplete
		"1.2",    // incomplete
		"v1.0.0", // v prefix
		"1.0.0a", // non-numeric
		"a.b.c",  // non-numeric
	}

	for _, version := range invalidVersions {
		entry := &PluginEntry{
			Manifest: core.Manifest{
				Name:    "test-plugin",
				Version: version,
				Engine:  "native",
			},
			Enabled: true,
		}

		if err := reg.Register(entry); err == nil {
			t.Errorf("Register() should fail for invalid version %q", version)
		}
	}
}

func TestPluginEntry_InvalidEngine(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	reg, err := NewBoltRegistry(dbPath)
	if err != nil {
		t.Fatalf("NewBoltRegistry() error = %v", err)
	}
	defer reg.Close()

	invalidEngines := []string{
		"",        // empty
		"unknown", // invalid
		"docker",  // invalid
		"container",
	}

	for _, engine := range invalidEngines {
		entry := &PluginEntry{
			Manifest: core.Manifest{
				Name:    "test-plugin",
				Version: "1.0.0",
				Engine:  engine,
			},
			Enabled: true,
		}

		if err := reg.Register(entry); err == nil {
			t.Errorf("Register() should fail for invalid engine %q", engine)
		}
	}
}

func TestPluginEntry_DuplicateProvides(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	reg, err := NewBoltRegistry(dbPath)
	if err != nil {
		t.Fatalf("NewBoltRegistry() error = %v", err)
	}
	defer reg.Close()

	entry := &PluginEntry{
		Manifest: core.Manifest{
			Name:     "duplicate-plugin",
			Version:  "1.0.0",
			Engine:   "native",
			Provides: []string{"capability", "capability"}, // duplicate
		},
		Enabled: true,
	}

	if err := reg.Register(entry); err == nil {
		t.Error("Register() should fail for duplicate provides")
	}
}

func TestPluginEntry_InvalidDependency(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	reg, err := NewBoltRegistry(dbPath)
	if err != nil {
		t.Fatalf("NewBoltRegistry() error = %v", err)
	}
	defer reg.Close()

	invalidDependencies := []string{
		"",     // empty
		"dep@", // empty version
	}

	for _, dep := range invalidDependencies {
		entry := &PluginEntry{
			Manifest: core.Manifest{
				Name:     "test-plugin",
				Version:  "1.0.0",
				Engine:   "native",
				Requires: []string{dep},
			},
			Enabled: true,
		}

		if err := reg.Register(entry); err == nil {
			t.Errorf("Register() should fail for invalid dependency %q", dep)
		}
	}
}

// =============================================================================
// Error Helper Tests
// =============================================================================

func TestIsPluginNotFound(t *testing.T) {
	// Test ErrPluginNotFound
	if !IsPluginNotFound(ErrPluginNotFound) {
		t.Error("IsPluginNotFound should return true for ErrPluginNotFound")
	}

	// Test wrapped error
	wrapped := &PluginError{PluginName: "test", Op: "get", Msg: "plugin not found"}
	if !IsPluginNotFound(wrapped) {
		t.Error("IsPluginNotFound should return true for wrapped ErrPluginNotFound")
	}

	// Test other error
	other := &PluginError{PluginName: "test", Op: "register", Msg: "plugin already exists"}
	if IsPluginNotFound(other) {
		t.Error("IsPluginNotFound should return false for other PluginError")
	}

	// Test non-PluginError
	if IsPluginNotFound(errors.New("other error")) {
		t.Error("IsPluginNotFound should return false for non-PluginError")
	}
}

func TestIsPluginExists(t *testing.T) {
	// Test ErrPluginExists
	if !IsPluginExists(ErrPluginExists) {
		t.Error("IsPluginExists should return true for ErrPluginExists")
	}

	// Test wrapped error
	wrapped := &PluginError{PluginName: "test", Op: "register", Msg: "plugin already exists"}
	if !IsPluginExists(wrapped) {
		t.Error("IsPluginExists should return true for wrapped ErrPluginExists")
	}

	// Test other error
	other := &PluginError{PluginName: "test", Op: "get", Msg: "plugin not found"}
	if IsPluginExists(other) {
		t.Error("IsPluginExists should return false for other PluginError")
	}

	// Test non-PluginError
	if IsPluginExists(errors.New("other error")) {
		t.Error("IsPluginExists should return false for non-PluginError")
	}
}

func TestPluginError_ErrorFormatting(t *testing.T) {
	// Test with plugin name
	err := &PluginError{PluginName: "my-plugin", Op: "register", Msg: "failed"}
	expected := "registry: register plugin my-plugin: failed"
	if err.Error() != expected {
		t.Errorf("Error() = %q, want %q", err.Error(), expected)
	}

	// Test without plugin name
	err2 := &PluginError{PluginName: "", Op: "close", Msg: "database error"}
	expected2 := "registry: close: database error"
	if err2.Error() != expected2 {
		t.Errorf("Error() = %q, want %q", err2.Error(), expected2)
	}
}

// =============================================================================
// ValidateManifest Function Tests
// =============================================================================

func TestValidateManifest_ValidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	manifestPath := filepath.Join(tmpDir, "manifest.yaml")

	content := "name: valid-plugin\nversion: 1.0.0\nengine: native"
	if err := os.WriteFile(manifestPath, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if err := ValidateManifest(manifestPath); err != nil {
		t.Errorf("ValidateManifest() error = %v", err)
	}
}

func TestValidateManifest_ValidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	manifestPath := filepath.Join(tmpDir, "manifest.json")

	content := `{"name": "valid-plugin", "version": "1.0.0", "engine": "native"}`
	if err := os.WriteFile(manifestPath, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if err := ValidateManifest(manifestPath); err != nil {
		t.Errorf("ValidateManifest() error = %v", err)
	}
}

func TestValidateManifest_InvalidContent(t *testing.T) {
	tmpDir := t.TempDir()
	manifestPath := filepath.Join(tmpDir, "manifest.yaml")

	// Missing required fields
	content := "name: invalid-plugin"
	if err := os.WriteFile(manifestPath, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if err := ValidateManifest(manifestPath); err == nil {
		t.Error("ValidateManifest() should fail for invalid content")
	}
}

func TestValidateManifest_MalformedYAML(t *testing.T) {
	tmpDir := t.TempDir()
	manifestPath := filepath.Join(tmpDir, "manifest.yaml")

	// Invalid YAML syntax
	content := "name: malformed\n  invalid: yaml: syntax"
	if err := os.WriteFile(manifestPath, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if err := ValidateManifest(manifestPath); err == nil {
		t.Error("ValidateManifest() should fail for malformed YAML")
	}
}

func TestValidateManifest_MalformedJSON(t *testing.T) {
	tmpDir := t.TempDir()
	manifestPath := filepath.Join(tmpDir, "manifest.json")

	// Invalid JSON syntax
	content := `{name: "malformed", version: "1.0.0"}` // missing quotes
	if err := os.WriteFile(manifestPath, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if err := ValidateManifest(manifestPath); err == nil {
		t.Error("ValidateManifest() should fail for malformed JSON")
	}
}

func TestValidateManifest_FileNotFound(t *testing.T) {
	manifestPath := filepath.Join(t.TempDir(), "nonexistent.yaml")

	if err := ValidateManifest(manifestPath); err == nil {
		t.Error("ValidateManifest() should fail for non-existent file")
	}
}

// =============================================================================
// Edge Cases
// =============================================================================

func TestBoltRegistry_EmptyName(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	reg, err := NewBoltRegistry(dbPath)
	if err != nil {
		t.Fatalf("NewBoltRegistry() error = %v", err)
	}
	defer reg.Close()

	// Get with empty name
	_, err = reg.Get("")
	if err == nil {
		t.Error("Get() should fail for empty name")
	}

	// Remove with empty name
	err = reg.Remove("")
	if err == nil {
		t.Error("Remove() should fail for empty name")
	}

	// Enable with empty name
	err = reg.Enable("")
	if err == nil {
		t.Error("Enable() should fail for empty name")
	}

	// Disable with empty name
	err = reg.Disable("")
	if err == nil {
		t.Error("Disable() should fail for empty name")
	}

	// Update with empty name
	err = reg.Update("", testManifest("test", "1.0.0"))
	if err == nil {
		t.Error("Update() should fail for empty name")
	}
}

func TestBoltRegistry_SpecialCharactersInName(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	reg, err := NewBoltRegistry(dbPath)
	if err != nil {
		t.Fatalf("NewBoltRegistry() error = %v", err)
	}
	defer reg.Close()

	// Valid name with hyphens (should succeed)
	entry := &PluginEntry{
		Manifest: testManifest("my-test-plugin", "1.0.0"),
		Enabled:  true,
	}
	if err := reg.Register(entry); err != nil {
		t.Errorf("Register() should succeed for valid hyphenated name: %v", err)
	}

	// Verify retrieval
	retrieved, err := reg.Get("my-test-plugin")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if retrieved.Manifest.Name != "my-test-plugin" {
		t.Errorf("Name mismatch = %s", retrieved.Manifest.Name)
	}
}

func TestBoltRegistry_UpdatePreservesDiscoveredPath(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	reg, err := NewBoltRegistry(dbPath)
	if err != nil {
		t.Fatalf("NewBoltRegistry() error = %v", err)
	}
	defer reg.Close()

	// Register with discovered path
	entry := &PluginEntry{
		Manifest:       testManifest("path-plugin", "1.0.0"),
		Enabled:        true,
		DiscoveredPath: "/original/path",
	}
	if err := reg.Register(entry); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	// Update manifest
	newManifest := testManifest("path-plugin", "2.0.0")
	if err := reg.Update("path-plugin", newManifest); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	// Verify DiscoveredPath is preserved (update preserves entry, just changes manifest)
	retrieved, err := reg.Get("path-plugin")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	// Note: Update implementation preserves the existing entry's state
	// and only modifies the manifest. DiscoveredPath should remain.
	if retrieved.DiscoveredPath != "/original/path" {
		t.Errorf("Update should preserve DiscoveredPath, got %s", retrieved.DiscoveredPath)
	}
}

// =============================================================================
// ManifestsEqual Tests
// =============================================================================

func TestManifestsEqual_Equal(t *testing.T) {
	a := &core.Manifest{
		Name:        "test",
		Version:     "1.0.0",
		Description: "desc",
		Author:      "author",
		License:     "MIT",
		Engine:      "native",
		Requires:    []string{"dep@1.0.0"},
		Provides:    []string{"cap1", "cap2"},
	}

	b := &core.Manifest{
		Name:        "test",
		Version:     "1.0.0",
		Description: "desc",
		Author:      "author",
		License:     "MIT",
		Engine:      "native",
		Requires:    []string{"dep@1.0.0"},
		Provides:    []string{"cap1", "cap2"},
	}

	if !manifestsEqual(a, b) {
		t.Error("manifestsEqual should return true for equal manifests")
	}
}

func TestManifestsEqual_DifferentName(t *testing.T) {
	a := &core.Manifest{Name: "plugin-a", Version: "1.0.0", Engine: "native"}
	b := &core.Manifest{Name: "plugin-b", Version: "1.0.0", Engine: "native"}

	if manifestsEqual(a, b) {
		t.Error("manifestsEqual should return false for different names")
	}
}

func TestManifestsEqual_DifferentVersion(t *testing.T) {
	a := &core.Manifest{Name: "test", Version: "1.0.0", Engine: "native"}
	b := &core.Manifest{Name: "test", Version: "2.0.0", Engine: "native"}

	if manifestsEqual(a, b) {
		t.Error("manifestsEqual should return false for different versions")
	}
}

func TestManifestsEqual_DifferentDescription(t *testing.T) {
	a := &core.Manifest{Name: "test", Version: "1.0.0", Description: "desc a", Engine: "native"}
	b := &core.Manifest{Name: "test", Version: "1.0.0", Description: "desc b", Engine: "native"}

	if manifestsEqual(a, b) {
		t.Error("manifestsEqual should return false for different descriptions")
	}
}

func TestManifestsEqual_DifferentRequiresLength(t *testing.T) {
	a := &core.Manifest{Name: "test", Version: "1.0.0", Engine: "native", Requires: []string{"a"}}
	b := &core.Manifest{Name: "test", Version: "1.0.0", Engine: "native", Requires: []string{"a", "b"}}

	if manifestsEqual(a, b) {
		t.Error("manifestsEqual should return false for different requires length")
	}
}

func TestManifestsEqual_DifferentProvidesLength(t *testing.T) {
	a := &core.Manifest{Name: "test", Version: "1.0.0", Engine: "native", Provides: []string{"a"}}
	b := &core.Manifest{Name: "test", Version: "1.0.0", Engine: "native", Provides: []string{"a", "b"}}

	if manifestsEqual(a, b) {
		t.Error("manifestsEqual should return false for different provides length")
	}
}

func TestManifestsEqual_DifferentRequiresContent(t *testing.T) {
	a := &core.Manifest{Name: "test", Version: "1.0.0", Engine: "native", Requires: []string{"a@1.0.0", "b@1.0.0"}}
	b := &core.Manifest{Name: "test", Version: "1.0.0", Engine: "native", Requires: []string{"a@1.0.0", "c@1.0.0"}}

	if manifestsEqual(a, b) {
		t.Error("manifestsEqual should return false for different requires content")
	}
}

func TestManifestsEqual_DifferentProvidesContent(t *testing.T) {
	a := &core.Manifest{Name: "test", Version: "1.0.0", Engine: "native", Provides: []string{"cap1", "cap2"}}
	b := &core.Manifest{Name: "test", Version: "1.0.0", Engine: "native", Provides: []string{"cap1", "cap3"}}

	if manifestsEqual(a, b) {
		t.Error("manifestsEqual should return false for different provides content")
	}
}

func TestManifestsEqual_DifferentEngine(t *testing.T) {
	a := &core.Manifest{Name: "test", Version: "1.0.0", Engine: "native"}
	b := &core.Manifest{Name: "test", Version: "1.0.0", Engine: "wasm"}

	if manifestsEqual(a, b) {
		t.Error("manifestsEqual should return false for different engines")
	}
}

// =============================================================================
// Registry Creation Error Tests
// =============================================================================

func TestNewBoltRegistry_InvalidPath(t *testing.T) {
	// Try to create registry in a path that's a file
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "existing-file")
	if err := os.WriteFile(filePath, []byte("test"), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	// Try to create registry where a file exists
	_, err := NewBoltRegistry(filePath)
	if err == nil {
		t.Error("NewBoltRegistry() should fail when path is existing file")
	}
}

func TestNewBoltRegistry_BucketCreation(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	reg, err := NewBoltRegistry(dbPath)
	if err != nil {
		t.Fatalf("NewBoltRegistry() error = %v", err)
	}
	defer reg.Close()

	// Verify bucket was created
	err = reg.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(BucketName))
		if bucket == nil {
			return errors.New("bucket not found")
		}
		return nil
	})

	if err != nil {
		t.Errorf("Bucket should be created: %v", err)
	}
}

// =============================================================================
// State Persistence Stress Test
// =============================================================================

func TestBoltRegistry_StatePersistenceStress(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "stress.db")

	// Perform many state changes
	reg, err := NewBoltRegistry(dbPath)
	if err != nil {
		t.Fatalf("NewBoltRegistry() error = %v", err)
	}

	// Register multiple plugins
	for i := 0; i < 10; i++ {
		entry := &PluginEntry{
			Manifest: testManifest("stress-"+string(rune('0'+i)), "1.0.0"),
			Enabled:  true,
		}
		if err := reg.Register(entry); err != nil {
			t.Fatalf("Register() error = %v", err)
		}
	}

	// Randomly enable/disable
	for i := 0; i < 50; i++ {
		name := "stress-" + string(rune('0'+i%10))
		if i%2 == 0 {
			reg.Enable(name)
		} else {
			reg.Disable(name)
		}
	}

	// Capture final states
	finalStates := make(map[string]bool)
	for i := 0; i < 10; i++ {
		name := "stress-" + string(rune('0'+i))
		entry, err := reg.Get(name)
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}
		finalStates[name] = entry.Enabled
	}

	// Close and reopen
	reg.Close()

	reg2, err := NewBoltRegistry(dbPath)
	if err != nil {
		t.Fatalf("NewBoltRegistry() reopen error = %v", err)
	}
	defer reg2.Close()

	// Verify all states persisted
	for i := 0; i < 10; i++ {
		name := "stress-" + string(rune('0'+i))
		entry, err := reg2.Get(name)
		if err != nil {
			t.Fatalf("Get() after reopen error = %v", err)
		}
		if entry.Enabled != finalStates[name] {
			t.Errorf("State for %s did not persist: got %v, want %v", name, entry.Enabled, finalStates[name])
		}
	}
}

func TestPluginError_Unwrap(t *testing.T) {
	err := &PluginError{PluginName: "test", Op: "get", Msg: "plugin not found"}
	if err.Unwrap() != nil {
		t.Error("Unwrap should return nil for PluginError")
	}
}

func TestLoadAndRegister_DiscoveryError(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	reg, err := NewBoltRegistry(dbPath)
	if err != nil {
		t.Fatalf("NewBoltRegistry() error = %v", err)
	}
	defer reg.Close()

	discovery := &mockDiscovery{err: errors.New("discovery failed")}
	count, err := LoadAndRegister(reg, discovery)
	if err == nil {
		t.Error("LoadAndRegister should return error when discovery fails")
	}
	if count != 0 {
		t.Errorf("LoadAndRegister should return 0 count on error, got %d", count)
	}
}

func TestLoadAndRegister_RegistryClosed(t *testing.T) {
	rootDir := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "test.db")

	pluginDir := filepath.Join(rootDir, "closed-plugin")
	if err := os.Mkdir(pluginDir, 0755); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "manifest.yaml"), []byte("name: closed-plugin\nversion: 1.0.0\nengine: native"), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	reg, err := NewBoltRegistry(dbPath)
	if err != nil {
		t.Fatalf("NewBoltRegistry() error = %v", err)
	}

	discovery := NewFSDiscovery(rootDir)

	reg.Close()

	count, err := LoadAndRegister(reg, discovery)
	if err != nil {
		t.Fatalf("LoadAndRegister() should not return top-level error, got %v", err)
	}
	if count != 0 {
		t.Errorf("LoadAndRegister should return 0 when registry closed, got %d", count)
	}
}

func TestLoadAndRegister_ExistingPluginUpdateError(t *testing.T) {
	rootDir := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "test.db")

	pluginDir := filepath.Join(rootDir, "existing-plugin")
	if err := os.Mkdir(pluginDir, 0755); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "manifest.yaml"), []byte("name: existing-plugin\nversion: 2.0.0\nengine: native"), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	reg, err := NewBoltRegistry(dbPath)
	if err != nil {
		t.Fatalf("NewBoltRegistry() error = %v", err)
	}
	defer reg.Close()

	entry := &PluginEntry{Manifest: testManifest("existing-plugin", "1.0.0"), Enabled: true}
	if err := reg.Register(entry); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	discovery := NewFSDiscovery(rootDir)

	count, err := LoadAndRegister(reg, discovery)
	if err != nil {
		t.Fatalf("LoadAndRegister() error = %v", err)
	}
	if count != 1 {
		t.Errorf("LoadAndRegister should count updated plugin, got %d", count)
	}

	retrieved, err := reg.Get("existing-plugin")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if retrieved.Manifest.Version != "2.0.0" {
		t.Errorf("Version should be updated to 2.0.0, got %s", retrieved.Manifest.Version)
	}
}

type mockDiscovery struct {
	plugins map[string]string
	err     error
}

func (m *mockDiscovery) Discover() (map[string]string, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.plugins, nil
}

func TestManifestsEqual_DifferentAuthor(t *testing.T) {
	a := &core.Manifest{Name: "test", Version: "1.0.0", Author: "author-a", Engine: "native"}
	b := &core.Manifest{Name: "test", Version: "1.0.0", Author: "author-b", Engine: "native"}

	if manifestsEqual(a, b) {
		t.Error("manifestsEqual should return false for different authors")
	}
}

func TestManifestsEqual_DifferentLicense(t *testing.T) {
	a := &core.Manifest{Name: "test", Version: "1.0.0", License: "MIT", Engine: "native"}
	b := &core.Manifest{Name: "test", Version: "1.0.0", License: "Apache", Engine: "native"}

	if manifestsEqual(a, b) {
		t.Error("manifestsEqual should return false for different licenses")
	}
}

func TestNewBoltRegistry_MkdirAllError(t *testing.T) {
	_, err := NewBoltRegistry("/proc/nonexistent/registry/test.db")
	if err == nil {
		t.Error("NewBoltRegistry should fail when directory creation fails")
	}
}

func TestBoltRegistry_List_Empty(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	registry, err := NewBoltRegistry(dbPath)
	if err != nil {
		t.Fatalf("NewBoltRegistry() error = %v", err)
	}
	defer registry.Close()

	entries, err := registry.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("List() should return empty slice for empty registry, got %d", len(entries))
	}
}

func TestBoltRegistry_Update_InvalidManifest(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	registry, err := NewBoltRegistry(dbPath)
	if err != nil {
		t.Fatalf("NewBoltRegistry() error = %v", err)
	}
	defer registry.Close()

	entry := &PluginEntry{
		Manifest: core.Manifest{
			Name:    "test-plugin",
			Version: "1.0.0",
			Engine:  "native",
		},
	}
	if err := registry.Register(entry); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	invalidManifest := core.Manifest{
		Name:    "test-plugin",
		Version: "invalid-version",
		Engine:  "native",
	}
	err = registry.Update("test-plugin", invalidManifest)
	if err == nil {
		t.Error("Update() should fail with invalid manifest")
	}
}

func TestBoltRegistry_Enable_NotFound(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	registry, err := NewBoltRegistry(dbPath)
	if err != nil {
		t.Fatalf("NewBoltRegistry() error = %v", err)
	}
	defer registry.Close()

	err = registry.Enable("nonexistent")
	if err == nil {
		t.Error("Enable() should fail for nonexistent plugin")
	}
}

func TestBoltRegistry_Disable_NotFound(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	registry, err := NewBoltRegistry(dbPath)
	if err != nil {
		t.Fatalf("NewBoltRegistry() error = %v", err)
	}
	defer registry.Close()

	err = registry.Disable("nonexistent")
	if err == nil {
		t.Error("Disable() should fail for nonexistent plugin")
	}
}

func TestBoltRegistry_Get_DeserializeError(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	registry, err := NewBoltRegistry(dbPath)
	if err != nil {
		t.Fatalf("NewBoltRegistry() error = %v", err)
	}
	defer registry.Close()

	registry.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(BucketName))
		bucket.Put([]byte("bad-plugin"), []byte("not valid json"))
		return nil
	})

	_, err = registry.Get("bad-plugin")
	if err == nil {
		t.Error("Get() should fail for corrupted data")
	}
}

func TestBoltRegistry_List_DeserializeError(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	registry, err := NewBoltRegistry(dbPath)
	if err != nil {
		t.Fatalf("NewBoltRegistry() error = %v", err)
	}
	defer registry.Close()

	registry.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(BucketName))
		bucket.Put([]byte("bad-plugin"), []byte("not valid json"))
		return nil
	})

	_, err = registry.List()
	if err == nil {
		t.Error("List() should fail for corrupted data")
	}
}

func TestLoadAndRegister_DiscoverError(t *testing.T) {
	mockDisc := &mockDiscovery{err: errors.New("discover failed")}
	registry := &mockRegistryForLoadAndRegister{}

	count, err := LoadAndRegister(registry, mockDisc)
	if err == nil {
		t.Error("LoadAndRegister should fail when discovery fails")
	}
	if count != 0 {
		t.Errorf("count = %d, want 0", count)
	}
}

type mockRegistryForLoadAndRegister struct{}

func (m *mockRegistryForLoadAndRegister) Register(entry *PluginEntry) error { return nil }
func (m *mockRegistryForLoadAndRegister) Get(name string) (*PluginEntry, error) {
	return nil, errors.New("not found")
}
func (m *mockRegistryForLoadAndRegister) Update(name string, manifest core.Manifest) error {
	return nil
}
func (m *mockRegistryForLoadAndRegister) Remove(name string) error { return nil }
func (m *mockRegistryForLoadAndRegister) List() ([]*PluginEntry, error) {
	return nil, nil
}
func (m *mockRegistryForLoadAndRegister) Enable(name string) error  { return nil }
func (m *mockRegistryForLoadAndRegister) Disable(name string) error { return nil }
func (m *mockRegistryForLoadAndRegister) Close() error              { return nil }
