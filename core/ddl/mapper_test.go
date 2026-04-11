package ddl

import "testing"

func TestTypeMapper_MapColumnType_SQLite_AllTypes(t *testing.T) {
	tests := []struct {
		name        string
		fieldType   FieldType
		constraints *Constraints
		expected    string
	}{
		{
			name:        "text maps to TEXT",
			fieldType:   FieldTypeText,
			constraints: nil,
			expected:    "TEXT",
		},
		{
			name:        "number maps to INTEGER",
			fieldType:   FieldTypeNumber,
			constraints: nil,
			expected:    "INTEGER",
		},
		{
			name:        "decimal maps to REAL",
			fieldType:   FieldTypeDecimal,
			constraints: nil,
			expected:    "REAL",
		},
		{
			name:        "boolean maps to INTEGER",
			fieldType:   FieldTypeBoolean,
			constraints: nil,
			expected:    "INTEGER",
		},
		{
			name:        "datetime maps to TEXT",
			fieldType:   FieldTypeDatetime,
			constraints: nil,
			expected:    "TEXT",
		},
		{
			name:        "json maps to TEXT",
			fieldType:   FieldTypeJSON,
			constraints: nil,
			expected:    "TEXT",
		},
		{
			name:        "relation maps to INTEGER",
			fieldType:   FieldTypeRelation,
			constraints: nil,
			expected:    "INTEGER",
		},
		{
			name:        "unknown maps to TEXT",
			fieldType:   FieldType("unknown"),
			constraints: nil,
			expected:    "TEXT",
		},
		{
			name:        "text with MaxLength still maps to TEXT",
			fieldType:   FieldTypeText,
			constraints: &Constraints{MaxLength: new(255)},
			expected:    "TEXT",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mapper := NewTypeMapper(DialectSQLite)
			if got := mapper.MapColumnType(tt.fieldType, tt.constraints); got != tt.expected {
				t.Errorf("MapColumnType(%q, %v) = %q, want %q", tt.fieldType, tt.constraints, got, tt.expected)
			}
		})
	}
}

func TestTypeMapper_MapColumnType_PostgreSQL_AllTypes(t *testing.T) {
	tests := []struct {
		name        string
		fieldType   FieldType
		constraints *Constraints
		expected    string
	}{
		{
			name:        "text maps to TEXT",
			fieldType:   FieldTypeText,
			constraints: nil,
			expected:    "TEXT",
		},
		{
			name:        "text with MaxLength maps to VARCHAR(N)",
			fieldType:   FieldTypeText,
			constraints: &Constraints{MaxLength: new(255)},
			expected:    "VARCHAR(255)",
		},
		{
			name:        "text with MaxLength 100 maps to VARCHAR(100)",
			fieldType:   FieldTypeText,
			constraints: &Constraints{MaxLength: new(100)},
			expected:    "VARCHAR(100)",
		},
		{
			name:        "number maps to BIGINT",
			fieldType:   FieldTypeNumber,
			constraints: nil,
			expected:    "BIGINT",
		},
		{
			name:        "decimal maps to NUMERIC",
			fieldType:   FieldTypeDecimal,
			constraints: nil,
			expected:    "NUMERIC",
		},
		{
			name:        "boolean maps to BOOLEAN",
			fieldType:   FieldTypeBoolean,
			constraints: nil,
			expected:    "BOOLEAN",
		},
		{
			name:        "datetime maps to TIMESTAMP WITH TIME ZONE",
			fieldType:   FieldTypeDatetime,
			constraints: nil,
			expected:    "TIMESTAMP WITH TIME ZONE",
		},
		{
			name:        "json maps to JSONB",
			fieldType:   FieldTypeJSON,
			constraints: nil,
			expected:    "JSONB",
		},
		{
			name:        "relation maps to BIGINT",
			fieldType:   FieldTypeRelation,
			constraints: nil,
			expected:    "BIGINT",
		},
		{
			name:        "unknown maps to TEXT",
			fieldType:   FieldType("unknown"),
			constraints: nil,
			expected:    "TEXT",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mapper := NewTypeMapper(DialectPostgreSQL)
			if got := mapper.MapColumnType(tt.fieldType, tt.constraints); got != tt.expected {
				t.Errorf("MapColumnType(%q, %v) = %q, want %q", tt.fieldType, tt.constraints, got, tt.expected)
			}
		})
	}
}

func TestTypeMapper_MapColumnType_UnknownDialect(t *testing.T) {
	tests := []struct {
		name        string
		fieldType   FieldType
		constraints *Constraints
		expected    string
	}{
		{
			name:        "text maps to TEXT",
			fieldType:   FieldTypeText,
			constraints: nil,
			expected:    "TEXT",
		},
		{
			name:        "number maps to INTEGER",
			fieldType:   FieldTypeNumber,
			constraints: nil,
			expected:    "INTEGER",
		},
		{
			name:        "decimal maps to REAL",
			fieldType:   FieldTypeDecimal,
			constraints: nil,
			expected:    "REAL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mapper := NewTypeMapper(Dialect("unknown"))
			if got := mapper.MapColumnType(tt.fieldType, tt.constraints); got != tt.expected {
				t.Errorf("MapColumnType(%q, %v) = %q, want %q", tt.fieldType, tt.constraints, got, tt.expected)
			}
		})
	}
}

func TestTypeMapper_FormatDefaultValue_SQLite(t *testing.T) {
	tests := []struct {
		name      string
		value     interface{}
		fieldType FieldType
		expected  string
	}{
		{
			name:      "string value",
			value:     "hello",
			fieldType: FieldTypeText,
			expected:  "'hello'",
		},
		{
			name:      "string with single quote",
			value:     "O'Reilly",
			fieldType: FieldTypeText,
			expected:  "'O'Reilly'",
		},
		{
			name:      "int value",
			value:     42,
			fieldType: FieldTypeNumber,
			expected:  "42",
		},
		{
			name:      "int64 value",
			value:     int64(123),
			fieldType: FieldTypeNumber,
			expected:  "123",
		},
		{
			name:      "float64 value",
			value:     float64(1.23),
			fieldType: FieldTypeDecimal,
			expected:  "1.230000",
		},
		{
			name:      "bool true",
			value:     true,
			fieldType: FieldTypeBoolean,
			expected:  "1",
		},
		{
			name:      "bool false",
			value:     false,
			fieldType: FieldTypeBoolean,
			expected:  "0",
		},
		{
			name:      "nil value",
			value:     nil,
			fieldType: FieldTypeText,
			expected:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mapper := NewTypeMapper(DialectSQLite)
			if got := mapper.FormatDefaultValue(tt.value, tt.fieldType); got != tt.expected {
				t.Errorf("FormatDefaultValue(%v, %q) = %q, want %q", tt.value, tt.fieldType, got, tt.expected)
			}
		})
	}
}

func TestTypeMapper_FormatDefaultValue_PostgreSQL(t *testing.T) {
	tests := []struct {
		name      string
		value     interface{}
		fieldType FieldType
		expected  string
	}{
		{
			name:      "string value",
			value:     "hello",
			fieldType: FieldTypeText,
			expected:  "'hello'",
		},
		{
			name:      "string with single quote",
			value:     "O'Reilly",
			fieldType: FieldTypeText,
			expected:  "'O'Reilly'",
		},
		{
			name:      "int value",
			value:     42,
			fieldType: FieldTypeNumber,
			expected:  "42",
		},
		{
			name:      "int64 value",
			value:     int64(123),
			fieldType: FieldTypeNumber,
			expected:  "123",
		},
		{
			name:      "float64 value",
			value:     float64(1.23),
			fieldType: FieldTypeDecimal,
			expected:  "1.230000",
		},
		{
			name:      "bool true",
			value:     true,
			fieldType: FieldTypeBoolean,
			expected:  "TRUE",
		},
		{
			name:      "bool false",
			value:     false,
			fieldType: FieldTypeBoolean,
			expected:  "FALSE",
		},
		{
			name:      "nil value",
			value:     nil,
			fieldType: FieldTypeText,
			expected:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mapper := NewTypeMapper(DialectPostgreSQL)
			if got := mapper.FormatDefaultValue(tt.value, tt.fieldType); got != tt.expected {
				t.Errorf("FormatDefaultValue(%v, %q) = %q, want %q", tt.value, tt.fieldType, got, tt.expected)
			}
		})
	}
}

func TestTypeMapper_FormatDefaultValue_UnknownType(t *testing.T) {
	tests := []struct {
		name     string
		value    interface{}
		expected string
	}{
		{
			name:     "unknown type uses default case",
			value:    []byte("data"),
			expected: "'[100 97 116 97]'",
		},
		{
			name:     "struct value uses default case",
			value:    struct{}{},
			expected: "'{}'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mapper := NewTypeMapper(DialectSQLite)
			if got := mapper.FormatDefaultValue(tt.value, FieldTypeText); got != tt.expected {
				t.Errorf("FormatDefaultValue(%v, Text) = %q, want %q", tt.value, got, tt.expected)
			}
		})
	}
}

func TestTypeMapper_NeedsRebuild_SQLite(t *testing.T) {
	tests := []struct {
		name     string
		op       DiffOperation
		expected bool
	}{
		{
			name:     "OpColumnDrop returns true",
			op:       DiffOperation{Type: OpColumnDrop},
			expected: true,
		},
		{
			name:     "OpColumnModify returns true",
			op:       DiffOperation{Type: OpColumnModify},
			expected: true,
		},
		{
			name:     "OpColumnAdd returns false",
			op:       DiffOperation{Type: OpColumnAdd},
			expected: false,
		},
		{
			name:     "OpTableCreate returns false",
			op:       DiffOperation{Type: OpTableCreate},
			expected: false,
		},
		{
			name:     "OpTableDrop returns false",
			op:       DiffOperation{Type: OpTableDrop},
			expected: false,
		},
		{
			name:     "OpIndexAdd returns false",
			op:       DiffOperation{Type: OpIndexAdd},
			expected: false,
		},
		{
			name:     "OpIndexDrop returns false",
			op:       DiffOperation{Type: OpIndexDrop},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mapper := NewTypeMapper(DialectSQLite)
			if got := mapper.NeedsRebuild(tt.op); got != tt.expected {
				t.Errorf("NeedsRebuild(%q) = %v, want %v", tt.op.Type, got, tt.expected)
			}
		})
	}
}

func TestTypeMapper_NeedsRebuild_PostgreSQL(t *testing.T) {
	tests := []struct {
		name     string
		op       DiffOperation
		expected bool
	}{
		{
			name:     "OpColumnDrop returns false",
			op:       DiffOperation{Type: OpColumnDrop},
			expected: false,
		},
		{
			name:     "OpColumnModify returns false",
			op:       DiffOperation{Type: OpColumnModify},
			expected: false,
		},
		{
			name:     "OpColumnAdd returns false",
			op:       DiffOperation{Type: OpColumnAdd},
			expected: false,
		},
		{
			name:     "OpTableCreate returns false",
			op:       DiffOperation{Type: OpTableCreate},
			expected: false,
		},
		{
			name:     "OpTableDrop returns false",
			op:       DiffOperation{Type: OpTableDrop},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mapper := NewTypeMapper(DialectPostgreSQL)
			if got := mapper.NeedsRebuild(tt.op); got != tt.expected {
				t.Errorf("NeedsRebuild(%q) = %v, want %v", tt.op.Type, got, tt.expected)
			}
		})
	}
}

func TestTypeMapper_NeedsRebuild_UnknownDialect(t *testing.T) {
	mapper := NewTypeMapper(Dialect("unknown"))

	if got := mapper.NeedsRebuild(DiffOperation{Type: OpColumnDrop}); got {
		t.Errorf("NeedsRebuild(OpColumnDrop) = %v, want false for unknown dialect", got)
	}

	if got := mapper.NeedsRebuild(DiffOperation{Type: OpColumnModify}); got {
		t.Errorf("NeedsRebuild(OpColumnModify) = %v, want false for unknown dialect", got)
	}

	if got := mapper.NeedsRebuild(DiffOperation{Type: OpColumnAdd}); got {
		t.Errorf("NeedsRebuild(OpColumnAdd) = %v, want false for unknown dialect", got)
	}
}
