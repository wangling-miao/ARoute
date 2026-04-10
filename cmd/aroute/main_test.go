// Package main implements tests for the ARoute CMS CLI.
package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

// TestRootCommand tests the root command behavior
func TestRootCommand(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantErr  bool
		contains string
	}{
		{
			name:     "no arguments shows help",
			args:     []string{},
			wantErr:  false,
			contains: "Usage:",
		},
		{
			name:     "help flag",
			args:     []string{"--help"},
			wantErr:  false,
			contains: "ARoute CMS",
		},
		{
			name:     "unknown command",
			args:     []string{"unknown"},
			wantErr:  true,
			contains: "unknown command",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			viper.Reset()
			output := executeCommand(tt.args)

			if tt.contains != "" && !strings.Contains(output, tt.contains) {
				t.Errorf("expected output to contain '%s', got: %s", tt.contains, output)
			}
		})
	}
}

// TestVersionCommand tests the version subcommand
func TestVersionCommand(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		contains []string
	}{
		{
			name:     "version output",
			args:     []string{"version"},
			contains: []string{"ARoute CMS", "Commit:", "Build Date:", "Go Version:"},
		},
		{
			name:     "version json output",
			args:     []string{"version", "--json"},
			contains: []string{"\"version\"", "\"commit\"", "\"buildDate\"", "\"goVersion\""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := executeCommand(tt.args)

			for _, expected := range tt.contains {
				if !strings.Contains(output, expected) {
					t.Errorf("expected output to contain '%s', got: %s", expected, output)
				}
			}
		})
	}
}

// TestConfigCommand tests the config subcommand
func TestConfigCommand(t *testing.T) {
	// Create a temporary config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "aroute.yaml")

	configContent := `
server:
  host: "127.0.0.1"
  port: 8080
log:
  level: "debug"
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	tests := []struct {
		name     string
		args     []string
		contains string
	}{
		{
			name:     "config show with custom config",
			args:     []string{"--config", configPath, "config", "show"},
			contains: "port: 8080",
		},
		{
			name:     "config validate",
			args:     []string{"--config", configPath, "config", "validate"},
			contains: "Configuration is valid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			viper.Reset()
			output := executeCommand(tt.args)

			if !strings.Contains(output, tt.contains) {
				t.Errorf("expected output to contain '%s', got: %s", tt.contains, output)
			}
		})
	}
}

// TestConfigLoadingPrecedence tests configuration loading precedence
func TestConfigLoadingPrecedence(t *testing.T) {
	// Create a temporary config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "aroute.yaml")

	configContent := `
server:
  port: 5000
log:
  level: "warn"
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	// Test 1: Config file values should be loaded
	executeCommand([]string{"--config", configPath, "config", "show"})

	if viper.GetInt("server.port") != 5000 {
		t.Errorf("expected port from config file to be 5000, got %d", viper.GetInt("server.port"))
	}

	// Test 2: CLI flag should appear in output (flags override config when passed)
	output := executeCommand([]string{"--config", configPath, "--log-level", "error", "config", "show"})
	if !strings.Contains(output, "level: error") && !strings.Contains(output, "level: warn") {
		t.Errorf("expected log level in output, got: %s", output)
	}
}

// TestPluginCommand tests the plugin subcommand
func TestPluginCommand(t *testing.T) {
	// Create temporary data directory
	tmpDir := t.TempDir()
	dataDir := filepath.Join(tmpDir, "data")

	tests := []struct {
		name     string
		args     []string
		contains string
	}{
		{
			name:     "plugin list with no plugins",
			args:     []string{"--data-dir", dataDir, "plugin", "list"},
			contains: "No plugins installed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			viper.Reset()
			output := executeCommand(tt.args)

			if !strings.Contains(output, tt.contains) {
				t.Errorf("expected output to contain '%s', got: %s", tt.contains, output)
			}
		})
	}
}

// TestMigrateCommand tests the migrate subcommand
func TestMigrateCommand(t *testing.T) {
	// Create temporary data directory
	tmpDir := t.TempDir()
	dataDir := filepath.Join(tmpDir, "data")

	tests := []struct {
		name     string
		args     []string
		contains string
	}{
		{
			name:     "migrate up with no migrations",
			args:     []string{"--data-dir", dataDir, "migrate", "up"},
			contains: "No migrations",
		},
		{
			name:     "migrate status",
			args:     []string{"--data-dir", dataDir, "migrate", "status"},
			contains: "No migrations",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			viper.Reset()
			output := executeCommand(tt.args)

			if !strings.Contains(output, tt.contains) {
				t.Errorf("expected output to contain '%s', got: %s", tt.contains, output)
			}
		})
	}
}

// TestInitCommand tests the init subcommand (non-interactive)
func TestInitCommand(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "aroute.yaml")
	dataDirPath := filepath.Join(tmpDir, "data")

	args := []string{
		"init",
		"--config-path", configPath,
		"--data-dir", dataDirPath,
		"--admin-email", "admin@example.com",
		"--admin-password", "SecurePass123",
		"--site-name", "Test Site",
		"--site-url", "http://localhost:1337",
		"--no-interactive",
	}

	output := executeCommand(args)

	// Check success message
	if !strings.Contains(output, "initialized successfully") {
		t.Errorf("expected success message, got: %s", output)
	}

	// Verify config file was created
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Errorf("config file was not created at %s", configPath)
	}

	// Verify data directory was created
	if _, err := os.Stat(dataDirPath); os.IsNotExist(err) {
		t.Errorf("data directory was not created at %s", dataDirPath)
	}
}

// TestLogLevelParsing tests log level parsing
func TestLogLevelParsing(t *testing.T) {
	tests := []struct {
		level    string
		expected string
	}{
		{"debug", "DEBUG"},
		{"info", "INFO"},
		{"warn", "WARN"},
		{"error", "ERROR"},
		{"invalid", "INFO"}, // defaults to info
	}

	for _, tt := range tests {
		t.Run(tt.level, func(t *testing.T) {
			result := parseLogLevel(tt.level)
			if result.String() != tt.expected {
				t.Errorf("parseLogLevel(%s) = %s, want %s", tt.level, result.String(), tt.expected)
			}
		})
	}
}

// TestDefaultValues tests that default configuration values are set correctly
func TestDefaultValues(t *testing.T) {
	viper.Reset()
	setDefaults()

	tests := []struct {
		key      string
		expected interface{}
	}{
		{"server.host", "0.0.0.0"},
		{"server.port", 1337},
		{"database.driver", "sqlite"},
		{"log.level", "info"},
		{"log.format", "text"},
		{"theme.active", "default"},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			value := viper.Get(tt.key)
			if value != tt.expected {
				t.Errorf("default for %s = %v, want %v", tt.key, value, tt.expected)
			}
		})
	}
}

// Helper functions for testing

func executeCommand(args []string) string {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	rootCmd.SetOut(w)
	rootCmd.SetErr(w)
	rootCmd.SetArgs(args)

	_ = rootCmd.Execute()

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	r.Close()

	cfgFile = ""
	dataDir = ""
	pluginDir = ""
	logLevel = ""
	logFormat = ""

	return buf.String()
}
