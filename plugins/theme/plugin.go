package theme

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
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

// seedBuiltinThemes copies built-in themes from the project "themes/" directory
// into the plugin's data directory if they don't already exist there.
func seedBuiltinThemes(themesDir string, logger interface{ Info(string, ...interface{}) }) {
	// Try multiple locations for the built-in themes directory:
	// 1. Relative to current working directory (development: go run ./cmd/aroute serve)
	// 2. Relative to the executable (installed binary)
	var sourceDir string
	candidates := []string{"themes"}
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		candidates = append(candidates,
			filepath.Join(exeDir, "themes"),
			filepath.Join(exeDir, "..", "themes"),
		)
	}

	for _, dir := range candidates {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			sourceDir = dir
			break
		}
	}

	if sourceDir == "" {
		return
	}

	entries, err := os.ReadDir(sourceDir)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		destDir := filepath.Join(themesDir, entry.Name())
		if _, err := os.Stat(filepath.Join(destDir, "theme.yaml")); err == nil {
			continue
		}

		if err := os.MkdirAll(destDir, 0o755); err != nil {
			continue
		}

		srcPath := filepath.Join(sourceDir, entry.Name())
		if err := copyDir(srcPath, destDir); err != nil {
			continue
		}

		logger.Info("Seeded built-in theme", "theme", entry.Name())
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

	// Seed built-in themes from project root "themes/" into the data directory
	seedBuiltinThemes(themesDir, logger)

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
