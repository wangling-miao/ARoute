// Package http provides the HTTP server plugin for Aroute CMS.
// It implements a production-ready HTTP server using chi router with
// middleware support, CORS, graceful shutdown, and health checks.
package http

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/wangling-miao/aroute/core"
)

// Plugin implements the core.Plugin interface for HTTP server functionality.
// It provides a production-ready HTTP server with middleware, CORS, health checks,
// static file serving, and graceful shutdown support.
type Plugin struct {
	*core.BasePlugin

	mu      sync.RWMutex
	ctx     core.CoreContext
	router  *chi.Mux
	server  *http.Server
	running bool
}

// New creates a new HTTP plugin instance.
func New() *Plugin {
	return &Plugin{
		BasePlugin: core.NewBasePlugin("http", "1.0.0"),
	}
}

// Name returns the plugin name.
func (p *Plugin) Name() string {
	return "http"
}

// Version returns the plugin version.
func (p *Plugin) Version() string {
	return "1.0.0"
}

// Manifest returns the plugin manifest.
func (p *Plugin) Manifest() *core.Manifest {
	return &core.Manifest{
		Name:        "http",
		Version:     "1.0.0",
		Description: "Production-ready HTTP server with middleware, CORS, health checks, and graceful shutdown",
		Author:      "Aroute Team",
		License:     "MIT",
		Engine:      "native",
		Requires:    []string{},
		After:       []string{},
		Provides:    []string{"http.server", "http.router"},
	}
}

// Init initializes the HTTP plugin.
// It sets up the router, registers services, and configures middleware.
func (p *Plugin) Init(ctx core.CoreContext) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.ctx = ctx
	logger := ctx.Logger()

	logger.Info("Initializing HTTP plugin")

	// Initialize chi router
	p.router = chi.NewRouter()

	// Setup global middleware FIRST (before any routes)
	p.setupMiddleware(logger)

	// Setup CORS (must be before routes)
	p.setupCORS(ctx)

	// Register RouteRegistrar service
	if err := ctx.Services().Provide(func(container core.ServiceContainer) (RouteRegistrar, error) {
		return NewRouteRegistrar(p.router), nil
	}); err != nil {
		return fmt.Errorf("failed to register RouteRegistrar service: %w", err)
	}

	// Setup health check endpoint
	p.setupHealthCheck()

	// Setup static file serving
	if err := p.setupStaticFiles(ctx); err != nil {
		logger.Warn("Static file setup failed, continuing without static files", "error", err)
	}

	logger.Info("HTTP plugin initialized successfully")
	return nil
}

// setupMiddleware configures the global middleware stack.
func (p *Plugin) setupMiddleware(logger *slog.Logger) {
	p.router.Use(middleware.RequestID)
	p.router.Use(middleware.RealIP)
	p.router.Use(middleware.Recoverer)
	p.router.Use(p.slogMiddleware(logger))
	p.router.Use(middleware.Timeout(60 * time.Second))

	logger.Debug("Middleware stack configured",
		"middlewares", []string{"RequestID", "RealIP", "Recoverer", "SlogLogger", "Timeout"},
	)
}

func (p *Plugin) slogMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

			defer func() {
				status := ww.Status()
				if status == 0 {
					status = 200
				}
				duration := time.Since(start)

				logger.Info("request completed",
					"method", r.Method,
					"path", r.URL.Path,
					"status", status,
					"bytes", ww.BytesWritten(),
					"duration", duration.String(),
					"request_id", middleware.GetReqID(r.Context()),
				)
			}()

			next.ServeHTTP(ww, r)
		})
	}
}

// Start starts the HTTP server.
// It listens on the configured address and serves requests.
func (p *Plugin) Start() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.running {
		return nil
	}

	logger := p.ctx.Logger()
	config := p.ctx.Config()

	// Get server configuration - check for http.addr first, then fall back to server.host/port
	addr := config.GetString("http.addr")
	if addr == "" {
		host := config.GetString("server.host")
		if host == "" {
			host = "0.0.0.0"
		}
		port := config.GetInt("server.port")
		if port == 0 {
			port = 8080
		}
		addr = fmt.Sprintf("%s:%d", host, port)
	}

	// Create HTTP server
	p.server = &http.Server{
		Addr:         addr,
		Handler:      p.router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	logger.Info("Starting HTTP server", "address", addr)

	// Start server in goroutine
	go func() {
		if err := p.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("HTTP server error", "error", err)
		}
	}()

	p.running = true
	logger.Info("HTTP server started successfully", "address", addr)

	return nil
}

// Stop gracefully shuts down the HTTP server.
// It waits for active connections to finish with a timeout.
func (p *Plugin) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.running || p.server == nil {
		return nil
	}

	logger := p.ctx.Logger()
	logger.Info("Stopping HTTP server")

	// Create shutdown context with timeout
	ctx, cancel := context.WithTimeout(p.ctx.Context(), 30*time.Second)
	defer cancel()

	// Graceful shutdown
	if err := p.server.Shutdown(ctx); err != nil {
		logger.Error("HTTP server shutdown error", "error", err)
		return fmt.Errorf("shutdown failed: %w", err)
	}

	p.running = false
	logger.Info("HTTP server stopped successfully")

	return nil
}
