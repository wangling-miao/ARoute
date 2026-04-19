package webhook

import (
	_ "embed"
	"fmt"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

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
			p.service.PruneOldDeliveries()
		}

		for {
			select {
			case <-ticker.C:
				if p.service != nil {
					p.service.PruneOldDeliveries()
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

	handler := &adminHandler{service: p.service}

	p.registrar.Route("/admin/api/webhooks", func(r chi.Router) {
		r.Get("/", handler.listWebhooks)
		r.Post("/", handler.createWebhook)
		r.Get("/{webhookID}", handler.getWebhook)
		r.Put("/{webhookID}", handler.updateWebhook)
		r.Patch("/{webhookID}", handler.patchWebhook)
		r.Delete("/{webhookID}", handler.deleteWebhook)
		r.Post("/{webhookID}/test", handler.testWebhook)
		r.Get("/{webhookID}/deliveries", handler.listDeliveries)
	})
}
