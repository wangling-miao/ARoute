package webhook

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

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
	assert.Equal(t, "webhook", p.Name())
	assert.Equal(t, "1.0.0", p.Version())

	manifest := p.Manifest()
	require.NotNil(t, manifest)
	assert.Equal(t, "webhook", manifest.Name)
	assert.Equal(t, "native", manifest.Engine)
}

func TestPlugin_Init_DefaultConfig(t *testing.T) {
	p := New()
	ctx := newMockCoreContext(&mockConfigProvider{data: map[string]interface{}{}})
	t.Cleanup(func() { p.Stop() })

	err := p.Init(ctx)
	require.NoError(t, err)

	var ws interfaces.WebhookService
	err = ctx.Services().Get(&ws)
	require.NoError(t, err)
	assert.NotNil(t, ws)

	s := p.service
	require.NotNil(t, s)
	assert.Equal(t, 10*time.Second, s.config.DeliveryTimeout)
	assert.Equal(t, 5, s.config.MaxRetries)
	assert.Equal(t, 10, s.config.MaxConsecutiveFailures)
	assert.Equal(t, 30, s.config.DeliveryLogRetentionDays)
}

func TestPlugin_Init_CustomConfig(t *testing.T) {
	p := New()
	cfg := &mockConfigProvider{
		data: map[string]interface{}{
			"delivery_timeout_seconds":    5,
			"max_retries":                 3,
			"max_consecutive_failures":    5,
			"delivery_log_retention_days": 7,
		},
	}
	ctx := newMockCoreContext(cfg)
	t.Cleanup(func() { p.Stop() })

	err := p.Init(ctx)
	require.NoError(t, err)

	s := p.service
	require.NotNil(t, s)
	assert.Equal(t, 5*time.Second, s.config.DeliveryTimeout)
	assert.Equal(t, 3, s.config.MaxRetries)
	assert.Equal(t, 5, s.config.MaxConsecutiveFailures)
	assert.Equal(t, 7, s.config.DeliveryLogRetentionDays)
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

func TestPlugin_Init_RegistersEventSubscription(t *testing.T) {
	p := New()
	ctx := newMockCoreContext(&mockConfigProvider{data: map[string]interface{}{}})
	t.Cleanup(func() { p.Stop() })

	err := p.Init(ctx)
	require.NoError(t, err)

	assert.NotEmpty(t, p.handlerID, "plugin should subscribe to EventBus")

	svc := p.service
	_, err = svc.Create("https://example.com/hook", []string{"**"}, "test-secret")
	require.NoError(t, err)

	ctx.Events().Emit(context.Background(), events.Event{
		Topic: "test.event",
		Data:  map[string]interface{}{"key": "value"},
	})

	assert.Eventually(t, func() bool {
		_, total := svc.GetDeliveries(svc.List()[0].ID, 10, 0)
		return total > 0
	}, 15*time.Second, 200*time.Millisecond, "HandleEvent should have recorded a delivery")
}

func TestPlugin_Init_WiresEventBus(t *testing.T) {
	p := New()
	ctx := newMockCoreContext(&mockConfigProvider{data: map[string]interface{}{}})
	t.Cleanup(func() { p.Stop() })

	err := p.Init(ctx)
	require.NoError(t, err)

	assert.NotNil(t, p.service.eventBus, "service should have EventBus wired")
}

func TestPlugin_Start_StopPruneChannel(t *testing.T) {
	p := New()
	ctx := newMockCoreContext(&mockConfigProvider{data: map[string]interface{}{}})

	err := p.Init(ctx)
	require.NoError(t, err)

	err = p.Start()
	require.NoError(t, err)
	assert.NotNil(t, p.stopPrune, "Start should create stopPrune channel")

	err = p.Stop()
	require.NoError(t, err)
	assert.Nil(t, p.stopPrune, "Stop should nil the stopPrune channel")
}
