package ddl

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/wangling-miao/aroute/sdk/interfaces"
)

type SQLiteExecutor struct {
	db     interfaces.DatabaseService
	mapper *TypeMapper
}

func NewSQLiteExecutor(db interfaces.DatabaseService) *SQLiteExecutor {
	return &SQLiteExecutor{
		db:     db,
		mapper: NewTypeMapper(DialectSQLite),
	}
}

func (e *SQLiteExecutor) Execute(ctx context.Context, ops []DiffOperation, force bool) error {
	for _, op := range ops {
		if op.IsDestructive() && !force {
			return fmt.Errorf("destructive operation '%s' on '%s.%s' requires explicit confirmation (force=true)",
				op.Type, op.TableName, op.ColumnName)
		}
	}

	for _, op := range ops {
		if err := validateOperation(op); err != nil {
			return fmt.Errorf("invalid operation: %w", err)
		}
	}

	tx, err := e.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()

	for _, op := range ops {
		if err := e.executeOp(ctx, tx, op); err != nil {
			return fmt.Errorf("executing %s: %w", op.Type, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}

	return nil
}

func (e *SQLiteExecutor) executeOp(ctx context.Context, tx *sql.Tx, op DiffOperation) error {
	if e.mapper.NeedsRebuild(op) {
		return e.rebuildTable(ctx, tx, op)
	}

	switch op.Type {
	case OpTableCreate:
		return e.createTable(ctx, tx, op)
	case OpColumnAdd:
		return e.addColumn(ctx, tx, op)
	case OpColumnDrop:
		return e.dropColumn(ctx, tx, op)
	case OpColumnModify:
		return e.modifyColumn(ctx, tx, op)
	case OpIndexAdd:
		return e.addIndex(ctx, tx, op)
	case OpIndexDrop:
		return e.dropIndex(ctx, tx, op)
	default:
		return fmt.Errorf("unsupported operation type: %s", op.Type)
	}
}

func (e *SQLiteExecutor) createTable(ctx context.Context, tx *sql.Tx, op DiffOperation) error {
	var columns []string
	for _, field := range op.Schema.Fields {
		colDef := e.buildColumnDef(field)
		columns = append(columns, colDef)
	}

	query := fmt.Sprintf("CREATE TABLE IF NOT EXISTS \"%s\" (\n  %s\n)",
		op.TableName,
		strings.Join(columns, ",\n  "))

	_, err := tx.ExecContext(ctx, query)
	return err
}

func (e *SQLiteExecutor) buildColumnDef(field FieldDefinition) string {
	parts := []string{
		fmt.Sprintf("\"%s\"", field.Name),
		e.mapper.MapColumnType(field.Type, field.Constraints),
	}

	if field.Constraints != nil {
		if !field.Constraints.Nullable {
			parts = append(parts, "NOT NULL")
		}
		if field.Constraints.Unique {
			parts = append(parts, "UNIQUE")
		}
		if field.Constraints.Default != nil {
			defVal := e.mapper.FormatDefaultValue(field.Constraints.Default, field.Type)
			parts = append(parts, fmt.Sprintf("DEFAULT %s", defVal))
		}
	}

	if field.Type == FieldTypeRelation && field.ForeignKey != nil {
		refCol := field.ForeignKey.Column
		if refCol == "" {
			refCol = "id"
		}
		fkClause := fmt.Sprintf("REFERENCES \"%s\"(\"%s\")", field.ForeignKey.Table, refCol)
		if field.ForeignKey.OnDelete != "" {
			fkClause += fmt.Sprintf(" ON DELETE %s", field.ForeignKey.OnDelete)
		}
		if field.ForeignKey.OnUpdate != "" {
			fkClause += fmt.Sprintf(" ON UPDATE %s", field.ForeignKey.OnUpdate)
		}
		parts = append(parts, fkClause)
	}

	return strings.Join(parts, " ")
}

func (e *SQLiteExecutor) addColumn(ctx context.Context, tx *sql.Tx, op DiffOperation) error {
	colDef := fmt.Sprintf("\"%s\" %s", op.ColumnName, op.ColumnType)

	if op.Constraints != nil {
		if !op.Constraints.Nullable {
			colDef += " NOT NULL"
		}
		if op.Constraints.Default != nil {
			defVal := e.mapper.FormatDefaultValue(op.Constraints.Default, FieldType(op.ColumnType))
			colDef += fmt.Sprintf(" DEFAULT %s", defVal)
		}
	}

	if FieldType(op.ColumnType) == FieldTypeRelation && op.ForeignKey != nil {
		refCol := op.ForeignKey.Column
		if refCol == "" {
			refCol = "id"
		}
		fkClause := fmt.Sprintf(" REFERENCES \"%s\"(\"%s\")", op.ForeignKey.Table, refCol)
		if op.ForeignKey.OnDelete != "" {
			fkClause += fmt.Sprintf(" ON DELETE %s", op.ForeignKey.OnDelete)
		}
		if op.ForeignKey.OnUpdate != "" {
			fkClause += fmt.Sprintf(" ON UPDATE %s", op.ForeignKey.OnUpdate)
		}
		colDef += fkClause
	}

	query := fmt.Sprintf("ALTER TABLE \"%s\" ADD COLUMN %s", op.TableName, colDef)
	_, err := tx.ExecContext(ctx, query)
	return err
}

func (e *SQLiteExecutor) dropColumn(ctx context.Context, tx *sql.Tx, op DiffOperation) error {
	_, err := tx.ExecContext(ctx,
		fmt.Sprintf("ALTER TABLE \"%s\" DROP COLUMN \"%s\"",
			op.TableName, op.ColumnName))
	return err
}

func (e *SQLiteExecutor) modifyColumn(ctx context.Context, tx *sql.Tx, op DiffOperation) error {
	return fmt.Errorf("column modification requires table rebuild")
}

func (e *SQLiteExecutor) addIndex(ctx context.Context, tx *sql.Tx, op DiffOperation) error {
	uniqueStr := ""
	if op.IndexUnique {
		uniqueStr = "UNIQUE "
	}

	cols := make([]string, len(op.IndexColumns))
	for i, c := range op.IndexColumns {
		cols[i] = fmt.Sprintf("\"%s\"", c)
	}

	query := fmt.Sprintf("CREATE %sINDEX IF NOT EXISTS \"%s\" ON \"%s\" (%s)",
		uniqueStr, op.IndexName, op.TableName, strings.Join(cols, ", "))

	_, err := tx.ExecContext(ctx, query)
	return err
}

func (e *SQLiteExecutor) dropIndex(ctx context.Context, tx *sql.Tx, op DiffOperation) error {
	_, err := tx.ExecContext(ctx, fmt.Sprintf("DROP INDEX IF EXISTS \"%s\"", op.IndexName))
	return err
}

func (e *SQLiteExecutor) rebuildTable(ctx context.Context, tx *sql.Tx, op DiffOperation) error {
	schema, err := e.getIntrospectedSchema(ctx, op.TableName)
	if err != nil {
		return fmt.Errorf("introspecting table: %w", err)
	}

	var desiredSchema *Schema
	if op.Schema != nil {
		desiredSchema = op.Schema
	} else {
		desiredSchema = e.buildSchemaFromIntrospection(schema)
		if op.Type == OpColumnDrop {
			desiredSchema = e.removeField(desiredSchema, op.ColumnName)
		} else if op.Type == OpColumnModify {
			desiredSchema = e.modifyField(desiredSchema, op.ColumnName, op.ColumnType, op.Constraints)
		}
	}

	tempTableName := fmt.Sprintf("_%s_new", op.TableName)

	var columns []string
	for _, field := range desiredSchema.Fields {
		colDef := e.buildColumnDef(field)
		columns = append(columns, colDef)
	}

	createQuery := fmt.Sprintf("CREATE TABLE \"%s\" (\n  %s\n)",
		tempTableName,
		strings.Join(columns, ",\n  "))

	if _, err := tx.ExecContext(ctx, createQuery); err != nil {
		return fmt.Errorf("creating temp table: %w", err)
	}

	var sourceCols []string
	var targetCols []string
	for _, field := range desiredSchema.Fields {
		sourceCols = append(sourceCols, fmt.Sprintf("\"%s\"", field.Name))
		targetCols = append(targetCols, fmt.Sprintf("\"%s\"", field.Name))
	}

	insertQuery := fmt.Sprintf("INSERT INTO \"%s\" (%s) SELECT %s FROM \"%s\"",
		tempTableName,
		strings.Join(targetCols, ", "),
		strings.Join(sourceCols, ", "),
		op.TableName)

	if _, err := tx.ExecContext(ctx, insertQuery); err != nil {
		return fmt.Errorf("copying data: %w", err)
	}

	if _, err := tx.ExecContext(ctx, fmt.Sprintf("DROP TABLE \"%s\"", op.TableName)); err != nil {
		return fmt.Errorf("dropping old table: %w", err)
	}

	if _, err := tx.ExecContext(ctx, fmt.Sprintf("ALTER TABLE \"%s\" RENAME TO \"%s\"", tempTableName, op.TableName)); err != nil {
		return fmt.Errorf("renaming table: %w", err)
	}

	for _, idx := range desiredSchema.Indexes {
		idxOp := DiffOperation{
			Type:         OpIndexAdd,
			TableName:    op.TableName,
			IndexName:    idx.Name,
			IndexColumns: idx.Columns,
			IndexUnique:  idx.Unique,
		}
		if err := e.addIndex(ctx, tx, idxOp); err != nil {
			return fmt.Errorf("recreating index %s: %w", idx.Name, err)
		}
	}

	return nil
}

func (e *SQLiteExecutor) getIntrospectedSchema(ctx context.Context, tableName string) (*interfaces.TableDefinition, error) {
	schema, err := e.db.SchemaIntrospect(ctx)
	if err != nil {
		return nil, err
	}

	for _, table := range schema.Tables {
		if table.Name == tableName {
			return &table, nil
		}
	}

	return nil, fmt.Errorf("table %s not found", tableName)
}

func (e *SQLiteExecutor) buildSchemaFromIntrospection(table *interfaces.TableDefinition) *Schema {
	schema := &Schema{
		Name:   table.Name,
		Fields: make([]FieldDefinition, 0, len(table.Columns)),
	}

	for _, col := range table.Columns {
		field := FieldDefinition{
			Name: col.Name,
			Type: e.inferFieldType(col.Type),
		}
		if col.Nullable {
			field.Constraints = &Constraints{Nullable: true}
		}
		schema.Fields = append(schema.Fields, field)
	}

	return schema
}

func (e *SQLiteExecutor) inferFieldType(sqlType string) FieldType {
	switch strings.ToUpper(sqlType) {
	case "INTEGER", "INT", "BIGINT", "SMALLINT", "TINYINT":
		return FieldTypeNumber
	case "REAL", "DOUBLE", "FLOAT", "NUMERIC", "DECIMAL":
		return FieldTypeDecimal
	case "TEXT", "VARCHAR", "CHAR", "CLOB":
		return FieldTypeText
	case "BOOLEAN", "BOOL":
		return FieldTypeBoolean
	case "JSON", "JSONB":
		return FieldTypeJSON
	default:
		return FieldTypeText
	}
}

func (e *SQLiteExecutor) removeField(schema *Schema, fieldName string) *Schema {
	newSchema, err := schema.Clone()
	if err != nil {
		// Clone failed; return original schema (operation will be skipped upstream)
		return schema
	}
	newFields := make([]FieldDefinition, 0, len(schema.Fields))
	for _, f := range newSchema.Fields {
		if f.Name != fieldName {
			newFields = append(newFields, f)
		}
	}
	newSchema.Fields = newFields
	return newSchema
}

func (e *SQLiteExecutor) modifyField(schema *Schema, fieldName, newType string, constraints *Constraints) *Schema {
	newSchema, err := schema.Clone()
	if err != nil {
		return schema
	}
	for i, f := range newSchema.Fields {
		if f.Name == fieldName {
			newSchema.Fields[i].Type = FieldType(newType)
			newSchema.Fields[i].Constraints = constraints
			break
		}
	}
	return newSchema
}
