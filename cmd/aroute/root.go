// Package main implements the CLI for ARoute CMS.
package main

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// Global configuration variables
var (
	cfgFile   string
	dataDir   string
	pluginDir string
	logLevel  string
	logFormat string
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "aroute",
	Short: "ARoute CMS - A modern, microkernel-based CMS",
	Long: `ARoute CMS is a modern Content Management System built on a pure microkernel architecture.
It features plugin-based extensibility, dynamic content types, multiple theme engines,
and supports both SQLite and PostgreSQL databases.

All functionality (HTTP server, database, authentication, content management) is provided
through plugins - the core only manages plugin lifecycle, service discovery, and event distribution.`,
	SilenceUsage: true,
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		// Cobra already prints the error
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	// Global persistent flags (available to all subcommands)
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is ./aroute.yaml, $HOME/.config/aroute/aroute.yaml)")
	rootCmd.PersistentFlags().StringVar(&dataDir, "data-dir", "", "data directory (default is ./data)")
	rootCmd.PersistentFlags().StringVar(&pluginDir, "plugin-dir", "", "plugin directory (default is ./plugins)")
	rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "info", "log level (debug, info, warn, error)")
	rootCmd.PersistentFlags().StringVar(&logFormat, "log-format", "text", "log format (text, json)")

	// Bind flags to viper
	viper.BindPFlag("data_dir", rootCmd.PersistentFlags().Lookup("data-dir"))
	viper.BindPFlag("plugins.dir", rootCmd.PersistentFlags().Lookup("plugin-dir"))
	viper.BindPFlag("log.level", rootCmd.PersistentFlags().Lookup("log-level"))
	viper.BindPFlag("log.format", rootCmd.PersistentFlags().Lookup("log-format"))
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	// Set environment variable prefix
	viper.SetEnvPrefix("AROUTE")
	viper.AutomaticEnv() // read in environment variables that match

	// If a config file is explicitly specified, use it
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		// Search for config in multiple locations
		// 1. Current directory
		viper.AddConfigPath(".")
		// 2. Home directory .config/aroute
		home, err := os.UserHomeDir()
		if err == nil {
			viper.AddConfigPath(filepath.Join(home, ".config", "aroute"))
		}
		// 3. /etc/aroute (system-wide config)
		viper.AddConfigPath("/etc/aroute")

		// Config file name without extension (viper will auto-detect format)
		viper.SetConfigName("aroute")
		viper.SetConfigType("yaml")
	}

	// Set default values
	setDefaults()

	// Read config file (if exists)
	if err := viper.ReadInConfig(); err != nil {
		// Config file not found is OK - use defaults and env vars
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok && !os.IsNotExist(err) {
			// Config file was found but another error occurred
			fmt.Fprintf(os.Stderr, "Error reading config file: %v\n", err)
			os.Exit(1)
		}
	}
}

// setDefaults sets default configuration values
func setDefaults() {
	// Server defaults
	viper.SetDefault("server.host", "0.0.0.0")
	viper.SetDefault("server.port", 1337)

	// Database defaults
	viper.SetDefault("database.driver", "sqlite")
	viper.SetDefault("database.sqlite.path", "data/aroute.db")

	// Auth defaults
	viper.SetDefault("auth.jwt_secret", "change-me-to-a-random-string")
	viper.SetDefault("auth.access_token_ttl", "15m")
	viper.SetDefault("auth.refresh_token_ttl", "7d")

	// Media defaults
	viper.SetDefault("media.storage", "local")
	viper.SetDefault("media.local.upload_dir", "data/uploads")
	viper.SetDefault("media.max_file_size", "50MB")

	// Search defaults
	viper.SetDefault("search.index_dir", "data/search")

	// Cache defaults
	viper.SetDefault("cache.max_size", 256)
	viper.SetDefault("cache.default_ttl", "5m")

	// Theme defaults
	viper.SetDefault("theme.active", "default")
	viper.SetDefault("theme.dir", "themes")

	// Logging defaults
	viper.SetDefault("log.level", "info")
	viper.SetDefault("log.format", "text")

	// CORS defaults
	viper.SetDefault("cors.allowed_origins", []string{"*"})
	viper.SetDefault("cors.allowed_methods", []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"})
	viper.SetDefault("cors.allowed_headers", []string{"Authorization", "Content-Type"})

	// Plugins defaults
	viper.SetDefault("plugins.dir", "plugins")

	// Data directory default
	viper.SetDefault("data_dir", "data")
}

// getLogger creates a configured slog logger
func getLogger() *slog.Logger {
	level := parseLogLevel(viper.GetString("log.level"))
	format := viper.GetString("log.format")

	var handler slog.Handler
	opts := &slog.HandlerOptions{Level: level}

	if format == "json" {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}

	return slog.New(handler)
}

// parseLogLevel converts string level to slog.Level
func parseLogLevel(level string) slog.Level {
	switch level {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// getDataDir returns the configured data directory, creating it if needed
func getDataDir() string {
	dir := viper.GetString("data_dir")
	if dir == "" {
		dir = "data"
	}
	return dir
}

// getPluginDir returns the configured plugin directory
func getPluginDir() string {
	dir := viper.GetString("plugins.dir")
	if dir == "" {
		dir = "plugins"
	}
	return dir
}
