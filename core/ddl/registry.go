package ddl

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/wangling-miao/aroute/sdk/interfaces"
)

// Registry manages Content Type schema definitions.
// It provides CRUD operations and persists schemas to the database.
type Registry struct {
	db interfaces.DatabaseService
}

// NewRegistry creates a new schema registry.
func NewRegistry(db interfaces.DatabaseService) *Registry {
	return &Registry{db: db}
}

// Init initializes the registry by creating the required tables.
func (r *Registry) Init(ctx context.Context) error {
	_, err := r.db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS _content_types (
			name VARCHAR(255) PRIMARY KEY,
			schema_json TEXT NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return fmt.Errorf("creating _content_types table: %w", err)
	}

	return nil
}

// Create stores a new Content Type schema and executes DDL.
func (r *Registry) Create(ctx context.Context, schema *Schema) error {
	if err := schema.Validate(); err != nil {
		return fmt.Errorf("validating schema: %w", err)
	}

	existing, err := r.Get(ctx, schema.Name)
	if err == nil && existing != nil {
		return fmt.Errorf("content type '%s' already exists", schema.Name)
	}

	schemaJSON, err := json.Marshal(schema)
	if err != nil {
		return fmt.Errorf("serializing schema: %w", err)
	}

	_, err = r.db.Exec(ctx,
		"INSERT INTO _content_types (name, schema_json, created_at, updated_at) VALUES (?, ?, ?, ?)",
		schema.Name, string(schemaJSON), time.Now(), time.Now(),
	)
	if err != nil {
		return fmt.Errorf("inserting schema into registry: %w", err)
	}

	return nil
}

// Get retrieves a Content Type schema by name.
func (r *Registry) Get(ctx context.Context, name string) (*Schema, error) {
	var schemaJSON string
	var createdAt, updatedAt time.Time

	err := r.db.QueryRow(ctx,
		"SELECT schema_json, created_at, updated_at FROM _content_types WHERE name = ?",
		name,
	).Scan(&schemaJSON, &createdAt, &updatedAt)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("content type '%s' not found", name)
		}
		return nil, fmt.Errorf("querying schema: %w", err)
	}

	var schema Schema
	if err := json.Unmarshal([]byte(schemaJSON), &schema); err != nil {
		return nil, fmt.Errorf("deserializing schema: %w", err)
	}

	return &schema, nil
}

// Update updates an existing Content Type schema.
func (r *Registry) Update(ctx context.Context, schema *Schema) error {
	if err := schema.Validate(); err != nil {
		return fmt.Errorf("validating schema: %w", err)
	}

	existing, err := r.Get(ctx, schema.Name)
	if err != nil {
		return err
	}
	if existing == nil {
		return fmt.Errorf("content type '%s' not found", schema.Name)
	}

	schemaJSON, err := json.Marshal(schema)
	if err != nil {
		return fmt.Errorf("serializing schema: %w", err)
	}

	_, err = r.db.Exec(ctx,
		"UPDATE _content_types SET schema_json = ?, updated_at = ? WHERE name = ?",
		string(schemaJSON), time.Now(), schema.Name,
	)
	if err != nil {
		return fmt.Errorf("updating schema in registry: %w", err)
	}

	return nil
}

// Delete removes a Content Type schema from the registry.
// The force parameter is required to confirm destructive operations.
func (r *Registry) Delete(ctx context.Context, name string, force bool) error {
	existing, err := r.Get(ctx, name)
	if err != nil {
		return err
	}
	if existing == nil {
		return fmt.Errorf("content type '%s' not found", name)
	}

	if !force {
		return fmt.Errorf("destructive operation 'table_drop' on '%s' requires explicit confirmation (force=true)", name)
	}

	_, err = r.db.Exec(ctx, "DELETE FROM _content_types WHERE name = ?", name)
	if err != nil {
		return fmt.Errorf("deleting schema from registry: %w", err)
	}

	return nil
}

// List returns all registered Content Type schemas.
func (r *Registry) List(ctx context.Context) ([]*Schema, error) {
	rows, err := r.db.Query(ctx, "SELECT schema_json FROM _content_types ORDER BY name")
	if err != nil {
		return nil, fmt.Errorf("querying schemas: %w", err)
	}
	defer rows.Close()

	var schemas []*Schema
	for rows.Next() {
		var schemaJSON string
		if err := rows.Scan(&schemaJSON); err != nil {
			return nil, fmt.Errorf("scanning schema: %w", err)
		}

		var schema Schema
		if err := json.Unmarshal([]byte(schemaJSON), &schema); err != nil {
			return nil, fmt.Errorf("deserializing schema: %w", err)
		}

		schemas = append(schemas, &schema)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating schemas: %w", err)
	}

	return schemas, nil
}

// Exists checks if a Content Type schema exists.
func (r *Registry) Exists(ctx context.Context, name string) (bool, error) {
	var count int
	err := r.db.QueryRow(ctx, "SELECT COUNT(*) FROM _content_types WHERE name = ?", name).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("checking schema existence: %w", err)
	}
	return count > 0, nil
}
