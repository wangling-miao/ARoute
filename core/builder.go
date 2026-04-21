// Package core provides the microkernel infrastructure for Aroute CMS.
// This file implements a builder for creating fully configured Aroute engines.
package core

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
)

// Builder provides a fluent interface for constructing Aroute engines.
// It handles the initialization order and dependency injection automatically.
type Builder struct {
	config *Config

	// Optional overrides for testing
	containerOverride  ServiceContainer
	eventBusOverride   EventBus
	registryOverride   PluginRegistry
	lifecycleOverride  LifecycleManager
	dispatcherOverride EngineDispatcher
	licenseOverride    LicenseValidator
}

// NewBuilder creates a new Aroute engine builder.
func NewBuilder() *Builder {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDir = "."
	}
	return &Builder{
		config: &Config{
			DataDir:   filepath.Join(homeDir, ".aroute", "data"),
			PluginDir: filepath.Join(homeDir, ".aroute", "plugins"),
		},
	}
}

// WithDataDir sets the data directory.
func (b *Builder) WithDataDir(dir string) *Builder {
	b.config.DataDir = dir
	return b
}

// WithPluginDir sets the plugin directory.
func (b *Builder) WithPluginDir(dir string) *Builder {
	b.config.PluginDir = dir
	return b
}

// WithLicense sets the license file path and public key.
func (b *Builder) WithLicense(path string, pubKey *ecdsa.PublicKey) *Builder {
	b.config.LicensePath = path
	b.config.PublicKey = pubKey
	return b
}

// WithLogger sets the logger.
func (b *Builder) WithLogger(logger *slog.Logger) *Builder {
	b.config.Logger = logger
	return b
}

// WithContainer overrides the service container (for testing).
func (b *Builder) WithContainer(container ServiceContainer) *Builder {
	b.containerOverride = container
	return b
}

// WithEventBus overrides the event bus (for testing).
func (b *Builder) WithEventBus(eventBus EventBus) *Builder {
	b.eventBusOverride = eventBus
	return b
}

// WithRegistry overrides the registry (for testing).
func (b *Builder) WithRegistry(registry PluginRegistry) *Builder {
	b.registryOverride = registry
	return b
}

// WithLifecycle overrides the lifecycle manager (for testing).
func (b *Builder) WithLifecycle(lifecycle LifecycleManager) *Builder {
	b.lifecycleOverride = lifecycle
	return b
}

// WithDispatcher overrides the engine dispatcher (for testing).
func (b *Builder) WithDispatcher(dispatcher EngineDispatcher) *Builder {
	b.dispatcherOverride = dispatcher
	return b
}

// WithLicenseValidator overrides the license validator (for testing).
func (b *Builder) WithLicenseValidator(validator LicenseValidator) *Builder {
	b.licenseOverride = validator
	return b
}

// Build creates a new Aroute engine with all subsystems initialized.
// This method requires concrete implementations to be provided via the
// InitializeFunc type - the actual wiring is done by importing packages
// that provide InitializeFunc implementations.
//
// For production use, use BuildWithFactories which accepts factory functions.
func (b *Builder) Build(ctx context.Context) (*Aroute, error) {
	return New(ctx,
		b.containerOverride,
		b.eventBusOverride,
		b.registryOverride,
		b.lifecycleOverride,
		b.dispatcherOverride,
		b.licenseOverride,
		WithDataDir(b.config.DataDir),
		WithPluginDir(b.config.PluginDir),
		WithLicensePath(b.config.LicensePath),
		WithPublicKey(b.config.PublicKey),
		WithLogger(b.config.Logger),
	)
}

// BuildWithFactories creates an Aroute engine using factory functions.
// This is the recommended way to create an engine in production.
func (b *Builder) BuildWithFactories(ctx context.Context, factories *Factories) (*Aroute, error) {
	logger := b.config.Logger
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		}))
	}

	// Ensure directories exist
	if err := os.MkdirAll(b.config.DataDir, 0755); err != nil {
		return nil, fmt.Errorf("create data directory: %w", err)
	}
	if err := os.MkdirAll(b.config.PluginDir, 0755); err != nil {
		return nil, fmt.Errorf("create plugin directory: %w", err)
	}

	// Initialize subsystems using factories
	var container ServiceContainer
	var eventBus EventBus
	var registry PluginRegistry
	var lifecycle LifecycleManager
	var dispatcher EngineDispatcher
	var license LicenseValidator
	var err error

	// Use overrides if provided, otherwise use factories
	if b.containerOverride != nil {
		container = b.containerOverride
	} else if factories.NewContainer != nil {
		container, err = factories.NewContainer()
		if err != nil {
			return nil, fmt.Errorf("create container: %w", err)
		}
	}

	if b.eventBusOverride != nil {
		eventBus = b.eventBusOverride
	} else if factories.NewEventBus != nil {
		eventBus, err = factories.NewEventBus()
		if err != nil {
			return nil, fmt.Errorf("create event bus: %w", err)
		}
	}

	if b.registryOverride != nil {
		registry = b.registryOverride
	} else if factories.NewRegistry != nil {
		registry, err = factories.NewRegistry(b.config.DataDir)
		if err != nil {
			return nil, fmt.Errorf("create registry: %w", err)
		}
	}

	if b.dispatcherOverride != nil {
		dispatcher = b.dispatcherOverride
	} else if factories.NewDispatcher != nil {
		dispatcher, err = factories.NewDispatcher()
		if err != nil {
			return nil, fmt.Errorf("create dispatcher: %w", err)
		}
	}

	if b.licenseOverride != nil {
		license = b.licenseOverride
	} else if factories.NewLicense != nil {
		license, err = factories.NewLicense(b.config.LicensePath, b.config.PublicKey)
		if err != nil {
			return nil, fmt.Errorf("create license validator: %w", err)
		}
	}

	if b.lifecycleOverride != nil {
		lifecycle = b.lifecycleOverride
	} else if factories.NewLifecycle != nil {
		lifecycle, err = factories.NewLifecycle(registry, container, eventBus, logger, b.config.DataDir, b.config.PluginDir)
		if err != nil {
			return nil, fmt.Errorf("create lifecycle manager: %w", err)
		}
	}

	return New(ctx, container, eventBus, registry, lifecycle, dispatcher, license,
		WithDataDir(b.config.DataDir),
		WithPluginDir(b.config.PluginDir),
		WithLogger(logger),
	)
}

// Factories provides factory functions for creating subsystems.
// Import packages that implement these factories.
type Factories struct {
	NewContainer  func() (ServiceContainer, error)
	NewEventBus   func() (EventBus, error)
	NewRegistry   func(dataDir string) (PluginRegistry, error)
	NewDispatcher func() (EngineDispatcher, error)
	NewLicense    func(licensePath string, pubKey *ecdsa.PublicKey) (LicenseValidator, error)
	NewLifecycle  func(registry PluginRegistry, container ServiceContainer, eventBus EventBus, logger *slog.Logger, dataDir, pluginDir string) (LifecycleManager, error)
}

// CoreContextFactory creates a CoreContext for a specific plugin.
type CoreContextFactory func(ctx context.Context, pluginName string, logger *slog.Logger, container ServiceContainer, eventBus EventBus, dataDir, pluginDir string) CoreContext

// DefaultCoreContextFactory creates a CoreContext with default configuration.
func DefaultCoreContextFactory(ctx context.Context, pluginName string, logger *slog.Logger, container ServiceContainer, eventBus EventBus, dataDir, pluginDir string) CoreContext {
	pluginLogger := logger.With("plugin", pluginName)
	pluginDataDir := filepath.Join(dataDir, "plugin_data", pluginName)
	pluginConfig := NewScopedConfig(pluginName, make(map[string]interface{}))
	return NewCoreContext(ctx, container, eventBus, pluginConfig, pluginLogger, pluginDataDir, pluginDir)
}
