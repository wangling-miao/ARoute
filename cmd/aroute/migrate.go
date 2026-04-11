// Package main implements the migrate subcommand for ARoute CMS.
package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/wangling-miao/aroute/plugins/database"
)

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Manage database migrations",
	Long: `Apply and revert database migrations.

Migrations are SQL files stored in the migrations directory and are
executed in order by their filename (e.g., 2026041301_create_users.sql).

Each migration file should contain UP statements followed by '-- @down'
marker and DOWN statements for rollback.

Use 'migrate up' to apply pending migrations and 'migrate down' to
revert previously applied migrations.`,
}

var migrateUpCmd = &cobra.Command{
	Use:   "up",
	Short: "Apply pending migrations",
	Long:  `Apply all pending database migrations in order.`,
	RunE:  runMigrateUp,
}

var migrateDownCmd = &cobra.Command{
	Use:   "down",
	Short: "Revert migrations",
	Long: `Revert the last N applied migrations (default 1).

Use --steps to revert multiple migrations.`,
	RunE: runMigrateDown,
}

var migrateStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show migration status",
	Long:  `Display the status of all migrations (pending vs applied).`,
	RunE:  runMigrateStatus,
}

var migrateSteps int

func init() {
	rootCmd.AddCommand(migrateCmd)
	migrateCmd.AddCommand(migrateUpCmd)
	migrateCmd.AddCommand(migrateDownCmd)
	migrateCmd.AddCommand(migrateStatusCmd)

	migrateDownCmd.Flags().IntVarP(&migrateSteps, "steps", "n", 1, "number of migrations to revert")
}

func openDatabaseForMigration() (*sql.DB, database.Driver, error) {
	driver := viper.GetString("database.driver")
	if driver == "" {
		driver = "sqlite"
	}

	switch driver {
	case "sqlite", "sqlite3":
		dbPath := viper.GetString("database.sqlite.path")
		if dbPath == "" {
			dataDir := getDataDir()
			dbPath = filepath.Join(dataDir, "aroute.db")
		}

		if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
			return nil, database.DriverSQLite, fmt.Errorf("create data directory: %w", err)
		}

		busyTimeout := viper.GetInt("database.sqlite.busy_timeout")
		if busyTimeout == 0 {
			busyTimeout = 10000
		}

		dsn := fmt.Sprintf("%s?_pragma=busy_timeout(%d)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(ON)",
			dbPath, busyTimeout)

		db, err := sql.Open("sqlite", dsn)
		if err != nil {
			return nil, database.DriverSQLite, fmt.Errorf("open sqlite: %w", err)
		}

		db.SetMaxOpenConns(1)
		db.SetMaxIdleConns(1)

		return db, database.DriverSQLite, nil

	case "postgres", "postgresql", "pg":
		connStr := buildPostgresConnStr()
		db, err := sql.Open("pgx", connStr)
		if err != nil {
			return nil, database.DriverPostgreSQL, fmt.Errorf("open postgres: %w", err)
		}

		db.SetMaxOpenConns(viper.GetInt("database.pool.max_conns"))
		db.SetMaxIdleConns(viper.GetInt("database.pool.min_conns"))

		return db, database.DriverPostgreSQL, nil

	default:
		return nil, database.DriverSQLite, fmt.Errorf("unsupported database driver: %s", driver)
	}
}

func buildPostgresConnStr() string {
	connStr := viper.GetString("database.connection_string")
	if connStr != "" {
		return connStr
	}

	host := viper.GetString("database.postgres.host")
	if host == "" {
		host = "localhost"
	}
	port := viper.GetInt("database.postgres.port")
	if port == 0 {
		port = 5432
	}
	user := viper.GetString("database.postgres.user")
	password := viper.GetString("database.postgres.password")
	dbname := viper.GetString("database.postgres.dbname")
	sslmode := viper.GetString("database.postgres.sslmode")
	if sslmode == "" {
		sslmode = "disable"
	}

	return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		host, port, user, password, dbname, sslmode)
}

func ensureMigrationsTable(ctx context.Context, db *sql.DB, driver database.Driver) error {
	var createTableSQL string
	if driver == database.DriverSQLite {
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

	_, err := db.ExecContext(ctx, createTableSQL)
	return err
}

func runMigrateUp(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	dataDir := getDataDir()
	migrationsDir := filepath.Join(dataDir, "migrations")

	if _, err := os.Stat(migrationsDir); os.IsNotExist(err) {
		fmt.Println("No migrations directory found at:", migrationsDir)
		fmt.Println("Create migration files with naming format: YYYYMMDDNN_description.sql")
		return nil
	}

	db, driver, err := openDatabaseForMigration()
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping database: %w", err)
	}

	if err := ensureMigrationsTable(ctx, db, driver); err != nil {
		return fmt.Errorf("create migrations table: %w", err)
	}

	service := database.NewService(db, driver)
	runner := database.NewMigrationRunner(service, migrationsDir)

	if err := runner.Load(ctx); err != nil {
		return fmt.Errorf("load migrations: %w", err)
	}

	total := runner.TotalCount()
	pending := runner.PendingCount()

	if pending == 0 {
		fmt.Printf("All migrations applied (%d total).\n", total)
		return nil
	}

	fmt.Printf("Found %d pending migrations (%d total).\n", pending, total)

	appliedCount, err := runner.Apply(ctx)
	if err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}

	fmt.Printf("Successfully applied %d migration(s).\n", appliedCount)

	logger.Info("Migration apply completed",
		"applied", appliedCount,
		"total", total,
		"pending_before", pending,
		"pending_after", runner.PendingCount(),
	)

	return nil
}

func runMigrateDown(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	dataDir := getDataDir()
	migrationsDir := filepath.Join(dataDir, "migrations")

	db, driver, err := openDatabaseForMigration()
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping database: %w", err)
	}

	if err := ensureMigrationsTable(ctx, db, driver); err != nil {
		return fmt.Errorf("create migrations table: %w", err)
	}

	service := database.NewService(db, driver)
	runner := database.NewMigrationRunner(service, migrationsDir)

	if err := runner.Load(ctx); err != nil {
		return fmt.Errorf("load migrations: %w", err)
	}

	applied := runner.AppliedCount()
	if applied == 0 {
		fmt.Println("No applied migrations to revert.")
		return nil
	}

	if migrateSteps > applied {
		fmt.Printf("Warning: Only %d migrations are applied. Reverting all.\n", applied)
		migrateSteps = applied
	}

	fmt.Printf("Reverting %d migration(s).\n", migrateSteps)

	revertedCount, err := runner.Revert(ctx, migrateSteps)
	if err != nil {
		return fmt.Errorf("revert migrations: %w", err)
	}

	fmt.Printf("Successfully reverted %d migration(s).\n", revertedCount)

	logger.Info("Migration revert completed",
		"reverted", revertedCount,
		"applied_before", applied,
		"applied_after", runner.AppliedCount(),
	)

	return nil
}

func runMigrateStatus(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	dataDir := getDataDir()
	migrationsDir := filepath.Join(dataDir, "migrations")

	if _, err := os.Stat(migrationsDir); os.IsNotExist(err) {
		fmt.Println("No migrations directory found at:", migrationsDir)
		return nil
	}

	db, driver, err := openDatabaseForMigration()
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping database: %w", err)
	}

	if err := ensureMigrationsTable(ctx, db, driver); err != nil {
		return fmt.Errorf("create migrations table: %w", err)
	}

	service := database.NewService(db, driver)
	runner := database.NewMigrationRunner(service, migrationsDir)

	if err := runner.Load(ctx); err != nil {
		return fmt.Errorf("load migrations: %w", err)
	}

	status, err := runner.Status(ctx)
	if err != nil {
		return fmt.Errorf("get status: %w", err)
	}

	if len(status) == 0 {
		fmt.Println("No migrations found.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "VERSION\tNAME\tSTATUS\tAPPLIED AT")
	fmt.Fprintln(w, "-------\t----\t------\t----------")

	pendingCount := 0
	appliedCount := 0

	for _, s := range status {
		appliedAt := "-"
		if s.Status == "applied" {
			appliedAt = s.AppliedAt.Format(time.RFC3339)
			appliedCount++
		} else {
			pendingCount++
		}
		fmt.Fprintf(w, "%d\t%s\t%s\t%s\n", s.Version, s.Name, s.Status, appliedAt)
	}
	w.Flush()

	fmt.Printf("\nSummary: %d applied, %d pending, %d total\n",
		appliedCount, pendingCount, len(status))

	return nil
}
