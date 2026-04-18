package cache

import (
	_ "embed"
	"fmt"
	"sync"
	"time"

	"github.com/wangling-miao/aroute/core"
	"github.com/wangling-miao/aroute/sdk/interfaces"
)

//go:embed manifest.yaml
var manifestData []byte

type Plugin struct {
	*core.BasePlugin

	mu      sync.RWMutex
	ctx     core.CoreContext
	service *Service
	running bool
}

func New() *Plugin {
	manifest, err := core.ParseManifest(manifestData, ".yaml")
	if err != nil {
		panic("cache plugin: failed to parse embedded manifest: " + err.Error())
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
	logger.Info("Initializing cache plugin")

	cfg := Config{
		NumCounters: int64(ctx.Config().GetInt("num_counters")),
		MaxCost:     int64(ctx.Config().GetInt("max_cost")),
		BufferItems: int64(ctx.Config().GetInt("buffer_items")),
	}

	if cfg.NumCounters <= 0 {
		cfg.NumCounters = 1_000_000
	}
	if cfg.MaxCost <= 0 {
		cfg.MaxCost = 64 * 1024 * 1024
	}
	if cfg.BufferItems <= 0 {
		cfg.BufferItems = 64
	}

	defaultTTLSec := ctx.Config().GetInt("default_ttl_seconds")
	if defaultTTLSec > 0 {
		cfg.DefaultTTL = time.Duration(defaultTTLSec) * time.Second
	} else {
		cfg.DefaultTTL = 5 * time.Minute
	}

	cfg.WarmUp = WarmUpConfig{
		Enabled:      ctx.Config().GetBool("warmup_enabled"),
		ContentTypes: ctx.Config().GetStringSlice("warmup_content_types"),
		MaxItems:     ctx.Config().GetInt("warmup_max_items"),
	}

	svc, err := NewService(cfg, ctx.Events(), logger)
	if err != nil {
		return fmt.Errorf("create cache service: %w", err)
	}
	p.service = svc

	if err := ctx.Services().Provide(func(container core.ServiceContainer) (interfaces.CacheService, error) {
		return p.service, nil
	}); err != nil {
		return fmt.Errorf("failed to register CacheService: %w", err)
	}

	ctx.Events().SubscribeBroadcast("content.**", p.service.HandleContentEvent)
	ctx.Events().SubscribeBroadcast("content_type.**", p.service.HandleContentTypeEvent)

	// Register cache bypass middleware with HTTP router (best-effort)
	var registrar interfaces.RouteRegistrar
	if err := ctx.Services().Get(&registrar); err == nil {
		registrar.Use(BypassMiddleware)
		logger.Info("Cache bypass middleware registered")
	} else {
		logger.Debug("RouteRegistrar not available, cache bypass middleware not registered")
	}

	logger.Info("Cache plugin initialized",
		"num_counters", cfg.NumCounters,
		"max_cost", cfg.MaxCost,
		"default_ttl", cfg.DefaultTTL,
		"warmup_enabled", cfg.WarmUp.Enabled,
	)

	if cfg.WarmUp.Enabled {
		var contentSvc interfaces.ContentService
		if err := ctx.Services().Get(&contentSvc); err != nil {
			logger.Debug("cache warm-up skipped: ContentService not available")
		} else {
			if err := svc.WarmUp(ctx.Context(), contentSvc); err != nil {
				logger.Warn("cache warm-up encountered errors", "error", err)
			}
		}
	}

	return nil
}

func (p *Plugin) Start() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.running {
		return nil
	}

	p.ctx.Logger().Info("Cache plugin started")
	p.running = true
	return nil
}

func (p *Plugin) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.running {
		return nil
	}

	p.ctx.Logger().Info("Stopping cache plugin")
	if p.service != nil {
		p.service.Close()
	}
	p.running = false
	p.ctx.Logger().Info("Cache plugin stopped")
	return nil
}
