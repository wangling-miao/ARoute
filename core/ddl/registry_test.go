package ddl

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"

	"github.com/wangling-miao/aroute/sdk/interfaces"
	_ "modernc.org/sqlite"
)

type testDB struct {
	db *sql.DB
}

func newTestDB(t *testing.T) *testDB {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}

	return &testDB{db: db}
}

func (tdb *testDB) Close() error {
	return tdb.db.Close()
}

func (tdb *testDB) Exec(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	return tdb.db.ExecContext(ctx, query, args...)
}

func (tdb *testDB) Query(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	return tdb.db.QueryContext(ctx, query, args...)
}

func (tdb *testDB) QueryRow(ctx context.Context, query string, args ...interface{}) *sql.Row {
	return tdb.db.QueryRowContext(ctx, query, args...)
}

func (tdb *testDB) BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
	return tdb.db.BeginTx(ctx, opts)
}

func (tdb *testDB) Ping(ctx context.Context) error {
	return tdb.db.PingContext(ctx)
}

func (tdb *testDB) Prepare(ctx context.Context, query string) (*sql.Stmt, error) {
	return tdb.db.PrepareContext(ctx, query)
}

func (tdb *testDB) SchemaIntrospect(ctx context.Context) (*interfaces.DatabaseSchema, error) {
	return nil, fmt.Errorf("not implemented for test")
}

func TestRegistry_Init(t *testing.T) {
	tdb := newTestDB(t)
	defer tdb.Close()

	registry := NewRegistry(tdb)

	err := registry.Init(context.Background())
	if err != nil {
		t.Fatalf("Registry.Init() error = %v", err)
	}

	var count int
	err = tdb.QueryRow(context.Background(),
		"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='_content_types'").Scan(&count)
	if err != nil {
		t.Fatalf("failed to check table existence: %v", err)
	}

	if count != 1 {
		t.Errorf("_content_types table not created, count = %d", count)
	}
}

func TestRegistry_Create(t *testing.T) {
	tdb := newTestDB(t)
	defer tdb.Close()

	registry := NewRegistry(tdb)
	if err := registry.Init(context.Background()); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	schema := &Schema{
		Name: "posts",
		Fields: []FieldDefinition{
			{Name: "title", Type: FieldTypeText},
			{Name: "body", Type: FieldTypeText},
		},
	}

	err := registry.Create(context.Background(), schema)
	if err != nil {
		t.Fatalf("Registry.Create() error = %v", err)
	}

	var name string
	err = tdb.QueryRow(context.Background(),
		"SELECT name FROM _content_types WHERE name = ?", "posts").Scan(&name)
	if err != nil {
		t.Fatalf("failed to query schema: %v", err)
	}

	if name != "posts" {
		t.Errorf("stored name = %q, want %q", name, "posts")
	}
}

func TestRegistry_Create_Duplicate(t *testing.T) {
	tdb := newTestDB(t)
	defer tdb.Close()

	registry := NewRegistry(tdb)
	if err := registry.Init(context.Background()); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	schema := &Schema{
		Name: "posts",
		Fields: []FieldDefinition{
			{Name: "title", Type: FieldTypeText},
		},
	}

	if err := registry.Create(context.Background(), schema); err != nil {
		t.Fatalf("first Create() error = %v", err)
	}

	err := registry.Create(context.Background(), schema)
	if err == nil {
		t.Error("second Create() should fail for duplicate content type")
	}
}

func TestRegistry_Get(t *testing.T) {
	tdb := newTestDB(t)
	defer tdb.Close()

	registry := NewRegistry(tdb)
	if err := registry.Init(context.Background()); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	original := &Schema{
		Name: "posts",
		Fields: []FieldDefinition{
			{Name: "title", Type: FieldTypeText},
			{Name: "body", Type: FieldTypeText},
		},
	}

	if err := registry.Create(context.Background(), original); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	retrieved, err := registry.Get(context.Background(), "posts")
	if err != nil {
		t.Fatalf("Registry.Get() error = %v", err)
	}

	if retrieved.Name != original.Name {
		t.Errorf("retrieved.Name = %q, want %q", retrieved.Name, original.Name)
	}

	if len(retrieved.Fields) != len(original.Fields) {
		t.Errorf("len(retrieved.Fields) = %d, want %d", len(retrieved.Fields), len(original.Fields))
	}
}

func TestRegistry_Get_NotFound(t *testing.T) {
	tdb := newTestDB(t)
	defer tdb.Close()

	registry := NewRegistry(tdb)
	if err := registry.Init(context.Background()); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	_, err := registry.Get(context.Background(), "nonexistent")
	if err == nil {
		t.Error("Registry.Get() should return error for non-existent content type")
	}
}

func TestRegistry_Update(t *testing.T) {
	tdb := newTestDB(t)
	defer tdb.Close()

	registry := NewRegistry(tdb)
	if err := registry.Init(context.Background()); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	original := &Schema{
		Name: "posts",
		Fields: []FieldDefinition{
			{Name: "title", Type: FieldTypeText},
		},
	}

	if err := registry.Create(context.Background(), original); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	updated := &Schema{
		Name: "posts",
		Fields: []FieldDefinition{
			{Name: "title", Type: FieldTypeText},
			{Name: "body", Type: FieldTypeText},
		},
	}

	err := registry.Update(context.Background(), updated)
	if err != nil {
		t.Fatalf("Registry.Update() error = %v", err)
	}

	retrieved, err := registry.Get(context.Background(), "posts")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if len(retrieved.Fields) != 2 {
		t.Errorf("len(retrieved.Fields) = %d, want 2", len(retrieved.Fields))
	}
}

func TestRegistry_Delete(t *testing.T) {
	tdb := newTestDB(t)
	defer tdb.Close()

	registry := NewRegistry(tdb)
	if err := registry.Init(context.Background()); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	schema := &Schema{
		Name: "posts",
		Fields: []FieldDefinition{
			{Name: "title", Type: FieldTypeText},
		},
	}

	if err := registry.Create(context.Background(), schema); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	err := registry.Delete(context.Background(), "posts", false)
	if err == nil {
		t.Error("Registry.Delete() should require force=true")
	}

	err = registry.Delete(context.Background(), "posts", true)
	if err != nil {
		t.Fatalf("Registry.Delete() error = %v", err)
	}

	_, err = registry.Get(context.Background(), "posts")
	if err == nil {
		t.Error("Get() should return error after deletion")
	}
}

func TestRegistry_List(t *testing.T) {
	tdb := newTestDB(t)
	defer tdb.Close()

	registry := NewRegistry(tdb)
	if err := registry.Init(context.Background()); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	schemas := []*Schema{
		{Name: "posts", Fields: []FieldDefinition{{Name: "title", Type: FieldTypeText}}},
		{Name: "users", Fields: []FieldDefinition{{Name: "email", Type: FieldTypeText}}},
		{Name: "comments", Fields: []FieldDefinition{{Name: "body", Type: FieldTypeText}}},
	}

	for _, s := range schemas {
		if err := registry.Create(context.Background(), s); err != nil {
			t.Fatalf("Create(%s) error = %v", s.Name, err)
		}
	}

	list, err := registry.List(context.Background())
	if err != nil {
		t.Fatalf("Registry.List() error = %v", err)
	}

	if len(list) != 3 {
		t.Errorf("len(list) = %d, want 3", len(list))
	}
}

func TestRegistry_Exists(t *testing.T) {
	tdb := newTestDB(t)
	defer tdb.Close()

	registry := NewRegistry(tdb)
	if err := registry.Init(context.Background()); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	exists, err := registry.Exists(context.Background(), "posts")
	if err != nil {
		t.Fatalf("Exists() error = %v", err)
	}
	if exists {
		t.Error("Exists() should return false for non-existent content type")
	}

	schema := &Schema{
		Name:   "posts",
		Fields: []FieldDefinition{{Name: "title", Type: FieldTypeText}},
	}
	if err := registry.Create(context.Background(), schema); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	exists, err = registry.Exists(context.Background(), "posts")
	if err != nil {
		t.Fatalf("Exists() error = %v", err)
	}
	if !exists {
		t.Error("Exists() should return true for existing content type")
	}
}

func TestRegistry_Update_NotFound(t *testing.T) {
	tdb := newTestDB(t)
	defer tdb.Close()

	registry := NewRegistry(tdb)
	if err := registry.Init(context.Background()); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	updated := &Schema{
		Name:   "nonexistent",
		Fields: []FieldDefinition{{Name: "title", Type: FieldTypeText}},
	}

	err := registry.Update(context.Background(), updated)
	if err == nil {
		t.Error("Update() should return error for non-existent content type")
	}
}

func TestRegistry_Update_InvalidSchema(t *testing.T) {
	tdb := newTestDB(t)
	defer tdb.Close()

	registry := NewRegistry(tdb)
	if err := registry.Init(context.Background()); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	err := registry.Update(context.Background(), &Schema{Name: "", Fields: nil})
	if err == nil {
		t.Error("Update() should return error for invalid schema")
	}
}

func TestRegistry_Delete_NotFound(t *testing.T) {
	tdb := newTestDB(t)
	defer tdb.Close()

	registry := NewRegistry(tdb)
	if err := registry.Init(context.Background()); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	err := registry.Delete(context.Background(), "nonexistent", true)
	if err == nil {
		t.Error("Delete() should return error for non-existent content type")
	}
}

func TestRegistry_Delete_WithoutForce(t *testing.T) {
	tdb := newTestDB(t)
	defer tdb.Close()

	registry := NewRegistry(tdb)
	if err := registry.Init(context.Background()); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	schema := &Schema{
		Name:   "posts",
		Fields: []FieldDefinition{{Name: "title", Type: FieldTypeText}},
	}
	if err := registry.Create(context.Background(), schema); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	err := registry.Delete(context.Background(), "posts", false)
	if err == nil {
		t.Error("Delete() without force should return error")
	}
	if err != nil && !strings.Contains(err.Error(), "force=true") {
		t.Errorf("error should mention force=true, got: %v", err)
	}
}

func TestRegistry_Create_InvalidSchema(t *testing.T) {
	tdb := newTestDB(t)
	defer tdb.Close()

	registry := NewRegistry(tdb)
	if err := registry.Init(context.Background()); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	err := registry.Create(context.Background(), &Schema{Name: "", Fields: nil})
	if err == nil {
		t.Error("Create() should return error for invalid schema")
	}

	err = registry.Create(context.Background(), &Schema{Name: "test", Fields: []FieldDefinition{}})
	if err == nil {
		t.Error("Create() should return error for empty fields")
	}

	err = registry.Create(context.Background(), &Schema{Name: "test", Fields: []FieldDefinition{
		{Name: "f1", Type: "invalid_type"},
	}})
	if err == nil {
		t.Error("Create() should return error for invalid field type")
	}
}

func TestRegistry_List_Empty(t *testing.T) {
	tdb := newTestDB(t)
	defer tdb.Close()

	registry := NewRegistry(tdb)
	if err := registry.Init(context.Background()); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	list, err := registry.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(list) != 0 {
		t.Errorf("List() should return empty for empty registry, got %d items", len(list))
	}
}

func TestRegistry_Update_WithMultipleFields(t *testing.T) {
	tdb := newTestDB(t)
	defer tdb.Close()

	registry := NewRegistry(tdb)
	if err := registry.Init(context.Background()); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	original := &Schema{
		Name:   "posts",
		Fields: []FieldDefinition{{Name: "title", Type: FieldTypeText}},
	}
	if err := registry.Create(context.Background(), original); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	updated := &Schema{
		Name: "posts",
		Fields: []FieldDefinition{
			{Name: "title", Type: FieldTypeText},
			{Name: "body", Type: FieldTypeText},
			{Name: "published", Type: FieldTypeBoolean},
		},
	}
	if err := registry.Update(context.Background(), updated); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	retrieved, err := registry.Get(context.Background(), "posts")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if len(retrieved.Fields) != 3 {
		t.Errorf("len(retrieved.Fields) = %d, want 3", len(retrieved.Fields))
	}
}

type registryFailDB struct {
	*sql.DB
	failExec     bool
	failQuery    bool
	failQueryRow bool
}

func (f *registryFailDB) Exec(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	if f.failExec {
		return nil, fmt.Errorf("injected exec failure")
	}
	return f.DB.ExecContext(ctx, query, args...)
}

func (f *registryFailDB) Query(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	if f.failQuery {
		return nil, fmt.Errorf("injected query failure")
	}
	return f.DB.QueryContext(ctx, query, args...)
}

func (f *registryFailDB) QueryRow(ctx context.Context, query string, args ...interface{}) *sql.Row {
	if f.failQueryRow {
		return f.DB.QueryRowContext(ctx, "SELECT schema_json, created_at, updated_at FROM _nonexistent WHERE name = ?", args...)
	}
	return f.DB.QueryRowContext(ctx, query, args...)
}

func (f *registryFailDB) BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
	return f.DB.BeginTx(ctx, opts)
}

func (f *registryFailDB) Ping(ctx context.Context) error {
	return f.DB.PingContext(ctx)
}

func (f *registryFailDB) Prepare(ctx context.Context, query string) (*sql.Stmt, error) {
	return f.DB.PrepareContext(ctx, query)
}

func (f *registryFailDB) Close() error {
	return f.DB.Close()
}

func (f *registryFailDB) SchemaIntrospect(ctx context.Context) (*interfaces.DatabaseSchema, error) {
	return nil, fmt.Errorf("not implemented")
}

func TestRegistry_Init_ExecFail(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	failDB := &registryFailDB{DB: db, failExec: true}
	registry := NewRegistry(failDB)

	err = registry.Init(context.Background())
	if err == nil {
		t.Fatal("expected error when Exec fails, got nil")
	}
	if !strings.Contains(err.Error(), "creating _content_types table") {
		t.Errorf("error should mention table creation, got: %v", err)
	}
}

func TestRegistry_List_QueryFail(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	realDB := &testDB{db: db}
	registry := NewRegistry(realDB)
	if err := registry.Init(context.Background()); err != nil {
		t.Fatalf("Init: %v", err)
	}

	failDB := &registryFailDB{DB: db, failQuery: true}
	failRegistry := NewRegistry(failDB)

	_, err = failRegistry.List(context.Background())
	if err == nil {
		t.Fatal("expected error when Query fails, got nil")
	}
	if !strings.Contains(err.Error(), "querying schemas") {
		t.Errorf("error should mention querying schemas, got: %v", err)
	}
}

func TestRegistry_Exists_QueryRowFail(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	realDB := &testDB{db: db}
	registry := NewRegistry(realDB)
	if err := registry.Init(context.Background()); err != nil {
		t.Fatalf("Init: %v", err)
	}

	failDB := &registryFailDB{DB: db, failQueryRow: true}
	failRegistry := NewRegistry(failDB)

	_, err = failRegistry.Exists(context.Background(), "posts")
	if err == nil {
		t.Fatal("expected error when QueryRow fails, got nil")
	}
	if !strings.Contains(err.Error(), "checking schema existence") {
		t.Errorf("error should mention checking schema existence, got: %v", err)
	}
}

func TestRegistry_Get_QueryRowFail(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	realDB := &testDB{db: db}
	registry := NewRegistry(realDB)
	if err := registry.Init(context.Background()); err != nil {
		t.Fatalf("Init: %v", err)
	}

	failDB := &registryFailDB{DB: db, failQueryRow: true}
	failRegistry := NewRegistry(failDB)

	_, err = failRegistry.Get(context.Background(), "posts")
	if err == nil {
		t.Fatal("expected error when QueryRow fails, got nil")
	}
	if !strings.Contains(err.Error(), "querying schema") {
		t.Errorf("error should mention querying schema, got: %v", err)
	}
}

func TestRegistry_Create_InsertFail(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	realDB := &testDB{db: db}
	registry := NewRegistry(realDB)
	if err := registry.Init(context.Background()); err != nil {
		t.Fatalf("Init: %v", err)
	}

	schema := &Schema{
		Name:   "posts",
		Fields: []FieldDefinition{{Name: "title", Type: FieldTypeText}},
	}

	failDB := &registryFailDB{DB: db, failExec: true}
	failRegistry := NewRegistry(failDB)

	err = failRegistry.Create(context.Background(), schema)
	if err == nil {
		t.Fatal("expected error when INSERT fails, got nil")
	}
	if !strings.Contains(err.Error(), "inserting schema") {
		t.Errorf("error should mention inserting schema, got: %v", err)
	}
}

func TestRegistry_Update_ExecFail(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	realDB := &testDB{db: db}
	registry := NewRegistry(realDB)
	if err := registry.Init(context.Background()); err != nil {
		t.Fatalf("Init: %v", err)
	}

	schema := &Schema{
		Name:   "posts",
		Fields: []FieldDefinition{{Name: "title", Type: FieldTypeText}},
	}
	if err := registry.Create(context.Background(), schema); err != nil {
		t.Fatalf("Create: %v", err)
	}

	updated := &Schema{
		Name: "posts",
		Fields: []FieldDefinition{
			{Name: "title", Type: FieldTypeText},
			{Name: "body", Type: FieldTypeText},
		},
	}

	failDB := &registryFailDB{DB: db, failExec: true}
	failRegistry := NewRegistry(failDB)

	err = failRegistry.Update(context.Background(), updated)
	if err == nil {
		t.Fatal("expected error when UPDATE fails, got nil")
	}
	if !strings.Contains(err.Error(), "updating schema") {
		t.Errorf("error should mention updating schema, got: %v", err)
	}
}

func TestRegistry_Delete_ExecFail(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	realDB := &testDB{db: db}
	registry := NewRegistry(realDB)
	if err := registry.Init(context.Background()); err != nil {
		t.Fatalf("Init: %v", err)
	}

	schema := &Schema{
		Name:   "posts",
		Fields: []FieldDefinition{{Name: "title", Type: FieldTypeText}},
	}
	if err := registry.Create(context.Background(), schema); err != nil {
		t.Fatalf("Create: %v", err)
	}

	failDB := &registryFailDB{DB: db, failExec: true}
	failRegistry := NewRegistry(failDB)

	err = failRegistry.Delete(context.Background(), "posts", true)
	if err == nil {
		t.Fatal("expected error when DELETE fails, got nil")
	}
	if !strings.Contains(err.Error(), "deleting schema") {
		t.Errorf("error should mention deleting schema, got: %v", err)
	}
}
