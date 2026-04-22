package queue

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wangling-miao/aroute/core"
	"github.com/wangling-miao/aroute/core/events"
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

func newMockCoreContext(config core.ConfigProvider) *mockCoreContext {
	return &mockCoreContext{
		services:  services.NewContainer(),
		events:    events.NewEventBus(),
		config:    config,
		logger:    slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn})),
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

type mockConfigProvider struct {
	data map[string]interface{}
}

func (m *mockConfigProvider) GetString(key string) string {
	if v, ok := m.data[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func (m *mockConfigProvider) GetInt(key string) int {
	if v, ok := m.data[key]; ok {
		if i, ok := v.(int); ok {
			return i
		}
	}
	return 0
}

func (m *mockConfigProvider) GetBool(key string) bool {
	if v, ok := m.data[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}

func (m *mockConfigProvider) GetStringSlice(key string) []string {
	if v, ok := m.data[key]; ok {
		if s, ok := v.([]string); ok {
			return s
		}
	}
	return nil
}

func (m *mockConfigProvider) Get(key string) interface{} {
	return m.data[key]
}

func (m *mockConfigProvider) Unmarshal(key string, target interface{}) error {
	return nil
}

func TestPlugin_New(t *testing.T) {
	p := New()

	assert.NotNil(t, p)
	assert.Equal(t, "queue", p.Name())
	assert.Equal(t, "1.0.0", p.Version())

	manifest := p.Manifest()
	require.NotNil(t, manifest)
	assert.Equal(t, "queue", manifest.Name)
	assert.Equal(t, "native", manifest.Engine)
}

func TestPlugin_Init_DefaultConfig(t *testing.T) {
	p := New()
	ctx := newMockCoreContext(&mockConfigProvider{data: map[string]interface{}{}})
	t.Cleanup(func() { p.Stop() })

	err := p.Init(ctx)
	require.NoError(t, err)

	var queueSvc interfaces.QueueService
	err = ctx.Services().Get(&queueSvc)
	require.NoError(t, err)
	assert.NotNil(t, queueSvc)

	svc := p.service
	require.NotNil(t, svc)
	assert.Equal(t, runtime.NumCPU(), svc.config.Workers)
	assert.Equal(t, 30*time.Second, svc.config.ShutdownTimeout)
	assert.Equal(t, 3, svc.config.DefaultMaxRetries)
	assert.Equal(t, 60*time.Second, svc.config.DefaultTimeout)
}

func TestPlugin_Init_CustomConfig(t *testing.T) {
	p := New()
	cfg := &mockConfigProvider{
		data: map[string]interface{}{
			"workers":                  4,
			"shutdown_timeout_seconds": 10,
			"default_max_retries":      5,
			"default_timeout_seconds":  30,
		},
	}
	ctx := newMockCoreContext(cfg)
	t.Cleanup(func() { p.Stop() })

	err := p.Init(ctx)
	require.NoError(t, err)

	svc := p.service
	require.NotNil(t, svc)
	assert.Equal(t, 4, svc.config.Workers)
	assert.Equal(t, 10*time.Second, svc.config.ShutdownTimeout)
	assert.Equal(t, 5, svc.config.DefaultMaxRetries)
	assert.Equal(t, 30*time.Second, svc.config.DefaultTimeout)
}

func TestPlugin_Start(t *testing.T) {
	p := New()
	ctx := newMockCoreContext(&mockConfigProvider{data: map[string]interface{}{}})
	t.Cleanup(func() { p.Stop() })

	err := p.Init(ctx)
	require.NoError(t, err)

	err = p.Start()
	require.NoError(t, err)
	assert.True(t, p.running)
}

func TestPlugin_Stop(t *testing.T) {
	p := New()
	ctx := newMockCoreContext(&mockConfigProvider{data: map[string]interface{}{}})

	err := p.Init(ctx)
	require.NoError(t, err)

	err = p.Start()
	require.NoError(t, err)

	err = p.Stop()
	require.NoError(t, err)
	assert.False(t, p.running)
}

func TestPlugin_StartStop_Idempotent(t *testing.T) {
	p := New()
	ctx := newMockCoreContext(&mockConfigProvider{data: map[string]interface{}{}})

	err := p.Init(ctx)
	require.NoError(t, err)

	err = p.Start()
	require.NoError(t, err)
	err = p.Start()
	require.NoError(t, err)
	assert.True(t, p.running)

	err = p.Stop()
	require.NoError(t, err)
	err = p.Stop()
	require.NoError(t, err)
	assert.False(t, p.running)
}

// --- Mock Route Registrar ---

type mockRouteRegistrar struct {
	mu     sync.Mutex
	router *chi.Mux
}

func newMockRouteRegistrar() *mockRouteRegistrar {
	return &mockRouteRegistrar{
		router: chi.NewMux(),
	}
}

func (m *mockRouteRegistrar) Handle(pattern string, handler http.Handler) {
	m.mu.Lock()
	m.router.Handle(pattern, handler)
	m.mu.Unlock()
}

func (m *mockRouteRegistrar) HandleFunc(pattern string, handler http.HandlerFunc) {
	m.mu.Lock()
	m.router.HandleFunc(pattern, handler)
	m.mu.Unlock()
}

func (m *mockRouteRegistrar) Use(middlewares ...func(http.Handler) http.Handler) {}

func (m *mockRouteRegistrar) Middlewares() []func(http.Handler) http.Handler { return nil }

func (m *mockRouteRegistrar) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	m.router.ServeHTTP(w, r)
}

// --- Test Helpers ---

// newDeadLetterTestService creates a service with DefaultMaxRetries=0 so tasks
// are dead-lettered on first failure without retries. This avoids the plugin
// init path which maps 0→3.
func newDeadLetterTestService(t *testing.T) *Service {
	t.Helper()
	cfg := Config{
		Workers:           1,
		ShutdownTimeout:   5 * time.Second,
		DefaultMaxRetries: 0,
		DefaultTimeout:    2 * time.Second,
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	svc := NewService(cfg, logger, nil)
	svc.Start()
	return svc
}

// withPluginAndRegistrar creates a plugin with a mock RouteRegistrar.
func withPluginAndRegistrar(t *testing.T) (*Plugin, *mockRouteRegistrar) {
	t.Helper()
	p := New()
	registrar := newMockRouteRegistrar()
	ctx := newMockCoreContext(&mockConfigProvider{data: map[string]interface{}{}})
	ctx.services.Provide(func(_ core.ServiceContainer) (interfaces.RouteRegistrar, error) {
		return registrar, nil
	})
	require.NoError(t, p.Init(ctx))
	require.NoError(t, p.Start())
	t.Cleanup(func() { p.Stop() })
	return p, registrar
}

// --- HTTP Handler Tests ---

func TestAdminHandler_ListDeadLetters_Empty(t *testing.T) {
	svc := newDeadLetterTestService(t)
	defer svc.Close(context.Background())

	h := &adminHandler{service: svc}
	req := httptest.NewRequest(http.MethodGet, "/admin/api/queue/dead-letter?page=1&page_size=10", nil)
	w := httptest.NewRecorder()
	h.listDeadLetters(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, float64(0), resp["total"])

	data := resp["data"].([]interface{})
	assert.Empty(t, data)
}

func TestAdminHandler_ListDeadLetters_WithData(t *testing.T) {
	svc := newDeadLetterTestService(t)
	defer svc.Close(context.Background())

	bgCtx := context.Background()

	require.NoError(t, svc.RegisterTask(bgCtx, "list-dl-test", func(ctx context.Context, payload interface{}) error {
		return errors.New("dl fail")
	}))
	taskID, err := svc.Enqueue(bgCtx, "list-dl-test", map[string]string{"key": "val"}, nil)
	require.NoError(t, err)

	assert.Eventually(t, func() bool {
		status, err := svc.GetStatus(bgCtx, taskID)
		return err == nil && status.Status == interfaces.TaskStatusDeadLettered
	}, 5*time.Second, 100*time.Millisecond)

	h := &adminHandler{service: svc}
	req := httptest.NewRequest(http.MethodGet, "/admin/api/queue/dead-letter?page=1&page_size=10", nil)
	w := httptest.NewRecorder()
	h.listDeadLetters(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, float64(1), resp["total"])
	assert.Equal(t, float64(1), resp["page"])
	assert.Equal(t, float64(10), resp["page_size"])

	data := resp["data"].([]interface{})
	require.Len(t, data, 1)
	entry := data[0].(map[string]interface{})
	assert.Equal(t, taskID, entry["task_id"])
	assert.Equal(t, "list-dl-test", entry["name"])
	assert.Equal(t, "dl fail", entry["last_error"])
}

func TestAdminHandler_ListDeadLetters_DefaultPagination(t *testing.T) {
	svc := newDeadLetterTestService(t)
	defer svc.Close(context.Background())

	bgCtx := context.Background()

	require.NoError(t, svc.RegisterTask(bgCtx, "pag-default-test", func(ctx context.Context, payload interface{}) error {
		return errors.New("fail")
	}))

	var taskIDs []string
	for i := 0; i < 3; i++ {
		id, err := svc.Enqueue(bgCtx, "pag-default-test", i, nil)
		require.NoError(t, err)
		taskIDs = append(taskIDs, id)
	}

	assert.Eventually(t, func() bool {
		_, total, _ := svc.ListDeadLetters(bgCtx, 1, 100)
		return total == 3
	}, 5*time.Second, 100*time.Millisecond)

	h := &adminHandler{service: svc}

	// Request without page/page_size query params — handler defaults to page=1, page_size=20
	req := httptest.NewRequest(http.MethodGet, "/admin/api/queue/dead-letter", nil)
	w := httptest.NewRecorder()
	h.listDeadLetters(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, float64(1), resp["page"])
	assert.Equal(t, float64(20), resp["page_size"])
	assert.Equal(t, float64(3), resp["total"])
	data := resp["data"].([]interface{})
	assert.Len(t, data, 3)

	_ = taskIDs
}

func TestAdminHandler_RetryDeadLetter_Success(t *testing.T) {
	svc := newDeadLetterTestService(t)
	defer svc.Close(context.Background())

	bgCtx := context.Background()

	var attempt atomic.Int32
	require.NoError(t, svc.RegisterTask(bgCtx, "retry-dl-test", func(ctx context.Context, payload interface{}) error {
		if attempt.Add(1) <= 1 {
			return errors.New("first fail")
		}
		return nil
	}))
	taskID, err := svc.Enqueue(bgCtx, "retry-dl-test", nil, nil)
	require.NoError(t, err)

	assert.Eventually(t, func() bool {
		status, err := svc.GetStatus(bgCtx, taskID)
		return err == nil && status.Status == interfaces.TaskStatusDeadLettered
	}, 5*time.Second, 100*time.Millisecond)

	h := &adminHandler{service: svc}
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("taskID", taskID)
	req := httptest.NewRequest(http.MethodPost, "/admin/api/queue/dead-letter/"+taskID+"/retry", nil)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()
	h.retryDeadLetter(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]string
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "ok", resp["status"])

	// Task should eventually complete after retry
	assert.Eventually(t, func() bool {
		status, err := svc.GetStatus(bgCtx, taskID)
		return err == nil && status.Status == interfaces.TaskStatusCompleted
	}, 5*time.Second, 100*time.Millisecond)
}

func TestAdminHandler_RetryDeadLetter_NotFound(t *testing.T) {
	svc := newDeadLetterTestService(t)
	defer svc.Close(context.Background())

	h := &adminHandler{service: svc}
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("taskID", "nonexistent-id")
	req := httptest.NewRequest(http.MethodPost, "/admin/api/queue/dead-letter/nonexistent-id/retry", nil)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()
	h.retryDeadLetter(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestAdminHandler_DeleteDeadLetter_Success(t *testing.T) {
	svc := newDeadLetterTestService(t)
	defer svc.Close(context.Background())

	bgCtx := context.Background()

	require.NoError(t, svc.RegisterTask(bgCtx, "del-dl-test", func(ctx context.Context, payload interface{}) error {
		return errors.New("always fail")
	}))
	taskID, err := svc.Enqueue(bgCtx, "del-dl-test", nil, nil)
	require.NoError(t, err)

	assert.Eventually(t, func() bool {
		status, err := svc.GetStatus(bgCtx, taskID)
		return err == nil && status.Status == interfaces.TaskStatusDeadLettered
	}, 5*time.Second, 100*time.Millisecond)

	h := &adminHandler{service: svc}
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("taskID", taskID)
	req := httptest.NewRequest(http.MethodDelete, "/admin/api/queue/dead-letter/"+taskID, nil)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()
	h.deleteDeadLetter(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]string
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "ok", resp["status"])

	// Dead letter should be gone
	_, total, _ := svc.ListDeadLetters(bgCtx, 1, 100)
	assert.Equal(t, 0, total)
}

func TestAdminHandler_DeleteDeadLetter_NotFound(t *testing.T) {
	svc := newDeadLetterTestService(t)
	defer svc.Close(context.Background())

	h := &adminHandler{service: svc}
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("taskID", "nonexistent-id")
	req := httptest.NewRequest(http.MethodDelete, "/admin/api/queue/dead-letter/nonexistent-id", nil)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()
	h.deleteDeadLetter(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestAdminHandler_RetryDeadLetter_MissingTaskID(t *testing.T) {
	svc := newDeadLetterTestService(t)
	defer svc.Close(context.Background())

	h := &adminHandler{service: svc}
	req := httptest.NewRequest(http.MethodPost, "/admin/api/queue/dead-letter//retry", nil)
	w := httptest.NewRecorder()
	h.retryDeadLetter(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAdminHandler_DeleteDeadLetter_MissingTaskID(t *testing.T) {
	svc := newDeadLetterTestService(t)
	defer svc.Close(context.Background())

	h := &adminHandler{service: svc}
	req := httptest.NewRequest(http.MethodDelete, "/admin/api/queue/dead-letter/", nil)
	w := httptest.NewRecorder()
	h.deleteDeadLetter(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// --- registerAdminRoutes coverage ---

// TestPlugin_Init_WithRouteRegistrar verifies registerAdminRoutes registers
// routes on the registrar and that they are reachable via HTTP.
func TestPlugin_Init_WithRouteRegistrar(t *testing.T) {
	p := New()
	registrar := newMockRouteRegistrar()
	ctx := newMockCoreContext(&mockConfigProvider{data: map[string]interface{}{}})
	ctx.services.Provide(func(_ core.ServiceContainer) (interfaces.RouteRegistrar, error) {
		return registrar, nil
	})
	require.NoError(t, p.Init(ctx))
	require.NoError(t, p.Start())
	t.Cleanup(func() { p.Stop() })

	// Verify routes are reachable via the registrar's backing router
	t.Run("list dead letters via registrar", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/admin/api/queue/dead-letter?page=1&page_size=10", nil)
		w := httptest.NewRecorder()
		registrar.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, float64(0), resp["total"])
	})

	t.Run("retry dead letter not found via registrar", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/admin/api/queue/dead-letter/nonexistent/retry", nil)
		w := httptest.NewRecorder()
		registrar.ServeHTTP(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("delete dead letter not found via registrar", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/admin/api/queue/dead-letter/nonexistent", nil)
		w := httptest.NewRecorder()
		registrar.ServeHTTP(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}
