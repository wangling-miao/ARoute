// Package main implements the serve subcommand for ARoute CMS.
package main

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/wangling-miao/aroute/core"
	"github.com/wangling-miao/aroute/core/ddl"
	"github.com/wangling-miao/aroute/core/engine"
	"github.com/wangling-miao/aroute/core/events"
	"github.com/wangling-miao/aroute/core/license"
	"github.com/wangling-miao/aroute/core/lifecycle"
	"github.com/wangling-miao/aroute/core/loader"
	"github.com/wangling-miao/aroute/core/registry"
	"github.com/wangling-miao/aroute/core/services"
	"github.com/wangling-miao/aroute/plugins/api"
	"github.com/wangling-miao/aroute/plugins/auth"
	"github.com/wangling-miao/aroute/plugins/cache"
	"github.com/wangling-miao/aroute/plugins/content"
	"github.com/wangling-miao/aroute/plugins/database"
	httpplugin "github.com/wangling-miao/aroute/plugins/http"
	"github.com/wangling-miao/aroute/plugins/media"
	"github.com/wangling-miao/aroute/plugins/queue"
	"github.com/wangling-miao/aroute/plugins/search"
	"github.com/wangling-miao/aroute/plugins/theme"
	"github.com/wangling-miao/aroute/sdk/interfaces"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the ARoute CMS server",
	Long: `Start the ARoute CMS server with all enabled plugins.

The server will load configuration, initialize the database connection,
load and start all enabled plugins in dependency order, and begin
listening for HTTP requests on the configured host and port.

Graceful shutdown is triggered by SIGINT (Ctrl+C) or SIGTERM signals.`,
	RunE: runServe,
}

func init() {
	rootCmd.AddCommand(serveCmd)

	// Serve-specific flags
	serveCmd.Flags().StringP("host", "H", "", "host to bind to (default from config)")
	serveCmd.Flags().IntP("port", "p", 0, "port to listen on (default from config)")

	// Bind serve flags to viper
	viper.BindPFlag("server.host", serveCmd.Flags().Lookup("host"))
	viper.BindPFlag("server.port", serveCmd.Flags().Lookup("port"))
}

func runServe(cmd *cobra.Command, args []string) error {
	if err := initLogger(); err != nil {
		return fmt.Errorf("init logger: %w", err)
	}
	logger := getLogger()

	// Get configuration values
	dataDir := getDataDir()
	pluginDir := getPluginDir()
	host := viper.GetString("server.host")
	port := viper.GetInt("server.port")

	// Ensure directories exist
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return fmt.Errorf("create data directory: %w", err)
	}
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		return fmt.Errorf("create plugin directory: %w", err)
	}

	logger.Info("initializing aroute engine",
		"host", host,
		"port", port,
		"data_dir", dataDir,
		"plugin_dir", pluginDir,
		"log_level", viper.GetString("log.level"))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	container := services.NewContainer()
	eventBus := events.NewEventBus()

	registryPath := filepath.Join(dataDir, "registry.db")
	reg, err := registry.NewBoltRegistry(registryPath)
	if err != nil {
		return fmt.Errorf("create registry: %w", err)
	}

	dispatcher := engine.NewDispatcher()
	licenseValidator := license.NewValidator(nil, (*ecdsa.PublicKey)(nil))

	discovery := registry.NewFSDiscovery(pluginDir)
	registered, err := registry.LoadAndRegister(reg, discovery)
	if err != nil {
		logger.Warn("plugin discovery error", "error", err)
	}
	logger.Info("plugins discovered and registered", "count", registered)

	// Core context factory for plugins
	ctxFactory := func(pluginCtx context.Context, pluginName string) core.CoreContext {
		pluginLogger := logger.With("plugin", pluginName)
		pluginDataDir := filepath.Join(dataDir, "plugins", pluginName)
		pluginConfig := core.NewViperConfig(viper.GetViper())
		return core.NewCoreContext(pluginCtx, container, eventBus, pluginConfig, pluginLogger, pluginDataDir, pluginDir)
	}

	// Create native plugin loader and register built-in plugins
	pluginLoader := loader.NewNativePluginLoader()
	pluginLoader.Register("http", func() core.Plugin {
		return httpplugin.New()
	})
	pluginLoader.Register("database", func() core.Plugin {
		return database.New()
	})
	pluginLoader.Register("auth", func() core.Plugin {
		return auth.New()
	})
	pluginLoader.Register("content", func() core.Plugin {
		return content.New()
	})
	pluginLoader.Register("media", func() core.Plugin {
		return media.New()
	})
	pluginLoader.Register("theme", func() core.Plugin {
		return theme.New()
	})
	pluginLoader.Register("search", func() core.Plugin {
		return search.New()
	})
	pluginLoader.Register("api", func() core.Plugin {
		return api.New()
	})
	pluginLoader.Register("cache", func() core.Plugin {
		return cache.New()
	})
	pluginLoader.Register("queue", func() core.Plugin {
		return queue.New()
	})
	lifecycleManager := lifecycle.NewManager(
		&registryAdapterForLifecycle{registry: reg},
		&pluginLoaderAdapter{loader: pluginLoader},
		eventBus,
		container,
		ctxFactory,
	)

	// Create Aroute engine
	aroute, err := core.New(ctx, container, eventBus, &pluginRegistryAdapter{reg: reg}, lifecycleManager, &dispatcherAdapter{d: dispatcher}, &licenseAdapter{v: licenseValidator},
		core.WithDataDir(dataDir),
		core.WithPluginDir(pluginDir),
		core.WithLogger(logger),
	)
	if err != nil {
		return fmt.Errorf("create aroute engine: %w", err)
	}

	// Provide core services in container
	container.Provide(func(c *services.Container) (*events.EventBus, error) { return eventBus, nil })
	container.Provide(func(c *services.Container) (*license.Validator, error) { return licenseValidator, nil })

	logger.Info("starting aroute engine")
	if err := aroute.Start(ctx); err != nil {
		return fmt.Errorf("start engine: %w", err)
	}

	var dbService interfaces.DatabaseService
	if err := container.Get(&dbService); err != nil {
		logger.Warn("DDL Registry skipped: database service unavailable", "error", err)
	} else {
		ddlRegistry := ddl.NewRegistry(dbService)
		aroute.SetDDL(&ddlRegistryAdapter{reg: ddlRegistry})
		if err := aroute.DDL().Init(ctx); err != nil {
			logger.Warn("DDL initialization failed", "error", err)
		} else {
			logger.Info("DDL Registry initialized")
		}
	}

	logger.Info("aroute engine started successfully",
		"host", host,
		"port", port,
		"tier", licenseValidator.Tier().String())

	// Setup signal handling for graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Wait for shutdown signal
	sig := <-sigCh
	logger.Info("received shutdown signal", "signal", sig.String())

	// Create shutdown context with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	// Start graceful shutdown in a goroutine to allow second signal handling
	done := make(chan error, 1)
	go func() {
		logger.Info("shutting down gracefully...")
		if err := aroute.Stop(shutdownCtx); err != nil {
			logger.Error("shutdown error", "error", err)
			done <- err
			return
		}
		done <- nil
	}()

	// Wait for either shutdown completion or second signal
	select {
	case err := <-done:
		if err != nil {
			return err
		}
		logger.Info("stopped successfully")
		return nil
	case <-sigCh:
		// Second signal received - forced shutdown
		logger.Info("forced shutdown")
		shutdownCancel()
		return fmt.Errorf("forced shutdown")
	case <-shutdownCtx.Done():
		// Timeout occurred
		logger.Info("graceful shutdown timed out, forcing shutdown")
		return fmt.Errorf("shutdown timeout")
	}
}

// Adapter types to bridge between core interfaces and concrete implementations

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
	return core.LicenseInfoResult{Tier: a.Tier(), Features: info.Features, ExpiresAt: info.ExpiresAt}
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

type pluginLoaderAdapter struct {
	loader *loader.NativePluginLoader
}

func (a *pluginLoaderAdapter) Load(manifest core.Manifest) (core.Plugin, error) {
	return a.loader.Load(manifest)
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

type coreExecutorAsEngine struct {
	e core.EngineExecutor
}

func (c *coreExecutorAsEngine) Type() core.EngineType { return c.e.Type() }
func (c *coreExecutorAsEngine) Initialize(ctx context.Context) error {
	return c.e.Initialize(ctx)
}
func (c *coreExecutorAsEngine) ExecuteLifecycle(ctx context.Context, plugin core.Plugin, ctx2 core.CoreContext) error {
	return c.e.ExecuteLifecycle(ctx, plugin, ctx2)
}
func (c *coreExecutorAsEngine) Close() error { return c.e.Close() }

func (a *dispatcherAdapter) RegisterEngine(engineType core.EngineType, executor core.EngineExecutor) error {
	return a.d.RegisterEngine(engineType, &coreExecutorAsEngine{e: executor})
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

type ddlRegistryAdapter struct {
	reg *ddl.Registry
}

func (a *ddlRegistryAdapter) Init(ctx context.Context) error {
	return a.reg.Init(ctx)
}

func (a *ddlRegistryAdapter) Create(ctx context.Context, schema interface{}) error {
	s, ok := schema.(*ddl.Schema)
	if !ok {
		return fmt.Errorf("ddl: invalid schema type")
	}
	return a.reg.Create(ctx, s)
}

func (a *ddlRegistryAdapter) Get(ctx context.Context, name string) (interface{}, error) {
	return a.reg.Get(ctx, name)
}

func (a *ddlRegistryAdapter) Update(ctx context.Context, schema interface{}) error {
	s, ok := schema.(*ddl.Schema)
	if !ok {
		return fmt.Errorf("ddl: invalid schema type")
	}
	return a.reg.Update(ctx, s)
}

func (a *ddlRegistryAdapter) Delete(ctx context.Context, name string, force bool) error {
	return a.reg.Delete(ctx, name, force)
}

func (a *ddlRegistryAdapter) List(ctx context.Context) ([]interface{}, error) {
	schemas, err := a.reg.List(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]interface{}, len(schemas))
	for i, s := range schemas {
		result[i] = s
	}
	return result, nil
}
