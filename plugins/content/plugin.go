package content

import (
	_ "embed"
	"fmt"
	"sync"

	"github.com/wangling-miao/aroute/core"
	"github.com/wangling-miao/aroute/sdk/interfaces"
)

//go:embed manifest.yaml
var manifestData []byte

// Plugin implements the core.Plugin interface for content management.
type Plugin struct {
	*core.BasePlugin

	mu      sync.RWMutex
	ctx     core.CoreContext
	service *Service
	running bool
}

// New creates a new content plugin instance.
func New() *Plugin {
	manifest, err := core.ParseManifest(manifestData, ".yaml")
	if err != nil {
		panic("content plugin: failed to parse embedded manifest: " + err.Error())
	}
	return &Plugin{
		BasePlugin: core.NewBasePluginFromManifest(manifest),
	}
}

// Init initializes the content plugin.
func (p *Plugin) Init(ctx core.CoreContext) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.ctx = ctx
	logger := ctx.Logger()

	logger.Info("Initializing content plugin")

	var dbSvc interfaces.DatabaseService
	if err := ctx.Services().Get(&dbSvc); err != nil {
		return fmt.Errorf("database service not available: %w", err)
	}

	store := NewStore(dbSvc)
	if err := store.CreateTables(ctx.Context()); err != nil {
		return fmt.Errorf("create content tables: %w", err)
	}
	logger.Info("Content tables created or verified")

	svc := NewService(store, ctx.Events(), logger)
	p.service = svc

	if err := svc.InitializeBuiltInContentTypes(ctx.Context()); err != nil {
		return fmt.Errorf("initialize built-in content types: %w", err)
	}
	logger.Info("Built-in content types initialized")

	if err := ctx.Services().Provide(func(container core.ServiceContainer) (interfaces.ContentService, error) {
		return p.service, nil
	}); err != nil {
		return fmt.Errorf("failed to register ContentService: %w", err)
	}

	logger.Info("Content plugin initialized successfully",
		"service_registered", "content.service",
	)

	return nil
}

// Start starts the content plugin.
func (p *Plugin) Start() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.running {
		return nil
	}

	p.ctx.Logger().Info("Content plugin started successfully")
	p.running = true

	return nil
}

// Stop gracefully shuts down the content plugin.
func (p *Plugin) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.running {
		return nil
	}

	p.ctx.Logger().Info("Stopping content plugin")
	p.running = false
	p.ctx.Logger().Info("Content plugin stopped successfully")

	return nil
}
