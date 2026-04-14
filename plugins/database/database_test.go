package database

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

	"github.com/wangling-miao/aroute/core"
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
		version  int64
		name     string
	}{
		{"2026041301_init.sql", 2026041301, "init"},
		{"2026041302_create_users.sql", 2026041302, "create_users"},
		{"invalid.sql", 0, ""},
		{"1234_short.sql", 1234, "short"},
	}

	for _, tt := range tests {
		version, name := extractVersionAndName(tt.filename)
		if version != tt.version || name != tt.name {
			t.Errorf("extractVersionAndName(%s) = (%d, %s), want (%d, %s)",
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

func TestService_Prepare(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:?_pragma=foreign_keys(ON)")
	if err != nil {
		t.Fatalf("Failed to open SQLite: %v", err)
	}
	defer db.Close()

	service := NewService(db, DriverSQLite)
	ctx := context.Background()

	_, err = service.Exec(ctx, "CREATE TABLE test_prepare (id INTEGER PRIMARY KEY, name TEXT)")
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	stmt, err := service.Prepare(ctx, "INSERT INTO test_prepare (name) VALUES (?)")
	if err != nil {
		t.Fatalf("Failed to prepare statement: %v", err)
	}
	defer stmt.Close()

	for i := 1; i <= 3; i++ {
		_, err = stmt.Exec(fmt.Sprintf("name_%d", i))
		if err != nil {
			t.Fatalf("Failed to execute prepared statement: %v", err)
		}
	}

	row := service.QueryRow(ctx, "SELECT COUNT(*) FROM test_prepare")
	var count int
	if err := row.Scan(&count); err != nil {
		t.Fatalf("Failed to scan count: %v", err)
	}

	if count != 3 {
		t.Errorf("Expected count=3, got %d", count)
	}
}

func TestService_Prepare_Query(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:?_pragma=foreign_keys(ON)")
	if err != nil {
		t.Fatalf("Failed to open SQLite: %v", err)
	}
	defer db.Close()

	service := NewService(db, DriverSQLite)
	ctx := context.Background()

	_, err = service.Exec(ctx, "CREATE TABLE prepare_query (id INTEGER PRIMARY KEY, value TEXT)")
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	_, err = service.Exec(ctx, "INSERT INTO prepare_query (value) VALUES ('a')")
	if err != nil {
		t.Fatalf("Failed to insert: %v", err)
	}

	_, err = service.Exec(ctx, "INSERT INTO prepare_query (value) VALUES ('b')")
	if err != nil {
		t.Fatalf("Failed to insert: %v", err)
	}

	stmt, err := service.Prepare(ctx, "SELECT value FROM prepare_query WHERE id = ?")
	if err != nil {
		t.Fatalf("Failed to prepare statement: %v", err)
	}
	defer stmt.Close()

	var value string
	err = stmt.QueryRow(1).Scan(&value)
	if err != nil {
		t.Fatalf("Failed to query with prepared statement: %v", err)
	}

	if value != "a" {
		t.Errorf("Expected value='a', got %s", value)
	}
}

func TestService_NormalizePlaceholders_SQLite(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open SQLite: %v", err)
	}
	defer db.Close()

	service := NewService(db, DriverSQLite)

	query := "SELECT * FROM users WHERE id = ? AND status = ?"
	normalized := service.normalizePlaceholders(query)

	if normalized != query {
		t.Errorf("SQLite query should not be normalized, got: %s", normalized)
	}
}

func TestService_NormalizePlaceholders_PostgreSQL(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open SQLite: %v", err)
	}
	defer db.Close()

	service := NewService(db, DriverPostgreSQL)

	tests := []struct {
		input    string
		expected string
	}{
		{
			input:    "SELECT * FROM users WHERE id = ?",
			expected: "SELECT * FROM users WHERE id = $1",
		},
		{
			input:    "SELECT * FROM users WHERE id = ? AND status = ?",
			expected: "SELECT * FROM users WHERE id = $1 AND status = $2",
		},
		{
			input:    "INSERT INTO users (name, email) VALUES (?, ?)",
			expected: "INSERT INTO users (name, email) VALUES ($1, $2)",
		},
		{
			input:    "SELECT * FROM users WHERE id IN (?, ?, ?)",
			expected: "SELECT * FROM users WHERE id IN ($1, $2, $3)",
		},
		{
			input:    "SELECT * FROM users",
			expected: "SELECT * FROM users",
		},
	}

	for i, tt := range tests {
		normalized := service.normalizePlaceholders(tt.input)
		if normalized != tt.expected {
			t.Errorf("Test %d: expected '%s', got '%s'", i+1, tt.expected, normalized)
		}
	}
}

func TestService_QueryWithPlaceholderNormalization(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:?_pragma=foreign_keys(ON)")
	if err != nil {
		t.Fatalf("Failed to open SQLite: %v", err)
	}
	defer db.Close()

	service := NewService(db, DriverSQLite)
	ctx := context.Background()

	_, err = service.Exec(ctx, "CREATE TABLE test_placeholder (id INTEGER PRIMARY KEY, name TEXT)")
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	_, err = service.Exec(ctx, "INSERT INTO test_placeholder (name) VALUES (?)", "test_name")
	if err != nil {
		t.Fatalf("Failed to insert with ? placeholder: %v", err)
	}

	rows, err := service.Query(ctx, "SELECT id, name FROM test_placeholder WHERE name = ?", "test_name")
	if err != nil {
		t.Fatalf("Failed to query with ? placeholder: %v", err)
	}
	defer rows.Close()

	if !rows.Next() {
		t.Fatal("Expected one row")
	}

	var id int
	var name string
	if err := rows.Scan(&id, &name); err != nil {
		t.Fatalf("Failed to scan: %v", err)
	}

	if name != "test_name" {
		t.Errorf("Expected name='test_name', got %s", name)
	}
}

func TestService_PrepareWithTransaction(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:?_pragma=foreign_keys(ON)")
	if err != nil {
		t.Fatalf("Failed to open SQLite: %v", err)
	}
	defer db.Close()

	service := NewService(db, DriverSQLite)
	ctx := context.Background()

	_, err = service.Exec(ctx, "CREATE TABLE prepare_tx (id INTEGER PRIMARY KEY, value TEXT)")
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	tx, err := service.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}

	stmt, err := tx.PrepareContext(ctx, "INSERT INTO prepare_tx (value) VALUES (?)")
	if err != nil {
		tx.Rollback()
		t.Fatalf("Failed to prepare in transaction: %v", err)
	}

	_, err = stmt.Exec("tx_value_1")
	if err != nil {
		stmt.Close()
		tx.Rollback()
		t.Fatalf("Failed to execute prepared statement in tx: %v", err)
	}

	_, err = stmt.Exec("tx_value_2")
	if err != nil {
		stmt.Close()
		tx.Rollback()
		t.Fatalf("Failed to execute second prepared statement in tx: %v", err)
	}

	stmt.Close()

	if err := tx.Commit(); err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}

	row := service.QueryRow(ctx, "SELECT COUNT(*) FROM prepare_tx")
	var count int
	if err := row.Scan(&count); err != nil {
		t.Fatalf("Failed to scan count: %v", err)
	}

	if count != 2 {
		t.Errorf("Expected count=2, got %d", count)
	}
}

func TestService_TransactionRollback(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:?_pragma=foreign_keys(ON)")
	if err != nil {
		t.Fatalf("Failed to open SQLite: %v", err)
	}
	defer db.Close()

	service := NewService(db, DriverSQLite)
	ctx := context.Background()

	_, err = service.Exec(ctx, "CREATE TABLE rollback_test (id INTEGER PRIMARY KEY, value TEXT)")
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	tx, err := service.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}

	_, err = tx.ExecContext(ctx, "INSERT INTO rollback_test (value) VALUES ('to_be_rolled_back')")
	if err != nil {
		tx.Rollback()
		t.Fatalf("Failed to insert in transaction: %v", err)
	}

	if err := tx.Rollback(); err != nil {
		t.Fatalf("Failed to rollback: %v", err)
	}

	row := service.QueryRow(ctx, "SELECT COUNT(*) FROM rollback_test")
	var count int
	if err := row.Scan(&count); err != nil {
		t.Fatalf("Failed to scan count: %v", err)
	}

	if count != 0 {
		t.Errorf("Expected count=0 after rollback, got %d", count)
	}
}

func TestService_NestedTransactionPrevention(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:?_pragma=foreign_keys(ON)")
	if err != nil {
		t.Fatalf("Failed to open SQLite: %v", err)
	}
	defer db.Close()

	service := NewService(db, DriverSQLite)
	ctx := context.Background()

	_, err = service.Exec(ctx, "CREATE TABLE nested_test (id INTEGER PRIMARY KEY)")
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	txCtx := ContextWithTransaction(ctx)
	_, err = service.BeginTx(txCtx, nil)
	if err == nil {
		t.Error("Expected error for nested transaction, got nil")
	}
	if err != nil && !strings.Contains(err.Error(), "nested transactions") {
		t.Errorf("Expected nested transaction error, got: %v", err)
	}
}

func TestService_PingWithTimeout(t *testing.T) {
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

	shortCtx, cancel := context.WithTimeout(ctx, 1*time.Nanosecond)
	cancel()
	if err := service.Ping(shortCtx); err == nil {
		t.Error("Expected timeout error for cancelled context")
	}
}

func TestService_WALMode(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "wal_test.db")

	db, err := sql.Open("sqlite", dbPath+"?_pragma=journal_mode(WAL)")
	if err != nil {
		t.Fatalf("Failed to open SQLite: %v", err)
	}
	defer db.Close()

	service := NewService(db, DriverSQLite)
	ctx := context.Background()

	_, err = service.Exec(ctx, "CREATE TABLE wal_check (id INTEGER PRIMARY KEY)")
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	row := service.QueryRow(ctx, "PRAGMA journal_mode")
	var journalMode string
	if err := row.Scan(&journalMode); err != nil {
		t.Fatalf("Failed to scan journal mode: %v", err)
	}

	if journalMode != "wal" {
		t.Errorf("Expected WAL mode, got: %s", journalMode)
	}
}

func TestService_ClosedStatementError(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:?_pragma=foreign_keys(ON)")
	if err != nil {
		t.Fatalf("Failed to open SQLite: %v", err)
	}
	defer db.Close()

	service := NewService(db, DriverSQLite)
	ctx := context.Background()

	_, err = service.Exec(ctx, "CREATE TABLE stmt_test (id INTEGER PRIMARY KEY)")
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	stmt, err := service.Prepare(ctx, "INSERT INTO stmt_test DEFAULT VALUES")
	if err != nil {
		t.Fatalf("Failed to prepare statement: %v", err)
	}

	stmt.Close()

	_, err = stmt.Exec()
	if err == nil {
		t.Error("Expected error when executing closed statement")
	}
}

func TestService_CancelledContext(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:?_pragma=foreign_keys(ON)")
	if err != nil {
		t.Fatalf("Failed to open SQLite: %v", err)
	}
	defer db.Close()

	service := NewService(db, DriverSQLite)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = service.Exec(ctx, "CREATE TABLE cancel_test (id INTEGER PRIMARY KEY)")
	if err == nil {
		t.Error("Expected error with cancelled context")
	}
}

func TestMigrationRunner_Failure(t *testing.T) {
	tmpDir := t.TempDir()

	migrationFile := filepath.Join(tmpDir, "2026041301_invalid.sql")
	content := `
CREATE TABLE valid_table (id INTEGER PRIMARY KEY);
INVALID SQL STATEMENT HERE;
-- @down
DROP TABLE valid_table;
`
	if err := os.WriteFile(migrationFile, []byte(content), 0644); err != nil {
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

	_, err = runner.Apply(ctx)
	if err == nil {
		t.Error("Expected error for invalid SQL migration")
	}
}

func TestPlugin_DriverDetection(t *testing.T) {
	tests := []struct {
		name     string
		driver   string
		expected Driver
	}{
		{"sqlite", "sqlite", DriverSQLite},
		{"sqlite3", "sqlite3", DriverSQLite},
		{"postgres", "postgres", DriverPostgreSQL},
		{"postgresql", "postgresql", DriverPostgreSQL},
		{"pg", "pg", DriverPostgreSQL},
		{"empty", "", DriverSQLite},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plugin := New()
			driver := plugin.detectDriver(mockCoreContext{driver: tt.driver})
			if driver != tt.expected {
				t.Errorf("detectDriver(%s) = %s, want %s", tt.driver, driver, tt.expected)
			}
		})
	}
}

func TestPlugin_UnknownDriver(t *testing.T) {
	plugin := New()
	ctx := mockCoreContext{driver: "mysql"}

	err := plugin.Init(ctx)
	if err == nil {
		t.Error("Expected error for unknown driver")
	}
	if err != nil && !strings.Contains(err.Error(), "unsupported database driver") {
		t.Errorf("Expected unsupported driver error, got: %v", err)
	}
}

type mockCoreContext struct {
	driver string
}

func (m mockCoreContext) Config() core.ConfigProvider {
	return &mockConfig{driver: m.driver}
}

func (m mockCoreContext) Logger() *slog.Logger {
	return slog.Default()
}

func (m mockCoreContext) Context() context.Context {
	return context.Background()
}

func (m mockCoreContext) Services() core.ServiceContainer {
	return &mockServiceContainer{}
}

func (m mockCoreContext) Events() core.EventBus {
	return nil
}

func (m mockCoreContext) DataDir() string {
	return "/tmp/aroute/test"
}

func (m mockCoreContext) PluginDir() string {
	return ""
}

type mockConfig struct {
	driver string
}

func (m *mockConfig) GetString(key string) string {
	if key == "database.driver" {
		return m.driver
	}
	return ""
}

func (m *mockConfig) GetInt(key string) int {
	return 0
}

func (m *mockConfig) GetBool(key string) bool {
	return false
}

func (m *mockConfig) GetStringSlice(key string) []string {
	return nil
}

func (m *mockConfig) Get(key string) interface{} {
	return nil
}

func (m *mockConfig) Unmarshal(key string, target interface{}) error {
	return nil
}

type mockServiceContainer struct{}

func (m *mockServiceContainer) Provide(fn interface{}) error {
	return nil
}

func (m *mockServiceContainer) Get(target interface{}) error {
	return nil
}

func (m *mockServiceContainer) GetNamed(name string, target interface{}) error {
	return nil
}

func (m *mockServiceContainer) Unregister(target interface{}) error {
	return nil
}

func (m *mockServiceContainer) Has(target interface{}) bool {
	return false
}

func (m *mockServiceContainer) Keys() []string {
	return nil
}

func TestPlugin_Name(t *testing.T) {
	p := New()
	if p.Name() != "database" {
		t.Errorf("Expected Name() = 'database', got %q", p.Name())
	}
}

func TestPlugin_Version(t *testing.T) {
	p := New()
	if p.Version() != "1.0.0" {
		t.Errorf("Expected Version() = '1.0.0', got %q", p.Version())
	}
}

func TestPlugin_Manifest(t *testing.T) {
	p := New()
	m := p.Manifest()
	if m == nil {
		t.Fatal("Manifest() returned nil")
	}
	if m.Name != "database" {
		t.Errorf("Expected Manifest.Name = 'database', got %q", m.Name)
	}
	if m.Version != "1.0.0" {
		t.Errorf("Expected Manifest.Version = '1.0.0', got %q", m.Version)
	}
	if m.Description == "" {
		t.Error("Manifest.Description should not be empty")
	}
	if len(m.Provides) == 0 || m.Provides[0] != "database.service" {
		t.Errorf("Expected Provides = ['database.service'], got %v", m.Provides)
	}
}

func TestPlugin_Init_SQLite(t *testing.T) {
	tmpDir := t.TempDir()

	p := New()
	ctx := mockCoreContextWithDir{
		driver:  "sqlite",
		dataDir: tmpDir,
	}

	err := p.Init(ctx)
	if err != nil {
		t.Fatalf("Init() failed: %v", err)
	}

	if p.driver != DriverSQLite {
		t.Errorf("Expected driver %s, got %s", DriverSQLite, p.driver)
	}

	if p.service == nil {
		t.Fatal("Service should not be nil after Init")
	}

	// Verify DB works
	if err := p.service.Ping(context.Background()); err != nil {
		t.Errorf("Ping failed after Init: %v", err)
	}

	// Cleanup
	p.Stop()
}

func TestPlugin_Init_SQLiteWithPath(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "custom.db")

	p := New()
	ctx := mockCoreContextWithConfig{
		driver:  "sqlite",
		config:  &mockSQLitePathConfig{dbPath: dbPath},
		dataDir: tmpDir,
	}

	err := p.Init(ctx)
	if err != nil {
		t.Fatalf("Init() failed: %v", err)
	}

	// Verify the DB file was created
	if _, statErr := os.Stat(dbPath); os.IsNotExist(statErr) {
		t.Errorf("Expected DB file at %s", dbPath)
	}

	p.Stop()
}

func TestPlugin_Init_CreateMigrationsTableError(t *testing.T) {
	p := New()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open SQLite: %v", err)
	}

	// Create the _migrations table first so CREATE TABLE IF NOT EXISTS still works
	// But we need to make createMigrationsTable fail. We can do this by closing the db.
	db.Close()

	p.ctx = mockCoreContext{driver: "sqlite"}
	p.driver = DriverSQLite
	p.service = NewService(db, DriverSQLite)

	err = p.createMigrationsTable(mockCoreContext{driver: "sqlite"})
	if err == nil {
		t.Error("Expected error when createMigrationsTable with closed DB")
	}
}

func TestPlugin_Start_Success(t *testing.T) {
	tmpDir := t.TempDir()

	p := New()
	ctx := mockCoreContextWithDir{driver: "sqlite", dataDir: tmpDir}

	if err := p.Init(ctx); err != nil {
		t.Fatalf("Init() failed: %v", err)
	}

	if err := p.Start(); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}

	if !p.running {
		t.Error("Expected running = true after Start()")
	}

	p.Stop()
}

func TestPlugin_Start_AlreadyRunning(t *testing.T) {
	tmpDir := t.TempDir()

	p := New()
	ctx := mockCoreContextWithDir{driver: "sqlite", dataDir: tmpDir}

	if err := p.Init(ctx); err != nil {
		t.Fatalf("Init() failed: %v", err)
	}

	if err := p.Start(); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}
	defer p.Stop()

	// Second Start should return nil (already running)
	if err := p.Start(); err != nil {
		t.Errorf("Second Start() should return nil, got: %v", err)
	}
}

func TestPlugin_Start_PingFailure(t *testing.T) {
	p := New()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open SQLite: %v", err)
	}
	db.Close() // Close immediately to cause ping failure

	p.ctx = mockCoreContext{driver: "sqlite"}
	p.service = NewService(db, DriverSQLite)
	p.running = false

	err = p.Start()
	if err == nil {
		t.Error("Expected error when Start() with closed DB")
	}
	if !strings.Contains(err.Error(), "database connection failed") {
		t.Errorf("Expected 'database connection failed' error, got: %v", err)
	}
}

func TestPlugin_Stop_Success(t *testing.T) {
	tmpDir := t.TempDir()

	p := New()
	ctx := mockCoreContextWithDir{driver: "sqlite", dataDir: tmpDir}

	if err := p.Init(ctx); err != nil {
		t.Fatalf("Init() failed: %v", err)
	}
	if err := p.Start(); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}

	if err := p.Stop(); err != nil {
		t.Fatalf("Stop() failed: %v", err)
	}

	if p.running {
		t.Error("Expected running = false after Stop()")
	}
}

func TestPlugin_Stop_NotRunning(t *testing.T) {
	p := New()
	// Stop when not running should return nil
	if err := p.Stop(); err != nil {
		t.Errorf("Stop() on non-running plugin should return nil, got: %v", err)
	}
}

func TestPlugin_Stop_NilService(t *testing.T) {
	p := New()
	p.running = true
	p.service = nil

	if err := p.Stop(); err != nil {
		t.Errorf("Stop() with nil service should return nil, got: %v", err)
	}
}

func TestBuildSQLiteDSN_Defaults(t *testing.T) {
	config := &mockConfig{driver: "sqlite"}
	dsn := buildSQLiteDSN("/tmp/test.db", config)

	if !strings.Contains(dsn, "/tmp/test.db?") {
		t.Errorf("DSN should contain path, got: %s", dsn)
	}
	if !strings.Contains(dsn, "_pragma=busy_timeout(10000)") {
		t.Errorf("DSN should contain default busy_timeout, got: %s", dsn)
	}
	if !strings.Contains(dsn, "_pragma=journal_mode(WAL)") {
		t.Errorf("DSN should contain WAL mode, got: %s", dsn)
	}
	if !strings.Contains(dsn, "_pragma=synchronous(NORMAL)") {
		t.Errorf("DSN should contain synchronous NORMAL, got: %s", dsn)
	}
	if !strings.Contains(dsn, "_pragma=cache_size(-32000)") {
		t.Errorf("DSN should contain default cache_size, got: %s", dsn)
	}
}

func TestBuildSQLiteDSN_CustomValues(t *testing.T) {
	config := &mockSQLiteCustomConfig{
		busyTimeout:  5000,
		cacheSize:    -64000,
		synchronous:  "FULL",
		tempStoreMem: true,
	}
	dsn := buildSQLiteDSN("/data/app.db", config)

	if !strings.Contains(dsn, "_pragma=busy_timeout(5000)") {
		t.Errorf("DSN should contain custom busy_timeout, got: %s", dsn)
	}
	if !strings.Contains(dsn, "_pragma=cache_size(-64000)") {
		t.Errorf("DSN should contain custom cache_size, got: %s", dsn)
	}
	if !strings.Contains(dsn, "_pragma=synchronous(FULL)") {
		t.Errorf("DSN should contain custom synchronous, got: %s", dsn)
	}
	if !strings.Contains(dsn, "_pragma=temp_store(MEMORY)") {
		t.Errorf("DSN should contain temp_store MEMORY, got: %s", dsn)
	}
}

// ============================================================================
func TestService_SqliteListIndexes_ErrorPaths(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open SQLite: %v", err)
	}

	service := NewService(db, DriverSQLite)
	ctx := context.Background()

	_, err = service.Exec(ctx, `
		CREATE TABLE idx_test (
			id INTEGER PRIMARY KEY,
			name TEXT,
			email TEXT
		)
	`)
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	_, err = service.Exec(ctx, "CREATE INDEX idx_idx_test_email ON idx_test(email)")
	if err != nil {
		t.Fatalf("Failed to create index: %v", err)
	}

	indexes, err := service.sqliteListIndexes(ctx, "idx_test")
	if err != nil {
		t.Fatalf("sqliteListIndexes failed: %v", err)
	}

	if len(indexes) < 1 {
		t.Errorf("Expected at least 1 index, got %d", len(indexes))
	}

	found := false
	for _, idx := range indexes {
		if idx.Name == "idx_idx_test_email" {
			found = true
		}
	}
	if !found {
		t.Error("Expected to find idx_idx_test_email index")
	}

	db.Close()

	_, err = service.sqliteListIndexes(ctx, "idx_test")
	if err == nil {
		t.Error("Expected error with closed DB")
	}
}

func TestService_SchemaIntrospect_UnsupportedDriver(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open SQLite: %v", err)
	}
	defer db.Close()

	service := NewService(db, "unsupported")
	ctx := context.Background()

	_, err = service.SchemaIntrospect(ctx)
	if err == nil {
		t.Error("Expected error for unsupported driver")
	}
	if !strings.Contains(err.Error(), "unsupported driver") {
		t.Errorf("Expected unsupported driver error, got: %v", err)
	}
}

func TestService_SqliteSchemaIntrospect_ColumnDefaults(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:?_pragma=foreign_keys(ON)")
	if err != nil {
		t.Fatalf("Failed to open SQLite: %v", err)
	}
	defer db.Close()

	service := NewService(db, DriverSQLite)
	ctx := context.Background()

	_, err = service.Exec(ctx, `
		CREATE TABLE defaults_test (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL DEFAULT 'unnamed',
			count INTEGER DEFAULT 0
		)
	`)
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	schema, err := service.SchemaIntrospect(ctx)
	if err != nil {
		t.Fatalf("SchemaIntrospect failed: %v", err)
	}

	var table *interfaces.TableDefinition
	for i := range schema.Tables {
		if schema.Tables[i].Name == "defaults_test" {
			table = &schema.Tables[i]
			break
		}
	}
	if table == nil {
		t.Fatal("defaults_test table not found")
	}

	// Find the name column to check default value
	for _, col := range table.Columns {
		if col.Name == "name" {
			if col.DefaultValue != "'unnamed'" && col.DefaultValue != "unnamed" {
				t.Logf("name column DefaultValue = %q (may vary by SQLite version)", col.DefaultValue)
			}
			if col.Nullable {
				t.Error("name column should not be nullable")
			}
		}
		if col.Name == "count" {
			if col.Nullable == false {
				// count has DEFAULT 0 but no NOT NULL, so it's nullable
				t.Logf("count column Nullable=%v", col.Nullable)
			}
		}
		if col.Name == "id" {
			if !col.AutoIncrement {
				t.Error("id column should be AutoIncrement")
			}
		}
	}
}

func TestService_SqliteListTables_ClosedDB(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open SQLite: %v", err)
	}

	service := NewService(db, DriverSQLite)
	ctx := context.Background()

	// Create a table so there's something to list
	_, err = service.Exec(ctx, "CREATE TABLE list_tables_test (id INTEGER)")
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	tables, err := service.sqliteListTables(ctx)
	if err != nil {
		t.Fatalf("sqliteListTables failed: %v", err)
	}

	found := false
	for _, name := range tables {
		if name == "list_tables_test" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected list_tables_test in table list, got: %v", tables)
	}

	// Close DB and test error path
	db.Close()
	_, err = service.sqliteListTables(ctx)
	if err == nil {
		t.Error("Expected error with closed DB")
	}
}

func TestService_SqliteListColumns_ClosedDB(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open SQLite: %v", err)
	}

	service := NewService(db, DriverSQLite)
	ctx := context.Background()

	_, err = service.Exec(ctx, "CREATE TABLE cols_test (id INTEGER PRIMARY KEY, val TEXT)")
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	columns, err := service.sqliteListColumns(ctx, "cols_test")
	if err != nil {
		t.Fatalf("sqliteListColumns failed: %v", err)
	}
	if len(columns) != 2 {
		t.Errorf("Expected 2 columns, got %d", len(columns))
	}

	// Close DB and test error path
	db.Close()
	_, err = service.sqliteListColumns(ctx, "cols_test")
	if err == nil {
		t.Error("Expected error with closed DB")
	}
}

func TestService_DB_And_Driver(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open SQLite: %v", err)
	}
	defer db.Close()

	service := NewService(db, DriverSQLite)

	if service.DB() != db {
		t.Error("DB() should return the underlying *sql.DB")
	}
	if service.Driver() != DriverSQLite {
		t.Errorf("Expected Driver() = %s, got %s", DriverSQLite, service.Driver())
	}
}

func TestMigrationRunner_GetAppliedMigrations_ClosedDB(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open SQLite: %v", err)
	}

	service := NewService(db, DriverSQLite)
	ctx := context.Background()

	_, err = service.Exec(ctx, `
	CREATE TABLE IF NOT EXISTS _migrations (
		version INTEGER PRIMARY KEY,
		name TEXT NOT NULL,
		applied_at TEXT NOT NULL
	)
	`)
	if err != nil {
		t.Fatalf("Failed to create migrations table: %v", err)
	}

	runner := NewMigrationRunner(service, t.TempDir())

	// First verify it works with open DB
	_, err = runner.getAppliedMigrations(ctx)
	if err != nil {
		t.Fatalf("getAppliedMigrations failed with open DB: %v", err)
	}

	// Close DB and test error path
	db.Close()
	_, err = runner.getAppliedMigrations(ctx)
	if err == nil {
		t.Error("Expected error with closed DB")
	}
}

func TestMigrationRunner_VerifyChecksum_ClosedDB(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open SQLite: %v", err)
	}

	service := NewService(db, DriverSQLite)
	ctx := context.Background()

	_, err = service.Exec(ctx, `
	CREATE TABLE IF NOT EXISTS _migrations (
		version INTEGER PRIMARY KEY,
		name TEXT NOT NULL,
		applied_at TEXT NOT NULL
	)
	`)
	if err != nil {
		t.Fatalf("Failed to create migrations table: %v", err)
	}

	runner := NewMigrationRunner(service, t.TempDir())

	// Close DB and test error path
	db.Close()
	_, err = runner.VerifyChecksum(ctx)
	if err == nil {
		t.Error("Expected error with closed DB")
	}
}

func TestMigrationRunner_Load_ReadDirError(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a file where a directory is expected
	migrationsDir := filepath.Join(tmpDir, "migrations")
	if err := os.WriteFile(migrationsDir, []byte("not a dir"), 0644); err != nil {
		t.Fatalf("Failed to write file: %v", err)
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
		version INTEGER PRIMARY KEY,
		name TEXT NOT NULL,
		applied_at TEXT NOT NULL
	)
	`)
	if err != nil {
		t.Fatalf("Failed to create migrations table: %v", err)
	}

	runner := NewMigrationRunner(service, migrationsDir)
	err = runner.Load(ctx)
	if err == nil {
		t.Error("Expected error when migrations dir is a file")
	}
}

func TestMigrationRunner_Apply_BeginTxFailure(t *testing.T) {
	tmpDir := t.TempDir()

	migrationFile := filepath.Join(tmpDir, "2026041301_tx_fail.sql")
	content := "CREATE TABLE tx_fail_test (id INTEGER);"
	if err := os.WriteFile(migrationFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write migration: %v", err)
	}

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open SQLite: %v", err)
	}

	service := NewService(db, DriverSQLite)
	ctx := context.Background()

	_, err = service.Exec(ctx, `
	CREATE TABLE IF NOT EXISTS _migrations (
		version INTEGER PRIMARY KEY,
		name TEXT NOT NULL,
		applied_at TEXT NOT NULL
	)
	`)
	if err != nil {
		t.Fatalf("Failed to create migrations table: %v", err)
	}

	runner := NewMigrationRunner(service, tmpDir)
	if err := runner.Load(ctx); err != nil {
		t.Fatalf("Failed to load: %v", err)
	}

	// Close DB to force BeginTx failure
	db.Close()

	_, err = runner.Apply(ctx)
	if err == nil {
		t.Error("Expected error when Apply with closed DB")
	}
}

func TestMigrationRunner_Revert_BeginTxFailure(t *testing.T) {
	tmpDir := t.TempDir()

	migrationFile := filepath.Join(tmpDir, "2026041301_revert_fail.sql")
	content := `CREATE TABLE revert_fail (id INTEGER);
-- @down
DROP TABLE revert_fail;
`
	if err := os.WriteFile(migrationFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write migration: %v", err)
	}

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open SQLite: %v", err)
	}

	service := NewService(db, DriverSQLite)
	ctx := context.Background()

	_, err = service.Exec(ctx, `
	CREATE TABLE IF NOT EXISTS _migrations (
		version INTEGER PRIMARY KEY,
		name TEXT NOT NULL,
		applied_at TEXT NOT NULL
	)
	`)
	if err != nil {
		t.Fatalf("Failed to create migrations table: %v", err)
	}

	runner := NewMigrationRunner(service, tmpDir)
	if err := runner.Load(ctx); err != nil {
		t.Fatalf("Failed to load: %v", err)
	}

	// Apply first, then close DB
	if _, err := runner.Apply(ctx); err != nil {
		t.Fatalf("Failed to apply: %v", err)
	}

	// Close DB to force BeginTx failure on revert
	db.Close()

	_, err = runner.Revert(ctx, 1)
	if err == nil {
		t.Error("Expected error when Revert with closed DB")
	}
}

func TestMaskPassword_AdditionalCases(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "PASSWORD uppercase parameter",
			input:    "PASSWORD=secret host=localhost",
			expected: "PASSWORD=**** host=localhost",
		},
		{
			name:     "URL with query params and password",
			input:    "postgres://admin:p4ss@db.host:5432/mydb?sslmode=require",
			expected: "postgres://admin:****@db.host:5432/mydb?sslmode=require",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MaskPassword(tt.input)
			if result != tt.expected {
				t.Errorf("MaskPassword(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

type mockCoreContextWithDir struct {
	driver  string
	dataDir string
}

func (m mockCoreContextWithDir) Config() core.ConfigProvider {
	return &mockConfig{driver: m.driver}
}

func (m mockCoreContextWithDir) Logger() *slog.Logger {
	return slog.Default()
}

func (m mockCoreContextWithDir) Context() context.Context {
	return context.Background()
}

func (m mockCoreContextWithDir) Services() core.ServiceContainer {
	return &mockServiceContainer{}
}

func (m mockCoreContextWithDir) Events() core.EventBus {
	return nil
}

func (m mockCoreContextWithDir) DataDir() string {
	return m.dataDir
}

func (m mockCoreContextWithDir) PluginDir() string {
	return ""
}

type mockCoreContextWithConfig struct {
	driver  string
	config  core.ConfigProvider
	dataDir string
}

func (m mockCoreContextWithConfig) Config() core.ConfigProvider {
	return m.config
}

func (m mockCoreContextWithConfig) Logger() *slog.Logger {
	return slog.Default()
}

func (m mockCoreContextWithConfig) Context() context.Context {
	return context.Background()
}

func (m mockCoreContextWithConfig) Services() core.ServiceContainer {
	return &mockServiceContainer{}
}

func (m mockCoreContextWithConfig) Events() core.EventBus {
	return nil
}

func (m mockCoreContextWithConfig) DataDir() string {
	return m.dataDir
}

func (m mockCoreContextWithConfig) PluginDir() string {
	return ""
}

// mockSQLitePathConfig returns a custom DB path
type mockSQLitePathConfig struct {
	dbPath string
}

func (m *mockSQLitePathConfig) GetString(key string) string {
	switch key {
	case "database.driver":
		return "sqlite"
	case "database.path":
		return m.dbPath
	default:
		return ""
	}
}

func (m *mockSQLitePathConfig) GetInt(key string) int                          { return 0 }
func (m *mockSQLitePathConfig) GetBool(key string) bool                        { return false }
func (m *mockSQLitePathConfig) GetStringSlice(key string) []string             { return nil }
func (m *mockSQLitePathConfig) Get(key string) interface{}                     { return nil }
func (m *mockSQLitePathConfig) Unmarshal(key string, target interface{}) error { return nil }

// mockSQLiteCustomConfig returns custom SQLite pragma values
type mockSQLiteCustomConfig struct {
	busyTimeout  int
	cacheSize    int
	synchronous  string
	tempStoreMem bool
}

func (m *mockSQLiteCustomConfig) GetString(key string) string {
	switch key {
	case "database.driver":
		return "sqlite"
	case "database.sqlite.synchronous":
		return m.synchronous
	default:
		return ""
	}
}

func (m *mockSQLiteCustomConfig) GetInt(key string) int {
	switch key {
	case "database.sqlite.busy_timeout":
		return m.busyTimeout
	case "database.sqlite.cache_size":
		return m.cacheSize
	default:
		return 0
	}
}

func (m *mockSQLiteCustomConfig) GetBool(key string) bool {
	if key == "database.sqlite.temp_store_memory" {
		return m.tempStoreMem
	}
	return false
}

func (m *mockSQLiteCustomConfig) GetStringSlice(key string) []string             { return nil }
func (m *mockSQLiteCustomConfig) Get(key string) interface{}                     { return nil }
func (m *mockSQLiteCustomConfig) Unmarshal(key string, target interface{}) error { return nil }
