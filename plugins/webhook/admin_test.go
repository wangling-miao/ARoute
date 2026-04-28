package webhook

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
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

// testRouteRegistrar implements interfaces.RouteRegistrar for testing.
type testRouteRegistrar struct {
	router chi.Router
}

func (r *testRouteRegistrar) Handle(pattern string, handler http.Handler) {
	r.router.Handle(pattern, handler)
}
func (r *testRouteRegistrar) HandleFunc(pattern string, handler http.HandlerFunc) {
	r.router.HandleFunc(pattern, handler)
}
func (r *testRouteRegistrar) Use(middlewares ...func(http.Handler) http.Handler)                      {}
func (r *testRouteRegistrar) Middlewares() []func(http.Handler) http.Handler                          { return nil }

// setupAdminRouter creates a chi.Router with admin routes wired to a real Service.
func setupAdminRouter(t *testing.T) (*Service, chi.Router) {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug}))
	svc := NewService(Config{
		DeliveryTimeout:          2 * time.Second,
		MaxRetries:               2,
		MaxConsecutiveFailures:   3,
		DeliveryLogRetentionDays: 30,
	}, logger)

	r := chi.NewRouter()
	handler := &adminHandler{service: svc}
	r.Route("/admin/api/webhooks", func(r chi.Router) {
		r.Get("/", handler.listWebhooks)
		r.Post("/", handler.createWebhook)
		r.Get("/{webhookID}", handler.getWebhook)
		r.Put("/{webhookID}", handler.updateWebhook)
		r.Patch("/{webhookID}", handler.patchWebhook)
		r.Delete("/{webhookID}", handler.deleteWebhook)
		r.Post("/{webhookID}/test", handler.testWebhook)
		r.Get("/{webhookID}/deliveries", handler.listDeliveries)
	})

	return svc, r
}

func doRequest(t *testing.T, router chi.Router, method, path string, body string) *httptest.ResponseRecorder {
	t.Helper()
	var bodyReader io.Reader
	if body != "" {
		bodyReader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, bodyReader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// ─── createWebhook Tests ─────────────────────────────────────────────────────

func TestAdmin_CreateWebhook_Success(t *testing.T) {
	svc, router := setupAdminRouter(t)
	_ = svc

	w := doRequest(t, router, "POST", "/admin/api/webhooks", `{"url":"https://example.com/hook","events":["content.created"],"secret":"test-secret"}`)
	assert.Equal(t, http.StatusCreated, w.Code)

	var wh interfaces.Webhook
	err := json.NewDecoder(w.Body).Decode(&wh)
	require.NoError(t, err)
	assert.NotEmpty(t, wh.ID)
	assert.Equal(t, "https://example.com/hook", wh.URL)
	assert.Equal(t, []string{"content.created"}, wh.Events)
	assert.True(t, wh.Enabled)
}

func TestAdmin_CreateWebhook_InvalidJSON(t *testing.T) {
	_, router := setupAdminRouter(t)

	w := doRequest(t, router, "POST", "/admin/api/webhooks", `{invalid json}`)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "invalid JSON")
}

func TestAdmin_CreateWebhook_InvalidURL(t *testing.T) {
	_, router := setupAdminRouter(t)

	w := doRequest(t, router, "POST", "/admin/api/webhooks", `{"url":"not-a-url","events":["a"],"secret":"test-secret"}`)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAdmin_CreateWebhook_EmptyEvents(t *testing.T) {
	_, router := setupAdminRouter(t)

	w := doRequest(t, router, "POST", "/admin/api/webhooks", `{"url":"https://example.com/hook","events":[],"secret":"test-secret"}`)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "events must not be empty")
}

func TestAdmin_CreateWebhook_ShortSecret(t *testing.T) {
	_, router := setupAdminRouter(t)

	w := doRequest(t, router, "POST", "/admin/api/webhooks", `{"url":"https://example.com/hook","events":["a"],"secret":"short"}`)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "secret must be at least 8 characters")
}

// ─── listWebhooks Tests ─────────────────────────────────────────────────────

func TestAdmin_ListWebhooks_Empty(t *testing.T) {
	_, router := setupAdminRouter(t)

	w := doRequest(t, router, "GET", "/admin/api/webhooks", "")
	assert.Equal(t, http.StatusOK, w.Code)

	var result []interface{}
	err := json.NewDecoder(w.Body).Decode(&result)
	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestAdmin_ListWebhooks_WithItems(t *testing.T) {
	svc, router := setupAdminRouter(t)

	_, err := svc.Create(context.Background(), "https://example.com/a", []string{"a"}, "secret123")
	require.NoError(t, err)
	_, err = svc.Create(context.Background(), "https://example.com/b", []string{"b"}, "secret456")
	require.NoError(t, err)

	w := doRequest(t, router, "GET", "/admin/api/webhooks", "")
	assert.Equal(t, http.StatusOK, w.Code)

	var result []interface{}
	err = json.NewDecoder(w.Body).Decode(&result)
	require.NoError(t, err)
	assert.Len(t, result, 2)
}

// ─── getWebhook Tests ────────────────────────────────────────────────────────

func TestAdmin_GetWebhook_Success(t *testing.T) {
	svc, router := setupAdminRouter(t)

	wh, err := svc.Create(context.Background(), "https://example.com/hook", []string{"content.created"}, "test-secret")
	require.NoError(t, err)

	w := doRequest(t, router, "GET", "/admin/api/webhooks/"+wh.ID, "")
	assert.Equal(t, http.StatusOK, w.Code)

	var result map[string]interface{}
	err = json.NewDecoder(w.Body).Decode(&result)
	require.NoError(t, err)
	assert.Equal(t, wh.ID, result["id"])
	assert.Equal(t, "https://example.com/hook", result["url"])

	stats, ok := result["stats"].(map[string]interface{})
	require.True(t, ok, "response should contain stats")
	assert.Equal(t, float64(0), stats["success_count"])
	assert.Equal(t, float64(0), stats["failure_count"])
}

func TestAdmin_GetWebhook_NotFound(t *testing.T) {
	_, router := setupAdminRouter(t)

	w := doRequest(t, router, "GET", "/admin/api/webhooks/nonexistent", "")
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestAdmin_GetWebhook_WithDeliveries(t *testing.T) {
	svc, router := setupAdminRouter(t)

	wh := createTestWebhook(svc, "https://example.com/hook", []string{"content.created"}, "test-secret")
	svc.mu.Lock()
	svc.deliveries[wh.ID] = []*interfaces.WebhookDelivery{
		{ID: "d1", WebhookID: wh.ID, Success: true},
		{ID: "d2", WebhookID: wh.ID, Success: false},
	}
	svc.mu.Unlock()

	w := doRequest(t, router, "GET", "/admin/api/webhooks/"+wh.ID, "")
	assert.Equal(t, http.StatusOK, w.Code)

	var result map[string]interface{}
	err := json.NewDecoder(w.Body).Decode(&result)
	require.NoError(t, err)

	stats, ok := result["stats"].(map[string]interface{})
	require.True(t, ok, "response should contain stats")
	assert.Equal(t, float64(1), stats["success_count"])
	assert.Equal(t, float64(1), stats["failure_count"])
	assert.Equal(t, float64(2), stats["total"])
}

// ─── updateWebhook Tests ─────────────────────────────────────────────────────

func TestAdmin_UpdateWebhook_Success(t *testing.T) {
	svc, router := setupAdminRouter(t)

	wh, err := svc.Create(context.Background(), "https://example.com/hook", []string{"content.created"}, "test-secret")
	require.NoError(t, err)

	w := doRequest(t, router, "PUT", "/admin/api/webhooks/"+wh.ID, `{"url":"https://example.com/new","events":["content.updated"]}`)
	assert.Equal(t, http.StatusOK, w.Code)

	var updated interfaces.Webhook
	err = json.NewDecoder(w.Body).Decode(&updated)
	require.NoError(t, err)
	assert.Equal(t, "https://example.com/new", updated.URL)
	assert.Equal(t, []string{"content.updated"}, updated.Events)
}

func TestAdmin_UpdateWebhook_InvalidJSON(t *testing.T) {
	svc, router := setupAdminRouter(t)

	wh, err := svc.Create(context.Background(), "https://example.com/hook", []string{"content.created"}, "test-secret")
	require.NoError(t, err)

	w := doRequest(t, router, "PUT", "/admin/api/webhooks/"+wh.ID, `{invalid}`)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAdmin_UpdateWebhook_NotFound(t *testing.T) {
	_, router := setupAdminRouter(t)

	w := doRequest(t, router, "PUT", "/admin/api/webhooks/nonexistent", `{"url":"https://example.com/hook","events":["a"]}`)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestAdmin_UpdateWebhook_InvalidURL(t *testing.T) {
	svc, router := setupAdminRouter(t)

	wh, err := svc.Create(context.Background(), "https://example.com/hook", []string{"content.created"}, "test-secret")
	require.NoError(t, err)

	w := doRequest(t, router, "PUT", "/admin/api/webhooks/"+wh.ID, `{"url":"bad-url","events":["a"]}`)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// ─── patchWebhook Tests ──────────────────────────────────────────────────────

func TestAdmin_PatchWebhook_EnableDisable(t *testing.T) {
	svc, router := setupAdminRouter(t)

	wh, err := svc.Create(context.Background(), "https://example.com/hook", []string{"content.created"}, "test-secret")
	require.NoError(t, err)

	// Disable
	w := doRequest(t, router, "PATCH", "/admin/api/webhooks/"+wh.ID, `{"enabled":false}`)
	assert.Equal(t, http.StatusOK, w.Code)

	got, _ := svc.Get(context.Background(),wh.ID)
	assert.False(t, got.Enabled)

	// Re-enable
	w = doRequest(t, router, "PATCH", "/admin/api/webhooks/"+wh.ID, `{"enabled":true}`)
	assert.Equal(t, http.StatusOK, w.Code)

	got, _ = svc.Get(context.Background(),wh.ID)
	assert.True(t, got.Enabled)
}

func TestAdmin_PatchWebhook_UpdateSecret(t *testing.T) {
	svc, router := setupAdminRouter(t)

	wh, err := svc.Create(context.Background(), "https://example.com/hook", []string{"content.created"}, "test-secret")
	require.NoError(t, err)

	w := doRequest(t, router, "PATCH", "/admin/api/webhooks/"+wh.ID, `{"secret":"new-secret-value"}`)
	assert.Equal(t, http.StatusOK, w.Code)

	got, _ := svc.Get(context.Background(),wh.ID)
	assert.Equal(t, "new-secret-value", got.Secret)
}

func TestAdmin_PatchWebhook_InvalidJSON(t *testing.T) {
	svc, router := setupAdminRouter(t)

	wh, err := svc.Create(context.Background(), "https://example.com/hook", []string{"content.created"}, "test-secret")
	require.NoError(t, err)

	w := doRequest(t, router, "PATCH", "/admin/api/webhooks/"+wh.ID, `{invalid}`)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAdmin_PatchWebhook_NotFound(t *testing.T) {
	_, router := setupAdminRouter(t)

	w := doRequest(t, router, "PATCH", "/admin/api/webhooks/nonexistent", `{"enabled":true}`)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestAdmin_PatchWebhook_SecretNotFound(t *testing.T) {
	_, router := setupAdminRouter(t)

	w := doRequest(t, router, "PATCH", "/admin/api/webhooks/nonexistent", `{"secret":"new-secret"}`)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

// ─── deleteWebhook Tests ─────────────────────────────────────────────────────

func TestAdmin_DeleteWebhook_Success(t *testing.T) {
	svc, router := setupAdminRouter(t)

	wh, err := svc.Create(context.Background(), "https://example.com/hook", []string{"content.created"}, "test-secret")
	require.NoError(t, err)

	w := doRequest(t, router, "DELETE", "/admin/api/webhooks/"+wh.ID, "")
	assert.Equal(t, http.StatusOK, w.Code)

	var result map[string]string
	err = json.NewDecoder(w.Body).Decode(&result)
	require.NoError(t, err)
	assert.Equal(t, "ok", result["status"])

	list := svc.List(context.Background())
	assert.Empty(t, list)
}

func TestAdmin_DeleteWebhook_NotFound(t *testing.T) {
	_, router := setupAdminRouter(t)

	w := doRequest(t, router, "DELETE", "/admin/api/webhooks/nonexistent", "")
	assert.Equal(t, http.StatusNotFound, w.Code)
}

// ─── testWebhook Tests ───────────────────────────────────────────────────────

func TestAdmin_TestWebhook_Success(t *testing.T) {
	svc, router := setupAdminRouter(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer server.Close()

	wh := createTestWebhook(svc, server.URL, []string{"content.created"}, "test-secret")

	w := doRequest(t, router, "POST", "/admin/api/webhooks/"+wh.ID+"/test", "")
	assert.Equal(t, http.StatusOK, w.Code)

	var delivery interfaces.WebhookDelivery
	err := json.NewDecoder(w.Body).Decode(&delivery)
	require.NoError(t, err)
	assert.True(t, delivery.Success)
	assert.Equal(t, "webhook.test", delivery.Event)
}

func TestAdmin_TestWebhook_NotFound(t *testing.T) {
	_, router := setupAdminRouter(t)

	w := doRequest(t, router, "POST", "/admin/api/webhooks/nonexistent/test", "")
	assert.Equal(t, http.StatusNotFound, w.Code)
}

// ─── listDeliveries Tests ────────────────────────────────────────────────────

func TestAdmin_ListDeliveries_DefaultPagination(t *testing.T) {
	svc, router := setupAdminRouter(t)

	wh := createTestWebhook(svc, "https://example.com/hook", []string{"content.created"}, "test-secret")
	svc.mu.Lock()
	svc.deliveries[wh.ID] = []*interfaces.WebhookDelivery{
		{ID: "d1", WebhookID: wh.ID, Success: true},
		{ID: "d2", WebhookID: wh.ID, Success: false},
	}
	svc.mu.Unlock()

	w := doRequest(t, router, "GET", "/admin/api/webhooks/"+wh.ID+"/deliveries", "")
	assert.Equal(t, http.StatusOK, w.Code)

	var result map[string]interface{}
	err := json.NewDecoder(w.Body).Decode(&result)
	require.NoError(t, err)
	assert.Equal(t, float64(2), result["total"])
	assert.Equal(t, float64(20), result["limit"])
	assert.Equal(t, float64(0), result["offset"])
}

func TestAdmin_ListDeliveries_CustomPagination(t *testing.T) {
	svc, router := setupAdminRouter(t)

	wh := createTestWebhook(svc, "https://example.com/hook", []string{"content.created"}, "test-secret")
	svc.mu.Lock()
	svc.deliveries[wh.ID] = []*interfaces.WebhookDelivery{
		{ID: "d1", WebhookID: wh.ID, Success: true},
		{ID: "d2", WebhookID: wh.ID, Success: false},
		{ID: "d3", WebhookID: wh.ID, Success: true},
	}
	svc.mu.Unlock()

	w := doRequest(t, router, "GET", "/admin/api/webhooks/"+wh.ID+"/deliveries?limit=1&offset=1", "")
	assert.Equal(t, http.StatusOK, w.Code)

	var result map[string]interface{}
	err := json.NewDecoder(w.Body).Decode(&result)
	require.NoError(t, err)
	assert.Equal(t, float64(3), result["total"])
	assert.Equal(t, float64(1), result["limit"])
	assert.Equal(t, float64(1), result["offset"])
}

// ─── registerAdminRoutes via Plugin Init ──────────────────────────────────────

func TestPlugin_Init_RegistersAdminRoutes(t *testing.T) {
	p := New()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	bus := events.NewEventBus()
	container := services.NewContainer()

	chiRouter := chi.NewRouter()
	registrar := &testRouteRegistrar{router: chiRouter}

	err := container.Provide(func(c core.ServiceContainer) (interfaces.RouteRegistrar, error) {
		return registrar, nil
	})
	require.NoError(t, err)

	ctx := &mockCoreContext{
		services: container,
		events:   bus,
		config:   &mockConfigProvider{data: map[string]interface{}{}},
		logger:   logger,
		dataDir:  os.TempDir(),
		ctx:      context.Background(),
	}

	err = p.Init(ctx)
	require.NoError(t, err)

	assert.NotNil(t, p.registrar, "registrar should be wired")
	assert.Equal(t, registrar, p.registrar)
}

func TestPlugin_Init_NoRegistrar(t *testing.T) {
	p := New()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	bus := events.NewEventBus()
	container := services.NewContainer()

	ctx := &mockCoreContext{
		services: container,
		events:   bus,
		config:   &mockConfigProvider{data: map[string]interface{}{}},
		logger:   logger,
		dataDir:  os.TempDir(),
		ctx:      context.Background(),
	}

	err := p.Init(ctx)
	require.NoError(t, err)
	assert.Nil(t, p.registrar, "registrar should be nil when not available")
}
