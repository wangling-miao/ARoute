package loader

import (
	"testing"

	"github.com/wangling-miao/aroute/core"
)

func TestNativePluginLoader_Register(t *testing.T) {
	loader := NewNativePluginLoader()

	loader.Register("test", func() core.Plugin {
		return &mockPlugin{name: "test"}
	})

	if !loader.Has("test") {
		t.Error("expected plugin to be registered")
	}

	names := loader.List()
	if len(names) != 1 {
		t.Errorf("expected 1 plugin, got %d", len(names))
	}
}

func TestNativePluginLoader_Unregister(t *testing.T) {
	loader := NewNativePluginLoader()

	loader.Register("test", func() core.Plugin {
		return &mockPlugin{name: "test"}
	})

	loader.Unregister("test")

	if loader.Has("test") {
		t.Error("expected plugin to be unregistered")
	}
}

func TestNativePluginLoader_Load(t *testing.T) {
	loader := NewNativePluginLoader()

	loader.Register("test", func() core.Plugin {
		return &mockPlugin{name: "test", version: "1.0.0"}
	})

	manifest := core.Manifest{
		Name:    "test",
		Version: "1.0.0",
		Engine:  "native",
	}

	plugin, err := loader.Load(manifest)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if plugin.Name() != "test" {
		t.Errorf("expected name 'test', got '%s'", plugin.Name())
	}

	if plugin.Version() != "1.0.0" {
		t.Errorf("expected version '1.0.0', got '%s'", plugin.Version())
	}
}

func TestNativePluginLoader_Load_NotFound(t *testing.T) {
	loader := NewNativePluginLoader()

	manifest := core.Manifest{
		Name:    "unknown",
		Version: "1.0.0",
		Engine:  "native",
	}

	_, err := loader.Load(manifest)
	if err == nil {
		t.Error("expected error for unknown plugin")
	}
}

func TestNativePluginLoader_Load_WasmEngine(t *testing.T) {
	loader := NewNativePluginLoader()

	loader.Register("test", func() core.Plugin {
		return &mockPlugin{name: "test"}
	})

	manifest := core.Manifest{
		Name:    "test",
		Version: "1.0.0",
		Engine:  "wasm",
	}

	_, err := loader.Load(manifest)
	if err == nil {
		t.Error("expected error for wasm engine")
	}
}

func TestNativePluginLoader_Load_InvalidEngine(t *testing.T) {
	loader := NewNativePluginLoader()

	manifest := core.Manifest{
		Name:    "test",
		Version: "1.0.0",
		Engine:  "invalid",
	}

	_, err := loader.Load(manifest)
	if err == nil {
		t.Error("expected error for invalid engine")
	}
}

func TestNativePluginLoader_Load_NilFactory(t *testing.T) {
	loader := NewNativePluginLoader()

	loader.Register("nil-plugin", func() core.Plugin {
		return nil
	})

	manifest := core.Manifest{
		Name:    "nil-plugin",
		Version: "1.0.0",
		Engine:  "native",
	}

	_, err := loader.Load(manifest)
	if err == nil {
		t.Error("expected error for nil factory result")
	}
}

type mockPlugin struct {
	name    string
	version string
}

func (p *mockPlugin) Name() string                    { return p.name }
func (p *mockPlugin) Version() string                 { return p.version }
func (p *mockPlugin) Manifest() *core.Manifest        { return nil }
func (p *mockPlugin) Init(ctx core.CoreContext) error { return nil }
func (p *mockPlugin) Start() error                    { return nil }
func (p *mockPlugin) Stop() error                     { return nil }
