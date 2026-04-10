package main

import (
	"context"
	"crypto/ecdsa"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/wangling-miao/aroute/core"
	"github.com/wangling-miao/aroute/core/engine"
	"github.com/wangling-miao/aroute/core/events"
	"github.com/wangling-miao/aroute/core/license"
	"github.com/wangling-miao/aroute/core/lifecycle"
	"github.com/wangling-miao/aroute/core/registry"
	"github.com/wangling-miao/aroute/core/services"
	"github.com/wangling-miao/aroute/internal/version"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Printf("aroute %s\n", version.Version)
		fmt.Printf("  commit:     %s\n", version.Commit)
		fmt.Printf("  build date: %s\n", version.BuildDate)
		fmt.Printf("  go version: %s\n", version.GoVersion)
		os.Exit(0)
	}

	dataDir := flag.String("data-dir", filepath.Join(os.Getenv("HOME"), ".aroute", "data"), "Data directory")
	pluginDir := flag.String("plugin-dir", filepath.Join(os.Getenv("HOME"), ".aroute", "plugins"), "Plugin directory")
	logLevel := flag.String("log-level", "info", "Log level")
	flag.Parse()

	var level slog.Level
	switch *logLevel {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
	slog.SetDefault(logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	logger.Info("initializing aroute engine", "data_dir", *dataDir, "plugin_dir", *pluginDir)

	container := services.NewContainer()
	eventBus := events.NewEventBus()

	registryPath := filepath.Join(*dataDir, "registry.db")
	reg, err := registry.NewBoltRegistry(registryPath)
	if err != nil {
		logger.Error("failed to create registry", "error", err)
		os.Exit(1)
	}

	dispatcher := engine.NewDispatcher()
	licenseValidator := license.NewValidator(nil, (*ecdsa.PublicKey)(nil))

	ctxFactory := func(pluginCtx context.Context, pluginName string) core.CoreContext {
		pluginLogger := logger.With("plugin", pluginName)
		pluginDataDir := filepath.Join(*dataDir, "plugins", pluginName)
		pluginConfig := core.NewScopedConfig(pluginName, make(map[string]interface{}))
		return core.NewCoreContext(pluginCtx, container, &eventBusAdapter{eb: eventBus}, pluginConfig, pluginLogger, pluginDataDir, *pluginDir)
	}

	lifecycleManager := lifecycle.NewManager(
		&registryAdapterForLifecycle{registry: reg},
		nil,
		&eventBusAdapter{eb: eventBus},
		container,
		ctxFactory,
	)

	aroute, err := core.New(ctx, container, &eventBusAdapter{eb: eventBus}, &pluginRegistryAdapter{reg: reg}, lifecycleManager, &dispatcherAdapter{d: dispatcher}, &licenseAdapter{v: licenseValidator},
		core.WithDataDir(*dataDir),
		core.WithPluginDir(*pluginDir),
		core.WithLogger(logger),
	)
	if err != nil {
		logger.Error("failed to create aroute engine", "error", err)
		os.Exit(1)
	}

	container.Provide(func(c *services.Container) (*events.EventBus, error) { return eventBus, nil })
	container.Provide(func(c *services.Container) (*license.Validator, error) { return licenseValidator, nil })

	logger.Info("starting aroute engine")
	if err := aroute.Start(ctx); err != nil {
		logger.Error("failed to start", "error", err)
		os.Exit(1)
	}

	logger.Info("aroute engine started successfully")

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	logger.Info("shutting down")
	aroute.Stop(ctx)
	logger.Info("stopped")
}

type eventBusAdapter struct {
	eb *events.EventBus
}

func (a *eventBusAdapter) SubscribeFilter(event string, priority int, handler core.FilterHandler) string {
	return a.eb.SubscribeFilter(event, priority, func(ctx context.Context, e *events.Event) (*events.Event, error) {
		next := func() (interface{}, error) { return e.Data, nil }
		result, err := handler(ctx, e.Topic, e.Data, next)
		if err != nil {
			return nil, err
		}
		if m, ok := result.(map[string]interface{}); ok {
			e.Data = m
		}
		return e, nil
	})
}

func (a *eventBusAdapter) SubscribeBroadcast(event string, handler core.BroadcastHandler) string {
	return a.eb.SubscribeBroadcast(event, func(ctx context.Context, e events.Event) {
		handler(ctx, e.Topic, e.Data)
	})
}

func (a *eventBusAdapter) Emit(ctx context.Context, event string, data interface{}) {
	if m, ok := data.(map[string]interface{}); ok {
		a.eb.Emit(ctx, events.Event{Topic: event, Data: m})
	}
}

func (a *eventBusAdapter) DispatchFilter(ctx context.Context, event string, data interface{}) (interface{}, error) {
	m, _ := data.(map[string]interface{})
	e, err := a.eb.DispatchFilter(ctx, &events.Event{Topic: event, Data: m})
	if err != nil {
		return nil, err
	}
	return e.Data, nil
}

func (a *eventBusAdapter) Unsubscribe(handlerID string) error {
	a.eb.Unsubscribe(handlerID)
	return nil
}

type pluginRegistryAdapter struct {
	reg *registry.BoltRegistry
}

func (a *pluginRegistryAdapter) Register(entry *core.PluginEntry) error {
	return a.reg.Register(&registry.PluginEntry{
		Manifest:       entry.Manifest,
		Enabled:        entry.Enabled,
		DiscoveredPath: entry.DiscoveredPath,
	})
}

func (a *pluginRegistryAdapter) Get(name string) (*core.PluginEntry, error) {
	entry, err := a.reg.Get(name)
	if err != nil {
		return nil, err
	}
	return &core.PluginEntry{
		Manifest:       entry.Manifest,
		Enabled:        entry.Enabled,
		DiscoveredPath: entry.DiscoveredPath,
	}, nil
}

func (a *pluginRegistryAdapter) List() ([]*core.PluginEntry, error) {
	entries, err := a.reg.List()
	if err != nil {
		return nil, err
	}
	result := make([]*core.PluginEntry, len(entries))
	for i, e := range entries {
		result[i] = &core.PluginEntry{
			Manifest:       e.Manifest,
			Enabled:        e.Enabled,
			DiscoveredPath: e.DiscoveredPath,
		}
	}
	return result, nil
}

func (a *pluginRegistryAdapter) Update(name string, manifest core.Manifest) error {
	return a.reg.Update(name, manifest)
}

func (a *pluginRegistryAdapter) Remove(name string) error  { return a.reg.Remove(name) }
func (a *pluginRegistryAdapter) Enable(name string) error  { return a.reg.Enable(name) }
func (a *pluginRegistryAdapter) Disable(name string) error { return a.reg.Disable(name) }
func (a *pluginRegistryAdapter) Close() error              { return a.reg.Close() }

type licenseAdapter struct {
	v *license.Validator
}

func (a *licenseAdapter) Tier() core.LicenseTier {
	switch a.v.Tier() {
	case license.TierPro:
		return core.LicenseTierPro
	case license.TierEnterprise:
		return core.LicenseTierEnterprise
	default:
		return core.LicenseTierOpen
	}
}

func (a *licenseAdapter) IsFeatureAllowed(feature string) bool { return a.v.IsFeatureAllowed(feature) }
func (a *licenseAdapter) IsExpired() bool                      { return a.v.IsExpired() }
func (a *licenseAdapter) Validate() error                      { return a.v.Validate() }
func (a *licenseAdapter) LicenseInfo() core.LicenseInfoResult {
	info := a.v.LicenseInfo()
	return core.LicenseInfoResult{Tier: a.Tier(), Features: info.Features}
}

type registryAdapterForLifecycle struct {
	registry registry.Registry
}

func (a *registryAdapterForLifecycle) List() ([]core.Manifest, error) {
	entries, err := a.registry.List()
	if err != nil {
		return nil, err
	}
	manifests := make([]core.Manifest, len(entries))
	for i, e := range entries {
		manifests[i] = e.Manifest
	}
	return manifests, nil
}

func (a *registryAdapterForLifecycle) Get(name string) (core.Manifest, error) {
	entry, err := a.registry.Get(name)
	if err != nil {
		return core.Manifest{}, err
	}
	return entry.Manifest, nil
}

func (a *registryAdapterForLifecycle) IsEnabled(name string) (bool, error) {
	entry, err := a.registry.Get(name)
	if err != nil {
		return false, err
	}
	return entry.Enabled, nil
}

type dispatcherAdapter struct {
	d engine.Dispatcher
}

type engineExecutorAdapter struct {
	e engine.Engine
}

func (a *engineExecutorAdapter) Type() core.EngineType                { return a.e.Type() }
func (a *engineExecutorAdapter) Initialize(ctx context.Context) error { return a.e.Initialize(ctx) }
func (a *engineExecutorAdapter) ExecuteLifecycle(ctx context.Context, plugin core.Plugin, coreCtx core.CoreContext) error {
	return a.e.ExecuteLifecycle(ctx, plugin, coreCtx)
}
func (a *engineExecutorAdapter) Close() error { return a.e.Close() }

func (a *dispatcherAdapter) RegisterEngine(engineType core.EngineType, executor core.EngineExecutor) error {
	return nil
}

func (a *dispatcherAdapter) GetEngine(engineType core.EngineType) (core.EngineExecutor, error) {
	e, err := a.d.GetEngine(engineType)
	if err != nil {
		return nil, err
	}
	return &engineExecutorAdapter{e: e}, nil
}

func (a *dispatcherAdapter) Execute(ctx context.Context, plugin core.Plugin, manifest *core.Manifest, coreCtx core.CoreContext) error {
	return a.d.Execute(ctx, plugin, manifest, coreCtx)
}

func (a *dispatcherAdapter) Close() error { return a.d.Close() }
