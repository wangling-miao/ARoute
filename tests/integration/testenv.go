package integration

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/wangling-miao/aroute/core"
	"github.com/wangling-miao/aroute/core/engine"
	"github.com/wangling-miao/aroute/core/events"
	"github.com/wangling-miao/aroute/core/license"
	"github.com/wangling-miao/aroute/core/lifecycle"
	"github.com/wangling-miao/aroute/core/loader"
	"github.com/wangling-miao/aroute/core/registry"
	"github.com/wangling-miao/aroute/core/services"
	"github.com/wangling-miao/aroute/plugins/api"
	"github.com/wangling-miao/aroute/plugins/auth"
	"github.com/wangling-miao/aroute/plugins/cache"
	"github.com/wangling-miao/aroute/plugins/content"
	"github.com/wangling-miao/aroute/plugins/database"
	httpplugin "github.com/wangling-miao/aroute/plugins/http"
	"github.com/wangling-miao/aroute/plugins/queue"
	"github.com/wangling-miao/aroute/plugins/search"
	"github.com/wangling-miao/aroute/plugins/theme"
	"github.com/wangling-miao/aroute/plugins/webhook"
	"github.com/wangling-miao/aroute/sdk/interfaces"
)

// testEnv holds the full integration test environment.
type testEnv struct {
	aroute    *core.Aroute
	container *services.Container
	eventBus  *events.EventBus
	viper     *viper.Viper
	tmpDir    string
	logger    *slog.Logger

	adminEmail    string
	adminPassword string
}

// newTestEnv creates a fully initialized ARoute environment for integration testing.
func newTestEnv(t *testing.T) *testEnv {
	t.Helper()

	ctx := context.Background()
	tmpDir := t.TempDir()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelWarn,
	}))

	v := viper.New()
	v.Set("server.host", "127.0.0.1")
	v.Set("server.port", 18080) // Use non-standard port to avoid conflicts
	v.Set("database.driver", "sqlite")
	v.Set("database.path", filepath.Join(tmpDir, "test.db"))
	v.Set("auth.jwt_secret", "test-integration-secret-key-32ch")
	v.Set("auth.access_token_ttl", "1h")
	v.Set("auth.refresh_token_ttl", "24h")
	v.Set("auth.admin.email", "admin@test.aroute.local")
	v.Set("auth.admin.password", "TestAdmin123!")
	v.Set("auth.bcrypt_cost", 4)
	v.Set("theme.active", "default")

	container := services.NewContainer()
	eventBus := events.NewEventBus()

	registryPath := filepath.Join(tmpDir, "registry.db")
	reg, err := registry.NewBoltRegistry(registryPath)
	if err != nil {
		t.Fatalf("create registry: %v", err)
	}

	dispatcher := engine.NewDispatcher()
	licenseValidator := license.NewValidator(nil, nil)

	// Register built-in plugins in the bbolt registry with dependency info
	builtins := map[string]core.Manifest{
		"database": {Name: "database", Version: "1.0.0", Engine: "native"},
		"http":     {Name: "http", Version: "1.0.0", Engine: "native"},
		"auth":     {Name: "auth", Version: "1.0.0", Engine: "native", Requires: []string{"database"}, After: []string{"database", "http"}},
		"content":  {Name: "content", Version: "1.0.0", Engine: "native", Requires: []string{"database"}, After: []string{"database", "auth"}},
		"search":   {Name: "search", Version: "1.0.0", Engine: "native", Requires: []string{"content", "database"}, After: []string{"content", "database"}},
		"theme":    {Name: "theme", Version: "1.0.0", Engine: "native", Requires: []string{"database"}, After: []string{"database", "content"}},
		"cache":    {Name: "cache", Version: "1.0.0", Engine: "native", After: []string{"http"}},
		"queue":    {Name: "queue", Version: "1.0.0", Engine: "native", After: []string{"database", "http"}},
		"webhook":  {Name: "webhook", Version: "1.0.0", Engine: "native", After: []string{"http"}},
		"api":      {Name: "api", Version: "1.0.0", Engine: "native", Requires: []string{"http", "content", "auth"}, After: []string{"http", "content", "auth"}},
	}
	for name, m := range builtins {
		if err := reg.Register(&registry.PluginEntry{
			Manifest: m,
			Enabled:  true,
		}); err != nil {
			t.Fatalf("register plugin %s: %v", name, err)
		}
	}

	ctxFactory := func(pluginCtx context.Context, pluginName string) core.CoreContext {
		pluginLogger := logger.With("plugin", pluginName)
		pluginDataDir := filepath.Join(tmpDir, "plugin_data", pluginName)
		os.MkdirAll(pluginDataDir, 0755)
		// For the theme plugin, set data dir to a location with themes
		if pluginName == "theme" {
			pluginDataDir = filepath.Join(tmpDir, "themes")
			os.MkdirAll(pluginDataDir, 0755)
		}
		return core.NewCoreContext(pluginCtx, container, eventBus, core.NewViperConfig(v), pluginLogger, pluginDataDir, filepath.Join(tmpDir, "plugins"))
	}

	// Copy built-in themes to temp dir so theme plugin can find them
	projectThemesDir := filepath.Join("..", "..", "themes")
	if info, err := os.Stat(projectThemesDir); err == nil && info.IsDir() {
		destThemesDir := filepath.Join(tmpDir, "themes")
		os.MkdirAll(destThemesDir, 0755)
		copyDir(projectThemesDir, destThemesDir)
	}

	pluginLoader := loader.NewNativePluginLoader()
	pluginLoader.Register("http", func() core.Plugin { return httpplugin.New() })
	pluginLoader.Register("database", func() core.Plugin { return database.New() })
	pluginLoader.Register("auth", func() core.Plugin { return auth.New() })
	pluginLoader.Register("content", func() core.Plugin { return content.New() })
	pluginLoader.Register("search", func() core.Plugin { return search.New() })
	pluginLoader.Register("theme", func() core.Plugin { return theme.New() })
	pluginLoader.Register("cache", func() core.Plugin { return cache.New() })
	pluginLoader.Register("queue", func() core.Plugin { return queue.New() })
	pluginLoader.Register("webhook", func() core.Plugin { return webhook.New() })
	pluginLoader.Register("api", func() core.Plugin { return api.New() })

	lifecycleMgr := lifecycle.NewManager(
		&regAdapter{registry: reg},
		&ldrAdapter{loader: pluginLoader},
		eventBus, container, ctxFactory,
	)

	aroute, err := core.New(ctx, container, eventBus,
		&coreRegAdapter{reg: reg},
		lifecycleMgr,
		&coreDispAdapter{d: dispatcher},
		&coreLicAdapter{v: licenseValidator},
		nil,
		core.WithDataDir(tmpDir),
		core.WithPluginDir(filepath.Join(tmpDir, "plugins")),
		core.WithLogger(logger),
	)
	if err != nil {
		t.Fatalf("create aroute: %v", err)
	}

	if err := aroute.Start(ctx); err != nil {
		t.Fatalf("start aroute: %v", err)
	}

	env := &testEnv{
		aroute: aroute, container: container, eventBus: eventBus,
		viper: v, tmpDir: tmpDir, logger: logger,
		adminEmail: "admin@test.aroute.local", adminPassword: "TestAdmin123!",
	}

	t.Cleanup(func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		aroute.Stop(shutdownCtx)
		reg.Close()
	})

	return env
}

func (env *testEnv) authToken(t *testing.T) string {
	t.Helper()
	authSvc := env.getAuthService(t)
	result, err := authSvc.Authenticate(context.Background(), &interfaces.AuthRequest{
		Email:    env.adminEmail,
		Password: env.adminPassword,
	})
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	if result.AccessToken == "" {
		t.Fatal("no access_token in auth result")
	}
	return result.AccessToken
}

func (env *testEnv) getContentService(t *testing.T) interfaces.ContentService {
	t.Helper()
	var svc interfaces.ContentService
	if err := env.container.Get(&svc); err != nil {
		t.Fatalf("get content service: %v", err)
	}
	return svc
}

func (env *testEnv) getAuthService(t *testing.T) interfaces.AuthService {
	t.Helper()
	var svc interfaces.AuthService
	if err := env.container.Get(&svc); err != nil {
		t.Fatalf("get auth service: %v", err)
	}
	return svc
}

func (env *testEnv) getSearchService(t *testing.T) interfaces.SearchService {
	t.Helper()
	var svc interfaces.SearchService
	if err := env.container.Get(&svc); err != nil {
		t.Fatalf("get search service: %v", err)
	}
	return svc
}

func (env *testEnv) getThemeService(t *testing.T) interfaces.ThemeService {
	t.Helper()
	var svc interfaces.ThemeService
	if err := env.container.Get(&svc); err != nil {
		t.Fatalf("get theme service: %v", err)
	}
	return svc
}

func (env *testEnv) getCacheService(t *testing.T) interfaces.CacheService {
	t.Helper()
	var svc interfaces.CacheService
	if err := env.container.Get(&svc); err != nil {
		t.Fatalf("get cache service: %v", err)
	}
	return svc
}

func (env *testEnv) getDatabaseService(t *testing.T) interfaces.DatabaseService {
	t.Helper()
	var svc interfaces.DatabaseService
	if err := env.container.Get(&svc); err != nil {
		t.Fatalf("get database service: %v", err)
	}
	return svc
}

// Adapter types

type regAdapter struct{ registry registry.Registry }

func (a *regAdapter) List() ([]core.Manifest, error) {
	entries, err := a.registry.List()
	if err != nil {
		return nil, err
	}
	ms := make([]core.Manifest, len(entries))
	for i, e := range entries {
		ms[i] = e.Manifest
	}
	return ms, nil
}
func (a *regAdapter) Get(name string) (core.Manifest, error) {
	e, err := a.registry.Get(name)
	if err != nil {
		return core.Manifest{}, err
	}
	return e.Manifest, nil
}
func (a *regAdapter) IsEnabled(name string) (bool, error) {
	e, err := a.registry.Get(name)
	if err != nil {
		return false, err
	}
	return e.Enabled, nil
}
func (a *regAdapter) Enable(name string) error  { return a.registry.Enable(name) }
func (a *regAdapter) Disable(name string) error { return a.registry.Disable(name) }

type ldrAdapter struct{ loader *loader.NativePluginLoader }

func (a *ldrAdapter) Load(m core.Manifest) (core.Plugin, error) { return a.loader.Load(m) }

type coreRegAdapter struct{ reg *registry.BoltRegistry }

func (a *coreRegAdapter) Register(e *core.PluginEntry) error {
	return a.reg.Register(&registry.PluginEntry{Manifest: e.Manifest, Enabled: e.Enabled, DiscoveredPath: e.DiscoveredPath})
}
func (a *coreRegAdapter) Get(name string) (*core.PluginEntry, error) {
	e, err := a.reg.Get(name)
	if err != nil {
		return nil, err
	}
	return &core.PluginEntry{Manifest: e.Manifest, Enabled: e.Enabled, DiscoveredPath: e.DiscoveredPath}, nil
}
func (a *coreRegAdapter) List() ([]*core.PluginEntry, error) {
	es, err := a.reg.List()
	if err != nil {
		return nil, err
	}
	rs := make([]*core.PluginEntry, len(es))
	for i, e := range es {
		rs[i] = &core.PluginEntry{Manifest: e.Manifest, Enabled: e.Enabled, DiscoveredPath: e.DiscoveredPath}
	}
	return rs, nil
}
func (a *coreRegAdapter) Update(name string, m core.Manifest) error { return a.reg.Update(name, m) }
func (a *coreRegAdapter) Remove(name string) error                  { return a.reg.Remove(name) }
func (a *coreRegAdapter) Enable(name string) error                  { return a.reg.Enable(name) }
func (a *coreRegAdapter) Disable(name string) error                 { return a.reg.Disable(name) }
func (a *coreRegAdapter) Close() error                              { return a.reg.Close() }

type coreLicAdapter struct{ v *license.Validator }

func (a *coreLicAdapter) Tier() core.LicenseTier         { return core.LicenseTierOpen }
func (a *coreLicAdapter) IsFeatureAllowed(f string) bool { return a.v.IsFeatureAllowed(f) }
func (a *coreLicAdapter) IsExpired() bool                { return a.v.IsExpired() }
func (a *coreLicAdapter) Validate() error                { return a.v.Validate() }
func (a *coreLicAdapter) LicenseInfo() core.LicenseInfoResult {
	info := a.v.LicenseInfo()
	return core.LicenseInfoResult{Tier: core.LicenseTierOpen, Features: info.Features, ExpiresAt: info.ExpiresAt}
}

type coreDispAdapter struct{ d engine.Dispatcher }

func (a *coreDispAdapter) RegisterEngine(t core.EngineType, e core.EngineExecutor) error {
	return a.d.RegisterEngine(t, nil)
}
func (a *coreDispAdapter) GetEngine(t core.EngineType) (core.EngineExecutor, error) {
	return nil, fmt.Errorf("not found")
}
func (a *coreDispAdapter) Execute(ctx context.Context, p core.Plugin, m *core.Manifest, c core.CoreContext) error {
	return nil
}
func (a *coreDispAdapter) Close() error { return a.d.Close() }

// copyDir recursively copies a directory.
func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0644)
	})
}
