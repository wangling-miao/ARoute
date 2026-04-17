package theme

import (
	"context"
	"database/sql"
	"fmt"
	"html/template"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wangling-miao/aroute/core"
	"github.com/wangling-miao/aroute/core/events"
	"github.com/wangling-miao/aroute/plugins/database"
	"github.com/wangling-miao/aroute/sdk/interfaces"
	"github.com/yuin/gopher-lua"

	_ "modernc.org/sqlite"
)

func setupCoverageTestDB(t *testing.T) (*Store, interfaces.DatabaseService, *sql.DB) {
	t.Helper()
	dbName := fmt.Sprintf("theme_cov_test_%d", time.Now().UnixNano())
	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s?mode=memory&cache=shared&_pragma=foreign_keys(ON)", dbName))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	dbSvc := database.NewService(db, database.DriverSQLite)
	store := NewStore(dbSvc)
	if err := store.CreateTables(context.Background()); err != nil {
		t.Fatalf("create tables: %v", err)
	}
	return store, dbSvc, db
}

func setupCoverageService(t *testing.T) (*Service, *sql.DB) {
	t.Helper()
	store, _, db := setupCoverageTestDB(t)
	eb := &events.EventBus{}
	return NewService(store, eb, slog.Default(), "default"), db
}

func createCoverageTheme(t *testing.T, themesDir, themeName, engine string, templates map[string]string) {
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

func installCoverageTheme(t *testing.T, svc *Service, themesDir, themeName, engine string, templates map[string]string) {
	t.Helper()
	srcDir := t.TempDir()
	createCoverageTheme(t, srcDir, themeName, engine, templates)
	if err := svc.InstallTheme(context.Background(), filepath.Join(srcDir, themeName)); err != nil {
		t.Fatalf("install theme: %v", err)
	}
}

// ==================== Plugin Init/Start/Stop Tests ====================

func TestPlugin_Init_Success(t *testing.T) {
	p := New()

	dbName := fmt.Sprintf("plugin_init_test_%d", time.Now().UnixNano())
	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s?mode=memory&cache=shared&_pragma=foreign_keys(ON)", dbName))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	dbSvc := database.NewService(db, database.DriverSQLite)

	mockCtx := &mockCoreContext{
		services: newMockServiceContainer(dbSvc),
		logger:   slog.Default(),
		ctx:      context.Background(),
		dataDir:  t.TempDir(),
		config:   &mockConfigProvider{values: map[string]string{"active": "default"}},
	}

	if err := p.Init(mockCtx); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
}

func TestPlugin_Init_NoDatabase(t *testing.T) {
	p := New()
	mockCtx := &mockCoreContext{
		services: newMockEmptyContainer(),
		logger:   slog.Default(),
		ctx:      context.Background(),
		dataDir:  t.TempDir(),
		config:   &mockConfigProvider{values: map[string]string{}},
	}
	err := p.Init(mockCtx)
	if err == nil {
		t.Fatal("expected error when database service not available")
	}
	if !strings.Contains(err.Error(), "database service") {
		t.Errorf("expected database service error, got: %v", err)
	}
}

func TestPlugin_StartStop_FullCycle(t *testing.T) {
	p := New()
	dbName := fmt.Sprintf("plugin_cycle_test_%d", time.Now().UnixNano())
	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s?mode=memory&cache=shared&_pragma=foreign_keys(ON)", dbName))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()
	dbSvc := database.NewService(db, database.DriverSQLite)

	mockCtx := &mockCoreContext{
		services: newMockServiceContainer(dbSvc),
		logger:   slog.Default(),
		ctx:      context.Background(),
		dataDir:  t.TempDir(),
		config:   &mockConfigProvider{values: map[string]string{}},
	}

	if err := p.Init(mockCtx); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := p.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := p.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
}

// ==================== Service Coverage Tests ====================

func TestService_Render_LuaEngine(t *testing.T) {
	svc, db := setupCoverageService(t)
	defer db.Close()

	themesDir := t.TempDir()
	createCoverageTheme(t, themesDir, "lua-theme", "lua", map[string]string{
		"post.lua": `return "<h1>" .. (data.title or "no title") .. "</h1>"`,
	})
	svc.LoadThemes(themesDir)

	srcDir := t.TempDir()
	createCoverageTheme(t, srcDir, "lua-theme", "lua", map[string]string{
		"post.lua": `return "<h1>" .. (data.title or "no title") .. "</h1>"`,
	})
	svc.InstallTheme(context.Background(), filepath.Join(srcDir, "lua-theme"))
	svc.SetActiveTheme(context.Background(), "lua-theme")

	result, err := svc.Render(context.Background(), "post.lua", map[string]interface{}{
		"title": "Hello Lua",
	})
	if err != nil {
		t.Fatalf("Render lua: %v", err)
	}
	if !strings.Contains(result, "Hello Lua") {
		t.Errorf("expected 'Hello Lua' in output, got: %s", result)
	}
	svc.Close()
}

func TestService_Render_ReactEngine(t *testing.T) {
	svc, db := setupCoverageService(t)
	defer db.Close()

	themesDir := t.TempDir()
	jsCode := `var data = JSON.parse(typeof __AROUTE_DATA__ !== 'undefined' ? __AROUTE_DATA__ : '{}'); data.title || 'no title';`
	createCoverageTheme(t, themesDir, "react-theme", "react", map[string]string{
		"post.js": jsCode,
	})
	svc.LoadThemes(themesDir)

	srcDir := t.TempDir()
	createCoverageTheme(t, srcDir, "react-theme", "react", map[string]string{
		"post.js": jsCode,
	})
	svc.InstallTheme(context.Background(), filepath.Join(srcDir, "react-theme"))
	svc.SetActiveTheme(context.Background(), "react-theme")

	result, err := svc.Render(context.Background(), "post.html", map[string]interface{}{
		"title": "Hello React",
	})
	if err != nil {
		t.Fatalf("Render react: %v", err)
	}
	if !strings.Contains(result, "Hello React") {
		t.Errorf("expected 'Hello React' in output, got: %s", result)
	}
	svc.Close()
}

func TestService_Render_UnsupportedEngine(t *testing.T) {
	svc, db := setupCoverageService(t)
	defer db.Close()

	themesDir := t.TempDir()
	createCoverageTheme(t, themesDir, "bad-engine", "gotemplate", nil)
	svc.LoadThemes(themesDir)

	_, err := svc.Render(context.Background(), "index.html", map[string]interface{}{
		"title": "Test",
	})
	if err == nil {
		t.Fatal("expected error for unsupported engine or no engine loaded")
	}
}

func TestService_EmitEvent(t *testing.T) {
	svc, db := setupCoverageService(t)
	defer db.Close()

	themesDir := t.TempDir()
	installCoverageTheme(t, svc, themesDir, "evt-theme", "gotemplate", map[string]string{
		"index.html": `<h1>{{.title}}</h1>`,
	})
	svc.SetActiveTheme(context.Background(), "evt-theme")
}

func TestService_Close_WithEngines(t *testing.T) {
	svc, db := setupCoverageService(t)
	defer db.Close()

	themesDir := t.TempDir()
	installCoverageTheme(t, svc, themesDir, "close-theme", "gotemplate", map[string]string{
		"index.html": `<h1>{{.title}}</h1>`,
	})
	svc.SetActiveTheme(context.Background(), "close-theme")
	svc.Close()
}

func TestService_InstallTheme_DuplicateSlug(t *testing.T) {
	svc, db := setupCoverageService(t)
	defer db.Close()

	srcDir := t.TempDir()
	createCoverageTheme(t, srcDir, "dup", "gotemplate", map[string]string{
		"index.html": `<h1>dup</h1>`,
	})
	if err := svc.InstallTheme(context.Background(), filepath.Join(srcDir, "dup")); err != nil {
		t.Fatalf("first install: %v", err)
	}
	err := svc.InstallTheme(context.Background(), filepath.Join(srcDir, "dup"))
	if err == nil {
		t.Fatal("expected error for duplicate slug install")
	}
}

func TestService_LoadThemes_CreateMissingDir(t *testing.T) {
	svc, db := setupCoverageService(t)
	defer db.Close()

	missingDir := filepath.Join(t.TempDir(), "does_not_exist")
	if err := svc.LoadThemes(missingDir); err != nil {
		t.Fatalf("LoadThemes should create missing dir: %v", err)
	}
	if _, err := os.Stat(missingDir); os.IsNotExist(err) {
		t.Fatal("directory was not created")
	}
}

func TestService_ReloadActiveEngineLocked(t *testing.T) {
	svc, db := setupCoverageService(t)
	defer db.Close()

	themesDir := t.TempDir()
	installCoverageTheme(t, svc, themesDir, "reload-theme", "gotemplate", map[string]string{
		"index.html": `<h1>{{.title}}</h1>`,
	})
	svc.SetActiveTheme(context.Background(), "reload-theme")

	_, err := svc.Render(context.Background(), "index.html", map[string]interface{}{"title": "Reloaded"})
	if err != nil {
		t.Fatalf("render after reload: %v", err)
	}
}

// ==================== Engine Go Coverage ====================

func TestGoTemplateEngine_Render_WithFuncs(t *testing.T) {
	themesDir := t.TempDir()
	tmplDir := filepath.Join(themesDir, "funcenv", "templates")
	os.MkdirAll(tmplDir, 0o755)

	tmpl := `{{.title}}|{{formatDate .date "2006"}}|{{truncate .body 10}}|{{asset "css/style.css"}}|{{slugify .slug}}`
	os.WriteFile(filepath.Join(tmplDir, "index.html"), []byte(tmpl), 0o644)

	engine := NewGoTemplateEngine(themesDir, "funcenv", slog.Default())
	result, err := engine.Render("index.html", map[string]interface{}{
		"title": "Test",
		"date":  time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC),
		"body":  "This is a very long body text that should be truncated",
		"slug":  "Hello World!",
	})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(result, "Test") {
		t.Error("missing title")
	}
	if !strings.Contains(result, "2024") {
		t.Error("missing formatted date")
	}
	if !strings.Contains(result, "...") {
		t.Error("missing truncation ellipsis")
	}
	if !strings.Contains(result, "/themes/funcenv/assets/css/style.css") {
		t.Error("missing asset path")
	}
	if !strings.Contains(result, "hello-world") {
		t.Error("missing slugified value")
	}
}

func TestFormatDate_DefaultCase(t *testing.T) {
	result := formatDate(12345, "2006")
	if result != "12345" {
		t.Errorf("expected '12345' for non-time type, got %q", result)
	}
}

func TestTruncate_ExactLength(t *testing.T) {
	result := truncate("hello", 5)
	if result != "hello" {
		t.Errorf("expected 'hello', got %q", result)
	}
}

func TestTruncate_VerySmall(t *testing.T) {
	result := truncate("hello world", 2)
	if result != "he" {
		t.Errorf("expected 'he', got %q", result)
	}
}

func TestMarkdown_CodeBlock(t *testing.T) {
	input := "Hello **bold** and *italic* text"
	result := markdown(input)
	if !strings.Contains(string(result), "<strong>bold</strong>") {
		t.Error("missing bold")
	}
	if !strings.Contains(string(result), "<em>italic</em>") {
		t.Error("missing italic")
	}
	if !strings.Contains(string(result), "<p>") {
		t.Error("missing paragraph wrapper")
	}
}

func TestDict_NonStringKey(t *testing.T) {
	_, err := dict(123, "value")
	if err == nil {
		t.Fatal("expected error for non-string key")
	}
}

func TestBuiltinFunc_Now(t *testing.T) {
	fm := newBuiltinFuncMap("test")
	nowFn, ok := fm["now"]
	if !ok {
		t.Fatal("now function not found")
	}
	result := nowFn.(func() time.Time)()
	if result.IsZero() {
		t.Error("now() returned zero time")
	}
}

func TestBuiltinFunc_Year(t *testing.T) {
	fm := newBuiltinFuncMap("test")
	yearFn, ok := fm["year"]
	if !ok {
		t.Fatal("year function not found")
	}
	result := yearFn.(func() int)()
	if result < 2024 {
		t.Errorf("year() returned unexpected value: %d", result)
	}
}

func TestBuiltinFunc_I18n(t *testing.T) {
	fm := newBuiltinFuncMap("test")
	i18nFn, ok := fm["i18n"]
	if !ok {
		t.Fatal("i18n function not found")
	}
	result := i18nFn.(func(string, ...interface{}) string)("welcome_message")
	if result != "welcome_message" {
		t.Errorf("i18n stub should return key, got %v", result)
	}
}

func TestBuiltinFunc_Raw(t *testing.T) {
	fm := newBuiltinFuncMap("test")
	rawFn, ok := fm["raw"]
	if !ok {
		t.Fatal("raw function not found")
	}
	result := rawFn.(func(string) template.HTML)("<b>bold</b>")
	if result != template.HTML("<b>bold</b>") {
		t.Errorf("raw should not escape, got %v", result)
	}
}

func TestBuiltinFunc_Join(t *testing.T) {
	result, err := join(", ", []string{"a", "b", "c"})
	if err != nil {
		t.Fatalf("join: %v", err)
	}
	if result != "a, b, c" {
		t.Errorf("expected 'a, b, c', got %q", result)
	}

	result2, err := join("-", []interface{}{1, "two", 3.0})
	if err != nil {
		t.Fatalf("join interface: %v", err)
	}
	if result2 != "1-two-3" {
		t.Errorf("expected '1-two-3', got %q", result2)
	}

	_, err = join(",", 123)
	if err == nil {
		t.Error("expected error for non-slice input")
	}
}

// ==================== Lua Engine Coverage ====================

func TestLuaEngine_Render_WithCMSGlobals(t *testing.T) {
	dir := t.TempDir()
	tmplDir := filepath.Join(dir, "mytheme", "templates")
	os.MkdirAll(tmplDir, 0o755)

	luaScript := `
local path = cms.asset("css/style.css")
local url = cms.url("post", {slug = "hello"})
local partial = cms.partial("header", {})
local q = cms.query("posts", {})
return path .. "|" .. url
`
	os.WriteFile(filepath.Join(tmplDir, "post.lua"), []byte(luaScript), 0o644)

	engine := NewLuaEngine(dir, "mytheme", slog.Default(), 2)
	defer engine.Close()

	result, err := engine.Render(context.Background(), "post.lua", map[string]interface{}{
		"title": "Hello",
	})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(result, "/themes/mytheme/assets/css/style.css") {
		t.Errorf("expected asset path in output, got: %s", result)
	}
	if !strings.Contains(result, "/post/hello") {
		t.Errorf("expected url in output, got: %s", result)
	}
}

func TestLuaEngine_Render_EmptyResult(t *testing.T) {
	dir := t.TempDir()
	tmplDir := filepath.Join(dir, "mytheme", "templates")
	os.MkdirAll(tmplDir, 0o755)

	os.WriteFile(filepath.Join(tmplDir, "empty.lua"), []byte(`-- no return value`), 0o644)

	engine := NewLuaEngine(dir, "mytheme", slog.Default(), 2)
	defer engine.Close()

	result, err := engine.Render(context.Background(), "empty.lua", nil)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if result != "" {
		t.Errorf("expected empty result, got: %q", result)
	}
}

func TestLuaEngine_Render_NestedData(t *testing.T) {
	dir := t.TempDir()
	tmplDir := filepath.Join(dir, "datatheme", "templates")
	os.MkdirAll(tmplDir, 0o755)

	luaScript := `return data.user.name .. " is " .. data.user.age .. " years old"`
	os.WriteFile(filepath.Join(tmplDir, "profile.lua"), []byte(luaScript), 0o644)

	engine := NewLuaEngine(dir, "datatheme", slog.Default(), 2)
	defer engine.Close()

	result, err := engine.Render(context.Background(), "profile.lua", map[string]interface{}{
		"user": map[string]interface{}{
			"name": "Alice",
			"age":  30,
		},
	})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(result, "Alice") || !strings.Contains(result, "30") {
		t.Errorf("expected nested data output, got: %s", result)
	}
}

func TestLuaEngine_Render_ArrayData(t *testing.T) {
	dir := t.TempDir()
	tmplDir := filepath.Join(dir, "arrtheme", "templates")
	os.MkdirAll(tmplDir, 0o755)

	luaScript := `return data.items[1] .. "," .. data.items[2] .. "," .. data.items[3]`
	os.WriteFile(filepath.Join(tmplDir, "list.lua"), []byte(luaScript), 0o644)

	engine := NewLuaEngine(dir, "arrtheme", slog.Default(), 2)
	defer engine.Close()

	result, err := engine.Render(context.Background(), "list.lua", map[string]interface{}{
		"items": []interface{}{"x", "y", "z"},
	})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if result != "x,y,z" {
		t.Errorf("expected 'x,y,z', got: %s", result)
	}
}

func TestLuaEngine_Render_HtmlExtensionMapping(t *testing.T) {
	dir := t.TempDir()
	tmplDir := filepath.Join(dir, "exttheme", "templates")
	os.MkdirAll(tmplDir, 0o755)

	luaScript := `return "mapped ok"`
	os.WriteFile(filepath.Join(tmplDir, "page.lua"), []byte(luaScript), 0o644)

	engine := NewLuaEngine(dir, "exttheme", slog.Default(), 2)
	defer engine.Close()

	// Requesting "page.html" should resolve to "page.lua"
	result, err := engine.Render(context.Background(), "page.html", nil)
	if err != nil {
		t.Fatalf("Render with .html extension: %v", err)
	}
	if result != "mapped ok" {
		t.Errorf("expected 'mapped ok', got: %s", result)
	}
}

func TestLuaEngine_Render_BoolAndNilData(t *testing.T) {
	dir := t.TempDir()
	tmplDir := filepath.Join(dir, "niltheme", "templates")
	os.MkdirAll(tmplDir, 0o755)

	luaScript := `
local parts = {}
if data.active == true then parts[#parts+1] = "active:true" end
if data.missing == nil then parts[#parts+1] = "missing:nil" end
return table.concat(parts, "|")
`
	os.WriteFile(filepath.Join(tmplDir, "check.lua"), []byte(luaScript), 0o644)

	engine := NewLuaEngine(dir, "niltheme", slog.Default(), 2)
	defer engine.Close()

	result, err := engine.Render(context.Background(), "check.lua", map[string]interface{}{
		"active": true,
	})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(result, "active:true") {
		t.Error("expected bool data to be injected correctly")
	}
	if !strings.Contains(result, "missing:nil") {
		t.Error("expected nil data to be nil in Lua")
	}
}

func TestLStatePool_PutFull(t *testing.T) {
	pool := NewLStatePool("", "", slog.Default(), 1)
	L := pool.newSandboxedState()
	L2 := pool.newSandboxedState()
	pool.Put(L)
	pool.Put(L2) // overflow — should close L2
	pool.Close()
}

// ==================== React SSR Coverage ====================

func TestReactSSREngine_Render_WithBytecode(t *testing.T) {
	dir := t.TempDir()
	tmplDir := filepath.Join(dir, "mytheme", "templates")
	os.MkdirAll(tmplDir, 0o755)

	jsCode := `var data = JSON.parse(__AROUTE_DATA__); JSON.stringify({title: data.title});`
	os.WriteFile(filepath.Join(tmplDir, "post.js"), []byte(jsCode), 0o644)

	engine := NewReactSSREngine(dir, "mytheme", slog.Default(), 2)
	defer engine.Close()

	result, err := engine.Render("post.html", map[string]interface{}{
		"title": "Bytecode Test",
		"site":  map[string]interface{}{"name": "MySite"},
	})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(result, "Bytecode Test") {
		t.Errorf("expected 'Bytecode Test' in output, got: %s", result)
	}
	if !strings.Contains(result, "__AROUTE_DATA__") {
		t.Error("expected hydration script in output")
	}
}

func TestReactSSREngine_Render_WithHTMLBody(t *testing.T) {
	dir := t.TempDir()
	tmplDir := filepath.Join(dir, "mytheme", "templates")
	os.MkdirAll(tmplDir, 0o755)

	jsCode := `"<html><body><h1>Hello</h1></body></html>";`
	os.WriteFile(filepath.Join(tmplDir, "page.js"), []byte(jsCode), 0o644)

	engine := NewReactSSREngine(dir, "mytheme", slog.Default(), 2)
	defer engine.Close()

	result, err := engine.Render("page.html", map[string]interface{}{
		"page": map[string]interface{}{"title": "Test"},
	})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(result, "<html><body>") {
		t.Error("expected HTML body")
	}
	idx := strings.Index(result, "<script>window.__AROUTE_DATA__")
	if idx == -1 {
		t.Error("hydration script should be before </body>")
	}
}

func TestReactSSREngine_Render_NoBodyTag(t *testing.T) {
	dir := t.TempDir()
	tmplDir := filepath.Join(dir, "mytheme", "templates")
	os.MkdirAll(tmplDir, 0o755)

	jsCode := `"<div>No body tag</div>";`
	os.WriteFile(filepath.Join(tmplDir, "simple.js"), []byte(jsCode), 0o644)

	engine := NewReactSSREngine(dir, "mytheme", slog.Default(), 2)
	defer engine.Close()

	result, err := engine.Render("simple.html", map[string]interface{}{
		"title": "Simple",
	})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(result, "<div>No body tag</div>") {
		t.Error("expected div content")
	}
	if !strings.HasSuffix(result, "</script>") {
		t.Errorf("expected hydration script appended at end, got: %s", result[len(result)-50:])
	}
}

func TestReactSSREngine_Close_Nil(t *testing.T) {
	e := &ReactSSREngine{}
	if err := e.Close(); err != nil {
		t.Fatalf("Close nil pool: %v", err)
	}
}

// ==================== Store Coverage ====================

func TestStore_CreateTables_Error(t *testing.T) {
	store := NewStore(&failingDBService{})
	err := store.CreateTables(context.Background())
	if err == nil {
		t.Fatal("expected error from failing DB")
	}
}

func TestStore_ScanThemeRow(t *testing.T) {
	store, _, db := setupCoverageTestDB(t)
	defer db.Close()

	ctx := context.Background()
	store.Create(ctx, &ThemeRecord{
		Name:    "ScanTest",
		Slug:    "scantest",
		Version: "1.0.0",
		Engine:  "gotemplate",
		Active:  true,
	})

	rows, err := db.QueryContext(ctx, `SELECT id, name, slug, version, engine, active, installed_at, settings FROM _themes WHERE slug = 'scantest'`)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()

	if !rows.Next() {
		t.Fatal("no rows")
	}
	rec, err := store.scanThemeRow(rows)
	if err != nil {
		t.Fatalf("scanThemeRow: %v", err)
	}
	if rec.Name != "ScanTest" {
		t.Errorf("expected ScanTest, got %s", rec.Name)
	}
	if !rec.Active {
		t.Error("expected active=true")
	}
}

// ==================== GoToLuaValue Extra Types ====================

func TestGoToLuaValue_Int64AndFloat32(t *testing.T) {
	L := newStateForTest(t)
	defer L.Close()

	v1 := goToLuaValue(L, int64(42))
	if v1.String() != "42" {
		t.Errorf("int64: expected 42, got %s", v1.String())
	}

	v2 := goToLuaValue(L, float32(3.14))
	// float32 has limited precision; just check it's a number near 3.14
	f2 := lua.LVAsNumber(v2)
	if f2 < 3.13 || f2 > 3.15 {
		t.Errorf("float32: expected ~3.14, got %v", f2)
	}

	v3 := goToLuaValue(L, complex(1, 2))
	if v3.String() == "" {
		t.Error("complex should fallback to Sprint")
	}
}

func TestGoToLuaValue_Bool(t *testing.T) {
	L := newStateForTest(t)
	defer L.Close()

	v := goToLuaValue(L, true)
	if v.String() != "true" {
		t.Errorf("expected 'true', got %s", v.String())
	}
}

func TestGoToLuaValue_Nil(t *testing.T) {
	L := newStateForTest(t)
	defer L.Close()

	v := goToLuaValue(L, nil)
	if v != lua.LNil {
		t.Errorf("expected LNil, got %v", v)
	}
}

func TestGoToLuaValue_String(t *testing.T) {
	L := newStateForTest(t)
	defer L.Close()

	v := goToLuaValue(L, "hello")
	if v.String() != "hello" {
		t.Errorf("expected 'hello', got %s", v.String())
	}
}

func TestGoToLuaValue_Int(t *testing.T) {
	L := newStateForTest(t)
	defer L.Close()

	v := goToLuaValue(L, 42)
	if v.String() != "42" {
		t.Errorf("expected '42', got %s", v.String())
	}
}

func TestGoToLuaValue_Float64(t *testing.T) {
	L := newStateForTest(t)
	defer L.Close()

	v := goToLuaValue(L, 3.14)
	if v.String() != "3.14" {
		t.Errorf("expected '3.14', got %s", v.String())
	}
}

// ==================== CMS URL with string param ====================

func TestLuaEngine_CMSUrl_StringParam(t *testing.T) {
	dir := t.TempDir()
	tmplDir := filepath.Join(dir, "urltheme", "templates")
	os.MkdirAll(tmplDir, 0o755)

	luaScript := `return cms.url("category", "tech")`
	os.WriteFile(filepath.Join(tmplDir, "url.lua"), []byte(luaScript), 0o644)

	engine := NewLuaEngine(dir, "urltheme", slog.Default(), 2)
	defer engine.Close()

	result, err := engine.Render(context.Background(), "url.lua", nil)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if result != "/category/tech" {
		t.Errorf("expected '/category/tech', got: %s", result)
	}
}

func TestLuaEngine_CMSUrl_NoSlug(t *testing.T) {
	dir := t.TempDir()
	tmplDir := filepath.Join(dir, "urltheme2", "templates")
	os.MkdirAll(tmplDir, 0o755)

	luaScript := `return cms.url("posts")`
	os.WriteFile(filepath.Join(tmplDir, "bare.lua"), []byte(luaScript), 0o644)

	engine := NewLuaEngine(dir, "urltheme2", slog.Default(), 2)
	defer engine.Close()

	result, err := engine.Render(context.Background(), "bare.lua", nil)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if result != "/posts" {
		t.Errorf("expected '/posts', got: %s", result)
	}
}

// ==================== FormatDate Coverage ====================

func TestFormatDate_RFC3339String(t *testing.T) {
	result := formatDate("2024-03-15T10:30:00Z", "2006")
	if result != "2024" {
		t.Errorf("expected '2024', got %q", result)
	}
}

func TestFormatDate_DateString(t *testing.T) {
	result := formatDate("2024-03-15", "January")
	if result != "March" {
		t.Errorf("expected 'March', got %q", result)
	}
}

func TestFormatDate_UnparsableString(t *testing.T) {
	result := formatDate("not-a-date", "2006")
	if result != "not-a-date" {
		t.Errorf("expected unparsable string returned as-is, got %q", result)
	}
}

func TestFormatDate_TimeTime(t *testing.T) {
	result := formatDate(time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC), "2006-01-02")
	if result != "2024-06-15" {
		t.Errorf("expected '2024-06-15', got %q", result)
	}
}

// ==================== React injectDataField ====================

func TestReactSSREngine_InjectDataFields(t *testing.T) {
	dir := t.TempDir()
	tmplDir := filepath.Join(dir, "dtheme", "templates")
	os.MkdirAll(tmplDir, 0o755)

	jsCode := `
var site = JSON.parse(__AROUTE_SITE__);
var page = JSON.parse(__AROUTE_PAGE__);
site.name + ":" + page.title;
`
	os.WriteFile(filepath.Join(tmplDir, "fields.js"), []byte(jsCode), 0o644)

	engine := NewReactSSREngine(dir, "dtheme", slog.Default(), 2)
	defer engine.Close()

	result, err := engine.Render("fields.html", map[string]interface{}{
		"site": map[string]interface{}{"name": "MyBlog"},
		"page": map[string]interface{}{"title": "First Post"},
	})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(result, "MyBlog:First Post") {
		t.Errorf("expected field injection, got: %s", result)
	}
}

// ==================== Service edge cases ====================

func TestService_Render_ActiveThemeNotLoaded(t *testing.T) {
	svc, db := setupCoverageService(t)
	defer db.Close()

	// "default" is active but no themes loaded — Render should fail
	_, err := svc.Render(context.Background(), "index.html", nil)
	if err == nil {
		t.Fatal("expected error when active theme not found in map")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected not found error, got: %v", err)
	}
}

func TestService_Close_NilEngines(t *testing.T) {
	svc, db := setupCoverageService(t)
	defer db.Close()
	// Close without setting any active theme — engines are nil
	svc.Close()
}

// ==================== Mock Helpers ====================

// failingDBService implements interfaces.DatabaseService but always returns errors.
type failingDBService struct{}

func (f *failingDBService) Query(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	return nil, fmt.Errorf("db error")
}
func (f *failingDBService) QueryRow(ctx context.Context, query string, args ...interface{}) *sql.Row {
	return nil
}
func (f *failingDBService) Exec(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	return nil, fmt.Errorf("db error")
}
func (f *failingDBService) BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
	return nil, fmt.Errorf("db error")
}
func (f *failingDBService) Ping(ctx context.Context) error { return nil }
func (f *failingDBService) Close() error                   { return nil }
func (f *failingDBService) SchemaIntrospect(ctx context.Context) (*interfaces.DatabaseSchema, error) {
	return nil, fmt.Errorf("not implemented")
}
func (f *failingDBService) Prepare(ctx context.Context, query string) (*sql.Stmt, error) {
	return nil, fmt.Errorf("not implemented")
}

// mockCoreContext implements core.CoreContext for testing plugin Init/Start/Stop.
type mockCoreContext struct {
	services core.ServiceContainer
	logger   *slog.Logger
	ctx      context.Context
	dataDir  string
	config   core.ConfigProvider
	events   core.EventBus
}

func (m *mockCoreContext) Services() core.ServiceContainer { return m.services }
func (m *mockCoreContext) Events() core.EventBus           { return m.events }
func (m *mockCoreContext) Config() core.ConfigProvider     { return m.config }
func (m *mockCoreContext) Logger() *slog.Logger            { return m.logger }
func (m *mockCoreContext) DataDir() string                 { return m.dataDir }
func (m *mockCoreContext) PluginDir() string               { return "" }
func (m *mockCoreContext) Context() context.Context        { return m.ctx }

// mockConfigProvider implements core.ConfigProvider for testing.
type mockConfigProvider struct {
	values map[string]string
}

func (m *mockConfigProvider) GetString(key string) string                    { return m.values[key] }
func (m *mockConfigProvider) GetInt(key string) int                          { return 0 }
func (m *mockConfigProvider) GetBool(key string) bool                        { return false }
func (m *mockConfigProvider) GetStringSlice(key string) []string             { return nil }
func (m *mockConfigProvider) Get(key string) interface{}                     { return m.values[key] }
func (m *mockConfigProvider) Unmarshal(key string, target interface{}) error { return nil }

// mockServiceContainer implements core.ServiceContainer with a database service.
type mockServiceContainer struct {
	dbSvc interfaces.DatabaseService
}

func newMockServiceContainer(dbSvc interfaces.DatabaseService) *mockServiceContainer {
	return &mockServiceContainer{dbSvc: dbSvc}
}

func (m *mockServiceContainer) Provide(provider interface{}) error {
	// Accept any provider — for testing we just ignore it.
	return nil
}

func (m *mockServiceContainer) Get(target interface{}) error {
	if ptr, ok := target.(*interfaces.DatabaseService); ok {
		*ptr = m.dbSvc
		return nil
	}
	return fmt.Errorf("service not found")
}

func (m *mockServiceContainer) GetNamed(name string, target interface{}) error {
	return m.Get(target)
}

func (m *mockServiceContainer) Unregister(target interface{}) error { return nil }
func (m *mockServiceContainer) Has(target interface{}) bool {
	_, ok := target.(*interfaces.DatabaseService)
	return ok
}
func (m *mockServiceContainer) Keys() []string { return []string{"database"} }

// mockEmptyContainer implements core.ServiceContainer with no services.
type mockEmptyContainer struct{}

func newMockEmptyContainer() *mockEmptyContainer { return &mockEmptyContainer{} }

func (m *mockEmptyContainer) Provide(provider interface{}) error { return nil }
func (m *mockEmptyContainer) Get(target interface{}) error {
	return fmt.Errorf("no services available")
}
func (m *mockEmptyContainer) GetNamed(name string, target interface{}) error {
	return fmt.Errorf("no services available")
}
func (m *mockEmptyContainer) Unregister(target interface{}) error { return nil }
func (m *mockEmptyContainer) Has(target interface{}) bool         { return false }
func (m *mockEmptyContainer) Keys() []string                      { return nil }

func newStateForTest(t *testing.T) *lua.LState {
	t.Helper()
	return lua.NewState(lua.Options{SkipOpenLibs: true})
}
