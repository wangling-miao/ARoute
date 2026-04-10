package logger

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"gopkg.in/natefinch/lumberjack.v2"
)

// OutputTarget defines where logs should be written.
type OutputTarget string

const (
	OutputStdout OutputTarget = "stdout"
	OutputFile   OutputTarget = "file"
	OutputBoth   OutputTarget = "both"
)

// Config defines the logger configuration.
type Config struct {
	// Level: debug, info, warn, error
	Level string `yaml:"level" json:"level" mapstructure:"level"`

	// Format: json, text
	Format string `yaml:"format" json:"format" mapstructure:"format"`

	// Output: stdout, file, both
	Output OutputTarget `yaml:"output" json:"output" mapstructure:"output"`

	// File settings (used when Output is file or both)
	File FileConfig `yaml:"file" json:"file" mapstructure:"file"`
}

// FileConfig defines file logging settings.
type FileConfig struct {
	// Path: directory for log files (created if not exists)
	Path string `yaml:"path" json:"path" mapstructure:"path"`

	// Name: base name for log files (default: aroute.log)
	Name string `yaml:"name" json:"name" mapstructure:"name"`

	// MaxSize: max size in MB before rotation (default: 100)
	MaxSize int `yaml:"max_size" json:"max_size" mapstructure:"max_size"`

	// MaxAge: max days to keep old files (default: 30, 0 = forever)
	MaxAge int `yaml:"max_age" json:"max_age" mapstructure:"max_age"`

	// MaxBackups: max number of old files to keep (default: 10, 0 = all)
	MaxBackups int `yaml:"max_backups" json:"max_backups" mapstructure:"max_backups"`

	// Compress: compress rotated files (default: true)
	Compress bool `yaml:"compress" json:"compress" mapstructure:"compress"`
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		Level:  "info",
		Format: "json",
		Output: OutputStdout,
		File: FileConfig{
			Path:       "data/logs",
			Name:       "aroute.log",
			MaxSize:    100,
			MaxAge:     30,
			MaxBackups: 10,
			Compress:   true,
		},
	}
}

// Manager manages the global logger instance.
type Manager struct {
	mu     sync.RWMutex
	logger *slog.Logger
	config *Config
	closer io.Closer
}

var (
	globalManager *Manager
	globalMu      sync.RWMutex
)

// NewManager creates a new logger manager with the given configuration.
func NewManager(cfg *Config) (*Manager, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	m := &Manager{
		config: cfg,
	}

	logger, closer, err := m.createLogger(cfg)
	if err != nil {
		return nil, err
	}

	m.logger = logger
	m.closer = closer

	return m, nil
}

// createLogger creates a slog.Logger based on configuration.
func (m *Manager) createLogger(cfg *Config) (*slog.Logger, io.Closer, error) {
	level := parseLevel(cfg.Level)
	opts := &slog.HandlerOptions{Level: level}

	var writers []io.Writer
	var closers []io.Closer

	switch cfg.Output {
	case OutputStdout:
		writers = []io.Writer{os.Stdout}

	case OutputFile:
		fileWriter, closer, err := createFileWriter(&cfg.File)
		if err != nil {
			return nil, nil, err
		}
		writers = []io.Writer{fileWriter}
		closers = []io.Closer{closer}

	case OutputBoth:
		fileWriter, closer, err := createFileWriter(&cfg.File)
		if err != nil {
			return nil, nil, err
		}
		writers = []io.Writer{os.Stdout, fileWriter}
		closers = []io.Closer{closer}

	default:
		writers = []io.Writer{os.Stdout}
	}

	multiWriter := io.MultiWriter(writers...)

	var handler slog.Handler
	if cfg.Format == "json" {
		handler = slog.NewJSONHandler(multiWriter, opts)
	} else {
		handler = slog.NewTextHandler(multiWriter, opts)
	}

	closer := multiCloser(closers)
	return slog.New(handler), closer, nil
}

// createFileWriter creates a rotating file writer using lumberjack.
func createFileWriter(cfg *FileConfig) (io.Writer, io.Closer, error) {
	if cfg.Path == "" {
		cfg.Path = "data/logs"
	}
	if cfg.Name == "" {
		cfg.Name = "aroute.log"
	}
	if cfg.MaxSize == 0 {
		cfg.MaxSize = 100
	}

	if err := os.MkdirAll(cfg.Path, 0755); err != nil {
		return nil, nil, fmt.Errorf("create log directory: %w", err)
	}

	filePath := filepath.Join(cfg.Path, cfg.Name)

	lj := &lumberjack.Logger{
		Filename:   filePath,
		MaxSize:    cfg.MaxSize,
		MaxAge:     cfg.MaxAge,
		MaxBackups: cfg.MaxBackups,
		Compress:   cfg.Compress,
		LocalTime:  true,
	}

	return lj, lj, nil
}

// multiCloser combines multiple closers into one.
type multiCloser []io.Closer

func (mc multiCloser) Close() error {
	for _, c := range mc {
		if c != nil {
			_ = c.Close()
		}
	}
	return nil
}

// parseLevel converts string to slog.Level.
func parseLevel(level string) slog.Level {
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

// Logger returns the underlying slog.Logger.
func (m *Manager) Logger() *slog.Logger {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.logger
}

// Close closes the logger and releases resources.
func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closer != nil {
		return m.closer.Close()
	}
	return nil
}

// Global functions for convenience

// Init initializes the global logger manager.
func Init(cfg *Config) error {
	globalMu.Lock()
	defer globalMu.Unlock()

	m, err := NewManager(cfg)
	if err != nil {
		return err
	}

	if globalManager != nil {
		_ = globalManager.Close()
	}

	globalManager = m
	slog.SetDefault(m.Logger())
	return nil
}

// Get returns the global logger.
func Get() *slog.Logger {
	globalMu.RLock()
	defer globalMu.RUnlock()

	if globalManager == nil {
		return slog.Default()
	}
	return globalManager.Logger()
}

// Close closes the global logger.
func Close() error {
	globalMu.Lock()
	defer globalMu.Unlock()

	if globalManager != nil {
		err := globalManager.Close()
		globalManager = nil
		return err
	}
	return nil
}

// Component returns a logger with component context.
func Component(name string) *slog.Logger {
	return Get().With("component", name)
}

// Plugin returns a logger with plugin context.
func Plugin(name string) *slog.Logger {
	return Get().With("plugin", name)
}

// Action logs a structured action: who + when + what + result.
func Action(logger *slog.Logger, actor, action string, result string, attrs ...any) {
	args := []any{"actor", actor, "action", action, "result", result}
	args = append(args, attrs...)
	logger.Info("action", args...)
}

// ActionSuccess logs a successful action.
func ActionSuccess(logger *slog.Logger, actor, action string, attrs ...any) {
	args := []any{"actor", actor, "action", action, "result", "success"}
	args = append(args, attrs...)
	logger.Info("action completed", args...)
}

// ActionError logs a failed action.
func ActionError(logger *slog.Logger, actor, action string, err error, attrs ...any) {
	args := []any{"actor", actor, "action", action, "result", "failed", "error", err.Error()}
	args = append(args, attrs...)
	logger.Error("action failed", args...)
}
