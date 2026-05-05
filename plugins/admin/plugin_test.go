package admin

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wangling-miao/aroute/core"
	"github.com/wangling-miao/aroute/core/services"
	"github.com/wangling-miao/aroute/sdk/interfaces"
)

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
	return &mockCoreContext{
		services:  services.NewContainer(),
		config:    &mockConfig{data: map[string]interface{}{}},
		logger:    slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})),
		dataDir:   createMockAdminDir(),
		pluginDir: os.TempDir(),
		ctx:       context.Background(),
	}
}

// createMockAdminDir creates a temp directory that mimics data/plugin_data/admin/.
// The admin plugin's DataDir() returns this path, and variants are subdirectories:
//   <tmp>/admin/default/index.html
func createMockAdminDir() string {
	dir, err := os.MkdirTemp("", "admin-test-*")
	if err != nil {
		panic(err)
	}
	variantDir := filepath.Join(dir, "default")
	os.MkdirAll(filepath.Join(variantDir, "assets"), 0o755)
	os.WriteFile(filepath.Join(variantDir, "index.html"), []byte("<!doctype html><html><head></head><body>admin</body></html>"), 0o644)
	os.WriteFile(filepath.Join(variantDir, "assets", "index-CamzK9Cm.js"), []byte("// js"), 0o644)
	return dir
}

func (m *mockCoreContext) Services() core.ServiceContainer { return m.services }
func (m *mockCoreContext) Events() core.EventBus           { return m.events }
func (m *mockCoreContext) Config() core.ConfigProvider     { return m.config }
func (m *mockCoreContext) Logger() *slog.Logger            { return m.logger }
func (m *mockCoreContext) DataDir() string                 { return m.dataDir }
func (m *mockCoreContext) PluginDir() string               { return m.pluginDir }
func (m *mockCoreContext) Context() context.Context        { return m.ctx }

type mockConfig struct {
	data map[string]interface{}
}

func (m *mockConfig) GetString(key string) string                    { return "" }
func (m *mockConfig) GetInt(key string) int                          { return 0 }
func (m *mockConfig) GetBool(key string) bool                        { return false }
func (m *mockConfig) GetStringSlice(key string) []string             { return nil }
func (m *mockConfig) Get(key string) interface{}                     { return m.data[key] }
func (m *mockConfig) Unmarshal(key string, target interface{}) error { return nil }
func (m *mockConfig) Set(key string, value interface{})              { m.data[key] = value }
func (m *mockConfig) Save() error                                    { return nil }

func setupRouterWithAdmin(t *testing.T) (*Plugin, chi.Router) {
	t.Helper()
	p := New()
	ctx := newMockCoreContext()
	router := chi.NewRouter()

	registrar := httpPluginRouteRegistrar{router: router}

	err := ctx.Services().Provide(func(c core.ServiceContainer) (interfaces.RouteRegistrar, error) {
		return &registrar, nil
	})
	require.NoError(t, err)

	err = p.Init(ctx)
	require.NoError(t, err)

	return p, router
}

type httpPluginRouteRegistrar struct {
	router chi.Router
}

func (r *httpPluginRouteRegistrar) Handle(pattern string, handler http.Handler) {
	r.router.Handle(pattern, handler)
}
func (r *httpPluginRouteRegistrar) HandleFunc(pattern string, handler http.HandlerFunc) {
	r.router.HandleFunc(pattern, handler)
}
func (r *httpPluginRouteRegistrar) Use(middlewares ...func(http.Handler) http.Handler) {}
func (r *httpPluginRouteRegistrar) Middlewares() []func(http.Handler) http.Handler     { return nil }

func TestPlugin_New(t *testing.T) {
	p := New()
	assert.NotNil(t, p)
	assert.Equal(t, "admin", p.Name())
	assert.Equal(t, "1.0.0", p.Version())
}

func TestPlugin_Init(t *testing.T) {
	_, router := setupRouterWithAdmin(t)

	walkRoutes(t, router, func(method, route string) {
		t.Logf("Registered route: %s %s", method, route)
	})
}

func TestPlugin_StartStop(t *testing.T) {
	p, _ := setupRouterWithAdmin(t)

	err := p.Start()
	require.NoError(t, err)
	assert.True(t, p.running)

	err = p.Start()
	require.NoError(t, err)

	err = p.Stop()
	require.NoError(t, err)
	assert.False(t, p.running)

	err = p.Stop()
	require.NoError(t, err)
}

func TestPlugin_ServesIndexHTML(t *testing.T) {
	p, _ := setupRouterWithAdmin(t)

	req := httptest.NewRequest("GET", "/", nil)
	req.URL.Path = "/"
	w := httptest.NewRecorder()
	p.handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestPlugin_ClientRouteFallback(t *testing.T) {
	p, _ := setupRouterWithAdmin(t)

	req := httptest.NewRequest("GET", "/content/articles", nil)
	w := httptest.NewRecorder()
	p.handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestPlugin_ServeAdmin(t *testing.T) {
	p, router := setupRouterWithAdmin(t)
	require.NoError(t, p.Start())
	defer p.Stop()

	req := httptest.NewRequest("GET", "/admin/", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Header().Get("Content-Type"), "text/html")
}

func TestPlugin_ServeAdminClientRoute(t *testing.T) {
	p, router := setupRouterWithAdmin(t)
	require.NoError(t, p.Start())
	defer p.Stop()

	req := httptest.NewRequest("GET", "/admin/content/articles", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Header().Get("Content-Type"), "text/html")
}

func TestPlugin_ServeStaticAssetWithCache(t *testing.T) {
	p, _ := setupRouterWithAdmin(t)

	req := httptest.NewRequest("GET", "/assets/index-CamzK9Cm.js", nil)
	w := httptest.NewRecorder()
	p.handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "public, max-age=31536000, immutable", w.Header().Get("Cache-Control"))
}

func TestPlugin_ServeIndexHTMLCacheControl(t *testing.T) {
	p, _ := setupRouterWithAdmin(t)

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	p.handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "no-cache", w.Header().Get("Cache-Control"))
}

func TestPlugin_IndexHTMLContentType(t *testing.T) {
	p, _ := setupRouterWithAdmin(t)

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	p.handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "text/html; charset=utf-8", w.Header().Get("Content-Type"))
	assert.Contains(t, w.Body.String(), "<!doctype")
}

func TestPlugin_DevModeProxy(t *testing.T) {
	t.Setenv("AROUTE_DEV_MODE", "true")

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("<!doctype html><html>dev</html>"))
	}))
	defer backend.Close()

	p := New()
	ctx := newMockCoreContext()
	router := chi.NewRouter()
	registrar := httpPluginRouteRegistrar{router: router}

	err := ctx.Services().Provide(func(c core.ServiceContainer) (interfaces.RouteRegistrar, error) {
		return &registrar, nil
	})
	require.NoError(t, err)

	err = p.Init(ctx)
	require.NoError(t, err)

	assert.True(t, p.devMode)
	assert.NotNil(t, p.devProxy)

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	p.handler.ServeHTTP(w, req)
}

func TestPlugin_NonAssetPathNoCacheHeader(t *testing.T) {
	p, _ := setupRouterWithAdmin(t)

	req := httptest.NewRequest("GET", "/nonexistent-path", nil)
	w := httptest.NewRecorder()
	p.handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "no-cache", w.Header().Get("Cache-Control"))
}

func walkRoutes(t *testing.T, r chi.Router, fn func(method, route string)) {
	t.Helper()
	walk := chi.Walk(r, func(method, route string, handler http.Handler, middlewares ...func(http.Handler) http.Handler) error {
		fn(method, route)
		return nil
	})
	if walk != nil {
		t.Logf("Walk error: %v", walk)
	}
}
