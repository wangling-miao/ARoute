package queue

import (
	_ "embed"
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/wangling-miao/aroute/core"
	"github.com/wangling-miao/aroute/sdk/interfaces"
)

//go:embed manifest.yaml
var manifestData []byte

// Plugin implements core.Plugin for the queue service.
type Plugin struct {
	*core.BasePlugin

	mu      sync.RWMutex
	ctx     core.CoreContext
	service *Service
	running bool
}

// New creates a new queue plugin instance.
func New() *Plugin {
	manifest, err := core.ParseManifest(manifestData, ".yaml")
	if err != nil {
		panic("queue plugin: failed to parse embedded manifest: " + err.Error())
	}
	return &Plugin{
		BasePlugin: core.NewBasePluginFromManifest(manifest),
	}
}

// Init initializes the queue plugin with the core context.
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

	svc := NewService(cfg, logger)
	p.service = svc

	if err := ctx.Services().Provide(func(container core.ServiceContainer) (interfaces.QueueService, error) {
		return p.service, nil
	}); err != nil {
		return fmt.Errorf("failed to register QueueService: %w", err)
	}

	logger.Info("Queue plugin initialized",
		"workers", cfg.Workers,
		"shutdown_timeout", cfg.ShutdownTimeout,
		"default_max_retries", cfg.DefaultMaxRetries,
		"default_timeout", cfg.DefaultTimeout,
	)

	return nil
}

// Start starts the worker pool.
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

// Stop gracefully shuts down the worker pool.
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
