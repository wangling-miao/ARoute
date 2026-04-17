package theme

import (
	_ "embed"
	"fmt"
	"sync"

	"github.com/wangling-miao/aroute/core"
	"github.com/wangling-miao/aroute/sdk/interfaces"
)

//go:embed manifest.yaml
var manifestData []byte

type Plugin struct {
	*core.BasePlugin

	mu      sync.RWMutex
	ctx     core.CoreContext
	service *Service
	running bool
}

func New() *Plugin {
	manifest, err := core.ParseManifest(manifestData, ".yaml")
	if err != nil {
		panic("theme plugin: failed to parse embedded manifest: " + err.Error())
	}
	return &Plugin{
		BasePlugin: core.NewBasePluginFromManifest(manifest),
	}
}

func (p *Plugin) Init(ctx core.CoreContext) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.ctx = ctx
	logger := ctx.Logger()

	logger.Info("Initializing theme plugin")

	var dbSvc interfaces.DatabaseService
	if err := ctx.Services().Get(&dbSvc); err != nil {
		return fmt.Errorf("database service not available: %w", err)
	}

	store := NewStore(dbSvc)
	if err := store.CreateTables(ctx.Context()); err != nil {
		return fmt.Errorf("create theme tables: %w", err)
	}
	logger.Info("Theme tables created or verified")

	activeTheme := ctx.Config().GetString("active")
	if activeTheme == "" {
		activeTheme = "default"
	}

	svc := NewService(store, ctx.Events(), logger, activeTheme)

	themesDir := ctx.DataDir()
	if err := svc.LoadThemes(themesDir); err != nil {
		logger.Warn("Failed to load themes from directory", "dir", themesDir, "error", err)
	}

	p.service = svc

	if err := ctx.Services().Provide(func(container core.ServiceContainer) (interfaces.ThemeService, error) {
		return p.service, nil
	}); err != nil {
		return fmt.Errorf("failed to register ThemeService: %w", err)
	}

	logger.Info("Theme plugin initialized successfully",
		"service_registered", "theme.service",
		"active_theme", activeTheme,
	)

	return nil
}

func (p *Plugin) Start() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.running {
		return nil
	}

	p.ctx.Logger().Info("Theme plugin started successfully")
	p.running = true

	return nil
}

func (p *Plugin) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.running {
		return nil
	}

	p.ctx.Logger().Info("Stopping theme plugin")
	p.running = false
	p.ctx.Logger().Info("Theme plugin stopped successfully")

	return nil
}
