package ddl

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"testing"

	"github.com/wangling-miao/aroute/sdk/interfaces"
	_ "modernc.org/sqlite"
)

type introspectTestDB struct {
	db *sql.DB
}

func newIntrospectTestDB(t *testing.T) *introspectTestDB {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}

	return &introspectTestDB{db: db}
}

func (tdb *introspectTestDB) Close() error {
	return tdb.db.Close()
}

func (tdb *introspectTestDB) Exec(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	return tdb.db.ExecContext(ctx, query, args...)
}

func (tdb *introspectTestDB) Query(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	return tdb.db.QueryContext(ctx, query, args...)
}

func (tdb *introspectTestDB) QueryRow(ctx context.Context, query string, args ...interface{}) *sql.Row {
	return tdb.db.QueryRowContext(ctx, query, args...)
}

func (tdb *introspectTestDB) BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
	return tdb.db.BeginTx(ctx, opts)
}

func (tdb *introspectTestDB) Ping(ctx context.Context) error {
	return tdb.db.PingContext(ctx)
}

func (tdb *introspectTestDB) Prepare(ctx context.Context, query string) (*sql.Stmt, error) {
	return tdb.db.PrepareContext(ctx, query)
}

func (tdb *introspectTestDB) SchemaIntrospect(ctx context.Context) (*interfaces.DatabaseSchema, error) {
	rows, err := tdb.db.QueryContext(ctx,
		"SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name")
	if err != nil {
		return nil, fmt.Errorf("listing tables: %w", err)
	}
	defer rows.Close()

	var schema interfaces.DatabaseSchema
	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err != nil {
			return nil, fmt.Errorf("scanning table name: %w", err)
		}

		colRows, err := tdb.db.QueryContext(ctx, fmt.Sprintf("PRAGMA table_info(\"%s\")", tableName))
		if err != nil {
			return nil, fmt.Errorf("querying columns for %s: %w", tableName, err)
		}

		var td interfaces.TableDefinition
		td.Name = tableName

		for colRows.Next() {
			var cid int
			var name, colType string
			var notNull, pk int
			var defaultValue sql.NullString

			if err := colRows.Scan(&cid, &name, &colType, &notNull, &defaultValue, &pk); err != nil {
				colRows.Close()
				return nil, fmt.Errorf("scanning column for %s: %w", tableName, err)
			}

			td.Columns = append(td.Columns, interfaces.ColumnDefinition{
				Name:         name,
				Type:         colType,
				Nullable:     notNull == 0,
				DefaultValue: defaultValue.String,
			})

			if pk == 1 {
				td.PrimaryKey = append(td.PrimaryKey, name)
			}
		}
		colRows.Close()

		schema.Tables = append(schema.Tables, td)
	}

	return &schema, nil
}

func TestIntrospectTable_NonExistent(t *testing.T) {
	tdb := newIntrospectTestDB(t)
	defer tdb.Close()

	introspector := NewIntrospector(tdb, DialectSQLite)

	result, err := introspector.IntrospectTable(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("IntrospectTable() unexpected error: %v", err)
	}

	if result != nil {
		t.Errorf("IntrospectTable() = %+v, want nil for nonexistent table", result)
	}
}

func TestIntrospectTable_SimpleTable(t *testing.T) {
	tdb := newIntrospectTestDB(t)
	defer tdb.Close()

	ctx := context.Background()

	_, err := tdb.Exec(ctx, `CREATE TABLE users (id INTEGER, name TEXT, email TEXT)`)
	if err != nil {
		t.Fatalf("failed to create table: %v", err)
	}

	introspector := NewIntrospector(tdb, DialectSQLite)

	result, err := introspector.IntrospectTable(ctx, "users")
	if err != nil {
		t.Fatalf("IntrospectTable() error = %v", err)
	}

	if result == nil {
		t.Fatal("IntrospectTable() returned nil, want non-nil result")
	}

	if result.Name != "users" {
		t.Errorf("result.Name = %q, want %q", result.Name, "users")
	}

	wantColCount := 3
	if len(result.Columns) != wantColCount {
		t.Fatalf("len(result.Columns) = %d, want %d", len(result.Columns), wantColCount)
	}

	colByName := make(map[string]interfaces.ColumnDefinition)
	for _, col := range result.Columns {
		colByName[col.Name] = col
	}

	tests := []struct {
		name    string
		colType string
	}{
		{"id", "INTEGER"},
		{"name", "TEXT"},
		{"email", "TEXT"},
	}

	for _, tt := range tests {
		col, ok := colByName[tt.name]
		if !ok {
			t.Errorf("column %q not found in result", tt.name)
			continue
		}
		if col.Type != tt.colType {
			t.Errorf("column %q Type = %q, want %q", tt.name, col.Type, tt.colType)
		}
	}
}

func TestIntrospectTable_NotNull(t *testing.T) {
	tdb := newIntrospectTestDB(t)
	defer tdb.Close()

	ctx := context.Background()

	_, err := tdb.Exec(ctx, `CREATE TABLE items (id INTEGER NOT NULL, label TEXT, value REAL NOT NULL)`)
	if err != nil {
		t.Fatalf("failed to create table: %v", err)
	}

	introspector := NewIntrospector(tdb, DialectSQLite)

	result, err := introspector.IntrospectTable(ctx, "items")
	if err != nil {
		t.Fatalf("IntrospectTable() error = %v", err)
	}

	if result == nil {
		t.Fatal("IntrospectTable() returned nil, want non-nil result")
	}

	colByName := make(map[string]interfaces.ColumnDefinition)
	for _, col := range result.Columns {
		colByName[col.Name] = col
	}

	tests := []struct {
		name     string
		nullable bool
	}{
		{"id", false},
		{"label", true},
		{"value", false},
	}

	for _, tt := range tests {
		col, ok := colByName[tt.name]
		if !ok {
			t.Errorf("column %q not found", tt.name)
			continue
		}
		if col.Nullable != tt.nullable {
			t.Errorf("column %q Nullable = %v, want %v", tt.name, col.Nullable, tt.nullable)
		}
	}
}

func TestIntrospectTable_DefaultValues(t *testing.T) {
	tdb := newIntrospectTestDB(t)
	defer tdb.Close()

	ctx := context.Background()

	_, err := tdb.Exec(ctx, `CREATE TABLE products (
		id INTEGER NOT NULL,
		name TEXT NOT NULL DEFAULT 'unnamed',
		price REAL DEFAULT 0.0,
		active INTEGER DEFAULT 1
	)`)
	if err != nil {
		t.Fatalf("failed to create table: %v", err)
	}

	introspector := NewIntrospector(tdb, DialectSQLite)

	result, err := introspector.IntrospectTable(ctx, "products")
	if err != nil {
		t.Fatalf("IntrospectTable() error = %v", err)
	}

	if result == nil {
		t.Fatal("IntrospectTable() returned nil, want non-nil result")
	}

	colByName := make(map[string]interfaces.ColumnDefinition)
	for _, col := range result.Columns {
		colByName[col.Name] = col
	}

	tests := []struct {
		name   string
		hasDef bool
		defVal string
	}{
		{"id", false, ""},
		{"name", true, "'unnamed'"},
		{"price", true, "0.0"},
		{"active", true, "1"},
	}

	for _, tt := range tests {
		col, ok := colByName[tt.name]
		if !ok {
			t.Errorf("column %q not found", tt.name)
			continue
		}

		if tt.hasDef {
			defStr, _ := col.DefaultValue.(string)
			if defStr != tt.defVal {
				t.Errorf("column %q DefaultValue = %q, want %q", tt.name, defStr, tt.defVal)
			}
		} else {
			if col.DefaultValue != nil {
				defStr, _ := col.DefaultValue.(string)
				if defStr != "" {
					t.Errorf("column %q DefaultValue = %v, want nil/empty", tt.name, col.DefaultValue)
				}
			}
		}
	}
}

func TestIntrospectTable_Indexes(t *testing.T) {
	tdb := newIntrospectTestDB(t)
	defer tdb.Close()

	ctx := context.Background()

	_, err := tdb.Exec(ctx, `CREATE TABLE orders (id INTEGER, customer_id INTEGER, total REAL)`)
	if err != nil {
		t.Fatalf("failed to create table: %v", err)
	}

	_, err = tdb.Exec(ctx, `CREATE INDEX idx_orders_customer ON orders(customer_id)`)
	if err != nil {
		t.Fatalf("failed to create index: %v", err)
	}

	introspector := NewIntrospector(tdb, DialectSQLite)

	result, err := introspector.IntrospectTable(ctx, "orders")
	if err != nil {
		t.Fatalf("IntrospectTable() error = %v", err)
	}

	if result == nil {
		t.Fatal("IntrospectTable() returned nil, want non-nil result")
	}

	if len(result.Indexes) == 0 {
		t.Fatal("expected at least one index, got none")
	}

	found := false
	for _, idx := range result.Indexes {
		if idx.Name == "idx_orders_customer" {
			found = true
			if len(idx.Columns) != 1 || idx.Columns[0] != "customer_id" {
				t.Errorf("index columns = %v, want [customer_id]", idx.Columns)
			}
			if idx.Unique {
				t.Error("index should not be unique")
			}
		}
	}
	if !found {
		t.Error("idx_orders_customer not found in indexes")
	}
}

func TestIntrospectTable_UniqueIndex(t *testing.T) {
	tdb := newIntrospectTestDB(t)
	defer tdb.Close()

	ctx := context.Background()

	_, err := tdb.Exec(ctx, `CREATE TABLE accounts (id INTEGER, email TEXT)`)
	if err != nil {
		t.Fatalf("failed to create table: %v", err)
	}

	_, err = tdb.Exec(ctx, `CREATE UNIQUE INDEX idx_accounts_email ON accounts(email)`)
	if err != nil {
		t.Fatalf("failed to create unique index: %v", err)
	}

	introspector := NewIntrospector(tdb, DialectSQLite)

	result, err := introspector.IntrospectTable(ctx, "accounts")
	if err != nil {
		t.Fatalf("IntrospectTable() error = %v", err)
	}

	if result == nil {
		t.Fatal("IntrospectTable() returned nil, want non-nil result")
	}

	found := false
	for _, idx := range result.Indexes {
		if idx.Name == "idx_accounts_email" {
			found = true
			if !idx.Unique {
				t.Error("idx_accounts_email should be unique")
			}
		}
	}
	if !found {
		t.Error("idx_accounts_email not found in indexes")
	}
}

func TestIntrospectTable_PrimaryKey(t *testing.T) {
	tdb := newIntrospectTestDB(t)
	defer tdb.Close()

	ctx := context.Background()

	_, err := tdb.Exec(ctx, `CREATE TABLE articles (id INTEGER PRIMARY KEY, title TEXT)`)
	if err != nil {
		t.Fatalf("failed to create table: %v", err)
	}

	introspector := NewIntrospector(tdb, DialectSQLite)

	result, err := introspector.IntrospectTable(ctx, "articles")
	if err != nil {
		t.Fatalf("IntrospectTable() error = %v", err)
	}

	if result == nil {
		t.Fatal("IntrospectTable() returned nil, want non-nil result")
	}

	if len(result.PrimaryKey) != 1 || result.PrimaryKey[0] != "id" {
		t.Errorf("PrimaryKey = %v, want [id]", result.PrimaryKey)
	}
}

func TestIntrospectTable_CompositePrimaryKey(t *testing.T) {
	tdb := newIntrospectTestDB(t)
	defer tdb.Close()

	ctx := context.Background()

	_, err := tdb.Exec(ctx, `CREATE TABLE junction (user_id INTEGER, role_id INTEGER, PRIMARY KEY (user_id, role_id))`)
	if err != nil {
		t.Fatalf("failed to create table: %v", err)
	}

	introspector := NewIntrospector(tdb, DialectSQLite)

	result, err := introspector.IntrospectTable(ctx, "junction")
	if err != nil {
		t.Fatalf("IntrospectTable() error = %v", err)
	}

	if result == nil {
		t.Fatal("IntrospectTable() returned nil, want non-nil result")
	}

	if len(result.PrimaryKey) != 2 {
		t.Fatalf("len(PrimaryKey) = %d, want 2", len(result.PrimaryKey))
	}

	pkSet := make(map[string]bool)
	for _, pk := range result.PrimaryKey {
		pkSet[pk] = true
	}
	if !pkSet["user_id"] || !pkSet["role_id"] {
		t.Errorf("PrimaryKey = %v, want [user_id, role_id]", result.PrimaryKey)
	}
}

func TestIntrospectTable_DefaultDialect(t *testing.T) {
	tdb := newIntrospectTestDB(t)
	defer tdb.Close()

	ctx := context.Background()

	_, err := tdb.Exec(ctx, `CREATE TABLE fallback_test (id INTEGER, val TEXT)`)
	if err != nil {
		t.Fatalf("failed to create table: %v", err)
	}

	introspector := NewIntrospector(tdb, Dialect("unknown"))

	result, err := introspector.IntrospectTable(ctx, "fallback_test")
	if err != nil {
		t.Fatalf("IntrospectTable() error = %v", err)
	}

	if result == nil {
		t.Fatal("IntrospectTable() with unknown dialect returned nil, want non-nil result")
	}

	if result.Name != "fallback_test" {
		t.Errorf("result.Name = %q, want %q", result.Name, "fallback_test")
	}

	if len(result.Columns) != 2 {
		t.Errorf("len(result.Columns) = %d, want 2", len(result.Columns))
	}
}

func TestListTables_EmptyDatabase(t *testing.T) {
	tdb := newIntrospectTestDB(t)
	defer tdb.Close()

	introspector := NewIntrospector(tdb, DialectSQLite)

	tables, err := introspector.ListTables(context.Background())
	if err != nil {
		t.Fatalf("ListTables() error = %v", err)
	}

	if len(tables) != 0 {
		t.Errorf("ListTables() = %v, want empty slice", tables)
	}
}

func TestListTables_MultipleTables(t *testing.T) {
	tdb := newIntrospectTestDB(t)
	defer tdb.Close()

	ctx := context.Background()

	tableNames := []string{"users", "posts", "comments"}
	for _, name := range tableNames {
		_, err := tdb.Exec(ctx, fmt.Sprintf("CREATE TABLE %s (id INTEGER, name TEXT)", name))
		if err != nil {
			t.Fatalf("failed to create table %s: %v", name, err)
		}
	}

	introspector := NewIntrospector(tdb, DialectSQLite)

	tables, err := introspector.ListTables(ctx)
	if err != nil {
		t.Fatalf("ListTables() error = %v", err)
	}

	if len(tables) != len(tableNames) {
		t.Fatalf("ListTables() returned %d tables, want %d", len(tables), len(tableNames))
	}

	sorted := make([]string, len(tableNames))
	copy(sorted, tableNames)
	sort.Strings(sorted)

	for i, want := range sorted {
		if tables[i] != want {
			t.Errorf("tables[%d] = %q, want %q", i, tables[i], want)
		}
	}
}

func TestListTables_ExcludesSystemTables(t *testing.T) {
	tdb := newIntrospectTestDB(t)
	defer tdb.Close()

	ctx := context.Background()

	_, err := tdb.Exec(ctx, `CREATE TABLE my_table (id INTEGER)`)
	if err != nil {
		t.Fatalf("failed to create table: %v", err)
	}

	_, err = tdb.Exec(ctx, `CREATE TABLE _hidden (id INTEGER)`)
	if err != nil {
		t.Fatalf("failed to create _hidden table: %v", err)
	}

	introspector := NewIntrospector(tdb, DialectSQLite)

	tables, err := introspector.ListTables(ctx)
	if err != nil {
		t.Fatalf("ListTables() error = %v", err)
	}

	for _, tbl := range tables {
		if tbl == "_hidden" {
			t.Error("ListTables() should exclude tables starting with _")
		}
		if len(tbl) >= 7 && tbl[:7] == "sqlite_" {
			t.Errorf("ListTables() should exclude tables starting with sqlite_, got %q", tbl)
		}
	}

	found := false
	for _, tbl := range tables {
		if tbl == "my_table" {
			found = true
		}
	}
	if !found {
		t.Error("ListTables() should include my_table")
	}
}

func TestListTables_SortedOrder(t *testing.T) {
	tdb := newIntrospectTestDB(t)
	defer tdb.Close()

	ctx := context.Background()

	for _, name := range []string{"zebra", "alpha", "middle"} {
		_, err := tdb.Exec(ctx, fmt.Sprintf("CREATE TABLE %s (id INTEGER)", name))
		if err != nil {
			t.Fatalf("failed to create table %s: %v", name, err)
		}
	}

	introspector := NewIntrospector(tdb, DialectSQLite)

	tables, err := introspector.ListTables(ctx)
	if err != nil {
		t.Fatalf("ListTables() error = %v", err)
	}

	expected := []string{"alpha", "middle", "zebra"}
	if len(tables) != len(expected) {
		t.Fatalf("ListTables() returned %d tables, want %d", len(tables), len(expected))
	}

	for i, want := range expected {
		if tables[i] != want {
			t.Errorf("tables[%d] = %q, want %q", i, tables[i], want)
		}
	}
}

func TestIntrospectTable_MultiColumnIndex(t *testing.T) {
	tdb := newIntrospectTestDB(t)
	defer tdb.Close()

	ctx := context.Background()

	_, err := tdb.Exec(ctx, `CREATE TABLE events (id INTEGER, category TEXT, priority INTEGER)`)
	if err != nil {
		t.Fatalf("failed to create table: %v", err)
	}

	_, err = tdb.Exec(ctx, `CREATE INDEX idx_events_cat_pri ON events(category, priority)`)
	if err != nil {
		t.Fatalf("failed to create index: %v", err)
	}

	introspector := NewIntrospector(tdb, DialectSQLite)

	result, err := introspector.IntrospectTable(ctx, "events")
	if err != nil {
		t.Fatalf("IntrospectTable() error = %v", err)
	}

	if result == nil {
		t.Fatal("IntrospectTable() returned nil, want non-nil result")
	}

	found := false
	for _, idx := range result.Indexes {
		if idx.Name == "idx_events_cat_pri" {
			found = true
			if len(idx.Columns) != 2 {
				t.Errorf("index has %d columns, want 2", len(idx.Columns))
			}
			if len(idx.Columns) >= 2 {
				if idx.Columns[0] != "category" || idx.Columns[1] != "priority" {
					t.Errorf("index columns = %v, want [category, priority]", idx.Columns)
				}
			}
		}
	}
	if !found {
		t.Error("idx_events_cat_pri not found in indexes")
	}
}

func TestIntrospectTable_ColumnTypes(t *testing.T) {
	tdb := newIntrospectTestDB(t)
	defer tdb.Close()

	ctx := context.Background()

	_, err := tdb.Exec(ctx, `CREATE TABLE type_test (
		col_int INTEGER,
		col_text TEXT,
		col_real REAL,
		col_blob BLOB
	)`)
	if err != nil {
		t.Fatalf("failed to create table: %v", err)
	}

	introspector := NewIntrospector(tdb, DialectSQLite)

	result, err := introspector.IntrospectTable(ctx, "type_test")
	if err != nil {
		t.Fatalf("IntrospectTable() error = %v", err)
	}

	if result == nil {
		t.Fatal("IntrospectTable() returned nil, want non-nil result")
	}

	colByName := make(map[string]interfaces.ColumnDefinition)
	for _, col := range result.Columns {
		colByName[col.Name] = col
	}

	tests := []struct {
		name    string
		colType string
	}{
		{"col_int", "INTEGER"},
		{"col_text", "TEXT"},
		{"col_real", "REAL"},
		{"col_blob", "BLOB"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			col, ok := colByName[tt.name]
			if !ok {
				t.Fatalf("column %q not found", tt.name)
			}
			if col.Type != tt.colType {
				t.Errorf("Type = %q, want %q", col.Type, tt.colType)
			}
		})
	}
}
