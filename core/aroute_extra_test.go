package core

import (
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/spf13/viper"
)

type errorMockLifecycleManager struct {
	mockLifecycleManager
	loadAllError error
	startError   error
	stopError    error
}

func (m *errorMockLifecycleManager) LoadAll(ctx context.Context) error {
	if m.loadAllError != nil {
		return m.loadAllError
	}
	return m.mockLifecycleManager.LoadAll(ctx)
}

func (m *errorMockLifecycleManager) Start(ctx context.Context) error {
	if m.startError != nil {
		return m.startError
	}
	return m.mockLifecycleManager.Start(ctx)
}

func (m *errorMockLifecycleManager) Stop(ctx context.Context) error {
	if m.stopError != nil {
		return m.stopError
	}
	return m.mockLifecycleManager.Stop(ctx)
}

type errorMockRegistry struct {
	mockPluginRegistry
	closeError error
}

func (r *errorMockRegistry) Close() error {
	if r.closeError != nil {
		return r.closeError
	}
	return r.mockPluginRegistry.Close()
}

// errorMockDispatcher returns error on Close
type errorMockDispatcher struct {
	mockEngineDispatcher
	closeError error
}

func (d *errorMockDispatcher) Close() error {
	if d.closeError != nil {
		return d.closeError
	}
	return d.mockEngineDispatcher.Close()
}

// errorMockLicenseValidator returns error on Validate
type errorMockLicenseValidator struct {
	mockLicenseValidator
	validateError error
}

func (v *errorMockLicenseValidator) Validate() error {
	if v.validateError != nil {
		return v.validateError
	}
	return v.mockLicenseValidator.Validate()
}

// mockDDLRegistry implements DDLRegistry for testing
type mockDDLRegistry struct {
	initError   error
	createError error
	getError    error
	updateError error
	deleteError error
	listError   error
	items       []interface{}
	mu          sync.RWMutex
}

func newMockDDLRegistry() *mockDDLRegistry {
	return &mockDDLRegistry{
		items: make([]interface{}, 0),
	}
}

func (r *mockDDLRegistry) Init(ctx context.Context) error {
	return r.initError
}

func (r *mockDDLRegistry) Create(ctx context.Context, schema interface{}) error {
	if r.createError != nil {
		return r.createError
	}
	r.mu.Lock()
	r.items = append(r.items, schema)
	r.mu.Unlock()
	return nil
}

func (r *mockDDLRegistry) Get(ctx context.Context, name string) (interface{}, error) {
	if r.getError != nil {
		return nil, r.getError
	}
	return fmt.Sprintf("schema-%s", name), nil
}

func (r *mockDDLRegistry) Update(ctx context.Context, schema interface{}) error {
	return r.updateError
}

func (r *mockDDLRegistry) Delete(ctx context.Context, name string, force bool) error {
	return r.deleteError
}

func (r *mockDDLRegistry) List(ctx context.Context) ([]interface{}, error) {
	if r.listError != nil {
		return nil, r.listError
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.items, nil
}

// ============================================
// Functional Options Tests
// ============================================

func TestNew_WithDataDir(t *testing.T) {
	ctx := context.Background()
	tmpDir, err := os.MkdirTemp("", "aroute-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	customDataDir := filepath.Join(tmpDir, "custom-data")
	aroute, err := New(ctx,
		newMockServiceContainer(),
		newMockEventBus(),
		newMockPluginRegistry(),
		newMockLifecycleManager(),
		newMockEngineDispatcher(),
		newMockLicenseValidator(),
		WithDataDir(customDataDir),
		WithPluginDir(filepath.Join(tmpDir, "plugins")),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if aroute.DataDir() != customDataDir {
		t.Errorf("DataDir() = %q, want %q", aroute.DataDir(), customDataDir)
	}

	// Verify directory was created
	if _, err := os.Stat(customDataDir); os.IsNotExist(err) {
		t.Error("data directory was not created")
	}
}

func TestNew_WithPluginDir(t *testing.T) {
	ctx := context.Background()
	tmpDir, err := os.MkdirTemp("", "aroute-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	customPluginDir := filepath.Join(tmpDir, "custom-plugins")
	aroute, err := New(ctx,
		newMockServiceContainer(),
		newMockEventBus(),
		newMockPluginRegistry(),
		newMockLifecycleManager(),
		newMockEngineDispatcher(),
		newMockLicenseValidator(),
		WithDataDir(filepath.Join(tmpDir, "data")),
		WithPluginDir(customPluginDir),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if aroute.PluginDir() != customPluginDir {
		t.Errorf("PluginDir() = %q, want %q", aroute.PluginDir(), customPluginDir)
	}

	// Verify directory was created
	if _, err := os.Stat(customPluginDir); os.IsNotExist(err) {
		t.Error("plugin directory was not created")
	}
}

func TestNew_WithLogger(t *testing.T) {
	ctx := context.Background()
	tmpDir, err := os.MkdirTemp("", "aroute-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create custom logger
	customLogger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	aroute, err := New(ctx,
		newMockServiceContainer(),
		newMockEventBus(),
		newMockPluginRegistry(),
		newMockLifecycleManager(),
		newMockEngineDispatcher(),
		newMockLicenseValidator(),
		WithDataDir(filepath.Join(tmpDir, "data")),
		WithPluginDir(filepath.Join(tmpDir, "plugins")),
		WithLogger(customLogger),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if aroute == nil {
		t.Fatal("New() returned nil")
	}

	// Verify engine was created successfully with custom logger
	// (Logger is internal, we verify via successful creation)
}

func TestNew_WithLicensePath(t *testing.T) {
	ctx := context.Background()
	tmpDir, err := os.MkdirTemp("", "aroute-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	aroute, err := New(ctx,
		newMockServiceContainer(),
		newMockEventBus(),
		newMockPluginRegistry(),
		newMockLifecycleManager(),
		newMockEngineDispatcher(),
		newMockLicenseValidator(),
		WithDataDir(filepath.Join(tmpDir, "data")),
		WithPluginDir(filepath.Join(tmpDir, "plugins")),
		WithLicensePath(filepath.Join(tmpDir, "license.json")),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if aroute == nil {
		t.Fatal("New() returned nil")
	}
}

func TestNew_AllOptionsCombined(t *testing.T) {
	ctx := context.Background()
	tmpDir, err := os.MkdirTemp("", "aroute-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	customDataDir := filepath.Join(tmpDir, "my-data")
	customPluginDir := filepath.Join(tmpDir, "my-plugins")
	customLogger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	aroute, err := New(ctx,
		newMockServiceContainer(),
		newMockEventBus(),
		newMockPluginRegistry(),
		newMockLifecycleManager(),
		newMockEngineDispatcher(),
		newMockLicenseValidator(),
		WithDataDir(customDataDir),
		WithPluginDir(customPluginDir),
		WithLogger(customLogger),
		WithLicensePath(filepath.Join(tmpDir, "license.json")),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if aroute.DataDir() != customDataDir {
		t.Errorf("DataDir() = %q, want %q", aroute.DataDir(), customDataDir)
	}
	if aroute.PluginDir() != customPluginDir {
		t.Errorf("PluginDir() = %q, want %q", aroute.PluginDir(), customPluginDir)
	}
}

// ============================================
// Error Handling Tests
// ============================================

func TestNew_DataDirCreationError(t *testing.T) {
	ctx := context.Background()

	// Try to create data dir in a path that cannot be written (e.g., /proc)
	aroute, err := New(ctx,
		newMockServiceContainer(),
		newMockEventBus(),
		newMockPluginRegistry(),
		newMockLifecycleManager(),
		newMockEngineDispatcher(),
		newMockLicenseValidator(),
		WithDataDir("/proc/nonexistent/aroute/data"),
		WithPluginDir("/tmp/aroute/plugins"),
	)
	if err == nil {
		t.Error("New() should fail when data directory cannot be created")
		if aroute != nil {
			aroute.Stop(ctx)
		}
	}
}

func TestNew_PluginDirCreationError(t *testing.T) {
	ctx := context.Background()
	tmpDir, err := os.MkdirTemp("", "aroute-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a file to block directory creation
	blockFile := filepath.Join(tmpDir, "blocked")
	if err := os.WriteFile(blockFile, []byte{}, 0644); err != nil {
		t.Fatalf("write block file: %v", err)
	}

	aroute, err := New(ctx,
		newMockServiceContainer(),
		newMockEventBus(),
		newMockPluginRegistry(),
		newMockLifecycleManager(),
		newMockEngineDispatcher(),
		newMockLicenseValidator(),
		WithDataDir(filepath.Join(tmpDir, "data")),
		WithPluginDir(blockFile), // File path, not directory
	)
	if err == nil {
		t.Error("New() should fail when plugin directory cannot be created")
		if aroute != nil {
			aroute.Stop(ctx)
		}
	}
}

func TestNew_LicenseValidationError(t *testing.T) {
	ctx := context.Background()
	tmpDir, err := os.MkdirTemp("", "aroute-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	license := &errorMockLicenseValidator{
		validateError: errors.New("license expired"),
	}

	aroute, err := New(ctx,
		newMockServiceContainer(),
		newMockEventBus(),
		newMockPluginRegistry(),
		newMockLifecycleManager(),
		newMockEngineDispatcher(),
		license,
		WithDataDir(filepath.Join(tmpDir, "data")),
		WithPluginDir(filepath.Join(tmpDir, "plugins")),
	)
	if err == nil {
		t.Error("New() should fail when license validation fails")
		if aroute != nil {
			aroute.Stop(ctx)
		}
		return
	}

	if !errors.Is(err, license.validateError) {
		t.Errorf("error should wrap license error, got: %v", err)
	}
}

func TestStart_LoadAllError(t *testing.T) {
	ctx := context.Background()
	tmpDir, err := os.MkdirTemp("", "aroute-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	lifecycle := &errorMockLifecycleManager{
		loadAllError: errors.New("failed to load plugins"),
	}

	aroute, err := New(ctx,
		newMockServiceContainer(),
		newMockEventBus(),
		newMockPluginRegistry(),
		lifecycle,
		newMockEngineDispatcher(),
		newMockLicenseValidator(),
		WithDataDir(filepath.Join(tmpDir, "data")),
		WithPluginDir(filepath.Join(tmpDir, "plugins")),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if err := aroute.Start(ctx); err == nil {
		t.Error("Start() should fail when LoadAll fails")
	}
}

func TestStart_StartPluginsError(t *testing.T) {
	ctx := context.Background()
	tmpDir, err := os.MkdirTemp("", "aroute-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	lifecycle := &errorMockLifecycleManager{
		startError: errors.New("failed to start plugins"),
	}

	aroute, err := New(ctx,
		newMockServiceContainer(),
		newMockEventBus(),
		newMockPluginRegistry(),
		lifecycle,
		newMockEngineDispatcher(),
		newMockLicenseValidator(),
		WithDataDir(filepath.Join(tmpDir, "data")),
		WithPluginDir(filepath.Join(tmpDir, "plugins")),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if err := aroute.Start(ctx); err == nil {
		t.Error("Start() should fail when Start fails")
	}
}

func TestStart_DDLInitError(t *testing.T) {
	ctx := context.Background()
	tmpDir, err := os.MkdirTemp("", "aroute-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	ddl := &mockDDLRegistry{
		initError: errors.New("ddl init failed"),
	}

	aroute, err := New(ctx,
		newMockServiceContainer(),
		newMockEventBus(),
		newMockPluginRegistry(),
		newMockLifecycleManager(),
		newMockEngineDispatcher(),
		newMockLicenseValidator(),
		WithDataDir(filepath.Join(tmpDir, "data")),
		WithPluginDir(filepath.Join(tmpDir, "plugins")),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	aroute.SetDDL(ddl)

	if err := aroute.Start(ctx); err == nil {
		t.Error("Start() should fail when DDL init fails")
	}
}

func TestStop_MultipleErrors(t *testing.T) {
	ctx := context.Background()
	tmpDir, err := os.MkdirTemp("", "aroute-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	lifecycle := &errorMockLifecycleManager{
		stopError: errors.New("stop plugins failed"),
	}
	registry := &errorMockRegistry{
		closeError: errors.New("registry close failed"),
	}
	dispatcher := &errorMockDispatcher{
		closeError: errors.New("dispatcher close failed"),
	}

	aroute, err := New(ctx,
		newMockServiceContainer(),
		newMockEventBus(),
		registry,
		lifecycle,
		dispatcher,
		newMockLicenseValidator(),
		WithDataDir(filepath.Join(tmpDir, "data")),
		WithPluginDir(filepath.Join(tmpDir, "plugins")),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	// Start first
	if err := aroute.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Stop should collect all errors
	err = aroute.Stop(ctx)
	if err == nil {
		t.Error("Stop() should return error when multiple subsystems fail")
	}

	// Verify error message contains all subsystems
	errStr := err.Error()
	if !containsSubstring(errStr, "stop plugins") {
		t.Errorf("error should mention plugins: %q", errStr)
	}
	if !containsSubstring(errStr, "close dispatcher") {
		t.Errorf("error should mention dispatcher: %q", errStr)
	}
	if !containsSubstring(errStr, "close registry") {
		t.Errorf("error should mention registry: %q", errStr)
	}
}

func TestStop_WithTimeout(t *testing.T) {
	ctx := context.Background()
	tmpDir, err := os.MkdirTemp("", "aroute-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create lifecycle that blocks
	blockLifecycle := &blockingMockLifecycle{
		mockLifecycleManager: *newMockLifecycleManager(),
		stopDelay:            100 * time.Millisecond,
	}

	aroute, err := New(ctx,
		newMockServiceContainer(),
		newMockEventBus(),
		newMockPluginRegistry(),
		blockLifecycle,
		newMockEngineDispatcher(),
		newMockLicenseValidator(),
		WithDataDir(filepath.Join(tmpDir, "data")),
		WithPluginDir(filepath.Join(tmpDir, "plugins")),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if err := aroute.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Create context with timeout shorter than stop delay
	timeoutCtx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// Stop should complete even with short timeout (just returns errors)
	// Note: The implementation doesn't enforce context timeout on Stop,
	// but we test the context propagation
	done := make(chan error, 1)
	go func() {
		done <- aroute.Stop(timeoutCtx)
	}()

	select {
	case err := <-done:
		// Stop completed (may or may not timeout)
		if err != nil {
			// Acceptable - timeout or subsystem error
		}
	case <-time.After(200 * time.Millisecond):
		t.Error("Stop() took too long")
	}
}

// blockingMockLifecycle adds delay to Stop
type blockingMockLifecycle struct {
	mockLifecycleManager
	stopDelay time.Duration
}

func (m *blockingMockLifecycle) Stop(ctx context.Context) error {
	time.Sleep(m.stopDelay)
	return m.mockLifecycleManager.Stop(ctx)
}

// ============================================
// Accessor Methods Tests
// ============================================

func TestAroute_ServicesAccessor(t *testing.T) {
	ctx := context.Background()
	tmpDir, err := os.MkdirTemp("", "aroute-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	container := newMockServiceContainer()
	aroute, err := New(ctx,
		container,
		newMockEventBus(),
		newMockPluginRegistry(),
		newMockLifecycleManager(),
		newMockEngineDispatcher(),
		newMockLicenseValidator(),
		WithDataDir(filepath.Join(tmpDir, "data")),
		WithPluginDir(filepath.Join(tmpDir, "plugins")),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if aroute.Services() != container {
		t.Error("Services() should return the same container passed to New()")
	}
}

func TestAroute_EventsAccessor(t *testing.T) {
	ctx := context.Background()
	tmpDir, err := os.MkdirTemp("", "aroute-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	eventBus := newMockEventBus()
	aroute, err := New(ctx,
		newMockServiceContainer(),
		eventBus,
		newMockPluginRegistry(),
		newMockLifecycleManager(),
		newMockEngineDispatcher(),
		newMockLicenseValidator(),
		WithDataDir(filepath.Join(tmpDir, "data")),
		WithPluginDir(filepath.Join(tmpDir, "plugins")),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if aroute.Events() != eventBus {
		t.Error("Events() should return the same event bus passed to New()")
	}
}

func TestAroute_RegistryAccessor(t *testing.T) {
	ctx := context.Background()
	tmpDir, err := os.MkdirTemp("", "aroute-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	registry := newMockPluginRegistry()
	aroute, err := New(ctx,
		newMockServiceContainer(),
		newMockEventBus(),
		registry,
		newMockLifecycleManager(),
		newMockEngineDispatcher(),
		newMockLicenseValidator(),
		WithDataDir(filepath.Join(tmpDir, "data")),
		WithPluginDir(filepath.Join(tmpDir, "plugins")),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if aroute.Registry() != registry {
		t.Error("Registry() should return the same registry passed to New()")
	}
}

func TestAroute_LifecycleAccessor(t *testing.T) {
	ctx := context.Background()
	tmpDir, err := os.MkdirTemp("", "aroute-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	lifecycle := newMockLifecycleManager()
	aroute, err := New(ctx,
		newMockServiceContainer(),
		newMockEventBus(),
		newMockPluginRegistry(),
		lifecycle,
		newMockEngineDispatcher(),
		newMockLicenseValidator(),
		WithDataDir(filepath.Join(tmpDir, "data")),
		WithPluginDir(filepath.Join(tmpDir, "plugins")),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if aroute.Lifecycle() != lifecycle {
		t.Error("Lifecycle() should return the same lifecycle manager passed to New()")
	}
}

func TestAroute_DispatcherAccessor(t *testing.T) {
	ctx := context.Background()
	tmpDir, err := os.MkdirTemp("", "aroute-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dispatcher := newMockEngineDispatcher()
	aroute, err := New(ctx,
		newMockServiceContainer(),
		newMockEventBus(),
		newMockPluginRegistry(),
		newMockLifecycleManager(),
		dispatcher,
		newMockLicenseValidator(),
		WithDataDir(filepath.Join(tmpDir, "data")),
		WithPluginDir(filepath.Join(tmpDir, "plugins")),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if aroute.Dispatcher() != dispatcher {
		t.Error("Dispatcher() should return the same dispatcher passed to New()")
	}
}

func TestAroute_LicenseAccessor(t *testing.T) {
	ctx := context.Background()
	tmpDir, err := os.MkdirTemp("", "aroute-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	license := newMockLicenseValidator()
	aroute, err := New(ctx,
		newMockServiceContainer(),
		newMockEventBus(),
		newMockPluginRegistry(),
		newMockLifecycleManager(),
		newMockEngineDispatcher(),
		license,
		WithDataDir(filepath.Join(tmpDir, "data")),
		WithPluginDir(filepath.Join(tmpDir, "plugins")),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if aroute.License() != license {
		t.Error("License() should return the same license validator passed to New()")
	}
}

func TestAroute_DDLAccessor_NilBeforeSet(t *testing.T) {
	ctx := context.Background()
	tmpDir, err := os.MkdirTemp("", "aroute-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	aroute, err := New(ctx,
		newMockServiceContainer(),
		newMockEventBus(),
		newMockPluginRegistry(),
		newMockLifecycleManager(),
		newMockEngineDispatcher(),
		newMockLicenseValidator(),
		WithDataDir(filepath.Join(tmpDir, "data")),
		WithPluginDir(filepath.Join(tmpDir, "plugins")),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if aroute.DDL() != nil {
		t.Error("DDL() should be nil before SetDDL is called")
	}
}

func TestAroute_SetDDL(t *testing.T) {
	ctx := context.Background()
	tmpDir, err := os.MkdirTemp("", "aroute-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	aroute, err := New(ctx,
		newMockServiceContainer(),
		newMockEventBus(),
		newMockPluginRegistry(),
		newMockLifecycleManager(),
		newMockEngineDispatcher(),
		newMockLicenseValidator(),
		WithDataDir(filepath.Join(tmpDir, "data")),
		WithPluginDir(filepath.Join(tmpDir, "plugins")),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ddl := newMockDDLRegistry()
	aroute.SetDDL(ddl)

	if aroute.DDL() != ddl {
		t.Error("DDL() should return the DDL registry set via SetDDL()")
	}
}

func TestAroute_SetDDL_ThreadSafe(t *testing.T) {
	ctx := context.Background()
	tmpDir, err := os.MkdirTemp("", "aroute-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	aroute, err := New(ctx,
		newMockServiceContainer(),
		newMockEventBus(),
		newMockPluginRegistry(),
		newMockLifecycleManager(),
		newMockEngineDispatcher(),
		newMockLicenseValidator(),
		WithDataDir(filepath.Join(tmpDir, "data")),
		WithPluginDir(filepath.Join(tmpDir, "plugins")),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	// Concurrent SetDDL calls
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			ddl := newMockDDLRegistry()
			aroute.SetDDL(ddl)
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("SetDDL goroutine timeout")
		}
	}

	// DDL should be set (one of them)
	if aroute.DDL() == nil {
		t.Error("DDL() should not be nil after concurrent SetDDL")
	}
}

func TestAroute_ContextAccessor(t *testing.T) {
	ctx := context.Background()
	tmpDir, err := os.MkdirTemp("", "aroute-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	aroute, err := New(ctx,
		newMockServiceContainer(),
		newMockEventBus(),
		newMockPluginRegistry(),
		newMockLifecycleManager(),
		newMockEngineDispatcher(),
		newMockLicenseValidator(),
		WithDataDir(filepath.Join(tmpDir, "data")),
		WithPluginDir(filepath.Join(tmpDir, "plugins")),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	engineCtx := aroute.Context()
	if engineCtx == nil {
		t.Fatal("Context() should not return nil")
	}

	// Context should be valid initially
	if engineCtx.Err() != nil {
		t.Error("Context should not have error initially")
	}
}

func TestAroute_ContextCancelledOnStop(t *testing.T) {
	ctx := context.Background()
	tmpDir, err := os.MkdirTemp("", "aroute-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	aroute, err := New(ctx,
		newMockServiceContainer(),
		newMockEventBus(),
		newMockPluginRegistry(),
		newMockLifecycleManager(),
		newMockEngineDispatcher(),
		newMockLicenseValidator(),
		WithDataDir(filepath.Join(tmpDir, "data")),
		WithPluginDir(filepath.Join(tmpDir, "plugins")),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if err := aroute.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	engineCtx := aroute.Context()

	if err := aroute.Stop(ctx); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}

	// Context should be cancelled after Stop
	if engineCtx.Err() == nil {
		t.Error("Context should be cancelled after Stop")
	}
}

func TestAroute_DataDirAccessor(t *testing.T) {
	ctx := context.Background()
	tmpDir, err := os.MkdirTemp("", "aroute-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	expectedDataDir := filepath.Join(tmpDir, "expected-data")
	aroute, err := New(ctx,
		newMockServiceContainer(),
		newMockEventBus(),
		newMockPluginRegistry(),
		newMockLifecycleManager(),
		newMockEngineDispatcher(),
		newMockLicenseValidator(),
		WithDataDir(expectedDataDir),
		WithPluginDir(filepath.Join(tmpDir, "plugins")),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if aroute.DataDir() != expectedDataDir {
		t.Errorf("DataDir() = %q, want %q", aroute.DataDir(), expectedDataDir)
	}
}

func TestAroute_PluginDirAccessor(t *testing.T) {
	ctx := context.Background()
	tmpDir, err := os.MkdirTemp("", "aroute-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	expectedPluginDir := filepath.Join(tmpDir, "expected-plugins")
	aroute, err := New(ctx,
		newMockServiceContainer(),
		newMockEventBus(),
		newMockPluginRegistry(),
		newMockLifecycleManager(),
		newMockEngineDispatcher(),
		newMockLicenseValidator(),
		WithDataDir(filepath.Join(tmpDir, "data")),
		WithPluginDir(expectedPluginDir),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if aroute.PluginDir() != expectedPluginDir {
		t.Errorf("PluginDir() = %q, want %q", aroute.PluginDir(), expectedPluginDir)
	}
}

func TestAroute_IsStarted(t *testing.T) {
	ctx := context.Background()
	tmpDir, err := os.MkdirTemp("", "aroute-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	aroute, err := New(ctx,
		newMockServiceContainer(),
		newMockEventBus(),
		newMockPluginRegistry(),
		newMockLifecycleManager(),
		newMockEngineDispatcher(),
		newMockLicenseValidator(),
		WithDataDir(filepath.Join(tmpDir, "data")),
		WithPluginDir(filepath.Join(tmpDir, "plugins")),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	// Initially not started
	if aroute.IsStarted() {
		t.Error("IsStarted() should be false initially")
	}

	// After Start
	if err := aroute.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if !aroute.IsStarted() {
		t.Error("IsStarted() should be true after Start()")
	}

	// After Stop
	if err := aroute.Stop(ctx); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if aroute.IsStarted() {
		t.Error("IsStarted() should be false after Stop()")
	}
}

func TestAroute_IsStarted_ThreadSafe(t *testing.T) {
	ctx := context.Background()
	tmpDir, err := os.MkdirTemp("", "aroute-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	aroute, err := New(ctx,
		newMockServiceContainer(),
		newMockEventBus(),
		newMockPluginRegistry(),
		newMockLifecycleManager(),
		newMockEngineDispatcher(),
		newMockLicenseValidator(),
		WithDataDir(filepath.Join(tmpDir, "data")),
		WithPluginDir(filepath.Join(tmpDir, "plugins")),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if err := aroute.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Concurrent reads
	done := make(chan bool, 20)
	for i := 0; i < 20; i++ {
		go func() {
			started := aroute.IsStarted()
			_ = started // Just read, don't assert
			done <- true
		}()
	}

	for i := 0; i < 20; i++ {
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("IsStarted concurrent read timeout")
		}
	}
}

// ============================================
// RegisterPlugin Tests
// ============================================

func TestAroute_RegisterPlugin_NilManifest(t *testing.T) {
	ctx := context.Background()
	tmpDir, err := os.MkdirTemp("", "aroute-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	aroute, err := New(ctx,
		newMockServiceContainer(),
		newMockEventBus(),
		newMockPluginRegistry(),
		newMockLifecycleManager(),
		newMockEngineDispatcher(),
		newMockLicenseValidator(),
		WithDataDir(filepath.Join(tmpDir, "data")),
		WithPluginDir(filepath.Join(tmpDir, "plugins")),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	// Plugin with nil manifest
	plugin := &nilManifestPlugin{name: "test"}

	if err := aroute.RegisterPlugin(plugin); err == nil {
		t.Error("RegisterPlugin() should fail when manifest is nil")
	}
}

type nilManifestPlugin struct {
	name string
}

func (p *nilManifestPlugin) Name() string               { return p.name }
func (p *nilManifestPlugin) Version() string            { return "1.0.0" }
func (p *nilManifestPlugin) Manifest() *Manifest        { return nil }
func (p *nilManifestPlugin) Init(ctx CoreContext) error { return nil }
func (p *nilManifestPlugin) Start() error               { return nil }
func (p *nilManifestPlugin) Stop() error                { return nil }

func TestAroute_RegisterPlugin_RegistryError(t *testing.T) {
	ctx := context.Background()
	tmpDir, err := os.MkdirTemp("", "aroute-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Registry that returns error on Register
	registry := &errorRegisterRegistry{}
	aroute, err := New(ctx,
		newMockServiceContainer(),
		newMockEventBus(),
		registry,
		newMockLifecycleManager(),
		newMockEngineDispatcher(),
		newMockLicenseValidator(),
		WithDataDir(filepath.Join(tmpDir, "data")),
		WithPluginDir(filepath.Join(tmpDir, "plugins")),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	plugin := newMockPlugin("test", "1.0.0")
	if err := aroute.RegisterPlugin(plugin); err == nil {
		t.Error("RegisterPlugin() should fail when registry returns error")
	}
}

type errorRegisterRegistry struct {
	mockPluginRegistry
}

func (r *errorRegisterRegistry) Register(entry *PluginEntry) error {
	return errors.New("registry error")
}

// ============================================
// Builder Pattern Tests
// ============================================

func TestBuilder_FluentChaining(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "aroute-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	builder := NewBuilder()
	b1 := builder.WithDataDir(filepath.Join(tmpDir, "data"))
	if b1 != builder {
		t.Error("WithDataDir should return same builder")
	}

	b2 := b1.WithPluginDir(filepath.Join(tmpDir, "plugins"))
	if b2 != builder {
		t.Error("WithPluginDir should return same builder")
	}

	b3 := b2.WithLogger(logger)
	if b3 != builder {
		t.Error("WithLogger should return same builder")
	}

	b4 := b3.WithContainer(newMockServiceContainer())
	if b4 != builder {
		t.Error("WithContainer should return same builder")
	}
}

func TestBuilder_WithLicense(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "aroute-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	builder := NewBuilder()
	b := builder.WithLicense(filepath.Join(tmpDir, "license.json"), nil)
	if b != builder {
		t.Error("WithLicense should return same builder")
	}
}

func TestBuilder_WithRegistry(t *testing.T) {
	registry := newMockPluginRegistry()
	builder := NewBuilder()
	b := builder.WithRegistry(registry)
	if b != builder {
		t.Error("WithRegistry should return same builder")
	}
}

func TestBuilder_WithLifecycle(t *testing.T) {
	lifecycle := newMockLifecycleManager()
	builder := NewBuilder()
	b := builder.WithLifecycle(lifecycle)
	if b != builder {
		t.Error("WithLifecycle should return same builder")
	}
}

func TestBuilder_WithDispatcher(t *testing.T) {
	dispatcher := newMockEngineDispatcher()
	builder := NewBuilder()
	b := builder.WithDispatcher(dispatcher)
	if b != builder {
		t.Error("WithDispatcher should return same builder")
	}
}

func TestBuilder_Build_WithAllOverrides(t *testing.T) {
	ctx := context.Background()
	tmpDir, err := os.MkdirTemp("", "aroute-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	container := newMockServiceContainer()
	eventBus := newMockEventBus()
	registry := newMockPluginRegistry()
	lifecycle := newMockLifecycleManager()
	dispatcher := newMockEngineDispatcher()
	license := newMockLicenseValidator()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	aroute, err := NewBuilder().
		WithDataDir(filepath.Join(tmpDir, "data")).
		WithPluginDir(filepath.Join(tmpDir, "plugins")).
		WithLogger(logger).
		WithContainer(container).
		WithEventBus(eventBus).
		WithRegistry(registry).
		WithLifecycle(lifecycle).
		WithDispatcher(dispatcher).
		WithLicenseValidator(license).
		Build(ctx)

	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	// Verify all subsystems
	if aroute.Services() != container {
		t.Error("Services() mismatch")
	}
	if aroute.Events() != eventBus {
		t.Error("Events() mismatch")
	}
	if aroute.Registry() != registry {
		t.Error("Registry() mismatch")
	}
	if aroute.Lifecycle() != lifecycle {
		t.Error("Lifecycle() mismatch")
	}
	if aroute.Dispatcher() != dispatcher {
		t.Error("Dispatcher() mismatch")
	}
	if aroute.License() != license {
		t.Error("License() mismatch")
	}
}

func TestBuilder_BuildWithFactories(t *testing.T) {
	ctx := context.Background()
	tmpDir, err := os.MkdirTemp("", "aroute-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	container := newMockServiceContainer()
	eventBus := newMockEventBus()
	registry := newMockPluginRegistry()
	lifecycle := newMockLifecycleManager()
	dispatcher := newMockEngineDispatcher()
	license := newMockLicenseValidator()

	factories := &Factories{
		NewContainer:  func() (ServiceContainer, error) { return container, nil },
		NewEventBus:   func() (EventBus, error) { return eventBus, nil },
		NewRegistry:   func(dataDir string) (PluginRegistry, error) { return registry, nil },
		NewDispatcher: func() (EngineDispatcher, error) { return dispatcher, nil },
		NewLicense:    func(licensePath string, pubKey *ecdsa.PublicKey) (LicenseValidator, error) { return license, nil },
		NewLifecycle: func(registry PluginRegistry, container ServiceContainer, eventBus EventBus, logger *slog.Logger, dataDir, pluginDir string) (LifecycleManager, error) {
			return lifecycle, nil
		},
	}

	aroute, err := NewBuilder().
		WithDataDir(filepath.Join(tmpDir, "data")).
		WithPluginDir(filepath.Join(tmpDir, "plugins")).
		BuildWithFactories(ctx, factories)

	if err != nil {
		t.Fatalf("BuildWithFactories() error = %v", err)
	}

	if aroute == nil {
		t.Fatal("BuildWithFactories() returned nil")
	}
}

func TestBuilder_BuildWithFactories_OverridesTakePrecedence(t *testing.T) {
	ctx := context.Background()
	tmpDir, err := os.MkdirTemp("", "aroute-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	overrideContainer := newMockServiceContainer()
	overrideEventBus := newMockEventBus()
	overrideRegistry := newMockPluginRegistry()
	overrideLifecycle := newMockLifecycleManager()
	overrideDispatcher := newMockEngineDispatcher()
	overrideLicense := newMockLicenseValidator()

	factories := &Factories{
		NewContainer: func() (ServiceContainer, error) { return newMockServiceContainer(), nil },
	}

	aroute, err := NewBuilder().
		WithDataDir(filepath.Join(tmpDir, "data")).
		WithPluginDir(filepath.Join(tmpDir, "plugins")).
		WithContainer(overrideContainer).
		WithEventBus(overrideEventBus).
		WithRegistry(overrideRegistry).
		WithLifecycle(overrideLifecycle).
		WithDispatcher(overrideDispatcher).
		WithLicenseValidator(overrideLicense).
		BuildWithFactories(ctx, factories)

	if err != nil {
		t.Fatalf("BuildWithFactories() error = %v", err)
	}

	if aroute.Services() != overrideContainer {
		t.Error("Override container should be used, not factory")
	}
}

func TestBuilder_BuildWithFactories_FactoryError(t *testing.T) {
	ctx := context.Background()
	tmpDir, err := os.MkdirTemp("", "aroute-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	factories := &Factories{
		NewContainer: func() (ServiceContainer, error) { return nil, errors.New("container factory error") },
	}

	aroute, err := NewBuilder().
		WithDataDir(filepath.Join(tmpDir, "data")).
		WithPluginDir(filepath.Join(tmpDir, "plugins")).
		BuildWithFactories(ctx, factories)

	if err == nil {
		t.Error("BuildWithFactories() should fail when factory returns error")
		if aroute != nil {
			aroute.Stop(ctx)
		}
	}
}

func TestBuilder_BuildWithFactories_DataDirError(t *testing.T) {
	ctx := context.Background()

	factories := &Factories{}

	aroute, err := NewBuilder().
		WithDataDir("/proc/nonexistent/aroute/data").
		WithPluginDir("/tmp/aroute/plugins").
		BuildWithFactories(ctx, factories)

	if err == nil {
		t.Error("BuildWithFactories() should fail when data directory cannot be created")
		if aroute != nil {
			aroute.Stop(ctx)
		}
	}
}

// ============================================
// ScopedConfig Tests
// ============================================

func TestScopedConfig_GetString(t *testing.T) {
	base := map[string]interface{}{
		"key1": "value1",
		"key2": 123,
		"key3": nil,
	}
	config := NewScopedConfig("test", base)

	if config.GetString("key1") != "value1" {
		t.Error("GetString should return string value")
	}
	if config.GetString("key2") != "" {
		t.Error("GetString should return empty for non-string")
	}
	if config.GetString("key3") != "" {
		t.Error("GetString should return empty for nil")
	}
	if config.GetString("nonexistent") != "" {
		t.Error("GetString should return empty for missing key")
	}
}

func TestScopedConfig_GetInt(t *testing.T) {
	base := map[string]interface{}{
		"int_key":    42,
		"int64_key":  int64(100),
		"float_key":  3.14,
		"string_key": "not an int",
	}
	config := NewScopedConfig("test", base)

	if config.GetInt("int_key") != 42 {
		t.Errorf("GetInt int_key = %d, want 42", config.GetInt("int_key"))
	}
	if config.GetInt("int64_key") != 100 {
		t.Errorf("GetInt int64_key = %d, want 100", config.GetInt("int64_key"))
	}
	if config.GetInt("float_key") != 3 {
		t.Errorf("GetInt float_key = %d, want 3", config.GetInt("float_key"))
	}
	if config.GetInt("string_key") != 0 {
		t.Error("GetInt should return 0 for string")
	}
	if config.GetInt("nonexistent") != 0 {
		t.Error("GetInt should return 0 for missing key")
	}
}

func TestScopedConfig_GetBool(t *testing.T) {
	base := map[string]interface{}{
		"true_key":   true,
		"false_key":  false,
		"string_key": "not bool",
	}
	config := NewScopedConfig("test", base)

	if !config.GetBool("true_key") {
		t.Error("GetBool true_key should be true")
	}
	if config.GetBool("false_key") {
		t.Error("GetBool false_key should be false")
	}
	if config.GetBool("string_key") {
		t.Error("GetBool should return false for non-bool")
	}
	if config.GetBool("nonexistent") {
		t.Error("GetBool should return false for missing key")
	}
}

func TestScopedConfig_GetStringSlice(t *testing.T) {
	base := map[string]interface{}{
		"slice_key":   []string{"a", "b", "c"},
		"non_slice":   "not a slice",
		"empty_slice": []string{},
	}
	config := NewScopedConfig("test", base)

	slice := config.GetStringSlice("slice_key")
	if len(slice) != 3 {
		t.Errorf("GetStringSlice slice_key len = %d, want 3", len(slice))
	}

	if config.GetStringSlice("non_slice") != nil {
		t.Error("GetStringSlice should return nil for non-slice")
	}

	if len(config.GetStringSlice("empty_slice")) != 0 {
		t.Error("GetStringSlice empty_slice should be empty")
	}

	if config.GetStringSlice("nonexistent") != nil {
		t.Error("GetStringSlice should return nil for missing key")
	}
}

func TestScopedConfig_Get(t *testing.T) {
	base := map[string]interface{}{
		"any_key": "any_value",
	}
	config := NewScopedConfig("test", base)

	if config.Get("any_key") != "any_value" {
		t.Error("Get should return raw value")
	}
	if config.Get("nonexistent") != nil {
		t.Error("Get should return nil for missing key")
	}
}

func TestScopedConfig_Unmarshal(t *testing.T) {
	base := map[string]interface{}{}
	config := NewScopedConfig("test", base)

	err := config.Unmarshal("key", nil)
	if err == nil {
		t.Error("Unmarshal should return error (not implemented)")
	}
}

// ============================================
// LicenseTier Tests
// ============================================

func TestLicenseTier_String(t *testing.T) {
	tests := []struct {
		tier LicenseTier
		want string
	}{
		{LicenseTierOpen, "open"},
		{LicenseTierPro, "pro"},
		{LicenseTierEnterprise, "enterprise"},
		{LicenseTier(99), "unknown"},
	}

	for _, tt := range tests {
		if got := tt.tier.String(); got != tt.want {
			t.Errorf("LicenseTier(%d).String() = %q, want %q", tt.tier, got, tt.want)
		}
	}
}

func TestLicenseValidator_TierMethods(t *testing.T) {
	validator := &mockLicenseValidator{
		tier:     LicenseTierPro,
		features: map[string]bool{"feature1": true, "feature2": false},
		expired:  false,
	}

	if validator.Tier() != LicenseTierPro {
		t.Error("Tier() should return Pro")
	}
	if !validator.IsFeatureAllowed("feature1") {
		t.Error("IsFeatureAllowed(feature1) should be true")
	}
	if validator.IsFeatureAllowed("feature2") {
		t.Error("IsFeatureAllowed(feature2) should be false")
	}
	if validator.IsFeatureAllowed("nonexistent") {
		t.Error("IsFeatureAllowed(nonexistent) should be false")
	}
	if validator.IsExpired() {
		t.Error("IsExpired() should be false")
	}

	info := validator.LicenseInfo()
	if info.Tier != LicenseTierPro {
		t.Error("LicenseInfo.Tier should be Pro")
	}
}

func TestLicenseValidator_Expired(t *testing.T) {
	validator := &mockLicenseValidator{
		tier:    LicenseTierEnterprise,
		expired: true,
	}

	if !validator.IsExpired() {
		t.Error("IsExpired() should be true")
	}
}

// ============================================
// DDL Tests with Start
// ============================================

func TestStart_WithDDL_Success(t *testing.T) {
	ctx := context.Background()
	tmpDir, err := os.MkdirTemp("", "aroute-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	aroute, err := New(ctx,
		newMockServiceContainer(),
		newMockEventBus(),
		newMockPluginRegistry(),
		newMockLifecycleManager(),
		newMockEngineDispatcher(),
		newMockLicenseValidator(),
		WithDataDir(filepath.Join(tmpDir, "data")),
		WithPluginDir(filepath.Join(tmpDir, "plugins")),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ddl := newMockDDLRegistry()
	aroute.SetDDL(ddl)

	if err := aroute.Start(ctx); err != nil {
		t.Fatalf("Start() with DDL error = %v", err)
	}

	if !aroute.IsStarted() {
		t.Error("Should be started after successful Start")
	}

	aroute.Stop(ctx)
}

func TestStart_WithoutDDL_Success(t *testing.T) {
	ctx := context.Background()
	tmpDir, err := os.MkdirTemp("", "aroute-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	aroute, err := New(ctx,
		newMockServiceContainer(),
		newMockEventBus(),
		newMockPluginRegistry(),
		newMockLifecycleManager(),
		newMockEngineDispatcher(),
		newMockLicenseValidator(),
		WithDataDir(filepath.Join(tmpDir, "data")),
		WithPluginDir(filepath.Join(tmpDir, "plugins")),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	// Don't set DDL - should still work
	if err := aroute.Start(ctx); err != nil {
		t.Fatalf("Start() without DDL error = %v", err)
	}

	aroute.Stop(ctx)
}

// ============================================
// Helper Functions
// ============================================

func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstringHelper(s, substr))
}

func containsSubstringHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestLoadLicenseFile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "aroute-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	licenseContent := map[string]interface{}{
		"tier":     "pro",
		"expires":  "2025-12-31",
		"features": []string{"feature1", "feature2"},
	}

	licenseFile := filepath.Join(tmpDir, "license.json")
	data, err := json.Marshal(licenseContent)
	if err != nil {
		t.Fatalf("marshal license: %v", err)
	}
	if err := os.WriteFile(licenseFile, data, 0644); err != nil {
		t.Fatalf("write license file: %v", err)
	}

	lic, err := LoadLicenseFile(licenseFile)
	if err != nil {
		t.Fatalf("LoadLicenseFile() error = %v", err)
	}

	if lic["tier"] != "pro" {
		t.Errorf("tier = %v, want pro", lic["tier"])
	}
}

func TestLoadLicenseFile_FileNotFound(t *testing.T) {
	_, err := LoadLicenseFile("/nonexistent/license.json")
	if err == nil {
		t.Error("LoadLicenseFile() should fail for nonexistent file")
	}
}

func TestLoadLicenseFile_InvalidJSON(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "aroute-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	invalidFile := filepath.Join(tmpDir, "invalid.json")
	if err := os.WriteFile(invalidFile, []byte("not valid json {"), 0644); err != nil {
		t.Fatalf("write invalid file: %v", err)
	}

	_, err = LoadLicenseFile(invalidFile)
	if err == nil {
		t.Error("LoadLicenseFile() should fail for invalid JSON")
	}
}

// ============================================
// DefaultCoreContextFactory Tests
// ============================================

func TestDefaultCoreContextFactory(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	container := newMockServiceContainer()
	eventBus := newMockEventBus()

	coreCtx := DefaultCoreContextFactory(ctx, "test-plugin", logger, container, eventBus, "/data", "/plugins")

	if coreCtx == nil {
		t.Fatal("DefaultCoreContextFactory returned nil")
	}

	// Verify all accessors return non-nil (basic check)
	if coreCtx.Services() != container {
		t.Error("Services() should return provided container")
	}
	if coreCtx.Events() != eventBus {
		t.Error("Events() should return provided eventBus")
	}
	if coreCtx.Config() == nil {
		t.Error("Config() should not be nil")
	}
	if coreCtx.DataDir() == "" {
		t.Error("DataDir() should not be empty")
	}
	if coreCtx.PluginDir() == "" {
		t.Error("PluginDir() should not be empty")
	}
}

// ============================================
// ViperConfig Tests
// ============================================

func TestNewViperConfig(t *testing.T) {
	v := viper.New()
	config := NewViperConfig(v)
	if config == nil {
		t.Fatal("NewViperConfig() returned nil")
	}
}

func TestViperConfig_GetString(t *testing.T) {
	v := viper.New()
	v.Set("key1", "value1")
	v.Set("key2", 123)
	config := NewViperConfig(v)

	if config.GetString("key1") != "value1" {
		t.Errorf("GetString(key1) = %q, want %q", config.GetString("key1"), "value1")
	}
	if config.GetString("key2") != "123" {
		t.Errorf("GetString(key2) = %q, want %q", config.GetString("key2"), "123")
	}
	if config.GetString("nonexistent") != "" {
		t.Error("GetString(nonexistent) should return empty string")
	}
}

func TestViperConfig_GetInt(t *testing.T) {
	v := viper.New()
	v.Set("port", 8080)
	v.Set("invalid", "not-a-number")
	config := NewViperConfig(v)

	if config.GetInt("port") != 8080 {
		t.Errorf("GetInt(port) = %d, want 8080", config.GetInt("port"))
	}
	if config.GetInt("nonexistent") != 0 {
		t.Error("GetInt(nonexistent) should return 0")
	}
}

func TestViperConfig_GetBool(t *testing.T) {
	v := viper.New()
	v.Set("enabled", true)
	v.Set("disabled", false)
	config := NewViperConfig(v)

	if !config.GetBool("enabled") {
		t.Error("GetBool(enabled) should be true")
	}
	if config.GetBool("disabled") {
		t.Error("GetBool(disabled) should be false")
	}
	if config.GetBool("nonexistent") {
		t.Error("GetBool(nonexistent) should be false")
	}
}

func TestViperConfig_GetStringSlice(t *testing.T) {
	v := viper.New()
	v.Set("tags", []string{"a", "b", "c"})
	config := NewViperConfig(v)

	slice := config.GetStringSlice("tags")
	if len(slice) != 3 || slice[0] != "a" || slice[1] != "b" || slice[2] != "c" {
		t.Errorf("GetStringSlice(tags) = %v, want [a b c]", slice)
	}
	if config.GetStringSlice("nonexistent") != nil {
		t.Error("GetStringSlice(nonexistent) should return nil/empty")
	}
}

func TestViperConfig_Get(t *testing.T) {
	v := viper.New()
	v.Set("raw_key", "raw_value")
	config := NewViperConfig(v)

	if config.Get("raw_key") != "raw_value" {
		t.Errorf("Get(raw_key) = %v, want raw_value", config.Get("raw_key"))
	}
	if config.Get("nonexistent") != nil {
		t.Error("Get(nonexistent) should return nil")
	}
}

func TestViperConfig_Unmarshal(t *testing.T) {
	v := viper.New()
	v.Set("server.host", "localhost")
	v.Set("server.port", 3000)
	config := NewViperConfig(v)

	var target struct {
		Host string
		Port int
	}
	err := config.Unmarshal("server", &target)
	if err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if target.Host != "localhost" {
		t.Errorf("target.Host = %q, want %q", target.Host, "localhost")
	}
	if target.Port != 3000 {
		t.Errorf("target.Port = %d, want 3000", target.Port)
	}
}

// ============================================
// BuildWithFactories Additional Error Path Tests
// ============================================

func TestBuilder_BuildWithFactories_EventBusFactoryError(t *testing.T) {
	ctx := context.Background()
	tmpDir, err := os.MkdirTemp("", "aroute-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	factories := &Factories{
		NewEventBus: func() (EventBus, error) { return nil, errors.New("event bus factory error") },
	}

	aroute, err := NewBuilder().
		WithDataDir(filepath.Join(tmpDir, "data")).
		WithPluginDir(filepath.Join(tmpDir, "plugins")).
		BuildWithFactories(ctx, factories)

	if err == nil {
		t.Error("BuildWithFactories() should fail when event bus factory returns error")
		if aroute != nil {
			aroute.Stop(ctx)
		}
	}
}

func TestBuilder_BuildWithFactories_RegistryFactoryError(t *testing.T) {
	ctx := context.Background()
	tmpDir, err := os.MkdirTemp("", "aroute-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	factories := &Factories{
		NewRegistry: func(dataDir string) (PluginRegistry, error) { return nil, errors.New("registry factory error") },
	}

	aroute, err := NewBuilder().
		WithDataDir(filepath.Join(tmpDir, "data")).
		WithPluginDir(filepath.Join(tmpDir, "plugins")).
		BuildWithFactories(ctx, factories)

	if err == nil {
		t.Error("BuildWithFactories() should fail when registry factory returns error")
		if aroute != nil {
			aroute.Stop(ctx)
		}
	}
}

func TestBuilder_BuildWithFactories_DispatcherFactoryError(t *testing.T) {
	ctx := context.Background()
	tmpDir, err := os.MkdirTemp("", "aroute-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	factories := &Factories{
		NewDispatcher: func() (EngineDispatcher, error) { return nil, errors.New("dispatcher factory error") },
	}

	aroute, err := NewBuilder().
		WithDataDir(filepath.Join(tmpDir, "data")).
		WithPluginDir(filepath.Join(tmpDir, "plugins")).
		BuildWithFactories(ctx, factories)

	if err == nil {
		t.Error("BuildWithFactories() should fail when dispatcher factory returns error")
		if aroute != nil {
			aroute.Stop(ctx)
		}
	}
}

func TestBuilder_BuildWithFactories_LicenseFactoryError(t *testing.T) {
	ctx := context.Background()
	tmpDir, err := os.MkdirTemp("", "aroute-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	factories := &Factories{
		NewLicense: func(licensePath string, pubKey *ecdsa.PublicKey) (LicenseValidator, error) {
			return nil, errors.New("license factory error")
		},
	}

	aroute, err := NewBuilder().
		WithDataDir(filepath.Join(tmpDir, "data")).
		WithPluginDir(filepath.Join(tmpDir, "plugins")).
		BuildWithFactories(ctx, factories)

	if err == nil {
		t.Error("BuildWithFactories() should fail when license factory returns error")
		if aroute != nil {
			aroute.Stop(ctx)
		}
	}
}

func TestBuilder_BuildWithFactories_LifecycleFactoryError(t *testing.T) {
	ctx := context.Background()
	tmpDir, err := os.MkdirTemp("", "aroute-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	factories := &Factories{
		NewLifecycle: func(registry PluginRegistry, container ServiceContainer, eventBus EventBus, logger *slog.Logger, dataDir, pluginDir string) (LifecycleManager, error) {
			return nil, errors.New("lifecycle factory error")
		},
	}

	aroute, err := NewBuilder().
		WithDataDir(filepath.Join(tmpDir, "data")).
		WithPluginDir(filepath.Join(tmpDir, "plugins")).
		BuildWithFactories(ctx, factories)

	if err == nil {
		t.Error("BuildWithFactories() should fail when lifecycle factory returns error")
		if aroute != nil {
			aroute.Stop(ctx)
		}
	}
}

func TestBuilder_BuildWithFactories_PluginDirError(t *testing.T) {
	ctx := context.Background()

	factories := &Factories{}

	aroute, err := NewBuilder().
		WithDataDir("/tmp/aroute-test-data").
		WithPluginDir("/proc/nonexistent/aroute/plugins").
		BuildWithFactories(ctx, factories)

	if err == nil {
		t.Error("BuildWithFactories() should fail when plugin directory cannot be created")
		if aroute != nil {
			aroute.Stop(ctx)
		}
	}
}
