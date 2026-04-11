// Package main implements the init subcommand for ARoute CMS.
package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/term"
	"gopkg.in/yaml.v3"

	_ "modernc.org/sqlite"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a new ARoute CMS instance",
	Long: `Perform interactive first-run setup for ARoute CMS.

This command will:
  1. Create a configuration file
  2. Set up the data directory
  3. Initialize the database
  4. Run database migrations
  5. Create the initial admin user

The wizard prompts for essential configuration values with sensible defaults.`,
	RunE: runInit,
}

var (
	initNoInteractive bool
	initAdminEmail    string
	initAdminPassword string
	initSiteName      string
	initSiteURL       string
	initConfigPath    string
	initDataDirPath   string
	initSkipMigrate   bool
)

func init() {
	rootCmd.AddCommand(initCmd)

	initCmd.Flags().BoolVar(&initNoInteractive, "no-interactive", false, "skip interactive prompts")
	initCmd.Flags().StringVar(&initAdminEmail, "admin-email", "", "admin user email")
	initCmd.Flags().StringVar(&initAdminPassword, "admin-password", "", "admin user password")
	initCmd.Flags().StringVar(&initSiteName, "site-name", "My ARoute CMS", "site name")
	initCmd.Flags().StringVar(&initSiteURL, "site-url", "http://localhost:1337", "site URL")
	initCmd.Flags().StringVar(&initConfigPath, "config-path", "", "config file path (default: ./aroute.yaml)")
	initCmd.Flags().StringVar(&initDataDirPath, "data-dir", "", "data directory path (default: ./data)")
	initCmd.Flags().BoolVar(&initSkipMigrate, "skip-migrate", false, "skip database migrations")
}

func runInit(cmd *cobra.Command, args []string) error {
	reader := bufio.NewReader(os.Stdin)

	// Prompt for configuration values
	configPath := "./aroute.yaml"
	if initConfigPath != "" {
		configPath = initConfigPath
	} else if !initNoInteractive {
		fmt.Printf("Config file path [%s]: ", configPath)
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)
		if input != "" {
			configPath = input
		}
	}

	// Check if config file already exists
	if _, err := os.Stat(configPath); err == nil {
		if !initNoInteractive {
			fmt.Printf("Config file already exists at %s. Overwrite? [y/N]: ", configPath)
			input, _ := reader.ReadString('\n')
			input = strings.TrimSpace(strings.ToLower(input))
			if input != "y" && input != "yes" {
				fmt.Println("Initialization aborted.")
				return nil
			}
		} else {
			return fmt.Errorf("config file already exists at %s", configPath)
		}
	}

	// Prompt for data directory
	dataDirPath := "./data"
	if initDataDirPath != "" {
		dataDirPath = initDataDirPath
	} else if !initNoInteractive {
		fmt.Printf("Data directory [%s]: ", dataDirPath)
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)
		if input != "" {
			dataDirPath = input
		}
	}

	// Prompt for admin email
	adminEmail := initAdminEmail
	if adminEmail == "" && !initNoInteractive {
		fmt.Print("Admin email: ")
		input, _ := reader.ReadString('\n')
		adminEmail = strings.TrimSpace(input)
		if adminEmail == "" {
			return fmt.Errorf("admin email is required")
		}
	}
	if adminEmail == "" {
		return fmt.Errorf("admin email is required (use --admin-email or run interactively)")
	}

	// Prompt for admin password with hidden input
	adminPassword := initAdminPassword
	if adminPassword == "" && !initNoInteractive {
		adminPassword, err := readPasswordHidden("Admin password (min 8 characters): ")
		if err != nil {
			return fmt.Errorf("reading password: %w", err)
		}

		// Validate password length
		for len(adminPassword) < 8 {
			fmt.Println("Password must be at least 8 characters")
			adminPassword, err = readPasswordHidden("Admin password (min 8 characters): ")
			if err != nil {
				return fmt.Errorf("reading password: %w", err)
			}
		}

		// Confirm password
		confirmPassword, err := readPasswordHidden("Confirm admin password: ")
		if err != nil {
			return fmt.Errorf("reading password confirmation: %w", err)
		}

		if adminPassword != confirmPassword {
			return fmt.Errorf("passwords do not match")
		}
	}
	if adminPassword == "" {
		return fmt.Errorf("admin password is required (use --admin-password or run interactively)")
	}
	if len(adminPassword) < 8 {
		return fmt.Errorf("password must be at least 8 characters")
	}

	// Prompt for site name
	siteName := initSiteName
	if !initNoInteractive && initSiteName == "" {
		fmt.Printf("Site name [%s]: ", siteName)
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)
		if input != "" {
			siteName = input
		}
	}

	// Prompt for site URL
	siteURL := initSiteURL
	if !initNoInteractive && initSiteURL == "" {
		fmt.Printf("Site URL [%s]: ", siteURL)
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)
		if input != "" {
			siteURL = input
		}
	}

	// Create directories
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	if err := os.MkdirAll(dataDirPath, 0755); err != nil {
		return fmt.Errorf("create data directory: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(dataDirPath, "plugins"), 0755); err != nil {
		return fmt.Errorf("create plugins directory: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(dataDirPath, "uploads"), 0755); err != nil {
		return fmt.Errorf("create uploads directory: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(dataDirPath, "migrations"), 0755); err != nil {
		return fmt.Errorf("create migrations directory: %w", err)
	}

	dbPath := filepath.Join(dataDirPath, "aroute.db")

	// Generate configuration file
	jwtSecret, err := generateSecureRandomSecret(32)
	if err != nil {
		return fmt.Errorf("generate JWT secret: %w", err)
	}

	config := map[string]interface{}{
		"site": map[string]interface{}{
			"name": siteName,
			"url":  siteURL,
		},
		"server": map[string]interface{}{
			"host": "0.0.0.0",
			"port": 1337,
		},
		"database": map[string]interface{}{
			"driver": "sqlite",
			"sqlite": map[string]interface{}{
				"path": dbPath,
			},
		},
		"auth": map[string]interface{}{
			"jwt_secret":        jwtSecret,
			"access_token_ttl":  "15m",
			"refresh_token_ttl": "7d",
		},
		"media": map[string]interface{}{
			"storage":       "local",
			"upload_dir":    filepath.Join(dataDirPath, "uploads"),
			"max_file_size": "50MB",
		},
		"search": map[string]interface{}{
			"index_dir": filepath.Join(dataDirPath, "search"),
		},
		"cache": map[string]interface{}{
			"max_size":    256,
			"default_ttl": "5m",
		},
		"theme": map[string]interface{}{
			"active": "default",
			"dir":    "themes",
		},
		"log": map[string]interface{}{
			"level":  "info",
			"format": "text",
		},
		"plugins": map[string]interface{}{
			"dir": "plugins",
		},
		"data_dir": dataDirPath,
		"admin": map[string]interface{}{
			"email": adminEmail,
		},
	}

	// Write config file
	data, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("write config file: %w", err)
	}

	fmt.Println("\n✓ Configuration file created:", configPath)
	fmt.Println("✓ Data directory created:", dataDirPath)

	// Initialize database
	fmt.Println("\nInitializing database...")
	db, err := initDatabase(dbPath)
	if err != nil {
		return fmt.Errorf("initialize database: %w", err)
	}
	defer db.Close()

	fmt.Println("✓ Database initialized:", dbPath)

	// Create migrations table and run migrations
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if !initSkipMigrate {
		fmt.Println("\nRunning database migrations...")
		if err := runMigrations(ctx, db, dataDirPath); err != nil {
			return fmt.Errorf("run migrations: %w", err)
		}
	}

	// Create admin user
	fmt.Println("\nCreating admin user...")
	if err := createAdminUser(ctx, db, adminEmail, adminPassword); err != nil {
		return fmt.Errorf("create admin user: %w", err)
	}
	fmt.Println("✓ Admin user created:", adminEmail)

	fmt.Println("\nARoute CMS initialized successfully!")
	fmt.Println("\nNext steps:")
	fmt.Println("  1. Review the configuration file:", configPath)
	fmt.Println("  2. Add content types to define your schema")
	fmt.Println("  3. Start the server: aroute serve")
	fmt.Println("  4. Access the admin panel at:", siteURL+"/admin")
	fmt.Println("\nAdmin credentials:")
	fmt.Println("  Email:    ", adminEmail)
	fmt.Println("  Password: [the password you provided]")

	return nil
}

// readPasswordHidden reads password from terminal with hidden input
func readPasswordHidden(prompt string) (string, error) {
	fmt.Print(prompt)

	// Check if stdin is a terminal
	if term.IsTerminal(int(os.Stdin.Fd())) {
		password, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println() // Print newline after password input
		if err != nil {
			return "", err
		}
		return string(password), nil
	}

	// If not a terminal (e.g., piped input), use bufio reader
	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(input), nil
}

// generateSecureRandomSecret generates a cryptographically secure random string
func generateSecureRandomSecret(length int) (string, error) {
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)

	_, err := rand.Read(b)
	if err != nil {
		return "", fmt.Errorf("generate random bytes: %w", err)
	}

	for i := range b {
		b[i] = chars[b[i]%byte(len(chars))]
	}

	return string(b), nil
}

// initDatabase initializes SQLite database connection
func initDatabase(dbPath string) (*sql.DB, error) {
	dsn := fmt.Sprintf("%s?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(ON)", dbPath)

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Test connection
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	// SQLite connection pool settings
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)

	return db, nil
}

// runMigrations executes database migrations
func runMigrations(ctx context.Context, db *sql.DB, dataDirPath string) error {
	// Create _migrations table to track applied migrations
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS _migrations (
			version TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			applied_at TEXT NOT NULL
		)
	`)
	if err != nil {
		return fmt.Errorf("create migrations table: %w", err)
	}

	migrationsDir := filepath.Join(dataDirPath, "migrations")

	// Check if migrations directory exists and has files
	if _, err := os.Stat(migrationsDir); os.IsNotExist(err) {
		fmt.Println("✓ No migrations directory found, skipping migrations")
		return nil
	}

	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		return fmt.Errorf("read migrations directory: %w", err)
	}

	// Filter SQL migration files
	var migrationFiles []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") {
			migrationFiles = append(migrationFiles, entry.Name())
		}
	}

	if len(migrationFiles) == 0 {
		fmt.Println("✓ No migration files found, skipping migrations")
		return nil
	}

	// Sort migration files by name (timestamp-based naming)
	for _, filename := range migrationFiles {
		filePath := filepath.Join(migrationsDir, filename)

		// Check if migration already applied
		var appliedAt string
		err := db.QueryRowContext(ctx,
			"SELECT applied_at FROM _migrations WHERE version = ?",
			extractVersion(filename),
		).Scan(&appliedAt)

		if err == nil {
			// Migration already applied
			fmt.Printf("  - Migration %s already applied, skipping\n", filename)
			continue
		}

		if err != sql.ErrNoRows {
			return fmt.Errorf("check migration status: %w", err)
		}

		// Read and execute migration
		content, err := os.ReadFile(filePath)
		if err != nil {
			return fmt.Errorf("read migration file %s: %w", filename, err)
		}

		// Execute migration
		if err := executeMigration(ctx, db, string(content), filename); err != nil {
			return fmt.Errorf("execute migration %s: %w", filename, err)
		}

		// Record migration
		version := extractVersion(filename)
		name := extractName(filename)
		appliedAtTime := time.Now().Format(time.RFC3339)

		_, err = db.ExecContext(ctx,
			"INSERT INTO _migrations (version, name, applied_at) VALUES (?, ?, ?)",
			version, name, appliedAtTime,
		)
		if err != nil {
			return fmt.Errorf("record migration %s: %w", filename, err)
		}

		fmt.Printf("  - Applied migration: %s\n", filename)
	}

	return nil
}

// executeMigration executes migration SQL content
func executeMigration(ctx context.Context, db *sql.DB, content string, filename string) error {
	// Extract UP section if migration has -- @down marker
	upContent := content
	downMarker := "-- @down"
	idx := strings.Index(content, downMarker)
	if idx > 0 {
		upContent = strings.TrimSpace(content[:idx])
	}

	// Split and execute statements
	statements := splitSQLStatements(upContent)

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	for i, stmt := range statements {
		if strings.TrimSpace(stmt) == "" {
			continue
		}

		_, err = tx.ExecContext(ctx, stmt)
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("statement %d in %s: %w", i+1, filename, err)
		}
	}

	return tx.Commit()
}

// splitSQLStatements splits SQL content into individual statements
func splitSQLStatements(content string) []string {
	statements := strings.Split(content, ";")
	result := []string{}
	for _, stmt := range statements {
		stmt = strings.TrimSpace(stmt)
		if stmt != "" {
			result = append(result, stmt)
		}
	}
	return result
}

// extractVersion extracts version from migration filename (YYYYMMDDNN_description.sql)
func extractVersion(filename string) string {
	if len(filename) >= 10 {
		return filename[:10]
	}
	return filename
}

// extractName extracts name from migration filename
func extractName(filename string) string {
	if len(filename) > 14 && strings.HasSuffix(filename, ".sql") {
		return filename[11 : len(filename)-4]
	}
	return filename
}

// createAdminUser creates the initial admin user in the database
func createAdminUser(ctx context.Context, db *sql.DB, email, password string) error {
	// Create users table if it doesn't exist
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			email TEXT UNIQUE NOT NULL,
			password_hash TEXT NOT NULL,
			username TEXT,
			display_name TEXT,
			role TEXT NOT NULL DEFAULT 'user',
			is_active BOOLEAN NOT NULL DEFAULT true,
			is_verified BOOLEAN NOT NULL DEFAULT false,
			last_login_at TEXT,
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return fmt.Errorf("create users table: %w", err)
	}

	// Create index on email
	_, err = db.ExecContext(ctx, `
		CREATE INDEX IF NOT EXISTS idx_users_email ON users(email)
	`)
	if err != nil {
		return fmt.Errorf("create users email index: %w", err)
	}

	// Hash password with bcrypt (cost 12 for strong security)
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	// Check if admin user already exists
	var existingID int
	err = db.QueryRowContext(ctx, "SELECT id FROM users WHERE email = ?", email).Scan(&existingID)
	if err == nil {
		// User exists, update password
		_, err = db.ExecContext(ctx,
			"UPDATE users SET password_hash = ?, updated_at = ? WHERE email = ?",
			string(passwordHash), time.Now().Format(time.RFC3339), email,
		)
		if err != nil {
			return fmt.Errorf("update admin user: %w", err)
		}
		return nil
	}

	if err != sql.ErrNoRows {
		return fmt.Errorf("check existing user: %w", err)
	}

	// Create admin user
	now := time.Now().Format(time.RFC3339)
	_, err = db.ExecContext(ctx, `
		INSERT INTO users (email, password_hash, username, display_name, role, is_active, is_verified, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'admin', true, true, ?, ?)
	`, email, string(passwordHash), email, "Administrator", now, now)
	if err != nil {
		return fmt.Errorf("insert admin user: %w", err)
	}

	return nil
}
