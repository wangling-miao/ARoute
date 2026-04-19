package api

import (
	_ "embed"
	"fmt"
	"net/http"
	"sync"

	"github.com/go-chi/chi/v5"

	"github.com/wangling-miao/aroute/core"
	"github.com/wangling-miao/aroute/sdk/interfaces"
)

//go:embed manifest.yaml
var manifestData []byte

// Plugin implements the core.Plugin interface for the REST API.
type Plugin struct {
	*core.BasePlugin

	mu         sync.RWMutex
	ctx        core.CoreContext
	contentSvc interfaces.ContentService
	authSvc    interfaces.AuthService
	registrar  interfaces.RouteRegistrar
	handler    *Handler
	running    bool
}

// New creates a new API plugin instance.
func New() *Plugin {
	manifest, err := core.ParseManifest(manifestData, ".yaml")
	if err != nil {
		panic("api plugin: failed to parse embedded manifest: " + err.Error())
	}
	return &Plugin{
		BasePlugin: core.NewBasePluginFromManifest(manifest),
	}
}

// Init initializes the API plugin by resolving dependencies and registering routes.
func (p *Plugin) Init(ctx core.CoreContext) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.ctx = ctx
	logger := ctx.Logger()

	logger.Info("Initializing API plugin")

	if err := ctx.Services().Get(&p.contentSvc); err != nil {
		return fmt.Errorf("content service not available: %w", err)
	}
	logger.Info("Content service resolved")

	var authSvc interfaces.AuthService
	if err := ctx.Services().Get(&authSvc); err != nil {
		logger.Warn("Auth service not available, running in public API mode", "error", err)
	}
	p.authSvc = authSvc

	var registrar interfaces.RouteRegistrar
	if err := ctx.Services().Get(&registrar); err != nil {
		return fmt.Errorf("route registrar not available: %w", err)
	}
	p.registrar = registrar
	logger.Info("Route registrar resolved")

	p.handler = NewHandler(p.contentSvc)

	p.registerRoutes()

	logger.Info("API plugin initialized successfully",
		"auth_enabled", p.authSvc != nil,
	)

	return nil
}

// Start starts the API plugin.
func (p *Plugin) Start() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.running {
		return nil
	}

	p.ctx.Logger().Info("API plugin started successfully")
	p.running = true

	return nil
}

// Stop gracefully shuts down the API plugin.
func (p *Plugin) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.running {
		return nil
	}

	p.ctx.Logger().Info("Stopping API plugin")
	p.running = false
	p.ctx.Logger().Info("API plugin stopped successfully")

	return nil
}

func (p *Plugin) registerRoutes() {
	publicRead := false
	var config core.ConfigProvider
	if p.ctx != nil {
		config = p.ctx.Config()
	}
	if config != nil {
		publicRead = config.GetBool("api.public_read")
	}
	contentNeg := contentNegotiationMiddleware()
	rateLimitMW := rateLimitMiddleware(config)

	docsEnabled := true
	docsUI := "swagger"
	if config != nil {
		if v, ok := config.Get("api.docs.enabled").(bool); ok {
			docsEnabled = v
		}
		if v := config.GetString("api.docs.ui"); v != "" {
			docsUI = v
		}
	}

	if docsEnabled {
		p.registrar.Route("/api/v1/openapi.json", func(r chi.Router) {
			r.Get("/", p.handler.handleDocs)
		})
		uiHandler := p.handler.docsUIHandler(docsUI)
		p.registrar.Route("/api/docs", func(r chi.Router) {
			r.Get("/", uiHandler)
		})
	}

	p.registrar.Route("/api/v1", func(r chi.Router) {
		r.Use(contentNeg)
		r.Use(perContentTypeAuthMiddleware(p.authSvc, publicRead, config))
		r.Use(rateLimitMW)

		r.Get("/content-types", p.handler.ListContentTypes)

		r.Route("/{contentType}", func(cr chi.Router) {
			cr.Get("/", p.handler.List)
			cr.Post("/", p.handler.Create)

			cr.Route("/{id}", func(ir chi.Router) {
				ir.Get("/", p.handler.Get)
				ir.Put("/", p.handler.Update)
				ir.Delete("/", p.handler.Delete)
			})
		})
	})
}

func (h *Handler) docsUIHandler(uiType string) http.HandlerFunc {
	if uiType == "redoc" {
		return func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write([]byte(redocHTML))
		}
	}
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(swaggerUIHTML))
	}
}
