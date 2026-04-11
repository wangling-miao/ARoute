package database

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/wangling-miao/aroute/sdk/interfaces"
)

type txCtxKey struct{}

type Service struct {
	db     *sql.DB
	driver Driver
}

func NewService(db *sql.DB, driver Driver) *Service {
	return &Service{
		db:     db,
		driver: driver,
	}
}

func (s *Service) Query(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	normalizedQuery := s.normalizePlaceholders(query)
	return s.db.QueryContext(ctx, normalizedQuery, args...)
}

func (s *Service) QueryRow(ctx context.Context, query string, args ...interface{}) *sql.Row {
	normalizedQuery := s.normalizePlaceholders(query)
	return s.db.QueryRowContext(ctx, normalizedQuery, args...)
}

func (s *Service) Exec(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	normalizedQuery := s.normalizePlaceholders(query)
	return s.db.ExecContext(ctx, normalizedQuery, args...)
}

func (s *Service) BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
	if ctx.Value(txCtxKey{}) != nil {
		return nil, fmt.Errorf("nested transactions are not supported; use savepoints within existing transaction")
	}
	return s.db.BeginTx(ctx, opts)
}

func (s *Service) Ping(ctx context.Context) error {
	_, hasDeadline := ctx.Deadline()
	if !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
	}
	return s.db.PingContext(ctx)
}

func (s *Service) Close() error {
	return s.db.Close()
}

func ContextWithTransaction(ctx context.Context) context.Context {
	return context.WithValue(ctx, txCtxKey{}, true)
}

func (s *Service) SchemaIntrospect(ctx context.Context) (*interfaces.DatabaseSchema, error) {
	switch s.driver {
	case DriverSQLite:
		return s.sqliteSchemaIntrospect(ctx)
	case DriverPostgreSQL:
		return s.pgSchemaIntrospect(ctx)
	default:
		return nil, fmt.Errorf("unsupported driver for schema introspection: %s", s.driver)
	}
}

func (s *Service) Prepare(ctx context.Context, query string) (*sql.Stmt, error) {
	normalizedQuery := s.normalizePlaceholders(query)
	return s.db.PrepareContext(ctx, normalizedQuery)
}

func (s *Service) sqliteSchemaIntrospect(ctx context.Context) (*interfaces.DatabaseSchema, error) {
	tableNames, err := s.sqliteListTables(ctx)
	if err != nil {
		return nil, err
	}

	schema := &interfaces.DatabaseSchema{Tables: []interfaces.TableDefinition{}}

	for _, tableName := range tableNames {
		if strings.HasPrefix(tableName, "_") || tableName == "sqlite_sequence" {
			continue
		}

		tableDef := interfaces.TableDefinition{Name: tableName}

		columns, err := s.sqliteListColumns(ctx, tableName)
		if err != nil {
			return nil, fmt.Errorf("failed to list columns for table %s: %w", tableName, err)
		}
		tableDef.Columns = columns

		indexes, err := s.sqliteListIndexes(ctx, tableName)
		if err != nil {
			return nil, fmt.Errorf("failed to list indexes for table %s: %w", tableName, err)
		}
		tableDef.Indexes = indexes

		schema.Tables = append(schema.Tables, tableDef)
	}

	return schema, nil
}

func (s *Service) sqliteListTables(ctx context.Context) ([]string, error) {
	rows, err := s.Query(ctx, "SELECT name FROM sqlite_master WHERE type='table' ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tables := []string{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		tables = append(tables, name)
	}
	return tables, rows.Err()
}

func (s *Service) sqliteListColumns(ctx context.Context, tableName string) ([]interfaces.ColumnDefinition, error) {
	rows, err := s.Query(ctx, fmt.Sprintf("PRAGMA table_info(%s)", tableName))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	columns := []interfaces.ColumnDefinition{}
	for rows.Next() {
		var cid int
		var name, typeStr string
		var notnull, pk int
		var dfltValue sql.NullString
		if err := rows.Scan(&cid, &name, &typeStr, &notnull, &dfltValue, &pk); err != nil {
			return nil, err
		}

		col := interfaces.ColumnDefinition{
			Name:          name,
			Type:          typeStr,
			Nullable:      notnull == 0,
			AutoIncrement: pk > 0 && strings.Contains(strings.ToUpper(typeStr), "INTEGER"),
		}

		if dfltValue.Valid {
			col.DefaultValue = dfltValue.String
		}

		columns = append(columns, col)
	}
	return columns, rows.Err()
}

func (s *Service) sqliteListIndexes(ctx context.Context, tableName string) ([]interfaces.IndexDefinition, error) {
	rows, err := s.Query(ctx, fmt.Sprintf("PRAGMA index_list(%s)", tableName))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	indexes := []interfaces.IndexDefinition{}
	for rows.Next() {
		var seq int
		var name, origin string
		var unique int
		var partial int
		if err := rows.Scan(&seq, &name, &unique, &origin, &partial); err != nil {
			return nil, err
		}

		if origin == "pk" {
			continue
		}

		indexDef := interfaces.IndexDefinition{
			Name:   name,
			Unique: unique == 1,
		}

		idxInfoRows, err := s.Query(ctx, fmt.Sprintf("PRAGMA index_info(%s)", name))
		if err != nil {
			return nil, err
		}

		cols := []string{}
		for idxInfoRows.Next() {
			var seqNo, cid int
			var colName string
			if err := idxInfoRows.Scan(&seqNo, &cid, &colName); err != nil {
				idxInfoRows.Close()
				return nil, err
			}
			cols = append(cols, colName)
		}
		idxInfoRows.Close()

		indexDef.Columns = cols
		indexes = append(indexes, indexDef)
	}
	return indexes, rows.Err()
}

func (s *Service) pgSchemaIntrospect(ctx context.Context) (*interfaces.DatabaseSchema, error) {
	tableNames, err := s.pgListTables(ctx)
	if err != nil {
		return nil, err
	}

	schema := &interfaces.DatabaseSchema{Tables: []interfaces.TableDefinition{}}

	for _, tableName := range tableNames {
		if strings.HasPrefix(tableName, "_") {
			continue
		}

		tableDef := interfaces.TableDefinition{Name: tableName}

		columns, err := s.pgListColumns(ctx, tableName)
		if err != nil {
			return nil, fmt.Errorf("failed to list columns for table %s: %w", tableName, err)
		}
		tableDef.Columns = columns

		indexes, err := s.pgListIndexes(ctx, tableName)
		if err != nil {
			return nil, fmt.Errorf("failed to list indexes for table %s: %w", tableName, err)
		}
		tableDef.Indexes = indexes

		schema.Tables = append(schema.Tables, tableDef)
	}

	return schema, nil
}

func (s *Service) pgListTables(ctx context.Context) ([]string, error) {
	rows, err := s.Query(ctx, `
		SELECT table_name 
		FROM information_schema.tables 
		WHERE table_schema = 'public' AND table_type = 'BASE TABLE'
		ORDER BY table_name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tables := []string{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		tables = append(tables, name)
	}
	return tables, rows.Err()
}

func (s *Service) pgListColumns(ctx context.Context, tableName string) ([]interfaces.ColumnDefinition, error) {
	rows, err := s.Query(ctx, `
		SELECT column_name, data_type, is_nullable, column_default
		FROM information_schema.columns
		WHERE table_schema = 'public' AND table_name = $1
		ORDER BY ordinal_position
	`, tableName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	columns := []interfaces.ColumnDefinition{}
	for rows.Next() {
		var name, dataType, isNullable string
		var dfltValue sql.NullString
		if err := rows.Scan(&name, &dataType, &isNullable, &dfltValue); err != nil {
			return nil, err
		}

		col := interfaces.ColumnDefinition{
			Name:     name,
			Type:     dataType,
			Nullable: isNullable == "YES",
		}

		if dfltValue.Valid {
			col.DefaultValue = dfltValue.String
		}

		columns = append(columns, col)
	}
	return columns, rows.Err()
}

func (s *Service) pgListIndexes(ctx context.Context, tableName string) ([]interfaces.IndexDefinition, error) {
	rows, err := s.Query(ctx, `
		SELECT indexname, indexdef
		FROM pg_indexes
		WHERE schemaname = 'public' AND tablename = $1
	`, tableName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	indexes := []interfaces.IndexDefinition{}
	for rows.Next() {
		var name, def string
		if err := rows.Scan(&name, &def); err != nil {
			return nil, err
		}

		unique := strings.Contains(def, "UNIQUE")
		indexDef := interfaces.IndexDefinition{
			Name:   name,
			Unique: unique,
		}

		indexes = append(indexes, indexDef)
	}
	return indexes, rows.Err()
}

func (s *Service) DB() *sql.DB {
	return s.db
}

func (s *Service) Driver() Driver {
	return s.driver
}

func MaskPassword(connStr string) string {
	if strings.Contains(connStr, "password=") || strings.Contains(connStr, "PASSWORD=") {
		parts := strings.Split(connStr, " ")
		for i, part := range parts {
			if strings.HasPrefix(strings.ToLower(part), "password=") {
				idx := strings.Index(part, "=")
				parts[i] = part[:idx+1] + "****"
			}
		}
		return strings.Join(parts, " ")
	}

	if strings.Contains(connStr, "@") && strings.Contains(connStr, "://") {
		re := regexp.MustCompile(`://([^:]+):([^@]+)@`)
		return re.ReplaceAllString(connStr, "://$1:****@")
	}

	return connStr
}

func (s *Service) normalizePlaceholders(query string) string {
	if s.driver != DriverPostgreSQL {
		return query
	}

	placeholderCount := 0
	result := make([]byte, 0, len(query)+10)

	for i := 0; i < len(query); i++ {
		if query[i] == '?' {
			placeholderCount++
			result = append(result, '$')
			result = append(result, []byte(fmt.Sprintf("%d", placeholderCount))...)
		} else {
			result = append(result, query[i])
		}
	}

	return string(result)
}
