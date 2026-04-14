package core

import (
	"context"
	"log/slog"

	"github.com/wangling-miao/aroute/core/events"
)

// CoreContext provides plugins with access to Core services and resources.
// It's passed to plugins during initialization and provides the bridge
// between plugins and the Core microkernel.
//
// CoreContext is read-only after creation - plugins cannot replace
// the container, event bus, or other core references.
// It is safe for concurrent use across goroutines.
type CoreContext interface {
	// Services returns the service container for dependency injection.
	// Plugins use this to obtain and provide services to other plugins.
	Services() ServiceContainer

	// Events returns the event bus for plugin communication.
	// Plugins use this to subscribe to events and emit events.
	Events() EventBus

	// Config returns the configuration provider for this plugin.
	// The ConfigProvider is scoped to the plugin's own configuration namespace.
	Config() ConfigProvider

	// Logger returns a structured logger pre-configured with the plugin's name.
	// All plugins should use this logger for consistent log formatting.
	Logger() *slog.Logger

	// DataDir returns the plugin-specific data directory.
	// Plugins can use this for persistent storage.
	// Path: ~/.aroute/data/plugins/{plugin-name}/
	DataDir() string

	// PluginDir returns the plugin installation directory.
	// Path: ~/.aroute/plugins/{plugin-name}/
	PluginDir() string

	// Context returns the context for request-scoped values and cancellation.
	// Use this for coordinating plugin shutdown.
	Context() context.Context
}

// coreContextImpl is the concrete implementation of CoreContext.
// It holds the actual data and implements the getter methods.
type coreContextImpl struct {
	services  ServiceContainer
	events    EventBus
	config    ConfigProvider
	logger    *slog.Logger
	dataDir   string
	pluginDir string
	ctx       context.Context
}

// NewCoreContext creates a new CoreContext with the provided values.
func NewCoreContext(ctx context.Context, services ServiceContainer, events EventBus, config ConfigProvider, logger *slog.Logger, dataDir, pluginDir string) CoreContext {
	return &coreContextImpl{
		services:  services,
		events:    events,
		config:    config,
		logger:    logger,
		dataDir:   dataDir,
		pluginDir: pluginDir,
		ctx:       ctx,
	}
}

func (c *coreContextImpl) Services() ServiceContainer { return c.services }
func (c *coreContextImpl) Events() EventBus           { return c.events }
func (c *coreContextImpl) Config() ConfigProvider     { return c.config }
func (c *coreContextImpl) Logger() *slog.Logger       { return c.logger }
func (c *coreContextImpl) DataDir() string            { return c.dataDir }
func (c *coreContextImpl) PluginDir() string          { return c.pluginDir }
func (c *coreContextImpl) Context() context.Context   { return c.ctx }

// ServiceContainer defines the interface for dependency injection.
// Plugins use this to register services (Provide) and obtain services (Get).
//
// The container uses lazy initialization - services are created on first Get.
// It's safe for concurrent use.
type ServiceContainer interface {
	// Provide registers a service provider.
	// The provider is called lazily on first Get.
	// Returns an error if a service with the same type is already registered.
	Provide(provider interface{}) error

	// Get retrieves a service by type.
	// Returns an error if the service is not found or initialization failed.
	Get(target interface{}) error

	// GetNamed retrieves a named service instance.
	// Use when multiple instances of the same type exist.
	GetNamed(name string, target interface{}) error

	// Unregister removes a service from the container.
	// Used for hot-plug support when disabling plugins.
	Unregister(target interface{}) error

	// Has checks if a service is registered.
	Has(target interface{}) bool

	// Keys returns all registered service names.
	Keys() []string
}

// EventBus defines the interface for the event system.
// Supports two modes: Filter (sequential, can abort) and Broadcast (concurrent, fire-and-forget).
// The interface is satisfied directly by *events.EventBus.
type EventBus interface {
	// SubscribeFilter registers a handler for filter-style events.
	// Filter handlers are called in priority order and can abort the chain.
	// Returns a handler ID for unsubscribe.
	SubscribeFilter(topic string, priority int, handler events.FilterHandler) string

	// SubscribeBroadcast registers a handler for broadcast-style events.
	// Broadcast handlers are called concurrently, errors are logged but don't abort.
	// Returns a handler ID for unsubscribe.
	SubscribeBroadcast(topic string, handler events.BroadcastHandler) string

	// Emit sends a broadcast event to all matching handlers.
	// Handlers are called concurrently. Errors are logged but don't affect other handlers.
	Emit(ctx context.Context, event events.Event)

	// DispatchFilter sends a filter event through the handler chain.
	// Handlers are called in priority order. A handler can abort by returning error.
	// The modified event is passed through the chain.
	DispatchFilter(ctx context.Context, event *events.Event) (*events.Event, error)

	// Unsubscribe removes a handler by ID.
	Unsubscribe(handlerID string)
}

// ConfigProvider provides configuration values to plugins.
type ConfigProvider interface {
	// GetString returns a string configuration value.
	GetString(key string) string

	// GetInt returns an integer configuration value.
	GetInt(key string) int

	// GetBool returns a boolean configuration value.
	GetBool(key string) bool

	// GetStringSlice returns a string slice configuration value.
	GetStringSlice(key string) []string

	// Get returns a raw configuration value.
	Get(key string) interface{}

	// Unmarshal unmarshals a configuration section into a struct.
	Unmarshal(key string, target interface{}) error
}

// BasePlugin provides a default implementation of the Plugin interface.
// Plugins can embed BasePlugin to avoid implementing all methods.
type BasePlugin struct {
	name     string
	version  string
	manifest *Manifest
}

// NewBasePlugin creates a BasePlugin with the given name and version.
func NewBasePlugin(name, version string) *BasePlugin {
	return &BasePlugin{
		name:    name,
		version: version,
	}
}

// NewBasePluginFromManifest creates a BasePlugin populated from a loaded Manifest.
func NewBasePluginFromManifest(m *Manifest) *BasePlugin {
	return &BasePlugin{
		name:     m.Name,
		version:  m.Version,
		manifest: m,
	}
}

func (p *BasePlugin) Name() string               { return p.name }
func (p *BasePlugin) Version() string            { return p.version }
func (p *BasePlugin) Manifest() *Manifest        { return p.manifest }
func (p *BasePlugin) Init(ctx CoreContext) error { return nil }
func (p *BasePlugin) Start() error               { return nil }
func (p *BasePlugin) Stop() error                { return nil }

// Compile-time interface checks
var _ Plugin = (*BasePlugin)(nil)
var _ EventBus = (*events.EventBus)(nil)
