package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/wangling-miao/aroute/core"
)

// Manager orchestrates plugin lifecycle across the entire system.
// It manages state transitions, dependency resolution, ordered startup/shutdown,
// and supports hot-plug (runtime enable/disable).
//
// Thread safety: All methods are safe for concurrent use.
// Implementations must handle concurrent calls to LoadAll, Start, Stop, Enable, Disable.
type Manager interface {
	// LoadAll discovers and loads all plugins from the registry.
	// It calls the discovery system to find plugins, then validates and registers
	// them without starting.
	//
	// LoadAll does NOT start plugins - use Start after LoadAll.
	//
	// Thread safety: Serialized operation (mutual exclusion).
	LoadAll(ctx context.Context) error

	// Start initializes and activates all enabled plugins in dependency order.
	// It resolves dependencies, validates no cycles exist, builds a startup order
	// via topological sort, then calls Init → Startfor each plugin.
	//
	// If a plugin fails to start, it's marked as Failed and all plugins that depend
	// on it are skipped (not started).
	//
	// Thread safety: Serialized operation (mutual exclusion).
	Start(ctx context.Context) error

	// Stop gracefully shuts down all active plugins in reverse dependency order.
	// It builds a reverse topological order and calls Stop for each plugin.
	//
	// Stop attempts to stop all plugins even if some fail. Errors are collected
	// and returned as a combined error.
	//
	// Thread safety: Serialized operation (mutual exclusion).
	Stop(ctx context.Context) error

	// Enable performs hot-plug for a disabled plugin.
	// It verifies all dependencies are satisfied, then loads and starts the plugin.
	//
	// Enable fails if:
	// - Plugin is not in Disabled/Stopped state
	// - Dependencies are not yet active
	// - Plugin startup fails
	//
	// Thread safety: Serialized operation (mutual exclusion).
	Enable(ctx context.Context, pluginName string) error

	// Disable performs hot-unplug for an active plugin.
	// It verifies no other active plugins depend on it, then stops and unloads.
	//
	// Disable fails if:
	// - Plugin is not active
	// - Other active plugins depend on this plugin
	//
	// Thread safety: Serialized operation (mutual exclusion).
	Disable(ctx context.Context, pluginName string) error

	// Retry attempts to recover a plugin from Failed state.
	// It transitions the plugin through Resolved → Starting → Active.
	//
	// Retry fails if:
	// - Plugin is not in Failed state
	// - Plugin startup fails again
	//
	// Thread safety: Serialized operation (mutual exclusion).
	Retry(ctx context.Context, pluginName string) error

	// GetState returns the current lifecycle state of a plugin.
	// Returns an error if the plugin is not registered.
	//
	// Thread safety: Concurrent read operation.
	GetState(pluginName string) (core.PluginState, error)

	// GetPlugin retrieves the loaded Plugin instance by name.
	// Returns nil if not loaded or not found.
	//
	// Thread safety: Concurrent read operation.
	GetPlugin(pluginName string) core.Plugin

	// ListPlugins returns names of all registered plugins.
	//
	// Thread safety: Concurrent read operation.
	ListPlugins() []string
}

// StateError represents an invalid state transition.
type StateError struct {
	PluginName   string
	CurrentState core.PluginState
	TargetState  core.PluginState
	Message      string
}

func (e *StateError) Error() string {
	return fmt.Sprintf("lifecycle: plugin %s: cannot transition from %s to %s: %s",
		e.PluginName, e.CurrentState, e.TargetState, e.Message)
}

// DependencyError represents a dependency resolution failure.
type DependencyError struct {
	PluginName string
	Dependency string
	Message    string
	CyclePath  []string // For cycle detection
}

func (e *DependencyError) Error() string {
	if len(e.CyclePath) > 0 {
		return fmt.Sprintf("lifecycle: dependency cycle detected: %v", e.CyclePath)
	}
	return fmt.Sprintf("lifecycle: plugin %s: dependency %s %s", e.PluginName, e.Dependency, e.Message)
}

// Errors returned by the lifecycle manager.
var (
	ErrPluginNotFound         = errors.New("lifecycle: plugin not found")
	ErrPluginNotLoaded        = errors.New("lifecycle: plugin not loaded")
	ErrPluginAlreadyLoaded    = errors.New("lifecycle: plugin already loaded")
	ErrInvalidState           = errors.New("lifecycle: invalid state transition")
	ErrDependencyNotSatisfied = errors.New("lifecycle: dependency not satisfied")
	ErrDependencyCycle        = errors.New("lifecycle: dependency cycle detected")
	ErrActiveDependents       = errors.New("lifecycle: other plugins depend on this plugin")
)

// stateMachine defines valid state transitions.
// Keys are current states, values are sets of valid next states.
var stateMachine = map[core.PluginState]map[core.PluginState]bool{
	core.StateRegistered: {
		core.StateResolved: true,
		core.StateFailed:   true,
	},
	core.StateResolved: {
		core.StateStarting: true,
		core.StateFailed:   true,
		core.StateStopped:  true,
	},
	core.StateStarting: {
		core.StateActive: true,
		core.StateFailed: true,
	},
	core.StateActive: {
		core.StateStopping: true,
	},
	core.StateStopping: {
		core.StateStopped: true,
		core.StateFailed:  true,
	},
	core.StateStopped: {
		core.StateResolved: true,
		core.StateStarting: true,
		core.StateFailed:   true,
	},
	core.StateFailed: {
		core.StateStopping: true,
		core.StateStopped:  true,
	},
}

// canTransition checks if a state transition is valid.
func canTransition(from, to core.PluginState) bool {
	validTargets, ok := stateMachine[from]
	if !ok {
		return false
	}
	return validTargets[to]
}

// PluginLoadInfo tracks runtime information for a loaded plugin.
type PluginLoadInfo struct {
	Plugin    core.Plugin
	Manifest  *core.Manifest
	State     core.PluginState
	LoadError error
}

// CoreContextFactory creates a CoreContext for a specific plugin.
type CoreContextFactory func(ctx context.Context, pluginName string) core.CoreContext

// ManagerImpl implements Manager with thread-safe operations.
type ManagerImpl struct {
	mu           sync.RWMutex
	registry     PluginRegistry
	pluginLoader PluginLoader
	eventBus     core.EventBus
	container    core.ServiceContainer
	ctxFactory   CoreContextFactory
	resolver     CrossTypeResolver
	plugins      map[string]*PluginLoadInfo
	order        []string
}

// PluginRegistry defines the interface for accessing plugin metadata.
// This is a minimal interface needed by the lifecycle manager.
type PluginRegistry interface {
	List() ([]core.Manifest, error)
	Get(name string) (core.Manifest, error)
	IsEnabled(name string) (bool, error)
}

// CrossTypeResolver provides cross-type dependency ordering from a unified routing registry.
// If available, the lifecycle manager uses it instead of its built-in topological sort.
type CrossTypeResolver interface {
	PluginStartupOrder() ([]string, error)
}

// PluginLoader creates Plugin instances from manifests.
type PluginLoader interface {
	Load(manifest core.Manifest) (core.Plugin, error)
}

// NewManager creates a new lifecycle manager.
func NewManager(registry PluginRegistry, loader PluginLoader, eventBus core.EventBus, container core.ServiceContainer, ctxFactory CoreContextFactory) *ManagerImpl {
	return &ManagerImpl{
		registry:     registry,
		pluginLoader: loader,
		eventBus:     eventBus,
		container:    container,
		ctxFactory:   ctxFactory,
		plugins:      make(map[string]*PluginLoadInfo),
	}
}

// WithResolver sets the cross-type resolver for dependency ordering.
func WithResolver(r CrossTypeResolver) func(*ManagerImpl) {
	return func(m *ManagerImpl) { m.resolver = r }
}

// NewManagerWithResolver creates a lifecycle manager with an optional cross-type resolver.
func NewManagerWithResolver(registry PluginRegistry, loader PluginLoader, eventBus core.EventBus, container core.ServiceContainer, ctxFactory CoreContextFactory, resolver CrossTypeResolver) *ManagerImpl {
	m := NewManager(registry, loader, eventBus, container, ctxFactory)
	m.resolver = resolver
	return m
}
