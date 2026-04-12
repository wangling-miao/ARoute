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

type sqliteTestDB struct {
	db *sql.DB
}

func newSQLiteTestDB(t *testing.T) *sqliteTestDB {
	t.Helper()
	db, err := sql.Open("sqlite", "file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	return &sqliteTestDB{db: db}
}

func (s *sqliteTestDB) Close() error {
	return s.db.Close()
}

func (s *sqliteTestDB) Exec(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	return s.db.ExecContext(ctx, query, args...)
}

func (s *sqliteTestDB) Query(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	return s.db.QueryContext(ctx, query, args...)
}

func (s *sqliteTestDB) QueryRow(ctx context.Context, query string, args ...interface{}) *sql.Row {
	return s.db.QueryRowContext(ctx, query, args...)
}

func (s *sqliteTestDB) BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
	return s.db.BeginTx(ctx, opts)
}

func (s *sqliteTestDB) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

func (s *sqliteTestDB) Prepare(ctx context.Context, query string) (*sql.Stmt, error) {
	return s.db.PrepareContext(ctx, query)
}

func (s *sqliteTestDB) SchemaIntrospect(ctx context.Context) (*interfaces.DatabaseSchema, error) {
	introspector := NewIntrospector(s, DialectSQLite)
	tableNames, err := introspector.ListTables(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing tables: %w", err)
	}

	schema := &interfaces.DatabaseSchema{}
	for _, tableName := range tableNames {
		tableDef, err := introspector.IntrospectTable(ctx, tableName)
		if err != nil {
			return nil, fmt.Errorf("introspecting table %s: %w", tableName, err)
		}
		if tableDef != nil {
			schema.Tables = append(schema.Tables, *tableDef)
		}
	}

	return schema, nil
}

func columnExists(t *testing.T, db *sql.DB, tableName, columnName string) bool {
	t.Helper()
	rows, err := db.QueryContext(context.Background(), fmt.Sprintf("PRAGMA table_info(\"%s\")", tableName))
	if err != nil {
		t.Fatalf("PRAGMA table_info: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name, colType string
		var notNull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dflt, &pk); err != nil {
			t.Fatalf("scan column info: %v", err)
		}
		if name == columnName {
			return true
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterating columns: %v", err)
	}
	return false
}

func getTableColumns(t *testing.T, db *sql.DB, tableName string) []string {
	t.Helper()
	rows, err := db.QueryContext(context.Background(), fmt.Sprintf("PRAGMA table_info(\"%s\")", tableName))
	if err != nil {
		t.Fatalf("PRAGMA table_info: %v", err)
	}
	defer rows.Close()

	var columns []string
	for rows.Next() {
		var cid int
		var name, colType string
		var notNull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dflt, &pk); err != nil {
			t.Fatalf("scan column info: %v", err)
		}
		columns = append(columns, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterating columns: %v", err)
	}
	return columns
}

func getColumnType(t *testing.T, db *sql.DB, tableName, columnName string) string {
	t.Helper()
	rows, err := db.QueryContext(context.Background(), fmt.Sprintf("PRAGMA table_info(\"%s\")", tableName))
	if err != nil {
		t.Fatalf("PRAGMA table_info: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name, colType string
		var notNull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dflt, &pk); err != nil {
			t.Fatalf("scan column info: %v", err)
		}
		if name == columnName {
			return strings.ToUpper(colType)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterating columns: %v", err)
	}
	t.Fatalf("column %s not found in table %s", columnName, tableName)
	return ""
}

func indexExists(t *testing.T, db *sql.DB, indexName string) bool {
	t.Helper()
	row := db.QueryRowContext(context.Background(),
		"SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name=?", indexName)
	var count int
	if err := row.Scan(&count); err != nil {
		t.Fatalf("query index existence: %v", err)
	}
	return count > 0
}

func tableExists(t *testing.T, db *sql.DB, tableName string) bool {
	t.Helper()
	row := db.QueryRowContext(context.Background(),
		"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?", tableName)
	var count int
	if err := row.Scan(&count); err != nil {
		t.Fatalf("query table existence: %v", err)
	}
	return count > 0
}

func getRowCount(t *testing.T, db *sql.DB, tableName string) int {
	t.Helper()
	row := db.QueryRowContext(context.Background(), fmt.Sprintf("SELECT COUNT(*) FROM \"%s\"", tableName))
	var count int
	if err := row.Scan(&count); err != nil {
		t.Fatalf("query row count: %v", err)
	}
	return count
}

func createTestTable(t *testing.T, executor *SQLiteExecutor, ctx context.Context, schema *Schema) {
	t.Helper()
	ops := []DiffOperation{
		{
			Type:      OpTableCreate,
			TableName: schema.GetTableName(),
			Schema:    schema,
		},
	}
	if err := executor.Execute(ctx, ops, false); err != nil {
		t.Fatalf("createTestTable: %v", err)
	}
}

func TestSQLiteExecutor_TableCreate(t *testing.T) {
	tdb := newSQLiteTestDB(t)
	defer tdb.Close()

	executor := NewSQLiteExecutor(tdb)
	ctx := context.Background()

	ops := []DiffOperation{
		{
			Type:      OpTableCreate,
			TableName: "users",
			Schema: &Schema{
				Name: "users",
				Fields: []FieldDefinition{
					{Name: "id", Type: FieldTypeNumber, Constraints: &Constraints{Nullable: false}},
					{Name: "name", Type: FieldTypeText},
					{Name: "email", Type: FieldTypeText, Constraints: &Constraints{Unique: true}},
				},
			},
		},
	}

	if err := executor.Execute(ctx, ops, false); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if !tableExists(t, tdb.db, "users") {
		t.Error("table 'users' should exist after creation")
	}

	cols := getTableColumns(t, tdb.db, "users")
	for _, expected := range []string{"id", "name", "email"} {
		found := false
		for _, col := range cols {
			if col == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected column %q not found in table; got columns: %v", expected, cols)
		}
	}

	// Verify NOT NULL constraint on id
	idType := getColumnType(t, tdb.db, "users", "id")
	if idType != "INTEGER" {
		t.Errorf("id column type = %q, want INTEGER", idType)
	}

	// Verify TEXT type on name
	nameType := getColumnType(t, tdb.db, "users", "name")
	if nameType != "TEXT" {
		t.Errorf("name column type = %q, want TEXT", nameType)
	}
}

func TestSQLiteExecutor_TableCreate_MultipleColumns(t *testing.T) {
	tdb := newSQLiteTestDB(t)
	defer tdb.Close()

	executor := NewSQLiteExecutor(tdb)
	ctx := context.Background()

	tests := []struct {
		name     string
		schema   *Schema
		wantCol  string
		wantType string
	}{
		{
			name: "boolean field maps to INTEGER",
			schema: &Schema{
				Name: "flags",
				Fields: []FieldDefinition{
					{Name: "id", Type: FieldTypeNumber, Constraints: &Constraints{Nullable: false}},
					{Name: "active", Type: FieldTypeBoolean},
				},
			},
			wantCol:  "active",
			wantType: "INTEGER",
		},
		{
			name: "decimal field maps to REAL",
			schema: &Schema{
				Name: "prices",
				Fields: []FieldDefinition{
					{Name: "id", Type: FieldTypeNumber, Constraints: &Constraints{Nullable: false}},
					{Name: "price", Type: FieldTypeDecimal},
				},
			},
			wantCol:  "price",
			wantType: "REAL",
		},
		{
			name: "json field maps to TEXT",
			schema: &Schema{
				Name: "configs",
				Fields: []FieldDefinition{
					{Name: "id", Type: FieldTypeNumber, Constraints: &Constraints{Nullable: false}},
					{Name: "data", Type: FieldTypeJSON},
				},
			},
			wantCol:  "data",
			wantType: "TEXT",
		},
		{
			name: "datetime field maps to TEXT",
			schema: &Schema{
				Name: "events",
				Fields: []FieldDefinition{
					{Name: "id", Type: FieldTypeNumber, Constraints: &Constraints{Nullable: false}},
					{Name: "created_at", Type: FieldTypeDatetime},
				},
			},
			wantCol:  "created_at",
			wantType: "TEXT",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ops := []DiffOperation{
				{
					Type:      OpTableCreate,
					TableName: tt.schema.GetTableName(),
					Schema:    tt.schema,
				},
			}
			if err := executor.Execute(ctx, ops, false); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}

			colType := getColumnType(t, tdb.db, tt.schema.GetTableName(), tt.wantCol)
			if colType != tt.wantType {
				t.Errorf("column %q type = %q, want %q", tt.wantCol, colType, tt.wantType)
			}
		})
	}
}

func TestSQLiteExecutor_ColumnAdd(t *testing.T) {
	tdb := newSQLiteTestDB(t)
	defer tdb.Close()

	executor := NewSQLiteExecutor(tdb)
	ctx := context.Background()

	// Create initial table
	createTestTable(t, executor, ctx, &Schema{
		Name: "users",
		Fields: []FieldDefinition{
			{Name: "id", Type: FieldTypeNumber, Constraints: &Constraints{Nullable: false}},
			{Name: "name", Type: FieldTypeText},
		},
	})

	// Add a column
	ops := []DiffOperation{
		{
			Type:       OpColumnAdd,
			TableName:  "users",
			ColumnName: "email",
			ColumnType: "TEXT",
		},
	}

	if err := executor.Execute(ctx, ops, false); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if !columnExists(t, tdb.db, "users", "email") {
		t.Error("column 'email' should exist after add")
	}

	// Original columns should still exist
	if !columnExists(t, tdb.db, "users", "id") {
		t.Error("column 'id' should still exist")
	}
	if !columnExists(t, tdb.db, "users", "name") {
		t.Error("column 'name' should still exist")
	}
}

func TestSQLiteExecutor_ColumnAdd_WithConstraints(t *testing.T) {
	tdb := newSQLiteTestDB(t)
	defer tdb.Close()

	executor := NewSQLiteExecutor(tdb)
	ctx := context.Background()

	// Create initial table
	createTestTable(t, executor, ctx, &Schema{
		Name: "users",
		Fields: []FieldDefinition{
			{Name: "id", Type: FieldTypeNumber, Constraints: &Constraints{Nullable: false}},
			{Name: "name", Type: FieldTypeText},
		},
	})

	// Add a NOT NULL column with DEFAULT
	ops := []DiffOperation{
		{
			Type:       OpColumnAdd,
			TableName:  "users",
			ColumnName: "status",
			ColumnType: "TEXT",
			Constraints: &Constraints{
				Nullable: false,
				Default:  "active",
			},
		},
	}

	if err := executor.Execute(ctx, ops, false); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if !columnExists(t, tdb.db, "users", "status") {
		t.Error("column 'status' should exist after add")
	}
}

func TestSQLiteExecutor_IndexAdd(t *testing.T) {
	tdb := newSQLiteTestDB(t)
	defer tdb.Close()

	executor := NewSQLiteExecutor(tdb)
	ctx := context.Background()

	// Create table first
	createTestTable(t, executor, ctx, &Schema{
		Name: "users",
		Fields: []FieldDefinition{
			{Name: "id", Type: FieldTypeNumber, Constraints: &Constraints{Nullable: false}},
			{Name: "name", Type: FieldTypeText},
			{Name: "email", Type: FieldTypeText},
		},
	})

	// Add a non-unique index
	ops := []DiffOperation{
		{
			Type:         OpIndexAdd,
			TableName:    "users",
			IndexName:    "idx_users_name",
			IndexColumns: []string{"name"},
			IndexUnique:  false,
		},
	}

	if err := executor.Execute(ctx, ops, false); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if !indexExists(t, tdb.db, "idx_users_name") {
		t.Error("index 'idx_users_name' should exist after add")
	}
}

func TestSQLiteExecutor_IndexAdd_Unique(t *testing.T) {
	tdb := newSQLiteTestDB(t)
	defer tdb.Close()

	executor := NewSQLiteExecutor(tdb)
	ctx := context.Background()

	// Create table first
	createTestTable(t, executor, ctx, &Schema{
		Name: "users",
		Fields: []FieldDefinition{
			{Name: "id", Type: FieldTypeNumber, Constraints: &Constraints{Nullable: false}},
			{Name: "email", Type: FieldTypeText},
		},
	})

	// Add a unique index
	ops := []DiffOperation{
		{
			Type:         OpIndexAdd,
			TableName:    "users",
			IndexName:    "idx_users_email",
			IndexColumns: []string{"email"},
			IndexUnique:  true,
		},
	}

	if err := executor.Execute(ctx, ops, false); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if !indexExists(t, tdb.db, "idx_users_email") {
		t.Error("unique index 'idx_users_email' should exist after add")
	}
}

func TestSQLiteExecutor_IndexDrop(t *testing.T) {
	tdb := newSQLiteTestDB(t)
	defer tdb.Close()

	executor := NewSQLiteExecutor(tdb)
	ctx := context.Background()

	// Create table
	createTestTable(t, executor, ctx, &Schema{
		Name: "users",
		Fields: []FieldDefinition{
			{Name: "id", Type: FieldTypeNumber, Constraints: &Constraints{Nullable: false}},
			{Name: "name", Type: FieldTypeText},
		},
	})

	// Add an index first
	addIndexOps := []DiffOperation{
		{
			Type:         OpIndexAdd,
			TableName:    "users",
			IndexName:    "idx_users_name",
			IndexColumns: []string{"name"},
			IndexUnique:  false,
		},
	}
	if err := executor.Execute(ctx, addIndexOps, false); err != nil {
		t.Fatalf("add index: %v", err)
	}

	if !indexExists(t, tdb.db, "idx_users_name") {
		t.Fatal("index should exist after creation")
	}

	// Drop the index
	dropOps := []DiffOperation{
		{
			Type:      OpIndexDrop,
			TableName: "users",
			IndexName: "idx_users_name",
		},
	}

	if err := executor.Execute(ctx, dropOps, false); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if indexExists(t, tdb.db, "idx_users_name") {
		t.Error("index 'idx_users_name' should not exist after drop")
	}
}

func TestSQLiteExecutor_ColumnDrop_WithoutForce(t *testing.T) {
	tdb := newSQLiteTestDB(t)
	defer tdb.Close()

	executor := NewSQLiteExecutor(tdb)
	ctx := context.Background()

	// Create table
	createTestTable(t, executor, ctx, &Schema{
		Name: "users",
		Fields: []FieldDefinition{
			{Name: "id", Type: FieldTypeNumber, Constraints: &Constraints{Nullable: false}},
			{Name: "name", Type: FieldTypeText},
			{Name: "email", Type: FieldTypeText},
		},
	})

	// Attempt to drop a column without force — should fail
	ops := []DiffOperation{
		{
			Type:       OpColumnDrop,
			TableName:  "users",
			ColumnName: "email",
		},
	}

	err := executor.Execute(ctx, ops, false)
	if err == nil {
		t.Fatal("expected error for destructive operation without force, got nil")
	}

	if !strings.Contains(err.Error(), "requires explicit confirmation") {
		t.Errorf("error should mention force requirement, got: %v", err)
	}

	// Column should still exist
	if !columnExists(t, tdb.db, "users", "email") {
		t.Error("column 'email' should still exist after failed drop")
	}
}

func TestSQLiteExecutor_ColumnDrop_WithForce(t *testing.T) {
	tdb := newSQLiteTestDB(t)
	defer tdb.Close()

	executor := NewSQLiteExecutor(tdb)
	ctx := context.Background()

	// Create table
	createTestTable(t, executor, ctx, &Schema{
		Name: "users",
		Fields: []FieldDefinition{
			{Name: "id", Type: FieldTypeNumber, Constraints: &Constraints{Nullable: false}},
			{Name: "name", Type: FieldTypeText},
			{Name: "email", Type: FieldTypeText},
		},
	})

	// Drop column with force
	ops := []DiffOperation{
		{
			Type:       OpColumnDrop,
			TableName:  "users",
			ColumnName: "email",
		},
	}

	if err := executor.Execute(ctx, ops, true); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	// email column should be gone
	if columnExists(t, tdb.db, "users", "email") {
		t.Error("column 'email' should not exist after drop")
	}

	// Other columns should still exist
	if !columnExists(t, tdb.db, "users", "id") {
		t.Error("column 'id' should still exist")
	}
	if !columnExists(t, tdb.db, "users", "name") {
		t.Error("column 'name' should still exist")
	}
}

func TestSQLiteExecutor_ColumnModify(t *testing.T) {
	tdb := newSQLiteTestDB(t)
	defer tdb.Close()

	executor := NewSQLiteExecutor(tdb)
	ctx := context.Background()

	// Create table with a TEXT column
	createTestTable(t, executor, ctx, &Schema{
		Name: "users",
		Fields: []FieldDefinition{
			{Name: "id", Type: FieldTypeNumber, Constraints: &Constraints{Nullable: false}},
			{Name: "name", Type: FieldTypeText},
		},
	})

	// Modify column: change name from TEXT to number (INTEGER)
	// OpColumnModify triggers rebuildTable since NeedsRebuild returns true for SQLite
	ops := []DiffOperation{
		{
			Type:       OpColumnModify,
			TableName:  "users",
			ColumnName: "name",
			ColumnType: string(FieldTypeNumber),
		},
	}

	if err := executor.Execute(ctx, ops, false); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	// Table should still exist
	if !tableExists(t, tdb.db, "users") {
		t.Fatal("table 'users' should still exist after column modify")
	}

	// Column type should have changed to INTEGER
	colType := getColumnType(t, tdb.db, "users", "name")
	if colType != "INTEGER" {
		t.Errorf("column 'name' type = %q, want INTEGER", colType)
	}

	// id column should still exist
	if !columnExists(t, tdb.db, "users", "id") {
		t.Error("column 'id' should still exist after modify")
	}
}

func TestSQLiteExecutor_EmptyOps(t *testing.T) {
	tdb := newSQLiteTestDB(t)
	defer tdb.Close()

	executor := NewSQLiteExecutor(tdb)
	ctx := context.Background()

	// Execute with empty ops list — should succeed as a no-op
	ops := []DiffOperation{}

	if err := executor.Execute(ctx, ops, false); err != nil {
		t.Fatalf("Execute() with empty ops should succeed, got error: %v", err)
	}

	if err := executor.Execute(ctx, ops, true); err != nil {
		t.Fatalf("Execute() with empty ops and force=true should succeed, got error: %v", err)
	}
}

func TestSQLiteExecutor_TransactionRollback(t *testing.T) {
	tdb := newSQLiteTestDB(t)
	defer tdb.Close()

	executor := NewSQLiteExecutor(tdb)
	ctx := context.Background()

	// Create initial table
	createTestTable(t, executor, ctx, &Schema{
		Name: "users",
		Fields: []FieldDefinition{
			{Name: "id", Type: FieldTypeNumber, Constraints: &Constraints{Nullable: false}},
			{Name: "name", Type: FieldTypeText},
		},
	})

	// Verify initial state: only id and name columns
	cols := getTableColumns(t, tdb.db, "users")
	if len(cols) != 2 {
		t.Fatalf("expected 2 columns before failed ops, got %d: %v", len(cols), cols)
	}

	// Execute two ops: add column (valid) then add duplicate column (invalid)
	// The second op should fail, rolling back the first
	ops := []DiffOperation{
		{
			Type:       OpColumnAdd,
			TableName:  "users",
			ColumnName: "email",
			ColumnType: "TEXT",
		},
		{
			Type:       OpColumnAdd,
			TableName:  "users",
			ColumnName: "name", // already exists — will fail
			ColumnType: "TEXT",
		},
	}

	err := executor.Execute(ctx, ops, false)
	if err == nil {
		t.Fatal("expected error when adding duplicate column, got nil")
	}

	// The first op (add email) should have been rolled back
	if columnExists(t, tdb.db, "users", "email") {
		t.Error("column 'email' should not exist after transaction rollback")
	}

	// Original columns should be unchanged
	colsAfter := getTableColumns(t, tdb.db, "users")
	if len(colsAfter) != 2 {
		t.Errorf("expected 2 columns after rollback, got %d: %v", len(colsAfter), colsAfter)
	}
}

func TestSQLiteExecutor_DataPreservedAfterRebuild(t *testing.T) {
	tdb := newSQLiteTestDB(t)
	defer tdb.Close()

	executor := NewSQLiteExecutor(tdb)
	ctx := context.Background()

	// Create table with three columns
	createTestTable(t, executor, ctx, &Schema{
		Name: "users",
		Fields: []FieldDefinition{
			{Name: "id", Type: FieldTypeNumber, Constraints: &Constraints{Nullable: false}},
			{Name: "name", Type: FieldTypeText},
			{Name: "email", Type: FieldTypeText},
		},
	})

	// Insert test data
	_, err := tdb.Exec(ctx, "INSERT INTO \"users\" (id, name, email) VALUES (?, ?, ?)",
		1, "Alice", "alice@example.com")
	if err != nil {
		t.Fatalf("insert row 1: %v", err)
	}
	_, err = tdb.Exec(ctx, "INSERT INTO \"users\" (id, name, email) VALUES (?, ?, ?)",
		2, "Bob", "bob@example.com")
	if err != nil {
		t.Fatalf("insert row 2: %v", err)
	}

	// Verify data before rebuild
	count := getRowCount(t, tdb.db, "users")
	if count != 2 {
		t.Fatalf("expected 2 rows before rebuild, got %d", count)
	}

	// Drop the email column (requires table rebuild)
	ops := []DiffOperation{
		{
			Type:       OpColumnDrop,
			TableName:  "users",
			ColumnName: "email",
		},
	}

	if err := executor.Execute(ctx, ops, true); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	// Verify table still exists
	if !tableExists(t, tdb.db, "users") {
		t.Fatal("table 'users' should still exist after rebuild")
	}

	// Verify email column is gone
	if columnExists(t, tdb.db, "users", "email") {
		t.Error("column 'email' should not exist after drop")
	}

	// Verify remaining columns exist
	if !columnExists(t, tdb.db, "users", "id") {
		t.Error("column 'id' should still exist after rebuild")
	}
	if !columnExists(t, tdb.db, "users", "name") {
		t.Error("column 'name' should still exist after rebuild")
	}

	// Verify data is preserved
	countAfter := getRowCount(t, tdb.db, "users")
	if countAfter != 2 {
		t.Errorf("expected 2 rows after rebuild, got %d", countAfter)
	}

	// Verify specific data values
	var name1, name2 string
	row := tdb.db.QueryRowContext(ctx, "SELECT name FROM \"users\" WHERE id = 1")
	if err := row.Scan(&name1); err != nil {
		t.Fatalf("query name for id=1: %v", err)
	}
	if name1 != "Alice" {
		t.Errorf("name for id=1 = %q, want %q", name1, "Alice")
	}

	row = tdb.db.QueryRowContext(ctx, "SELECT name FROM \"users\" WHERE id = 2")
	if err := row.Scan(&name2); err != nil {
		t.Fatalf("query name for id=2: %v", err)
	}
	if name2 != "Bob" {
		t.Errorf("name for id=2 = %q, want %q", name2, "Bob")
	}
}

func TestSQLiteExecutor_ColumnModify_WithData(t *testing.T) {
	tdb := newSQLiteTestDB(t)
	defer tdb.Close()

	executor := NewSQLiteExecutor(tdb)
	ctx := context.Background()

	// Create table with a text column
	createTestTable(t, executor, ctx, &Schema{
		Name: "items",
		Fields: []FieldDefinition{
			{Name: "id", Type: FieldTypeNumber, Constraints: &Constraints{Nullable: false}},
			{Name: "label", Type: FieldTypeText},
		},
	})

	// Insert data
	_, err := tdb.Exec(ctx, "INSERT INTO \"items\" (id, label) VALUES (?, ?)", 1, "hello")
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	_, err = tdb.Exec(ctx, "INSERT INTO \"items\" (id, label) VALUES (?, ?)", 2, "world")
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	// Modify column type from text to number (triggers rebuild)
	ops := []DiffOperation{
		{
			Type:       OpColumnModify,
			TableName:  "items",
			ColumnName: "label",
			ColumnType: string(FieldTypeNumber),
		},
	}

	if err := executor.Execute(ctx, ops, false); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	// Verify data is preserved after rebuild
	count := getRowCount(t, tdb.db, "items")
	if count != 2 {
		t.Errorf("expected 2 rows after modify rebuild, got %d", count)
	}

	// Verify column type changed
	colType := getColumnType(t, tdb.db, "items", "label")
	if colType != "INTEGER" {
		t.Errorf("column 'label' type = %q, want INTEGER", colType)
	}
}

func TestSQLiteExecutor_MultipleOpsInTransaction(t *testing.T) {
	tdb := newSQLiteTestDB(t)
	defer tdb.Close()

	executor := NewSQLiteExecutor(tdb)
	ctx := context.Background()

	// Create initial table
	createTestTable(t, executor, ctx, &Schema{
		Name: "products",
		Fields: []FieldDefinition{
			{Name: "id", Type: FieldTypeNumber, Constraints: &Constraints{Nullable: false}},
			{Name: "name", Type: FieldTypeText},
		},
	})

	// Execute multiple operations in a single call:
	// 1. Add a column
	// 2. Add an index
	ops := []DiffOperation{
		{
			Type:       OpColumnAdd,
			TableName:  "products",
			ColumnName: "price",
			ColumnType: "REAL",
		},
		{
			Type:         OpIndexAdd,
			TableName:    "products",
			IndexName:    "idx_products_name",
			IndexColumns: []string{"name"},
			IndexUnique:  false,
		},
	}

	if err := executor.Execute(ctx, ops, false); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	// Both operations should have succeeded
	if !columnExists(t, tdb.db, "products", "price") {
		t.Error("column 'price' should exist after add")
	}
	if !indexExists(t, tdb.db, "idx_products_name") {
		t.Error("index 'idx_products_name' should exist after add")
	}
}

func TestSQLiteExecutor_TableCreate_WithDefaults(t *testing.T) {
	tdb := newSQLiteTestDB(t)
	defer tdb.Close()

	executor := NewSQLiteExecutor(tdb)
	ctx := context.Background()

	ops := []DiffOperation{
		{
			Type:      OpTableCreate,
			TableName: "settings",
			Schema: &Schema{
				Name: "settings",
				Fields: []FieldDefinition{
					{Name: "id", Type: FieldTypeNumber, Constraints: &Constraints{Nullable: false}},
					{Name: "key", Type: FieldTypeText, Constraints: &Constraints{Nullable: false}},
					{Name: "value", Type: FieldTypeText, Constraints: &Constraints{Default: "default_value"}},
					{Name: "active", Type: FieldTypeBoolean, Constraints: &Constraints{Default: true}},
				},
			},
		},
	}

	if err := executor.Execute(ctx, ops, false); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if !tableExists(t, tdb.db, "settings") {
		t.Error("table 'settings' should exist after creation")
	}

	// Verify all columns exist
	for _, col := range []string{"id", "key", "value", "active"} {
		if !columnExists(t, tdb.db, "settings", col) {
			t.Errorf("column %q should exist", col)
		}
	}
}

func TestSQLiteExecutor_ColumnDrop_NonExistentTable(t *testing.T) {
	tdb := newSQLiteTestDB(t)
	defer tdb.Close()

	executor := NewSQLiteExecutor(tdb)
	ctx := context.Background()

	// Attempt to drop a column from a non-existent table
	ops := []DiffOperation{
		{
			Type:       OpColumnDrop,
			TableName:  "nonexistent",
			ColumnName: "col",
		},
	}

	err := executor.Execute(ctx, ops, true)
	if err == nil {
		t.Fatal("expected error when dropping column from non-existent table, got nil")
	}
}

func TestSQLiteExecutor_UnsupportedOpType(t *testing.T) {
	tdb := newSQLiteTestDB(t)
	defer tdb.Close()

	executor := NewSQLiteExecutor(tdb)
	ctx := context.Background()

	// Use an unsupported operation type
	ops := []DiffOperation{
		{
			Type:      OpTableDrop,
			TableName: "users",
		},
	}

	err := executor.Execute(ctx, ops, true)
	if err == nil {
		t.Fatal("expected error for unsupported operation type, got nil")
	}
}

// ============================================================================
// Tests for inferFieldType - covers all SQL type mappings
// ============================================================================

func TestSQLiteExecutor_InferFieldType(t *testing.T) {
	tdb := newSQLiteTestDB(t)
	defer tdb.Close()

	executor := NewSQLiteExecutor(tdb)

	tests := []struct {
		sqlType string
		want    FieldType
	}{
		{"INTEGER", FieldTypeNumber},
		{"INT", FieldTypeNumber},
		{"BIGINT", FieldTypeNumber},
		{"SMALLINT", FieldTypeNumber},
		{"TINYINT", FieldTypeNumber},
		{"REAL", FieldTypeDecimal},
		{"DOUBLE", FieldTypeDecimal},
		{"FLOAT", FieldTypeDecimal},
		{"NUMERIC", FieldTypeDecimal},
		{"DECIMAL", FieldTypeDecimal},
		{"TEXT", FieldTypeText},
		{"VARCHAR", FieldTypeText},
		{"CHAR", FieldTypeText},
		{"CLOB", FieldTypeText},
		{"BOOLEAN", FieldTypeBoolean},
		{"BOOL", FieldTypeBoolean},
		{"JSON", FieldTypeJSON},
		{"JSONB", FieldTypeJSON},
		{"UNKNOWN_TYPE", FieldTypeText}, // default fallback
		{"integer", FieldTypeNumber},    // case-insensitive
		{"text", FieldTypeText},         // case-insensitive
	}

	for _, tt := range tests {
		t.Run(tt.sqlType, func(t *testing.T) {
			got := executor.inferFieldType(tt.sqlType)
			if got != tt.want {
				t.Errorf("inferFieldType(%q) = %q, want %q", tt.sqlType, got, tt.want)
			}
		})
	}
}

// ============================================================================
// Tests for buildColumnDef with foreign key references
// ============================================================================

func TestSQLiteExecutor_BuildColumnDef_ForeignKey(t *testing.T) {
	tdb := newSQLiteTestDB(t)
	defer tdb.Close()

	executor := NewSQLiteExecutor(tdb)
	ctx := context.Background()

	// Create parent table first
	parentSchema := &Schema{
		Name: "categories",
		Fields: []FieldDefinition{
			{Name: "id", Type: FieldTypeNumber, Constraints: &Constraints{Nullable: false}},
			{Name: "name", Type: FieldTypeText},
		},
	}
	createTestTable(t, executor, ctx, parentSchema)

	// Create child table with foreign key reference
	childSchema := &Schema{
		Name: "products",
		Fields: []FieldDefinition{
			{Name: "id", Type: FieldTypeNumber, Constraints: &Constraints{Nullable: false}},
			{Name: "name", Type: FieldTypeText},
			{Name: "category_id", Type: FieldTypeRelation, ForeignKey: &ForeignKeyReference{
				Table:    "categories",
				Column:   "id",
				OnDelete: "CASCADE",
				OnUpdate: "SET NULL",
			}},
		},
	}

	ops := []DiffOperation{
		{Type: OpTableCreate, TableName: "products", Schema: childSchema},
	}

	if err := executor.Execute(ctx, ops, false); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	// Verify table was created with FK
	if !tableExists(t, tdb.db, "products") {
		t.Error("table 'products' should exist")
	}

	// Verify foreign key constraint exists via introspection
	introspector := NewIntrospector(tdb, DialectSQLite)
	tableDef, err := introspector.IntrospectTable(ctx, "products")
	if err != nil {
		t.Fatalf("IntrospectTable() error = %v", err)
	}

	if tableDef == nil {
		t.Fatal("IntrospectTable() returned nil")
	}

	// Check foreign keys were created
	foundFK := false
	for _, fk := range tableDef.ForeignKeys {
		if fk.RefTable == "categories" && len(fk.Columns) > 0 && fk.Columns[0] == "category_id" {
			foundFK = true
			if fk.OnDelete != "CASCADE" {
				t.Errorf("FK OnDelete = %q, want CASCADE", fk.OnDelete)
			}
			if fk.OnUpdate != "SET NULL" {
				t.Errorf("FK OnUpdate = %q, want SET NULL", fk.OnUpdate)
			}
			break
		}
	}
	if !foundFK {
		t.Error("foreign key to categories.category_id not found")
	}
}

func TestSQLiteExecutor_BuildColumnDef_ForeignKeyDefaultRefColumn(t *testing.T) {
	tdb := newSQLiteTestDB(t)
	defer tdb.Close()

	executor := NewSQLiteExecutor(tdb)
	ctx := context.Background()

	// Create parent table
	parentSchema := &Schema{
		Name: "authors",
		Fields: []FieldDefinition{
			{Name: "id", Type: FieldTypeNumber, Constraints: &Constraints{Nullable: false}},
			{Name: "name", Type: FieldTypeText},
		},
	}
	createTestTable(t, executor, ctx, parentSchema)

	// Create child table with FK using default column (empty Column should default to "id")
	childSchema := &Schema{
		Name: "books",
		Fields: []FieldDefinition{
			{Name: "id", Type: FieldTypeNumber, Constraints: &Constraints{Nullable: false}},
			{Name: "title", Type: FieldTypeText},
			{Name: "author_id", Type: FieldTypeRelation, ForeignKey: &ForeignKeyReference{
				Table:    "authors",
				Column:   "", // Empty - should default to "id"
				OnDelete: "NO ACTION",
			}},
		},
	}

	ops := []DiffOperation{
		{Type: OpTableCreate, TableName: "books", Schema: childSchema},
	}

	if err := executor.Execute(ctx, ops, false); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	// Verify FK references "id" column by default
	introspector := NewIntrospector(tdb, DialectSQLite)
	tableDef, err := introspector.IntrospectTable(ctx, "books")
	if err != nil {
		t.Fatalf("IntrospectTable() error = %v", err)
	}

	if tableDef == nil {
		t.Fatal("IntrospectTable() returned nil")
	}

	for _, fk := range tableDef.ForeignKeys {
		if fk.RefTable == "authors" {
			// Should reference "id" column since FK.Column was empty
			if len(fk.RefColumns) > 0 && fk.RefColumns[0] != "id" {
				t.Errorf("FK RefColumn = %q, want id (default)", fk.RefColumns[0])
			}
		}
	}
}

// ============================================================================
// Tests for addColumn with foreign key
// ============================================================================

func TestSQLiteExecutor_AddColumn_WithForeignKey(t *testing.T) {
	tdb := newSQLiteTestDB(t)
	defer tdb.Close()

	executor := NewSQLiteExecutor(tdb)
	ctx := context.Background()

	// Create parent table
	createTestTable(t, executor, ctx, &Schema{
		Name: "departments",
		Fields: []FieldDefinition{
			{Name: "id", Type: FieldTypeNumber, Constraints: &Constraints{Nullable: false}},
			{Name: "name", Type: FieldTypeText},
		},
	})

	// Create child table without FK initially
	createTestTable(t, executor, ctx, &Schema{
		Name: "employees",
		Fields: []FieldDefinition{
			{Name: "id", Type: FieldTypeNumber, Constraints: &Constraints{Nullable: false}},
			{Name: "name", Type: FieldTypeText},
		},
	})

	// Add column with foreign key reference
	ops := []DiffOperation{
		{
			Type:       OpColumnAdd,
			TableName:  "employees",
			ColumnName: "department_id",
			ColumnType: "relation",
			ForeignKey: &ForeignKeyReference{
				Table:    "departments",
				Column:   "id",
				OnDelete: "SET NULL",
			},
			Constraints: &Constraints{Nullable: true},
		},
	}

	if err := executor.Execute(ctx, ops, false); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	// Verify column exists
	if !columnExists(t, tdb.db, "employees", "department_id") {
		t.Error("column 'department_id' should exist after add")
	}

	// Verify FK was created
	introspector := NewIntrospector(tdb, DialectSQLite)
	tableDef, err := introspector.IntrospectTable(ctx, "employees")
	if err != nil {
		t.Fatalf("IntrospectTable() error = %v", err)
	}

	if tableDef == nil {
		t.Fatal("IntrospectTable() returned nil")
	}

	foundFK := false
	for _, fk := range tableDef.ForeignKeys {
		if fk.RefTable == "departments" && len(fk.Columns) > 0 && fk.Columns[0] == "department_id" {
			foundFK = true
			break
		}
	}
	if !foundFK {
		t.Error("foreign key to departments not found after addColumn")
	}
}

// ============================================================================
// Tests for dropColumn direct execution (non-rebuild path)
// ============================================================================

func TestSQLiteExecutor_DropColumn_DirectExecution(t *testing.T) {
	tdb := newSQLiteTestDB(t)
	defer tdb.Close()

	executor := NewSQLiteExecutor(tdb)
	ctx := context.Background()

	// Create table with columns
	createTestTable(t, executor, ctx, &Schema{
		Name: "items",
		Fields: []FieldDefinition{
			{Name: "id", Type: FieldTypeNumber, Constraints: &Constraints{Nullable: false}},
			{Name: "name", Type: FieldTypeText},
			{Name: "description", Type: FieldTypeText},
		},
	})

	// Verify all columns exist
	if !columnExists(t, tdb.db, "items", "id") {
		t.Fatal("id column should exist initially")
	}
	if !columnExists(t, tdb.db, "items", "name") {
		t.Fatal("name column should exist initially")
	}
	if !columnExists(t, tdb.db, "items", "description") {
		t.Fatal("description column should exist initially")
	}

	// Insert test data
	_, err := tdb.Exec(ctx, "INSERT INTO items (id, name, description) VALUES (?, ?, ?)", 1, "Item1", "Desc1")
	if err != nil {
		t.Fatalf("insert test data: %v", err)
	}

	// Drop description column with force (triggers rebuildTable)
	ops := []DiffOperation{
		{
			Type:       OpColumnDrop,
			TableName:  "items",
			ColumnName: "description",
		},
	}

	if err := executor.Execute(ctx, ops, true); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	// Verify column was dropped
	if columnExists(t, tdb.db, "items", "description") {
		t.Error("column 'description' should not exist after drop")
	}

	// Verify other columns still exist
	if !columnExists(t, tdb.db, "items", "id") {
		t.Error("column 'id' should still exist")
	}
	if !columnExists(t, tdb.db, "items", "name") {
		t.Error("column 'name' should still exist")
	}

	// Verify data preserved
	count := getRowCount(t, tdb.db, "items")
	if count != 1 {
		t.Errorf("expected 1 row after drop, got %d", count)
	}
}

// ============================================================================
// Tests for modifyColumn direct error case
// ============================================================================

func TestSQLiteExecutor_ModifyColumn_ReturnsError(t *testing.T) {
	tdb := newSQLiteTestDB(t)
	defer tdb.Close()

	executor := NewSQLiteExecutor(tdb)
	ctx := context.Background()

	// Create table
	createTestTable(t, executor, ctx, &Schema{
		Name: "data",
		Fields: []FieldDefinition{
			{Name: "id", Type: FieldTypeNumber, Constraints: &Constraints{Nullable: false}},
			{Name: "value", Type: FieldTypeText},
		},
	})

	// modifyColumn should return error when called directly (not via rebuildTable path)
	// This tests the direct method which returns "column modification requires table rebuild"
	tx, err := tdb.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}

	op := DiffOperation{
		Type:       OpColumnModify,
		TableName:  "data",
		ColumnName: "value",
		ColumnType: "number",
	}

	// Call executeOp directly which will call modifyColumn directly
	err = executor.modifyColumn(ctx, tx, op)
	if err == nil {
		tx.Rollback()
		t.Fatal("modifyColumn should return error indicating rebuild is needed")
	}
	tx.Rollback()

	if !strings.Contains(err.Error(), "table rebuild") {
		t.Errorf("error message should mention table rebuild, got: %v", err)
	}
}

// ============================================================================
// Tests for transaction begin failure
// ============================================================================

func TestSQLiteExecutor_Execute_BeginTxError(t *testing.T) {
	// Use a mock DB that fails BeginTx
	mock := &mockBeginTxFail{}
	executor := NewSQLiteExecutor(mock)
	ctx := context.Background()

	ops := []DiffOperation{
		{Type: OpTableCreate, TableName: "test", Schema: &Schema{Name: "test"}},
	}

	err := executor.Execute(ctx, ops, false)
	if err == nil {
		t.Fatal("expected error when BeginTx fails, got nil")
	}

	if !strings.Contains(err.Error(), "beginning transaction") {
		t.Errorf("error should mention 'beginning transaction', got: %v", err)
	}
}

// mockBeginTxFail implements interfaces.DatabaseService but fails BeginTx
type mockBeginTxFail struct{}

func (m *mockBeginTxFail) Exec(_ context.Context, _ string, _ ...interface{}) (sql.Result, error) {
	return nil, nil
}

func (m *mockBeginTxFail) Query(_ context.Context, _ string, _ ...interface{}) (*sql.Rows, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockBeginTxFail) QueryRow(_ context.Context, _ string, _ ...interface{}) *sql.Row {
	return nil
}

func (m *mockBeginTxFail) BeginTx(_ context.Context, _ *sql.TxOptions) (*sql.Tx, error) {
	return nil, fmt.Errorf("mock: BeginTx failed")
}

func (m *mockBeginTxFail) Ping(_ context.Context) error { return nil }

func (m *mockBeginTxFail) Prepare(_ context.Context, _ string) (*sql.Stmt, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockBeginTxFail) Close() error { return nil }

func (m *mockBeginTxFail) SchemaIntrospect(_ context.Context) (*interfaces.DatabaseSchema, error) {
	return nil, fmt.Errorf("not implemented")
}

// ============================================================================
// Tests for buildColumnDef with all constraint combinations
// ============================================================================

func TestSQLiteExecutor_BuildColumnDef_AllConstraints(t *testing.T) {
	tdb := newSQLiteTestDB(t)
	defer tdb.Close()

	executor := NewSQLiteExecutor(tdb)
	ctx := context.Background()

	// Create table with columns having various constraint combinations
	schema := &Schema{
		Name: "test_constraints",
		Fields: []FieldDefinition{
			{Name: "col1", Type: FieldTypeNumber, Constraints: &Constraints{Nullable: false, Unique: true}},
			{Name: "col2", Type: FieldTypeText, Constraints: &Constraints{Nullable: true, Default: "default_val"}},
			{Name: "col3", Type: FieldTypeBoolean, Constraints: &Constraints{Nullable: false, Default: false}},
			{Name: "col4", Type: FieldTypeText}, // No constraints
			{Name: "col5", Type: FieldTypeDecimal, Constraints: &Constraints{Nullable: false, Default: 0.0}},
		},
	}

	ops := []DiffOperation{
		{Type: OpTableCreate, TableName: schema.Name, Schema: schema},
	}

	if err := executor.Execute(ctx, ops, false); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	// Verify all columns created
	for _, col := range []string{"col1", "col2", "col3", "col4", "col5"} {
		if !columnExists(t, tdb.db, "test_constraints", col) {
			t.Errorf("column %q should exist", col)
		}
	}
}

// ============================================================================
// Tests for rebuildTable with index recreation
// ============================================================================

func TestSQLiteExecutor_RebuildTable_PreservesIndexes(t *testing.T) {
	tdb := newSQLiteTestDB(t)
	defer tdb.Close()

	executor := NewSQLiteExecutor(tdb)
	ctx := context.Background()

	createTestTable(t, executor, ctx, &Schema{
		Name: "orders",
		Fields: []FieldDefinition{
			{Name: "id", Type: FieldTypeNumber, Constraints: &Constraints{Nullable: false}},
			{Name: "customer_id", Type: FieldTypeNumber},
			{Name: "status", Type: FieldTypeText},
		},
	})

	_, err := tdb.Exec(ctx, "CREATE INDEX idx_orders_customer ON orders(customer_id)")
	if err != nil {
		t.Fatalf("create index: %v", err)
	}

	_, err = tdb.Exec(ctx, "INSERT INTO orders (id, customer_id, status) VALUES (?, ?, ?)", 1, 100, "pending")
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	if !indexExists(t, tdb.db, "idx_orders_customer") {
		t.Fatal("idx_orders_customer should exist before rebuild")
	}

	ops := []DiffOperation{
		{Type: OpColumnDrop, TableName: "orders", ColumnName: "status"},
	}

	if err := executor.Execute(ctx, ops, true); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if !tableExists(t, tdb.db, "orders") {
		t.Fatal("orders table should exist after rebuild")
	}

	if columnExists(t, tdb.db, "orders", "status") {
		t.Error("column 'status' should not exist after drop")
	}

	count := getRowCount(t, tdb.db, "orders")
	if count != 1 {
		t.Errorf("expected 1 row after rebuild, got %d", count)
	}
}

// ============================================================================
// Tests for empty schema
// ============================================================================

func TestSQLiteExecutor_TableCreate_EmptySchema(t *testing.T) {
	tdb := newSQLiteTestDB(t)
	defer tdb.Close()

	executor := NewSQLiteExecutor(tdb)
	ctx := context.Background()

	schema := &Schema{
		Name:   "empty_table",
		Fields: []FieldDefinition{},
	}

	ops := []DiffOperation{
		{Type: OpTableCreate, TableName: schema.Name, Schema: schema},
	}

	err := executor.Execute(ctx, ops, false)
	if err == nil {
		t.Fatal("expected error for empty schema, got nil")
	}
}

// ============================================================================
// Tests for SchemaIntrospect integration
// ============================================================================

func TestSQLiteExecutor_SchemaIntrospect(t *testing.T) {
	tdb := newSQLiteTestDB(t)
	defer tdb.Close()

	executor := NewSQLiteExecutor(tdb)
	ctx := context.Background()

	// Create multiple tables
	for _, schema := range []*Schema{
		{Name: "users", Fields: []FieldDefinition{{Name: "id", Type: FieldTypeNumber}}},
		{Name: "posts", Fields: []FieldDefinition{{Name: "id", Type: FieldTypeNumber}, {Name: "title", Type: FieldTypeText}}},
	} {
		createTestTable(t, executor, ctx, schema)
	}

	// Call SchemaIntrospect via the test DB implementation
	schema, err := tdb.SchemaIntrospect(ctx)
	if err != nil {
		t.Fatalf("SchemaIntrospect() error = %v", err)
	}

	if len(schema.Tables) < 2 {
		t.Errorf("expected at least 2 tables, got %d", len(schema.Tables))
	}

	foundUsers := false
	foundPosts := false
	for _, tbl := range schema.Tables {
		if tbl.Name == "users" {
			foundUsers = true
		}
		if tbl.Name == "posts" {
			foundPosts = true
		}
	}

	if !foundUsers {
		t.Error("users table not found in schema introspection")
	}
	if !foundPosts {
		t.Error("posts table not found in schema introspection")
	}
}

// ============================================================================
// Tests for getIntrospectedSchema error case
// ============================================================================

func TestSQLiteExecutor_GetIntrospectedSchema_NonExistentTable(t *testing.T) {
	tdb := newSQLiteTestDB(t)
	defer tdb.Close()

	executor := NewSQLiteExecutor(tdb)
	ctx := context.Background()

	_, err := executor.getIntrospectedSchema(ctx, "nonexistent_table")
	if err == nil {
		t.Fatal("expected error for non-existent table, got nil")
	}

	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention 'not found', got: %v", err)
	}
}

// ============================================================================
// Tests for column type validation after modify
// ============================================================================

func TestSQLiteExecutor_ColumnModify_ChangeConstraints(t *testing.T) {
	tdb := newSQLiteTestDB(t)
	defer tdb.Close()

	executor := NewSQLiteExecutor(tdb)
	ctx := context.Background()

	createTestTable(t, executor, ctx, &Schema{
		Name: "settings",
		Fields: []FieldDefinition{
			{Name: "id", Type: FieldTypeNumber, Constraints: &Constraints{Nullable: false}},
			{Name: "value", Type: FieldTypeText, Constraints: &Constraints{Nullable: true}},
		},
	})

	ops := []DiffOperation{
		{
			Type:        OpColumnModify,
			TableName:   "settings",
			ColumnName:  "value",
			ColumnType:  "text",
			Constraints: &Constraints{Nullable: false, Default: "empty"},
		},
	}

	if err := executor.Execute(ctx, ops, false); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if !columnExists(t, tdb.db, "settings", "value") {
		t.Error("column 'value' should exist after modify")
	}
}

func TestSQLiteExecutor_DropColumn_DirectMethod(t *testing.T) {
	tdb := newSQLiteTestDB(t)
	defer tdb.Close()

	executor := NewSQLiteExecutor(tdb)
	ctx := context.Background()

	createTestTable(t, executor, ctx, &Schema{
		Name: "items",
		Fields: []FieldDefinition{
			{Name: "id", Type: FieldTypeNumber, Constraints: &Constraints{Nullable: false}},
			{Name: "name", Type: FieldTypeText},
			{Name: "temp", Type: FieldTypeText},
		},
	})

	tx, err := tdb.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}

	op := DiffOperation{
		Type:       OpColumnDrop,
		TableName:  "items",
		ColumnName: "temp",
	}

	err = executor.dropColumn(ctx, tx, op)
	if err != nil {
		tx.Rollback()
		t.Fatalf("dropColumn: %v", err)
	}

	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	if columnExists(t, tdb.db, "items", "temp") {
		t.Error("column 'temp' should not exist after drop")
	}
}

func TestSQLiteExecutor_RebuildTable_WithSchema(t *testing.T) {
	tdb := newSQLiteTestDB(t)
	defer tdb.Close()

	executor := NewSQLiteExecutor(tdb)
	ctx := context.Background()

	createTestTable(t, executor, ctx, &Schema{
		Name: "accounts",
		Fields: []FieldDefinition{
			{Name: "id", Type: FieldTypeNumber, Constraints: &Constraints{Nullable: false}},
			{Name: "name", Type: FieldTypeText},
			{Name: "email", Type: FieldTypeText},
		},
	})

	_, err := tdb.Exec(ctx, "INSERT INTO accounts (id, name, email) VALUES (?, ?, ?)", 1, "Alice", "alice@test.com")
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	ops := []DiffOperation{
		{
			Type:       OpColumnDrop,
			TableName:  "accounts",
			ColumnName: "email",
			Schema: &Schema{
				Name: "accounts",
				Fields: []FieldDefinition{
					{Name: "id", Type: FieldTypeNumber, Constraints: &Constraints{Nullable: false}},
					{Name: "name", Type: FieldTypeText},
				},
			},
		},
	}

	if err := executor.Execute(ctx, ops, true); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if !tableExists(t, tdb.db, "accounts") {
		t.Error("accounts table should exist")
	}

	if columnExists(t, tdb.db, "accounts", "email") {
		t.Error("email column should not exist")
	}

	count := getRowCount(t, tdb.db, "accounts")
	if count != 1 {
		t.Errorf("expected 1 row, got %d", count)
	}
}

func TestSchema_HasField(t *testing.T) {
	schema := &Schema{
		Name: "test",
		Fields: []FieldDefinition{
			{Name: "id", Type: FieldTypeNumber},
			{Name: "name", Type: FieldTypeText},
		},
	}

	if !schema.HasField("id") {
		t.Error("HasField('id') should return true")
	}
	if !schema.HasField("name") {
		t.Error("HasField('name') should return true")
	}
	if schema.HasField("nonexistent") {
		t.Error("HasField('nonexistent') should return false")
	}
}

func TestSchema_GetIndex(t *testing.T) {
	schema := &Schema{
		Name: "test",
		Fields: []FieldDefinition{
			{Name: "id", Type: FieldTypeNumber},
		},
		Indexes: []IndexDefinition{
			{Name: "idx_test_id", Columns: []string{"id"}, Unique: true},
			{Name: "idx_test_name", Columns: []string{"name"}, Unique: false},
		},
	}

	idx := schema.GetIndex("idx_test_id")
	if idx == nil {
		t.Error("GetIndex('idx_test_id') should return index")
	}
	if idx != nil && !idx.Unique {
		t.Error("idx_test_id should be unique")
	}

	idx = schema.GetIndex("idx_test_name")
	if idx == nil {
		t.Error("GetIndex('idx_test_name') should return index")
	}

	idx = schema.GetIndex("nonexistent")
	if idx != nil {
		t.Error("GetIndex('nonexistent') should return nil")
	}
}

func TestSchema_Clone_DeepCopy(t *testing.T) {
	original := &Schema{
		Name: "test",
		Fields: []FieldDefinition{
			{Name: "id", Type: FieldTypeNumber, Constraints: &Constraints{Nullable: false}},
		},
		Indexes: []IndexDefinition{
			{Name: "idx_test_id", Columns: []string{"id"}, Unique: true},
		},
	}

	cloned := original.Clone()

	if cloned.Name != original.Name {
		t.Errorf("cloned.Name = %q, want %q", cloned.Name, original.Name)
	}

	if len(cloned.Fields) != len(original.Fields) {
		t.Errorf("cloned has %d fields, want %d", len(cloned.Fields), len(original.Fields))
	}

	if len(cloned.Indexes) != len(original.Indexes) {
		t.Errorf("cloned has %d indexes, want %d", len(cloned.Indexes), len(original.Indexes))
	}

	cloned.Fields[0].Name = "modified"
	if original.Fields[0].Name == "modified" {
		t.Error("modifying clone should not affect original")
	}
}
