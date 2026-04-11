package database

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/wangling-miao/aroute/core"
)

func (p *Plugin) initPostgreSQL(ctx core.CoreContext, logger *slog.Logger) error {
	config := ctx.Config()

	connStr := config.GetString("database.connection_string")
	if connStr == "" {
		host := config.GetString("database.postgres.host")
		if host == "" {
			host = "localhost"
		}
		port := config.GetInt("database.postgres.port")
		if port == 0 {
			port = 5432
		}
		user := config.GetString("database.postgres.user")
		if user == "" {
			user = "aroute"
		}
		password := config.GetString("database.postgres.password")
		dbname := config.GetString("database.postgres.dbname")
		if dbname == "" {
			dbname = "aroute"
		}
		sslmode := config.GetString("database.postgres.sslmode")
		if sslmode == "" {
			sslmode = "disable"
		}

		connStr = fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
			user, password, host, port, dbname, sslmode)
	}

	maskedConnStr := MaskPassword(connStr)
	logger.Debug("PostgreSQL connection string", "dsn", maskedConnStr)

	poolConfig, err := pgxpool.ParseConfig(connStr)
	if err != nil {
		logger.Error("Failed to parse PostgreSQL connection string", "error", err)
		return fmt.Errorf("failed to parse PostgreSQL config: %w", err)
	}

	maxConns := config.GetInt("database.pool.max_conns")
	if maxConns > 0 {
		poolConfig.MaxConns = int32(maxConns)
	} else {
		poolConfig.MaxConns = 20
	}

	minConns := config.GetInt("database.pool.min_conns")
	if minConns > 0 {
		poolConfig.MinConns = int32(minConns)
	}

	maxConnLifetime := config.GetString("database.pool.max_conn_lifetime")
	if maxConnLifetime != "" {
		duration, err := time.ParseDuration(maxConnLifetime)
		if err == nil {
			poolConfig.MaxConnLifetime = duration
		}
	} else {
		poolConfig.MaxConnLifetime = 1 * time.Hour
	}

	poolConfig.MaxConnLifetimeJitter = 5 * time.Minute

	maxConnIdleTime := config.GetString("database.pool.max_conn_idle_time")
	if maxConnIdleTime != "" {
		duration, err := time.ParseDuration(maxConnIdleTime)
		if err == nil {
			poolConfig.MaxConnIdleTime = duration
		}
	} else {
		poolConfig.MaxConnIdleTime = 30 * time.Minute
	}

	poolConfig.HealthCheckPeriod = 1 * time.Minute

	poolConfig.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		statementTimeout := config.GetString("database.statement_timeout")
		if statementTimeout != "" {
			_, err := conn.Exec(ctx, fmt.Sprintf("SET statement_timeout = %s", statementTimeout))
			if err != nil {
				return err
			}
		}

		timezone := config.GetString("database.timezone")
		if timezone != "" {
			_, err := conn.Exec(ctx, fmt.Sprintf("SET timezone = '%s'", timezone))
			if err != nil {
				return err
			}
		}

		return nil
	}

	pool, err := pgxpool.NewWithConfig(ctx.Context(), poolConfig)
	if err != nil {
		logger.Error("Failed to create PostgreSQL pool", "error", err)
		return fmt.Errorf("failed to create PostgreSQL pool: %w", err)
	}

	if err := pool.Ping(ctx.Context()); err != nil {
		pool.Close()
		logger.Error("Failed to ping PostgreSQL database", "error", err)
		return fmt.Errorf("failed to ping PostgreSQL database: %w", err)
	}

	db := stdlib.OpenDBFromPool(pool)

	p.service = NewService(db, DriverPostgreSQL)

	stats := pool.Stat()
	logger.Info("PostgreSQL database initialized successfully",
		"max_conns", poolConfig.MaxConns,
		"min_conns", poolConfig.MinConns,
		"current_conns", stats.TotalConns(),
		"idle_conns", stats.IdleConns(),
	)

	return nil
}
