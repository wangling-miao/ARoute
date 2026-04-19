// Package admin provides the Admin UI plugin for Aroute CMS.
// It serves the React SPA embedded via go:embed at /admin/,
// with support for development mode proxy to Vite dev server.
package admin

import (
	_ "embed"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"sync"

	"github.com/go-chi/chi/v5"

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

// Init initializes the Admin UI plugin.
func (p *Plugin) Init(ctx core.CoreContext) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.ctx = ctx
	logger := ctx.Logger()

	logger.Info("Initializing Admin UI plugin")

	// Check dev mode
	p.devMode = os.Getenv("AROUTE_DEV_MODE") == "true"

	if p.devMode {
		logger.Info("Admin UI running in development mode (proxy to Vite)")
		viteURL, _ := url.Parse("http://localhost:5173")
		p.devProxy = httputil.NewSingleHostReverseProxy(viteURL)
		// Rewrite to preserve /admin/ prefix
		defaultDirector := p.devProxy.Director
		p.devProxy.Director = func(req *http.Request) {
			defaultDirector(req)
			req.Host = viteURL.Host
		}
	} else {
		logger.Info("Admin UI running in production mode (embedded assets)")
	}

	// Build the handler
	p.handler = p.buildHandler()

	// Register routes via the RouteRegistrar service
	var registrar interfaces.RouteRegistrar
	if err := ctx.Services().Get(&registrar); err != nil {
		return fmt.Errorf("route registrar not available: %w", err)
	}

	registrar.Route("/admin", func(r chi.Router) {
		r.HandleFunc("/*", p.serveAdmin)
	})

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

	subFS, err := fs.Sub(adminDistFS, "dist")
	if err != nil {
		p.ctx.Logger().Error("Failed to create sub filesystem for admin UI", "error", err)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "Admin UI assets not available", http.StatusInternalServerError)
		})
	}

	fileServer := http.FileServer(http.FS(subFS))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/admin")
		if path == "" || path == "/" {
			p.serveIndexHTML(w, subFS)
			return
		}

		cleanPath := strings.TrimPrefix(path, "/")
		f, err := subFS.Open(cleanPath)
		if err == nil {
			f.Close()
			if strings.Contains(path, "/assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			}
			r.URL.Path = path
			fileServer.ServeHTTP(w, r)
			return
		}

		p.serveIndexHTML(w, subFS)
	})
}

func (p *Plugin) serveIndexHTML(w http.ResponseWriter, fsys fs.FS) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	data, err := fs.ReadFile(fsys, "index.html")
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
