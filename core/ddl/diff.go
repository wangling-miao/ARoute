package ddl

import (
	"fmt"
	"strings"

	"github.com/wangling-miao/aroute/sdk/interfaces"
)

type DiffEngine struct{}

func NewDiffEngine() *DiffEngine {
	return &DiffEngine{}
}

func (e *DiffEngine) Diff(desired *Schema, actual *interfaces.TableDefinition) (*DiffResult, error) {
	result := NewDiffResult()

	if actual == nil {
		result.AddOperation(DiffOperation{
			Type:      OpTableCreate,
			TableName: desired.GetTableName(),
			Schema:    desired,
		})
		return result, nil
	}

	e.diffColumns(desired, actual, result)
	e.diffIndexes(desired, actual, result)

	return result, nil
}

func (e *DiffEngine) diffColumns(desired *Schema, actual *interfaces.TableDefinition, result *DiffResult) {
	desiredFields := make(map[string]FieldDefinition)
	for _, f := range desired.Fields {
		desiredFields[f.Name] = f
	}

	actualColumns := make(map[string]interfaces.ColumnDefinition)
	for _, c := range actual.Columns {
		actualColumns[c.Name] = c
	}

	for _, field := range desired.Fields {
		actualCol, exists := actualColumns[field.Name]
		if !exists {
			result.AddOperation(DiffOperation{
				Type:        OpColumnAdd,
				TableName:   desired.GetTableName(),
				ColumnName:  field.Name,
				ColumnType:  string(field.Type),
				Constraints: field.Constraints,
			})
			continue
		}

		if e.columnNeedsModification(field, actualCol) {
			result.AddOperation(DiffOperation{
				Type:          OpColumnModify,
				TableName:     desired.GetTableName(),
				ColumnName:    field.Name,
				ColumnType:    string(field.Type),
				OldColumnType: actualCol.Type,
				Constraints:   field.Constraints,
			})
		}
	}

	for colName := range actualColumns {
		if _, exists := desiredFields[colName]; !exists {
			result.AddOperation(DiffOperation{
				Type:       OpColumnDrop,
				TableName:  desired.GetTableName(),
				ColumnName: colName,
			})
		}
	}
}

func (e *DiffEngine) columnNeedsModification(field FieldDefinition, actual interfaces.ColumnDefinition) bool {
	return !strings.EqualFold(string(field.Type), actual.Type)
}

func (e *DiffEngine) diffIndexes(desired *Schema, actual *interfaces.TableDefinition, result *DiffResult) {
	desiredIndexes := make(map[string]IndexDefinition)
	for _, idx := range desired.Indexes {
		desiredIndexes[idx.Name] = idx
	}

	for _, field := range desired.Fields {
		if field.Index {
			idxName := fmt.Sprintf("idx_%s_%s", desired.Name, field.Name)
			if _, exists := desiredIndexes[idxName]; !exists {
				desiredIndexes[idxName] = IndexDefinition{
					Name:    idxName,
					Columns: []string{field.Name},
					Unique:  field.Constraints != nil && field.Constraints.Unique,
				}
			}
		}
	}

	actualIndexes := make(map[string]interfaces.IndexDefinition)
	for _, idx := range actual.Indexes {
		actualIndexes[idx.Name] = idx
	}

	for _, idx := range desiredIndexes {
		if _, exists := actualIndexes[idx.Name]; !exists {
			result.AddOperation(DiffOperation{
				Type:         OpIndexAdd,
				TableName:    desired.GetTableName(),
				IndexName:    idx.Name,
				IndexColumns: idx.Columns,
				IndexUnique:  idx.Unique,
			})
		}
	}

	for idxName := range actualIndexes {
		if _, exists := desiredIndexes[idxName]; !exists {
			result.AddOperation(DiffOperation{
				Type:      OpIndexDrop,
				TableName: desired.GetTableName(),
				IndexName: idxName,
			})
		}
	}
}
