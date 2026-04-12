package ddl

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/wangling-miao/aroute/sdk/interfaces"
)

type pgTestDB struct {
	db        *sql.DB
	available bool
}

func newPGTestDB(t *testing.T) *pgTestDB {
	t.Helper()

	connStr := os.Getenv("PG_TEST_CONN_STRING")
	if connStr == "" {
		connStr = "postgres://aroute:aroute_dev_password@localhost:5432/aroute?sslmode=disable"
	}

	db, err := sql.Open("pgx", connStr)
	if err != nil {
		t.Logf("PostgreSQL connection open error: %v (tests will use mocks)", err)
		return &pgTestDB{available: false}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		t.Logf("PostgreSQL ping failed: %v (tests will use mocks)", err)
		db.Close()
		return &pgTestDB{available: false}
	}

	return &pgTestDB{db: db, available: true}
}

func (p *pgTestDB) Close() error {
	if p.db != nil {
		return p.db.Close()
	}
	return nil
}

func (p *pgTestDB) IsAvailable() bool {
	return p.available
}

func (p *pgTestDB) Exec(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	return p.db.ExecContext(ctx, query, args...)
}

func (p *pgTestDB) Query(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	return p.db.QueryContext(ctx, query, args...)
}

func (p *pgTestDB) QueryRow(ctx context.Context, query string, args ...interface{}) *sql.Row {
	return p.db.QueryRowContext(ctx, query, args...)
}

func (p *pgTestDB) BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
	return p.db.BeginTx(ctx, opts)
}

func (p *pgTestDB) Ping(ctx context.Context) error {
	return p.db.PingContext(ctx)
}

func (p *pgTestDB) Prepare(ctx context.Context, query string) (*sql.Stmt, error) {
	return p.db.PrepareContext(ctx, query)
}

func (p *pgTestDB) SchemaIntrospect(ctx context.Context) (*interfaces.DatabaseSchema, error) {
	introspector := NewIntrospector(p, DialectPostgreSQL)
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

func pgTableExists(t *testing.T, db *sql.DB, tableName string) bool {
	t.Helper()
	var exists bool
	err := db.QueryRowContext(context.Background(),
		"SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_name = $1 AND table_schema = 'public')",
		tableName).Scan(&exists)
	if err != nil {
		t.Fatalf("checking table existence: %v", err)
	}
	return exists
}

func pgColumnExists(t *testing.T, db *sql.DB, tableName, columnName string) bool {
	t.Helper()
	var exists bool
	err := db.QueryRowContext(context.Background(),
		"SELECT EXISTS (SELECT FROM information_schema.columns WHERE table_name = $1 AND column_name = $2 AND table_schema = 'public')",
		tableName, columnName).Scan(&exists)
	if err != nil {
		t.Fatalf("checking column existence: %v", err)
	}
	return exists
}

func pgIndexExists(t *testing.T, db *sql.DB, indexName string) bool {
	t.Helper()
	var exists bool
	err := db.QueryRowContext(context.Background(),
		"SELECT EXISTS (SELECT FROM pg_indexes WHERE indexname = $1 AND schemaname = 'public')",
		indexName).Scan(&exists)
	if err != nil {
		t.Fatalf("checking index existence: %v", err)
	}
	return exists
}

func pgGetColumnType(t *testing.T, db *sql.DB, tableName, columnName string) string {
	t.Helper()
	var dataType string
	err := db.QueryRowContext(context.Background(),
		"SELECT data_type FROM information_schema.columns WHERE table_name = $1 AND column_name = $2 AND table_schema = 'public'",
		tableName, columnName).Scan(&dataType)
	if err != nil {
		t.Fatalf("getting column type: %v", err)
	}
	return strings.ToUpper(dataType)
}

func pgDropTestTable(t *testing.T, db *sql.DB, tableName string) {
	t.Helper()
	_, err := db.ExecContext(context.Background(), fmt.Sprintf("DROP TABLE IF EXISTS \"%s\" CASCADE", tableName))
	if err != nil {
		t.Logf("cleanup: drop table %s: %v", tableName, err)
	}
}

func pgCreateTestTable(t *testing.T, executor *PostgreSQLExecutor, ctx context.Context, schema *Schema) {
	t.Helper()
	ops := []DiffOperation{
		{Type: OpTableCreate, TableName: schema.GetTableName(), Schema: schema},
	}
	if err := executor.Execute(ctx, ops, false); err != nil {
		t.Fatalf("createTestTable: %v", err)
	}
}

func TestPostgreSQLExecutor_TableCreate(t *testing.T) {
	pgdb := newPGTestDB(t)
	defer pgdb.Close()

	if !pgdb.IsAvailable() {
		t.Skip("PostgreSQL not available - skipping integration test")
	}

	executor := NewPostgreSQLExecutor(pgdb)
	ctx := context.Background()

	tableName := "test_pg_users"
	pgDropTestTable(t, pgdb.db, tableName)

	ops := []DiffOperation{
		{
			Type:      OpTableCreate,
			TableName: tableName,
			Schema: &Schema{
				Name: tableName,
				Fields: []FieldDefinition{
					{Name: "id", Type: FieldTypeNumber, Constraints: &Constraints{Nullable: false}},
					{Name: "name", Type: FieldTypeText},
					{Name: "email", Type: FieldTypeText, Constraints: &Constraints{Unique: true}},
					{Name: "created_at", Type: FieldTypeDatetime},
					{Name: "metadata", Type: FieldTypeJSON},
				},
			},
		},
	}

	if err := executor.Execute(ctx, ops, false); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if !pgTableExists(t, pgdb.db, tableName) {
		t.Error("table should exist after creation")
	}

	for _, col := range []string{"id", "name", "email", "created_at", "metadata"} {
		if !pgColumnExists(t, pgdb.db, tableName, col) {
			t.Errorf("column %q should exist", col)
		}
	}

	colType := pgGetColumnType(t, pgdb.db, tableName, "id")
	if colType != "BIGINT" {
		t.Errorf("id column type = %q, want BIGINT", colType)
	}

	colType = pgGetColumnType(t, pgdb.db, tableName, "metadata")
	if colType != "JSONB" {
		t.Errorf("metadata column type = %q, want JSONB", colType)
	}

	pgDropTestTable(t, pgdb.db, tableName)
}

func TestPostgreSQLExecutor_ColumnAdd(t *testing.T) {
	pgdb := newPGTestDB(t)
	defer pgdb.Close()

	if !pgdb.IsAvailable() {
		t.Skip("PostgreSQL not available - skipping integration test")
	}

	executor := NewPostgreSQLExecutor(pgdb)
	ctx := context.Background()

	tableName := "test_pg_products"
	pgDropTestTable(t, pgdb.db, tableName)

	pgCreateTestTable(t, executor, ctx, &Schema{
		Name: tableName,
		Fields: []FieldDefinition{
			{Name: "id", Type: FieldTypeNumber, Constraints: &Constraints{Nullable: false}},
			{Name: "name", Type: FieldTypeText},
		},
	})

	ops := []DiffOperation{
		{
			Type:        OpColumnAdd,
			TableName:   tableName,
			ColumnName:  "price",
			ColumnType:  "decimal",
			Constraints: &Constraints{Nullable: true, Default: 0.0},
		},
	}

	if err := executor.Execute(ctx, ops, false); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if !pgColumnExists(t, pgdb.db, tableName, "price") {
		t.Error("column 'price' should exist after add")
	}

	pgDropTestTable(t, pgdb.db, tableName)
}

func TestPostgreSQLExecutor_ColumnAdd_WithForeignKey(t *testing.T) {
	pgdb := newPGTestDB(t)
	defer pgdb.Close()

	if !pgdb.IsAvailable() {
		t.Skip("PostgreSQL not available - skipping integration test")
	}

	executor := NewPostgreSQLExecutor(pgdb)
	ctx := context.Background()

	parentTable := "test_pg_categories"
	pgDropTestTable(t, pgdb.db, parentTable)

	_, err := pgdb.Exec(ctx, fmt.Sprintf(`
		CREATE TABLE "%s" (
			id BIGINT PRIMARY KEY,
			name TEXT
		)`, parentTable))
	if err != nil {
		t.Fatalf("create parent table: %v", err)
	}

	childTable := "test_pg_items"
	pgDropTestTable(t, pgdb.db, childTable)

	_, err = pgdb.Exec(ctx, fmt.Sprintf(`
		CREATE TABLE "%s" (
			id BIGINT PRIMARY KEY,
			name TEXT
		)`, childTable))
	if err != nil {
		t.Fatalf("create child table: %v", err)
	}

	ops := []DiffOperation{
		{
			Type:       OpColumnAdd,
			TableName:  childTable,
			ColumnName: "category_id",
			ColumnType: "relation",
			ForeignKey: &ForeignKeyReference{
				Table:    parentTable,
				Column:   "id",
				OnDelete: "SET NULL",
			},
			Constraints: &Constraints{Nullable: true},
		},
	}

	if err := executor.Execute(ctx, ops, false); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if !pgColumnExists(t, pgdb.db, childTable, "category_id") {
		t.Error("column 'category_id' should exist after add")
	}

	pgDropTestTable(t, pgdb.db, childTable)
	pgDropTestTable(t, pgdb.db, parentTable)
}

func TestPostgreSQLExecutor_ColumnDrop(t *testing.T) {
	pgdb := newPGTestDB(t)
	defer pgdb.Close()

	if !pgdb.IsAvailable() {
		t.Skip("PostgreSQL not available - skipping integration test")
	}

	executor := NewPostgreSQLExecutor(pgdb)
	ctx := context.Background()

	tableName := "test_pg_logs"
	pgDropTestTable(t, pgdb.db, tableName)

	pgCreateTestTable(t, executor, ctx, &Schema{
		Name: tableName,
		Fields: []FieldDefinition{
			{Name: "id", Type: FieldTypeNumber, Constraints: &Constraints{Nullable: false}},
			{Name: "message", Type: FieldTypeText},
			{Name: "debug_info", Type: FieldTypeText},
		},
	})

	// Drop without force should fail
	ops := []DiffOperation{
		{Type: OpColumnDrop, TableName: tableName, ColumnName: "debug_info"},
	}

	err := executor.Execute(ctx, ops, false)
	if err == nil {
		t.Fatal("expected error for destructive operation without force")
	}
	if !strings.Contains(err.Error(), "destructive operation") {
		t.Errorf("error should mention destructive operation, got: %v", err)
	}

	// Drop with force should succeed
	if err := executor.Execute(ctx, ops, true); err != nil {
		t.Fatalf("Execute() with force error = %v", err)
	}

	if pgColumnExists(t, pgdb.db, tableName, "debug_info") {
		t.Error("column 'debug_info' should not exist after drop")
	}

	if !pgColumnExists(t, pgdb.db, tableName, "id") {
		t.Error("column 'id' should still exist")
	}

	pgDropTestTable(t, pgdb.db, tableName)
}

func TestPostgreSQLExecutor_ColumnModify_TypeOnly(t *testing.T) {
	pgdb := newPGTestDB(t)
	defer pgdb.Close()

	if !pgdb.IsAvailable() {
		t.Skip("PostgreSQL not available - skipping integration test")
	}

	executor := NewPostgreSQLExecutor(pgdb)
	ctx := context.Background()

	tableName := "test_pg_metrics"
	pgDropTestTable(t, pgdb.db, tableName)

	_, err := pgdb.Exec(ctx, fmt.Sprintf(`
		CREATE TABLE "%s" (
			id BIGINT PRIMARY KEY,
			value TEXT
		)`, tableName))
	if err != nil {
		t.Fatalf("create table: %v", err)
	}

	ops := []DiffOperation{
		{
			Type:       OpColumnModify,
			TableName:  tableName,
			ColumnName: "value",
			ColumnType: "number",
		},
	}

	if err := executor.Execute(ctx, ops, false); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	colType := pgGetColumnType(t, pgdb.db, tableName, "value")
	if colType != "BIGINT" {
		t.Errorf("column 'value' type = %q, want BIGINT", colType)
	}

	pgDropTestTable(t, pgdb.db, tableName)
}

func TestPostgreSQLExecutor_ColumnModify_WithNullable(t *testing.T) {
	pgdb := newPGTestDB(t)
	defer pgdb.Close()

	if !pgdb.IsAvailable() {
		t.Skip("PostgreSQL not available - skipping integration test")
	}

	executor := NewPostgreSQLExecutor(pgdb)
	ctx := context.Background()

	tableName := "test_pg_flags"
	pgDropTestTable(t, pgdb.db, tableName)

	_, err := pgdb.Exec(ctx, fmt.Sprintf(`
		CREATE TABLE "%s" (
			id BIGINT PRIMARY KEY,
			active BOOLEAN
		)`, tableName))
	if err != nil {
		t.Fatalf("create table: %v", err)
	}

	ops := []DiffOperation{
		{
			Type:       OpColumnModify,
			TableName:  tableName,
			ColumnName: "active",
			ColumnType: "boolean",
		},
	}

	if err := executor.Execute(ctx, ops, false); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	pgDropTestTable(t, pgdb.db, tableName)
}

func TestPostgreSQLExecutor_IndexAdd(t *testing.T) {
	pgdb := newPGTestDB(t)
	defer pgdb.Close()

	if !pgdb.IsAvailable() {
		t.Skip("PostgreSQL not available - skipping integration test")
	}

	executor := NewPostgreSQLExecutor(pgdb)
	ctx := context.Background()

	tableName := "test_pg_searches"
	pgDropTestTable(t, pgdb.db, tableName)

	pgCreateTestTable(t, executor, ctx, &Schema{
		Name: tableName,
		Fields: []FieldDefinition{
			{Name: "id", Type: FieldTypeNumber, Constraints: &Constraints{Nullable: false}},
			{Name: "keyword", Type: FieldTypeText},
			{Name: "url", Type: FieldTypeText},
		},
	})

	ops := []DiffOperation{
		{
			Type:         OpIndexAdd,
			TableName:    tableName,
			IndexName:    "idx_searches_keyword",
			IndexColumns: []string{"keyword"},
			IndexUnique:  false,
		},
	}

	if err := executor.Execute(ctx, ops, false); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if !pgIndexExists(t, pgdb.db, "idx_searches_keyword") {
		t.Error("index should exist after add")
	}

	pgDropTestTable(t, pgdb.db, tableName)
}

func TestPostgreSQLExecutor_IndexAdd_Unique(t *testing.T) {
	pgdb := newPGTestDB(t)
	defer pgdb.Close()

	if !pgdb.IsAvailable() {
		t.Skip("PostgreSQL not available - skipping integration test")
	}

	executor := NewPostgreSQLExecutor(pgdb)
	ctx := context.Background()

	tableName := "test_pg_tokens"
	pgDropTestTable(t, pgdb.db, tableName)

	pgCreateTestTable(t, executor, ctx, &Schema{
		Name: tableName,
		Fields: []FieldDefinition{
			{Name: "id", Type: FieldTypeNumber, Constraints: &Constraints{Nullable: false}},
			{Name: "token", Type: FieldTypeText},
		},
	})

	ops := []DiffOperation{
		{
			Type:         OpIndexAdd,
			TableName:    tableName,
			IndexName:    "idx_tokens_unique",
			IndexColumns: []string{"token"},
			IndexUnique:  true,
		},
	}

	if err := executor.Execute(ctx, ops, false); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if !pgIndexExists(t, pgdb.db, "idx_tokens_unique") {
		t.Error("unique index should exist after add")
	}

	pgDropTestTable(t, pgdb.db, tableName)
}

func TestPostgreSQLExecutor_IndexDrop(t *testing.T) {
	pgdb := newPGTestDB(t)
	defer pgdb.Close()

	if !pgdb.IsAvailable() {
		t.Skip("PostgreSQL not available - skipping integration test")
	}

	ctx := context.Background()

	tableName := "test_pg_tags"
	indexName := "idx_tags_name"
	pgDropTestTable(t, pgdb.db, tableName)

	_, err := pgdb.Exec(ctx, fmt.Sprintf(`
		CREATE TABLE "%s" (
			id BIGINT PRIMARY KEY,
			name TEXT
		)`, tableName))
	if err != nil {
		t.Fatalf("create table: %v", err)
	}

	_, err = pgdb.Exec(ctx, fmt.Sprintf(`CREATE INDEX %s ON "%s"(name)`, indexName, tableName))
	if err != nil {
		t.Fatalf("create index: %v", err)
	}

	if !pgIndexExists(t, pgdb.db, indexName) {
		t.Fatal("index should exist after creation")
	}

	_, err = pgdb.Exec(ctx, fmt.Sprintf(`DROP INDEX %s`, indexName))
	if err != nil {
		t.Fatalf("drop index: %v", err)
	}

	if pgIndexExists(t, pgdb.db, indexName) {
		t.Error("index should not exist after drop")
	}

	pgDropTestTable(t, pgdb.db, tableName)
}

func TestPostgreSQLExecutor_MultipleOps(t *testing.T) {
	pgdb := newPGTestDB(t)
	defer pgdb.Close()

	if !pgdb.IsAvailable() {
		t.Skip("PostgreSQL not available - skipping integration test")
	}

	executor := NewPostgreSQLExecutor(pgdb)
	ctx := context.Background()

	tableName := "test_pg_multi"
	pgDropTestTable(t, pgdb.db, tableName)

	pgCreateTestTable(t, executor, ctx, &Schema{
		Name: tableName,
		Fields: []FieldDefinition{
			{Name: "id", Type: FieldTypeNumber, Constraints: &Constraints{Nullable: false}},
			{Name: "name", Type: FieldTypeText},
		},
	})

	ops := []DiffOperation{
		{
			Type:        OpColumnAdd,
			TableName:   tableName,
			ColumnName:  "status",
			ColumnType:  "text",
			Constraints: &Constraints{Nullable: true, Default: "pending"},
		},
		{
			Type:         OpIndexAdd,
			TableName:    tableName,
			IndexName:    "idx_multi_name",
			IndexColumns: []string{"name"},
			IndexUnique:  false,
		},
	}

	if err := executor.Execute(ctx, ops, false); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if !pgColumnExists(t, pgdb.db, tableName, "status") {
		t.Error("column 'status' should exist")
	}
	if !pgIndexExists(t, pgdb.db, "idx_multi_name") {
		t.Error("index should exist")
	}

	pgDropTestTable(t, pgdb.db, tableName)
}

func TestPostgreSQLExecutor_TransactionRollback(t *testing.T) {
	pgdb := newPGTestDB(t)
	defer pgdb.Close()

	if !pgdb.IsAvailable() {
		t.Skip("PostgreSQL not available - skipping integration test")
	}

	executor := NewPostgreSQLExecutor(pgdb)
	ctx := context.Background()

	tableName := "test_pg_rollback"
	pgDropTestTable(t, pgdb.db, tableName)

	pgCreateTestTable(t, executor, ctx, &Schema{
		Name: tableName,
		Fields: []FieldDefinition{
			{Name: "id", Type: FieldTypeNumber, Constraints: &Constraints{Nullable: false}},
			{Name: "name", Type: FieldTypeText},
		},
	})

	// Two ops: valid add + invalid (duplicate column)
	ops := []DiffOperation{
		{
			Type:       OpColumnAdd,
			TableName:  tableName,
			ColumnName: "email",
			ColumnType: "text",
		},
		{
			Type:       OpColumnAdd,
			TableName:  tableName,
			ColumnName: "name",
			ColumnType: "text",
		},
	}

	err := executor.Execute(ctx, ops, false)
	if err == nil {
		t.Fatal("expected error for duplicate column")
	}

	// First op should have been rolled back
	if pgColumnExists(t, pgdb.db, tableName, "email") {
		t.Error("column 'email' should not exist after rollback")
	}

	pgDropTestTable(t, pgdb.db, tableName)
}

func TestPostgreSQLExecutor_EmptyOps(t *testing.T) {
	pgdb := newPGTestDB(t)
	defer pgdb.Close()

	if !pgdb.IsAvailable() {
		t.Skip("PostgreSQL not available - skipping integration test")
	}

	executor := NewPostgreSQLExecutor(pgdb)
	ctx := context.Background()

	if err := executor.Execute(ctx, []DiffOperation{}, false); err != nil {
		t.Fatalf("Execute() with empty ops should succeed, got: %v", err)
	}
}

func TestPostgreSQLExecutor_CreateTable_WithDefaults(t *testing.T) {
	pgdb := newPGTestDB(t)
	defer pgdb.Close()

	if !pgdb.IsAvailable() {
		t.Skip("PostgreSQL not available - skipping integration test")
	}

	executor := NewPostgreSQLExecutor(pgdb)
	ctx := context.Background()

	tableName := "test_pg_defaults"
	pgDropTestTable(t, pgdb.db, tableName)

	ops := []DiffOperation{
		{
			Type:      OpTableCreate,
			TableName: tableName,
			Schema: &Schema{
				Name: tableName,
				Fields: []FieldDefinition{
					{Name: "id", Type: FieldTypeNumber, Constraints: &Constraints{Nullable: false}},
					{Name: "active", Type: FieldTypeBoolean, Constraints: &Constraints{Default: true}},
					{Name: "count", Type: FieldTypeNumber, Constraints: &Constraints{Default: 0}},
					{Name: "label", Type: FieldTypeText, Constraints: &Constraints{Default: "unnamed"}},
				},
			},
		},
	}

	if err := executor.Execute(ctx, ops, false); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	for _, col := range []string{"id", "active", "count", "label"} {
		if !pgColumnExists(t, pgdb.db, tableName, col) {
			t.Errorf("column %q should exist", col)
		}
	}

	pgDropTestTable(t, pgdb.db, tableName)
}

func TestPostgreSQLExecutor_ColumnAdd_NotNullWithDefault(t *testing.T) {
	pgdb := newPGTestDB(t)
	defer pgdb.Close()

	if !pgdb.IsAvailable() {
		t.Skip("PostgreSQL not available - skipping integration test")
	}

	executor := NewPostgreSQLExecutor(pgdb)
	ctx := context.Background()

	tableName := "test_pg_notnull"
	pgDropTestTable(t, pgdb.db, tableName)

	pgCreateTestTable(t, executor, ctx, &Schema{
		Name: tableName,
		Fields: []FieldDefinition{
			{Name: "id", Type: FieldTypeNumber, Constraints: &Constraints{Nullable: false}},
		},
	})

	ops := []DiffOperation{
		{
			Type:        OpColumnAdd,
			TableName:   tableName,
			ColumnName:  "status",
			ColumnType:  "text",
			Constraints: &Constraints{Nullable: false, Default: "active"},
		},
	}

	if err := executor.Execute(ctx, ops, false); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if !pgColumnExists(t, pgdb.db, tableName, "status") {
		t.Error("column 'status' should exist")
	}

	pgDropTestTable(t, pgdb.db, tableName)
}

func TestPostgreSQLExecutor_ModifyColumn_DropDefault(t *testing.T) {
	pgdb := newPGTestDB(t)
	defer pgdb.Close()

	if !pgdb.IsAvailable() {
		t.Skip("PostgreSQL not available - skipping integration test")
	}

	executor := NewPostgreSQLExecutor(pgdb)
	ctx := context.Background()

	tableName := "test_pg_dropdefault"
	pgDropTestTable(t, pgdb.db, tableName)

	_, err := pgdb.Exec(ctx, fmt.Sprintf(`
		CREATE TABLE "%s" (
			id BIGINT PRIMARY KEY,
			level BIGINT DEFAULT 5
		)`, tableName))
	if err != nil {
		t.Fatalf("create table: %v", err)
	}

	ops := []DiffOperation{
		{
			Type:       OpColumnModify,
			TableName:  tableName,
			ColumnName: "level",
			ColumnType: "number",
		},
	}

	if err := executor.Execute(ctx, ops, false); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	pgDropTestTable(t, pgdb.db, tableName)
}

func TestPostgreSQLExecutor_UnsupportedOpType(t *testing.T) {
	pgdb := newPGTestDB(t)
	defer pgdb.Close()

	if !pgdb.IsAvailable() {
		t.Skip("PostgreSQL not available - skipping integration test")
	}

	executor := NewPostgreSQLExecutor(pgdb)
	ctx := context.Background()

	tableName := "test_pg_unsupported"
	pgDropTestTable(t, pgdb.db, tableName)

	pgCreateTestTable(t, executor, ctx, &Schema{
		Name: tableName,
		Fields: []FieldDefinition{
			{Name: "id", Type: FieldTypeNumber, Constraints: &Constraints{Nullable: false}},
		},
	})

	ops := []DiffOperation{
		{Type: OpTableDrop, TableName: tableName},
	}

	err := executor.Execute(ctx, ops, true)
	if err == nil {
		t.Fatal("expected error for unsupported operation type")
	}
	if !strings.Contains(err.Error(), "unsupported operation type") {
		t.Errorf("error should mention unsupported operation, got: %v", err)
	}

	pgDropTestTable(t, pgdb.db, tableName)
}
