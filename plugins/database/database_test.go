package database

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/wangling-miao/aroute/sdk/interfaces"
)

func TestService_QueryExec(t *testing.T) {
	db, err := sql.Open("sqlite", "file:test_query_exec?mode=memory&cache=shared&_pragma=foreign_keys(ON)")
	if err != nil {
		t.Fatalf("Failed to open SQLite: %v", err)
	}
	defer db.Close()

	service := NewService(db, DriverSQLite)
	ctx := context.Background()

	_, err = service.Exec(ctx, "CREATE TABLE test_table (id INTEGER PRIMARY KEY, name TEXT)")
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	_, err = service.Exec(ctx, "INSERT INTO test_table (name) VALUES (?)", "test_value")
	if err != nil {
		t.Fatalf("Failed to insert: %v", err)
	}

	rows, err := service.Query(ctx, "SELECT id, name FROM test_table")
	if err != nil {
		t.Fatalf("Failed to query: %v", err)
	}
	defer rows.Close()

	var id int
	var name string
	if !rows.Next() {
		t.Fatal("Expected one row")
	}
	if err := rows.Scan(&id, &name); err != nil {
		t.Fatalf("Failed to scan: %v", err)
	}

	if name != "test_value" {
		t.Errorf("Expected name=test_value, got %s", name)
	}

	row := service.QueryRow(ctx, "SELECT COUNT(*) FROM test_table")
	var count int
	if err := row.Scan(&count); err != nil {
		t.Fatalf("Failed to scan count: %v", err)
	}
	if count != 1 {
		t.Errorf("Expected count=1, got %d", count)
	}
}

func TestService_Transaction(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:?_pragma=foreign_keys(ON)")
	if err != nil {
		t.Fatalf("Failed to open SQLite: %v", err)
	}
	defer db.Close()

	service := NewService(db, DriverSQLite)
	ctx := context.Background()

	_, err = service.Exec(ctx, "CREATE TABLE tx_test (id INTEGER PRIMARY KEY, value TEXT)")
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	tx, err := service.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}

	_, err = tx.ExecContext(ctx, "INSERT INTO tx_test (value) VALUES (?)", "tx_value")
	if err != nil {
		tx.Rollback()
		t.Fatalf("Failed to insert in transaction: %v", err)
	}

	if err := tx.Commit(); err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}

	row := service.QueryRow(ctx, "SELECT value FROM tx_test WHERE id = 1")
	var value string
	if err := row.Scan(&value); err != nil {
		t.Fatalf("Failed to scan: %v", err)
	}

	if value != "tx_value" {
		t.Errorf("Expected value=tx_value, got %s", value)
	}
}

func TestService_Ping(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open SQLite: %v", err)
	}
	defer db.Close()

	service := NewService(db, DriverSQLite)
	ctx := context.Background()

	if err := service.Ping(ctx); err != nil {
		t.Errorf("Ping failed: %v", err)
	}
}

func TestService_SchemaIntrospect_SQLite(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:?_pragma=foreign_keys(ON)")
	if err != nil {
		t.Fatalf("Failed to open SQLite: %v", err)
	}
	defer db.Close()

	service := NewService(db, DriverSQLite)
	ctx := context.Background()

	_, err = service.Exec(ctx, `
		CREATE TABLE users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			email TEXT UNIQUE,
			created_at TEXT
		)
	`)
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	_, err = service.Exec(ctx, "CREATE INDEX idx_users_email ON users(email)")
	if err != nil {
		t.Fatalf("Failed to create index: %v", err)
	}

	schema, err := service.SchemaIntrospect(ctx)
	if err != nil {
		t.Fatalf("Schema introspect failed: %v", err)
	}

	var usersTable *interfaces.TableDefinition
	for _, table := range schema.Tables {
		if table.Name == "users" {
			usersTable = &table
			break
		}
	}

	if usersTable == nil {
		t.Fatal("users table not found in schema")
	}

	expectedColumns := 4
	if len(usersTable.Columns) != expectedColumns {
		t.Errorf("Expected %d columns, got %d", expectedColumns, len(usersTable.Columns))
	}

	var nameCol *interfaces.ColumnDefinition
	for i, col := range usersTable.Columns {
		if col.Name == "name" {
			nameCol = &usersTable.Columns[i]
			break
		}
	}

	if nameCol == nil {
		t.Fatal("name column not found")
	}

	if nameCol.Nullable {
		t.Error("name column should NOT be nullable")
	}

	if len(usersTable.Indexes) < 1 {
		t.Error("Expected at least one index")
	}
}

func TestMigrationRunner_Load(t *testing.T) {
	tmpDir := t.TempDir()

	migration1 := filepath.Join(tmpDir, "2026041301_init.sql")
	content1 := `
CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT);
-- @down
DROP TABLE users;
`
	if err := os.WriteFile(migration1, []byte(content1), 0644); err != nil {
		t.Fatalf("Failed to write migration file: %v", err)
	}

	migration2 := filepath.Join(tmpDir, "2026041302_add_email.sql")
	content2 := `
ALTER TABLE users ADD COLUMN email TEXT;
-- @down
ALTER TABLE users DROP COLUMN email;
`
	if err := os.WriteFile(migration2, []byte(content2), 0644); err != nil {
		t.Fatalf("Failed to write migration file: %v", err)
	}

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open SQLite: %v", err)
	}
	defer db.Close()

	service := NewService(db, DriverSQLite)
	ctx := context.Background()

	_, err = service.Exec(ctx, `
CREATE TABLE IF NOT EXISTS _migrations (
	version TEXT PRIMARY KEY,
	name TEXT NOT NULL,
	applied_at TEXT NOT NULL
)
`)
	if err != nil {
		t.Fatalf("Failed to create migrations table: %v", err)
	}

	runner := NewMigrationRunner(service, tmpDir)
	if err := runner.Load(ctx); err != nil {
		t.Fatalf("Failed to load migrations: %v", err)
	}

	if runner.TotalCount() != 2 {
		t.Errorf("Expected 2 migrations, got %d", runner.TotalCount())
	}

	if runner.PendingCount() != 2 {
		t.Errorf("Expected 2 pending, got %d", runner.PendingCount())
	}

	if runner.AppliedCount() != 0 {
		t.Errorf("Expected 0 applied, got %d", runner.AppliedCount())
	}
}

func TestMigrationRunner_Apply(t *testing.T) {
	tmpDir := t.TempDir()

	migration1 := filepath.Join(tmpDir, "2026041301_init.sql")
	content1 := `
CREATE TABLE test_apply (id INTEGER PRIMARY KEY, name TEXT);
-- @down
DROP TABLE test_apply;
`
	if err := os.WriteFile(migration1, []byte(content1), 0644); err != nil {
		t.Fatalf("Failed to write migration file: %v", err)
	}

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open SQLite: %v", err)
	}
	defer db.Close()

	service := NewService(db, DriverSQLite)
	ctx := context.Background()

	_, err = service.Exec(ctx, `
CREATE TABLE IF NOT EXISTS _migrations (
	version TEXT PRIMARY KEY,
	name TEXT NOT NULL,
	applied_at TEXT NOT NULL
)
`)
	if err != nil {
		t.Fatalf("Failed to create migrations table: %v", err)
	}

	runner := NewMigrationRunner(service, tmpDir)
	if err := runner.Load(ctx); err != nil {
		t.Fatalf("Failed to load migrations: %v", err)
	}

	appliedCount, err := runner.Apply(ctx)
	if err != nil {
		t.Fatalf("Failed to apply migrations: %v", err)
	}

	if appliedCount != 1 {
		t.Errorf("Expected 1 migration applied, got %d", appliedCount)
	}

	rows, err := service.Query(ctx, "SELECT name FROM sqlite_master WHERE type='table' AND name='test_apply'")
	if err != nil {
		t.Fatalf("Failed to query tables: %v", err)
	}
	defer rows.Close()

	if !rows.Next() {
		t.Error("Expected test_apply table to exist")
	}

	appliedCount2, err := runner.Apply(ctx)
	if err != nil {
		t.Fatalf("Second apply failed: %v", err)
	}

	if appliedCount2 != 0 {
		t.Errorf("Expected 0 migrations applied on second run, got %d", appliedCount2)
	}
}

func TestMigrationRunner_Revert(t *testing.T) {
	tmpDir := t.TempDir()

	migration1 := filepath.Join(tmpDir, "2026041301_init.sql")
	content1 := `
CREATE TABLE revert_test (id INTEGER PRIMARY KEY, name TEXT);
-- @down
DROP TABLE revert_test;
`
	if err := os.WriteFile(migration1, []byte(content1), 0644); err != nil {
		t.Fatalf("Failed to write migration file: %v", err)
	}

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open SQLite: %v", err)
	}
	defer db.Close()

	service := NewService(db, DriverSQLite)
	ctx := context.Background()

	_, err = service.Exec(ctx, `
CREATE TABLE IF NOT EXISTS _migrations (
	version TEXT PRIMARY KEY,
	name TEXT NOT NULL,
	applied_at TEXT NOT NULL
)
`)
	if err != nil {
		t.Fatalf("Failed to create migrations table: %v", err)
	}

	runner := NewMigrationRunner(service, tmpDir)
	if err := runner.Load(ctx); err != nil {
		t.Fatalf("Failed to load migrations: %v", err)
	}

	if _, err := runner.Apply(ctx); err != nil {
		t.Fatalf("Failed to apply migrations: %v", err)
	}

	revertedCount, err := runner.Revert(ctx, 1)
	if err != nil {
		t.Fatalf("Failed to revert migrations: %v", err)
	}

	if revertedCount != 1 {
		t.Errorf("Expected 1 migration reverted, got %d", revertedCount)
	}

	rows, err := service.Query(ctx, "SELECT name FROM sqlite_master WHERE type='table' AND name='revert_test'")
	if err != nil {
		t.Fatalf("Failed to query tables: %v", err)
	}
	defer rows.Close()

	if rows.Next() {
		t.Error("Expected revert_test table to be dropped")
	}
}

func TestMigrationRunner_Status(t *testing.T) {
	tmpDir := t.TempDir()

	migration1 := filepath.Join(tmpDir, "2026041301_init.sql")
	content1 := `
CREATE TABLE status_test (id INTEGER PRIMARY KEY);
-- @down
DROP TABLE status_test;
`
	if err := os.WriteFile(migration1, []byte(content1), 0644); err != nil {
		t.Fatalf("Failed to write migration file: %v", err)
	}

	migration2 := filepath.Join(tmpDir, "2026041302_pending.sql")
	content2 := `
-- This migration will remain pending in this test
SELECT 1;
-- @down
SELECT 2;
`
	if err := os.WriteFile(migration2, []byte(content2), 0644); err != nil {
		t.Fatalf("Failed to write migration file: %v", err)
	}

	db, err := sql.Open("sqlite", "file:status_test?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("Failed to open SQLite: %v", err)
	}
	defer db.Close()

	service := NewService(db, DriverSQLite)
	ctx := context.Background()

	_, err = service.Exec(ctx, `
CREATE TABLE IF NOT EXISTS _migrations (
	version TEXT PRIMARY KEY,
	name TEXT NOT NULL,
	applied_at TEXT NOT NULL
)
`)
	if err != nil {
		t.Fatalf("Failed to create migrations table: %v", err)
	}

	runner := NewMigrationRunner(service, tmpDir)
	if err := runner.Load(ctx); err != nil {
		t.Fatalf("Failed to load migrations: %v", err)
	}

	appliedCount, err := runner.Apply(ctx)
	if err != nil {
		t.Fatalf("Failed to apply migrations: %v", err)
	}

	if appliedCount != 2 {
		t.Errorf("Expected 2 migrations applied, got %d", appliedCount)
	}

	status, err := runner.Status(ctx)
	if err != nil {
		t.Fatalf("Failed to get status: %v", err)
	}

	if len(status) != 2 {
		t.Errorf("Expected 2 status entries, got %d", len(status))
	}

	var appliedFound int
	for _, s := range status {
		if s.Status == "applied" {
			appliedFound++
		}
	}

	if appliedFound != 2 {
		t.Errorf("Expected 2 applied migrations in status, got %d", appliedFound)
	}

	pendingCount := runner.PendingCount()
	if pendingCount != 0 {
		t.Errorf("Expected 0 pending migrations, got %d", pendingCount)
	}
}

func TestMaskPassword(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "postgres URL with password",
			input:    "postgres://user:secret@localhost:5432/mydb",
			expected: "postgres://user:****@localhost:5432/mydb",
		},
		{
			name:     "connection string with password= parameter",
			input:    "host=localhost password=mypass user=test",
			expected: "host=localhost password=**** user=test",
		},
		{
			name:     "no password to mask",
			input:    "host=localhost user=test",
			expected: "host=localhost user=test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MaskPassword(tt.input)
			if result != tt.expected {
				t.Errorf("MaskPassword(%s) = %s, want %s", tt.input, result, tt.expected)
			}
		})
	}
}

func TestExtractVersionAndName(t *testing.T) {
	tests := []struct {
		filename string
		version  string
		name     string
	}{
		{"2026041301_init.sql", "2026041301", "init"},
		{"2026041302_create_users.sql", "2026041302", "create_users"},
		{"invalid.sql", "", ""},
		{"1234_short.sql", "1234", "short"},
	}

	for _, tt := range tests {
		version, name := extractVersionAndName(tt.filename)
		if version != tt.version || name != tt.name {
			t.Errorf("extractVersionAndName(%s) = (%s, %s), want (%s, %s)",
				tt.filename, version, name, tt.version, tt.name)
		}
	}
}

func TestSplitSQLStatements(t *testing.T) {
	content := `
CREATE TABLE users (id INTEGER PRIMARY KEY);
INSERT INTO users (name) VALUES ('test');

-- Comment
SELECT * FROM users;
`
	statements := splitSQLStatements(content)

	expectedCount := 3
	if len(statements) != expectedCount {
		t.Errorf("Expected %d statements, got %d", expectedCount, len(statements))
	}
}
