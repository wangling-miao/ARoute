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

// Error path tests

func TestCreateFileWriter_MkdirAllError(t *testing.T) {
	// Test error when path is invalid (e.g., path under a file)
	tmpFile := filepath.Join(t.TempDir(), "blocked")
	if err := os.WriteFile(tmpFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Setup failed: %v", err)
	}

	// Try to create logs under a file path (should fail)
	cfg := &FileConfig{
		Path:    tmpFile + "/logs", // path under a file, should fail MkdirAll
		Name:    "test.log",
		MaxSize: 10,
	}

	_, _, err := createFileWriter(cfg)
	if err == nil {
		t.Error("Expected error from MkdirAll, got nil")
	}
	if !strings.Contains(err.Error(), "create log directory") {
		t.Errorf("Expected 'create log directory' error, got: %v", err)
	}
}

func TestNewManager_FileError(t *testing.T) {
	// Test that NewManager propagates file creation errors
	tmpFile := filepath.Join(t.TempDir(), "blocked")
	if err := os.WriteFile(tmpFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Setup failed: %v", err)
	}

	cfg := &Config{
		Level:  "info",
		Format: "json",
		Output: OutputFile,
		File: FileConfig{
			Path:    tmpFile + "/logs",
			Name:    "test.log",
			MaxSize: 10,
		},
	}

	m, err := NewManager(cfg)
	if err == nil {
		t.Error("Expected error from NewManager, got nil")
		if m != nil {
			m.Close()
		}
	}
	if !strings.Contains(err.Error(), "create log directory") {
		t.Errorf("Expected directory creation error, got: %v", err)
	}
}

func TestNewManager_BothOutputError(t *testing.T) {
	// Test error propagation for OutputBoth
	tmpFile := filepath.Join(t.TempDir(), "blocked")
	if err := os.WriteFile(tmpFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Setup failed: %v", err)
	}

	cfg := &Config{
		Level:  "info",
		Format: "json",
		Output: OutputBoth,
		File: FileConfig{
			Path:    tmpFile + "/logs",
			Name:    "test.log",
			MaxSize: 10,
		},
	}

	m, err := NewManager(cfg)
	if err == nil {
		t.Error("Expected error from NewManager with OutputBoth, got nil")
		if m != nil {
			m.Close()
		}
	}
}

func TestNewManager_TextFormat(t *testing.T) {
	cfg := &Config{
		Level:  "info",
		Format: "text",
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

	logger.Info("text format test")
}

func TestNewManager_UnknownOutput(t *testing.T) {
	// Test default case (unknown output falls back to stdout)
	cfg := &Config{
		Level:  "info",
		Format: "json",
		Output: OutputTarget("unknown"),
	}

	m, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("NewManager failed for unknown output: %v", err)
	}
	defer m.Close()

	logger := m.Logger()
	if logger == nil {
		t.Fatal("Logger is nil")
	}

	logger.Info("unknown output test")
}

func TestNewManager_NilConfig(t *testing.T) {
	// Test that nil config uses defaults
	m, err := NewManager(nil)
	if err != nil {
		t.Fatalf("NewManager with nil config failed: %v", err)
	}
	defer m.Close()

	if m.config.Level != "info" {
		t.Errorf("Expected default level 'info', got '%s'", m.config.Level)
	}
}

func TestManager_CloseNilCloser(t *testing.T) {
	// Test Close with nil closer (stdout output has no closer)
	cfg := &Config{
		Level:  "info",
		Format: "json",
		Output: OutputStdout,
	}

	m, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	// Multiple Close calls should work
	if err := m.Close(); err != nil {
		t.Errorf("First Close failed: %v", err)
	}
	if err := m.Close(); err != nil {
		t.Errorf("Second Close failed: %v", err)
	}
}

func TestGet_NilGlobalManager(t *testing.T) {
	// Ensure global manager is nil
	Close()

	logger := Get()
	if logger == nil {
		t.Error("Get() returned nil, should return slog.Default()")
	}
	logger.Info("test with default logger")
}

func TestClose_NilGlobalManager(t *testing.T) {
	// Ensure global manager is nil
	Close()

	err := Close()
	if err != nil {
		t.Errorf("Close with nil globalManager should return nil, got: %v", err)
	}
}

func TestInit_ErrorPropagation(t *testing.T) {
	// Reset global state
	Close()

	tmpFile := filepath.Join(t.TempDir(), "blocked")
	if err := os.WriteFile(tmpFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Setup failed: %v", err)
	}

	cfg := &Config{
		Level:  "info",
		Format: "json",
		Output: OutputFile,
		File: FileConfig{
			Path:    tmpFile + "/logs",
			Name:    "test.log",
			MaxSize: 10,
		},
	}

	err := Init(cfg)
	if err == nil {
		t.Error("Expected Init to return error, got nil")
		Close()
	}
	if !strings.Contains(err.Error(), "create log directory") {
		t.Errorf("Expected directory creation error, got: %v", err)
	}
}

func TestInit_ReplacesExisting(t *testing.T) {
	// Initialize first logger
	cfg1 := &Config{Level: "info", Format: "json", Output: OutputStdout}
	if err := Init(cfg1); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	logger1 := Get()
	if logger1 == nil {
		t.Fatal("First logger is nil")
	}

	// Initialize second logger (should replace first)
	cfg2 := &Config{Level: "debug", Format: "text", Output: OutputStdout}
	if err := Init(cfg2); err != nil {
		t.Fatalf("Second Init failed: %v", err)
	}
	defer Close()

	logger2 := Get()
	if logger2 == nil {
		t.Fatal("Second logger is nil")
	}

	// Both should work (they may be the same handler with different config)
	logger2.Debug("debug message after replacement")
}

func TestCreateFileWriter_Defaults(t *testing.T) {
	// Test that defaults are applied when config fields are empty/zero
	cfg := &FileConfig{
		Path:    "", // Should default to "data/logs"
		Name:    "", // Should default to "aroute.log"
		MaxSize: 0,  // Should default to 100
	}

	// We can't test actual file creation with defaults without side effects,
	// but we can verify the defaults are set by creating with explicit path
	tmpDir := t.TempDir()
	cfg.Path = tmpDir

	_, closer, err := createFileWriter(cfg)
	if err != nil {
		t.Fatalf("createFileWriter failed: %v", err)
	}
	defer closer.Close()

	if cfg.Name != "aroute.log" {
		t.Errorf("Expected default name 'aroute.log', got '%s'", cfg.Name)
	}
	if cfg.MaxSize != 100 {
		t.Errorf("Expected default MaxSize 100, got %d", cfg.MaxSize)
	}

}

func TestMultiCloser(t *testing.T) {
	// Test multiCloser with nil and non-nil closers
	mc := multiCloser{nil, nil, nil}
	if err := mc.Close(); err != nil {
		t.Errorf("multiCloser with nils should return nil, got: %v", err)
	}

	// Test with actual closers
	tmpDir := t.TempDir()
	cfg := &FileConfig{Path: tmpDir, Name: "test.log", MaxSize: 10}
	_, closer1, err := createFileWriter(cfg)
	if err != nil {
		t.Fatalf("createFileWriter failed: %v", err)
	}
	_, closer2, err := createFileWriter(cfg)
	if err != nil {
		t.Fatalf("createFileWriter second failed: %v", err)
	}

	mc2 := multiCloser{closer1, closer2, nil}
	if err := mc2.Close(); err != nil {
		t.Errorf("multiCloser.Close failed: %v", err)
	}
}

func TestActionError_WithAttrs(t *testing.T) {
	buf := &bytes.Buffer{}
	handler := slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	testLogger := slog.New(handler)

	ActionError(testLogger, "actor", "action", os.ErrNotExist, "extra_key", "extra_value")

	output := buf.String()
	if !strings.Contains(output, "extra_key") {
		t.Error("ActionError missing extra attribute")
	}
	if !strings.Contains(output, "extra_value") {
		t.Error("ActionError missing extra attribute value")
	}
}
