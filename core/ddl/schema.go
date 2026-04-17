// Package ddl provides a dynamic DDL (Data Definition Language) engine for Aroute CMS.
// It supports schema definition, diff computation, and DDL execution for both SQLite and PostgreSQL.
package ddl

import (
	"encoding/json"
	"fmt"
)

// FieldType represents a supported field type in the CMS.
type FieldType string

const (
	FieldTypeText     FieldType = "text"
	FieldTypeNumber   FieldType = "number"
	FieldTypeDecimal  FieldType = "decimal"
	FieldTypeBoolean  FieldType = "boolean"
	FieldTypeDatetime FieldType = "datetime"
	FieldTypeJSON     FieldType = "json"
	FieldTypeRelation FieldType = "relation"
)

// IsValid checks if the field type is a supported type.
func (ft FieldType) IsValid() bool {
	switch ft {
	case FieldTypeText, FieldTypeNumber, FieldTypeDecimal, FieldTypeBoolean,
		FieldTypeDatetime, FieldTypeJSON, FieldTypeRelation:
		return true
	default:
		return false
	}
}

// String returns the string representation of the field type.
func (ft FieldType) String() string {
	return string(ft)
}

// Constraints represents field-level constraints.
type Constraints struct {
	// Nullable indicates whether the field can be NULL.
	Nullable bool `json:"nullable,omitempty"`

	// Unique indicates whether the field value must be unique.
	Unique bool `json:"unique,omitempty"`

	// Default is the default value for the field.
	Default interface{} `json:"default,omitempty"`

	// MaxLength is the maximum length for text fields.
	MaxLength *int `json:"max_length,omitempty"`

	// Min is the minimum value for number fields.
	Min *float64 `json:"min,omitempty"`

	// Max is the maximum value for number fields.
	Max *float64 `json:"max,omitempty"`

	// Pattern is a regex pattern for validation.
	Pattern string `json:"pattern,omitempty"`
}

// IndexDefinition represents a database index.
type IndexDefinition struct {
	// Name is the index name.
	Name string `json:"name"`

	// Columns is the list of column names in the index.
	Columns []string `json:"columns"`

	// Unique indicates whether the index has a uniqueness constraint.
	Unique bool `json:"unique"`
}

// ForeignKeyReference represents a foreign key reference for a relation field.
type ForeignKeyReference struct {
	// Table is the referenced table name.
	Table string `json:"table"`

	// Column is the referenced column name (defaults to "id" if not specified).
	Column string `json:"column,omitempty"`

	// OnDelete is the action on delete (e.g., "CASCADE", "SET NULL", "NO ACTION").
	OnDelete string `json:"on_delete,omitempty"`

	// OnUpdate is the action on update (e.g., "CASCADE", "SET NULL", "NO ACTION").
	OnUpdate string `json:"on_update,omitempty"`
}

// FieldDefinition represents a field in a Content Type schema.
type FieldDefinition struct {
	// Name is the field name (used as column name).
	Name string `json:"name"`

	// Type is the field type.
	Type FieldType `json:"type"`

	// Constraints are optional field constraints.
	Constraints *Constraints `json:"constraints,omitempty"`

	// Index indicates whether to create an index on this field.
	Index bool `json:"index,omitempty"`

	// ForeignKey specifies the foreign key reference for relation fields.
	// Only applicable when Type is FieldTypeRelation.
	ForeignKey *ForeignKeyReference `json:"foreign_key,omitempty"`
}

// Schema represents a Content Type schema definition.
type Schema struct {
	// Name is the content type name and table name.
	Name string `json:"name"`

	// Fields is the list of field definitions.
	Fields []FieldDefinition `json:"fields"`

	// Indexes is the list of index definitions.
	Indexes []IndexDefinition `json:"indexes,omitempty"`

	// TableName is the database table name (defaults to Name if not specified).
	TableName string `json:"table_name,omitempty"`
}

// GetTableName returns the table name for the schema.
func (s *Schema) GetTableName() string {
	if s.TableName != "" {
		return s.TableName
	}
	return s.Name
}

// Validate validates the schema definition.
func (s *Schema) Validate() error {
	if s.Name == "" {
		return fmt.Errorf("schema name is required")
	}

	if len(s.Fields) == 0 {
		return fmt.Errorf("schema must have at least one field")
	}

	// Check for duplicate field names
	fieldNames := make(map[string]bool)
	for _, field := range s.Fields {
		if field.Name == "" {
			return fmt.Errorf("field name is required")
		}

		if !field.Type.IsValid() {
			return fmt.Errorf("invalid field type '%s' for field '%s', supported types: text, number, decimal, boolean, datetime, json, relation",
				field.Type, field.Name)
		}

		if fieldNames[field.Name] {
			return fmt.Errorf("duplicate field name '%s'", field.Name)
		}
		fieldNames[field.Name] = true
	}

	// Validate index definitions
	for _, idx := range s.Indexes {
		if idx.Name == "" {
			return fmt.Errorf("index name is required")
		}

		if len(idx.Columns) == 0 {
			return fmt.Errorf("index '%s' must have at least one column", idx.Name)
		}

		// Verify columns exist
		for _, col := range idx.Columns {
			if !fieldNames[col] {
				return fmt.Errorf("index '%s' references non-existent column '%s'", idx.Name, col)
			}
		}
	}

	return nil
}

// ToJSON serializes the schema to JSON.
func (s *Schema) ToJSON() ([]byte, error) {
	return json.MarshalIndent(s, "", "  ")
}

// FromJSON deserializes a schema from JSON.
func FromJSON(data []byte) (*Schema, error) {
	var schema Schema
	if err := json.Unmarshal(data, &schema); err != nil {
		return nil, fmt.Errorf("parsing schema JSON: %w", err)
	}

	if err := schema.Validate(); err != nil {
		return nil, fmt.Errorf("validating schema: %w", err)
	}

	return &schema, nil
}

// GetField returns a field by name, or nil if not found.
func (s *Schema) GetField(name string) *FieldDefinition {
	for i := range s.Fields {
		if s.Fields[i].Name == name {
			return &s.Fields[i]
		}
	}
	return nil
}

// HasField checks if a field exists.
func (s *Schema) HasField(name string) bool {
	return s.GetField(name) != nil
}

// GetIndex returns an index by name, or nil if not found.
func (s *Schema) GetIndex(name string) *IndexDefinition {
	for i := range s.Indexes {
		if s.Indexes[i].Name == name {
			return &s.Indexes[i]
		}
	}
	return nil
}

// Clone creates a deep copy of the schema.
func (s *Schema) Clone() (*Schema, error) {
	data, err := json.Marshal(s)
	if err != nil {
		return nil, fmt.Errorf("marshal schema for clone: %w", err)
	}

	var clone Schema
	if err := json.Unmarshal(data, &clone); err != nil {
		return nil, fmt.Errorf("unmarshal schema for clone: %w", err)
	}

	return &clone, nil
}
