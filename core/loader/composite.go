package loader

import (
	"fmt"

	"github.com/wangling-miao/aroute/core"
)

// CompositeLoader delegates to NativePluginLoader or WasmLoader based on engine type.
type CompositeLoader struct {
	native *NativePluginLoader
	wasm   *WasmLoader
}

// NewCompositeLoader creates a loader that handles both native and wasm plugins.
func NewCompositeLoader(native *NativePluginLoader, loaders ...interface{}) *CompositeLoader {
	c := &CompositeLoader{native: native}
	for _, item := range loaders {
		switch loader := item.(type) {
		case *WasmLoader:
			c.wasm = loader
		}
	}
	return c
}

// Load creates a plugin instance by dispatching to the appropriate sub-loader.
func (c *CompositeLoader) Load(manifest core.Manifest) (core.Plugin, error) {
	if manifest.Engine == "" || manifest.Engine == "native" || manifest.Engine == "l1" {
		return c.native.Load(manifest)
	}
	if manifest.Engine == "wasm" || manifest.Engine == "l3" {
		if c.wasm == nil {
			return nil, fmt.Errorf("loader: wasm loader not configured for plugin '%s'", manifest.Name)
		}
		return c.wasm.Load(manifest)
	}
	return nil, fmt.Errorf("loader: unsupported engine type '%s' for plugin '%s'", manifest.Engine, manifest.Name)
}
