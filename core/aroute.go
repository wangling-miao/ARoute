package core

import (
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Aroute is the main engine that integrates all core subsystems.
type Aroute struct {
	mu sync.RWMutex

	// Core subsystems (interfaces)
	container  ServiceContainer
	eventBus   EventBus
	registry   PluginRegistry
	lifecycle  LifecycleManager
	dispatcher EngineDispatcher
	license    LicenseValidator
	ddl        DDLRegistry
	routing    RoutingRegistry

	// Configuration
	config *Config

	// State
	started bool
	ctx     context.Context
	cancel  context.CancelFunc

	// Directories
	dataDir   string
	pluginDir string
}

// PluginRegistry defines the interface for plugin metadata storage.
type PluginRegistry interface {
	Register(entry *PluginEntry) error
	Get(name string) (*PluginEntry, error)
	List() ([]*PluginEntry, error)
	Update(name string, manifest Manifest) error
	Remove(name string) error
	Enable(name string) error
	Disable(name string) error
	Close() error
}

// RoutingRegistry provides unified route management with cross-type dependency ordering.
// Implemented by registry.BoltUnifiedRegistry; nil when the unified registry is unavailable.
type RoutingRegistry interface {
	PluginStartupOrder() ([]string, error)
}

// PluginEntry represents a registered plugin with its manifest and state.
type PluginEntry struct {
	Manifest       Manifest
	Enabled        bool
	DiscoveredPath string
}

// LifecycleManager defines the interface for plugin lifecycle management.
type LifecycleManager interface {
	LoadAll(ctx context.Context) error
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	Enable(ctx context.Context, pluginName string) error
	Disable(ctx context.Context, pluginName string) error
	GetState(pluginName string) (PluginState, error)
	GetPlugin(pluginName string) Plugin
	ListPlugins() []string
}

// EngineDispatcher defines the interface for plugin execution.
type EngineDispatcher interface {
	RegisterEngine(engineType EngineType, engine EngineExecutor) error
	GetEngine(engineType EngineType) (EngineExecutor, error)
	Execute(ctx context.Context, plugin Plugin, manifest *Manifest, coreCtx CoreContext) error
	Close() error
}

// EngineExecutor defines the interface for a plugin execution backend.
type EngineExecutor interface {
	Type() EngineType
	Initialize(ctx context.Context) error
	ExecuteLifecycle(ctx context.Context, plugin Plugin, coreCtx CoreContext) error
	Close() error
}

// LicenseValidator defines the interface for license validation.
type LicenseValidator interface {
	Tier() LicenseTier
	IsFeatureAllowed(feature string) bool
	IsExpired() bool
	Validate() error
	LicenseInfo() LicenseInfoResult
}

// LicenseTier represents the license tier level.
type LicenseTier int

const (
	LicenseTierOpen LicenseTier = iota
	LicenseTierPro
	LicenseTierEnterprise
)

func (t LicenseTier) String() string {
	switch t {
	case LicenseTierOpen:
		return "open"
	case LicenseTierPro:
		return "pro"
	case LicenseTierEnterprise:
		return "enterprise"
	default:
		return "unknown"
	}
}

// LicenseInfoResult represents license state information.
type LicenseInfoResult struct {
	Tier      LicenseTier
	Features  []string
	ExpiresAt *time.Time
}

// DDLRegistry defines the interface for schema management.
type DDLRegistry interface {
	Init(ctx context.Context) error
	Create(ctx context.Context, schema interface{}) error
	Get(ctx context.Context, name string) (interface{}, error)
	Update(ctx context.Context, schema interface{}) error
	Delete(ctx context.Context, name string, force bool) error
	List(ctx context.Context) ([]interface{}, error)
}

// Config holds the configuration for the Aroute engine.
type Config struct {
	DataDir     string
	PluginDir   string
	LicensePath string
	PublicKey   *ecdsa.PublicKey
	Logger      *slog.Logger
}

// Option is a functional option for configuring the Aroute engine.
type Option func(*Config)

// WithDataDir sets the data directory.
func WithDataDir(dir string) Option {
	return func(c *Config) { c.DataDir = dir }
}

// WithPluginDir sets the plugin directory.
func WithPluginDir(dir string) Option {
	return func(c *Config) { c.PluginDir = dir }
}

// WithLicensePath sets the license file path.
func WithLicensePath(path string) Option {
	return func(c *Config) { c.LicensePath = path }
}

// WithPublicKey sets the public key for license verification.
func WithPublicKey(key *ecdsa.PublicKey) Option {
	return func(c *Config) { c.PublicKey = key }
}

// WithLogger sets the base logger.
func WithLogger(logger *slog.Logger) Option {
	return func(c *Config) { c.Logger = logger }
}

// New creates a new Aroute engine.
// The subsystems must be injected - this allows for flexible testing and configuration.
func New(ctx context.Context, container ServiceContainer, eventBus EventBus, registry PluginRegistry, lifecycle LifecycleManager, dispatcher EngineDispatcher, license LicenseValidator, routing RoutingRegistry, opts ...Option) (*Aroute, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("determine home directory: %w", err)
	}
	cfg := &Config{
		DataDir:   filepath.Join(homeDir, ".aroute", "data"),
		PluginDir: filepath.Join(homeDir, ".aroute", "plugins"),
	}
	for _, opt := range opts {
		opt(cfg)
	}

	logger := cfg.Logger
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		}))
	}

	if err := os.MkdirAll(cfg.DataDir, 0755); err != nil {
		return nil, fmt.Errorf("create data directory: %w", err)
	}
	if err := os.MkdirAll(cfg.PluginDir, 0755); err != nil {
		return nil, fmt.Errorf("create plugin directory: %w", err)
	}

	engineCtx, cancel := context.WithCancel(ctx)

	if err := license.Validate(); err != nil {
		cancel()
		return nil, fmt.Errorf("validate license: %w", err)
	}

	logger.Info("aroute engine initialized",
		"data_dir", cfg.DataDir,
		"plugin_dir", cfg.PluginDir,
		"tier", license.Tier().String(),
	)

	return &Aroute{
		container:  container,
		eventBus:   eventBus,
		registry:   registry,
		lifecycle:  lifecycle,
		dispatcher: dispatcher,
		license:    license,
		routing:    routing,
		config:     cfg,
		ctx:        engineCtx,
		cancel:     cancel,
		dataDir:    cfg.DataDir,
		pluginDir:  cfg.PluginDir,
	}, nil
}

// Start loads and starts all plugins in dependency order.
func (a *Aroute) Start(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.started {
		return fmt.Errorf("aroute: already started")
	}

	if err := a.lifecycle.LoadAll(ctx); err != nil {
		return fmt.Errorf("load plugins: %w", err)
	}

	if err := a.lifecycle.Start(ctx); err != nil {
		return fmt.Errorf("start plugins: %w", err)
	}

	if a.ddl != nil {
		if err := a.ddl.Init(ctx); err != nil {
			return fmt.Errorf("init ddl: %w", err)
		}
	}

	a.started = true
	return nil
}

// Stop gracefully shuts down the engine.
func (a *Aroute) Stop(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if !a.started {
		return nil
	}

	var errs []error

	if err := a.lifecycle.Stop(ctx); err != nil {
		errs = append(errs, fmt.Errorf("stop plugins: %w", err))
	}

	if err := a.dispatcher.Close(); err != nil {
		errs = append(errs, fmt.Errorf("close dispatcher: %w", err))
	}

	if err := a.registry.Close(); err != nil {
		errs = append(errs, fmt.Errorf("close registry: %w", err))
	}

	a.cancel()
	a.started = false

	if len(errs) > 0 {
		return fmt.Errorf("aroute stop errors: %v", errs)
	}
	return nil
}

// Services returns the service container.
func (a *Aroute) Services() ServiceContainer {
	return a.container
}

// Events returns the event bus.
func (a *Aroute) Events() EventBus {
	return a.eventBus
}

// Registry returns the plugin registry.
func (a *Aroute) Registry() PluginRegistry {
	return a.registry
}

// Routing returns the unified routing registry, or nil if not available.
func (a *Aroute) Routing() RoutingRegistry {
	return a.routing
}

// Lifecycle returns the lifecycle manager.
func (a *Aroute) Lifecycle() LifecycleManager {
	return a.lifecycle
}

// Dispatcher returns the engine dispatcher.
func (a *Aroute) Dispatcher() EngineDispatcher {
	return a.dispatcher
}

// License returns the license validator.
func (a *Aroute) License() LicenseValidator {
	return a.license
}

// DDL returns the DDL registry.
func (a *Aroute) DDL() DDLRegistry {
	return a.ddl
}

// SetDDL sets the DDL registry.
func (a *Aroute) SetDDL(ddl DDLRegistry) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.ddl = ddl
}

// Context returns the engine's context.
func (a *Aroute) Context() context.Context {
	return a.ctx
}

// IsStarted returns true if the engine is started.
func (a *Aroute) IsStarted() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.started
}

// RegisterPlugin registers a native plugin programmatically.
func (a *Aroute) RegisterPlugin(plugin Plugin) error {
	manifest := plugin.Manifest()
	if manifest == nil {
		return fmt.Errorf("aroute: plugin manifest is nil")
	}

	entry := &PluginEntry{
		Manifest: *manifest,
		Enabled:  true,
	}

	if err := a.registry.Register(entry); err != nil {
		return fmt.Errorf("register plugin %s: %w", plugin.Name(), err)
	}

	return nil
}

// DataDir returns the data directory.
func (a *Aroute) DataDir() string {
	return a.dataDir
}

// PluginDir returns the plugin directory.
func (a *Aroute) PluginDir() string {
	return a.pluginDir
}

// ScopedConfig implements ConfigProvider for a plugin's namespace.
type ScopedConfig struct {
	prefix string
	base   map[string]interface{}
}

func (c *ScopedConfig) GetString(key string) string {
	if v, ok := c.base[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func (c *ScopedConfig) GetInt(key string) int {
	if v, ok := c.base[key]; ok {
		switch n := v.(type) {
		case int:
			return n
		case int64:
			return int(n)
		case float64:
			return int(n)
		}
	}
	return 0
}

func (c *ScopedConfig) GetBool(key string) bool {
	if v, ok := c.base[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}

func (c *ScopedConfig) GetStringSlice(key string) []string {
	if v, ok := c.base[key]; ok {
		if s, ok := v.([]string); ok {
			return s
		}
	}
	return nil
}

func (c *ScopedConfig) Get(key string) interface{} {
	return c.base[key]
}

func (c *ScopedConfig) Unmarshal(key string, target interface{}) error {
	val, ok := c.base[key]
	if !ok {
		return fmt.Errorf("config: key %q not found", key)
	}

	data, err := json.Marshal(val)
	if err != nil {
		return fmt.Errorf("config: marshal value for key %q: %w", key, err)
	}

	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("config: unmarshal key %q into target: %w", key, err)
	}

	return nil
}

// NewScopedConfig creates a scoped config for a plugin.
func NewScopedConfig(pluginName string, base map[string]interface{}) *ScopedConfig {
	return &ScopedConfig{
		prefix: "plugins." + pluginName,
		base:   base,
	}
}

// LoadLicenseFile loads a license from a JSON file.
func LoadLicenseFile(path string) (map[string]interface{}, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read license file: %w", err)
	}

	var lic map[string]interface{}
	if err := json.Unmarshal(data, &lic); err != nil {
		return nil, fmt.Errorf("parse license: %w", err)
	}

	return lic, nil
}
