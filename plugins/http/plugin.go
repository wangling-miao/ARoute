// Package http provides the HTTP server plugin for Aroute CMS.
// It implements a production-ready HTTP server using chi router with
// middleware support, CORS, graceful shutdown, and health checks.
package http

import (
	"context"
	_ "embed"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/wangling-miao/aroute/core"
)

//go:embed manifest.yaml
var manifestData []byte

type tlsConfig struct {
	certFile string
	keyFile  string
}

// Plugin implements the core.Plugin interface for HTTP server functionality.
// It provides a production-ready HTTP server with middleware, CORS, health checks,
// static file serving, and graceful shutdown support.
type Plugin struct {
	*core.BasePlugin

	mu      sync.RWMutex
	ctx     core.CoreContext
	router  *chi.Mux
	server  *http.Server
	tls     tlsConfig
	running bool
}

// New creates a new HTTP plugin instance.
func New() *Plugin {
	manifest, err := core.ParseManifest(manifestData, ".yaml")
	if err != nil {
		panic("http plugin: failed to parse embedded manifest: " + err.Error())
	}
	return &Plugin{
		BasePlugin: core.NewBasePluginFromManifest(manifest),
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
// If TLS configuration is provided, it enables HTTPS.
func (p *Plugin) Start() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.running {
		return nil
	}

	logger := p.ctx.Logger()
	config := p.ctx.Config()

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

	p.tls.certFile = config.GetString("server.tls.cert_file")
	p.tls.keyFile = config.GetString("server.tls.key_file")

	p.server = &http.Server{
		Addr:         addr,
		Handler:      p.router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	logger.Info("Starting HTTP server", "address", addr, "tls", p.tls.certFile != "")

	go func() {
		var err error
		if p.tls.certFile != "" && p.tls.keyFile != "" {
			if _, errCert := os.Stat(p.tls.certFile); errCert != nil {
				logger.Error("TLS cert file not found", "path", p.tls.certFile, "error", errCert)
				return
			}
			if _, errKey := os.Stat(p.tls.keyFile); errKey != nil {
				logger.Error("TLS key file not found", "path", p.tls.keyFile, "error", errKey)
				return
			}
			err = p.server.ListenAndServeTLS(p.tls.certFile, p.tls.keyFile)
		} else {
			err = p.server.ListenAndServe()
		}
		if err != nil && err != http.ErrServerClosed {
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
