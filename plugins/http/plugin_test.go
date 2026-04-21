package http

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/wangling-miao/aroute/core"
	"github.com/wangling-miao/aroute/core/services"
	"github.com/wangling-miao/aroute/sdk/interfaces"
)

// mockCoreContext implements core.CoreContext for testing
type mockCoreContext struct {
	services  core.ServiceContainer
	events    core.EventBus
	config    core.ConfigProvider
	logger    *slog.Logger
	dataDir   string
	pluginDir string
	ctx       context.Context
}

func newMockCoreContext() *mockCoreContext {
	container := services.NewContainer()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	return &mockCoreContext{
		services:  container,
		events:    nil, // Not needed for HTTP plugin tests
		config:    &mockConfigProvider{},
		logger:    logger,
		dataDir:   os.TempDir(),
		pluginDir: os.TempDir(),
		ctx:       context.Background(),
	}
}

func (m *mockCoreContext) Services() core.ServiceContainer { return m.services }
func (m *mockCoreContext) Events() core.EventBus           { return m.events }
func (m *mockCoreContext) Config() core.ConfigProvider     { return m.config }
func (m *mockCoreContext) Logger() *slog.Logger            { return m.logger }
func (m *mockCoreContext) DataDir() string                 { return m.dataDir }
func (m *mockCoreContext) PluginDir() string               { return m.pluginDir }
func (m *mockCoreContext) Context() context.Context        { return m.ctx }

// mockConfigProvider implements core.ConfigProvider for testing
type mockConfigProvider struct {
	data map[string]interface{}
}

func (m *mockConfigProvider) GetString(key string) string {
	if v, ok := m.data[key]; ok {
		return v.(string)
	}
	return ""
}

func (m *mockConfigProvider) GetInt(key string) int {
	if v, ok := m.data[key]; ok {
		return v.(int)
	}
	return 0
}

func (m *mockConfigProvider) GetBool(key string) bool {
	if v, ok := m.data[key]; ok {
		return v.(bool)
	}
	return false
}

func (m *mockConfigProvider) GetStringSlice(key string) []string {
	if v, ok := m.data[key]; ok {
		return v.([]string)
	}
	return nil
}

func (m *mockConfigProvider) Get(key string) interface{} {
	return m.data[key]
}

func (m *mockConfigProvider) Unmarshal(key string, target interface{}) error {
	return nil
}

// TestPluginInitialization tests basic plugin initialization
func TestPluginInitialization(t *testing.T) {
	plugin := New()

	if plugin.Name() != "http" {
		t.Errorf("Expected plugin name 'http', got '%s'", plugin.Name())
	}

	if plugin.Version() != "1.0.0" {
		t.Errorf("Expected plugin version '1.0.0', got '%s'", plugin.Version())
	}

	manifest := plugin.Manifest()
	if manifest.Name != "http" {
		t.Errorf("Expected manifest name 'http', got '%s'", manifest.Name)
	}

	if manifest.Engine != "native" {
		t.Errorf("Expected engine 'native', got '%s'", manifest.Engine)
	}
}

// TestPluginInit tests plugin initialization with CoreContext
func TestPluginInit(t *testing.T) {
	plugin := New()
	ctx := newMockCoreContext()

	err := plugin.Init(ctx)
	if err != nil {
		t.Fatalf("Plugin initialization failed: %v", err)
	}

	// Verify router was created
	if plugin.router == nil {
		t.Error("Router was not initialized")
	}

	// Verify RouteRegistrar service was registered
	var registrar interfaces.RouteRegistrar
	err = ctx.Services().Get(&registrar)
	if err != nil {
		t.Error("RouteRegistrar service was not registered")
	}
}

// TestHealthEndpoint tests the /healthz endpoint
func TestHealthEndpoint(t *testing.T) {
	plugin := New()
	ctx := newMockCoreContext()

	err := plugin.Init(ctx)
	if err != nil {
		t.Fatalf("Plugin initialization failed: %v", err)
	}

	// Create test request
	req := httptest.NewRequest("GET", "/healthz", nil)
	w := httptest.NewRecorder()

	// Serve request
	plugin.router.ServeHTTP(w, req)

	// Check response
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	// Verify response body
	var status HealthStatus
	err = json.NewDecoder(w.Body).Decode(&status)
	if err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if status.Status != "healthy" {
		t.Errorf("Expected status 'healthy', got '%s'", status.Status)
	}

	if status.Version != "1.0.0" {
		t.Errorf("Expected version '1.0.0', got '%s'", status.Version)
	}

	// Verify timestamp is valid
	_, err = time.Parse(time.RFC3339, status.Timestamp)
	if err != nil {
		t.Errorf("Invalid timestamp format: %v", err)
	}
}

// TestRouteRegistration tests route registration via RouteRegistrar
func TestRouteRegistration(t *testing.T) {
	plugin := New()
	ctx := newMockCoreContext()

	err := plugin.Init(ctx)
	if err != nil {
		t.Fatalf("Plugin initialization failed: %v", err)
	}

	// Get RouteRegistrar service
	var registrar interfaces.RouteRegistrar
	err = ctx.Services().Get(&registrar)
	if err != nil {
		t.Fatalf("Failed to get RouteRegistrar: %v", err)
	}

	// Register a test route
	registrar.Register("GET /test", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("test response"))
	})

	// Test the registered route
	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	plugin.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	if w.Body.String() != "test response" {
		t.Errorf("Expected body 'test response', got '%s'", w.Body.String())
	}
}

// TestMiddlewareChain tests that middleware is properly applied
func TestMiddlewareChain(t *testing.T) {
	plugin := New()
	ctx := newMockCoreContext()

	err := plugin.Init(ctx)
	if err != nil {
		t.Fatalf("Plugin initialization failed: %v", err)
	}

	registrar := NewRouteRegistrar(plugin.router)
	registrar.Register("GET /test-middleware", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("middleware applied"))
	})

	req := httptest.NewRequest("GET", "/test-middleware", nil)
	w := httptest.NewRecorder()

	plugin.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

// TestCORS tests CORS configuration
func TestCORS(t *testing.T) {
	config := &mockConfigProvider{
		data: map[string]interface{}{
			"http.cors.allowed_origins": []string{"http://example.com"},
			"http.cors.allowed_methods": []string{"GET", "POST"},
			"http.cors.allowed_headers": []string{"Content-Type"},
		},
	}

	ctx := newMockCoreContext()
	ctx.config = config

	plugin := New()
	err := plugin.Init(ctx)
	if err != nil {
		t.Fatalf("Plugin initialization failed: %v", err)
	}

	// Test CORS preflight request
	req := httptest.NewRequest("OPTIONS", "/healthz", nil)
	req.Header.Set("Origin", "http://example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")

	w := httptest.NewRecorder()
	plugin.router.ServeHTTP(w, req)

	// Check CORS headers
	if w.Header().Get("Access-Control-Allow-Origin") != "http://example.com" {
		t.Error("CORS origin header not set correctly")
	}

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200 for OPTIONS, got %d", w.Code)
	}
}

// TestStaticFileServing tests static file serving
func TestStaticFileServing(t *testing.T) {
	// Create temporary static directory
	staticDir := filepath.Join(os.TempDir(), "aroute-static-test")
	os.MkdirAll(staticDir, 0755)
	defer os.RemoveAll(staticDir)

	// Create test file
	testFile := filepath.Join(staticDir, "test.txt")
	os.WriteFile(testFile, []byte("static content"), 0644)

	config := &mockConfigProvider{
		data: map[string]interface{}{
			"http.static_dir": staticDir,
		},
	}

	ctx := newMockCoreContext()
	ctx.config = config
	ctx.dataDir = os.TempDir()

	plugin := New()
	err := plugin.Init(ctx)
	if err != nil {
		t.Fatalf("Plugin initialization failed: %v", err)
	}

	// Test static file request
	req := httptest.NewRequest("GET", "/static/test.txt", nil)
	w := httptest.NewRecorder()

	plugin.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200 for static file, got %d", w.Code)
	}

	if w.Body.String() != "static content" {
		t.Errorf("Expected body 'static content', got '%s'", w.Body.String())
	}
}

// TestRouteGroup tests route grouping with middleware
func TestRouteGroup(t *testing.T) {
	plugin := New()
	ctx := newMockCoreContext()

	err := plugin.Init(ctx)
	if err != nil {
		t.Fatalf("Plugin initialization failed: %v", err)
	}

	registrar := NewRouteRegistrar(plugin.router)

	// Create route group with custom middleware
	registrar.Group(func(r chi.Router) {
		r.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("X-Custom-Middleware", "applied")
				next.ServeHTTP(w, r)
			})
		})

		r.Get("/group-test", func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("group response"))
		})
	})

	// Test group route
	req := httptest.NewRequest("GET", "/group-test", nil)
	w := httptest.NewRecorder()

	plugin.router.ServeHTTP(w, req)

	if w.Header().Get("X-Custom-Middleware") != "applied" {
		t.Error("Group middleware was not applied")
	}

	if w.Body.String() != "group response" {
		t.Errorf("Expected body 'group response', got '%s'", w.Body.String())
	}
}

// TestRegisterUnsupportedHandler tests Register with an unsupported handler type
func TestRegisterUnsupportedHandler(t *testing.T) {
	r := chi.NewRouter()
	registrar := NewRouteRegistrar(r)

	// Pass an unsupported handler type (int)
	registrar.Register("GET /bad", 42)

	// Route should not be registered; requesting it should 404
	req := httptest.NewRequest("GET", "/bad", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected 404 for unregistered route, got %d", w.Code)
	}
}

// TestRouteRegistrarUse tests the Use method for collecting middleware
func TestRouteRegistrarUse(t *testing.T) {
	r := chi.NewRouter()
	registrar := NewRouteRegistrar(r)

	mw1 := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-MW1", "yes")
			next.ServeHTTP(w, r)
		})
	}
	mw2 := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-MW2", "yes")
			next.ServeHTTP(w, r)
		})
	}

	registrar.Use(mw1, mw2)

	mws := registrar.Middlewares()
	if len(mws) != 2 {
		t.Fatalf("Expected 2 middlewares, got %d", len(mws))
	}
}

// TestRouteRegistrarRoute tests the Route method
func TestRouteRegistrarRoute(t *testing.T) {
	r := chi.NewRouter()
	registrar := NewRouteRegistrar(r)

	registrar.Route("/api", func(sub chi.Router) {
		sub.Get("/hello", func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("hello from route"))
		})
	})

	req := httptest.NewRequest("GET", "/api/hello", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}
	if w.Body.String() != "hello from route" {
		t.Errorf("Expected 'hello from route', got '%s'", w.Body.String())
	}
}

// TestStartWithCustomAddr tests Start with custom http.addr config
func TestStartWithCustomAddr(t *testing.T) {
	config := &mockConfigProvider{
		data: map[string]interface{}{
			"http.addr": "127.0.0.1:0",
		},
	}

	ctx := newMockCoreContext()
	ctx.config = config

	plugin := New()
	err := plugin.Init(ctx)
	if err != nil {
		t.Fatalf("Plugin initialization failed: %v", err)
	}

	err = plugin.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Give server time to start
	time.Sleep(50 * time.Millisecond)

	// Stop server
	err = plugin.Stop()
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
}

// TestStartWhenAlreadyRunning tests Start is idempotent
func TestStartWhenAlreadyRunning(t *testing.T) {
	config := &mockConfigProvider{
		data: map[string]interface{}{
			"http.addr": "127.0.0.1:0",
		},
	}

	ctx := newMockCoreContext()
	ctx.config = config

	plugin := New()
	err := plugin.Init(ctx)
	if err != nil {
		t.Fatalf("Plugin initialization failed: %v", err)
	}

	err = plugin.Start()
	if err != nil {
		t.Fatalf("First start failed: %v", err)
	}
	time.Sleep(50 * time.Millisecond)

	// Second start should be a no-op
	err = plugin.Start()
	if err != nil {
		t.Fatalf("Second start failed: %v", err)
	}

	err = plugin.Stop()
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
}

// TestStopWhenNotRunning tests Stop when server hasn't started
func TestStopWhenNotRunning(t *testing.T) {
	plugin := New()
	ctx := newMockCoreContext()
	err := plugin.Init(ctx)
	if err != nil {
		t.Fatalf("Plugin initialization failed: %v", err)
	}

	// Stop without starting should be fine
	err = plugin.Stop()
	if err != nil {
		t.Errorf("Stop on non-running server should not error, got: %v", err)
	}
}

// TestStopWithNilServer tests Stop when server is nil
func TestStopWithNilServer(t *testing.T) {
	plugin := New()
	plugin.running = true
	plugin.ctx = newMockCoreContext()

	// Server is nil, should return nil
	err := plugin.Stop()
	if err != nil {
		t.Errorf("Expected nil error, got: %v", err)
	}
}

// TestGetCollectedMiddlewaresNilCtx tests getCollectedMiddlewares with nil ctx
func TestGetCollectedMiddlewaresNilCtx(t *testing.T) {
	plugin := New()
	// ctx is nil
	mws := plugin.getCollectedMiddlewares()
	if mws != nil {
		t.Errorf("Expected nil, got %v", mws)
	}
}

// TestGetCollectedMiddlewaresNoRegistrar tests getCollectedMiddlewares when no registrar in container
func TestGetCollectedMiddlewaresNoRegistrar(t *testing.T) {
	plugin := New()
	ctx := newMockCoreContext()
	// Don't init (so no registrar in container)
	plugin.ctx = ctx
	mws := plugin.getCollectedMiddlewares()
	if mws != nil {
		t.Errorf("Expected nil when no registrar, got %v", mws)
	}
}

// TestSlogMiddlewareStatusZero tests slogMiddleware when status is 0
func TestSlogMiddlewareStatusZero(t *testing.T) {
	plugin := New()
	ctx := newMockCoreContext()

	err := plugin.Init(ctx)
	if err != nil {
		t.Fatalf("Plugin initialization failed: %v", err)
	}

	// Register a handler that writes body without setting status
	registrar := NewRouteRegistrar(plugin.router)
	registrar.Register("GET /test-zero-status", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("no explicit status"))
	})

	req := httptest.NewRequest("GET", "/test-zero-status", nil)
	w := httptest.NewRecorder()
	plugin.router.ServeHTTP(w, req)

	// Handler writes body, status should be 200 by Go default
	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}
}

// TestGracefulShutdown tests graceful shutdown functionality
func TestGracefulShutdown(t *testing.T) {
	plugin := New()
	ctx := newMockCoreContext()

	// Create shutdown context with timeout
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ctx.ctx = shutdownCtx

	err := plugin.Init(ctx)
	if err != nil {
		t.Fatalf("Plugin initialization failed: %v", err)
	}

	// Start server (in background for test)
	go func() {
		plugin.Start()
	}()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Stop server gracefully
	err = plugin.Stop()
	if err != nil {
		t.Errorf("Graceful shutdown failed: %v", err)
	}

	// Verify server is no longer running
	if plugin.running {
		t.Error("Server should not be running after stop")
	}
}

// TestSubRouter tests sub-router mounting
func TestSubRouter(t *testing.T) {
	plugin := New()
	ctx := newMockCoreContext()

	err := plugin.Init(ctx)
	if err != nil {
		t.Fatalf("Plugin initialization failed: %v", err)
	}

	registrar := NewRouteRegistrar(plugin.router)

	// Create sub-router
	subRouter := chi.NewRouter()
	subRouter.Get("/sub-test", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("sub-router response"))
	})

	// Mount sub-router
	registrar.Mount("/api", subRouter)

	// Test mounted route
	req := httptest.NewRequest("GET", "/api/sub-test", nil)
	w := httptest.NewRecorder()

	plugin.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	if w.Body.String() != "sub-router response" {
		t.Errorf("Expected body 'sub-router response', got '%s'", w.Body.String())
	}
}
