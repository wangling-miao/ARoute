package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wangling-miao/aroute/core"
	"github.com/wangling-miao/aroute/core/events"
	"github.com/wangling-miao/aroute/sdk/interfaces"
)

// --- Mock implementations ---

type pluginMockContentService struct {
	mockContentService
}

type pluginMockAuthService struct {
	mockAuthService
}

// pluginMockServiceContainer provides services for plugin tests.
// It resolves services by type using reflection, matching the
// CoreContainer.Get(&target) pattern.
type pluginMockServiceContainer struct {
	services map[string]interface{}
}

func newPluginMockServiceContainer(svcs ...interface{}) *pluginMockServiceContainer {
	m := &pluginMockServiceContainer{services: make(map[string]interface{})}
	for _, svc := range svcs {
		m.services[reflect.TypeOf(svc).String()] = svc
	}
	return m
}

func (m *pluginMockServiceContainer) Get(target interface{}) error {
	rv := reflect.ValueOf(target)
	if rv.Kind() != reflect.Ptr {
		return fmt.Errorf("target must be a pointer")
	}
	targetType := rv.Elem().Type().String()

	// Direct type match
	if svc, ok := m.services[targetType]; ok {
		rv.Elem().Set(reflect.ValueOf(svc))
		return nil
	}

	// Interface match: check if any stored service implements the target interface
	for _, svc := range m.services {
		svcVal := reflect.ValueOf(svc)
		if svcVal.Type().Implements(rv.Elem().Type()) {
			rv.Elem().Set(svcVal)
			return nil
		}
	}

	return fmt.Errorf("service not found: %s", targetType)
}

func (m *pluginMockServiceContainer) Provide(_ interface{}) error            { return nil }
func (m *pluginMockServiceContainer) GetNamed(_ string, _ interface{}) error { return nil }
func (m *pluginMockServiceContainer) Unregister(_ interface{}) error         { return nil }
func (m *pluginMockServiceContainer) Has(_ interface{}) bool                 { return true }
func (m *pluginMockServiceContainer) Keys() []string                         { return nil }

type pluginMockEventBus struct{}

func (m *pluginMockEventBus) SubscribeFilter(_ string, _ int, _ events.FilterHandler) string {
	return ""
}
func (m *pluginMockEventBus) SubscribeBroadcast(_ string, _ events.BroadcastHandler) string {
	return ""
}
func (m *pluginMockEventBus) Emit(_ context.Context, _ events.Event) {}
func (m *pluginMockEventBus) DispatchFilter(_ context.Context, event *events.Event) (*events.Event, error) {
	return event, nil
}
func (m *pluginMockEventBus) Unsubscribe(_ string) {}

// pluginMockRegistrar is a mock RouteRegistrar that captures route registrations.
type pluginMockRegistrar struct {
	routes      []string
	middlewares []func(http.Handler) http.Handler
}

func newPluginMockRegistrar() *pluginMockRegistrar {
	return &pluginMockRegistrar{}
}

func (m *pluginMockRegistrar) Handle(pattern string, handler http.Handler) {
	m.routes = append(m.routes, pattern)
}

func (m *pluginMockRegistrar) HandleFunc(pattern string, handler http.HandlerFunc) {
	m.routes = append(m.routes, pattern)
}

func (m *pluginMockRegistrar) Use(middlewares ...func(http.Handler) http.Handler) {
	m.middlewares = append(m.middlewares, middlewares...)
}

func (m *pluginMockRegistrar) Middlewares() []func(http.Handler) http.Handler {
	return m.middlewares
}

func newPluginMockCoreContext(svcs ...interface{}) core.CoreContext {
	return core.NewCoreContext(
		context.Background(),
		newPluginMockServiceContainer(svcs...),
		&pluginMockEventBus{},
		nil,
		slog.Default(),
		"",
		"",
	)
}

// --- Tests ---

func TestPluginNew(t *testing.T) {
	p := New()

	assert.Equal(t, "api", p.Name())
	assert.Equal(t, "1.0.0", p.Version())
	assert.NotNil(t, p.Manifest())
	assert.Equal(t, "REST API plugin with auto-generated CRUD endpoints, OpenAPI 3.0 schema", p.Manifest().Description)
}

func TestPluginInit_Success(t *testing.T) {
	contentSvc := &pluginMockContentService{}
	registrar := newPluginMockRegistrar()

	coreCtx := newPluginMockCoreContext(
		contentSvc,
		registrar,
	)

	p := New()
	err := p.Init(coreCtx)
	require.NoError(t, err)

	assert.Equal(t, contentSvc, p.contentSvc)
	assert.NotNil(t, p.handler)
	assert.False(t, p.running)
	// Routes should have been registered
	assert.NotEmpty(t, registrar.routes)
}

func TestPluginInit_WithAuthService(t *testing.T) {
	contentSvc := &pluginMockContentService{}
	authSvc := &pluginMockAuthService{
		mockAuthService: mockAuthService{
			verifyTokenFunc: func(ctx context.Context, token string) (*interfaces.UserClaims, error) {
				return &interfaces.UserClaims{UserID: "user-1", Email: "test@test.com"}, nil
			},
		},
	}
	registrar := newPluginMockRegistrar()

	coreCtx := newPluginMockCoreContext(contentSvc, registrar, authSvc)

	p := New()
	err := p.Init(coreCtx)
	require.NoError(t, err)

	assert.Equal(t, authSvc, p.authSvc)
}

func TestPluginInit_MissingContentService(t *testing.T) {
	registrar := newPluginMockRegistrar()
	coreCtx := newPluginMockCoreContext(registrar)

	p := New()
	err := p.Init(coreCtx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "content service not available")
}

func TestPluginInit_MissingRegistrar(t *testing.T) {
	contentSvc := &pluginMockContentService{}
	coreCtx := newPluginMockCoreContext(contentSvc)

	p := New()
	err := p.Init(coreCtx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "route registrar not available")
}

func TestPluginStart(t *testing.T) {
	contentSvc := &pluginMockContentService{}
	registrar := newPluginMockRegistrar()
	coreCtx := newPluginMockCoreContext(contentSvc, registrar)

	p := New()
	require.NoError(t, p.Init(coreCtx))

	// First start
	err := p.Start()
	require.NoError(t, err)
	assert.True(t, p.running)

	// Idempotent start
	err = p.Start()
	require.NoError(t, err)
	assert.True(t, p.running)
}

func TestPluginStop(t *testing.T) {
	contentSvc := &pluginMockContentService{}
	registrar := newPluginMockRegistrar()
	coreCtx := newPluginMockCoreContext(contentSvc, registrar)

	p := New()
	require.NoError(t, p.Init(coreCtx))
	require.NoError(t, p.Start())

	// Stop running plugin
	err := p.Stop()
	require.NoError(t, err)
	assert.False(t, p.running)

	// Idempotent stop
	err = p.Stop()
	require.NoError(t, err)
	assert.False(t, p.running)
}

func TestPluginInit_PublicMode_NoAuthService(t *testing.T) {
	contentSvc := &pluginMockContentService{}
	registrar := newPluginMockRegistrar()

	coreCtx := newPluginMockCoreContext(contentSvc, registrar)

	p := New()
	err := p.Init(coreCtx)
	require.NoError(t, err)

	// Auth service should be nil (public mode)
	assert.Nil(t, p.authSvc)
}

func TestPluginLifecycle_FullCycle(t *testing.T) {
	contentSvc := &pluginMockContentService{}
	registrar := newPluginMockRegistrar()
	coreCtx := newPluginMockCoreContext(contentSvc, registrar)

	p := New()

	// Full lifecycle: New → Init → Start → Stop
	require.NoError(t, p.Init(coreCtx))
	require.NoError(t, p.Start())
	require.NoError(t, p.Stop())

	// Verify name and version unchanged
	assert.Equal(t, "api", p.Name())
	assert.Equal(t, "1.0.0", p.Version())
}

func TestRegisterRoutes_SetsUpEndpoints(t *testing.T) {
	contentSvc := &pluginMockContentService{}
	registrar := newPluginMockRegistrar()
	coreCtx := newPluginMockCoreContext(contentSvc, registrar)

	p := New()
	require.NoError(t, p.Init(coreCtx))

	// Verify routes were registered on the mock registrar
	assert.NotEmpty(t, registrar.routes, "expected routes to be registered")
	hasAPIRoute := false
	for _, r := range registrar.routes {
		if len(r) > 0 && r[0] == 'G' {
			hasAPIRoute = true
			break
		}
	}
	assert.True(t, hasAPIRoute, "expected API routes to be registered")
}
