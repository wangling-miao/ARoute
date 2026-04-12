package ddl

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/wangling-miao/aroute/sdk/interfaces"
)

func TestPostgreSQL_Introspect_NonExistentTable(t *testing.T) {
	pgdb := newPGTestDB(t)
	defer pgdb.Close()

	if !pgdb.IsAvailable() {
		t.Skip("PostgreSQL not available - skipping")
	}

	introspector := NewIntrospector(pgdb, DialectPostgreSQL)
	ctx := context.Background()

	result, err := introspector.IntrospectTable(ctx, "nonexistent_table_xyz")
	if err != nil {
		t.Fatalf("IntrospectTable() unexpected error: %v", err)
	}

	if result != nil {
		t.Errorf("IntrospectTable() = %+v, want nil for nonexistent table", result)
	}
}

func TestPostgreSQL_Introspect_SimpleTable(t *testing.T) {
	pgdb := newPGTestDB(t)
	defer pgdb.Close()

	if !pgdb.IsAvailable() {
		t.Skip("PostgreSQL not available - skipping")
	}

	ctx := context.Background()
	tableName := "pg_test_simple"

	pgDropTestTable(t, pgdb.db, tableName)

	_, err := pgdb.Exec(ctx, fmt.Sprintf(`
		CREATE TABLE "%s" (
			id BIGINT PRIMARY KEY,
			name TEXT,
			email VARCHAR(255)
		)`, tableName))
	if err != nil {
		t.Fatalf("create table: %v", err)
	}

	introspector := NewIntrospector(pgdb, DialectPostgreSQL)

	result, err := introspector.IntrospectTable(ctx, tableName)
	if err != nil {
		t.Fatalf("IntrospectTable() error = %v", err)
	}

	if result == nil {
		t.Fatal("IntrospectTable() returned nil")
	}

	if result.Name != tableName {
		t.Errorf("result.Name = %q, want %q", result.Name, tableName)
	}

	if len(result.Columns) != 3 {
		t.Fatalf("len(result.Columns) = %d, want 3", len(result.Columns))
	}

	colByName := make(map[string]interfaces.ColumnDefinition)
	for _, col := range result.Columns {
		colByName[col.Name] = col
	}

	tests := []struct {
		name         string
		colTypeMatch string
	}{
		{"id", "BIGINT"},
		{"name", "TEXT"},
		{"email", "VARCHAR"},
	}

	for _, tt := range tests {
		col, ok := colByName[tt.name]
		if !ok {
			t.Errorf("column %q not found", tt.name)
			continue
		}
		if !strings.Contains(col.Type, tt.colTypeMatch) {
			t.Errorf("column %q Type = %q, want containing %q", tt.name, col.Type, tt.colTypeMatch)
		}
	}

	pgDropTestTable(t, pgdb.db, tableName)
}

func TestPostgreSQL_Introspect_VarcharLength(t *testing.T) {
	pgdb := newPGTestDB(t)
	defer pgdb.Close()

	if !pgdb.IsAvailable() {
		t.Skip("PostgreSQL not available - skipping")
	}

	ctx := context.Background()
	tableName := "pg_test_varchar"

	pgDropTestTable(t, pgdb.db, tableName)

	_, err := pgdb.Exec(ctx, fmt.Sprintf(`
		CREATE TABLE "%s" (
			id BIGINT PRIMARY KEY,
			code VARCHAR(50)
		)`, tableName))
	if err != nil {
		t.Fatalf("create table: %v", err)
	}

	introspector := NewIntrospector(pgdb, DialectPostgreSQL)

	result, err := introspector.IntrospectTable(ctx, tableName)
	if err != nil {
		t.Fatalf("IntrospectTable() error = %v", err)
	}

	if result == nil {
		t.Fatal("IntrospectTable() returned nil")
	}

	colByName := make(map[string]interfaces.ColumnDefinition)
	for _, col := range result.Columns {
		colByName[col.Name] = col
	}

	col, ok := colByName["code"]
	if !ok {
		t.Fatal("column 'code' not found")
	}

	if !strings.Contains(col.Type, "VARCHAR(50)") {
		t.Errorf("column 'code' Type = %q, want VARCHAR(50)", col.Type)
	}

	pgDropTestTable(t, pgdb.db, tableName)
}

func TestPostgreSQL_Introspect_Nullability(t *testing.T) {
	pgdb := newPGTestDB(t)
	defer pgdb.Close()

	if !pgdb.IsAvailable() {
		t.Skip("PostgreSQL not available - skipping")
	}

	ctx := context.Background()
	tableName := "pg_test_nullable"

	pgDropTestTable(t, pgdb.db, tableName)

	_, err := pgdb.Exec(ctx, fmt.Sprintf(`
		CREATE TABLE "%s" (
			id BIGINT NOT NULL,
			optional TEXT,
			required TEXT NOT NULL
		)`, tableName))
	if err != nil {
		t.Fatalf("create table: %v", err)
	}

	introspector := NewIntrospector(pgdb, DialectPostgreSQL)

	result, err := introspector.IntrospectTable(ctx, tableName)
	if err != nil {
		t.Fatalf("IntrospectTable() error = %v", err)
	}

	if result == nil {
		t.Fatal("IntrospectTable() returned nil")
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
		{"optional", true},
		{"required", false},
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

	pgDropTestTable(t, pgdb.db, tableName)
}

func TestPostgreSQL_Introspect_DefaultValues(t *testing.T) {
	pgdb := newPGTestDB(t)
	defer pgdb.Close()

	if !pgdb.IsAvailable() {
		t.Skip("PostgreSQL not available - skipping")
	}

	ctx := context.Background()
	tableName := "pg_test_defaults"

	pgDropTestTable(t, pgdb.db, tableName)

	_, err := pgdb.Exec(ctx, fmt.Sprintf(`
		CREATE TABLE "%s" (
			id BIGINT PRIMARY KEY,
			count BIGINT DEFAULT 10,
			active BOOLEAN DEFAULT TRUE,
			label TEXT DEFAULT 'unnamed'
		)`, tableName))
	if err != nil {
		t.Fatalf("create table: %v", err)
	}

	introspector := NewIntrospector(pgdb, DialectPostgreSQL)

	result, err := introspector.IntrospectTable(ctx, tableName)
	if err != nil {
		t.Fatalf("IntrospectTable() error = %v", err)
	}

	if result == nil {
		t.Fatal("IntrospectTable() returned nil")
	}

	colByName := make(map[string]interfaces.ColumnDefinition)
	for _, col := range result.Columns {
		colByName[col.Name] = col
	}

	tests := []struct {
		name     string
		hasDef   bool
		defMatch string
	}{
		{"id", false, ""},
		{"count", true, "10"},
		{"active", true, "true"},
		{"label", true, "'unnamed'"},
	}

	for _, tt := range tests {
		col, ok := colByName[tt.name]
		if !ok {
			t.Errorf("column %q not found", tt.name)
			continue
		}

		defStr, _ := col.DefaultValue.(string)
		if tt.hasDef {
			if defStr == "" {
				t.Errorf("column %q should have default value", tt.name)
			}
			if tt.defMatch != "" && !strings.Contains(defStr, tt.defMatch) {
				t.Errorf("column %q DefaultValue = %q, want containing %q", tt.name, defStr, tt.defMatch)
			}
		}
	}

	pgDropTestTable(t, pgdb.db, tableName)
}

func TestPostgreSQL_Introspect_Indexes(t *testing.T) {
	pgdb := newPGTestDB(t)
	defer pgdb.Close()

	if !pgdb.IsAvailable() {
		t.Skip("PostgreSQL not available - skipping")
	}

	ctx := context.Background()
	tableName := "pg_test_indexes"

	pgDropTestTable(t, pgdb.db, tableName)

	_, err := pgdb.Exec(ctx, fmt.Sprintf(`
		CREATE TABLE "%s" (
			id BIGINT PRIMARY KEY,
			email TEXT,
			status TEXT
		)`, tableName))
	if err != nil {
		t.Fatalf("create table: %v", err)
	}

	_, err = pgdb.Exec(ctx, fmt.Sprintf(`CREATE INDEX idx_pg_email ON "%s"(email)`, tableName))
	if err != nil {
		t.Fatalf("create index: %v", err)
	}

	_, err = pgdb.Exec(ctx, fmt.Sprintf(`CREATE UNIQUE INDEX idx_pg_status_unique ON "%s"(status)`, tableName))
	if err != nil {
		t.Fatalf("create unique index: %v", err)
	}

	introspector := NewIntrospector(pgdb, DialectPostgreSQL)

	result, err := introspector.IntrospectTable(ctx, tableName)
	if err != nil {
		t.Fatalf("IntrospectTable() error = %v", err)
	}

	if result == nil {
		t.Fatal("IntrospectTable() returned nil")
	}

	idxByName := make(map[string]interfaces.IndexDefinition)
	for _, idx := range result.Indexes {
		idxByName[idx.Name] = idx
	}

	if _, ok := idxByName["idx_pg_email"]; !ok {
		t.Error("idx_pg_email not found")
	}

	uniqueIdx, ok := idxByName["idx_pg_status_unique"]
	if !ok {
		t.Error("idx_pg_status_unique not found")
	}
	if !uniqueIdx.Unique {
		t.Error("idx_pg_status_unique should be unique")
	}

	pgDropTestTable(t, pgdb.db, tableName)
}

func TestPostgreSQL_Introspect_ForeignKeys(t *testing.T) {
	pgdb := newPGTestDB(t)
	defer pgdb.Close()

	if !pgdb.IsAvailable() {
		t.Skip("PostgreSQL not available - skipping")
	}

	ctx := context.Background()

	parentTable := "pg_test_fk_parent"
	childTable := "pg_test_fk_child"

	pgDropTestTable(t, pgdb.db, childTable)
	pgDropTestTable(t, pgdb.db, parentTable)

	_, err := pgdb.Exec(ctx, fmt.Sprintf(`
		CREATE TABLE "%s" (
			id BIGINT PRIMARY KEY,
			name TEXT
		)`, parentTable))
	if err != nil {
		t.Fatalf("create parent table: %v", err)
	}

	_, err = pgdb.Exec(ctx, fmt.Sprintf(`
		CREATE TABLE "%s" (
			id BIGINT PRIMARY KEY,
			parent_id BIGINT REFERENCES "%s"(id) ON DELETE CASCADE ON UPDATE NO ACTION
		)`, childTable, parentTable))
	if err != nil {
		t.Fatalf("create child table: %v", err)
	}

	introspector := NewIntrospector(pgdb, DialectPostgreSQL)

	result, err := introspector.IntrospectTable(ctx, childTable)
	if err != nil {
		t.Fatalf("IntrospectTable() error = %v", err)
	}

	if result == nil {
		t.Fatal("IntrospectTable() returned nil")
	}

	if len(result.ForeignKeys) == 0 {
		t.Fatal("expected foreign key, got none")
	}

	foundFK := false
	for _, fk := range result.ForeignKeys {
		if fk.RefTable == parentTable {
			foundFK = true
			if fk.OnDelete != "CASCADE" {
				t.Errorf("FK OnDelete = %q, want CASCADE", fk.OnDelete)
			}
			break
		}
	}

	if !foundFK {
		t.Errorf("foreign key to %s not found", parentTable)
	}

	pgDropTestTable(t, pgdb.db, childTable)
	pgDropTestTable(t, pgdb.db, parentTable)
}

func TestPostgreSQL_Introspect_SpecialTypes(t *testing.T) {
	pgdb := newPGTestDB(t)
	defer pgdb.Close()

	if !pgdb.IsAvailable() {
		t.Skip("PostgreSQL not available - skipping")
	}

	ctx := context.Background()
	tableName := "pg_test_types"

	pgDropTestTable(t, pgdb.db, tableName)

	_, err := pgdb.Exec(ctx, fmt.Sprintf(`
		CREATE TABLE "%s" (
			id BIGINT PRIMARY KEY,
			data JSONB,
			ts TIMESTAMP WITH TIME ZONE,
			num NUMERIC,
			flag BOOLEAN
		)`, tableName))
	if err != nil {
		t.Fatalf("create table: %v", err)
	}

	introspector := NewIntrospector(pgdb, DialectPostgreSQL)

	result, err := introspector.IntrospectTable(ctx, tableName)
	if err != nil {
		t.Fatalf("IntrospectTable() error = %v", err)
	}

	if result == nil {
		t.Fatal("IntrospectTable() returned nil")
	}

	colByName := make(map[string]interfaces.ColumnDefinition)
	for _, col := range result.Columns {
		colByName[col.Name] = col
	}

	tests := []struct {
		name    string
		colType string
	}{
		{"id", "BIGINT"},
		{"data", "JSONB"},
		{"ts", "TIMESTAMP"},
		{"num", "NUMERIC"},
		{"flag", "BOOLEAN"},
	}

	for _, tt := range tests {
		col, ok := colByName[tt.name]
		if !ok {
			t.Errorf("column %q not found", tt.name)
			continue
		}
		if !strings.Contains(col.Type, tt.colType) {
			t.Errorf("column %q Type = %q, want containing %q", tt.name, col.Type, tt.colType)
		}
	}

	pgDropTestTable(t, pgdb.db, tableName)
}

func TestPostgreSQL_ListTables(t *testing.T) {
	pgdb := newPGTestDB(t)
	defer pgdb.Close()

	if !pgdb.IsAvailable() {
		t.Skip("PostgreSQL not available - skipping")
	}

	ctx := context.Background()

	// Create test tables
	for _, name := range []string{"pg_list_a", "pg_list_b", "pg_list_c"} {
		pgDropTestTable(t, pgdb.db, name)
		_, err := pgdb.Exec(ctx, fmt.Sprintf(`CREATE TABLE "%s" (id BIGINT PRIMARY KEY)`, name))
		if err != nil {
			t.Fatalf("create table %s: %v", name, err)
		}
	}

	// Also create a hidden table that should be excluded
	_, err := pgdb.Exec(ctx, `CREATE TABLE "_hidden_pg" (id BIGINT PRIMARY KEY)`)
	if err != nil {
		t.Fatalf("create hidden table: %v", err)
	}

	introspector := NewIntrospector(pgdb, DialectPostgreSQL)

	tables, err := introspector.ListTables(ctx)
	if err != nil {
		t.Fatalf("ListTables() error = %v", err)
	}

	foundA, foundB, foundC := false, false, false
	for _, tbl := range tables {
		if tbl == "pg_list_a" {
			foundA = true
		}
		if tbl == "pg_list_b" {
			foundB = true
		}
		if tbl == "pg_list_c" {
			foundC = true
		}
		if tbl == "_hidden_pg" {
			t.Error("_hidden_pg should be excluded from list")
		}
	}

	if !foundA || !foundB || !foundC {
		t.Errorf("expected to find pg_list_a, pg_list_b, pg_list_c in table list")
	}

	// Cleanup
	for _, name := range []string{"pg_list_a", "pg_list_b", "pg_list_c", "_hidden_pg"} {
		pgDropTestTable(t, pgdb.db, name)
	}
}

func TestPostgreSQL_ListTables_Empty(t *testing.T) {
	pgdb := newPGTestDB(t)
	defer pgdb.Close()

	if !pgdb.IsAvailable() {
		t.Skip("PostgreSQL not available - skipping")
	}

	ctx := context.Background()

	introspector := NewIntrospector(pgdb, DialectPostgreSQL)

	tables, err := introspector.ListTables(ctx)
	if err != nil {
		t.Fatalf("ListTables() error = %v", err)
	}

	if len(tables) != 0 {
		t.Errorf("expected 0 tables, got %d: %v", len(tables), tables)
	}
}

func TestPostgreSQL_Introspect_DialectRouting(t *testing.T) {
	pgdb := newPGTestDB(t)
	defer pgdb.Close()

	if !pgdb.IsAvailable() {
		t.Skip("PostgreSQL not available - skipping")
	}

	ctx := context.Background()
	tableName := "pg_test_routing"

	pgDropTestTable(t, pgdb.db, tableName)

	_, err := pgdb.Exec(ctx, fmt.Sprintf(`CREATE TABLE "%s" (id BIGINT PRIMARY KEY)`, tableName))
	if err != nil {
		t.Fatalf("create table: %v", err)
	}

	pgIntrospector := NewIntrospector(pgdb, DialectPostgreSQL)
	result, err := pgIntrospector.IntrospectTable(ctx, tableName)
	if err != nil {
		t.Fatalf("PostgreSQL IntrospectTable() error = %v", err)
	}
	if result == nil {
		t.Fatal("PostgreSQL IntrospectTable() returned nil")
	}
	if result.Name != tableName {
		t.Errorf("PostgreSQL result.Name = %q, want %q", result.Name, tableName)
	}

	pgDropTestTable(t, pgdb.db, tableName)
}

func TestPostgreSQL_Introspect_ErrorHandling(t *testing.T) {
	pgdb := newPGTestDB(t)
	defer pgdb.Close()

	if !pgdb.IsAvailable() {
		t.Skip("PostgreSQL not available - skipping")
	}

	ctx := context.Background()

	// Test query error scenario - use invalid SQL (covered by the mock fallback)
	// The real DB should handle errors gracefully
	introspector := NewIntrospector(pgdb, DialectPostgreSQL)

	// Non-existent table returns nil, not error
	result, err := introspector.IntrospectTable(ctx, "table_that_does_not_exist_xyz")
	if err != nil {
		t.Logf("IntrospectTable returned error for non-existent: %v (acceptable)", err)
	}
	if result != nil {
		t.Error("IntrospectTable should return nil for non-existent table")
	}
}
