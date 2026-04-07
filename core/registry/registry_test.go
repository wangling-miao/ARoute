package registry

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/wangling-miao/aroute/core"
)

// testManifest creates a valid manifest for testing.
func testManifest(name, version string) core.Manifest {
	return core.Manifest{
		Name:        name,
		Version:     version,
		Description: "Test plugin",
		Author:      "test",
		License:     "MIT",
		Engine:      "native",
	}
}

// TestBoltRegistry_Register tests plugin registration.
func TestBoltRegistry_Register(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	defer os.Remove(dbPath)

	reg, err := NewBoltRegistry(dbPath)
	if err != nil {
		t.Fatalf("NewBoltRegistry() error = %v", err)
	}
	defer reg.Close()

	// Test successful registration
	entry := &PluginEntry{
		Manifest: testManifest("test-plugin", "1.0.0"),
		Enabled:  true,
	}

	if err := reg.Register(entry); err != nil {
		t.Errorf("Register() error = %v", err)
	}

	// Test duplicate registration (shouldfail)
	if err := reg.Register(entry); err == nil {
		t.Error("Register() should fail for duplicate plugin")
	}

	// Test nil entry
	if err := reg.Register(nil); err == nil {
		t.Error("Register() should fail for nil entry")
	}

	// Test invalid manifest
	invalidEntry := &PluginEntry{
		Manifest: core.Manifest{Name: ""}, // Missing required fields
	}
	if err := reg.Register(invalidEntry); err == nil {
		t.Error("Register() should fail for invalid manifest")
	}
}

// TestBoltRegistry_Get tests plugin retrieval.
func TestBoltRegistry_Get(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	defer os.Remove(dbPath)

	reg, err := NewBoltRegistry(dbPath)
	if err != nil {
		t.Fatalf("NewBoltRegistry() error = %v", err)
	}
	defer reg.Close()

	// Register test plugin
	entry := &PluginEntry{
		Manifest: testManifest("test-plugin", "1.0.0"),
		Enabled:  true,
	}
	if err := reg.Register(entry); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	// Test successful get
	retrieved, err := reg.Get("test-plugin")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if retrieved.Manifest.Name != "test-plugin" {
		t.Errorf("Get() returned wrong name = %v", retrieved.Manifest.Name)
	}

	if retrieved.Manifest.Version != "1.0.0" {
		t.Errorf("Get() returned wrong version = %v", retrieved.Manifest.Version)
	}

	if !retrieved.Enabled {
		t.Error("Get() returned disabled plugin")
	}

	// Test get non-existent plugin
	_, err = reg.Get("non-existent")
	if err == nil {
		t.Error("Get() should fail for non-existent plugin")
	}
}

// TestBoltRegistry_List tests listing all plugins.
func TestBoltRegistry_List(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	defer os.Remove(dbPath)

	reg, err := NewBoltRegistry(dbPath)
	if err != nil {
		t.Fatalf("NewBoltRegistry() error = %v", err)
	}
	defer reg.Close()

	// Test empty list
	entries, err := reg.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	if len(entries) != 0 {
		t.Errorf("List() should return empty slice, got %d entries", len(entries))
	}

	// Register multiple plugins
	for i := 0; i < 5; i++ {
		entry := &PluginEntry{
			Manifest: testManifest("plugin-"+string(rune('0'+i)), "1.0.0"),
			Enabled:  true,
		}
		if err := reg.Register(entry); err != nil {
			t.Fatalf("Register() error = %v", err)
		}
	}

	// Test list returns all plugins
	entries, err = reg.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	if len(entries) != 5 {
		t.Errorf("List() returned %d entries, want 5", len(entries))
	}
}

// TestBoltRegistry_Update tests manifest updates.
func TestBoltRegistry_Update(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	defer os.Remove(dbPath)

	reg, err := NewBoltRegistry(dbPath)
	if err != nil {
		t.Fatalf("NewBoltRegistry() error = %v", err)
	}
	defer reg.Close()

	// Register plugin - caller can set any initial state, but registry enforces enabled=true
	entry := &PluginEntry{
		Manifest: testManifest("test-plugin", "1.0.0"),
		Enabled:  false, // Caller sets this, but registry will override
	}
	if err := reg.Register(entry); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	// Verify registry enforces initial state to enabled=true per spec
	retrieved, err := reg.Get("test-plugin")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !retrieved.Enabled {
		t.Error("Register() should enforce initial state to enabled=true")
	}

	// Now disable the plugin first
	if err := reg.Disable("test-plugin"); err != nil {
		t.Fatalf("Disable() error = %v", err)
	}

	// Update manifest
	newManifest := testManifest("test-plugin", "2.0.0")
	newManifest.Description = "Updated plugin"

	if err := reg.Update("test-plugin", newManifest); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	// Verify updated manifest and preserved state (disabled should stay disabled)
	retrieved, err = reg.Get("test-plugin")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if retrieved.Manifest.Version != "2.0.0" {
		t.Errorf("Update() did not update version = %v", retrieved.Manifest.Version)
	}

	// State should be preserved (disabled)
	if retrieved.Enabled {
		t.Error("Update() should preserve disabled state")
	}

	// Test update non-existent plugin
	if err := reg.Update("non-existent", newManifest); err == nil {
		t.Error("Update() should fail for non-existent plugin")
	}
}

// TestBoltRegistry_Remove tests plugin removal.
func TestBoltRegistry_Remove(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	defer os.Remove(dbPath)

	reg, err := NewBoltRegistry(dbPath)
	if err != nil {
		t.Fatalf("NewBoltRegistry() error = %v", err)
	}
	defer reg.Close()

	// Register plugin
	entry := &PluginEntry{
		Manifest: testManifest("test-plugin", "1.0.0"),
		Enabled:  true,
	}
	if err := reg.Register(entry); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	// Remove plugin
	if err := reg.Remove("test-plugin"); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}

	// Verify removed
	_, err = reg.Get("test-plugin")
	if err == nil {
		t.Error("Get() should fail after Remove()")
	}

	// Test remove non-existent plugin
	if err := reg.Remove("non-existent"); err == nil {
		t.Error("Remove() should fail for non-existent plugin")
	}

	// Test re-register after removal - should reset state to enabled=true
	if err := reg.Register(entry); err != nil {
		t.Errorf("Register() after Remove() error = %v", err)
	}

	// Verify state is enabled after re-registration
	reEnabled, err := reg.Get("test-plugin")
	if err != nil {
		t.Fatalf("Get() after re-register error = %v", err)
	}
	if !reEnabled.Enabled {
		t.Error("Re-register should reset state to enabled=true")
	}
}

// TestBoltRegistry_EnableDisable tests enable/disable operations.
func TestBoltRegistry_EnableDisable(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	defer os.Remove(dbPath)

	reg, err := NewBoltRegistry(dbPath)
	if err != nil {
		t.Fatalf("NewBoltRegistry() error = %v", err)
	}
	defer reg.Close()

	// Register plugin - registry enforces initial state enabled=true
	entry := &PluginEntry{
		Manifest: testManifest("test-plugin", "1.0.0"),
		Enabled:  false, // Caller sets this, but registry will override to true
	}
	if err := reg.Register(entry); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	// Verify enabled=true was enforced
	retrieved, err := reg.Get("test-plugin")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !retrieved.Enabled {
		t.Error("Register() should enforce enabled=true")
	}

	// Enable plugin (already enabled - should be no-op)
	if err := reg.Enable("test-plugin"); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}

	// Disable plugin
	if err := reg.Disable("test-plugin"); err != nil {
		t.Fatalf("Disable() error = %v", err)
	}

	// Verify disabled
	var retrieved2 *PluginEntry
	retrieved2, err = reg.Get("test-plugin")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if retrieved2.Enabled {
		t.Error("Disable() did not disable plugin")
	}

	// Test enable/disable non-existent plugin
	if err := reg.Enable("non-existent"); err == nil {
		t.Error("Enable() should fail for non-existent plugin")
	}
	if err := reg.Disable("non-existent"); err == nil {
		t.Error("Disable() should fail for non-existent plugin")
	}
}

// TestBoltRegistry_Persistence tests that state persists across close/reopen.
func TestBoltRegistry_Persistence(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	defer os.Remove(dbPath)

	// Create and populate registry
	reg, err := NewBoltRegistry(dbPath)
	if err != nil {
		t.Fatalf("NewBoltRegistry() error = %v", err)
	}

	// Register and disable plugin
	entry := &PluginEntry{
		Manifest: testManifest("test-plugin", "1.0.0"),
		Enabled:  true,
	}
	if err := reg.Register(entry); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	if err := reg.Disable("test-plugin"); err != nil {
		t.Fatalf("Disable() error = %v", err)
	}

	// Close registry
	if err := reg.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	// Reopen registry
	reg2, err := NewBoltRegistry(dbPath)
	if err != nil {
		t.Fatalf("NewBoltRegistry() reopen error = %v", err)
	}
	defer reg2.Close()

	// Verify state persisted
	retrieved, err := reg2.Get("test-plugin")
	if err != nil {
		t.Fatalf("Get() after reopen error = %v", err)
	}

	if retrieved.Enabled {
		t.Error("Disabled state should persist after reopen")
	}

	// Test multiple state changes persist
	if err := reg2.Enable("test-plugin"); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}

	if err := reg2.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	reg3, err := NewBoltRegistry(dbPath)
	if err != nil {
		t.Fatalf("NewBoltRegistry() reopen2 error = %v", err)
	}
	defer reg3.Close()

	retrieved, err = reg3.Get("test-plugin")
	if err != nil {
		t.Fatalf("Get() after reopen2 error = %v", err)
	}

	if !retrieved.Enabled {
		t.Error("Enabled state should persist after reopen")
	}
}

// TestBoltRegistry_ConcurrentReads tests concurrent read operations.
func TestBoltRegistry_ConcurrentReads(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	defer os.Remove(dbPath)

	reg, err := NewBoltRegistry(dbPath)
	if err != nil {
		t.Fatalf("NewBoltRegistry() error = %v", err)
	}
	defer reg.Close()

	// Register multiple plugins
	for i := 0; i < 10; i++ {
		entry := &PluginEntry{
			Manifest: testManifest("plugin-"+string(rune('0'+i)), "1.0.0"),
			Enabled:  true,
		}
		if err := reg.Register(entry); err != nil {
			t.Fatalf("Register() error = %v", err)
		}
	}

	// Concurrent reads
	var wg sync.WaitGroup
	errCh := make(chan error, 20)

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			// Mix Get and List operations
			if idx%2 == 0 {
				_, err := reg.Get("plugin-0")
				if err != nil {
					errCh <- err
					return
				}
			} else {
				_, err := reg.List()
				if err != nil {
					errCh <- err
					return
				}
			}
		}(i)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Errorf("Concurrent read error = %v", err)
	}
}

// TestBoltRegistry_SerializedWrites tests that concurrent writes are serialized.
func TestBoltRegistry_SerializedWrites(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	defer os.Remove(dbPath)

	reg, err := NewBoltRegistry(dbPath)
	if err != nil {
		t.Fatalf("NewBoltRegistry() error = %v", err)
	}
	defer reg.Close()

	var wg sync.WaitGroup
	errCh := make(chan error, 10)

	// Concurrent writes (should be serialized by bbolt)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			entry := &PluginEntry{
				Manifest: testManifest("plugin-"+string(rune('0'+idx)), "1.0.0"),
				Enabled:  true,
			}

			if err := reg.Register(entry); err != nil {
				errCh <- err
				return
			}
		}(i)
	}

	wg.Wait()
	close(errCh)

	// All writes should succeed
	for err := range errCh {
		t.Errorf("Concurrent write error = %v", err)
	}

	// Verify all plugins registered
	entries, err := reg.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	if len(entries) != 10 {
		t.Errorf("List() returned %d entries, want 10", len(entries))
	}
}

// TestBoltRegistry_LargeManifest tests handling large manifests.
func TestBoltRegistry_LargeManifest(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	defer os.Remove(dbPath)

	reg, err := NewBoltRegistry(dbPath)
	if err != nil {
		t.Fatalf("NewBoltRegistry() error = %v", err)
	}
	defer reg.Close()

	// Create manifest with many fields
	manifest := core.Manifest{
		Name:        "large-plugin",
		Version:     "1.0.0",
		Description: "A plugin with extensive metadata",
		Author:      "Test Author",
		License:     "MIT",
		Engine:      "native",
		Requires:    make([]string, 100), // Many dependencies
		Provides:    make([]string, 100), // Many capabilities
	}

	// Fill dependencies and capabilities with unique values
	for i := 0; i < 100; i++ {
		manifest.Requires[i] = "dep-" + string(rune('a'+i%26)) + string(rune('a'+i/26)) + "@1.0.0"
		manifest.Provides[i] = "capability-" + string(rune('a'+i%26)) + string(rune('a'+i/26))
	}

	entry := &PluginEntry{
		Manifest: manifest,
		Enabled:  true,
	}

	// Test registration with large manifest
	if err := reg.Register(entry); err != nil {
		t.Fatalf("Register() with large manifest error = %v", err)
	}

	// Verify retrieval
	retrieved, err := reg.Get("large-plugin")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if len(retrieved.Manifest.Requires) != 100 {
		t.Errorf("Requires truncated, got %d, want 100", len(retrieved.Manifest.Requires))
	}

	if len(retrieved.Manifest.Provides) != 100 {
		t.Errorf("Provides truncated, got %d, want 100", len(retrieved.Manifest.Provides))
	}
}

// TestPluginEntry_JSON tests JSON serialization/deserialization.
func TestPluginEntry_JSON(t *testing.T) {
	entry := &PluginEntry{
		Manifest: core.Manifest{
			Name:        "test-plugin",
			Version:     "1.2.3",
			Description: "Test description",
			Author:      "Test Author",
			License:     "MIT",
			Engine:      "native",
			Requires:    []string{"dep1@1.0.0", "dep2@2.0.0"},
			Provides:    []string{"service1", "service2"},
		},
		Enabled:        true,
		DiscoveredPath: "/path/to/plugin",
	}

	// Serialize
	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	// Deserialize
	var retrieved PluginEntry
	if err := json.Unmarshal(data, &retrieved); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	// Verify fields
	if retrieved.Manifest.Name != entry.Manifest.Name {
		t.Errorf("Name mismatch = %v", retrieved.Manifest.Name)
	}

	if retrieved.Manifest.Version != entry.Manifest.Version {
		t.Errorf("Version mismatch = %v", retrieved.Manifest.Version)
	}

	if retrieved.Enabled != entry.Enabled {
		t.Errorf("Enabled mismatch = %v", retrieved.Enabled)
	}

	if retrieved.DiscoveredPath != entry.DiscoveredPath {
		t.Errorf("DiscoveredPath mismatch = %v", retrieved.DiscoveredPath)
	}
}

// TestBoltRegistry_Closed tests that operations fail after Close().
func TestBoltRegistry_Closed(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	defer os.Remove(dbPath)

	reg, err := NewBoltRegistry(dbPath)
	if err != nil {
		t.Fatalf("NewBoltRegistry() error = %v", err)
	}

	// Close registry
	if err := reg.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	// All operations should fail after close
	entry := &PluginEntry{
		Manifest: testManifest("test", "1.0.0"),
	}

	if err := reg.Register(entry); err != ErrRegistryClosed {
		t.Error("Register() should fail after Close()")
	}

	if _, err := reg.Get("test"); err != ErrRegistryClosed {
		t.Error("Get() should fail after Close()")
	}

	if _, err := reg.List(); err != ErrRegistryClosed {
		t.Error("List() should fail after Close()")
	}

	if err := reg.Update("test", testManifest("test", "2.0.0")); err != ErrRegistryClosed {
		t.Error("Update() should fail after Close()")
	}

	if err := reg.Remove("test"); err != ErrRegistryClosed {
		t.Error("Remove() should fail after Close()")
	}

	if err := reg.Enable("test"); err != ErrRegistryClosed {
		t.Error("Enable() should fail after Close()")
	}

	if err := reg.Disable("test"); err != ErrRegistryClosed {
		t.Error("Disable() should fail after Close()")
	}

	// Double close should be safe
	if err := reg.Close(); err != nil {
		t.Errorf("Double Close() error = %v", err)
	}
}
