// Package admin provides the Admin UI plugin for Aroute CMS.
// It serves the React SPA from data/plugin_data/admin/ at /admin/,
// with support for development mode proxy to Vite dev server.
package admin

import (
	_ "embed"
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

	mu       sync.RWMutex
	ctx      core.CoreContext
	running  bool
	handler  http.Handler
	devMode  bool
	devProxy *httputil.ReverseProxy
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

// seedAdminAssets copies admin frontend assets from the project's admin/dist
// directory into the plugin's data directory if they don't already exist.
func seedAdminAssets(adminDir string, logger *slog.Logger) {
	var sourceDir string
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
			sourceDir = dir
			break
		}
	}

	if sourceDir == "" {
		logger.Warn("Admin frontend assets not found, admin UI will not be available")
		return
	}

	// Skip if already seeded (avoid overwriting user customizations)
	if _, err := os.Stat(filepath.Join(adminDir, "index.html")); err == nil {
		return
	}

	if err := os.MkdirAll(adminDir, 0o755); err != nil {
		logger.Error("Failed to create admin assets directory", "error", err)
		return
	}

	if err := copyDir(sourceDir, adminDir); err != nil {
		logger.Error("Failed to seed admin assets", "error", err)
		return
	}

	logger.Info("Seeded admin frontend assets")
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

	// Check dev mode
	p.devMode = os.Getenv("AROUTE_DEV_MODE") == "true"

	adminDir := ctx.DataDir()

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
		logger.Info("Admin UI running in production mode (file system)")
		seedAdminAssets(adminDir, logger)
	}

	// Build the handler
	p.handler = p.buildHandler()

	// Register routes via the RouteRegistrar service
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

	adminDir := p.ctx.DataDir()
	fileServer := http.FileServer(http.Dir(adminDir))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/admin")
		if path == "" || path == "/" {
			p.serveIndexHTML(w, adminDir)
			return
		}

		cleanPath := strings.TrimPrefix(path, "/")
		targetPath := filepath.Join(adminDir, filepath.Clean(cleanPath))
		if info, err := os.Stat(targetPath); err == nil && !info.IsDir() {
			if strings.Contains(path, "/assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			}
			r.URL.Path = path
			fileServer.ServeHTTP(w, r)
			return
		}

		p.serveIndexHTML(w, adminDir)
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
