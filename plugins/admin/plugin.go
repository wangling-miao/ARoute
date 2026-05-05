// Package admin provides the Admin UI plugin for Aroute CMS.
// It serves the React SPA from data/plugin_data/admin/{variant}/ at /admin/,
// with support for development mode proxy to Vite dev server
// and hot-swappable admin UI variants.
package admin

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/wangling-miao/aroute/core"
	"github.com/wangling-miao/aroute/sdk/interfaces"
)

//go:embed manifest.yaml
var manifestData []byte

// Plugin implements the core.Plugin interface for Admin UI.
type Plugin struct {
	*core.BasePlugin

	mu            sync.RWMutex
	ctx           core.CoreContext
	running       bool
	handler       http.Handler
	devMode       bool
	devProxy      *httputil.ReverseProxy
	activeVariant string
	adminDir      string // data/plugin_data/admin/
}

// New creates a new Admin UI plugin instance.
func New() *Plugin {
	manifest, err := core.ParseManifest(manifestData, ".yaml")
	if err != nil {
		panic("admin plugin: failed to parse embedded manifest: " + err.Error())
	}
	return &Plugin{
		BasePlugin: core.NewBasePluginFromManifest(manifest),
	}
}

// findAdminDist locates the admin/dist build output directory.
func findAdminDist() string {
	candidates := []string{"admin/dist"}
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		candidates = append(candidates,
			filepath.Join(exeDir, "admin", "dist"),
			filepath.Join(exeDir, "..", "admin", "dist"),
		)
	}
	for _, dir := range candidates {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			return dir
		}
	}
	return ""
}

// seedAdminVariants seeds admin UI variant directories under adminDir.
// Each variant is a subdirectory: data/plugin_data/admin/{variant}/.
func seedAdminVariants(adminDir string, logger *slog.Logger) {
	sourceDir := findAdminDist()
	if sourceDir == "" {
		logger.Warn("Admin frontend assets not found, admin UI will not be available")
		return
	}

	seedVariant(adminDir, "default", sourceDir, map[string]string{
		"variant":     "default",
		"name":        "ARoute Admin",
		"version":     "1.0.0",
		"description": "Standard ARoute admin interface",
	}, logger)

	// Seed additional variants from sibling admin-* build directories.
	extraVariants := []struct {
		buildDir string
		name     string
		info     map[string]string
	}{
		{
			buildDir: "admin-compact/dist",
			name:     "compact",
			info: map[string]string{
				"variant":     "compact",
				"name":        "ARoute Compact",
				"version":     "1.0.0",
				"description": "Compact admin interface (no logo)",
			},
		},
	}

	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		for i := range extraVariants {
			extraVariants[i].buildDir = filepath.Join(exeDir, extraVariants[i].buildDir)
		}
	}

	for _, ev := range extraVariants {
		src := sourceDir // fallback to default build
		if info, err := os.Stat(ev.buildDir); err == nil && info.IsDir() {
			src = ev.buildDir
		}
		seedVariant(adminDir, ev.name, src, ev.info, logger)
	}
}

// seedVariant copies a build output into a variant directory if it doesn't already exist.
func seedVariant(adminDir, name, sourceDir string, manifest map[string]string, logger *slog.Logger) {
	variantDir := filepath.Join(adminDir, name)

	if _, err := os.Stat(filepath.Join(variantDir, "index.html")); err == nil {
		return
	}

	if err := os.MkdirAll(variantDir, 0o755); err != nil {
		logger.Error("Failed to create admin variant directory", "variant", name, "error", err)
		return
	}

	if err := copyDir(sourceDir, variantDir); err != nil {
		logger.Error("Failed to seed admin variant", "variant", name, "error", err)
		return
	}

	manifestPath := filepath.Join(variantDir, "variant.json")
	if _, err := os.Stat(manifestPath); err != nil {
		data, _ := json.MarshalIndent(manifest, "", "  ")
		os.WriteFile(manifestPath, data, 0o644)
	}

	logger.Info("Seeded admin variant", "variant", name)
}

// copyDir recursively copies all files from src to dst.
func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		destPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return os.MkdirAll(destPath, 0o755)
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(destPath, data, 0o644)
	})
}

// Init initializes the Admin UI plugin.
func (p *Plugin) Init(ctx core.CoreContext) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.ctx = ctx
	logger := ctx.Logger()

	logger.Info("Initializing Admin UI plugin")

	p.devMode = os.Getenv("AROUTE_DEV_MODE") == "true"
	p.adminDir = ctx.DataDir()

	p.activeVariant = "default"
	if variant := ctx.Config().GetString("admin.theme"); variant != "" {
		p.activeVariant = variant
	}

	if p.devMode {
		logger.Info("Admin UI running in development mode (proxy to Vite)")
		viteURL, _ := url.Parse("http://localhost:5173")
		p.devProxy = httputil.NewSingleHostReverseProxy(viteURL)
		defaultDirector := p.devProxy.Director
		p.devProxy.Director = func(req *http.Request) {
			defaultDirector(req)
			req.Host = viteURL.Host
		}
	} else {
		logger.Info("Admin UI running in production mode (file system)",
			"variant", p.activeVariant,
		)
		seedAdminVariants(p.adminDir, logger)
	}

	p.handler = p.buildHandler()

	ctx.Services().Provide(func(container core.ServiceContainer) (interfaces.AdminUISwitcher, error) {
		return p, nil
	})

	var registrar interfaces.RouteRegistrar
	if err := ctx.Services().Get(&registrar); err != nil {
		return fmt.Errorf("route registrar not available: %w", err)
	}

	registrar.Handle("/admin", http.HandlerFunc(p.serveAdmin))
	registrar.Handle("/admin/*", http.HandlerFunc(p.serveAdmin))

	logger.Info("Admin UI plugin initialized successfully",
		"dev_mode", p.devMode,
	)

	return nil
}

// buildHandler creates the HTTP handler for serving admin UI.
func (p *Plugin) buildHandler() http.Handler {
	if p.devMode {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p.devProxy.ServeHTTP(w, r)
		})
	}

	variantDir := filepath.Join(p.adminDir, p.activeVariant)
	fileServer := http.FileServer(http.Dir(variantDir))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/admin")
		if path == "" || path == "/" {
			p.serveIndexHTML(w, variantDir)
			return
		}

		cleanPath := strings.TrimPrefix(path, "/")
		targetPath := filepath.Join(variantDir, filepath.Clean(cleanPath))
		if info, err := os.Stat(targetPath); err == nil && !info.IsDir() {
			if strings.Contains(path, "/assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			}
			r.URL.Path = path
			fileServer.ServeHTTP(w, r)
			return
		}

		p.serveIndexHTML(w, variantDir)
	})
}

func (p *Plugin) serveIndexHTML(w http.ResponseWriter, adminDir string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	data, err := os.ReadFile(filepath.Join(adminDir, "index.html"))
	if err != nil {
		http.Error(w, "Admin UI not available", http.StatusInternalServerError)
		return
	}
	w.Write(data)
}

// serveAdmin handles all /admin/* requests.
func (p *Plugin) serveAdmin(w http.ResponseWriter, r *http.Request) {
	p.handler.ServeHTTP(w, r)
}

// Start starts the Admin UI plugin.
func (p *Plugin) Start() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.running {
		return nil
	}

	p.ctx.Logger().Info("Admin UI plugin started successfully")
	p.running = true

	return nil
}

// Stop gracefully shuts down the Admin UI plugin.
func (p *Plugin) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.running {
		return nil
	}

	p.ctx.Logger().Info("Stopping Admin UI plugin")
	p.running = false
	p.ctx.Logger().Info("Admin UI plugin stopped successfully")

	return nil
}

// ── AdminUISwitcher interface implementation ─────────────────────

// GetActiveVariant returns the currently active admin UI variant name.
func (p *Plugin) GetActiveVariant(_ context.Context) (string, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.activeVariant, nil
}

// SetActiveVariant switches the admin UI to the specified variant (hot swap).
func (p *Plugin) SetActiveVariant(_ context.Context, variant string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.devMode {
		return fmt.Errorf("cannot switch admin variant in development mode")
	}

	variantDir := filepath.Join(p.adminDir, variant)
	if _, err := os.Stat(filepath.Join(variantDir, "index.html")); err != nil {
		return fmt.Errorf("admin variant %q not found: %w", variant, interfaces.ErrNotFound)
	}

	previous := p.activeVariant
	p.activeVariant = variant

	p.ctx.Config().Set("admin.theme", variant)
	if err := p.ctx.Config().Save(); err != nil {
		p.activeVariant = previous
		return fmt.Errorf("persist admin variant config: %w", err)
	}

	p.handler = p.buildHandler()

	p.ctx.Logger().Info("Admin UI variant switched",
		"variant", variant,
		"previous", previous,
	)
	return nil
}

// ListVariants returns metadata for all available admin UI variants.
func (p *Plugin) ListVariants(_ context.Context) ([]interfaces.VariantInfo, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.adminDir == "" {
		return nil, nil
	}

	entries, err := os.ReadDir(p.adminDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read admin directory: %w", err)
	}

	variants := make([]interfaces.VariantInfo, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(p.adminDir, entry.Name(), "index.html")); err != nil {
			continue
		}

		info := interfaces.VariantInfo{
			Variant: entry.Name(),
			Active:  entry.Name() == p.activeVariant,
		}

		manifestPath := filepath.Join(p.adminDir, entry.Name(), "variant.json")
		if data, err := os.ReadFile(manifestPath); err == nil {
			var m struct {
				Name        string `json:"name"`
				Version     string `json:"version"`
				Description string `json:"description"`
			}
			if json.Unmarshal(data, &m) == nil {
				info.Name = m.Name
				info.Version = m.Version
				info.Description = m.Description
			}
		}

		if info.Name == "" {
			info.Name = entry.Name()
		}

		variants = append(variants, info)
	}

	return variants, nil
}
