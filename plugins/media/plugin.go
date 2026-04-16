package media

import (
	_ "embed"
	"fmt"
	"sync"

	"github.com/wangling-miao/aroute/core"
	"github.com/wangling-miao/aroute/sdk/interfaces"
)

//go:embed manifest.yaml
var manifestData []byte

// Plugin implements the core.Plugin interface for media management.
type Plugin struct {
	*core.BasePlugin

	mu      sync.RWMutex
	ctx     core.CoreContext
	service *Service
	running bool
}

// New creates a new media plugin instance.
func New() *Plugin {
	manifest, err := core.ParseManifest(manifestData, ".yaml")
	if err != nil {
		panic("media plugin: failed to parse embedded manifest: " + err.Error())
	}
	return &Plugin{
		BasePlugin: core.NewBasePluginFromManifest(manifest),
	}
}

// Init initializes the media plugin.
func (p *Plugin) Init(ctx core.CoreContext) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.ctx = ctx
	logger := ctx.Logger()

	logger.Info("Initializing media plugin")

	var dbSvc interfaces.DatabaseService
	if err := ctx.Services().Get(&dbSvc); err != nil {
		return fmt.Errorf("database service not available: %w", err)
	}

	store := NewStore(dbSvc)
	if err := store.CreateTables(ctx.Context()); err != nil {
		return fmt.Errorf("create media tables: %w", err)
	}
	logger.Info("Media tables created or verified")

	storageType := ctx.Config().GetString("storage")
	if storageType == "" {
		storageType = "local"
	}
	dataDir := ctx.DataDir()

	storage, err := NewStorageBackend(storageType, dataDir, ctx.Config())
	if err != nil {
		return fmt.Errorf("create storage backend: %w", err)
	}

	svc := NewService(store, storage, ctx.Events(), logger)
	p.service = svc

	if err := ctx.Services().Provide(func(container core.ServiceContainer) (interfaces.MediaService, error) {
		return p.service, nil
	}); err != nil {
		return fmt.Errorf("failed to register MediaService: %w", err)
	}

	logger.Info("Media plugin initialized successfully",
		"service_registered", "media.service",
	)

	return nil
}

// Start starts the media plugin.
func (p *Plugin) Start() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.running {
		return nil
	}

	p.ctx.Logger().Info("Media plugin started successfully")
	p.running = true

	return nil
}

// Stop gracefully shuts down the media plugin.
func (p *Plugin) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.running {
		return nil
	}

	p.ctx.Logger().Info("Stopping media plugin")
	p.running = false
	p.ctx.Logger().Info("Media plugin stopped successfully")

	return nil
}
