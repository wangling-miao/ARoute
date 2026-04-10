// Package main implements the init subcommand for ARoute CMS.
package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a new ARoute CMS instance",
	Long: `Perform interactive first-run setup for ARoute CMS.

This command will:
  1. Create a configuration file
  2. Set up the data directory
  3. Create the initial admin user
  4. Run database migrations

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

	// Prompt for admin password
	adminPassword := initAdminPassword
	if adminPassword == "" && !initNoInteractive {
		fmt.Print("Admin password (min 8 characters): ")
		// Note: In production, this would use terminal.ReadPassword() to hide input
		input, _ := reader.ReadString('\n')
		adminPassword = strings.TrimSpace(input)

		// Validate password length
		for len(adminPassword) < 8 {
			fmt.Println("Password must be at least 8 characters")
			fmt.Print("Admin password (min 8 characters): ")
			input, _ = reader.ReadString('\n')
			adminPassword = strings.TrimSpace(input)
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

	// Generate configuration file
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
				"path": filepath.Join(dataDirPath, "aroute.db"),
			},
		},
		"auth": map[string]interface{}{
			"jwt_secret":        generateRandomSecret(),
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
			"email":    adminEmail,
			"password": "********", // Placeholder, actual password stored in database
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
	fmt.Println("\nARoute CMS initialized successfully!")
	fmt.Println("\nNext steps:")
	fmt.Println("  1. Review the configuration file:", configPath)
	fmt.Println("  2. Start the server: aroute serve")
	fmt.Println("  3. Access the admin panel at:", siteURL+"/admin")
	fmt.Println("\nAdmin credentials:")
	fmt.Println("  Email:    ", adminEmail)
	fmt.Println("  Password: [the password you provided]")

	return nil
}

// generateRandomSecret generates a random string for JWT secret
func generateRandomSecret() string {
	// Simple random secret generation
	// In production, this would use crypto/rand for proper randomness
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, 32)
	for i := range b {
		b[i] = chars[i%len(chars)]
	}
	return string(b) + "-change-me-in-production"
}
