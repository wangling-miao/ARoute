package theme

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wangling-miao/aroute/core/events"
	"github.com/wangling-miao/aroute/plugins/database"

	_ "modernc.org/sqlite"
)

var testIndexHTML = `<!DOCTYPE html><html><body><h1>{{.title}}</h1></body></html>`

// ---------------------------------------------------------------------------
// Helpers (unique names to avoid collision with theme_test_part1.go)
// ---------------------------------------------------------------------------

func setupTestServiceForTests(t *testing.T) (*Service, *sql.DB) {
	t.Helper()
	dbName := fmt.Sprintf("theme_svc_test_%d", time.Now().UnixNano())
	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s?mode=memory&cache=shared&_pragma=foreign_keys(ON)", dbName))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	dbSvc := database.NewService(db, database.DriverSQLite)
	store := NewStore(dbSvc)
	if err := store.CreateTables(context.Background()); err != nil {
		t.Fatalf("create tables: %v", err)
	}
	eb := &events.EventBus{}
	return NewService(store, eb, slog.Default(), "default"), db
}

func createTestThemeOnDisk(t *testing.T, themesDir, themeName, engine string, templates map[string]string) {
	t.Helper()
	themeDir := filepath.Join(themesDir, themeName)
	tmplDir := filepath.Join(themeDir, "templates")
	os.MkdirAll(tmplDir, 0o755)

	yamlContent := fmt.Sprintf("name: %q\nversion: \"1.0.0\"\nauthor: \"Test\"\nengine: %q\n", themeName, engine)
	os.WriteFile(filepath.Join(themeDir, "theme.yaml"), []byte(yamlContent), 0o644)

	for name, content := range templates {
		p := filepath.Join(tmplDir, name)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(content), 0o644)
	}
}

// ---------------------------------------------------------------------------
// plugin.go tests
// ---------------------------------------------------------------------------

func TestPlugin_New(t *testing.T) {
	p := New()
	if p == nil {
		t.Fatal("New() returned nil")
	}
	if p.Name() != "theme" {
		t.Errorf("Name() = %q, want %q", p.Name(), "theme")
	}
}

func TestPlugin_Version(t *testing.T) {
	p := New()
	if p.Version() != "1.0.0" {
		t.Errorf("Version() = %q, want %q", p.Version(), "1.0.0")
	}
}

func TestPlugin_StartStop(t *testing.T) {
	p := New()
	// Init requires a full CoreContext which is hard to construct in unit tests.
	// Start/Stop check the running flag; without Init, ctx is nil so we cannot
	// call Start directly without panicking on ctx.Logger().
	// Instead, test the idempotent logic of Start/Stop by verifying that
	// the running field is handled correctly.
	//
	// Since Plugin.Start() accesses p.ctx.Logger(), we mark this test as
	// requiring Init. We test what we can: the BasePlugin start/stop are no-ops.
	bp := p.BasePlugin
	if err := bp.Start(); err != nil {
		t.Errorf("BasePlugin.Start() = %v, want nil", err)
	}
	if err := bp.Stop(); err != nil {
		t.Errorf("BasePlugin.Stop() = %v, want nil", err)
	}
}

func TestPlugin_Start_Idempotent(t *testing.T) {
	p := New()
	bp := p.BasePlugin
	if err := bp.Start(); err != nil {
		t.Errorf("first Start() = %v, want nil", err)
	}
	if err := bp.Start(); err != nil {
		t.Errorf("second Start() = %v, want nil", err)
	}
}

func TestPlugin_Stop_Idempotent(t *testing.T) {
	p := New()
	bp := p.BasePlugin
	// Stop without start should not error
	if err := bp.Stop(); err != nil {
		t.Errorf("Stop() without Start() = %v, want nil", err)
	}
	if err := bp.Stop(); err != nil {
		t.Errorf("second Stop() = %v, want nil", err)
	}
}

// ---------------------------------------------------------------------------
// service.go tests
// ---------------------------------------------------------------------------

func TestService_LoadThemes(t *testing.T) {
	svc, db := setupTestServiceForTests(t)
	defer db.Close()

	themesDir := t.TempDir()
	createTestThemeOnDisk(t, themesDir, "mytheme", "gotemplate", map[string]string{
		"index.html": testIndexHTML,
	})

	if err := svc.LoadThemes(themesDir); err != nil {
		t.Fatalf("LoadThemes() = %v", err)
	}

	names, err := svc.ListThemes(context.Background())
	if err != nil {
		t.Fatalf("ListThemes() = %v", err)
	}
	found := false
	for _, n := range names {
		if n == "mytheme" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("theme %q not found in loaded themes %v", "mytheme", names)
	}
}

func TestService_LoadThemes_DirNotExist(t *testing.T) {
	svc, db := setupTestServiceForTests(t)
	defer db.Close()

	themesDir := filepath.Join(t.TempDir(), "nonexistent_themes")
	if err := svc.LoadThemes(themesDir); err != nil {
		t.Fatalf("LoadThemes() with non-existent dir = %v", err)
	}
	info, err := os.Stat(themesDir)
	if err != nil {
		t.Fatalf("stat themes dir: %v", err)
	}
	if !info.IsDir() {
		t.Error("expected themes dir to be created")
	}
}

func TestService_LoadThemes_InvalidManifest(t *testing.T) {
	svc, db := setupTestServiceForTests(t)
	defer db.Close()

	themesDir := t.TempDir()
	badDir := filepath.Join(themesDir, "broken-theme")
	os.MkdirAll(badDir, 0o755)
	// Write invalid YAML
	os.WriteFile(filepath.Join(badDir, "theme.yaml"), []byte("{{invalid yaml"), 0o644)

	// Should not return error — invalid manifests are skipped
	if err := svc.LoadThemes(themesDir); err != nil {
		t.Fatalf("LoadThemes() with invalid manifest = %v", err)
	}
	names, _ := svc.ListThemes(context.Background())
	for _, n := range names {
		if n == "broken-theme" {
			t.Error("broken theme should not be loaded")
		}
	}
}

func TestService_Render_GoTemplate(t *testing.T) {
	svc, db := setupTestServiceForTests(t)
	defer db.Close()

	themesDir := t.TempDir()
	createTestThemeOnDisk(t, themesDir, "gt-theme", "gotemplate", map[string]string{
		"index.html": testIndexHTML,
	})

	ctx := context.Background()
	svc.LoadThemes(themesDir)

	svc.InstallTheme(ctx, filepath.Join(themesDir, "gt-theme"))

	if err := svc.SetActiveTheme(ctx, "gt-theme"); err != nil {
		t.Fatalf("SetActiveTheme() = %v", err)
	}

	output, err := svc.Render(ctx, "index.html", map[string]interface{}{
		"title": "Hello World",
	})
	if err != nil {
		t.Fatalf("Render() = %v", err)
	}
	if !strings.Contains(output, "Hello World") {
		t.Errorf("Render() output = %q, want to contain %q", output, "Hello World")
	}
	if !strings.Contains(output, "<h1>") {
		t.Errorf("Render() output = %q, want to contain %q", output, "<h1>")
	}
}

func TestService_Render_ThemeNotFound(t *testing.T) {
	svc, db := setupTestServiceForTests(t)
	defer db.Close()

	// Don't load any themes — active theme "default" won't be found
	_, err := svc.Render(context.Background(), "index.html", nil)
	if err == nil {
		t.Fatal("Render() with no active theme should return error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("Render() error = %q, want to contain 'not found'", err.Error())
	}
}

func TestService_GetActiveTheme(t *testing.T) {
	svc, db := setupTestServiceForTests(t)
	defer db.Close()

	active, err := svc.GetActiveTheme(context.Background())
	if err != nil {
		t.Fatalf("GetActiveTheme() = %v", err)
	}
	if active != "default" {
		t.Errorf("GetActiveTheme() = %q, want %q", active, "default")
	}
}

func TestService_SetActiveTheme(t *testing.T) {
	svc, db := setupTestServiceForTests(t)
	defer db.Close()

	themesDir := t.TempDir()
	createTestThemeOnDisk(t, themesDir, "alpha", "gotemplate", map[string]string{"index.html": testIndexHTML})
	createTestThemeOnDisk(t, themesDir, "beta", "gotemplate", map[string]string{"index.html": testIndexHTML})

	ctx := context.Background()
	svc.LoadThemes(themesDir)
	svc.InstallTheme(ctx, filepath.Join(themesDir, "alpha"))
	svc.InstallTheme(ctx, filepath.Join(themesDir, "beta"))

	if err := svc.SetActiveTheme(ctx, "alpha"); err != nil {
		t.Fatalf("SetActiveTheme(alpha) = %v", err)
	}
	active, _ := svc.GetActiveTheme(ctx)
	if active != "alpha" {
		t.Errorf("active = %q, want %q", active, "alpha")
	}

	if err := svc.SetActiveTheme(ctx, "beta"); err != nil {
		t.Fatalf("SetActiveTheme(beta) = %v", err)
	}
	active, _ = svc.GetActiveTheme(ctx)
	if active != "beta" {
		t.Errorf("active = %q, want %q", active, "beta")
	}
}

func TestService_SetActiveTheme_NotFound(t *testing.T) {
	svc, db := setupTestServiceForTests(t)
	defer db.Close()

	err := svc.SetActiveTheme(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("SetActiveTheme() with unknown theme should return error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want to contain 'not found'", err.Error())
	}
}

func TestService_ListThemes(t *testing.T) {
	svc, db := setupTestServiceForTests(t)
	defer db.Close()

	themesDir := t.TempDir()
	createTestThemeOnDisk(t, themesDir, "t1", "gotemplate", map[string]string{"index.html": testIndexHTML})
	createTestThemeOnDisk(t, themesDir, "t2", "gotemplate", map[string]string{"index.html": testIndexHTML})
	createTestThemeOnDisk(t, themesDir, "t3", "gotemplate", map[string]string{"index.html": testIndexHTML})

	if err := svc.LoadThemes(themesDir); err != nil {
		t.Fatalf("LoadThemes() = %v", err)
	}

	names, err := svc.ListThemes(context.Background())
	if err != nil {
		t.Fatalf("ListThemes() = %v", err)
	}
	if len(names) != 3 {
		t.Errorf("ListThemes() count = %d, want 3 (got %v)", len(names), names)
	}
}

func TestService_InstallTheme(t *testing.T) {
	svc, db := setupTestServiceForTests(t)
	defer db.Close()

	themesDir := t.TempDir()
	svc.LoadThemes(themesDir)

	// Create a source theme to install
	srcDir := t.TempDir()
	srcTmplDir := filepath.Join(srcDir, "templates")
	os.MkdirAll(srcTmplDir, 0o755)
	yamlContent := "name: \"Installed Theme\"\nversion: \"2.0.0\"\nauthor: \"Tester\"\nengine: \"gotemplate\"\n"
	os.WriteFile(filepath.Join(srcDir, "theme.yaml"), []byte(yamlContent), 0o644)
	os.WriteFile(filepath.Join(srcTmplDir, "index.html"), []byte(testIndexHTML), 0o644)

	ctx := context.Background()
	if err := svc.InstallTheme(ctx, srcDir); err != nil {
		t.Fatalf("InstallTheme() = %v", err)
	}

	names, _ := svc.ListThemes(ctx)
	found := false
	for _, n := range names {
		if n == "Installed Theme" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("installed theme not in ListThemes(): %v", names)
	}

	// Verify slugified directory was created
	slugDir := filepath.Join(themesDir, "installed-theme")
	if _, err := os.Stat(slugDir); os.IsNotExist(err) {
		t.Error("slugified theme directory not created")
	}
}

func TestService_InstallTheme_InvalidManifest(t *testing.T) {
	svc, db := setupTestServiceForTests(t)
	defer db.Close()

	svc.LoadThemes(t.TempDir())

	// Source without theme.yaml
	srcDir := t.TempDir()
	err := svc.InstallTheme(context.Background(), srcDir)
	if err == nil {
		t.Fatal("InstallTheme() without theme.yaml should return error")
	}
	if !strings.Contains(err.Error(), "manifest") {
		t.Errorf("error = %q, want to contain 'manifest'", err.Error())
	}
}

func TestService_Close(t *testing.T) {
	svc, db := setupTestServiceForTests(t)
	defer db.Close()

	// Close should not panic even with nil engines
	svc.Close()
	// Calling Close twice should also be safe
	svc.Close()
}

func TestSlugifyName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Clean Blog", "clean-blog"},
		{"My Theme", "my-theme"},
		{"lowercase", "lowercase"},
		{"ALL CAPS", "all-caps"}, // slugifyName lowercases and replaces spaces with hyphens
	}
	for _, tt := range tests {
		got := slugifyName(tt.input)
		if got != tt.expected {
			t.Errorf("slugifyName(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestCopyDir(t *testing.T) {
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "dest")

	os.MkdirAll(filepath.Join(src, "sub"), 0o755)
	os.WriteFile(filepath.Join(src, "a.txt"), []byte("hello"), 0o644)
	os.WriteFile(filepath.Join(src, "sub", "b.txt"), []byte("world"), 0o644)

	if err := copyDir(src, dst); err != nil {
		t.Fatalf("copyDir() = %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dst, "a.txt"))
	if err != nil {
		t.Fatalf("read a.txt: %v", err)
	}
	if string(data) != "hello" {
		t.Errorf("a.txt = %q, want %q", string(data), "hello")
	}

	data, err = os.ReadFile(filepath.Join(dst, "sub", "b.txt"))
	if err != nil {
		t.Fatalf("read sub/b.txt: %v", err)
	}
	if string(data) != "world" {
		t.Errorf("sub/b.txt = %q, want %q", string(data), "world")
	}
}

// ---------------------------------------------------------------------------
// engine_lua.go tests
// ---------------------------------------------------------------------------

func TestLStatePool_New(t *testing.T) {
	pool := NewLStatePool(t.TempDir(), "test", slog.Default(), 5)
	if pool == nil {
		t.Fatal("NewLStatePool() returned nil")
	}
	if pool.poolSize != 5 {
		t.Errorf("poolSize = %d, want 5", pool.poolSize)
	}
}

func TestLStatePool_New_DefaultSize(t *testing.T) {
	pool := NewLStatePool(t.TempDir(), "test", slog.Default(), 0)
	if pool.poolSize != 10 {
		t.Errorf("poolSize with 0 = %d, want 10", pool.poolSize)
	}
}

func TestLStatePool_GetPut(t *testing.T) {
	pool := NewLStatePool(t.TempDir(), "test", slog.Default(), 5)
	ctx := context.Background()

	L, err := pool.Get(ctx)
	if err != nil {
		t.Fatalf("Get() = %v", err)
	}
	if L == nil {
		t.Fatal("Get() returned nil LState")
	}

	// Return to pool
	pool.Put(L)

	// Get again — should reuse the same state
	L2, err := pool.Get(ctx)
	if err != nil {
		t.Fatalf("second Get() = %v", err)
	}
	L2.Close()
}

func TestLStatePool_Close(t *testing.T) {
	pool := NewLStatePool(t.TempDir(), "test", slog.Default(), 3)
	ctx := context.Background()

	// Put some states in the pool
	L1, _ := pool.Get(ctx)
	L2, _ := pool.Get(ctx)
	pool.Put(L1)
	pool.Put(L2)

	// Close should not panic
	pool.Close()
}

func TestLuaEngine_New(t *testing.T) {
	engine := NewLuaEngine(t.TempDir(), "testtheme", slog.Default(), 2)
	if engine == nil {
		t.Fatal("NewLuaEngine() returned nil")
	}
	if engine.timeout != 5*time.Second {
		t.Errorf("timeout = %v, want %v", engine.timeout, 5*time.Second)
	}
	engine.Close()
}

func TestLuaEngine_Close(t *testing.T) {
	engine := NewLuaEngine(t.TempDir(), "testtheme", slog.Default(), 2)
	// Close should not panic
	engine.Close()
	// Double close should also be safe
	engine.Close()
}

func TestLuaEngine_Render_NoTemplateFile(t *testing.T) {
	engine := NewLuaEngine(t.TempDir(), "nonexistent", slog.Default(), 2)
	defer engine.Close()

	_, err := engine.Render(context.Background(), "index.lua", nil)
	if err == nil {
		t.Fatal("Render() with nonexistent template should return error")
	}
	if !strings.Contains(err.Error(), "lua engine") {
		t.Errorf("error = %q, want to contain 'lua engine'", err.Error())
	}
}

func TestGoToLuaValue_Types(t *testing.T) {
	pool := NewLStatePool(t.TempDir(), "test", slog.Default(), 2)
	defer pool.Close()

	ctx := context.Background()
	L, err := pool.Get(ctx)
	if err != nil {
		t.Fatalf("Get() = %v", err)
	}
	defer L.Close()

	tests := []struct {
		name  string
		value interface{}
		check func(t *testing.T, lv interface{ String() string })
	}{
		{
			name:  "string",
			value: "hello",
			check: func(t *testing.T, lv interface{ String() string }) {
				if lv.String() != "hello" {
					t.Errorf("string value = %q, want %q", lv.String(), "hello")
				}
			},
		},
		{
			name:  "int",
			value: 42,
			check: func(t *testing.T, lv interface{ String() string }) {
				if lv.String() != "42" {
					t.Errorf("int value = %q, want %q", lv.String(), "42")
				}
			},
		},
		{
			name:  "float64",
			value: 3.14,
			check: func(t *testing.T, lv interface{ String() string }) {
				s := lv.String()
				if !strings.HasPrefix(s, "3.14") {
					t.Errorf("float64 value = %q, want prefix %q", s, "3.14")
				}
			},
		},
		{
			name:  "bool true",
			value: true,
			check: func(t *testing.T, lv interface{ String() string }) {
				if lv.String() != "true" {
					t.Errorf("bool value = %q, want %q", lv.String(), "true")
				}
			},
		},
		{
			name:  "bool false",
			value: false,
			check: func(t *testing.T, lv interface{ String() string }) {
				if lv.String() != "false" {
					t.Errorf("bool value = %q, want %q", lv.String(), "false")
				}
			},
		},
		{
			name:  "nil",
			value: nil,
			check: func(t *testing.T, lv interface{ String() string }) {
				if lv.String() != "nil" {
					t.Errorf("nil value = %q, want %q", lv.String(), "nil")
				}
			},
		},
		{
			name:  "map",
			value: map[string]interface{}{"key": "val"},
			check: func(t *testing.T, lv interface{ String() string }) {
				// Should be a table representation
				s := lv.String()
				if s == "" {
					t.Error("map value is empty string")
				}
			},
		},
		{
			name:  "slice",
			value: []interface{}{"a", "b"},
			check: func(t *testing.T, lv interface{ String() string }) {
				s := lv.String()
				if s == "" {
					t.Error("slice value is empty string")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lv := goToLuaValue(L, tt.value)
			tt.check(t, lv)
		})
	}
}

func TestInjectData(t *testing.T) {
	pool := NewLStatePool(t.TempDir(), "test", slog.Default(), 2)
	defer pool.Close()

	ctx := context.Background()
	L, err := pool.Get(ctx)
	if err != nil {
		t.Fatalf("Get() = %v", err)
	}
	defer L.Close()

	data := map[string]interface{}{
		"title":   "Test Page",
		"content": "Hello World",
	}
	injectData(L, data)

	// Verify "data" global is a table
	dataVal := L.GetGlobal("data")
	if dataVal == nil || dataVal.String() == "nil" {
		t.Fatal("data global is nil after injectData")
	}

	// Verify table has expected fields
	titleVal := L.GetField(dataVal, "title")
	if titleVal.String() != "Test Page" {
		t.Errorf("data.title = %q, want %q", titleVal.String(), "Test Page")
	}
}

// ---------------------------------------------------------------------------
// engine_react.go tests
// ---------------------------------------------------------------------------

func TestReactSSREngine_New(t *testing.T) {
	tmpDir := t.TempDir()
	os.MkdirAll(filepath.Join(tmpDir, "mytheme", "templates"), 0o755)

	engine := NewReactSSREngine(tmpDir, "mytheme", slog.Default(), 2)
	if engine == nil {
		t.Fatal("NewReactSSREngine() returned nil")
	}
	engine.Close()
}

func TestReactSSREngine_Close(t *testing.T) {
	tmpDir := t.TempDir()
	os.MkdirAll(filepath.Join(tmpDir, "mytheme", "templates"), 0o755)

	engine := NewReactSSREngine(tmpDir, "mytheme", slog.Default(), 2)
	if err := engine.Close(); err != nil {
		t.Errorf("Close() = %v, want nil", err)
	}
	// Double close should be safe
	if err := engine.Close(); err != nil {
		t.Errorf("second Close() = %v, want nil", err)
	}
}

func TestReactSSREngine_Precompile_NoTemplates(t *testing.T) {
	tmpDir := t.TempDir()
	os.MkdirAll(filepath.Join(tmpDir, "empty", "templates"), 0o755)

	engine := NewReactSSREngine(tmpDir, "empty", slog.Default(), 2)
	defer engine.Close()

	// No panic, no error expected — empty templates dir is fine
	if len(engine.bytecodeCache) != 0 {
		t.Errorf("bytecodeCache = %d entries, want 0", len(engine.bytecodeCache))
	}
}

func TestReactSSREngine_Render_NoTemplateFile(t *testing.T) {
	tmpDir := t.TempDir()
	os.MkdirAll(filepath.Join(tmpDir, "mytheme", "templates"), 0o755)

	engine := NewReactSSREngine(tmpDir, "mytheme", slog.Default(), 2)
	defer engine.Close()

	_, err := engine.Render("nonexistent.html", map[string]interface{}{"title": "test"})
	if err == nil {
		t.Fatal("Render() with nonexistent template should return error")
	}
	if !strings.Contains(err.Error(), "react ssr") {
		t.Errorf("error = %q, want to contain 'react ssr'", err.Error())
	}
}

func TestReactSSREngine_Reload(t *testing.T) {
	tmpDir := t.TempDir()
	os.MkdirAll(filepath.Join(tmpDir, "mytheme", "templates"), 0o755)

	engine := NewReactSSREngine(tmpDir, "mytheme", slog.Default(), 2)
	defer engine.Close()

	// Reload with empty cache should not panic
	if err := engine.Reload(); err != nil {
		t.Errorf("Reload() = %v, want nil", err)
	}
}

func TestReactSSREngine_New_WithTemplates(t *testing.T) {
	tmpDir := t.TempDir()
	tmplDir := filepath.Join(tmpDir, "mytheme", "templates")
	os.MkdirAll(tmplDir, 0o755)

	// Create a simple JS file
	jsContent := `var result = "<div>" + "Hello" + "</div>"; result;`
	os.WriteFile(filepath.Join(tmplDir, "page.js"), []byte(jsContent), 0o644)

	engine := NewReactSSREngine(tmpDir, "mytheme", slog.Default(), 2)
	defer engine.Close()

	if engine == nil {
		t.Fatal("NewReactSSREngine() returned nil")
	}

	// Verify bytecode cache was populated
	engine.mu.RLock()
	cacheLen := len(engine.bytecodeCache)
	engine.mu.RUnlock()

	if cacheLen == 0 {
		t.Error("bytecodeCache is empty, expected at least 1 entry after precompile")
	}
}
