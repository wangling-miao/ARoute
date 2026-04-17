package theme

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/wangling-miao/aroute/core"
	"github.com/wangling-miao/aroute/core/events"
	"github.com/wangling-miao/aroute/sdk/interfaces"
)

// LuaEngine is defined in engine_lua.go.
// ReactSSREngine is defined in engine_react.go.

type Service struct {
	store  *Store
	events core.EventBus
	logger *slog.Logger

	mu          sync.RWMutex
	activeTheme string
	themesDir   string
	themes      map[string]*ThemeManifest
	goEngine    *GoTemplateEngine
	luaEngine   *LuaEngine
	reactEngine *ReactSSREngine
}

func NewService(store *Store, ev core.EventBus, logger *slog.Logger, activeTheme string) *Service {
	return &Service{
		store:       store,
		events:      ev,
		logger:      logger,
		activeTheme: activeTheme,
		themes:      make(map[string]*ThemeManifest),
	}
}

// LoadThemes scans the themes directory and loads each theme.yaml manifest.
func (s *Service) LoadThemes(themesDir string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.themesDir = themesDir

	entries, err := os.ReadDir(themesDir)
	if err != nil {
		if os.IsNotExist(err) {
			s.logger.Info("Themes directory does not exist, creating", "path", themesDir)
			return os.MkdirAll(themesDir, 0o755)
		}
		return fmt.Errorf("read themes directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		manifestPath := filepath.Join(themesDir, entry.Name(), "theme.yaml")
		manifest, err := LoadThemeManifest(manifestPath)
		if err != nil {
			s.logger.Warn("Failed to load theme manifest, skipping",
				"path", manifestPath, "error", err,
			)
			continue
		}
		s.themes[manifest.Name] = manifest
		s.logger.Info("Loaded theme manifest",
			"name", manifest.Name,
			"engine", manifest.Engine,
			"version", manifest.Version,
		)
	}

	s.logger.Info("Themes loaded", "count", len(s.themes))

	if manifest, ok := s.themes[s.activeTheme]; ok {
		if err := s.reloadActiveEngineLocked(manifest); err != nil {
			s.logger.Error("Failed to initialize active engine",
				"theme", s.activeTheme, "error", err,
			)
		}
	}

	return nil
}

func (s *Service) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.goEngine != nil {
		s.goEngine = nil
	}
	if s.luaEngine != nil {
		s.luaEngine.Close()
		s.luaEngine = nil
	}
	if s.reactEngine != nil {
		s.reactEngine.Close()
		s.reactEngine = nil
	}
}

func (s *Service) Render(ctx context.Context, templateName string, data map[string]interface{}) (string, error) {
	s.mu.RLock()
	activeSlug := s.activeTheme
	manifest, ok := s.themes[activeSlug]
	s.mu.RUnlock()

	if !ok {
		return "", fmt.Errorf("active theme %q not found: %w", activeSlug, interfaces.ErrNotFound)
	}

	switch manifest.Engine {
	case "gotemplate":
		if s.goEngine == nil {
			return "", fmt.Errorf("go template engine not initialized")
		}
		return s.goEngine.Render(templateName, data)
	case "lua":
		if s.luaEngine == nil {
			return "", fmt.Errorf("lua engine not initialized")
		}
		return s.luaEngine.Render(templateName, data)
	case "react":
		if s.reactEngine == nil {
			return "", fmt.Errorf("react SSR engine not initialized")
		}
		return s.reactEngine.Render(templateName, data)
	default:
		return "", fmt.Errorf("unsupported engine %q for theme %q", manifest.Engine, activeSlug)
	}
}

func (s *Service) GetActiveTheme(ctx context.Context) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.activeTheme, nil
}

func (s *Service) SetActiveTheme(ctx context.Context, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	manifest, ok := s.themes[name]
	if !ok {
		return fmt.Errorf("theme %q not found: %w", name, interfaces.ErrNotFound)
	}

	if err := s.store.SetActive(ctx, name); err != nil {
		return fmt.Errorf("set active theme in store: %w", err)
	}

	previous := s.activeTheme
	s.activeTheme = name

	if err := s.reloadActiveEngineLocked(manifest); err != nil {
		s.logger.Error("Failed to reload active engine",
			"theme", name, "engine", manifest.Engine, "error", err,
		)
	}

	s.emitEvent(ctx, "theme.activated", map[string]interface{}{
		"theme":    name,
		"engine":   manifest.Engine,
		"previous": previous,
	})

	s.logger.Info("Active theme changed",
		"theme", name,
		"engine", manifest.Engine,
		"previous", previous,
	)
	return nil
}

func (s *Service) ListThemes(ctx context.Context) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	names := make([]string, 0, len(s.themes))
	for name := range s.themes {
		names = append(names, name)
	}
	return names, nil
}

func (s *Service) InstallTheme(ctx context.Context, sourcePath string) error {
	manifest, err := LoadThemeManifest(filepath.Join(sourcePath, "theme.yaml"))
	if err != nil {
		return fmt.Errorf("validate theme manifest: %w", err)
	}

	slug := slugifyName(manifest.Name)

	s.mu.Lock()
	themesDir := s.themesDir
	s.mu.Unlock()

	destDir := filepath.Join(themesDir, slug)
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("create theme directory: %w", err)
	}

	if err := copyDir(sourcePath, destDir); err != nil {
		return fmt.Errorf("copy theme files: %w", err)
	}

	settingsJSON := "{}"
	record := &ThemeRecord{
		Name:     manifest.Name,
		Slug:     slug,
		Version:  manifest.Version,
		Engine:   manifest.Engine,
		Active:   false,
		Settings: settingsJSON,
	}

	if err := s.store.Create(ctx, record); err != nil {
		return fmt.Errorf("register theme in database: %w", err)
	}

	s.mu.Lock()
	s.themes[manifest.Name] = manifest
	s.mu.Unlock()

	s.emitEvent(ctx, "theme.installed", map[string]interface{}{
		"name":    manifest.Name,
		"slug":    slug,
		"engine":  manifest.Engine,
		"version": manifest.Version,
	})

	s.logger.Info("Theme installed",
		"name", manifest.Name,
		"slug", slug,
		"engine", manifest.Engine,
	)
	return nil
}

func (s *Service) reloadActiveEngineLocked(manifest *ThemeManifest) error {
	switch manifest.Engine {
	case "gotemplate":
		s.goEngine = NewGoTemplateEngine(s.themesDir, s.activeTheme, s.logger)
		s.luaEngine = nil
		s.reactEngine = nil
	case "lua":
		s.goEngine = nil
		s.luaEngine = NewLuaEngine(s.themesDir, s.activeTheme, s.logger, 10)
		s.reactEngine = nil
	case "react":
		s.goEngine = nil
		s.luaEngine = nil
		s.reactEngine = NewReactSSREngine(s.themesDir, s.activeTheme, s.logger, 4)
	default:
		return fmt.Errorf("unsupported engine: %s", manifest.Engine)
	}
	return nil
}

func (s *Service) emitEvent(ctx context.Context, topic string, data map[string]interface{}) {
	if s.events == nil {
		return
	}
	s.events.Emit(ctx, events.Event{
		Topic: topic,
		Data:  data,
	})
}

func slugifyName(name string) string {
	s := strings.ToLower(name)
	s = strings.ReplaceAll(s, " ", "-")
	return s
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		destPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return os.MkdirAll(destPath, info.Mode())
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(destPath, data, info.Mode())
	})
}
