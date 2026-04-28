package ddl

import (
	"fmt"
	"strings"
)

type Dialect string

const (
	DialectSQLite     Dialect = "sqlite"
	DialectPostgreSQL Dialect = "postgres"
)

type TypeMapper struct {
	dialect Dialect
}

func NewTypeMapper(dialect Dialect) *TypeMapper {
	return &TypeMapper{dialect: dialect}
}

func (m *TypeMapper) MapColumnType(fieldType FieldType, constraints *Constraints) string {
	switch m.dialect {
	case DialectSQLite:
		return m.mapSQLiteType(fieldType, constraints)
	case DialectPostgreSQL:
		return m.mapPostgreSQLType(fieldType, constraints)
	default:
		return m.mapSQLiteType(fieldType, constraints)
	}
}

func (m *TypeMapper) mapSQLiteType(fieldType FieldType, constraints *Constraints) string {
	switch fieldType {
	case FieldTypeText:
		return "TEXT"
	case FieldTypeNumber:
		return "INTEGER"
	case FieldTypeDecimal:
		return "REAL"
	case FieldTypeBoolean:
		return "INTEGER"
	case FieldTypeDatetime:
		return "TEXT"
	case FieldTypeJSON:
		return "TEXT"
	case FieldTypeRelation:
		return "TEXT"
	default:
		return "TEXT"
	}
}

func (m *TypeMapper) mapPostgreSQLType(fieldType FieldType, constraints *Constraints) string {
	switch fieldType {
	case FieldTypeText:
		if constraints != nil && constraints.MaxLength != nil {
			return fmt.Sprintf("VARCHAR(%d)", *constraints.MaxLength)
		}
		return "TEXT"
	case FieldTypeNumber:
		return "BIGINT"
	case FieldTypeDecimal:
		return "NUMERIC"
	case FieldTypeBoolean:
		return "BOOLEAN"
	case FieldTypeDatetime:
		return "TIMESTAMP WITH TIME ZONE"
	case FieldTypeJSON:
		return "JSONB"
	case FieldTypeRelation:
		return "TEXT"
	default:
		return "TEXT"
	}
}

func (m *TypeMapper) FormatDefaultValue(value interface{}, fieldType FieldType) string {
	if value == nil {
		return ""
	}

	switch v := value.(type) {
	case string:
		return "'" + strings.ReplaceAll(v, "'", "''") + "'"
	case int, int64:
		return fmt.Sprintf("%d", v)
	case float64:
		return fmt.Sprintf("%f", v)
	case bool:
		if m.dialect == DialectSQLite {
			if v {
				return "1"
			}
			return "0"
		}
		if v {
			return "TRUE"
		}
		return "FALSE"
	default:
		return "'" + strings.ReplaceAll(fmt.Sprintf("%v", v), "'", "''") + "'"
	}
}

func (m *TypeMapper) NeedsRebuild(op DiffOperation) bool {
	if m.dialect != DialectSQLite {
		return false
	}
	return op.Type == OpColumnDrop || op.Type == OpColumnModify
}
