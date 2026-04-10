// Package main implements the migrate subcommand for ARoute CMS.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Manage database migrations",
	Long: `Apply and revert database migrations.

Migrations are SQL files stored in the migrations directory and are
executed in order by their filename (e.g., 001_create_users.sql).

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
	Long:  `Display the status of all migrations.`,
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

func runMigrateUp(cmd *cobra.Command, args []string) error {
	dataDir := getDataDir()
	migrationsDir := filepath.Join(dataDir, "migrations")

	// Check if migrations directory exists
	if _, err := os.Stat(migrationsDir); os.IsNotExist(err) {
		fmt.Println("No migrations directory found.")
		fmt.Println("Migrations should be placed in:", migrationsDir)
		return nil
	}

	// Find migration files
	files, err := findMigrationFiles(migrationsDir)
	if err != nil {
		return fmt.Errorf("find migrations: %w", err)
	}

	if len(files) == 0 {
		fmt.Println("No pending migrations.")
		return nil
	}

	// TODO: Actually apply migrations using the database plugin
	// For now, just list what would be applied
	fmt.Println("Pending migrations:")
	for _, f := range files {
		fmt.Printf("  - %s\n", f)
	}
	fmt.Println("\nNote: Migration execution requires the database plugin to be running.")
	fmt.Println("Use 'aroute serve' to start the server and apply migrations automatically.")

	return nil
}

func runMigrateDown(cmd *cobra.Command, args []string) error {
	// TODO: Actually revert migrations using the database plugin
	// For now, just show what would happen
	fmt.Printf("Would revert %d migration(s).\n", migrateSteps)
	fmt.Println("\nNote: Migration execution requires the database plugin to be running.")
	fmt.Println("Use 'aroute serve' to start the server which handles migrations.")

	return nil
}

func runMigrateStatus(cmd *cobra.Command, args []string) error {
	dataDir := getDataDir()
	migrationsDir := filepath.Join(dataDir, "migrations")

	// Check if migrations directory exists
	if _, err := os.Stat(migrationsDir); os.IsNotExist(err) {
		fmt.Println("No migrations directory found.")
		return nil
	}

	// Find migration files
	files, err := findMigrationFiles(migrationsDir)
	if err != nil {
		return fmt.Errorf("find migrations: %w", err)
	}

	if len(files) == 0 {
		fmt.Println("No migrations found.")
		return nil
	}

	// Print status table
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "MIGRATION\tSTATUS")
	fmt.Fprintln(w, "---------\t------")

	for _, f := range files {
		// TODO: Check actual status from _migrations table
		status := "pending"
		fmt.Fprintf(w, "%s\t%s\n", f, status)
	}
	w.Flush()

	return nil
}

// findMigrationFiles finds all migration SQL files in a directory
func findMigrationFiles(dir string) ([]string, error) {
	var files []string

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if !entry.IsDir() && (filepath.Ext(entry.Name()) == ".sql" || filepath.Ext(entry.Name()) == ".SQL") {
			files = append(files, entry.Name())
		}
	}

	// Sort by filename (which should be numbered)
	// Note: In production, this would use the database's _migrations table
	return files, nil
}
