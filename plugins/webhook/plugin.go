package webhook

import (
	"context"
	_ "embed"
	"fmt"
	"sync"
	"time"

	"github.com/wangling-miao/aroute/core"
	"github.com/wangling-miao/aroute/sdk/interfaces"
)

//go:embed manifest.yaml
var manifestData []byte

// Plugin implements core.Plugin for the webhook service.
type Plugin struct {
	*core.BasePlugin

	mu        sync.RWMutex
	ctx       core.CoreContext
	service   *Service
	authSvc   interfaces.AuthService
	registrar interfaces.RouteRegistrar
	running   bool
	handlerID string
	stopPrune chan struct{}
}

// New creates a new webhook plugin instance.
func New() *Plugin {
	manifest, err := core.ParseManifest(manifestData, ".yaml")
	if err != nil {
		panic("webhook plugin: failed to parse embedded manifest: " + err.Error())
	}
	return &Plugin{
		BasePlugin: core.NewBasePluginFromManifest(manifest),
	}
}

// Init initializes the webhook plugin with the core context.
func (p *Plugin) Init(ctx core.CoreContext) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.ctx = ctx
	logger := ctx.Logger()
	logger.Info("Initializing webhook plugin")

	cfg := Config{
		DeliveryTimeout:          time.Duration(ctx.Config().GetInt("delivery_timeout_seconds")) * time.Second,
		MaxRetries:               ctx.Config().GetInt("max_retries"),
		MaxConsecutiveFailures:   ctx.Config().GetInt("max_consecutive_failures"),
		DeliveryLogRetentionDays: ctx.Config().GetInt("delivery_log_retention_days"),
	}

	if cfg.DeliveryTimeout <= 0 {
		cfg.DeliveryTimeout = 10 * time.Second
	}
	if cfg.MaxRetries <= 0 {
		cfg.MaxRetries = 5
	}
	if cfg.MaxConsecutiveFailures <= 0 {
		cfg.MaxConsecutiveFailures = 10
	}
	if cfg.DeliveryLogRetentionDays <= 0 {
		cfg.DeliveryLogRetentionDays = 30
	}

	svc := NewService(cfg, logger)
	svc.SetEventBus(ctx.Events())
	p.service = svc

	if err := ctx.Services().Provide(func(container core.ServiceContainer) (interfaces.WebhookService, error) {
		return p.service, nil
	}); err != nil {
		return fmt.Errorf("failed to register WebhookService: %w", err)
	}

	var authSvc interfaces.AuthService
	if err := ctx.Services().Get(&authSvc); err == nil {
		p.authSvc = authSvc
	} else {
		logger.Warn("Auth service not available, webhook admin endpoints will reject requests", "error", err)
	}

	var registrar interfaces.RouteRegistrar
	if err := ctx.Services().Get(&registrar); err != nil {
		logger.Warn("Route registrar not available, admin API endpoints not registered", "error", err)
	} else {
		p.registrar = registrar
		p.registerAdminRoutes()
	}

	p.handlerID = ctx.Events().SubscribeBroadcast("**", p.service.HandleEvent)

	logger.Info("Webhook plugin initialized",
		"delivery_timeout", cfg.DeliveryTimeout,
		"max_retries", cfg.MaxRetries,
		"max_consecutive_failures", cfg.MaxConsecutiveFailures,
		"delivery_log_retention_days", cfg.DeliveryLogRetentionDays,
	)

	return nil
}

// Start starts the webhook plugin.
func (p *Plugin) Start() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.running {
		return nil
	}

	p.running = true
	p.stopPrune = make(chan struct{})

	go func() {
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()

		if p.service != nil {
			p.service.PruneOldDeliveries(context.Background())
		}

		for {
			select {
			case <-ticker.C:
				if p.service != nil {
					p.service.PruneOldDeliveries(context.Background())
				}
			case <-p.stopPrune:
				return
			}
		}
	}()

	p.ctx.Logger().Info("Webhook plugin started")
	return nil
}

// Stop gracefully shuts down the webhook plugin.
func (p *Plugin) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.running {
		return nil
	}

	p.ctx.Logger().Info("Stopping webhook plugin")

	if p.stopPrune != nil {
		close(p.stopPrune)
		p.stopPrune = nil
	}

	if p.service != nil {
		if err := p.service.Close(); err != nil {
			p.ctx.Logger().Error("error closing webhook service", "error", err)
		}
	}
	if p.handlerID != "" {
		p.ctx.Events().Unsubscribe(p.handlerID)
		p.handlerID = ""
	}
	p.running = false
	p.ctx.Logger().Info("Webhook plugin stopped")
	return nil
}

func (p *Plugin) registerAdminRoutes() {
	if p.registrar == nil {
		return
	}

	handler := &adminHandler{service: p.service, authSvc: p.authSvc}

	p.registrar.HandleFunc("GET /admin/api/webhooks", handler.listWebhooks)
	p.registrar.HandleFunc("POST /admin/api/webhooks", handler.createWebhook)
	p.registrar.HandleFunc("GET /admin/api/webhooks/{webhookID}", handler.getWebhook)
	p.registrar.HandleFunc("PUT /admin/api/webhooks/{webhookID}", handler.updateWebhook)
	p.registrar.HandleFunc("PATCH /admin/api/webhooks/{webhookID}", handler.patchWebhook)
	p.registrar.HandleFunc("DELETE /admin/api/webhooks/{webhookID}", handler.deleteWebhook)
	p.registrar.HandleFunc("POST /admin/api/webhooks/{webhookID}/test", handler.testWebhook)
	p.registrar.HandleFunc("GET /admin/api/webhooks/{webhookID}/deliveries", handler.listDeliveries)
}
