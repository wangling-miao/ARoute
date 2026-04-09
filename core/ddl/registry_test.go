package ddl

import (
	"context"
	"database/sql"
	"fmt"
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
