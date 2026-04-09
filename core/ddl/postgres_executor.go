package ddl

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/wangling-miao/aroute/sdk/interfaces"
)

type PostgreSQLExecutor struct {
	db     interfaces.DatabaseService
	mapper *TypeMapper
}

func NewPostgreSQLExecutor(db interfaces.DatabaseService) *PostgreSQLExecutor {
	return &PostgreSQLExecutor{
		db:     db,
		mapper: NewTypeMapper(DialectPostgreSQL),
	}
}

func (e *PostgreSQLExecutor) Execute(ctx context.Context, ops []DiffOperation, force bool) error {
	for _, op := range ops {
		if op.IsDestructive() && !force {
			return fmt.Errorf("destructive operation '%s' on '%s.%s' requires explicit confirmation (force=true)",
				op.Type, op.TableName, op.ColumnName)
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

func (e *PostgreSQLExecutor) executeOp(ctx context.Context, tx *sql.Tx, op DiffOperation) error {
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

func (e *PostgreSQLExecutor) createTable(ctx context.Context, tx *sql.Tx, op DiffOperation) error {
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

func (e *PostgreSQLExecutor) buildColumnDef(field FieldDefinition) string {
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

	return strings.Join(parts, " ")
}

func (e *PostgreSQLExecutor) addColumn(ctx context.Context, tx *sql.Tx, op DiffOperation) error {
	colDef := fmt.Sprintf("\"%s\" %s", op.ColumnName,
		e.mapper.MapColumnType(FieldType(op.ColumnType), op.Constraints))

	if op.Constraints != nil {
		if !op.Constraints.Nullable {
			colDef += " NOT NULL"
		}
		if op.Constraints.Default != nil {
			defVal := e.mapper.FormatDefaultValue(op.Constraints.Default, FieldType(op.ColumnType))
			colDef += fmt.Sprintf(" DEFAULT %s", defVal)
		}
	}

	query := fmt.Sprintf("ALTER TABLE \"%s\" ADD COLUMN %s", op.TableName, colDef)
	_, err := tx.ExecContext(ctx, query)
	return err
}

func (e *PostgreSQLExecutor) dropColumn(ctx context.Context, tx *sql.Tx, op DiffOperation) error {
	query := fmt.Sprintf("ALTER TABLE \"%s\" DROP COLUMN \"%s\"",
		op.TableName, op.ColumnName)
	_, err := tx.ExecContext(ctx, query)
	return err
}

func (e *PostgreSQLExecutor) modifyColumn(ctx context.Context, tx *sql.Tx, op DiffOperation) error {
	newType := e.mapper.MapColumnType(FieldType(op.ColumnType), op.Constraints)

	query := fmt.Sprintf("ALTER TABLE \"%s\" ALTER COLUMN \"%s\" TYPE %s USING \"%s\"::%s",
		op.TableName, op.ColumnName, newType, op.ColumnName, newType)

	if _, err := tx.ExecContext(ctx, query); err != nil {
		return err
	}

	if op.Constraints != nil {
		if op.Constraints.Nullable {
			_, err := tx.ExecContext(ctx,
				"ALTER TABLE \"%s\" ALTER COLUMN \"%s\" DROP NOT NULL",
				op.TableName, op.ColumnName)
			if err != nil {
				return err
			}
		} else {
			_, err := tx.ExecContext(ctx,
				"ALTER TABLE \"%s\" ALTER COLUMN \"%s\" SET NOT NULL",
				op.TableName, op.ColumnName)
			if err != nil {
				return err
			}
		}

		if op.Constraints.Default != nil {
			defVal := e.mapper.FormatDefaultValue(op.Constraints.Default, FieldType(op.ColumnType))
			_, err := tx.ExecContext(ctx,
				"ALTER TABLE \"%s\" ALTER COLUMN \"%s\" SET DEFAULT %s",
				op.TableName, op.ColumnName, defVal)
			if err != nil {
				return err
			}
		} else {
			_, err := tx.ExecContext(ctx,
				"ALTER TABLE \"%s\" ALTER COLUMN \"%s\" DROP DEFAULT",
				op.TableName, op.ColumnName)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (e *PostgreSQLExecutor) addIndex(ctx context.Context, tx *sql.Tx, op DiffOperation) error {
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

func (e *PostgreSQLExecutor) dropIndex(ctx context.Context, tx *sql.Tx, op DiffOperation) error {
	_, err := tx.ExecContext(ctx, "DROP INDEX IF EXISTS \"%s\"", op.IndexName)
	return err
}
