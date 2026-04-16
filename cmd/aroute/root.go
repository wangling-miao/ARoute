// Package main implements the CLI for ARoute CMS.
package main

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/wangling-miao/aroute/core/logger"
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

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is ./aroute.yaml, $HOME/.config/aroute/aroute.yaml)")
	rootCmd.PersistentFlags().StringVar(&dataDir, "data-dir", "", "data directory (default is ./data)")
	rootCmd.PersistentFlags().StringVar(&pluginDir, "plugin-dir", "", "plugin directory (default is ./plugins)")
	rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "info", "log level (debug, info, warn, error)")
	rootCmd.PersistentFlags().StringVar(&logFormat, "log-format", "text", "log format (text, json)")

	viper.BindPFlag("data_dir", rootCmd.PersistentFlags().Lookup("data-dir"))
	viper.BindPFlag("plugins.dir", rootCmd.PersistentFlags().Lookup("plugin-dir"))
	viper.BindPFlag("log.level", rootCmd.PersistentFlags().Lookup("log-level"))
	viper.BindPFlag("log.format", rootCmd.PersistentFlags().Lookup("log-format"))
}

var skipConfigCommands = map[string]bool{
	"version":    true,
	"help":       true,
	"completion": true,
	"plugin":     true,
	"init":       true,
}

func initConfig() {
	if len(os.Args) > 1 {
		cmdName := os.Args[1]
		if skipConfigCommands[cmdName] {
			return
		}
	}

	viper.SetEnvPrefix("AROUTE")
	viper.AutomaticEnv()

	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
		viper.SetConfigType(detectConfigType(cfgFile))
	} else {
		viper.AddConfigPath(".")
		home, err := os.UserHomeDir()
		if err == nil {
			viper.AddConfigPath(filepath.Join(home, ".config", "aroute"))
		}
		viper.AddConfigPath("/etc/aroute")

		viper.SetConfigName("aroute")
		viper.SetConfigType("yaml")
	}

	setDefaults()

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok && !os.IsNotExist(err) {
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

	// Auth defaults (jwt_secret intentionally omitted — auto-generated if not set)
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
	viper.SetDefault("log.output", "stdout")
	viper.SetDefault("log.file.path", "data/logs")
	viper.SetDefault("log.file.name", "aroute.log")
	viper.SetDefault("log.file.max_size", 100)
	viper.SetDefault("log.file.max_age", 30)
	viper.SetDefault("log.file.max_backups", 10)
	viper.SetDefault("log.file.compress", true)

	// CORS defaults
	viper.SetDefault("cors.allowed_origins", []string{"*"})
	viper.SetDefault("cors.allowed_methods", []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"})
	viper.SetDefault("cors.allowed_headers", []string{"Authorization", "Content-Type"})

	// Plugins defaults
	viper.SetDefault("plugins.dir", "plugins")

	// Data directory default
	viper.SetDefault("data_dir", "data")
}

// initLogger initializes the global logger from configuration.
func initLogger() error {
	// Start with defaults
	defaultCfg := logger.DefaultConfig()

	// Override with config values if set
	cfg := &logger.Config{
		Level:  viper.GetString("log.level"),
		Format: viper.GetString("log.format"),
		Output: logger.OutputTarget(viper.GetString("log.output")),
		File: logger.FileConfig{
			Path:       viper.GetString("log.file.path"),
			Name:       viper.GetString("log.file.name"),
			MaxSize:    viper.GetInt("log.file.max_size"),
			MaxAge:     viper.GetInt("log.file.max_age"),
			MaxBackups: viper.GetInt("log.file.max_backups"),
			Compress:   viper.GetBool("log.file.compress"),
		},
	}

	// Apply defaults for empty values
	if cfg.Level == "" {
		cfg.Level = defaultCfg.Level
	}
	if cfg.Format == "" {
		cfg.Format = defaultCfg.Format
	}
	if cfg.Output == "" {
		cfg.Output = defaultCfg.Output
	}
	if cfg.File.Path == "" {
		cfg.File.Path = defaultCfg.File.Path
	}
	if cfg.File.Name == "" {
		cfg.File.Name = defaultCfg.File.Name
	}
	if cfg.File.MaxSize == 0 {
		cfg.File.MaxSize = defaultCfg.File.MaxSize
	}
	if cfg.File.MaxAge == 0 {
		cfg.File.MaxAge = defaultCfg.File.MaxAge
	}
	if cfg.File.MaxBackups == 0 {
		cfg.File.MaxBackups = defaultCfg.File.MaxBackups
	}

	return logger.Init(cfg)
}

func getLogger() *slog.Logger {
	return logger.Get()
}

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

// parseLogLevel converts string to slog.Level.
// Returns slog.LevelInfo for unknown/invalid levels.
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

// detectConfigType determines config file type from extension.
// Returns "toml" for .toml files, "yaml" for .yaml/.yml files,
// and "yaml" as default for unknown extensions.
func detectConfigType(configPath string) string {
	ext := filepath.Ext(configPath)
	switch ext {
	case ".toml":
		return "toml"
	case ".yaml", ".yml":
		return "yaml"
	default:
		return "yaml"
	}
}
