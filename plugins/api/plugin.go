package api

import (
	_ "embed"
	"fmt"
	"net/http"
	"sync"

	"github.com/wangling-miao/aroute/core"
	"github.com/wangling-miao/aroute/sdk/interfaces"
)

//go:embed manifest.yaml
var manifestData []byte

// Plugin implements the core.Plugin interface for the REST API.
type Plugin struct {
	*core.BasePlugin

	mu           sync.RWMutex
	ctx          core.CoreContext
	contentSvc   interfaces.ContentService
	authSvc      interfaces.AuthService
	registrar    interfaces.RouteRegistrar
	handler      *Handler
	adminHandler *AdminHandler
	running      bool
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
	p.handler.authSvc = p.authSvc
	p.adminHandler = NewAdminHandler(p.ctx, p.contentSvc, p.authSvc)

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

	// Register middleware via Use (applied as outer wrappers by HTTP plugin at start)
	p.registrar.Use(contentNeg)
	p.registrar.Use(perContentTypeAuthMiddleware(p.authSvc, publicRead, config))
	p.registrar.Use(rateLimitMW)

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
		p.registrar.HandleFunc("GET /api/v1/openapi.json", p.handler.handleDocs)
		uiHandler := p.handler.docsUIHandler(docsUI)
		p.registrar.HandleFunc("GET /api/docs", uiHandler)
	}

	p.registrar.HandleFunc("GET /api/v1/content-types", p.handler.ListContentTypes)
	p.registrar.HandleFunc("GET /api/v1/content-types/{name}", p.handler.GetContentType)
	p.registrar.HandleFunc("POST /api/v1/content-types", p.handler.CreateContentType)
	p.registrar.HandleFunc("PUT /api/v1/content-types/{name}", p.handler.UpdateContentType)
	p.registrar.HandleFunc("DELETE /api/v1/content-types/{name}", p.handler.DeleteContentType)

	p.registrar.HandleFunc("GET /api/v1/content/{contentType}", p.handler.List)
	p.registrar.HandleFunc("POST /api/v1/content/{contentType}", p.handler.Create)
	p.registrar.HandleFunc("GET /api/v1/content/{contentType}/{id}", p.handler.Get)
	p.registrar.HandleFunc("PUT /api/v1/content/{contentType}/{id}", p.handler.Update)
	p.registrar.HandleFunc("DELETE /api/v1/content/{contentType}/{id}", p.handler.Delete)

	p.registrar.HandleFunc("GET /api/v1/dashboard/stats", p.adminHandler.handleDashboardStats)
	p.registrar.HandleFunc("GET /api/v1/settings", p.adminHandler.handleGetSettings)
	p.registrar.HandleFunc("PUT /api/v1/settings", p.adminHandler.handleUpdateSettings)
	p.registrar.HandleFunc("GET /api/v1/plugins", p.adminHandler.handleListPlugins)
	p.registrar.HandleFunc("POST /api/v1/plugins/{name}/enable", p.adminHandler.handleEnablePlugin)
	p.registrar.HandleFunc("POST /api/v1/plugins/{name}/disable", p.adminHandler.handleDisablePlugin)
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
