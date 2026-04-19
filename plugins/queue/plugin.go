package queue

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"strconv"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/wangling-miao/aroute/core"
	"github.com/wangling-miao/aroute/sdk/interfaces"
)

//go:embed manifest.yaml
var manifestData []byte

type Plugin struct {
	*core.BasePlugin

	mu        sync.RWMutex
	ctx       core.CoreContext
	service   *Service
	registrar interfaces.RouteRegistrar
	running   bool
}

func New() *Plugin {
	manifest, err := core.ParseManifest(manifestData, ".yaml")
	if err != nil {
		panic("queue plugin: failed to parse embedded manifest: " + err.Error())
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
	logger.Info("Initializing queue plugin")

	cfg := Config{
		Workers:           ctx.Config().GetInt("workers"),
		ShutdownTimeout:   time.Duration(ctx.Config().GetInt("shutdown_timeout_seconds")) * time.Second,
		DefaultMaxRetries: ctx.Config().GetInt("default_max_retries"),
		DefaultTimeout:    time.Duration(ctx.Config().GetInt("default_timeout_seconds")) * time.Second,
	}

	if cfg.Workers <= 0 {
		cfg.Workers = runtime.NumCPU()
	}
	if cfg.ShutdownTimeout <= 0 {
		cfg.ShutdownTimeout = 30 * time.Second
	}
	if cfg.DefaultMaxRetries <= 0 {
		cfg.DefaultMaxRetries = 3
	}
	if cfg.DefaultTimeout <= 0 {
		cfg.DefaultTimeout = 60 * time.Second
	}

	var dbSvc interfaces.DatabaseService
	if err := ctx.Services().Get(&dbSvc); err != nil {
		logger.Warn("Database service not available, running without persistence", "error", err)
	}

	svc := NewService(cfg, logger, dbSvc)

	if dbSvc != nil {
		if err := svc.InitDB(ctx.Context()); err != nil {
			return fmt.Errorf("init queue tables: %w", err)
		}
	}

	p.service = svc

	if err := ctx.Services().Provide(func(container core.ServiceContainer) (interfaces.QueueService, error) {
		return p.service, nil
	}); err != nil {
		return fmt.Errorf("failed to register QueueService: %w", err)
	}

	var registrar interfaces.RouteRegistrar
	if err := ctx.Services().Get(&registrar); err != nil {
		logger.Warn("Route registrar not available, admin API endpoints not registered", "error", err)
	} else {
		p.registrar = registrar
		p.registerAdminRoutes()
	}

	logger.Info("Queue plugin initialized",
		"workers", cfg.Workers,
		"shutdown_timeout", cfg.ShutdownTimeout,
		"default_max_retries", cfg.DefaultMaxRetries,
		"default_timeout", cfg.DefaultTimeout,
	)

	return nil
}

func (p *Plugin) Start() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.running {
		return nil
	}

	p.service.Start()
	p.ctx.Logger().Info("Queue plugin started")
	p.running = true
	return nil
}

func (p *Plugin) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.running {
		return nil
	}

	p.ctx.Logger().Info("Stopping queue plugin")
	if p.service != nil {
		if err := p.service.Close(p.ctx.Context()); err != nil {
			p.ctx.Logger().Error("error closing queue service", "error", err)
		}
	}
	p.running = false
	p.ctx.Logger().Info("Queue plugin stopped")
	return nil
}

func (p *Plugin) registerAdminRoutes() {
	if p.registrar == nil {
		return
	}

	handler := &adminHandler{service: p.service}

	p.registrar.Route("/admin/api/queue", func(r chi.Router) {
		r.Get("/dead-letter", handler.listDeadLetters)
		r.Post("/dead-letter/{taskID}/retry", handler.retryDeadLetter)
		r.Delete("/dead-letter/{taskID}", handler.deleteDeadLetter)
	})
}

type adminHandler struct {
	service *Service
}

func (h *adminHandler) listDeadLetters(w http.ResponseWriter, r *http.Request) {
	pageStr := r.URL.Query().Get("page")
	pageSizeStr := r.URL.Query().Get("page_size")

	page := 1
	pageSize := 20

	if v, err := strconv.Atoi(pageStr); err == nil && v > 0 {
		page = v
	}
	if v, err := strconv.Atoi(pageSizeStr); err == nil && v > 0 {
		pageSize = v
	}

	entries, total, err := h.service.ListDeadLetters(r.Context(), page, pageSize)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"data":      entries,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

func (h *adminHandler) retryDeadLetter(w http.ResponseWriter, r *http.Request) {
	taskID := chi.URLParam(r, "taskID")
	if taskID == "" {
		http.Error(w, "missing task_id", http.StatusBadRequest)
		return
	}

	if err := h.service.RetryDeadLetter(r.Context(), taskID); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (h *adminHandler) deleteDeadLetter(w http.ResponseWriter, r *http.Request) {
	taskID := chi.URLParam(r, "taskID")
	if taskID == "" {
		http.Error(w, "missing task_id", http.StatusBadRequest)
		return
	}

	if err := h.service.DeleteDeadLetter(r.Context(), taskID); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
