package search

import (
	_ "embed"
	"fmt"
	"sync"

	"github.com/wangling-miao/aroute/core"
	"github.com/wangling-miao/aroute/sdk/interfaces"
)

//go:embed manifest.yaml
var manifestData []byte

// Plugin implements the core.Plugin interface for full-text search.
type Plugin struct {
	*core.BasePlugin

	mu      sync.RWMutex
	ctx     core.CoreContext
	service *Service
	running bool
}

// New creates a new search plugin instance.
func New() *Plugin {
	manifest, err := core.ParseManifest(manifestData, ".yaml")
	if err != nil {
		panic("search plugin: failed to parse embedded manifest: " + err.Error())
	}
	return &Plugin{
		BasePlugin: core.NewBasePluginFromManifest(manifest),
	}
}

// Init initializes the search plugin.
func (p *Plugin) Init(ctx core.CoreContext) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.ctx = ctx
	logger := ctx.Logger()

	logger.Info("Initializing search plugin")

	// Get ContentService dependency for rebuild operations
	var contentSvc interfaces.ContentService
	if err := ctx.Services().Get(&contentSvc); err != nil {
		return fmt.Errorf("content service not available: %w", err)
	}

	// Create the search service with bleve index
	svc, err := NewService(contentSvc, ctx.DataDir(), ctx.Events(), logger)
	if err != nil {
		return fmt.Errorf("create search service: %w", err)
	}
	p.service = svc

	// Register SearchService in the service container
	if err := ctx.Services().Provide(func(container core.ServiceContainer) (interfaces.SearchService, error) {
		return p.service, nil
	}); err != nil {
		return fmt.Errorf("failed to register SearchService: %w", err)
	}

	// Subscribe to content events for auto-indexing
	// Use wildcard "content.**" to catch content.{type}.created, content.{type}.updated, content.{type}.deleted
	ctx.Events().SubscribeBroadcast("content.**", p.service.HandleContentEvent)

	logger.Info("Search plugin initialized successfully")
	return nil
}

// Start starts the search plugin.
func (p *Plugin) Start() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.running {
		return nil
	}

	p.ctx.Logger().Info("Search plugin started successfully")
	p.running = true

	return nil
}

// Stop gracefully shuts down the search plugin.
func (p *Plugin) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.running {
		return nil
	}

	p.ctx.Logger().Info("Stopping search plugin")
	if p.service != nil {
		p.service.Close()
	}
	p.running = false
	p.ctx.Logger().Info("Search plugin stopped successfully")

	return nil
}
