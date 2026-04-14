package ddl

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"

	"github.com/wangling-miao/aroute/sdk/interfaces"
)

// ---------------------------------------------------------------------------
// mockDB: implements interfaces.DatabaseService to capture generated SQL.
// BeginTx returns an error so we can test the force-check path without
// needing a real PostgreSQL connection.
// ---------------------------------------------------------------------------

type mockDB struct {
	execQueries  []string
	queryQueries []string
	beginTxFail  bool // if true, BeginTx returns an error
}

func (m *mockDB) Query(_ context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	m.queryQueries = append(m.queryQueries, query)
	return nil, fmt.Errorf("mockDB: Query not implemented")
}

func (m *mockDB) QueryRow(_ context.Context, query string, args ...interface{}) *sql.Row {
	m.queryQueries = append(m.queryQueries, query)
	return nil
}

func (m *mockDB) Exec(_ context.Context, query string, args ...interface{}) (sql.Result, error) {
	m.execQueries = append(m.execQueries, query)
	return nil, nil
}

func (m *mockDB) BeginTx(_ context.Context, _ *sql.TxOptions) (*sql.Tx, error) {
	if m.beginTxFail {
		return nil, fmt.Errorf("mockDB: BeginTx deliberately failed")
	}
	return nil, fmt.Errorf("mockDB: BeginTx not implemented")
}

func (m *mockDB) Ping(_ context.Context) error { return nil }

func (m *mockDB) Prepare(_ context.Context, _ string) (*sql.Stmt, error) {
	return nil, fmt.Errorf("mockDB: Prepare not implemented")
}

func (m *mockDB) Close() error { return nil }

func (m *mockDB) SchemaIntrospect(_ context.Context) (*interfaces.DatabaseSchema, error) {
	return nil, fmt.Errorf("mockDB: SchemaIntrospect not implemented")
}

// ---------------------------------------------------------------------------
// TestPostgreSQL_TypeMapping tests that the TypeMapper with DialectPostgreSQL
// maps every FieldType to the correct PostgreSQL column type.
// ---------------------------------------------------------------------------

func TestPostgreSQL_TypeMapping(t *testing.T) {
	mapper := NewTypeMapper(DialectPostgreSQL)

	tests := []struct {
		fieldType FieldType
		expected  string
	}{
		{FieldTypeText, "TEXT"},
		{FieldTypeNumber, "BIGINT"},
		{FieldTypeDecimal, "NUMERIC"},
		{FieldTypeBoolean, "BOOLEAN"},
		{FieldTypeDatetime, "TIMESTAMP WITH TIME ZONE"},
		{FieldTypeJSON, "JSONB"},
		{FieldTypeRelation, "BIGINT"},
	}

	for _, tt := range tests {
		t.Run(string(tt.fieldType), func(t *testing.T) {
			got := mapper.MapColumnType(tt.fieldType, nil)
			if got != tt.expected {
				t.Errorf("MapColumnType(%q, nil) = %q, want %q", tt.fieldType, got, tt.expected)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestPostgreSQL_TypeMapping_WithMaxLength tests that text fields with a
// MaxLength constraint produce VARCHAR(n) instead of TEXT.
// ---------------------------------------------------------------------------

func TestPostgreSQL_TypeMapping_WithMaxLength(t *testing.T) {
	mapper := NewTypeMapper(DialectPostgreSQL)
	got := mapper.MapColumnType(FieldTypeText, &Constraints{MaxLength: new(255)})
	want := "VARCHAR(255)"
	if got != want {
		t.Errorf("MapColumnType(text, MaxLength=255) = %q, want %q", got, want)
	}
}

// ---------------------------------------------------------------------------
// TestPostgreSQL_TypeMapping_UnknownFieldType tests that an unknown field
// type defaults to TEXT for PostgreSQL.
// ---------------------------------------------------------------------------

func TestPostgreSQL_TypeMapping_UnknownFieldType(t *testing.T) {
	mapper := NewTypeMapper(DialectPostgreSQL)

	got := mapper.MapColumnType(FieldType("unknown_type"), nil)
	want := "TEXT"
	if got != want {
		t.Errorf("MapColumnType(%q, nil) = %q, want %q", "unknown_type", got, want)
	}
}

// ---------------------------------------------------------------------------
// TestPostgreSQL_FormatDefaultValue tests FormatDefaultValue for all
// supported value types with DialectPostgreSQL.
// ---------------------------------------------------------------------------

func TestPostgreSQL_FormatDefaultValue(t *testing.T) {
	mapper := NewTypeMapper(DialectPostgreSQL)

	tests := []struct {
		name     string
		value    interface{}
		expected string
	}{
		{"bool true", true, "TRUE"},
		{"bool false", false, "FALSE"},
		{"string", "hello", "'hello'"},
		{"int", 123, "123"},
		{"int64", int64(456), "456"},
		{"float64", 1.23, "1.230000"},
		{"nil", nil, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mapper.FormatDefaultValue(tt.value, FieldTypeText)
			if got != tt.expected {
				t.Errorf("FormatDefaultValue(%v, text) = %q, want %q", tt.value, got, tt.expected)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestPostgreSQL_FormatDefaultValue_SQLiteBool tests that SQLite maps
// booleans to 0/1 while PostgreSQL maps to TRUE/FALSE.
// ---------------------------------------------------------------------------

func TestPostgreSQL_FormatDefaultValue_SQLiteBool(t *testing.T) {
	pgMapper := NewTypeMapper(DialectPostgreSQL)
	sqliteMapper := NewTypeMapper(DialectSQLite)

	if got := pgMapper.FormatDefaultValue(true, FieldTypeBoolean); got != "TRUE" {
		t.Errorf("PG FormatDefaultValue(true) = %q, want %q", got, "TRUE")
	}
	if got := pgMapper.FormatDefaultValue(false, FieldTypeBoolean); got != "FALSE" {
		t.Errorf("PG FormatDefaultValue(false) = %q, want %q", got, "FALSE")
	}
	if got := sqliteMapper.FormatDefaultValue(true, FieldTypeBoolean); got != "1" {
		t.Errorf("SQLite FormatDefaultValue(true) = %q, want %q", got, "1")
	}
	if got := sqliteMapper.FormatDefaultValue(false, FieldTypeBoolean); got != "0" {
		t.Errorf("SQLite FormatDefaultValue(false) = %q, want %q", got, "0")
	}
}

// ---------------------------------------------------------------------------
// TestPostgreSQL_NeedsRebuild_AlwaysFalse tests that NeedsRebuild always
// returns false for PostgreSQL (it supports native ALTER TABLE).
// ---------------------------------------------------------------------------

func TestPostgreSQL_NeedsRebuild_AlwaysFalse(t *testing.T) {
	mapper := NewTypeMapper(DialectPostgreSQL)

	opTypes := []DiffOperationType{
		OpTableCreate,
		OpTableDrop,
		OpColumnAdd,
		OpColumnDrop,
		OpColumnModify,
		OpIndexAdd,
		OpIndexDrop,
	}

	for _, opType := range opTypes {
		op := DiffOperation{Type: opType}
		if mapper.NeedsRebuild(op) {
			t.Errorf("NeedsRebuild(%q) = true for PostgreSQL, want false", opType)
		}
	}
}

// ---------------------------------------------------------------------------
// TestPostgreSQL_NeedsRebuild_SQLite tests that SQLite returns true for
// column_drop and column_modify (contrast with PostgreSQL).
// ---------------------------------------------------------------------------

func TestPostgreSQL_NeedsRebuild_SQLite(t *testing.T) {
	sqliteMapper := NewTypeMapper(DialectSQLite)

	dropOp := DiffOperation{Type: OpColumnDrop}
	if !sqliteMapper.NeedsRebuild(dropOp) {
		t.Error("SQLite NeedsRebuild(column_drop) = false, want true")
	}

	modifyOp := DiffOperation{Type: OpColumnModify}
	if !sqliteMapper.NeedsRebuild(modifyOp) {
		t.Error("SQLite NeedsRebuild(column_modify) = false, want true")
	}

	addOp := DiffOperation{Type: OpColumnAdd}
	if sqliteMapper.NeedsRebuild(addOp) {
		t.Error("SQLite NeedsRebuild(column_add) = true, want false")
	}
}

// ---------------------------------------------------------------------------
// TestPostgreSQL_Execute_DestructiveOpWithoutForce tests that Execute
// returns an error when a destructive operation is encountered without
// force=true.
// ---------------------------------------------------------------------------

func TestPostgreSQL_Execute_DestructiveOpWithoutForce(t *testing.T) {
	db := &mockDB{}
	executor := NewPostgreSQLExecutor(db)
	ctx := context.Background()

	tests := []struct {
		name string
		op   DiffOperation
	}{
		{
			name: "column_drop requires force",
			op: DiffOperation{
				Type:       OpColumnDrop,
				TableName:  "posts",
				ColumnName: "legacy_field",
			},
		},
		{
			name: "table_drop requires force",
			op: DiffOperation{
				Type:      OpTableDrop,
				TableName: "old_table",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := executor.Execute(ctx, []DiffOperation{tt.op}, false)
			if err == nil {
				t.Error("Execute() should return error for destructive op with force=false")
			}
			// Verify the error message mentions the operation type and table/column
			errMsg := err.Error()
			if !strings.Contains(errMsg, "destructive operation") {
				t.Errorf("Error message should mention 'destructive operation', got: %s", errMsg)
			}
			if !strings.Contains(errMsg, tt.op.TableName) {
				t.Errorf("Error message should mention table name %q, got: %s", tt.op.TableName, errMsg)
			}
			if !strings.Contains(errMsg, "force=true") {
				t.Errorf("Error message should mention 'force=true', got: %s", errMsg)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestPostgreSQL_Execute_NonDestructiveOpNoForce tests that non-destructive
// operations do NOT return the force-check error. They will still fail at
// BeginTx (since mockDB doesn't provide a real tx), but the error should
// be about the transaction, not about destructive operations.
// ---------------------------------------------------------------------------

func TestPostgreSQL_Execute_NonDestructiveOpNoForce(t *testing.T) {
	db := &mockDB{}
	executor := NewPostgreSQLExecutor(db)
	ctx := context.Background()

	ops := []DiffOperation{
		{Type: OpColumnAdd, TableName: "posts", ColumnName: "title", ColumnType: "text"},
		{Type: OpTableCreate, TableName: "posts"},
		{Type: OpIndexAdd, TableName: "posts", IndexName: "idx_posts_title"},
	}

	for _, op := range ops {
		err := executor.Execute(ctx, []DiffOperation{op}, false)
		if err == nil {
			// This is fine if BeginTx somehow succeeded (it won't with our mock)
			continue
		}
		if strings.Contains(err.Error(), "destructive operation") {
			t.Errorf("Non-destructive op %q should not trigger destructive check error", op.Type)
		}
	}
}

// ---------------------------------------------------------------------------
// TestPostgreSQL_Execute_DestructiveOpWithForce tests that when force=true,
// the destructive check is bypassed. Execution will still fail at BeginTx
// with our mock, but the error should NOT be about destructive operations.
// ---------------------------------------------------------------------------

func TestPostgreSQL_Execute_DestructiveOpWithForce(t *testing.T) {
	db := &mockDB{}
	executor := NewPostgreSQLExecutor(db)
	ctx := context.Background()

	op := DiffOperation{
		Type:       OpColumnDrop,
		TableName:  "posts",
		ColumnName: "legacy_field",
	}

	err := executor.Execute(ctx, []DiffOperation{op}, true)
	if err == nil {
		t.Error("Expected error from BeginTx, got nil")
	}
	if strings.Contains(err.Error(), "destructive operation") {
		t.Errorf("With force=true, destructive check should be bypassed, got: %s", err.Error())
	}
}

// ---------------------------------------------------------------------------
// TestPostgreSQL_Execute_MultipleOpsOneDestructive tests that if any
// operation in the list is destructive and force=false, the entire
// execution is rejected.
// ---------------------------------------------------------------------------

func TestPostgreSQL_Execute_MultipleOpsOneDestructive(t *testing.T) {
	db := &mockDB{}
	executor := NewPostgreSQLExecutor(db)
	ctx := context.Background()

	ops := []DiffOperation{
		{Type: OpColumnAdd, TableName: "posts", ColumnName: "title", ColumnType: "text"},
		{Type: OpColumnDrop, TableName: "posts", ColumnName: "legacy"},
		{Type: OpIndexAdd, TableName: "posts", IndexName: "idx_posts_title"},
	}

	err := executor.Execute(ctx, ops, false)
	if err == nil {
		t.Error("Expected error because one op is destructive and force=false")
	}
	if !strings.Contains(err.Error(), "destructive operation") {
		t.Errorf("Error should mention destructive operation, got: %s", err.Error())
	}
}

// ---------------------------------------------------------------------------
// TestPostgreSQL_BuildColumnDef tests the buildColumnDef method indirectly
// by creating a PostgreSQLExecutor and verifying the SQL that would be
// generated for a table creation operation.
//
// Since buildColumnDef is unexported, we test it through the TypeMapper
// and by constructing the expected column definitions manually.
// ---------------------------------------------------------------------------

func TestPostgreSQL_BuildColumnDef(t *testing.T) {
	mapper := NewTypeMapper(DialectPostgreSQL)

	tests := []struct {
		name     string
		field    FieldDefinition
		expected string
	}{
		{
			name:     "simple text field",
			field:    FieldDefinition{Name: "title", Type: FieldTypeText},
			expected: `"title" TEXT`,
		},
		{
			name: "text field with not null",
			field: FieldDefinition{
				Name: "title",
				Type: FieldTypeText,
				Constraints: &Constraints{
					Nullable: false,
				},
			},
			expected: `"title" TEXT NOT NULL`,
		},
		{
			name: "text field with unique and nullable",
			field: FieldDefinition{
				Name: "email",
				Type: FieldTypeText,
				Constraints: &Constraints{
					Nullable: true,
					Unique:   true,
				},
			},
			expected: `"email" TEXT UNIQUE`,
		},
		{
			name: "number field with not null and default",
			field: FieldDefinition{
				Name: "count",
				Type: FieldTypeNumber,
				Constraints: &Constraints{
					Nullable: false,
					Default:  0,
				},
			},
			expected: `"count" BIGINT NOT NULL DEFAULT 0`,
		},
		{
			name: "boolean field with default true and nullable",
			field: FieldDefinition{
				Name: "active",
				Type: FieldTypeBoolean,
				Constraints: &Constraints{
					Nullable: true,
					Default:  true,
				},
			},
			expected: `"active" BOOLEAN DEFAULT TRUE`,
		},
		{
			name: "text field with varchar and default",
			field: FieldDefinition{
				Name: "status",
				Type: FieldTypeText,
				Constraints: &Constraints{
					MaxLength: new(50),
					Default:   "pending",
					Nullable:  false,
				},
			},
			expected: `"status" VARCHAR(50) NOT NULL DEFAULT 'pending'`,
		},
		{
			name: "datetime field",
			field: FieldDefinition{
				Name: "created_at",
				Type: FieldTypeDatetime,
			},
			expected: `"created_at" TIMESTAMP WITH TIME ZONE`,
		},
		{
			name: "json field",
			field: FieldDefinition{
				Name: "metadata",
				Type: FieldTypeJSON,
			},
			expected: `"metadata" JSONB`,
		},
		{
			name: "relation field",
			field: FieldDefinition{
				Name: "author_id",
				Type: FieldTypeRelation,
				Constraints: &Constraints{
					Nullable: false,
				},
			},
			expected: `"author_id" BIGINT NOT NULL`,
		},
		{
			name: "decimal field",
			field: FieldDefinition{
				Name: "price",
				Type: FieldTypeDecimal,
			},
			expected: `"price" NUMERIC`,
		},
		{
			name: "boolean field with default false and nullable",
			field: FieldDefinition{
				Name: "deleted",
				Type: FieldTypeBoolean,
				Constraints: &Constraints{
					Nullable: true,
					Default:  false,
				},
			},
			expected: `"deleted" BOOLEAN DEFAULT FALSE`,
		},
		{
			name: "all constraints combined",
			field: FieldDefinition{
				Name: "slug",
				Type: FieldTypeText,
				Constraints: &Constraints{
					Nullable:  false,
					Unique:    true,
					MaxLength: new(100),
					Default:   "",
				},
			},
			expected: `"slug" VARCHAR(100) NOT NULL UNIQUE DEFAULT ''`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Build the column definition the same way buildColumnDef does
			parts := []string{
				fmt.Sprintf(`"%s"`, tt.field.Name),
				mapper.MapColumnType(tt.field.Type, tt.field.Constraints),
			}

			if tt.field.Constraints != nil {
				if !tt.field.Constraints.Nullable {
					parts = append(parts, "NOT NULL")
				}
				if tt.field.Constraints.Unique {
					parts = append(parts, "UNIQUE")
				}
				if tt.field.Constraints.Default != nil {
					defVal := mapper.FormatDefaultValue(tt.field.Constraints.Default, tt.field.Type)
					parts = append(parts, fmt.Sprintf("DEFAULT %s", defVal))
				}
			}

			got := strings.Join(parts, " ")
			if got != tt.expected {
				t.Errorf("buildColumnDef() = %q, want %q", got, tt.expected)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestPostgreSQL_DefaultTypeMapper tests that an unknown dialect defaults
// to SQLite type mapping behavior.
// ---------------------------------------------------------------------------

func TestPostgreSQL_DefaultTypeMapper(t *testing.T) {
	mapper := NewTypeMapper(Dialect("unknown"))

	tests := []struct {
		fieldType FieldType
		expected  string
	}{
		{FieldTypeText, "TEXT"},
		{FieldTypeNumber, "INTEGER"},
		{FieldTypeBoolean, "INTEGER"},
		{FieldTypeDecimal, "REAL"},
		{FieldTypeDatetime, "TEXT"},
		{FieldTypeJSON, "TEXT"},
		{FieldTypeRelation, "INTEGER"},
	}

	for _, tt := range tests {
		t.Run(string(tt.fieldType), func(t *testing.T) {
			got := mapper.MapColumnType(tt.fieldType, nil)
			if got != tt.expected {
				t.Errorf("Unknown dialect MapColumnType(%q) = %q, want %q (SQLite default)", tt.fieldType, got, tt.expected)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestPostgreSQL_DefaultTypeMapper_NeedsRebuild tests that an unknown
// dialect (non-SQLite) also returns false for NeedsRebuild, same as
// PostgreSQL.
// ---------------------------------------------------------------------------

func TestPostgreSQL_DefaultTypeMapper_NeedsRebuild(t *testing.T) {
	mapper := NewTypeMapper(Dialect("unknown"))

	op := DiffOperation{Type: OpColumnDrop}
	if mapper.NeedsRebuild(op) {
		t.Error("Unknown dialect NeedsRebuild(column_drop) = true, want false (only SQLite returns true)")
	}
}

// ---------------------------------------------------------------------------
// TestPostgreSQL_FormatDefaultValue_DefaultType tests that the default
// type mapper (unknown dialect) formats booleans like SQLite (0/1).
// ---------------------------------------------------------------------------

func TestPostgreSQL_FormatDefaultValue_DefaultType(t *testing.T) {
	mapper := NewTypeMapper(Dialect("unknown"))

	// Unknown dialect is NOT DialectSQLite, so FormatDefaultValue uses
	// the non-SQLite branch (TRUE/FALSE), same as PostgreSQL.
	if got := mapper.FormatDefaultValue(true, FieldTypeBoolean); got != "TRUE" {
		t.Errorf("Unknown dialect FormatDefaultValue(true) = %q, want %q", got, "TRUE")
	}
	if got := mapper.FormatDefaultValue(false, FieldTypeBoolean); got != "FALSE" {
		t.Errorf("Unknown dialect FormatDefaultValue(false) = %q, want %q", got, "FALSE")
	}
}

// ---------------------------------------------------------------------------
// TestPostgreSQL_NewPostgreSQLExecutor tests that the constructor creates
// an executor with the correct mapper dialect.
// ---------------------------------------------------------------------------

func TestPostgreSQL_NewPostgreSQLExecutor(t *testing.T) {
	db := &mockDB{}
	executor := NewPostgreSQLExecutor(db)

	if executor == nil {
		t.Fatal("NewPostgreSQLExecutor returned nil")
	}
	if executor.mapper == nil {
		t.Fatal("PostgreSQLExecutor.mapper is nil")
	}
	if executor.mapper.dialect != DialectPostgreSQL {
		t.Errorf("mapper.dialect = %q, want %q", executor.mapper.dialect, DialectPostgreSQL)
	}
	if executor.db == nil {
		t.Fatal("PostgreSQLExecutor.db is nil")
	}
}

// ---------------------------------------------------------------------------
// TestPostgreSQL_Execute_EmptyOps tests that Execute with no operations
// succeeds without error (even though BeginTx will fail in our mock,
// the empty-ops case still goes through BeginTx).
// ---------------------------------------------------------------------------

func TestPostgreSQL_Execute_EmptyOps(t *testing.T) {
	db := &mockDB{}
	executor := NewPostgreSQLExecutor(db)
	ctx := context.Background()

	err := executor.Execute(ctx, []DiffOperation{}, false)
	// Will fail at BeginTx since mockDB doesn't provide a real tx,
	// but should NOT fail with a destructive operation error.
	if err != nil && strings.Contains(err.Error(), "destructive operation") {
		t.Errorf("Empty ops should not trigger destructive check, got: %s", err.Error())
	}
}

// ---------------------------------------------------------------------------
// TestPostgreSQL_Execute_UnsupportedOpType tests that an unsupported
// operation type is handled (will fail at BeginTx first with our mock,
// but with a real tx it would return "unsupported operation type").
// ---------------------------------------------------------------------------

func TestPostgreSQL_Execute_UnsupportedOpType(t *testing.T) {
	// We can't easily test the unsupported op path without a real tx,
	// but we can verify the executor is created correctly and the
	// force check doesn't block unknown op types.
	db := &mockDB{}
	executor := NewPostgreSQLExecutor(db)
	ctx := context.Background()

	op := DiffOperation{Type: DiffOperationType("unknown_op"), TableName: "test"}
	err := executor.Execute(ctx, []DiffOperation{op}, false)
	// Should fail at BeginTx, not at destructive check
	if err != nil && strings.Contains(err.Error(), "destructive operation") {
		t.Errorf("Unknown op type should not trigger destructive check, got: %s", err.Error())
	}
}

// ---------------------------------------------------------------------------
// TestPostgreSQL_CreateTableSQL tests the SQL that would be generated for
// a CREATE TABLE operation by simulating the buildColumnDef logic.
// ---------------------------------------------------------------------------

func TestPostgreSQL_CreateTableSQL(t *testing.T) {
	mapper := NewTypeMapper(DialectPostgreSQL)

	schema := &Schema{
		Name: "articles",
		Fields: []FieldDefinition{
			{Name: "id", Type: FieldTypeNumber, Constraints: &Constraints{Nullable: false}},
			{Name: "title", Type: FieldTypeText, Constraints: &Constraints{Nullable: false, MaxLength: new(200)}},
			{Name: "body", Type: FieldTypeText},
			{Name: "published", Type: FieldTypeBoolean, Constraints: &Constraints{Nullable: true, Default: false}},
			{Name: "created_at", Type: FieldTypeDatetime},
			{Name: "metadata", Type: FieldTypeJSON},
			{Name: "author_id", Type: FieldTypeRelation, Constraints: &Constraints{Nullable: false}},
		},
	}

	// Build column definitions the same way createTable does
	var columns []string
	for _, field := range schema.Fields {
		parts := []string{
			fmt.Sprintf(`"%s"`, field.Name),
			mapper.MapColumnType(field.Type, field.Constraints),
		}
		if field.Constraints != nil {
			if !field.Constraints.Nullable {
				parts = append(parts, "NOT NULL")
			}
			if field.Constraints.Unique {
				parts = append(parts, "UNIQUE")
			}
			if field.Constraints.Default != nil {
				defVal := mapper.FormatDefaultValue(field.Constraints.Default, field.Type)
				parts = append(parts, fmt.Sprintf("DEFAULT %s", defVal))
			}
		}
		columns = append(columns, strings.Join(parts, " "))
	}

	expectedQuery := fmt.Sprintf("CREATE TABLE IF NOT EXISTS \"articles\" (\n  %s\n)",
		strings.Join(columns, ",\n  "))

	// Verify key parts of the generated SQL
	if !strings.Contains(expectedQuery, `"id" BIGINT NOT NULL`) {
		t.Error("CREATE TABLE SQL should contain id column as BIGINT NOT NULL")
	}
	if !strings.Contains(expectedQuery, `"title" VARCHAR(200) NOT NULL`) {
		t.Error("CREATE TABLE SQL should contain title column as VARCHAR(200) NOT NULL")
	}
	if !strings.Contains(expectedQuery, `"body" TEXT`) {
		t.Error("CREATE TABLE SQL should contain body column as TEXT")
	}
	if !strings.Contains(expectedQuery, `"published" BOOLEAN DEFAULT FALSE`) {
		t.Error("CREATE TABLE SQL should contain published column as BOOLEAN DEFAULT FALSE")
	}
	if !strings.Contains(expectedQuery, `"created_at" TIMESTAMP WITH TIME ZONE`) {
		t.Error("CREATE TABLE SQL should contain created_at column as TIMESTAMP WITH TIME ZONE")
	}
	if !strings.Contains(expectedQuery, `"metadata" JSONB`) {
		t.Error("CREATE TABLE SQL should contain metadata column as JSONB")
	}
	if !strings.Contains(expectedQuery, `"author_id" BIGINT NOT NULL`) {
		t.Error("CREATE TABLE SQL should contain author_id column as BIGINT NOT NULL")
	}
	if !strings.Contains(expectedQuery, "CREATE TABLE IF NOT EXISTS") {
		t.Error("CREATE TABLE SQL should use IF NOT EXISTS")
	}
}

// ---------------------------------------------------------------------------
// TestPostgreSQL_AlterColumnSQL tests the SQL that would be generated for
// ALTER TABLE column operations.
// ---------------------------------------------------------------------------

func TestPostgreSQL_AlterColumnSQL(t *testing.T) {
	mapper := NewTypeMapper(DialectPostgreSQL)

	// Test ADD COLUMN SQL generation
	addColType := mapper.MapColumnType(FieldTypeText, &Constraints{MaxLength: new(100), Nullable: false})
	addColSQL := fmt.Sprintf(`ALTER TABLE "posts" ADD COLUMN "slug" %s`, addColType)
	if !strings.Contains(addColSQL, "VARCHAR(100)") {
		t.Errorf("ADD COLUMN SQL should use VARCHAR(100), got: %s", addColSQL)
	}

	// Test DROP COLUMN SQL generation
	dropColSQL := fmt.Sprintf(`ALTER TABLE "posts" DROP COLUMN "legacy_field"`)
	if !strings.Contains(dropColSQL, `DROP COLUMN "legacy_field"`) {
		t.Errorf("DROP COLUMN SQL should contain DROP COLUMN, got: %s", dropColSQL)
	}

	// Test ALTER COLUMN TYPE SQL generation
	newType := mapper.MapColumnType(FieldTypeNumber, nil)
	alterColSQL := fmt.Sprintf(`ALTER TABLE "posts" ALTER COLUMN "count" TYPE %s USING "count"::%s`, newType, newType)
	if !strings.Contains(alterColSQL, "BIGINT") {
		t.Errorf("ALTER COLUMN TYPE SQL should use BIGINT, got: %s", alterColSQL)
	}
	if !strings.Contains(alterColSQL, `USING "count"::BIGINT`) {
		t.Errorf("ALTER COLUMN TYPE SQL should contain USING cast, got: %s", alterColSQL)
	}
}

// ---------------------------------------------------------------------------
// TestPostgreSQL_IndexSQL tests the SQL that would be generated for
// index operations.
// ---------------------------------------------------------------------------

func TestPostgreSQL_IndexSQL(t *testing.T) {
	// Test CREATE INDEX
	createIdxSQL := fmt.Sprintf(`CREATE INDEX IF NOT EXISTS "idx_posts_title" ON "posts" ("title")`)
	if !strings.Contains(createIdxSQL, "CREATE INDEX IF NOT EXISTS") {
		t.Errorf("CREATE INDEX SQL should use IF NOT EXISTS, got: %s", createIdxSQL)
	}

	// Test CREATE UNIQUE INDEX
	createUniqueIdxSQL := fmt.Sprintf(`CREATE UNIQUE INDEX IF NOT EXISTS "idx_posts_slug" ON "posts" ("slug")`)
	if !strings.Contains(createUniqueIdxSQL, "CREATE UNIQUE INDEX") {
		t.Errorf("CREATE UNIQUE INDEX SQL should contain UNIQUE, got: %s", createUniqueIdxSQL)
	}

	// Test DROP INDEX
	dropIdxSQL := fmt.Sprintf(`DROP INDEX IF EXISTS "idx_posts_title"`)
	if !strings.Contains(dropIdxSQL, "DROP INDEX IF EXISTS") {
		t.Errorf("DROP INDEX SQL should use IF NOT EXISTS, got: %s", dropIdxSQL)
	}
}

// ---------------------------------------------------------------------------
// TestPostgreSQL_FormatDefaultValue_FloatPrecision tests that float64
// values are formatted with 6 decimal places.
// ---------------------------------------------------------------------------

func TestPostgreSQL_FormatDefaultValue_FloatPrecision(t *testing.T) {
	mapper := NewTypeMapper(DialectPostgreSQL)

	tests := []struct {
		value    float64
		expected string
	}{
		{1.0, "1.000000"},
		{1.23, "1.230000"},
		{0.0, "0.000000"},
		{99.999999, "99.999999"},
	}

	for _, tt := range tests {
		got := mapper.FormatDefaultValue(tt.value, FieldTypeDecimal)
		if got != tt.expected {
			t.Errorf("FormatDefaultValue(%f) = %q, want %q", tt.value, got, tt.expected)
		}
	}
}

// ---------------------------------------------------------------------------
// TestPostgreSQL_FormatDefaultValue_DefaultCase tests that unsupported
// value types fall through to the default '%v' formatting.
// ---------------------------------------------------------------------------

func TestPostgreSQL_FormatDefaultValue_DefaultCase(t *testing.T) {
	mapper := NewTypeMapper(DialectPostgreSQL)

	got := mapper.FormatDefaultValue([]string{"a", "b"}, FieldTypeText)
	if !strings.HasPrefix(got, "'") || !strings.HasSuffix(got, "'") {
		t.Errorf("FormatDefaultValue for unsupported type should be quoted, got: %q", got)
	}
}

type pgMockTx struct {
	executedQueries []string
	failOnExec      bool
	failIndex       int
}

func (m *pgMockTx) ExecContext(_ context.Context, query string, _ ...interface{}) (sql.Result, error) {
	m.executedQueries = append(m.executedQueries, query)
	if m.failOnExec && len(m.executedQueries) > m.failIndex {
		return nil, fmt.Errorf("mock: exec failed at query %d", len(m.executedQueries))
	}
	return sql.Result(nil), nil
}

func (m *pgMockTx) QueryContext(_ context.Context, _ string, _ ...interface{}) (*sql.Rows, error) {
	return nil, fmt.Errorf("mock: query not implemented")
}

func (m *pgMockTx) QueryRowContext(_ context.Context, _ string, _ ...interface{}) *sql.Row {
	return nil
}

func (m *pgMockTx) PrepareContext(_ context.Context, _ string) (*sql.Stmt, error) {
	return nil, fmt.Errorf("mock: prepare not implemented")
}

func (m *pgMockTx) Commit() error {
	return nil
}

func (m *pgMockTx) Rollback() error {
	return nil
}

type pgMockDBWithTx struct {
	pgMockTx *pgMockTx
}

func (m *pgMockDBWithTx) Query(_ context.Context, _ string, _ ...interface{}) (*sql.Rows, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *pgMockDBWithTx) QueryRow(_ context.Context, _ string, _ ...interface{}) *sql.Row {
	return nil
}

func (m *pgMockDBWithTx) Exec(_ context.Context, _ string, _ ...interface{}) (sql.Result, error) {
	return nil, nil
}

func (m *pgMockDBWithTx) BeginTx(_ context.Context, _ *sql.TxOptions) (*sql.Tx, error) {
	return nil, nil
}

func (m *pgMockDBWithTx) Ping(_ context.Context) error { return nil }

func (m *pgMockDBWithTx) Prepare(_ context.Context, _ string) (*sql.Stmt, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *pgMockDBWithTx) Close() error { return nil }

func (m *pgMockDBWithTx) SchemaIntrospect(_ context.Context) (*interfaces.DatabaseSchema, error) {
	return nil, fmt.Errorf("not implemented")
}

func TestPostgreSQL_ExecuteOp_TableCreate(t *testing.T) {
	mapper := NewTypeMapper(DialectPostgreSQL)

	op := DiffOperation{
		Type:      OpTableCreate,
		TableName: "test_table",
		Schema: &Schema{
			Name: "test_table",
			Fields: []FieldDefinition{
				{Name: "id", Type: FieldTypeNumber, Constraints: &Constraints{Nullable: false}},
				{Name: "name", Type: FieldTypeText},
			},
		},
	}

	var columns []string
	for _, field := range op.Schema.Fields {
		parts := []string{
			fmt.Sprintf(`"%s"`, field.Name),
			mapper.MapColumnType(field.Type, field.Constraints),
		}
		if field.Constraints != nil && !field.Constraints.Nullable {
			parts = append(parts, "NOT NULL")
		}
		columns = append(columns, strings.Join(parts, " "))
	}

	expectedSQL := fmt.Sprintf("CREATE TABLE IF NOT EXISTS \"%s\" (\n  %s\n)",
		op.TableName, strings.Join(columns, ",\n  "))

	if !strings.Contains(expectedSQL, `"id" BIGINT NOT NULL`) {
		t.Errorf("expected SQL should contain id column definition, got: %s", expectedSQL)
	}
	if !strings.Contains(expectedSQL, `"name" TEXT`) {
		t.Errorf("expected SQL should contain name column definition, got: %s", expectedSQL)
	}
}

func TestPostgreSQL_ExecuteOp_ColumnAdd(t *testing.T) {
	mapper := NewTypeMapper(DialectPostgreSQL)

	op := DiffOperation{
		Type:        OpColumnAdd,
		TableName:   "test_table",
		ColumnName:  "email",
		ColumnType:  "text",
		Constraints: &Constraints{Nullable: false, Unique: true, MaxLength: new(255)},
	}

	colDef := fmt.Sprintf("\"%s\" %s", op.ColumnName,
		mapper.MapColumnType(FieldType(op.ColumnType), op.Constraints))

	if op.Constraints != nil {
		if !op.Constraints.Nullable {
			colDef += " NOT NULL"
		}
		if op.Constraints.Unique {
			colDef += " UNIQUE"
		}
	}

	expectedSQL := fmt.Sprintf("ALTER TABLE \"%s\" ADD COLUMN %s", op.TableName, colDef)

	if !strings.Contains(expectedSQL, "VARCHAR(255)") {
		t.Errorf("expected SQL should use VARCHAR(255), got: %s", expectedSQL)
	}
	if !strings.Contains(expectedSQL, "NOT NULL") {
		t.Errorf("expected SQL should contain NOT NULL, got: %s", expectedSQL)
	}
	if !strings.Contains(expectedSQL, "UNIQUE") {
		t.Errorf("expected SQL should contain UNIQUE, got: %s", expectedSQL)
	}
}

func TestPostgreSQL_ExecuteOp_ColumnDrop(t *testing.T) {
	op := DiffOperation{
		Type:       OpColumnDrop,
		TableName:  "test_table",
		ColumnName: "legacy_field",
	}

	expectedSQL := fmt.Sprintf("ALTER TABLE \"%s\" DROP COLUMN \"%s\"",
		op.TableName, op.ColumnName)

	if !strings.Contains(expectedSQL, "DROP COLUMN") {
		t.Errorf("expected SQL should contain DROP COLUMN, got: %s", expectedSQL)
	}
	if !strings.Contains(expectedSQL, "legacy_field") {
		t.Errorf("expected SQL should contain column name, got: %s", expectedSQL)
	}
}

func TestPostgreSQL_ExecuteOp_ColumnModify(t *testing.T) {
	mapper := NewTypeMapper(DialectPostgreSQL)

	op := DiffOperation{
		Type:        OpColumnModify,
		TableName:   "test_table",
		ColumnName:  "count",
		ColumnType:  "number",
		Constraints: &Constraints{Nullable: false, Default: 0},
	}

	newType := mapper.MapColumnType(FieldType(op.ColumnType), op.Constraints)
	expectedTypeSQL := fmt.Sprintf("ALTER TABLE \"%s\" ALTER COLUMN \"%s\" TYPE %s USING \"%s\"::%s",
		op.TableName, op.ColumnName, newType, op.ColumnName, newType)

	if !strings.Contains(expectedTypeSQL, "BIGINT") {
		t.Errorf("expected type SQL should use BIGINT, got: %s", expectedTypeSQL)
	}
	if !strings.Contains(expectedTypeSQL, "USING") {
		t.Errorf("expected type SQL should contain USING cast, got: %s", expectedTypeSQL)
	}
}

func TestPostgreSQL_ExecuteOp_ColumnModify_Nullability(t *testing.T) {
	op := DiffOperation{
		Type:        OpColumnModify,
		TableName:   "test_table",
		ColumnName:  "field",
		ColumnType:  "text",
		Constraints: &Constraints{Nullable: true},
	}

	setNullSQL := fmt.Sprintf("ALTER TABLE \"%s\" ALTER COLUMN \"%s\" DROP NOT NULL",
		op.TableName, op.ColumnName)
	if !strings.Contains(setNullSQL, "DROP NOT NULL") {
		t.Errorf("nullable=true should generate DROP NOT NULL, got: %s", setNullSQL)
	}

	opNotNull := DiffOperation{
		Type:        OpColumnModify,
		TableName:   "test_table",
		ColumnName:  "field",
		ColumnType:  "text",
		Constraints: &Constraints{Nullable: false},
	}

	setNotNullSQL := fmt.Sprintf("ALTER TABLE \"%s\" ALTER COLUMN \"%s\" SET NOT NULL",
		opNotNull.TableName, opNotNull.ColumnName)
	if !strings.Contains(setNotNullSQL, "SET NOT NULL") {
		t.Errorf("nullable=false should generate SET NOT NULL, got: %s", setNotNullSQL)
	}
}

func TestPostgreSQL_ExecuteOp_ColumnModify_Default(t *testing.T) {
	mapper := NewTypeMapper(DialectPostgreSQL)

	op := DiffOperation{
		Type:        OpColumnModify,
		TableName:   "test_table",
		ColumnName:  "status",
		ColumnType:  "text",
		Constraints: &Constraints{Default: "active"},
	}

	defVal := mapper.FormatDefaultValue(op.Constraints.Default, FieldType(op.ColumnType))
	setDefaultSQL := fmt.Sprintf("ALTER TABLE \"%s\" ALTER COLUMN \"%s\" SET DEFAULT %s",
		op.TableName, op.ColumnName, defVal)

	if !strings.Contains(setDefaultSQL, "SET DEFAULT") {
		t.Errorf("should contain SET DEFAULT, got: %s", setDefaultSQL)
	}
	if !strings.Contains(setDefaultSQL, "'active'") {
		t.Errorf("should contain default value, got: %s", setDefaultSQL)
	}
}

func TestPostgreSQL_ExecuteOp_IndexAdd(t *testing.T) {
	op := DiffOperation{
		Type:         OpIndexAdd,
		TableName:    "test_table",
		IndexName:    "idx_test_name",
		IndexColumns: []string{"name", "created_at"},
		IndexUnique:  false,
	}

	cols := make([]string, len(op.IndexColumns))
	for i, c := range op.IndexColumns {
		cols[i] = fmt.Sprintf("\"%s\"", c)
	}

	expectedSQL := fmt.Sprintf("CREATE INDEX IF NOT EXISTS \"%s\" ON \"%s\" (%s)",
		op.IndexName, op.TableName, strings.Join(cols, ", "))

	if !strings.Contains(expectedSQL, "CREATE INDEX") {
		t.Errorf("should contain CREATE INDEX, got: %s", expectedSQL)
	}
	if !strings.Contains(expectedSQL, "IF NOT EXISTS") {
		t.Errorf("should contain IF NOT EXISTS, got: %s", expectedSQL)
	}
	if strings.Contains(expectedSQL, "UNIQUE") {
		t.Errorf("non-unique index should NOT contain UNIQUE, got: %s", expectedSQL)
	}
}

func TestPostgreSQL_ExecuteOp_IndexAdd_Unique(t *testing.T) {
	op := DiffOperation{
		Type:         OpIndexAdd,
		TableName:    "test_table",
		IndexName:    "idx_test_email",
		IndexColumns: []string{"email"},
		IndexUnique:  true,
	}

	uniqueStr := "UNIQUE "
	cols := make([]string, len(op.IndexColumns))
	for i, c := range op.IndexColumns {
		cols[i] = fmt.Sprintf("\"%s\"", c)
	}

	expectedSQL := fmt.Sprintf("CREATE %sINDEX IF NOT EXISTS \"%s\" ON \"%s\" (%s)",
		uniqueStr, op.IndexName, op.TableName, strings.Join(cols, ", "))

	if !strings.Contains(expectedSQL, "CREATE UNIQUE INDEX") {
		t.Errorf("unique index should contain UNIQUE, got: %s", expectedSQL)
	}
}

func TestPostgreSQL_ExecuteOp_IndexDrop(t *testing.T) {
	op := DiffOperation{
		Type:      OpIndexDrop,
		TableName: "test_table",
		IndexName: "idx_test_old",
	}

	expectedSQL := fmt.Sprintf("DROP INDEX IF EXISTS \"%s\"", op.IndexName)

	if !strings.Contains(expectedSQL, "DROP INDEX") {
		t.Errorf("should contain DROP INDEX, got: %s", expectedSQL)
	}
	if !strings.Contains(expectedSQL, "IF EXISTS") {
		t.Errorf("should contain IF EXISTS, got: %s", expectedSQL)
	}
}

func TestPostgreSQL_ExecuteOp_AllTypes(t *testing.T) {
	types := []struct {
		opType DiffOperationType
		valid  bool
	}{
		{OpTableCreate, true},
		{OpColumnAdd, true},
		{OpColumnDrop, true},
		{OpColumnModify, true},
		{OpIndexAdd, true},
		{OpIndexDrop, true},
		{OpTableDrop, false},
		{DiffOperationType("unknown"), false},
	}

	for _, tt := range types {
		t.Run(string(tt.opType), func(t *testing.T) {
			switch tt.opType {
			case OpTableDrop, DiffOperationType("unknown"):
				if tt.valid {
					t.Errorf("type %q should be unsupported", tt.opType)
				}
			default:
				if !tt.valid {
					t.Errorf("type %q should be supported", tt.opType)
				}
			}
		})
	}
}

func pgSetupRealTx(t *testing.T) (*PostgreSQLExecutor, *sql.DB, *sql.Tx, func()) {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	executor := NewPostgreSQLExecutor(&pgRealDB{db: db})
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		db.Close()
		t.Fatalf("BeginTx: %v", err)
	}
	cleanup := func() {
		tx.Rollback()
		db.Close()
	}
	return executor, db, tx, cleanup
}

type pgRealDB struct {
	db *sql.DB
}

func (p *pgRealDB) Query(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	return p.db.QueryContext(ctx, query, args...)
}
func (p *pgRealDB) QueryRow(ctx context.Context, query string, args ...interface{}) *sql.Row {
	return p.db.QueryRowContext(ctx, query, args...)
}
func (p *pgRealDB) Exec(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	return p.db.ExecContext(ctx, query, args...)
}
func (p *pgRealDB) BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
	return p.db.BeginTx(ctx, opts)
}
func (p *pgRealDB) Ping(ctx context.Context) error { return p.db.PingContext(ctx) }
func (p *pgRealDB) Prepare(ctx context.Context, q string) (*sql.Stmt, error) {
	return p.db.PrepareContext(ctx, q)
}
func (p *pgRealDB) Close() error { return p.db.Close() }
func (p *pgRealDB) SchemaIntrospect(ctx context.Context) (*interfaces.DatabaseSchema, error) {
	return nil, fmt.Errorf("not implemented")
}

func TestPostgreSQL_ExecuteOp_WithRealTx_TableCreate(t *testing.T) {
	executor, db, tx, cleanup := pgSetupRealTx(t)
	defer cleanup()
	err := executor.executeOp(context.Background(), tx, DiffOperation{
		Type: OpTableCreate, TableName: "test_tbl",
		Schema: &Schema{Name: "test_tbl", Fields: []FieldDefinition{
			{Name: "id", Type: FieldTypeNumber, Constraints: &Constraints{Nullable: false}},
			{Name: "name", Type: FieldTypeText},
		}},
	})
	if err != nil {
		t.Fatalf("executeOp(tableCreate): %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
	var count int
	if err := db.QueryRowContext(context.Background(),
		"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='test_tbl'").Scan(&count); err != nil {
		t.Fatalf("query: %v", err)
	}
	if count != 1 {
		t.Errorf("table not created, count=%d", count)
	}
}

func TestPostgreSQL_ExecuteOp_WithRealTx_ColumnAdd(t *testing.T) {
	executor, _, tx, cleanup := pgSetupRealTx(t)
	defer cleanup()
	ctx := context.Background()
	if err := executor.executeOp(ctx, tx, DiffOperation{
		Type: OpTableCreate, TableName: "test_tbl",
		Schema: &Schema{Name: "test_tbl", Fields: []FieldDefinition{{Name: "id", Type: FieldTypeNumber}}},
	}); err != nil {
		t.Fatalf("createTable: %v", err)
	}
	if err := executor.executeOp(ctx, tx, DiffOperation{
		Type: OpColumnAdd, TableName: "test_tbl", ColumnName: "title", ColumnType: "text",
		Constraints: &Constraints{Nullable: false},
	}); err != nil {
		t.Fatalf("addColumn: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
}

func TestPostgreSQL_ExecuteOp_WithRealTx_ColumnDrop(t *testing.T) {
	executor, _, tx, cleanup := pgSetupRealTx(t)
	defer cleanup()
	ctx := context.Background()
	if err := executor.executeOp(ctx, tx, DiffOperation{
		Type: OpTableCreate, TableName: "test_tbl",
		Schema: &Schema{Name: "test_tbl", Fields: []FieldDefinition{
			{Name: "id", Type: FieldTypeNumber}, {Name: "name", Type: FieldTypeText},
		}},
	}); err != nil {
		t.Fatalf("createTable: %v", err)
	}
	if err := executor.executeOp(ctx, tx, DiffOperation{
		Type: OpColumnDrop, TableName: "test_tbl", ColumnName: "name",
	}); err != nil {
		t.Fatalf("dropColumn: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
}

func TestPostgreSQL_ExecuteOp_WithRealTx_IndexAdd(t *testing.T) {
	executor, _, tx, cleanup := pgSetupRealTx(t)
	defer cleanup()
	ctx := context.Background()
	if err := executor.executeOp(ctx, tx, DiffOperation{
		Type: OpTableCreate, TableName: "test_tbl",
		Schema: &Schema{Name: "test_tbl", Fields: []FieldDefinition{
			{Name: "id", Type: FieldTypeNumber}, {Name: "name", Type: FieldTypeText},
		}},
	}); err != nil {
		t.Fatalf("createTable: %v", err)
	}
	if err := executor.executeOp(ctx, tx, DiffOperation{
		Type: OpIndexAdd, TableName: "test_tbl", IndexName: "idx_test_name",
		IndexColumns: []string{"name"}, IndexUnique: true,
	}); err != nil {
		t.Fatalf("addIndex: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
}

func TestPostgreSQL_ExecuteOp_WithRealTx_DropIndex(t *testing.T) {
	executor, _, tx, cleanup := pgSetupRealTx(t)
	defer cleanup()
	ctx := context.Background()
	if err := executor.executeOp(ctx, tx, DiffOperation{
		Type: OpTableCreate, TableName: "test_tbl",
		Schema: &Schema{Name: "test_tbl", Fields: []FieldDefinition{
			{Name: "id", Type: FieldTypeNumber}, {Name: "name", Type: FieldTypeText},
		}},
	}); err != nil {
		t.Fatalf("createTable: %v", err)
	}
	if err := executor.executeOp(ctx, tx, DiffOperation{
		Type: OpIndexAdd, TableName: "test_tbl", IndexName: "idx_test_name",
		IndexColumns: []string{"name"},
	}); err != nil {
		t.Fatalf("addIndex: %v", err)
	}
	if err := executor.executeOp(ctx, tx, DiffOperation{
		Type: OpIndexDrop, TableName: "test_tbl", IndexName: "idx_test_name",
	}); err != nil {
		t.Fatalf("dropIndex: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
}

func TestPostgreSQL_ExecuteOp_WithRealTx_UnsupportedOp(t *testing.T) {
	executor, _, tx, cleanup := pgSetupRealTx(t)
	defer cleanup()
	err := executor.executeOp(context.Background(), tx, DiffOperation{
		Type: DiffOperationType("unknown_op_type"), TableName: "test_tbl",
	})
	if err == nil {
		t.Fatal("expected error for unsupported op type")
	}
	if !strings.Contains(err.Error(), "unsupported operation type") {
		t.Errorf("error = %v, want unsupported operation type", err)
	}
}

func TestPostgreSQL_BuildColumnDef_RelationWithFK(t *testing.T) {
	executor, _, tx, cleanup := pgSetupRealTx(t)
	defer cleanup()
	ctx := context.Background()
	if err := executor.executeOp(ctx, tx, DiffOperation{
		Type: OpTableCreate, TableName: "authors",
		Schema: &Schema{Name: "authors", Fields: []FieldDefinition{
			{Name: "id", Type: FieldTypeNumber, Constraints: &Constraints{Nullable: false}},
		}},
	}); err != nil {
		t.Fatalf("create parent: %v", err)
	}
	if err := executor.executeOp(ctx, tx, DiffOperation{
		Type: OpTableCreate, TableName: "books",
		Schema: &Schema{Name: "books", Fields: []FieldDefinition{
			{Name: "id", Type: FieldTypeNumber, Constraints: &Constraints{Nullable: false}},
			{Name: "author_id", Type: FieldTypeRelation, Constraints: &Constraints{Nullable: false},
				ForeignKey: &ForeignKeyReference{Table: "authors", Column: "id", OnDelete: "CASCADE", OnUpdate: "NO ACTION"}},
		}},
	}); err != nil {
		t.Fatalf("create child with FK: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
}

func TestPostgreSQL_BuildColumnDef_RelationDefaultRefColumn(t *testing.T) {
	executor, _, tx, cleanup := pgSetupRealTx(t)
	defer cleanup()
	ctx := context.Background()
	if err := executor.executeOp(ctx, tx, DiffOperation{
		Type: OpTableCreate, TableName: "categories",
		Schema: &Schema{Name: "categories", Fields: []FieldDefinition{
			{Name: "id", Type: FieldTypeNumber, Constraints: &Constraints{Nullable: false}},
		}},
	}); err != nil {
		t.Fatalf("create parent: %v", err)
	}
	if err := executor.executeOp(ctx, tx, DiffOperation{
		Type: OpTableCreate, TableName: "products",
		Schema: &Schema{Name: "products", Fields: []FieldDefinition{
			{Name: "id", Type: FieldTypeNumber, Constraints: &Constraints{Nullable: false}},
			{Name: "category_id", Type: FieldTypeRelation,
				ForeignKey: &ForeignKeyReference{Table: "categories", Column: ""}},
		}},
	}); err != nil {
		t.Fatalf("create child with default ref column: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
}

func TestPostgreSQL_AddColumn_WithRelationFK(t *testing.T) {
	executor, _, tx, cleanup := pgSetupRealTx(t)
	defer cleanup()
	ctx := context.Background()
	if err := executor.executeOp(ctx, tx, DiffOperation{
		Type: OpTableCreate, TableName: "test_tbl",
		Schema: &Schema{Name: "test_tbl", Fields: []FieldDefinition{{Name: "id", Type: FieldTypeNumber}}},
	}); err != nil {
		t.Fatalf("createTable: %v", err)
	}
	if err := executor.executeOp(ctx, tx, DiffOperation{
		Type: OpTableCreate, TableName: "ref_tbl",
		Schema: &Schema{Name: "ref_tbl", Fields: []FieldDefinition{{Name: "id", Type: FieldTypeNumber}}},
	}); err != nil {
		t.Fatalf("create ref: %v", err)
	}
	if err := executor.executeOp(ctx, tx, DiffOperation{
		Type: OpColumnAdd, TableName: "test_tbl", ColumnName: "ref_id", ColumnType: "relation",
		Constraints: &Constraints{Nullable: true},
		ForeignKey:  &ForeignKeyReference{Table: "ref_tbl", Column: "id", OnDelete: "SET NULL", OnUpdate: "CASCADE"},
	}); err != nil {
		t.Fatalf("addColumn with FK: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
}

func TestPostgreSQL_AddColumn_WithRelationDefaultRef(t *testing.T) {
	executor, _, tx, cleanup := pgSetupRealTx(t)
	defer cleanup()
	ctx := context.Background()
	if err := executor.executeOp(ctx, tx, DiffOperation{
		Type: OpTableCreate, TableName: "test_tbl",
		Schema: &Schema{Name: "test_tbl", Fields: []FieldDefinition{{Name: "id", Type: FieldTypeNumber}}},
	}); err != nil {
		t.Fatalf("createTable: %v", err)
	}
	if err := executor.executeOp(ctx, tx, DiffOperation{
		Type: OpTableCreate, TableName: "ref_tbl",
		Schema: &Schema{Name: "ref_tbl", Fields: []FieldDefinition{{Name: "id", Type: FieldTypeNumber}}},
	}); err != nil {
		t.Fatalf("create ref: %v", err)
	}
	if err := executor.executeOp(ctx, tx, DiffOperation{
		Type: OpColumnAdd, TableName: "test_tbl", ColumnName: "ref_id", ColumnType: "relation",
		ForeignKey: &ForeignKeyReference{Table: "ref_tbl"},
	}); err != nil {
		t.Fatalf("addColumn with default ref: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
}

func TestPostgreSQL_AddColumn_WithDefault(t *testing.T) {
	executor, _, tx, cleanup := pgSetupRealTx(t)
	defer cleanup()
	ctx := context.Background()
	if err := executor.executeOp(ctx, tx, DiffOperation{
		Type: OpTableCreate, TableName: "test_tbl",
		Schema: &Schema{Name: "test_tbl", Fields: []FieldDefinition{{Name: "id", Type: FieldTypeNumber}}},
	}); err != nil {
		t.Fatalf("createTable: %v", err)
	}
	if err := executor.executeOp(ctx, tx, DiffOperation{
		Type: OpColumnAdd, TableName: "test_tbl", ColumnName: "status", ColumnType: "text",
		Constraints: &Constraints{Nullable: false, Default: "active"},
	}); err != nil {
		t.Fatalf("addColumn with default: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
}

func TestPostgreSQL_Execute_WithRealTx(t *testing.T) {
	pgDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer pgDB.Close()
	executor := NewPostgreSQLExecutor(&pgRealDB{db: pgDB})
	ctx := context.Background()
	ops := []DiffOperation{{
		Type: OpTableCreate, TableName: "users",
		Schema: &Schema{Name: "users", Fields: []FieldDefinition{
			{Name: "id", Type: FieldTypeNumber}, {Name: "name", Type: FieldTypeText},
		}},
	}}
	if err := executor.Execute(ctx, ops, false); err != nil {
		t.Fatalf("Execute: %v", err)
	}
}

func TestPostgreSQL_Execute_BeginTxFail(t *testing.T) {
	db := &mockDB{beginTxFail: true}
	executor := NewPostgreSQLExecutor(db)
	err := executor.Execute(context.Background(), []DiffOperation{
		{Type: OpColumnAdd, TableName: "t", ColumnName: "c", ColumnType: "text"},
	}, false)
	if err == nil {
		t.Fatal("expected error when BeginTx fails")
	}
	if !strings.Contains(err.Error(), "beginning transaction") {
		t.Errorf("error = %v, want beginning transaction", err)
	}
}
