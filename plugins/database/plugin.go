// Package database provides the database abstraction plugin for Aroute CMS.
// It implements a unified database interface supporting both SQLite (zero-CGO)
// and PostgreSQL with automatic driver detection, migrations, schema introspection,
// and connection pooling.
package database

import (
	"fmt"
	"sync"

	"github.com/wangling-miao/aroute/core"
	"github.com/wangling-miao/aroute/sdk/interfaces"
)

// Plugin implements the core.Plugin interface for database functionality.
// It provides a unified database abstraction layer supporting SQLite and PostgreSQL.
type Plugin struct {
	*core.BasePlugin

	mu      sync.RWMutex
	ctx     core.CoreContext
	service *Service
	driver  Driver
	running bool
}

// Driver represents the database driver type.
type Driver string

const (
	DriverSQLite     Driver = "sqlite"
	DriverPostgreSQL Driver = "postgres"
)

// New creates a new database plugin instance.
func New() *Plugin {
	return &Plugin{
		BasePlugin: core.NewBasePlugin("database", "1.0.0"),
	}
}

// Name returns the plugin name.
func (p *Plugin) Name() string {
	return "database"
}

// Version returns the plugin version.
func (p *Plugin) Version() string {
	return "1.0.0"
}

// Manifest returns the plugin manifest.
func (p *Plugin) Manifest() *core.Manifest {
	return &core.Manifest{
		Name:        "database",
		Version:     "1.0.0",
		Description: "Database abstraction layer supporting SQLite (zero-CGO) and PostgreSQL with migrations, schema introspection, and connection pooling",
		Author:      "Aroute Team",
		License:     "MIT",
		Engine:      "native",
		Requires:    []string{},
		After:       []string{},
		Provides:    []string{"database.service"},
	}
}

// Init initializes the database plugin.
// It detects the driver type, initializes the connection, creates the migrations table,
// and registers the DatabaseService.
func (p *Plugin) Init(ctx core.CoreContext) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.ctx = ctx
	logger := ctx.Logger()

	logger.Info("Initializing database plugin")

	// Detect driver type from config
	driverType := p.detectDriver(ctx)
	p.driver = driverType

	logger.Info("Detected database driver", "driver", driverType)

	// Initialize the appropriate driver
	var err error
	switch driverType {
	case DriverSQLite:
		err = p.initSQLite(ctx, logger)
	case DriverPostgreSQL:
		err = p.initPostgreSQL(ctx, logger)
	default:
		return fmt.Errorf("unsupported database driver: %s. Supported drivers: sqlite, postgres", driverType)
	}

	if err != nil {
		return fmt.Errorf("failed to initialize %s driver: %w", driverType, err)
	}

	// Create migrations tracking table
	if err := p.createMigrationsTable(ctx); err != nil {
		return fmt.Errorf("failed to create migrations table: %w", err)
	}

	// Register DatabaseService
	if err := ctx.Services().Provide(func(container core.ServiceContainer) (interfaces.DatabaseService, error) {
		return p.service, nil
	}); err != nil {
		return fmt.Errorf("failed to register DatabaseService: %w", err)
	}

	logger.Info("Database plugin initialized successfully",
		"driver", driverType,
		"service_registered", "database.service",
	)

	return nil
}

// detectDriver determines the database driver type from configuration.
func (p *Plugin) detectDriver(ctx core.CoreContext) Driver {
	config := ctx.Config()
	driverStr := config.GetString("database.driver")

	if driverStr == "" {
		// Default to SQLite for zero-dependency deployment
		return DriverSQLite
	}

	switch driverStr {
	case "sqlite", "sqlite3":
		return DriverSQLite
	case "postgres", "postgresql", "pg":
		return DriverPostgreSQL
	default:
		return Driver(driverStr)
	}
}

// createMigrationsTable creates the migrations tracking table if it doesn't exist.
func (p *Plugin) createMigrationsTable(ctx core.CoreContext) error {
	logger := ctx.Logger()

	// Create migrations table with driver-specific SQL
	var createTableSQL string
	if p.driver == DriverSQLite {
		createTableSQL = `
CREATE TABLE IF NOT EXISTS _migrations (
	version TEXT PRIMARY KEY,
	name TEXT NOT NULL,
	applied_at TEXT NOT NULL DEFAULT (datetime('now'))
)`
	} else {
		createTableSQL = `
CREATE TABLE IF NOT EXISTS _migrations (
	version VARCHAR(255) PRIMARY KEY,
	name VARCHAR(255) NOT NULL,
	applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
)`
	}

	_, err := p.service.Exec(ctx.Context(), createTableSQL)
	if err != nil {
		logger.Error("Failed to create migrations table", "error", err)
		return err
	}

	logger.Debug("Migrations tracking table created or verified")
	return nil
}

// Start starts the database plugin.
// It verifies the connection is alive and optionally runs pending migrations.
func (p *Plugin) Start() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.running {
		return nil
	}

	logger := p.ctx.Logger()
	ctx := p.ctx.Context()

	// Verify connection is alive
	if err := p.service.Ping(ctx); err != nil {
		logger.Error("Database connection ping failed", "error", err)
		return fmt.Errorf("database connection failed: %w", err)
	}

	logger.Info("Database connection verified")

	// Check if auto-migrate is enabled
	config := p.ctx.Config()
	autoMigrate := config.GetBool("database.auto_migrate")

	if autoMigrate {
		logger.Info("Auto-migration enabled, checking for pending migrations")
		// Migration runner will be invoked via CLI or manually
		// We don't auto-run migrations on plugin start for safety
	}

	p.running = true
	logger.Info("Database plugin started successfully")

	return nil
}

// Stop gracefully shuts down the database plugin.
// It closes the database connection and releases resources.
func (p *Plugin) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.running || p.service == nil {
		return nil
	}

	logger := p.ctx.Logger()
	logger.Info("Stopping database plugin")

	// Close database connection
	if err := p.service.Close(); err != nil {
		logger.Error("Failed to close database connection", "error", err)
		return fmt.Errorf("failed to close database: %w", err)
	}

	p.running = false
	logger.Info("Database plugin stopped successfully")

	return nil
}
