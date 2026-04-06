package core

import (
	"context"
	"log/slog"
)

// CoreContext provides plugins with access to Core services and resources.
// It's passed to plugins during initialization and provides the bridge
// between plugins and the Core microkernel.
//
// CoreContext is safe for concurrent use across goroutines.
type CoreContext struct {
	// Services is the service container for dependency injection.
	// Plugins use this to obtain and provide services to other plugins.
	Services ServiceContainer

	// Events is the event bus for plugin communication.
	// Plugins use this to subscribe to events and emit events.
	Events EventBus

	// Config provides configuration values for the plugin.
	// Implementation depends on the config system (viper, etc.).
	Config ConfigProvider

	// Logger is the structured logger instance.
	// All plugins should use this logger for consistent log formatting.
	Logger *slog.Logger

	// DataDir is the plugin-specific data directory.
	// Plugins can use this for persistent storage.
	// Path: ~/.aroute/data/plugins/{plugin-name}/
	DataDir string

	// PluginDir is the plugin installation directory.
	// Path: ~/.aroute/plugins/{plugin-name}/
	PluginDir string

	// Context provides request-scoped values and cancellation.
	// Use this for coordinating plugin shutdown.
	ctx context.Context
}

// NewCoreContext creates a new CoreContext with the provided values.
func NewCoreContext(ctx context.Context, services ServiceContainer, events EventBus, config ConfigProvider, logger *slog.Logger, dataDir, pluginDir string) *CoreContext {
	return &CoreContext{
		Services:  services,
		Events:    events,
		Config:    config,
		Logger:    logger,
		DataDir:   dataDir,
		PluginDir: pluginDir,
		ctx:       ctx,
	}
}

// Context returns the context associated with this CoreContext.
func (c *CoreContext) Context() context.Context {
	return c.ctx
}

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
type EventBus interface {
	// SubscribeFilter registers a handler for filter-style events.
	// Filter handlers are called in priority order and can abort the chain.
	// Returns a handler ID for unsubscribe.
	SubscribeFilter(event string, priority int, handler FilterHandler) string

	// SubscribeBroadcast registers a handler for broadcast-style events.
	// Broadcast handlers are called concurrently, errors are logged but don't abort.
	// Returns a handler ID for unsubscribe.
	SubscribeBroadcast(event string, handler BroadcastHandler) string

	// Emit sends a broadcast event to all matching handlers.
	// Handlers are called concurrently. Errors are logged but don't affect other handlers.
	Emit(ctx context.Context, event string, data interface{})

	// DispatchFilter sends a filter event through the handler chain.
	// Handlers are called in priority order. A handler can abort by returning error.
	// The modified event data is passed through the chain.
	DispatchFilter(ctx context.Context, event string, data interface{}) (interface{}, error)

	// Unsubscribe removes a handler by ID.
	Unsubscribe(handlerID string) error
}

// FilterHandler handles filter-style events.
// Handlers can modify the event data and must call next() to continue the chain.
// Returning an error aborts the chain.
type FilterHandler func(ctx context.Context, event string, data interface{}, next func() (interface{}, error)) (interface{}, error)

// BroadcastHandler handles broadcast-style events.
// Errors are logged but don't abort other handlers.
type BroadcastHandler func(ctx context.Context, event string, data interface{}) error

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

func (p *BasePlugin) Name() string                    { return p.name }
func (p *BasePlugin) Version() string                 { return p.version }
func (p *BasePlugin) Manifest() *Manifest             { return p.manifest }
func (p *BasePlugin) Init(ctx *CoreContext) error     { return nil }
func (p *BasePlugin) Start(ctx context.Context) error { return nil }
func (p *BasePlugin) Stop(ctx context.Context) error  { return nil }
func (p *BasePlugin) OnLoad() error                   { return nil }
func (p *BasePlugin) OnUnload() error                 { return nil }

// Compile-time interface checks
var _ Plugin = (*BasePlugin)(nil)
