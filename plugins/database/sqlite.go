package database

import (
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/wangling-miao/aroute/core"
	_ "modernc.org/sqlite"
)

func (p *Plugin) initSQLite(ctx core.CoreContext, logger *slog.Logger) error {
	config := ctx.Config()

	dbPath := config.GetString("database.path")
	if dbPath == "" {
		dbPath = config.GetString("database.sqlite.path")
	}
	if dbPath == "" {
		dataDir := ctx.DataDir()
		dbPath = filepath.Join(dataDir, "aroute.db")
	}

	dbDir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		logger.Error("Failed to create database directory", "error", err, "path", dbDir)
		return fmt.Errorf("failed to create database directory: %w", err)
	}

	logger.Debug("SQLite database path", "path", dbPath)

	dsn := buildSQLiteDSN(dbPath, config)

	logger.Debug("SQLite DSN (masked)", "dsn", dsn)

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		logger.Error("Failed to open SQLite database", "error", err)
		return fmt.Errorf("failed to open SQLite database: %w", err)
	}

	if err := db.Ping(); err != nil {
		db.Close()
		logger.Error("Failed to ping SQLite database", "error", err)
		return fmt.Errorf("failed to ping SQLite database: %w", err)
	}

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)

	p.service = NewService(db, DriverSQLite)

	logger.Info("SQLite database initialized successfully",
		"path", dbPath,
		"wal_mode", "enabled",
	)

	return nil
}

func buildSQLiteDSN(path string, config core.ConfigProvider) string {
	busyTimeout := config.GetInt("database.sqlite.busy_timeout")
	if busyTimeout == 0 {
		busyTimeout = 10000
	}

	cacheSize := config.GetInt("database.sqlite.cache_size")
	if cacheSize == 0 {
		cacheSize = -32000
	}

	synchronous := config.GetString("database.sqlite.synchronous")
	if synchronous == "" {
		synchronous = "NORMAL"
	}

	dsn := fmt.Sprintf("%s?_pragma=busy_timeout(%d)&_pragma=journal_mode(WAL)&_pragma=synchronous(%s)&_pragma=foreign_keys(ON)&_pragma=cache_size(%d)",
		path, busyTimeout, synchronous, cacheSize)

	if config.GetBool("database.sqlite.temp_store_memory") {
		dsn += "&_pragma=temp_store(MEMORY)"
	}

	return dsn
}
