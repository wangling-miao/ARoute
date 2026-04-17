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

	"github.com/wangling-miao/aroute/plugins/database"
	"github.com/wangling-miao/aroute/sdk/interfaces"
)

// ==================== Test Helpers ====================

func setupTestStore(t *testing.T) (*Store, *sql.DB) {
	t.Helper()
	dbName := fmt.Sprintf("theme_test_%d", time.Now().UnixNano())
	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s?mode=memory&cache=shared&_pragma=foreign_keys(ON)", dbName))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	dbSvc := database.NewService(db, database.DriverSQLite)
	store := NewStore(dbSvc)
	if err := store.CreateTables(context.Background()); err != nil {
		t.Fatalf("create tables: %v", err)
	}
	return store, db
}

func createTestThemeDir(t *testing.T, themeName string, templates map[string]string) string {
	t.Helper()
	themesDir := t.TempDir()
	dir := filepath.Join(themesDir, themeName, "templates")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	for name, content := range templates {
		p := filepath.Join(dir, name)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(content), 0o644)
	}
	return themesDir
}

// Template test data.
var indexHTML = `<!DOCTYPE html><html><body><h1>{{.title}}</h1><p>{{.body}}</p></body></html>`
var singleHTML = `<article><h2>{{.title}}</h2><div>{{.content}}</div></article>`
var funcTemplate = `<p>{{.title}} - {{formatDate .date "2006-01-02"}} - {{truncate .body 20}}</p>`

func writeThemeManifest(t *testing.T, dir, content string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	p := filepath.Join(dir, "theme.yaml")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	return p
}

// ==================== theme_manifest.go Tests ====================

func TestLoadThemeManifest_Valid(t *testing.T) {
	dir := t.TempDir()
	yaml := `name: mytheme
version: "1.0.0"
author: TestAuthor
description: A test theme
engine: gotemplate
aroute_version: "0.1.0"
settings:
  color: blue
`
	p := writeThemeManifest(t, dir, yaml)

	m, err := LoadThemeManifest(p)
	if err != nil {
		t.Fatalf("LoadThemeManifest: %v", err)
	}
	if m.Name != "mytheme" {
		t.Errorf("expected name 'mytheme', got %q", m.Name)
	}
	if m.Version != "1.0.0" {
		t.Errorf("expected version '1.0.0', got %q", m.Version)
	}
	if m.Author != "TestAuthor" {
		t.Errorf("expected author 'TestAuthor', got %q", m.Author)
	}
	if m.Engine != "gotemplate" {
		t.Errorf("expected engine 'gotemplate', got %q", m.Engine)
	}
	if m.ArouteVersion != "0.1.0" {
		t.Errorf("expected aroute_version '0.1.0', got %q", m.ArouteVersion)
	}
	if m.Settings["color"] != "blue" {
		t.Errorf("expected settings.color 'blue', got %v", m.Settings["color"])
	}
}

func TestLoadThemeManifest_MissingFile(t *testing.T) {
	_, err := LoadThemeManifest("/nonexistent/path/theme.yaml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !strings.Contains(err.Error(), "read theme manifest") {
		t.Errorf("expected 'read theme manifest' in error, got: %v", err)
	}
}

func TestLoadThemeManifest_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	p := writeThemeManifest(t, dir, `{{invalid yaml [[`)

	_, err := LoadThemeManifest(p)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
	if !strings.Contains(err.Error(), "parse theme manifest") {
		t.Errorf("expected 'parse theme manifest' in error, got: %v", err)
	}
}

func TestLoadThemeManifest_MissingName(t *testing.T) {
	dir := t.TempDir()
	p := writeThemeManifest(t, dir, `engine: gotemplate`+"\n")

	_, err := LoadThemeManifest(p)
	if err == nil {
		t.Fatal("expected error for missing name")
	}
	if !strings.Contains(err.Error(), "name is required") {
		t.Errorf("expected 'name is required' in error, got: %v", err)
	}
}

func TestLoadThemeManifest_MissingEngine(t *testing.T) {
	dir := t.TempDir()
	p := writeThemeManifest(t, dir, `name: test`+"\n")

	_, err := LoadThemeManifest(p)
	if err == nil {
		t.Fatal("expected error for missing engine")
	}
	if !strings.Contains(err.Error(), "engine is required") {
		t.Errorf("expected 'engine is required' in error, got: %v", err)
	}
}

func TestLoadThemeManifest_InvalidEngine(t *testing.T) {
	dir := t.TempDir()
	p := writeThemeManifest(t, dir, "name: test\nengine: jinja2\n")

	_, err := LoadThemeManifest(p)
	if err == nil {
		t.Fatal("expected error for unsupported engine")
	}
	if !strings.Contains(err.Error(), "unsupported engine") {
		t.Errorf("expected 'unsupported engine' in error, got: %v", err)
	}
}

func TestLoadThemeManifest_ValidEngines(t *testing.T) {
	engines := []string{"gotemplate", "lua", "react"}
	for _, eng := range engines {
		t.Run(eng, func(t *testing.T) {
			dir := t.TempDir()
			yaml := fmt.Sprintf("name: test-%s\nengine: %s\n", eng, eng)
			p := writeThemeManifest(t, dir, yaml)

			m, err := LoadThemeManifest(p)
			if err != nil {
				t.Fatalf("engine %q: %v", eng, err)
			}
			if m.Engine != eng {
				t.Errorf("expected engine %q, got %q", eng, m.Engine)
			}
		})
	}
}

func TestThemeManifest_Validate(t *testing.T) {
	tests := []struct {
		name     string
		manifest ThemeManifest
		wantErr  string
	}{
		{
			name:     "valid",
			manifest: ThemeManifest{Name: "test", Engine: "gotemplate"},
			wantErr:  "",
		},
		{
			name:     "missing name",
			manifest: ThemeManifest{Engine: "gotemplate"},
			wantErr:  "name is required",
		},
		{
			name:     "missing engine",
			manifest: ThemeManifest{Name: "test"},
			wantErr:  "engine is required",
		},
		{
			name:     "bad engine",
			manifest: ThemeManifest{Name: "test", Engine: "pug"},
			wantErr:  "unsupported engine",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.manifest.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("Validate() unexpected error: %v", err)
				}
			} else {
				if err == nil {
					t.Fatalf("Validate() expected error containing %q", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("Validate() error %q, want containing %q", err, tt.wantErr)
				}
			}
		})
	}
}

func TestSupportedEngines(t *testing.T) {
	for _, eng := range []string{"gotemplate", "lua", "react"} {
		if !SupportedEngines[eng] {
			t.Errorf("SupportedEngines[%q] = false, want true", eng)
		}
	}
	for _, eng := range []string{"jinja2", "handlebars", "mustache", ""} {
		if SupportedEngines[eng] {
			t.Errorf("SupportedEngines[%q] = true, want false", eng)
		}
	}
}

// ==================== store.go Tests ====================

func TestStore_CreateTables(t *testing.T) {
	store, db := setupTestStore(t)
	defer db.Close()

	// Idempotent — calling again should not error
	if err := store.CreateTables(context.Background()); err != nil {
		t.Fatalf("create tables idempotent: %v", err)
	}
}

func TestStore_Create(t *testing.T) {
	store, db := setupTestStore(t)
	defer db.Close()
	ctx := context.Background()

	rec := &ThemeRecord{
		Name:    "My Theme",
		Slug:    "my-theme",
		Version: "1.0.0",
		Engine:  "gotemplate",
		Active:  false,
	}
	if err := store.Create(ctx, rec); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if rec.ID == "" {
		t.Error("expected ID to be set")
	}
}

func TestStore_Create_GeneratesID(t *testing.T) {
	store, db := setupTestStore(t)
	defer db.Close()
	ctx := context.Background()

	rec := &ThemeRecord{
		ID:      "",
		Name:    "Auto ID Theme",
		Slug:    "auto-id-theme",
		Version: "1.0.0",
		Engine:  "gotemplate",
	}
	if err := store.Create(ctx, rec); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if rec.ID == "" {
		t.Fatal("expected auto-generated ID")
	}
}

func TestStore_GetBySlug_Found(t *testing.T) {
	store, db := setupTestStore(t)
	defer db.Close()
	ctx := context.Background()

	rec := &ThemeRecord{
		Name:    "Found Theme",
		Slug:    "found-theme",
		Version: "2.0.0",
		Engine:  "lua",
		Active:  true,
	}
	if err := store.Create(ctx, rec); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := store.GetBySlug(ctx, "found-theme")
	if err != nil {
		t.Fatalf("GetBySlug: %v", err)
	}
	if got.Name != "Found Theme" {
		t.Errorf("expected name 'Found Theme', got %q", got.Name)
	}
	if got.Slug != "found-theme" {
		t.Errorf("expected slug 'found-theme', got %q", got.Slug)
	}
	if got.Version != "2.0.0" {
		t.Errorf("expected version '2.0.0', got %q", got.Version)
	}
	if got.Engine != "lua" {
		t.Errorf("expected engine 'lua', got %q", got.Engine)
	}
}

func TestStore_GetBySlug_NotFound(t *testing.T) {
	store, db := setupTestStore(t)
	defer db.Close()

	_, err := store.GetBySlug(context.Background(), "nonexistent-slug")
	if err != interfaces.ErrNotFound {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

func TestStore_GetActive_None(t *testing.T) {
	store, db := setupTestStore(t)
	defer db.Close()

	_, err := store.GetActive(context.Background())
	if err != interfaces.ErrNotFound {
		t.Errorf("expected ErrNotFound when no active theme, got: %v", err)
	}
}

func TestStore_GetActive_Found(t *testing.T) {
	store, db := setupTestStore(t)
	defer db.Close()
	ctx := context.Background()

	rec := &ThemeRecord{
		Name:    "Active Theme",
		Slug:    "active-theme",
		Version: "1.0.0",
		Engine:  "gotemplate",
	}
	if err := store.Create(ctx, rec); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := store.SetActive(ctx, "active-theme"); err != nil {
		t.Fatalf("SetActive: %v", err)
	}

	got, err := store.GetActive(ctx)
	if err != nil {
		t.Fatalf("GetActive: %v", err)
	}
	if got.Slug != "active-theme" {
		t.Errorf("expected slug 'active-theme', got %q", got.Slug)
	}
	if !got.Active {
		t.Error("expected Active=true")
	}
}

func TestStore_List(t *testing.T) {
	store, db := setupTestStore(t)
	defer db.Close()
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		rec := &ThemeRecord{
			Name:    fmt.Sprintf("Theme %d", i),
			Slug:    fmt.Sprintf("theme-%d", i),
			Version: "1.0.0",
			Engine:  "gotemplate",
		}
		if err := store.Create(ctx, rec); err != nil {
			t.Fatalf("Create %d: %v", i, err)
		}
	}

	items, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) < 3 {
		t.Errorf("expected at least 3 items, got %d", len(items))
	}
}

func TestStore_List_Empty(t *testing.T) {
	store, db := setupTestStore(t)
	defer db.Close()

	items, err := store.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("expected empty slice, got %d items", len(items))
	}
}

func TestStore_SetActive(t *testing.T) {
	store, db := setupTestStore(t)
	defer db.Close()
	ctx := context.Background()

	for _, slug := range []string{"theme-a", "theme-b", "theme-c"} {
		rec := &ThemeRecord{
			Name:    slug,
			Slug:    slug,
			Version: "1.0.0",
			Engine:  "gotemplate",
		}
		if err := store.Create(ctx, rec); err != nil {
			t.Fatalf("Create %s: %v", slug, err)
		}
	}

	if err := store.SetActive(ctx, "theme-b"); err != nil {
		t.Fatalf("SetActive: %v", err)
	}

	active, err := store.GetActive(ctx)
	if err != nil {
		t.Fatalf("GetActive: %v", err)
	}
	if active.Slug != "theme-b" {
		t.Errorf("expected active 'theme-b', got %q", active.Slug)
	}

	// Activate a different one, verify only one active
	if err := store.SetActive(ctx, "theme-a"); err != nil {
		t.Fatalf("SetActive theme-a: %v", err)
	}
	active, err = store.GetActive(ctx)
	if err != nil {
		t.Fatalf("GetActive: %v", err)
	}
	if active.Slug != "theme-a" {
		t.Errorf("expected active 'theme-a', got %q", active.Slug)
	}
}

func TestStore_SetActive_NotFound(t *testing.T) {
	store, db := setupTestStore(t)
	defer db.Close()

	err := store.SetActive(context.Background(), "nonexistent")
	if err != interfaces.ErrNotFound {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

func TestStore_Delete(t *testing.T) {
	store, db := setupTestStore(t)
	defer db.Close()
	ctx := context.Background()

	rec := &ThemeRecord{
		Name:    "Delete Me",
		Slug:    "delete-me",
		Version: "1.0.0",
		Engine:  "gotemplate",
	}
	if err := store.Create(ctx, rec); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := store.Delete(ctx, "delete-me"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err := store.GetBySlug(ctx, "delete-me")
	if err != interfaces.ErrNotFound {
		t.Errorf("expected ErrNotFound after delete, got: %v", err)
	}
}

func TestStore_Delete_NotFound(t *testing.T) {
	store, db := setupTestStore(t)
	defer db.Close()

	err := store.Delete(context.Background(), "nonexistent")
	if err != interfaces.ErrNotFound {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

// ==================== engine_go.go Tests ====================

func TestGoTemplateEngine_LoadTemplates(t *testing.T) {
	themesDir := createTestThemeDir(t, "testtheme", map[string]string{
		"index.html": indexHTML,
	})

	e := NewGoTemplateEngine(themesDir, "testtheme", slog.Default())
	if !e.HasTemplate("index.html") {
		t.Error("expected index.html to be loaded")
	}
}

func TestGoTemplateEngine_LoadTemplates_NotFound(t *testing.T) {
	e := &GoTemplateEngine{
		themesDir: "/nonexistent/path",
		themeSlug: "missing",
		funcMap:   newBuiltinFuncMap("missing"),
		logger:    slog.Default(),
	}
	err := e.LoadTemplates()
	if err == nil {
		t.Fatal("expected error for nonexistent directory")
	}
	if !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("expected 'does not exist' in error, got: %v", err)
	}
}

func TestGoTemplateEngine_LoadTemplates_NoTemplates(t *testing.T) {
	themesDir := createTestThemeDir(t, "emptytheme", map[string]string{})

	e := &GoTemplateEngine{
		themesDir: themesDir,
		themeSlug: "emptytheme",
		funcMap:   newBuiltinFuncMap("emptytheme"),
		logger:    slog.Default(),
	}
	err := e.LoadTemplates()
	if err == nil {
		t.Fatal("expected error for empty templates dir")
	}
	if !strings.Contains(err.Error(), "no .html templates found") {
		t.Errorf("expected 'no .html templates found' in error, got: %v", err)
	}
}

func TestGoTemplateEngine_Render_Valid(t *testing.T) {
	themesDir := createTestThemeDir(t, "rendertheme", map[string]string{
		"index.html": indexHTML,
	})

	e := NewGoTemplateEngine(themesDir, "rendertheme", slog.Default())

	out, err := e.Render("index.html", map[string]interface{}{
		"title": "Test",
		"body":  "Hello World",
	})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(out, "Test") {
		t.Errorf("expected 'Test' in output, got: %s", out)
	}
	if !strings.Contains(out, "Hello World") {
		t.Errorf("expected 'Hello World' in output, got: %s", out)
	}
}

func TestGoTemplateEngine_Render_NoTemplatesLoaded(t *testing.T) {
	e := &GoTemplateEngine{
		themesDir: t.TempDir(),
		themeSlug: "notloaded",
		funcMap:   newBuiltinFuncMap("notloaded"),
		logger:    slog.Default(),
	}
	// Don't call LoadTemplates — templates is nil

	_, err := e.Render("index.html", nil)
	if err == nil {
		t.Fatal("expected error when no templates loaded")
	}
	if !strings.Contains(err.Error(), "no templates loaded") {
		t.Errorf("expected 'no templates loaded' in error, got: %v", err)
	}
}

func TestGoTemplateEngine_Render_FallbackSingle(t *testing.T) {
	themesDir := createTestThemeDir(t, "fallback1", map[string]string{
		"single.html": singleHTML,
	})

	e := NewGoTemplateEngine(themesDir, "fallback1", slog.Default())

	out, err := e.Render("post.html", map[string]interface{}{
		"title":   "Post Title",
		"content": "Post Content",
	})
	if err != nil {
		t.Fatalf("Render fallback: %v", err)
	}
	if !strings.Contains(out, "Post Title") {
		t.Errorf("expected 'Post Title' in fallback output, got: %s", out)
	}
}

func TestGoTemplateEngine_Render_FallbackIndex(t *testing.T) {
	themesDir := createTestThemeDir(t, "fallback2", map[string]string{
		"index.html": indexHTML,
	})

	e := NewGoTemplateEngine(themesDir, "fallback2", slog.Default())

	out, err := e.Render("post.html", map[string]interface{}{
		"title": "Fallback",
		"body":  "Falls to index",
	})
	if err != nil {
		t.Fatalf("Render fallback to index: %v", err)
	}
	if !strings.Contains(out, "Fallback") {
		t.Errorf("expected 'Fallback' in output, got: %s", out)
	}
}

func TestGoTemplateEngine_HasTemplate_True(t *testing.T) {
	themesDir := createTestThemeDir(t, "hastmpl", map[string]string{
		"index.html": indexHTML,
	})

	e := NewGoTemplateEngine(themesDir, "hastmpl", slog.Default())
	if !e.HasTemplate("index.html") {
		t.Error("expected HasTemplate('index.html') = true")
	}
}

func TestGoTemplateEngine_HasTemplate_False(t *testing.T) {
	themesDir := createTestThemeDir(t, "nomatch", map[string]string{
		"index.html": indexHTML,
	})

	e := NewGoTemplateEngine(themesDir, "nomatch", slog.Default())
	if e.HasTemplate("nonexistent.html") {
		t.Error("expected HasTemplate('nonexistent.html') = false")
	}
}

func TestGoTemplateEngine_Reload(t *testing.T) {
	themesDir := createTestThemeDir(t, "reload", map[string]string{
		"index.html": "<p>original</p>",
	})

	e := NewGoTemplateEngine(themesDir, "reload", slog.Default())

	out, err := e.Render("index.html", nil)
	if err != nil {
		t.Fatalf("Render original: %v", err)
	}
	if !strings.Contains(out, "original") {
		t.Errorf("expected 'original' in first render, got: %s", out)
	}

	// Modify the template file
	tmplPath := filepath.Join(themesDir, "reload", "templates", "index.html")
	if err := os.WriteFile(tmplPath, []byte("<p>updated</p>"), 0o644); err != nil {
		t.Fatalf("write updated template: %v", err)
	}

	if err := e.Reload(); err != nil {
		t.Fatalf("Reload: %v", err)
	}

	out, err = e.Render("index.html", nil)
	if err != nil {
		t.Fatalf("Render after reload: %v", err)
	}
	if !strings.Contains(out, "updated") {
		t.Errorf("expected 'updated' after reload, got: %s", out)
	}
}

// ==================== Built-in Function Tests ====================

func TestFormatDate_Time(t *testing.T) {
	now := time.Date(2024, 6, 15, 10, 30, 0, 0, time.UTC)
	got := formatDate(now, "2006-01-02")
	if got != "2024-06-15" {
		t.Errorf("formatDate(time, '2006-01-02') = %q, want '2024-06-15'", got)
	}
}

func TestFormatDate_String(t *testing.T) {
	got := formatDate("2024-06-15T10:30:00Z", "2006-01-02")
	if got != "2024-06-15" {
		t.Errorf("formatDate(RFC3339 string) = %q, want '2024-06-15'", got)
	}
}

func TestFormatDate_EmptyString(t *testing.T) {
	got := formatDate("", "2006-01-02")
	if got != "" {
		t.Errorf("formatDate('', layout) = %q, want ''", got)
	}
}

func TestFormatDate_Nil(t *testing.T) {
	got := formatDate(nil, "2006-01-02")
	if got != "" {
		t.Errorf("formatDate(nil, layout) = %q, want ''", got)
	}
}

func TestTruncate_Short(t *testing.T) {
	got := truncate("hello", 10)
	if got != "hello" {
		t.Errorf("truncate('hello', 10) = %q, want 'hello'", got)
	}
}

func TestTruncate_Long(t *testing.T) {
	got := truncate("abcdefghijklmnopqrstuvwxyz", 10)
	want := "abcdefg..."
	if got != want {
		t.Errorf("truncate(26chars, 10) = %q, want %q", got, want)
	}
}

func TestTruncate_Negative(t *testing.T) {
	s := "return as-is"
	got := truncate(s, -1)
	if got != s {
		t.Errorf("truncate(%q, -1) = %q, want %q", s, got, s)
	}
}

func TestSlugify(t *testing.T) {
	got := slugify("Hello World!")
	if got != "hello-world" {
		t.Errorf("slugify('Hello World!') = %q, want 'hello-world'", got)
	}
}

func TestSlugify_SpecialChars(t *testing.T) {
	got := slugify("Hello @World# 2024!!!")
	if got != "hello-world-2024" {
		t.Errorf("slugify with special chars = %q, want 'hello-world-2024'", got)
	}
}

func TestMarkdown(t *testing.T) {
	got := markdown("**bold** and *italic*")
	gotStr := string(got)
	if !strings.Contains(gotStr, "<strong>bold</strong>") {
		t.Errorf("expected <strong>bold</strong> in output, got: %s", gotStr)
	}
	if !strings.Contains(gotStr, "<em>italic</em>") {
		t.Errorf("expected <em>italic</em> in output, got: %s", gotStr)
	}
	if !strings.HasPrefix(gotStr, "<p>") {
		t.Errorf("expected output to start with <p>, got: %s", gotStr)
	}
}

func TestDict(t *testing.T) {
	m, err := dict("key1", "val1", "key2", 42)
	if err != nil {
		t.Fatalf("dict: %v", err)
	}
	if m["key1"] != "val1" {
		t.Errorf("expected key1=val1, got %v", m["key1"])
	}
	if m["key2"] != 42 {
		t.Errorf("expected key2=42, got %v", m["key2"])
	}
}

func TestDict_OddArgs(t *testing.T) {
	_, err := dict("key1", "val1", "key2")
	if err == nil {
		t.Fatal("expected error for odd number of arguments")
	}
	if !strings.Contains(err.Error(), "even number") {
		t.Errorf("expected 'even number' in error, got: %v", err)
	}
}

func TestJoin_Strings(t *testing.T) {
	got, err := join(", ", []string{"a", "b", "c"})
	if err != nil {
		t.Fatalf("join strings: %v", err)
	}
	if got != "a, b, c" {
		t.Errorf("join(', ', []string) = %q, want 'a, b, c'", got)
	}
}

func TestJoin_Interfaces(t *testing.T) {
	got, err := join("-", []interface{}{"x", 1, true})
	if err != nil {
		t.Fatalf("join interfaces: %v", err)
	}
	if got != "x-1-true" {
		t.Errorf("join('-', []interface{}) = %q, want 'x-1-true'", got)
	}
}

func TestJoin_InvalidType(t *testing.T) {
	_, err := join(", ", 123)
	if err == nil {
		t.Fatal("expected error for invalid type")
	}
	if !strings.Contains(err.Error(), "expects []string or []interface{}") {
		t.Errorf("expected type error, got: %v", err)
	}
}

func TestBuiltinFunc_Asset(t *testing.T) {
	fm := newBuiltinFuncMap("mytheme")
	assetFn, ok := fm["asset"].(func(string) string)
	if !ok {
		t.Fatal("asset function not found in FuncMap")
	}
	got := assetFn("css/style.css")
	want := "/themes/mytheme/assets/css/style.css"
	if got != want {
		t.Errorf("asset('css/style.css') = %q, want %q", got, want)
	}
}

func TestBuiltinFunc_Default(t *testing.T) {
	fm := newBuiltinFuncMap("test")
	defaultFn, ok := fm["default"].(func(interface{}, interface{}) interface{})
	if !ok {
		t.Fatal("default function not found in FuncMap")
	}

	// val is set → returns val
	got := defaultFn("fallback", "actual")
	if got != "actual" {
		t.Errorf("default('fallback', 'actual') = %v, want 'actual'", got)
	}

	// val is empty string → returns default
	got = defaultFn("fallback", "")
	if got != "fallback" {
		t.Errorf("default('fallback', '') = %v, want 'fallback'", got)
	}

	// val is nil → returns default
	got = defaultFn("fallback", nil)
	if got != "fallback" {
		t.Errorf("default('fallback', nil) = %v, want 'fallback'", got)
	}
}

func TestBuiltinFunc_SafeHTML(t *testing.T) {
	fm := newBuiltinFuncMap("test")
	safeFn, ok := fm["safeHTML"].(func(string) template.HTML)
	if !ok {
		t.Fatal("safeHTML function not found in FuncMap")
	}

	input := "<script>alert('xss')</script>"
	got := safeFn(input)
	if string(got) != input {
		t.Errorf("safeHTML(%q) = %q, want %q (no escaping)", input, string(got), input)
	}
}
