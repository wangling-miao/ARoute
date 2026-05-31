package loader

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/wangling-miao/aroute/core"
	"github.com/wangling-miao/aroute/core/trust"
)

// WasmLoader implements the PluginLoader interface for Wasm (L3) plugins.
// It loads .wasm binaries from the filesystem at runtime — community plugins
// are NOT compiled into the host binary.
//
// Expected directory layout under the plugin directory:
//
//	data/plugins/
//	├── example-wasm/
//	│   ├── manifest.yaml   (name, version, engine: wasm, ...)
//	│   └── plugin.wasm     (compiled Wasm binary)
//	└── another-plugin/
//	    ├── manifest.yaml
//	    └── plugin.wasm
type WasmLoader struct {
	mu        sync.RWMutex
	pluginDir string
	broker    *trust.Broker
}

// NewWasmLoader creates a WasmLoader that reads L3 plugins from pluginDir.
func NewWasmLoader(pluginDir string, broker ...*trust.Broker) *WasmLoader {
	var b *trust.Broker
	if len(broker) > 0 {
		b = broker[0]
	}
	return &WasmLoader{
		pluginDir: pluginDir,
		broker:    b,
	}
}

// Load creates a FilesystemWasmPlugin by reading the .wasm binary from disk.
func (l *WasmLoader) Load(manifest core.Manifest) (core.Plugin, error) {
	engineType, err := core.ParseEngine(manifest.Engine)
	if err != nil {
		return nil, fmt.Errorf("wasm loader: invalid engine type '%s': %w", manifest.Engine, err)
	}
	if engineType != core.EngineL3Wasm {
		return nil, fmt.Errorf("wasm loader: plugin '%s' has engine '%s', expected 'wasm'", manifest.Name, manifest.Engine)
	}

	pluginPath := filepath.Join(l.pluginDir, manifest.Name)
	wasmFile := filepath.Join(pluginPath, "plugin.wasm")

	wasmBytes, err := os.ReadFile(wasmFile)
	if err != nil {
		return nil, fmt.Errorf("wasm loader: read %s: %w", wasmFile, err)
	}

	return &FilesystemWasmPlugin{
		manifest:   manifest,
		wasmBytes:  wasmBytes,
		wasmPath:   wasmFile,
		pluginPath: pluginPath,
		broker:     l.broker,
	}, nil
}

// FilesystemWasmPlugin is an L3 plugin loaded from the filesystem.
// It runs the .wasm binary inside a wazero sandbox during lifecycle calls.
type FilesystemWasmPlugin struct {
	manifest   core.Manifest
	wasmBytes  []byte
	wasmPath   string
	pluginPath string
	runtime    wazero.Runtime
	module     api.Module
	broker     *trust.Broker
}

func (p *FilesystemWasmPlugin) Name() string               { return p.manifest.Name }
func (p *FilesystemWasmPlugin) Version() string            { return p.manifest.Version }
func (p *FilesystemWasmPlugin) Manifest() *core.Manifest   { return &p.manifest }
func (p *FilesystemWasmPlugin) WasmBytes() ([]byte, error) { return p.wasmBytes, nil }
func (p *FilesystemWasmPlugin) PluginPath() string         { return p.pluginPath }

func (p *FilesystemWasmPlugin) Init(ctx core.CoreContext) error {
	logger := ctx.Logger()
	logger.Info("Initializing L3 Wasm plugin", "engine", "wasm", "binary_size", len(p.wasmBytes), "path", p.wasmPath)
	if p.broker != nil {
		p.broker.RegisterGrant(trust.Grant{
			Plugin:       p.manifest.Name,
			Engine:       "wasm",
			TrustLevel:   "L3",
			Capabilities: append([]string(nil), p.manifest.Capabilities...),
		})
	}

	wasmCtx := context.Background()
	p.runtime = wazero.NewRuntimeWithConfig(wasmCtx,
		wazero.NewRuntimeConfig().WithMemoryLimitPages(512),
	)

	// Build the "cms" host module with Core API functions
	_, err := p.runtime.NewHostModuleBuilder("cms").
		NewFunctionBuilder().
		WithFunc(func(c context.Context, m api.Module, serviceID uint32) uint32 {
			logger.Debug("Wasm host: service_has", "service_id", serviceID)
			if !p.authorizeHostCall(c, "service:has") {
				return 0
			}
			return 1
		}).Export("service_has").
		NewFunctionBuilder().
		WithFunc(func(c context.Context, m api.Module, serviceID uint32) uint32 {
			if !p.authorizeHostCall(c, "service:get") {
				return 0
			}
			return serviceID + 1
		}).Export("service_get").
		NewFunctionBuilder().
		WithFunc(func(c context.Context, m api.Module, topicPtr, topicLen, callbackPtr, callbackLen uint32) uint32 {
			if !p.authorizeHostCall(c, "event:subscribe") {
				return 0
			}
			return 1
		}).Export("event_subscribe").
		NewFunctionBuilder().
		WithFunc(func(c context.Context, m api.Module, topicPtr, topicLen, dataPtr, dataLen uint32) {
			topic := readWasmString(m, topicPtr, topicLen)
			data := readWasmString(m, dataPtr, dataLen)
			if !p.authorizeHostCall(c, "event:publish:"+topic) {
				return
			}
			logger.Info("Wasm plugin published event", "topic", topic, "data", data)
		}).Export("event_publish").
		NewFunctionBuilder().
		WithFunc(func(c context.Context, m api.Module, msgPtr, msgLen uint32) {
			msg := readWasmString(m, msgPtr, msgLen)
			logger.Info("wasm plugin log", "message", msg)
		}).Export("host_log").
		NewFunctionBuilder().
		WithFunc(func(c context.Context, m api.Module, size uint32) uint32 {
			return 256
		}).Export("memory_alloc").
		NewFunctionBuilder().
		WithFunc(func(c context.Context, m api.Module, ptr uint32) {}).Export("memory_free").
		Instantiate(wasmCtx)
	if err != nil {
		return fmt.Errorf("wasm: building cms host module: %w", err)
	}

	mod, err := p.runtime.InstantiateWithConfig(wasmCtx, p.wasmBytes,
		wazero.NewModuleConfig().WithName(p.manifest.Name),
	)
	if err != nil {
		return fmt.Errorf("wasm: instantiating module: %w", err)
	}
	p.module = mod

	if fn := mod.ExportedFunction("init"); fn != nil {
		if _, err := fn.Call(wasmCtx); err != nil {
			return fmt.Errorf("wasm: calling init: %w", err)
		}
	}

	logger.Info("L3 Wasm plugin initialized successfully")
	return nil
}

func (p *FilesystemWasmPlugin) Start() error {
	if p.module == nil {
		return nil
	}
	if fn := p.module.ExportedFunction("start"); fn != nil {
		if _, err := fn.Call(context.Background()); err != nil {
			return fmt.Errorf("wasm: calling start: %w", err)
		}
	}
	return nil
}

func (p *FilesystemWasmPlugin) Stop() error {
	if p.module == nil {
		return nil
	}
	if fn := p.module.ExportedFunction("stop"); fn != nil {
		if _, err := fn.Call(context.Background()); err != nil {
			slog.Warn("wasm: calling stop", "plugin", p.manifest.Name, "error", err)
		}
	}
	p.module.Close(context.Background())
	if p.runtime != nil {
		p.runtime.Close(context.Background())
	}
	return nil
}

func readWasmString(m api.Module, ptr, length uint32) string {
	if length == 0 {
		return ""
	}
	mem := m.Memory()
	if mem == nil {
		return ""
	}
	buf, ok := mem.Read(ptr, length)
	if !ok {
		return ""
	}
	return string(buf)
}

func (p *FilesystemWasmPlugin) authorizeHostCall(ctx context.Context, capability string) bool {
	if p.broker == nil {
		return true
	}
	result := p.broker.Authorize(ctx, trust.AuthorizationRequest{
		Plugin:     p.manifest.Name,
		Engine:     "wasm",
		Capability: capability,
	})
	return result.Allowed
}
