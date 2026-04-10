package logger

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewManager_Stdout(t *testing.T) {
	cfg := &Config{
		Level:  "info",
		Format: "json",
		Output: OutputStdout,
	}

	m, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	defer m.Close()

	logger := m.Logger()
	if logger == nil {
		t.Fatal("Logger is nil")
	}

	logger.Info("test message 1")
	logger.Info("test message 2")
	logger.Info("test message 3")
}

func TestNewManager_File(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")

	cfg := &Config{
		Level:  "info",
		Format: "json",
		Output: OutputFile,
		File: FileConfig{
			Path:    tmpDir,
			Name:    "test.log",
			MaxSize: 10,
		},
	}

	m, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	defer m.Close()

	logger := m.Logger()
	logger.Info("test message 1", "key1", "value1")
	logger.Info("test message 2", "key2", "value2")
	logger.Info("test message 3", "key3", "value3")

	m.Close()

	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Read log file failed: %v", err)
	}

	lines := strings.Split(string(content), "\n")
	validLines := 0
	for _, line := range lines {
		if strings.Contains(line, "test message") {
			validLines++
		}
	}

	if validLines != 3 {
		t.Errorf("Expected 3 log lines, got %d", validLines)
	}
}

func TestNewManager_Both(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")

	cfg := &Config{
		Level:  "info",
		Format: "json",
		Output: OutputBoth,
		File: FileConfig{
			Path:    tmpDir,
			Name:    "test.log",
			MaxSize: 10,
		},
	}

	m, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	defer m.Close()

	logger := m.Logger()
	logger.Info("test message 1")
	logger.Info("test message 2")
	logger.Info("test message 3")

	m.Close()

	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Read log file failed: %v", err)
	}

	lines := strings.Split(string(content), "\n")
	validLines := 0
	for _, line := range lines {
		if strings.Contains(line, "test message") {
			validLines++
		}
	}

	if validLines != 3 {
		t.Errorf("Expected 3 log lines in file, got %d", validLines)
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Level != "info" {
		t.Errorf("Expected level 'info', got '%s'", cfg.Level)
	}
	if cfg.Output != OutputStdout {
		t.Errorf("Expected output 'stdout', got '%s'", cfg.Output)
	}
}

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"error", slog.LevelError},
		{"unknown", slog.LevelInfo},
	}

	for _, tt := range tests {
		result := parseLevel(tt.input)
		if result != tt.expected {
			t.Errorf("parseLevel(%s) = %v, expected %v", tt.input, result, tt.expected)
		}
	}
}

func TestComponent(t *testing.T) {
	Init(&Config{Level: "info", Format: "json", Output: OutputStdout})
	defer Close()

	logger := Component("test-component")
	if logger == nil {
		t.Fatal("Component logger is nil")
	}

	logger.Info("component test")
}

func TestPlugin(t *testing.T) {
	Init(&Config{Level: "info", Format: "json", Output: OutputStdout})
	defer Close()

	logger := Plugin("test-plugin")
	if logger == nil {
		t.Fatal("Plugin logger is nil")
	}

	logger.Info("plugin test")
}

func TestActionHelpers(t *testing.T) {
	buf := &bytes.Buffer{}
	handler := slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	testLogger := slog.New(handler)

	Action(testLogger, "actor1", "action1", "result1")
	if !strings.Contains(buf.String(), "actor1") {
		t.Error("Action log missing actor")
	}

	buf.Reset()
	ActionSuccess(testLogger, "actor2", "action2")
	if !strings.Contains(buf.String(), "success") {
		t.Error("ActionSuccess log missing 'success'")
	}

	buf.Reset()
	ActionError(testLogger, "actor3", "action3", os.ErrNotExist)
	if !strings.Contains(buf.String(), "failed") {
		t.Error("ActionError log missing 'failed'")
	}
}
