package database

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/wangling-miao/aroute/core"
	"github.com/wangling-miao/aroute/sdk/interfaces"
)

// PostgreSQL connection parameters from configs/aroute.yaml
const (
	pgHost     = "localhost"
	pgPort     = 5432
	pgUser     = "aroute"
	pgPassword = "aroute_dev_password"
	pgDBName   = "aroute"
	pgSSLMode  = "disable"
)

// Helper to check if PostgreSQL is available
func postgresAvailable() bool {
	connStr := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
		pgUser, pgPassword, pgHost, pgPort, pgDBName, pgSSLMode)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		return false
	}
	defer pool.Close()

	err = pool.Ping(ctx)
	return err == nil
}

// Create PostgreSQL service for testing
func createPostgresTestService(t *testing.T) (*Service, func()) {
	if !postgresAvailable() {
		t.Skip("PostgreSQL not available at localhost:5432")
	}

	connStr := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
		pgUser, pgPassword, pgHost, pgPort, pgDBName, pgSSLMode)

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		t.Fatalf("Failed to create PostgreSQL pool: %v", err)
	}

	db := stdlib.OpenDBFromPool(pool)
	service := NewService(db, DriverPostgreSQL)

	cleanup := func() {
		service.Close()
		pool.Close()
	}

	return service, cleanup
}

// Create test table with unique name to avoid conflicts
func createTestTableName(t *testing.T) string {
	return fmt.Sprintf("test_%s_%d", strings.ToLower(strings.ReplaceAll(t.Name(), "/", "_")), time.Now().UnixNano()%1000000)
}

// Clean up test table
func cleanupTestTable(t *testing.T, service *Service, tableName string) {
	ctx := context.Background()
	_, err := service.Exec(ctx, fmt.Sprintf("DROP TABLE IF EXISTS %s", tableName))
	if err != nil {
		t.Logf("Warning: failed to cleanup table %s: %v", tableName, err)
	}
}

// ============================================================================
// PostgreSQL Driver Tests
// ============================================================================

func TestPostgreSQL_ServiceCreation(t *testing.T) {
	service, cleanup := createPostgresTestService(t)
	defer cleanup()

	if service.Driver() != DriverPostgreSQL {
		t.Errorf("Expected driver %s, got %s", DriverPostgreSQL, service.Driver())
	}

	if service.DB() == nil {
		t.Error("DB() should not return nil")
	}
}

func TestPostgreSQL_Ping(t *testing.T) {
	service, cleanup := createPostgresTestService(t)
	defer cleanup()

	ctx := context.Background()
	if err := service.Ping(ctx); err != nil {
		t.Errorf("Ping failed: %v", err)
	}

	// Test with timeout context
	timeoutCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := service.Ping(timeoutCtx); err != nil {
		t.Errorf("Ping with timeout failed: %v", err)
	}
}

func TestPostgreSQL_PingWithCancelledContext(t *testing.T) {
	service, cleanup := createPostgresTestService(t)
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := service.Ping(ctx)
	if err == nil {
		t.Error("Expected error with cancelled context")
	}
}

// ============================================================================
// PostgreSQL Query/Exec/QueryRow Tests
// ============================================================================

func TestPostgreSQL_QueryExec(t *testing.T) {
	service, cleanup := createPostgresTestService(t)
	defer cleanup()

	ctx := context.Background()
	tableName := createTestTableName(t)
	defer cleanupTestTable(t, service, tableName)

	// Create table
	createSQL := fmt.Sprintf("CREATE TABLE %s (id SERIAL PRIMARY KEY, name TEXT, created_at TIMESTAMP DEFAULT NOW())", tableName)
	_, err := service.Exec(ctx, createSQL)
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	// Insert using ? placeholder (should be normalized to $1)
	insertSQL := fmt.Sprintf("INSERT INTO %s (name) VALUES (?)", tableName)
	_, err = service.Exec(ctx, insertSQL, "test_name")
	if err != nil {
		t.Fatalf("Failed to insert: %v", err)
	}

	// Insert multiple
	insertSQL2 := fmt.Sprintf("INSERT INTO %s (name) VALUES (?), (?)", tableName)
	_, err = service.Exec(ctx, insertSQL2, "name2", "name3")
	if err != nil {
		t.Fatalf("Failed to insert multiple: %v", err)
	}

	// Query
	querySQL := fmt.Sprintf("SELECT id, name FROM %s ORDER BY id", tableName)
	rows, err := service.Query(ctx, querySQL)
	if err != nil {
		t.Fatalf("Failed to query: %v", err)
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var id int
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			t.Fatalf("Failed to scan: %v", err)
		}
		count++
		if count == 1 && name != "test_name" {
			t.Errorf("Expected first name to be 'test_name', got '%s'", name)
		}
	}

	if count != 3 {
		t.Errorf("Expected 3 rows, got %d", count)
	}

	// QueryRow
	countSQL := fmt.Sprintf("SELECT COUNT(*) FROM %s", tableName)
	row := service.QueryRow(ctx, countSQL)
	var totalCount int
	if err := row.Scan(&totalCount); err != nil {
		t.Fatalf("Failed to scan count: %v", err)
	}
	if totalCount != 3 {
		t.Errorf("Expected count=3, got %d", totalCount)
	}
}

func TestPostgreSQL_QueryWithParameters(t *testing.T) {
	service, cleanup := createPostgresTestService(t)
	defer cleanup()

	ctx := context.Background()
	tableName := createTestTableName(t)
	defer cleanupTestTable(t, service, tableName)

	// Create and populate
	_, err := service.Exec(ctx, fmt.Sprintf("CREATE TABLE %s (id SERIAL PRIMARY KEY, value TEXT)", tableName))
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	values := []string{"alpha", "beta", "gamma", "delta"}
	for _, v := range values {
		_, err = service.Exec(ctx, fmt.Sprintf("INSERT INTO %s (value) VALUES (?)", tableName), v)
		if err != nil {
			t.Fatalf("Failed to insert: %v", err)
		}
	}

	// Query with WHERE using ? placeholder
	rows, err := service.Query(ctx, fmt.Sprintf("SELECT value FROM %s WHERE value = ?", tableName), "beta")
	if err != nil {
		t.Fatalf("Failed to query with parameter: %v", err)
	}
	defer rows.Close()

	if !rows.Next() {
		t.Fatal("Expected one row for 'beta'")
	}
	var result string
	if err := rows.Scan(&result); err != nil {
		t.Fatalf("Failed to scan: %v", err)
	}
	if result != "beta" {
		t.Errorf("Expected 'beta', got '%s'", result)
	}
}

func TestPostgreSQL_QueryRowNotFound(t *testing.T) {
	service, cleanup := createPostgresTestService(t)
	defer cleanup()

	ctx := context.Background()
	tableName := createTestTableName(t)
	defer cleanupTestTable(t, service, tableName)

	_, err := service.Exec(ctx, fmt.Sprintf("CREATE TABLE %s (id SERIAL PRIMARY KEY, value TEXT)", tableName))
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	// Query non-existent row
	row := service.QueryRow(ctx, fmt.Sprintf("SELECT value FROM %s WHERE id = ?", tableName), 999)
	var value string
	err = row.Scan(&value)
	if err != sql.ErrNoRows {
		t.Errorf("Expected sql.ErrNoRows, got: %v", err)
	}
}

func TestPostgreSQL_PrepareStatement(t *testing.T) {
	service, cleanup := createPostgresTestService(t)
	defer cleanup()

	ctx := context.Background()
	tableName := createTestTableName(t)
	defer cleanupTestTable(t, service, tableName)

	_, err := service.Exec(ctx, fmt.Sprintf("CREATE TABLE %s (id SERIAL PRIMARY KEY, value TEXT)", tableName))
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	// Prepare statement with ? placeholder
	stmt, err := service.Prepare(ctx, fmt.Sprintf("INSERT INTO %s (value) VALUES (?)", tableName))
	if err != nil {
		t.Fatalf("Failed to prepare: %v", err)
	}
	defer stmt.Close()

	// Execute multiple times
	for i := 0; i < 5; i++ {
		_, err = stmt.Exec(fmt.Sprintf("value_%d", i))
		if err != nil {
			t.Fatalf("Failed to execute prepared statement: %v", err)
		}
	}

	// Verify count
	row := service.QueryRow(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s", tableName))
	var count int
	if err := row.Scan(&count); err != nil {
		t.Fatalf("Failed to scan count: %v", err)
	}
	if count != 5 {
		t.Errorf("Expected 5 rows, got %d", count)
	}
}

// ============================================================================
// PostgreSQL Transaction Tests
// ============================================================================

func TestPostgreSQL_TransactionCommit(t *testing.T) {
	service, cleanup := createPostgresTestService(t)
	defer cleanup()

	ctx := context.Background()
	tableName := createTestTableName(t)
	defer cleanupTestTable(t, service, tableName)

	_, err := service.Exec(ctx, fmt.Sprintf("CREATE TABLE %s (id SERIAL PRIMARY KEY, value TEXT)", tableName))
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	tx, err := service.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}

	_, err = tx.ExecContext(ctx, fmt.Sprintf("INSERT INTO %s (value) VALUES ($1)", tableName), "tx_value")
	if err != nil {
		tx.Rollback()
		t.Fatalf("Failed to insert in transaction: %v", err)
	}

	if err := tx.Commit(); err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}

	// Verify data persisted
	row := service.QueryRow(ctx, fmt.Sprintf("SELECT value FROM %s WHERE value = ?", tableName), "tx_value")
	var value string
	if err := row.Scan(&value); err != nil {
		t.Fatalf("Failed to find committed value: %v", err)
	}
	if value != "tx_value" {
		t.Errorf("Expected 'tx_value', got '%s'", value)
	}
}

func TestPostgreSQL_TransactionRollback(t *testing.T) {
	service, cleanup := createPostgresTestService(t)
	defer cleanup()

	ctx := context.Background()
	tableName := createTestTableName(t)
	defer cleanupTestTable(t, service, tableName)

	_, err := service.Exec(ctx, fmt.Sprintf("CREATE TABLE %s (id SERIAL PRIMARY KEY, value TEXT)", tableName))
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	tx, err := service.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}

	_, err = tx.ExecContext(ctx, fmt.Sprintf("INSERT INTO %s (value) VALUES ($1)", tableName), "to_be_rolled_back")
	if err != nil {
		tx.Rollback()
		t.Fatalf("Failed to insert in transaction: %v", err)
	}

	if err := tx.Rollback(); err != nil {
		t.Fatalf("Failed to rollback: %v", err)
	}

	// Verify data was not persisted
	row := service.QueryRow(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE value = ?", tableName), "to_be_rolled_back")
	var count int
	if err := row.Scan(&count); err != nil {
		t.Fatalf("Failed to scan: %v", err)
	}
	if count != 0 {
		t.Errorf("Expected 0 rows after rollback, got %d", count)
	}
}

func TestPostgreSQL_NestedTransactionPrevention(t *testing.T) {
	service, cleanup := createPostgresTestService(t)
	defer cleanup()

	ctx := context.Background()

	// First transaction
	tx1, err := service.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("Failed to begin first transaction: %v", err)
	}
	defer tx1.Rollback()

	// Mark context as having transaction
	txCtx := ContextWithTransaction(ctx)

	// Attempt nested transaction
	_, err = service.BeginTx(txCtx, nil)
	if err == nil {
		t.Error("Expected error for nested transaction")
	}
	if err != nil && !strings.Contains(err.Error(), "nested transactions") {
		t.Errorf("Expected nested transaction error, got: %v", err)
	}
}

func TestPostgreSQL_TransactionWithOptions(t *testing.T) {
	service, cleanup := createPostgresTestService(t)
	defer cleanup()

	ctx := context.Background()
	tableName := createTestTableName(t)
	defer cleanupTestTable(t, service, tableName)

	_, err := service.Exec(ctx, fmt.Sprintf("CREATE TABLE %s (id SERIAL PRIMARY KEY, value TEXT)", tableName))
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	// Transaction with isolation level
	opts := &sql.TxOptions{
		Isolation: sql.LevelReadCommitted,
	}
	tx, err := service.BeginTx(ctx, opts)
	if err != nil {
		t.Fatalf("Failed to begin transaction with options: %v", err)
	}

	_, err = tx.ExecContext(ctx, fmt.Sprintf("INSERT INTO %s (value) VALUES ($1)", tableName), "iso_test")
	if err != nil {
		tx.Rollback()
		t.Fatalf("Failed to insert: %v", err)
	}

	if err := tx.Commit(); err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}

	// Verify
	row := service.QueryRow(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s", tableName))
	var count int
	if err := row.Scan(&count); err != nil {
		t.Fatalf("Failed to scan: %v", err)
	}
	if count != 1 {
		t.Errorf("Expected 1 row, got %d", count)
	}
}

// ============================================================================
// PostgreSQL Placeholder Normalization Tests
// ============================================================================

func TestPostgreSQL_NormalizePlaceholders(t *testing.T) {
	service, cleanup := createPostgresTestService(t)
	defer cleanup()

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
			input:    "INSERT INTO users (name, email, status) VALUES (?, ?, ?)",
			expected: "INSERT INTO users (name, email, status) VALUES ($1, $2, $3)",
		},
		{
			input:    "UPDATE users SET name = ?, email = ? WHERE id = ?",
			expected: "UPDATE users SET name = $1, email = $2 WHERE id = $3",
		},
		{
			input:    "SELECT * FROM users WHERE id IN (?, ?, ?, ?)",
			expected: "SELECT * FROM users WHERE id IN ($1, $2, $3, $4)",
		},
		{
			input:    "SELECT * FROM users", // No placeholders
			expected: "SELECT * FROM users",
		},
		{
			input:    "DELETE FROM users WHERE id = ?",
			expected: "DELETE FROM users WHERE id = $1",
		},
		{
			input:    "SELECT COUNT(*) FROM users WHERE created_at > ? AND status = ?",
			expected: "SELECT COUNT(*) FROM users WHERE created_at > $1 AND status = $2",
		},
	}

	for i, tt := range tests {
		normalized := service.normalizePlaceholders(tt.input)
		if normalized != tt.expected {
			t.Errorf("Test %d: normalizePlaceholders(%s) = %s, want %s", i+1, tt.input, normalized, tt.expected)
		}
	}
}

func TestPostgreSQL_EdgeCaseQueries(t *testing.T) {
	service, cleanup := createPostgresTestService(t)
	defer cleanup()

	ctx := context.Background()
	tableName := createTestTableName(t)
	defer cleanupTestTable(t, service, tableName)

	// Create table with various types
	createSQL := fmt.Sprintf(`
		CREATE TABLE %s (
			id SERIAL PRIMARY KEY,
			text_val TEXT,
			int_val INTEGER,
			float_val DOUBLE PRECISION,
			bool_val BOOLEAN,
			json_val JSONB,
			date_val DATE,
			time_val TIMESTAMP
		)`, tableName)
	_, err := service.Exec(ctx, createSQL)
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	// Insert NULL values
	_, err = service.Exec(ctx, fmt.Sprintf("INSERT INTO %s (text_val) VALUES (?)", tableName), nil)
	if err != nil {
		t.Fatalf("Failed to insert NULL: %v", err)
	}

	// Insert empty string
	_, err = service.Exec(ctx, fmt.Sprintf("INSERT INTO %s (text_val) VALUES (?)", tableName), "")
	if err != nil {
		t.Fatalf("Failed to insert empty string: %v", err)
	}

	// Insert special characters
	specialStr := "test with 'quotes' and \"double quotes\" and \n newlines"
	_, err = service.Exec(ctx, fmt.Sprintf("INSERT INTO %s (text_val) VALUES (?)", tableName), specialStr)
	if err != nil {
		t.Fatalf("Failed to insert special characters: %v", err)
	}

	// Query and verify special characters
	row := service.QueryRow(ctx, fmt.Sprintf("SELECT text_val FROM %s WHERE text_val = ?", tableName), specialStr)
	var result string
	if err := row.Scan(&result); err != nil {
		t.Fatalf("Failed to scan: %v", err)
	}
	if result != specialStr {
		t.Errorf("Special characters not preserved correctly")
	}
}

// ============================================================================
// PostgreSQL Schema Introspection Tests
// ============================================================================

func TestPostgreSQL_SchemaIntrospect(t *testing.T) {
	service, cleanup := createPostgresTestService(t)
	defer cleanup()

	ctx := context.Background()
	tableName := createTestTableName(t)
	defer cleanupTestTable(t, service, tableName)

	// Create table with various columns
	createSQL := fmt.Sprintf(`
		CREATE TABLE %s (
			id SERIAL PRIMARY KEY,
			name VARCHAR(100) NOT NULL,
			email VARCHAR(255),
			active BOOLEAN DEFAULT true,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			score INTEGER DEFAULT 0
		)`, tableName)
	_, err := service.Exec(ctx, createSQL)
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	// Create index
	_, err = service.Exec(ctx, fmt.Sprintf("CREATE INDEX idx_%s_email ON %s (email)", tableName, tableName))
	if err != nil {
		t.Fatalf("Failed to create index: %v", err)
	}

	// Create unique index
	_, err = service.Exec(ctx, fmt.Sprintf("CREATE UNIQUE INDEX idx_%s_name ON %s (name)", tableName, tableName))
	if err != nil {
		t.Fatalf("Failed to create unique index: %v", err)
	}

	// Introspect schema
	schema, err := service.SchemaIntrospect(ctx)
	if err != nil {
		t.Fatalf("Schema introspect failed: %v", err)
	}

	// Find our test table
	var testTable *interfaces.TableDefinition
	for _, table := range schema.Tables {
		if table.Name == tableName {
			testTable = &table
			break
		}
	}

	if testTable == nil {
		t.Fatalf("Table %s not found in schema", tableName)
	}

	// Check columns
	expectedColumns := 6
	if len(testTable.Columns) != expectedColumns {
		t.Errorf("Expected %d columns, got %d", expectedColumns, len(testTable.Columns))
	}

	// Find and check 'name' column
	var nameCol *interfaces.ColumnDefinition
	for i, col := range testTable.Columns {
		if col.Name == "name" {
			nameCol = &testTable.Columns[i]
			break
		}
	}

	if nameCol == nil {
		t.Fatal("name column not found")
	}

	if nameCol.Nullable {
		t.Error("name column should NOT be nullable (NOT NULL constraint)")
	}

	// Find and check 'email' column
	var emailCol *interfaces.ColumnDefinition
	for i, col := range testTable.Columns {
		if col.Name == "email" {
			emailCol = &testTable.Columns[i]
			break
		}
	}

	if emailCol == nil {
		t.Fatal("email column not found")
	}

	if !emailCol.Nullable {
		t.Error("email column should be nullable")
	}

	// Check indexes
	if len(testTable.Indexes) < 2 {
		t.Errorf("Expected at least 2 indexes, got %d", len(testTable.Indexes))
	}

	// Check unique index
	var nameIdx *interfaces.IndexDefinition
	for i, idx := range testTable.Indexes {
		if strings.Contains(idx.Name, "name") {
			nameIdx = &testTable.Indexes[i]
			break
		}
	}

	if nameIdx == nil {
		t.Fatal("name index not found")
	}

	if !nameIdx.Unique {
		t.Error("name index should be unique")
	}
}

func TestPostgreSQL_pgListTables(t *testing.T) {
	service, cleanup := createPostgresTestService(t)
	defer cleanup()

	ctx := context.Background()
	tableName := createTestTableName(t)
	defer cleanupTestTable(t, service, tableName)

	_, err := service.Exec(ctx, fmt.Sprintf("CREATE TABLE %s (id SERIAL PRIMARY KEY)", tableName))
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	tables, err := service.pgListTables(ctx)
	if err != nil {
		t.Fatalf("Failed to list tables: %v", err)
	}

	t.Logf("Tables found: %v", tables)
	t.Logf("Looking for table: %s", tableName)

	found := false
	for _, name := range tables {
		if name == tableName {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("Table %s not found in table list", tableName)
	}
}

func TestPostgreSQL_pgListColumns(t *testing.T) {
	service, cleanup := createPostgresTestService(t)
	defer cleanup()

	ctx := context.Background()
	tableName := createTestTableName(t)
	defer cleanupTestTable(t, service, tableName)

	// Create table with specific column types
	_, err := service.Exec(ctx, fmt.Sprintf(`
		CREATE TABLE %s (
			col_text TEXT,
			col_int INTEGER,
			col_bool BOOLEAN,
			col_json JSONB,
			col_uuid UUID
		)`, tableName))
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	// List columns
	columns, err := service.pgListColumns(ctx, tableName)
	if err != nil {
		t.Fatalf("Failed to list columns: %v", err)
	}

	if len(columns) != 5 {
		t.Errorf("Expected 5 columns, got %d", len(columns))
	}

	// Check column types
	expectedTypes := map[string]string{
		"col_text": "text",
		"col_int":  "integer",
		"col_bool": "boolean",
	}

	for _, col := range columns {
		if expectedType, ok := expectedTypes[col.Name]; ok {
			if col.Type != expectedType {
				t.Errorf("Column %s: expected type %s, got %s", col.Name, expectedType, col.Type)
			}
		}
	}
}

func TestPostgreSQL_pgListIndexes(t *testing.T) {
	service, cleanup := createPostgresTestService(t)
	defer cleanup()

	ctx := context.Background()
	tableName := createTestTableName(t)
	defer cleanupTestTable(t, service, tableName)

	// Create table
	_, err := service.Exec(ctx, fmt.Sprintf("CREATE TABLE %s (id SERIAL PRIMARY KEY, value TEXT, score INTEGER)", tableName))
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	// Create various indexes
	_, err = service.Exec(ctx, fmt.Sprintf("CREATE INDEX idx_%s_value ON %s (value)", tableName, tableName))
	if err != nil {
		t.Fatalf("Failed to create value index: %v", err)
	}

	_, err = service.Exec(ctx, fmt.Sprintf("CREATE UNIQUE INDEX idx_%s_score ON %s (score)", tableName, tableName))
	if err != nil {
		t.Fatalf("Failed to create score index: %v", err)
	}

	_, err = service.Exec(ctx, fmt.Sprintf("CREATE INDEX idx_%s_composite ON %s (value, score)", tableName, tableName))
	if err != nil {
		t.Fatalf("Failed to create composite index: %v", err)
	}

	// List indexes
	indexes, err := service.pgListIndexes(ctx, tableName)
	if err != nil {
		t.Fatalf("Failed to list indexes: %v", err)
	}

	// Should have at least 3 indexes (plus primary key index)
	if len(indexes) < 3 {
		t.Errorf("Expected at least 3 indexes, got %d", len(indexes))
	}

	// Check unique index
	var scoreIdx *interfaces.IndexDefinition
	for i, idx := range indexes {
		if strings.Contains(idx.Name, "score") {
			scoreIdx = &indexes[i]
			break
		}
	}

	if scoreIdx == nil {
		t.Fatal("score index not found")
	}

	if !scoreIdx.Unique {
		t.Error("score index should be unique")
	}
}

// ============================================================================
// Connection String Masking Tests (PostgreSQL-specific)
// ============================================================================

func TestMaskPassword_PostgreSQLURL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Standard PostgreSQL URL",
			input:    "postgres://user:secret@localhost:5432/mydb",
			expected: "postgres://user:****@localhost:5432/mydb",
		},
		{
			name:     "URL with @ in password (partial masking)",
			input:    "postgres://admin:p@ss!w0rd@db.example.com:5432/production",
			expected: "postgres://admin:****@ss!w0rd@db.example.com:5432/production",
		},
		{
			name:     "URL with special chars in password",
			input:    "postgres://user:pass%20word@host:5432/db",
			expected: "postgres://user:****@host:5432/db",
		},
		{
			name:     "URL without password",
			input:    "postgres://user@localhost:5432/mydb",
			expected: "postgres://user@localhost:5432/mydb",
		},
		{
			name:     "Connection string with password parameter",
			input:    "host=localhost port=5432 user=test password=mypass dbname=db",
			expected: "host=localhost port=5432 user=test password=**** dbname=db",
		},
		{
			name:     "Connection string with PASSWORD (uppercase)",
			input:    "host=localhost PASSWORD=secret user=test",
			expected: "host=localhost PASSWORD=**** user=test",
		},
		{
			name:     "URL with empty password (no masking)",
			input:    "postgres://user:@localhost:5432/mydb",
			expected: "postgres://user:@localhost:5432/mydb",
		},
		{
			name:     "URL with query params",
			input:    "postgres://user:pass@host:5432/db?sslmode=require&connect_timeout=10",
			expected: "postgres://user:****@host:5432/db?sslmode=require&connect_timeout=10",
		},
		{
			name:     "Non-PostgreSQL URL (should not change)",
			input:    "mysql://user:pass@localhost/db",
			expected: "mysql://user:****@localhost/db",
		},
		{
			name:     "No credentials at all",
			input:    "postgres://localhost:5432/mydb",
			expected: "postgres://localhost:5432/mydb",
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

func TestMaskPassword_ErrorMessages(t *testing.T) {
	// Test masking in error messages
	errMsg := "connection failed: postgres://user:secret@host:5432/db - timeout"
	masked := MaskPassword(errMsg)
	if strings.Contains(masked, "secret") {
		t.Errorf("Password should be masked in error message: %s", masked)
	}
	expected := "connection failed: postgres://user:****@host:5432/db - timeout"
	if masked != expected {
		t.Errorf("MaskPassword(%s) = %s, want %s", errMsg, masked, expected)
	}
}

// ============================================================================
// PostgreSQL Plugin Initialization Tests
// ============================================================================

func TestPlugin_InitPostgreSQL(t *testing.T) {
	if !postgresAvailable() {
		t.Skip("PostgreSQL not available")
	}

	plugin := New()
	ctx := mockPostgresCoreContext{}

	err := plugin.Init(ctx)
	if err != nil {
		t.Fatalf("Failed to init PostgreSQL plugin: %v", err)
	}

	if plugin.driver != DriverPostgreSQL {
		t.Errorf("Expected driver %s, got %s", DriverPostgreSQL, plugin.driver)
	}

	// Verify service is available
	if plugin.service == nil {
		t.Error("Service should not be nil after init")
	}

	// Verify connection works
	if err := plugin.service.Ping(context.Background()); err != nil {
		t.Errorf("Ping failed after init: %v", err)
	}

	// Cleanup
	plugin.Stop()
}

func TestPlugin_InitPostgreSQLInvalidConnection(t *testing.T) {
	plugin := New()
	ctx := mockInvalidPostgresCoreContext{}

	err := plugin.Init(ctx)
	if err == nil {
		t.Error("Expected error with invalid connection parameters")
		plugin.Stop()
	}
}

// Mock context for PostgreSQL testing
type mockPostgresCoreContext struct {
	mockCoreContext
}

func (m mockPostgresCoreContext) Config() core.ConfigProvider {
	return &mockPostgresConfig{}
}

func (m mockPostgresCoreContext) Services() core.ServiceContainer {
	return &mockPostgresServiceContainer{}
}

type mockPostgresConfig struct {
	mockConfig
}

func (m *mockPostgresConfig) GetString(key string) string {
	switch key {
	case "database.driver":
		return "postgres"
	case "database.postgres.host":
		return pgHost
	case "database.postgres.user":
		return pgUser
	case "database.postgres.password":
		return pgPassword
	case "database.postgres.dbname":
		return pgDBName
	case "database.postgres.sslmode":
		return pgSSLMode
	default:
		return ""
	}
}

func (m *mockPostgresConfig) GetInt(key string) int {
	switch key {
	case "database.postgres.port":
		return pgPort
	case "database.pool.max_conns":
		return 5
	case "database.pool.min_conns":
		return 1
	default:
		return 0
	}
}

func (m *mockPostgresConfig) GetStringSlice(key string) []string {
	return nil
}

func (m *mockPostgresConfig) GetBool(key string) bool {
	return false
}

func (m *mockPostgresConfig) Get(key string) interface{} {
	return nil
}

func (m *mockPostgresConfig) Unmarshal(key string, target interface{}) error {
	return nil
}

type mockPostgresServiceContainer struct {
	mockServiceContainer
}

func (m *mockPostgresServiceContainer) Provide(fn interface{}) error {
	return nil
}

// Mock context for invalid PostgreSQL
type mockInvalidPostgresCoreContext struct {
	mockCoreContext
}

func (m mockInvalidPostgresCoreContext) Config() core.ConfigProvider {
	return &mockInvalidPostgresConfig{}
}

type mockInvalidPostgresConfig struct {
	mockConfig
}

func (m *mockInvalidPostgresConfig) GetString(key string) string {
	switch key {
	case "database.driver":
		return "postgres"
	case "database.postgres.host":
		return "invalid_host_999"
	case "database.postgres.user":
		return "invalid_user"
	case "database.postgres.password":
		return "invalid_pass"
	case "database.postgres.dbname":
		return "invalid_db"
	case "database.postgres.sslmode":
		return "disable"
	default:
		return ""
	}
}

func (m *mockInvalidPostgresConfig) GetInt(key string) int {
	switch key {
	case "database.postgres.port":
		return 9999
	default:
		return 0
	}
}

func (m *mockInvalidPostgresConfig) GetStringSlice(key string) []string {
	return nil
}

func (m *mockInvalidPostgresConfig) GetBool(key string) bool {
	return false
}

func (m *mockInvalidPostgresConfig) Get(key string) interface{} {
	return nil
}

func (m *mockInvalidPostgresConfig) Unmarshal(key string, target interface{}) error {
	return nil
}

// ============================================================================
// PostgreSQL-Specific Error Handling Tests
// ============================================================================

func TestPostgreSQL_InvalidSQL(t *testing.T) {
	service, cleanup := createPostgresTestService(t)
	defer cleanup()

	ctx := context.Background()

	_, err := service.Exec(ctx, "INVALID SQL STATEMENT")
	if err == nil {
		t.Error("Expected error for invalid SQL")
	}
}

func TestPostgreSQL_DuplicateTable(t *testing.T) {
	service, cleanup := createPostgresTestService(t)
	defer cleanup()

	ctx := context.Background()
	tableName := createTestTableName(t)
	defer cleanupTestTable(t, service, tableName)

	// Create table
	_, err := service.Exec(ctx, fmt.Sprintf("CREATE TABLE %s (id SERIAL PRIMARY KEY)", tableName))
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	// Try to create same table again
	_, err = service.Exec(ctx, fmt.Sprintf("CREATE TABLE %s (id SERIAL PRIMARY KEY)", tableName))
	if err == nil {
		t.Error("Expected error for duplicate table creation")
	}
}

func TestPostgreSQL_DropNonExistentTable(t *testing.T) {
	service, cleanup := createPostgresTestService(t)
	defer cleanup()

	ctx := context.Background()

	// Drop non-existent table (should succeed with IF EXISTS)
	_, err := service.Exec(ctx, "DROP TABLE IF EXISTS nonexistent_table_xyz")
	if err != nil {
		t.Errorf("DROP TABLE IF EXISTS should not error: %v", err)
	}
}

func TestPostgreSQL_InsertIntoNonExistentTable(t *testing.T) {
	service, cleanup := createPostgresTestService(t)
	defer cleanup()

	ctx := context.Background()

	_, err := service.Exec(ctx, "INSERT INTO nonexistent_table_xyz (id) VALUES (1)")
	if err == nil {
		t.Error("Expected error for insert into non-existent table")
	}
}

func TestPostgreSQL_ConstraintViolation(t *testing.T) {
	service, cleanup := createPostgresTestService(t)
	defer cleanup()

	ctx := context.Background()
	tableName := createTestTableName(t)
	defer cleanupTestTable(t, service, tableName)

	// Create table with unique constraint
	_, err := service.Exec(ctx, fmt.Sprintf("CREATE TABLE %s (id SERIAL PRIMARY KEY, email VARCHAR(255) UNIQUE)", tableName))
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	// Insert first row
	_, err = service.Exec(ctx, fmt.Sprintf("INSERT INTO %s (email) VALUES (?)", tableName), "test@example.com")
	if err != nil {
		t.Fatalf("Failed to insert first row: %v", err)
	}

	// Try to insert duplicate
	_, err = service.Exec(ctx, fmt.Sprintf("INSERT INTO %s (email) VALUES (?)", tableName), "test@example.com")
	if err == nil {
		t.Error("Expected error for unique constraint violation")
	}
}

func TestPostgreSQL_NotNullViolation(t *testing.T) {
	service, cleanup := createPostgresTestService(t)
	defer cleanup()

	ctx := context.Background()
	tableName := createTestTableName(t)
	defer cleanupTestTable(t, service, tableName)

	// Create table with NOT NULL constraint
	_, err := service.Exec(ctx, fmt.Sprintf("CREATE TABLE %s (id SERIAL PRIMARY KEY, name TEXT NOT NULL)", tableName))
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	// Try to insert NULL
	_, err = service.Exec(ctx, fmt.Sprintf("INSERT INTO %s (name) VALUES (?)", tableName), nil)
	if err == nil {
		t.Error("Expected error for NOT NULL violation")
	}
}

// ============================================================================
// PostgreSQL Large Data Tests
// ============================================================================

func TestPostgreSQL_BatchInsert(t *testing.T) {
	service, cleanup := createPostgresTestService(t)
	defer cleanup()

	ctx := context.Background()
	tableName := createTestTableName(t)
	defer cleanupTestTable(t, service, tableName)

	// Create table
	_, err := service.Exec(ctx, fmt.Sprintf("CREATE TABLE %s (id SERIAL PRIMARY KEY, value TEXT)", tableName))
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	// Batch insert using prepared statement
	stmt, err := service.Prepare(ctx, fmt.Sprintf("INSERT INTO %s (value) VALUES (?)", tableName))
	if err != nil {
		t.Fatalf("Failed to prepare: %v", err)
	}
	defer stmt.Close()

	batchSize := 100
	for i := 0; i < batchSize; i++ {
		_, err = stmt.Exec(fmt.Sprintf("batch_value_%d", i))
		if err != nil {
			t.Fatalf("Failed to insert batch item %d: %v", i, err)
		}
	}

	// Verify count
	row := service.QueryRow(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s", tableName))
	var count int
	if err := row.Scan(&count); err != nil {
		t.Fatalf("Failed to scan count: %v", err)
	}
	if count != batchSize {
		t.Errorf("Expected %d rows, got %d", batchSize, count)
	}
}

func TestPostgreSQL_LargeTextValue(t *testing.T) {
	service, cleanup := createPostgresTestService(t)
	defer cleanup()

	ctx := context.Background()
	tableName := createTestTableName(t)
	defer cleanupTestTable(t, service, tableName)

	// Create table
	_, err := service.Exec(ctx, fmt.Sprintf("CREATE TABLE %s (id SERIAL PRIMARY KEY, content TEXT)", tableName))
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	// Create large text (10KB)
	largeText := strings.Repeat("Lorem ipsum dolor sit amet. ", 300)

	// Insert large text
	_, err = service.Exec(ctx, fmt.Sprintf("INSERT INTO %s (content) VALUES (?)", tableName), largeText)
	if err != nil {
		t.Fatalf("Failed to insert large text: %v", err)
	}

	// Retrieve and verify
	row := service.QueryRow(ctx, fmt.Sprintf("SELECT content FROM %s", tableName))
	var result string
	if err := row.Scan(&result); err != nil {
		t.Fatalf("Failed to scan large text: %v", err)
	}
	if result != largeText {
		t.Errorf("Large text not preserved correctly (length: expected %d, got %d)", len(largeText), len(result))
	}
}

// ============================================================================
// PostgreSQL Concurrent Access Tests
// ============================================================================

func TestPostgreSQL_ConcurrentInserts(t *testing.T) {
	service, cleanup := createPostgresTestService(t)
	defer cleanup()

	ctx := context.Background()
	tableName := createTestTableName(t)
	defer cleanupTestTable(t, service, tableName)

	// Create table
	_, err := service.Exec(ctx, fmt.Sprintf("CREATE TABLE %s (id SERIAL PRIMARY KEY, thread_id INTEGER)", tableName))
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	// Concurrent inserts
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(threadID int) {
			for j := 0; j < 10; j++ {
				_, err := service.Exec(ctx, fmt.Sprintf("INSERT INTO %s (thread_id) VALUES (?)", tableName), threadID)
				if err != nil {
					t.Errorf("Thread %d insert %d failed: %v", threadID, j, err)
				}
			}
			done <- true
		}(i)
	}

	// Wait for all threads
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify count
	row := service.QueryRow(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s", tableName))
	var count int
	if err := row.Scan(&count); err != nil {
		t.Fatalf("Failed to scan count: %v", err)
	}
	expectedCount := 100
	if count != expectedCount {
		t.Errorf("Expected %d rows after concurrent inserts, got %d", expectedCount, count)
	}
}
