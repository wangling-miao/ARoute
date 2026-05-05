package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wangling-miao/aroute/core"
	"github.com/wangling-miao/aroute/sdk/interfaces"
)

// ---------------------------------------------------------------------------
// mock LifecycleManager
// ---------------------------------------------------------------------------

type mockLifecycleManager struct {
	plugins    []string
	pluginMap  map[string]core.Plugin
	enableErr  error
	disableErr error
}

func (m *mockLifecycleManager) LoadAll(_ context.Context) error  { return nil }
func (m *mockLifecycleManager) Start(_ context.Context) error    { return nil }
func (m *mockLifecycleManager) Stop(_ context.Context) error     { return nil }
func (m *mockLifecycleManager) Enable(_ context.Context, _ string) error {
	return m.enableErr
}
func (m *mockLifecycleManager) Disable(_ context.Context, _ string) error {
	return m.disableErr
}
func (m *mockLifecycleManager) GetState(_ string) (core.PluginState, error) {
	return core.StateActive, nil
}
func (m *mockLifecycleManager) GetPlugin(name string) core.Plugin {
	if m.pluginMap != nil {
		return m.pluginMap[name]
	}
	return nil
}
func (m *mockLifecycleManager) ListPlugins() []string {
	return m.plugins
}

// ---------------------------------------------------------------------------
// mock CacheService
// ---------------------------------------------------------------------------

type mockCacheService struct {
	stats *interfaces.CacheStats
}

func (m *mockCacheService) Get(_ context.Context, _ string) (interface{}, bool) { return nil, false }
func (m *mockCacheService) Set(_ context.Context, _ string, _ interface{}, _ time.Duration) error {
	return nil
}
func (m *mockCacheService) Delete(_ context.Context, _ string) error    { return nil }
func (m *mockCacheService) Invalidate(_ context.Context, _ string) error { return nil }
func (m *mockCacheService) Stats(_ context.Context) *interfaces.CacheStats {
	return m.stats
}
func (m *mockCacheService) Flush(_ context.Context) error { return nil }

// ---------------------------------------------------------------------------
// mock Plugin for lifecycle tests
// ---------------------------------------------------------------------------

type mockPlugin struct {
	name    string
	version string
	manifest *core.Manifest
}

func (m *mockPlugin) Name() string                     { return m.name }
func (m *mockPlugin) Version() string                  { return m.version }
func (m *mockPlugin) Manifest() *core.Manifest         { return m.manifest }
func (m *mockPlugin) Init(_ core.CoreContext) error     { return nil }
func (m *mockPlugin) Start() error                     { return nil }
func (m *mockPlugin) Stop() error                      { return nil }

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// adminMockServiceContainer provides services for admin handler tests.
type adminMockServiceContainer struct {
	services map[string]interface{}
}

func newAdminMockServiceContainer(svcs ...interface{}) *adminMockServiceContainer {
	m := &adminMockServiceContainer{services: make(map[string]interface{})}
	for _, svc := range svcs {
		m.services[typeName(svc)] = svc
	}
	return m
}

func typeName(v interface{}) string {
	rv := reflect.ValueOf(v)
	return rv.Type().String()
}

func (m *adminMockServiceContainer) Get(target interface{}) error {
	rv := reflect.ValueOf(target)
	if rv.Kind() != reflect.Ptr {
		return fmt.Errorf("target must be a pointer")
	}
	targetType := rv.Elem().Type().String()

	if svc, ok := m.services[targetType]; ok {
		rv.Elem().Set(reflect.ValueOf(svc))
		return nil
	}

	// Interface match
	for _, svc := range m.services {
		svcVal := reflect.ValueOf(svc)
		if svcVal.Type().Implements(rv.Elem().Type()) {
			rv.Elem().Set(svcVal)
			return nil
		}
	}

	return fmt.Errorf("service not found: %s", targetType)
}

func (m *adminMockServiceContainer) Provide(_ interface{}) error            { return nil }
func (m *adminMockServiceContainer) GetNamed(_ string, _ interface{}) error { return nil }
func (m *adminMockServiceContainer) Unregister(_ interface{}) error         { return nil }
func (m *adminMockServiceContainer) Has(_ interface{}) bool                 { return true }
func (m *adminMockServiceContainer) Keys() []string                         { return nil }

func setupAdminHandler(contentSvc interfaces.ContentService, lifecycle core.LifecycleManager, cacheSvc interfaces.CacheService) *AdminHandler {
	svcs := []interface{}{contentSvc}
	if lifecycle != nil {
		svcs = append(svcs, lifecycle)
	}
	if cacheSvc != nil {
		svcs = append(svcs, cacheSvc)
	}

	coreCtx := core.NewCoreContext(
		context.Background(),
		newAdminMockServiceContainer(svcs...),
		&pluginMockEventBus{},
		newMockConfigProvider(map[string]interface{}{
			"site_name": "Test Site",
			"site_url":  "http://localhost",
		}),
		slog.Default(),
		"",
		"",
	)

	return NewAdminHandler(coreCtx, contentSvc, nil)
}

// ===========================================================================
// handleDashboardStats tests
// ===========================================================================

func TestHandleDashboardStats_Success(t *testing.T) {
	mock := &mockContentService{
		listContentTypesFunc: func(_ context.Context) ([]*interfaces.ContentType, error) {
			return []*interfaces.ContentType{
				{Name: "post"},
				{Name: "page"},
			}, nil
		},
		listFunc: func(_ context.Context, ct string, _ *interfaces.ListQuery) (*interfaces.Page, error) {
			return &interfaces.Page{
				Meta: interfaces.PageMeta{Total: 5},
			}, nil
		},
	}

	lm := &mockLifecycleManager{plugins: []string{"http", "api"}}
	cache := &mockCacheService{stats: &interfaces.CacheStats{HitRate: 0.85}}

	handler := setupAdminHandler(mock, lm, cache)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/stats", nil)

	handler.handleDashboardStats(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var resp APIResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))

	dataBytes, _ := json.Marshal(resp.Data)
	var data map[string]interface{}
	require.NoError(t, json.Unmarshal(dataBytes, &data))

	sysStatus, ok := data["system_status"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, float64(2), sysStatus["plugin_count"])
	assert.Equal(t, 0.85, sysStatus["cache_hit_ratio"])
}

func TestHandleDashboardStats_ListTypesError(t *testing.T) {
	mock := &mockContentService{
		listContentTypesFunc: func(_ context.Context) ([]*interfaces.ContentType, error) {
			return nil, fmt.Errorf("db error")
		},
	}

	handler := setupAdminHandler(mock, nil, nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/stats", nil)

	handler.handleDashboardStats(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	// Should still return data with empty content_counts
	var resp APIResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
}

func TestHandleDashboardStats_NoLifecycle(t *testing.T) {
	mock := &mockContentService{
		listContentTypesFunc: func(_ context.Context) ([]*interfaces.ContentType, error) {
			return nil, nil
		},
	}

	handler := setupAdminHandler(mock, nil, nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/stats", nil)

	handler.handleDashboardStats(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
}

// ===========================================================================
// handleGetSettings tests
// ===========================================================================

func TestHandleGetSettings(t *testing.T) {
	mock := &mockContentService{}
	handler := setupAdminHandler(mock, nil, nil)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)

	handler.handleGetSettings(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var resp APIResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))

	dataBytes, _ := json.Marshal(resp.Data)
	var settings map[string]interface{}
	require.NoError(t, json.Unmarshal(dataBytes, &settings))
	assert.Equal(t, "Test Site", settings["site_name"])
	assert.Equal(t, "http://localhost", settings["site_url"])
}

// ===========================================================================
// handleUpdateSettings tests
// ===========================================================================

func TestHandleUpdateSettings_ValidJSON(t *testing.T) {
	mock := &mockContentService{}
	handler := setupAdminHandler(mock, nil, nil)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/settings", strings.NewReader(`{"site_name":"New Name"}`))

	handler.handleUpdateSettings(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestHandleUpdateSettings_InvalidJSON(t *testing.T) {
	mock := &mockContentService{}
	handler := setupAdminHandler(mock, nil, nil)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/settings", strings.NewReader("{invalid}"))

	handler.handleUpdateSettings(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	apiErr := firstAPIError(t, rr.Body.Bytes())
	assert.Equal(t, "INVALID_JSON", apiErr.Code)
}

// ===========================================================================
// handleListPlugins tests
// ===========================================================================

func TestHandleListPlugins_WithLifecycle(t *testing.T) {
	mock := &mockContentService{}
	lm := &mockLifecycleManager{
		plugins: []string{"http", "api"},
		pluginMap: map[string]core.Plugin{
			"http": &mockPlugin{name: "http", version: "1.0.0", manifest: &core.Manifest{Name: "http", Description: "HTTP server", Author: "dev"}},
			"api":  &mockPlugin{name: "api", version: "1.0.0"},
		},
	}

	handler := setupAdminHandler(mock, lm, nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/plugins", nil)

	handler.handleListPlugins(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var resp APIResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))

	dataBytes, _ := json.Marshal(resp.Data)
	var plugins []map[string]interface{}
	require.NoError(t, json.Unmarshal(dataBytes, &plugins))
	assert.Len(t, plugins, 2)
}

func TestHandleListPlugins_NoLifecycle(t *testing.T) {
	mock := &mockContentService{}
	handler := setupAdminHandler(mock, nil, nil)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/plugins", nil)

	handler.handleListPlugins(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var resp APIResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))

	dataBytes, _ := json.Marshal(resp.Data)
	assert.Equal(t, "[]", string(dataBytes))
}

func TestHandleListPlugins_NilPluginInList(t *testing.T) {
	mock := &mockContentService{}
	lm := &mockLifecycleManager{
		plugins:   []string{"http", "missing"},
		pluginMap: map[string]core.Plugin{"http": &mockPlugin{name: "http", version: "1.0.0"}},
	}

	handler := setupAdminHandler(mock, lm, nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/plugins", nil)

	handler.handleListPlugins(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var resp APIResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))

	dataBytes, _ := json.Marshal(resp.Data)
	var plugins []map[string]interface{}
	require.NoError(t, json.Unmarshal(dataBytes, &plugins))
	// Only "http" should appear since "missing" returns nil from GetPlugin
	assert.Len(t, plugins, 1)
}

// ===========================================================================
// handleEnablePlugin tests
// ===========================================================================

func TestHandleEnablePlugin_Success(t *testing.T) {
	mock := &mockContentService{}
	lm := &mockLifecycleManager{}
	handler := setupAdminHandler(mock, lm, nil)

	rr := httptest.NewRecorder()
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("name", "http")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/plugins/http/enable", nil)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	handler.handleEnablePlugin(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var resp APIResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	dataBytes, _ := json.Marshal(resp.Data)
	var data map[string]interface{}
	require.NoError(t, json.Unmarshal(dataBytes, &data))
	assert.Equal(t, "enabled", data["status"])
}

func TestHandleEnablePlugin_EmptyName(t *testing.T) {
	mock := &mockContentService{}
	handler := setupAdminHandler(mock, nil, nil)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/plugins//enable", nil)

	handler.handleEnablePlugin(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestHandleEnablePlugin_EnableError(t *testing.T) {
	mock := &mockContentService{}
	lm := &mockLifecycleManager{enableErr: fmt.Errorf("cannot enable")}
	handler := setupAdminHandler(mock, lm, nil)

	rr := httptest.NewRecorder()
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("name", "broken")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/plugins/broken/enable", nil)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	handler.handleEnablePlugin(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

func TestHandleEnablePlugin_NoLifecycle(t *testing.T) {
	mock := &mockContentService{}
	handler := setupAdminHandler(mock, nil, nil)

	rr := httptest.NewRecorder()
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("name", "http")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/plugins/http/enable", nil)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	handler.handleEnablePlugin(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
}

// ===========================================================================
// handleDisablePlugin tests
// ===========================================================================

func TestHandleDisablePlugin_Success(t *testing.T) {
	mock := &mockContentService{}
	lm := &mockLifecycleManager{}
	handler := setupAdminHandler(mock, lm, nil)

	rr := httptest.NewRecorder()
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("name", "http")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/plugins/http/disable", nil)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	handler.handleDisablePlugin(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var resp APIResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	dataBytes, _ := json.Marshal(resp.Data)
	var data map[string]interface{}
	require.NoError(t, json.Unmarshal(dataBytes, &data))
	assert.Equal(t, "disabled", data["status"])
}

func TestHandleDisablePlugin_EmptyName(t *testing.T) {
	mock := &mockContentService{}
	handler := setupAdminHandler(mock, nil, nil)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/plugins//disable", nil)

	handler.handleDisablePlugin(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestHandleDisablePlugin_DisableError(t *testing.T) {
	mock := &mockContentService{}
	lm := &mockLifecycleManager{disableErr: fmt.Errorf("cannot disable")}
	handler := setupAdminHandler(mock, lm, nil)

	rr := httptest.NewRecorder()
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("name", "broken")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/plugins/broken/disable", nil)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	handler.handleDisablePlugin(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

func TestHandleDisablePlugin_NoLifecycle(t *testing.T) {
	mock := &mockContentService{}
	handler := setupAdminHandler(mock, nil, nil)

	rr := httptest.NewRecorder()
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("name", "http")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/plugins/http/disable", nil)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	handler.handleDisablePlugin(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
}
