package ddl

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/wangling-miao/aroute/sdk/interfaces"
)

type Introspector struct {
	db      interfaces.DatabaseService
	dialect Dialect
}

func NewIntrospector(db interfaces.DatabaseService, dialect Dialect) *Introspector {
	return &Introspector{
		db:      db,
		dialect: dialect,
	}
}

func (i *Introspector) IntrospectTable(ctx context.Context, tableName string) (*interfaces.TableDefinition, error) {
	switch i.dialect {
	case DialectSQLite:
		return i.introspectSQLiteTable(ctx, tableName)
	case DialectPostgreSQL:
		return i.introspectPostgreSQLTable(ctx, tableName)
	default:
		return i.introspectSQLiteTable(ctx, tableName)
	}
}

func (i *Introspector) introspectSQLiteTable(ctx context.Context, tableName string) (*interfaces.TableDefinition, error) {
	if err := sanitizeIdentifier(tableName, "table name"); err != nil {
		return nil, err
	}

	rows, err := i.db.Query(ctx, fmt.Sprintf("PRAGMA table_info(\"%s\")", tableName))
	if err != nil {
		return nil, fmt.Errorf("querying table info: %w", err)
	}
	defer rows.Close()

	table := &interfaces.TableDefinition{
		Name: tableName,
	}

	for rows.Next() {
		var cid int
		var name, colType string
		var notNull, pk int
		var defaultValue sql.NullString

		err := rows.Scan(&cid, &name, &colType, &notNull, &defaultValue, &pk)
		if err != nil {
			return nil, fmt.Errorf("scanning column info: %w", err)
		}

		col := interfaces.ColumnDefinition{
			Name:         name,
			Type:         strings.ToUpper(colType),
			Nullable:     notNull == 0,
			DefaultValue: defaultValue.String,
		}

		table.Columns = append(table.Columns, col)

		if pk > 0 {
			table.PrimaryKey = append(table.PrimaryKey, name)
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating columns: %w", err)
	}

	if len(table.Columns) == 0 {
		return nil, nil
	}

	type indexMeta struct {
		Name   string
		Unique bool
	}

	idxRows, err := i.db.Query(ctx, fmt.Sprintf("PRAGMA index_list(\"%s\")", tableName))
	if err != nil {
		return nil, fmt.Errorf("querying index list: %w", err)
	}

	var indexes []indexMeta
	for idxRows.Next() {
		var seq int
		var name, origin string
		var unique, partial int

		err := idxRows.Scan(&seq, &name, &unique, &origin, &partial)
		if err != nil {
			idxRows.Close()
			return nil, fmt.Errorf("scanning index info: %w", err)
		}

		if strings.HasPrefix(name, "sqlite_") {
			continue
		}

		indexes = append(indexes, indexMeta{
			Name:   name,
			Unique: unique == 1,
		})
	}
	idxRows.Close()

	for _, im := range indexes {
		idx := interfaces.IndexDefinition{
			Name:   im.Name,
			Unique: im.Unique,
		}

		if err := sanitizeIdentifier(im.Name, "index name"); err != nil {
			return nil, fmt.Errorf("invalid index name: %w", err)
		}

		colRows, err := i.db.Query(ctx, fmt.Sprintf("PRAGMA index_info(\"%s\")", im.Name))
		if err != nil {
			return nil, fmt.Errorf("querying index columns: %w", err)
		}

		for colRows.Next() {
			var seqNo, cid int
			var colName sql.NullString

			err := colRows.Scan(&seqNo, &cid, &colName)
			if err != nil {
				colRows.Close()
				return nil, fmt.Errorf("scanning index column: %w", err)
			}

			if colName.Valid {
				idx.Columns = append(idx.Columns, colName.String)
			}
		}
		colRows.Close()

		if len(idx.Columns) > 0 {
			table.Indexes = append(table.Indexes, idx)
		}
	}

	fkRows, err := i.db.Query(ctx, fmt.Sprintf("PRAGMA foreign_key_list(\"%s\")", tableName))
	if err == nil {
		defer fkRows.Close()
		for fkRows.Next() {
			var id, seq int
			var tableRef, fromCol, toCol string
			var onUpdate, onDelete, match string

			err := fkRows.Scan(&id, &seq, &tableRef, &fromCol, &toCol, &onUpdate, &onDelete, &match)
			if err != nil {
				continue
			}

			fk := interfaces.ForeignKeyDefinition{
				Name:       fmt.Sprintf("fk_%s_%s", tableName, fromCol),
				Columns:    []string{fromCol},
				RefTable:   tableRef,
				RefColumns: []string{toCol},
				OnDelete:   onDelete,
				OnUpdate:   onUpdate,
			}
			table.ForeignKeys = append(table.ForeignKeys, fk)
		}
	}

	return table, nil
}

func (i *Introspector) introspectPostgreSQLTable(ctx context.Context, tableName string) (*interfaces.TableDefinition, error) {
	rows, err := i.db.Query(ctx, `
		SELECT column_name, data_type, is_nullable, column_default,
			   character_maximum_length
		FROM information_schema.columns
		WHERE table_name = $1 AND table_schema = 'public'
		ORDER BY ordinal_position
	`, tableName)

	if err != nil {
		return nil, fmt.Errorf("querying table columns: %w", err)
	}
	defer rows.Close()

	table := &interfaces.TableDefinition{
		Name: tableName,
	}

	for rows.Next() {
		var name, dataType, isNullable string
		var defaultValue sql.NullString
		var maxLen sql.NullInt64

		err := rows.Scan(&name, &dataType, &isNullable, &defaultValue, &maxLen)
		if err != nil {
			return nil, fmt.Errorf("scanning column info: %w", err)
		}

		colType := strings.ToUpper(dataType)
		if maxLen.Valid && (colType == "CHARACTER VARYING" || colType == "VARCHAR") {
			colType = fmt.Sprintf("VARCHAR(%d)", maxLen.Int64)
		}

		col := interfaces.ColumnDefinition{
			Name:         name,
			Type:         colType,
			Nullable:     isNullable == "YES",
			DefaultValue: defaultValue.String,
		}

		table.Columns = append(table.Columns, col)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating columns: %w", err)
	}

	if len(table.Columns) == 0 {
		return nil, nil
	}

	idxRows, err := i.db.Query(ctx, `
		SELECT indexname, indexdef
		FROM pg_indexes
		WHERE tablename = $1 AND schemaname = 'public'
	`, tableName)

	if err != nil {
		return nil, fmt.Errorf("querying indexes: %w", err)
	}
	defer idxRows.Close()

	for idxRows.Next() {
		var name, def string
		err := idxRows.Scan(&name, &def)
		if err != nil {
			return nil, fmt.Errorf("scanning index info: %w", err)
		}

		idx := interfaces.IndexDefinition{
			Name:   name,
			Unique: strings.Contains(def, "UNIQUE"),
		}

		table.Indexes = append(table.Indexes, idx)
	}

	fkRows, err := i.db.Query(ctx, `
		SELECT
			tc.constraint_name,
			kcu.column_name,
			ccu.table_name AS foreign_table_name,
			ccu.column_name AS foreign_column_name,
			rc.delete_rule,
			rc.update_rule
		FROM information_schema.table_constraints AS tc
		JOIN information_schema.key_column_usage AS kcu
			ON tc.constraint_name = kcu.constraint_name
			AND tc.table_schema = kcu.table_schema
		JOIN information_schema.constraint_column_usage AS ccu
			ON ccu.constraint_name = tc.constraint_name
			AND ccu.table_schema = tc.table_schema
		JOIN information_schema.referential_constraints AS rc
			ON rc.constraint_name = tc.constraint_name
			AND rc.constraint_schema = tc.table_schema
		WHERE tc.constraint_type = 'FOREIGN KEY'
			AND tc.table_name = $1
			AND tc.table_schema = 'public'
	`, tableName)
	if err == nil {
		defer fkRows.Close()
		for fkRows.Next() {
			var name, columnName, refTable, refColumn, onDelete, onUpdate string
			err := fkRows.Scan(&name, &columnName, &refTable, &refColumn, &onDelete, &onUpdate)
			if err != nil {
				continue
			}

			fk := interfaces.ForeignKeyDefinition{
				Name:       name,
				Columns:    []string{columnName},
				RefTable:   refTable,
				RefColumns: []string{refColumn},
				OnDelete:   onDelete,
				OnUpdate:   onUpdate,
			}
			table.ForeignKeys = append(table.ForeignKeys, fk)
		}
	}

	return table, nil
}

func (i *Introspector) ListTables(ctx context.Context) ([]string, error) {
	var tables []string

	switch i.dialect {
	case DialectSQLite:
		rows, err := i.db.Query(ctx, `
			SELECT name FROM sqlite_master 
			WHERE type = 'table' AND name NOT LIKE 'sqlite_%' AND name NOT LIKE '\_%' ESCAPE '\'
			ORDER BY name
		`)
		if err != nil {
			return nil, fmt.Errorf("listing SQLite tables: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil {
				return nil, fmt.Errorf("scanning table name: %w", err)
			}
			tables = append(tables, name)
		}

	case DialectPostgreSQL:
		rows, err := i.db.Query(ctx, `
			SELECT table_name 
			FROM information_schema.tables 
			WHERE table_schema = 'public' AND table_type = 'BASE TABLE'
			AND table_name NOT LIKE '\_%' ESCAPE '\'
			ORDER BY table_name
		`)
		if err != nil {
			return nil, fmt.Errorf("listing PostgreSQL tables: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil {
				return nil, fmt.Errorf("scanning table name: %w", err)
			}
			tables = append(tables, name)
		}
	}

	return tables, nil
}
