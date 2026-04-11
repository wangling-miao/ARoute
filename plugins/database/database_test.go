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
