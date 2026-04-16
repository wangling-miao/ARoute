package ddl

import (
	"fmt"
	"regexp"
	"strings"
)

// DiffOperationType represents the type of schema diff operation.
type DiffOperationType string

const (
	// OpTableCreate indicates a new table needs to be created.
	OpTableCreate DiffOperationType = "table_create"

	// OpTableDrop indicates a table needs to be dropped.
	OpTableDrop DiffOperationType = "table_drop"

	// OpColumnAdd indicates a new column needs to be added.
	OpColumnAdd DiffOperationType = "column_add"

	// OpColumnDrop indicates a column needs to be dropped.
	OpColumnDrop DiffOperationType = "column_drop"

	// OpColumnModify indicates a column type or constraint has changed.
	OpColumnModify DiffOperationType = "column_modify"

	// OpIndexAdd indicates a new index needs to be created.
	OpIndexAdd DiffOperationType = "index_add"

	// OpIndexDrop indicates an index needs to be dropped.
	OpIndexDrop DiffOperationType = "index_drop"
)

// DiffOperation represents a single schema diff operation.
type DiffOperation struct {
	// Type is the operation type.
	Type DiffOperationType `json:"type"`

	// TableName is the affected table name.
	TableName string `json:"table_name"`

	// ColumnName is the affected column name (for column operations).
	ColumnName string `json:"column_name,omitempty"`

	// ColumnType is the column type (for column add/modify).
	ColumnType string `json:"column_type,omitempty"`

	// OldColumnType is the previous column type (for column modify).
	OldColumnType string `json:"old_column_type,omitempty"`

	// Constraints are the column constraints (for column add/modify).
	Constraints *Constraints `json:"constraints,omitempty"`

	// OldConstraints are the previous constraints (for column modify).
	OldConstraints *Constraints `json:"old_constraints,omitempty"`

	// ForeignKey is the foreign key reference (for relation column add/modify).
	ForeignKey *ForeignKeyReference `json:"foreign_key,omitempty"`

	// IndexName is the index name (for index operations).
	IndexName string `json:"index_name,omitempty"`

	// IndexColumns are the index columns (for index add).
	IndexColumns []string `json:"index_columns,omitempty"`

	// IndexUnique indicates if the index is unique (for index add).
	IndexUnique bool `json:"index_unique,omitempty"`

	// Schema is the full schema definition (for table_create).
	Schema *Schema `json:"schema,omitempty"`
}

// String returns a human-readable description of the operation.
func (op *DiffOperation) String() string {
	switch op.Type {
	case OpTableCreate:
		return fmt.Sprintf("create table %s", op.TableName)
	case OpTableDrop:
		return fmt.Sprintf("drop table %s", op.TableName)
	case OpColumnAdd:
		return fmt.Sprintf("add column %s.%s (%s)", op.TableName, op.ColumnName, op.ColumnType)
	case OpColumnDrop:
		return fmt.Sprintf("drop column %s.%s", op.TableName, op.ColumnName)
	case OpColumnModify:
		return fmt.Sprintf("modify column %s.%s from %s to %s", op.TableName, op.ColumnName, op.OldColumnType, op.ColumnType)
	case OpIndexAdd:
		uniqueStr := ""
		if op.IndexUnique {
			uniqueStr = "unique "
		}
		return fmt.Sprintf("add %sindex %s on %s (%v)", uniqueStr, op.IndexName, op.TableName, op.IndexColumns)
	case OpIndexDrop:
		return fmt.Sprintf("drop index %s on %s", op.IndexName, op.TableName)
	default:
		return fmt.Sprintf("unknown operation: %s", op.Type)
	}
}

// IsDestructive returns true if the operation can cause data loss.
func (op *DiffOperation) IsDestructive() bool {
	return op.Type == OpColumnDrop || op.Type == OpTableDrop
}

// DiffResult represents the result of a schema diff computation.
type DiffResult struct {
	// Operations is the list of diff operations needed to sync schema.
	Operations []DiffOperation `json:"operations"`

	// HasDestructiveOps indicates whether any operations are destructive.
	HasDestructiveOps bool `json:"has_destructive_ops"`
}

// IsEmpty returns true if there are no operations.
func (d *DiffResult) IsEmpty() bool {
	return len(d.Operations) == 0
}

// AddOperation adds an operation to the diff result.
func (d *DiffResult) AddOperation(op DiffOperation) {
	d.Operations = append(d.Operations, op)
	if op.IsDestructive() {
		d.HasDestructiveOps = true
	}
}

// GetDestructiveOperations returns all destructive operations.
func (d *DiffResult) GetDestructiveOperations() []DiffOperation {
	var destructive []DiffOperation
	for _, op := range d.Operations {
		if op.IsDestructive() {
			destructive = append(destructive, op)
		}
	}
	return destructive
}

// NewDiffResult creates a new empty diff result.
func NewDiffResult() *DiffResult {
	return &DiffResult{
		Operations: make([]DiffOperation, 0),
	}
}

var identifierRegex = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

var validCascadeActions = map[string]bool{
	"CASCADE": true, "SET NULL": true, "SET DEFAULT": true,
	"RESTRICT": true, "NO ACTION": true,
}

func sanitizeIdentifier(name, kind string) error {
	if name == "" {
		return fmt.Errorf("%s must not be empty", kind)
	}
	if !identifierRegex.MatchString(name) {
		return fmt.Errorf("invalid %s %q: must match [a-zA-Z_][a-zA-Z0-9_]*", kind, name)
	}
	if len(name) > 63 {
		return fmt.Errorf("%s %q exceeds maximum length of 63", kind, name)
	}
	return nil
}

func sanitizeCascadeAction(action string) (string, error) {
	upper := strings.ToUpper(strings.TrimSpace(action))
	if !validCascadeActions[upper] {
		return "", fmt.Errorf("invalid cascade action %q", action)
	}
	return upper, nil
}

func validateOperation(op DiffOperation) error {
	if err := sanitizeIdentifier(op.TableName, "table name"); err != nil {
		return err
	}
	if op.ColumnName != "" {
		if err := sanitizeIdentifier(op.ColumnName, "column name"); err != nil {
			return err
		}
	}
	if op.IndexName != "" {
		if err := sanitizeIdentifier(op.IndexName, "index name"); err != nil {
			return err
		}
	}
	for _, col := range op.IndexColumns {
		if err := sanitizeIdentifier(col, "index column"); err != nil {
			return err
		}
	}
	if op.ForeignKey != nil {
		if err := sanitizeIdentifier(op.ForeignKey.Table, "foreign key table"); err != nil {
			return err
		}
		if op.ForeignKey.Column != "" {
			if err := sanitizeIdentifier(op.ForeignKey.Column, "foreign key column"); err != nil {
				return err
			}
		}
		if op.ForeignKey.OnDelete != "" {
			if _, err := sanitizeCascadeAction(op.ForeignKey.OnDelete); err != nil {
				return err
			}
		}
		if op.ForeignKey.OnUpdate != "" {
			if _, err := sanitizeCascadeAction(op.ForeignKey.OnUpdate); err != nil {
				return err
			}
		}
	}
	if op.Schema != nil {
		for _, field := range op.Schema.Fields {
			if err := sanitizeIdentifier(field.Name, "field name"); err != nil {
				return err
			}
		}
	}
	return nil
}
