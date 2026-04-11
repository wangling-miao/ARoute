// Package interfaces defines shared interfaces for Aroute CMS plugins.
// These interfaces define the contracts that plugins must implement and can depend on.
package interfaces

import (
	"context"
	"database/sql"
)

// DatabaseService defines the interface for database operations.
// Plugins can obtain this service through the ServiceContainer to perform
// both static and dynamic database queries.
//
// This interface wraps the standard database/sql package with additional
// Aroute-specific functionality like schema introspection.
type DatabaseService interface {
	// Query executes a query that returns rows (SELECT).
	// The args are for any placeholder parameters in the query.
	Query(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error)

	// QueryRow executes a query that returns at most one row.
	// The args are for any placeholder parameters in the query.
	QueryRow(ctx context.Context, query string, args ...interface{}) *sql.Row

	// Exec executes a query that doesn't return rows (INSERT, UPDATE, DELETE).
	// The args are for any placeholder parameters in the query.
	Exec(ctx context.Context, query string, args ...interface{}) (sql.Result, error)

	// BeginTx starts a transaction with the specified context and options.
	// The returned Tx can be used for transactional operations.
	BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error)

	// Ping verifies the connection to the database is still alive.
	Ping(ctx context.Context) error

	// Close closes the database connection and releases resources.
	// It should be called when the service is no longer needed.
	Close() error

	// SchemaIntrospect returns schema information for introspection by the DDL Engine.
	// It lists all tables and their column definitions in the database.
	SchemaIntrospect(ctx context.Context) (*DatabaseSchema, error)

	// Prepare creates a prepared statement for later queries or executions.
	// The returned statement can be used multiple times with different arguments.
	// Multiple queries or executions can be run concurrently from the returned statement.
	// The caller must call the statement's Close method when the statement is no longer needed.
	Prepare(ctx context.Context, query string) (*sql.Stmt, error)
}

// DatabaseSchema represents the database schema structure.
type DatabaseSchema struct {
	// Tables is a list of table definitions in the database.
	Tables []TableDefinition `json:"tables"`
}

// TableDefinition represents a database table structure.
type TableDefinition struct {
	// Name is the table name.
	Name string `json:"name"`

	// Columns is a list of column definitions in the table.
	Columns []ColumnDefinition `json:"columns"`

	// Indexes is a list of index definitions on the table.
	Indexes []IndexDefinition `json:"indexes,omitempty"`

	// PrimaryKey is the name of the primary key column(s).
	PrimaryKey []string `json:"primary_key,omitempty"`

	// ForeignKeys is a list of foreign key constraints.
	ForeignKeys []ForeignKeyDefinition `json:"foreign_keys,omitempty"`
}

// ColumnDefinition represents a database column structure.
type ColumnDefinition struct {
	// Name is the column name.
	Name string `json:"name"`

	// Type is the column data type (e.g., "INTEGER", "TEXT", "VARCHAR(255)").
	Type string `json:"type"`

	// Nullable indicates whether the column can contain NULL values.
	Nullable bool `json:"nullable"`

	// DefaultValue is the default value for the column (may be nil).
	DefaultValue interface{} `json:"default_value,omitempty"`

	// AutoIncrement indicates whether the column is auto-incrementing.
	AutoIncrement bool `json:"auto_increment,omitempty"`

	// Comment is an optional comment for the column.
	Comment string `json:"comment,omitempty"`
}

// IndexDefinition represents a database index structure.
type IndexDefinition struct {
	// Name is the index name.
	Name string `json:"name"`

	// Columns is the list of column names in the index.
	Columns []string `json:"columns"`

	// Unique indicates whether the index has a uniqueness constraint.
	Unique bool `json:"unique"`

	// Type is the index type (e.g., "btree", "hash").
	Type string `json:"type,omitempty"`
}

// ForeignKeyDefinition represents a foreign key constraint.
type ForeignKeyDefinition struct {
	// Name is the foreign key constraint name.
	Name string `json:"name"`

	// Columns is the list of column names in the foreign key.
	Columns []string `json:"columns"`

	// RefTable is the referenced table name.
	RefTable string `json:"ref_table"`

	// RefColumns is the list of referenced column names.
	RefColumns []string `json:"ref_columns"`

	// OnDelete is the action on delete (e.g., "CASCADE", "SET NULL").
	OnDelete string `json:"on_delete,omitempty"`

	// OnUpdate is the action on update (e.g., "CASCADE", "SET NULL").
	OnUpdate string `json:"on_update,omitempty"`
}
