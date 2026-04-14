package ddl

import (
	"testing"
)

func TestFieldType_IsValid(t *testing.T) {
	tests := []struct {
		ft       FieldType
		expected bool
	}{
		{FieldTypeText, true},
		{FieldTypeNumber, true},
		{FieldTypeDecimal, true},
		{FieldTypeBoolean, true},
		{FieldTypeDatetime, true},
		{FieldTypeJSON, true},
		{FieldTypeRelation, true},
		{FieldType("binary_blob"), false},
		{FieldType(""), false},
	}

	for _, tt := range tests {
		t.Run(string(tt.ft), func(t *testing.T) {
			if got := tt.ft.IsValid(); got != tt.expected {
				t.Errorf("FieldType(%q).IsValid() = %v, want %v", tt.ft, got, tt.expected)
			}
		})
	}
}

func TestSchema_Validate(t *testing.T) {
	tests := []struct {
		name      string
		schema    *Schema
		wantError bool
	}{
		{
			name: "valid schema with basic fields",
			schema: &Schema{
				Name: "posts",
				Fields: []FieldDefinition{
					{Name: "title", Type: FieldTypeText},
					{Name: "body", Type: FieldTypeText},
					{Name: "published", Type: FieldTypeBoolean},
				},
			},
			wantError: false,
		},
		{
			name: "valid schema with constraints",
			schema: &Schema{
				Name: "users",
				Fields: []FieldDefinition{
					{
						Name: "email",
						Type: FieldTypeText,
						Constraints: &Constraints{
							Nullable:  false,
							Unique:    true,
							MaxLength: new(255),
						},
					},
				},
			},
			wantError: false,
		},
		{
			name: "valid schema with indexes",
			schema: &Schema{
				Name: "posts",
				Fields: []FieldDefinition{
					{Name: "title", Type: FieldTypeText},
				},
				Indexes: []IndexDefinition{
					{Name: "idx_posts_title", Columns: []string{"title"}, Unique: false},
				},
			},
			wantError: false,
		},
		{
			name: "invalid field type",
			schema: &Schema{
				Name: "posts",
				Fields: []FieldDefinition{
					{Name: "data", Type: FieldType("binary_blob")},
				},
			},
			wantError: true,
		},
		{
			name: "missing schema name",
			schema: &Schema{
				Name: "",
				Fields: []FieldDefinition{
					{Name: "title", Type: FieldTypeText},
				},
			},
			wantError: true,
		},
		{
			name: "duplicate field names",
			schema: &Schema{
				Name: "posts",
				Fields: []FieldDefinition{
					{Name: "title", Type: FieldTypeText},
					{Name: "title", Type: FieldTypeText},
				},
			},
			wantError: true,
		},
		{
			name: "empty fields array",
			schema: &Schema{
				Name:   "posts",
				Fields: []FieldDefinition{},
			},
			wantError: true,
		},
		{
			name: "index references non-existent column",
			schema: &Schema{
				Name: "posts",
				Fields: []FieldDefinition{
					{Name: "title", Type: FieldTypeText},
				},
				Indexes: []IndexDefinition{
					{Name: "idx_nonexistent", Columns: []string{"nonexistent"}, Unique: false},
				},
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.schema.Validate()
			if (err != nil) != tt.wantError {
				t.Errorf("Schema.Validate() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}

func TestSchema_JSONRoundTrip(t *testing.T) {
	original := &Schema{
		Name: "posts",
		Fields: []FieldDefinition{
			{
				Name: "title",
				Type: FieldTypeText,
				Constraints: &Constraints{
					Nullable:  false,
					MaxLength: new(255),
				},
			},
			{Name: "body", Type: FieldTypeText},
		},
		Indexes: []IndexDefinition{
			{Name: "idx_posts_title", Columns: []string{"title"}, Unique: false},
		},
	}

	data, err := original.ToJSON()
	if err != nil {
		t.Fatalf("Schema.ToJSON() error = %v", err)
	}

	parsed, err := FromJSON(data)
	if err != nil {
		t.Fatalf("FromJSON() error = %v", err)
	}

	if parsed.Name != original.Name {
		t.Errorf("parsed.Name = %q, want %q", parsed.Name, original.Name)
	}

	if len(parsed.Fields) != len(original.Fields) {
		t.Errorf("len(parsed.Fields) = %d, want %d", len(parsed.Fields), len(original.Fields))
	}

	if len(parsed.Indexes) != len(original.Indexes) {
		t.Errorf("len(parsed.Indexes) = %d, want %d", len(parsed.Indexes), len(original.Indexes))
	}
}

func TestSchema_GetField(t *testing.T) {
	schema := &Schema{
		Name: "posts",
		Fields: []FieldDefinition{
			{Name: "title", Type: FieldTypeText},
			{Name: "body", Type: FieldTypeText},
		},
	}

	if field := schema.GetField("title"); field == nil {
		t.Error("GetField(\"title\") returned nil")
	}

	if field := schema.GetField("nonexistent"); field != nil {
		t.Error("GetField(\"nonexistent\") should return nil")
	}
}

func TestSchema_Clone(t *testing.T) {
	original := &Schema{
		Name: "posts",
		Fields: []FieldDefinition{
			{Name: "title", Type: FieldTypeText},
		},
	}

	clone := original.Clone()

	clone.Fields[0].Name = "modified"

	if original.Fields[0].Name == "modified" {
		t.Error("Clone did not create a deep copy")
	}
}

func TestDiffOperation_IsDestructive(t *testing.T) {
	tests := []struct {
		op       DiffOperationType
		expected bool
	}{
		{OpTableCreate, false},
		{OpTableDrop, true},
		{OpColumnAdd, false},
		{OpColumnDrop, true},
		{OpColumnModify, false},
		{OpIndexAdd, false},
		{OpIndexDrop, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.op), func(t *testing.T) {
			op := DiffOperation{Type: tt.op}
			if got := op.IsDestructive(); got != tt.expected {
				t.Errorf("DiffOperation{Type: %q}.IsDestructive() = %v, want %v", tt.op, got, tt.expected)
			}
		})
	}
}

func TestDiffResult_AddOperation(t *testing.T) {
	result := NewDiffResult()

	if !result.IsEmpty() {
		t.Error("New DiffResult should be empty")
	}

	result.AddOperation(DiffOperation{Type: OpColumnAdd})
	if result.IsEmpty() {
		t.Error("DiffResult should not be empty after adding operation")
	}
	if result.HasDestructiveOps {
		t.Error("Non-destructive operation should not set HasDestructiveOps")
	}

	result.AddOperation(DiffOperation{Type: OpColumnDrop})
	if !result.HasDestructiveOps {
		t.Error("Destructive operation should set HasDestructiveOps")
	}

	destructive := result.GetDestructiveOperations()
	if len(destructive) != 1 {
		t.Errorf("GetDestructiveOperations() returned %d ops, want 1", len(destructive))
	}
}

func TestTypeMapper_MapColumnType(t *testing.T) {
	tests := []struct {
		dialect   Dialect
		fieldType FieldType
		expected  string
		maxLength *int
	}{
		{DialectSQLite, FieldTypeText, "TEXT", nil},
		{DialectSQLite, FieldTypeNumber, "INTEGER", nil},
		{DialectSQLite, FieldTypeBoolean, "INTEGER", nil},
		{DialectSQLite, FieldTypeJSON, "TEXT", nil},
		{DialectPostgreSQL, FieldTypeText, "TEXT", nil},
		{DialectPostgreSQL, FieldTypeText, "VARCHAR(255)", new(255)},
		{DialectPostgreSQL, FieldTypeNumber, "BIGINT", nil},
		{DialectPostgreSQL, FieldTypeBoolean, "BOOLEAN", nil},
		{DialectPostgreSQL, FieldTypeJSON, "JSONB", nil},
		{DialectPostgreSQL, FieldTypeDatetime, "TIMESTAMP WITH TIME ZONE", nil},
	}

	for _, tt := range tests {
		t.Run(string(tt.dialect)+"_"+string(tt.fieldType), func(t *testing.T) {
			mapper := NewTypeMapper(tt.dialect)
			var constraints *Constraints
			if tt.maxLength != nil {
				constraints = &Constraints{MaxLength: tt.maxLength}
			}
			if got := mapper.MapColumnType(tt.fieldType, constraints); got != tt.expected {
				t.Errorf("MapColumnType(%q, %v) = %q, want %q", tt.fieldType, constraints, got, tt.expected)
			}
		})
	}
}

func intPtr(i int) *int {
	return &i
}

func TestFromJSON_InvalidJSON(t *testing.T) {
	_, err := FromJSON([]byte(`{invalid json`))
	if err == nil {
		t.Error("FromJSON() should return error for invalid JSON")
	}
}

func TestFromJSON_InvalidSchema(t *testing.T) {
	_, err := FromJSON([]byte(`{"name": "", "fields": []}`))
	if err == nil {
		t.Error("FromJSON() should return error for empty name")
	}

	_, err = FromJSON([]byte(`{"name": "test", "fields": []}`))
	if err == nil {
		t.Error("FromJSON() should return error for empty fields")
	}

	_, err = FromJSON([]byte(`{"name": "test", "fields": [{"name": "f1", "type": "bad_type"}]}`))
	if err == nil {
		t.Error("FromJSON() should return error for invalid field type")
	}
}

func TestFromJSON_ValidFullSchema(t *testing.T) {
	data := []byte(`{
		"name": "articles",
		"fields": [
			{"name": "id", "type": "number", "constraints": {"nullable": false}},
			{"name": "title", "type": "text", "constraints": {"nullable": false, "max_length": 200}},
			{"name": "tags", "type": "json"},
			{"name": "author_id", "type": "relation", "foreign_key": {"table": "users", "column": "id", "on_delete": "CASCADE"}}
		],
		"indexes": [
			{"name": "idx_articles_title", "columns": ["title"], "unique": true}
		],
		"table_name": "cms_articles"
	}`)

	schema, err := FromJSON(data)
	if err != nil {
		t.Fatalf("FromJSON() error = %v", err)
	}
	if schema.Name != "articles" {
		t.Errorf("Name = %q, want %q", schema.Name, "articles")
	}
	if len(schema.Fields) != 4 {
		t.Errorf("len(Fields) = %d, want 4", len(schema.Fields))
	}
	if len(schema.Indexes) != 1 {
		t.Errorf("len(Indexes) = %d, want 1", len(schema.Indexes))
	}
	if schema.TableName != "cms_articles" {
		t.Errorf("TableName = %q, want %q", schema.TableName, "cms_articles")
	}
	if schema.GetTableName() != "cms_articles" {
		t.Errorf("GetTableName() = %q, want %q", schema.GetTableName(), "cms_articles")
	}
}

func TestSchema_GetTableName_Default(t *testing.T) {
	schema := &Schema{Name: "posts"}
	if schema.GetTableName() != "posts" {
		t.Errorf("GetTableName() = %q, want %q", schema.GetTableName(), "posts")
	}
}

func TestSchema_Clone_WithIndexes(t *testing.T) {
	original := &Schema{
		Name: "posts",
		Fields: []FieldDefinition{
			{Name: "id", Type: FieldTypeNumber, Constraints: &Constraints{Nullable: false}},
			{Name: "title", Type: FieldTypeText},
		},
		Indexes: []IndexDefinition{
			{Name: "idx_posts_title", Columns: []string{"title"}, Unique: true},
		},
	}

	clone := original.Clone()

	if len(clone.Indexes) != len(original.Indexes) {
		t.Fatalf("clone has %d indexes, want %d", len(clone.Indexes), len(original.Indexes))
	}

	clone.Indexes[0].Name = "modified_idx"
	if original.Indexes[0].Name == "modified_idx" {
		t.Error("modifying clone indexes should not affect original")
	}
}

func TestSchema_Clone_WithConstraints(t *testing.T) {
	original := &Schema{
		Name: "posts",
		Fields: []FieldDefinition{
			{Name: "title", Type: FieldTypeText, Constraints: &Constraints{Nullable: false, MaxLength: intPtr(100)}},
		},
	}

	clone := original.Clone()

	if clone.Fields[0].Constraints.MaxLength == nil {
		t.Fatal("cloned constraint MaxLength should not be nil")
	}

	*clone.Fields[0].Constraints.MaxLength = 200
	if *original.Fields[0].Constraints.MaxLength == 200 {
		t.Error("modifying cloned constraints should not affect original")
	}
}

func TestSchema_Validate_MissingFieldName(t *testing.T) {
	schema := &Schema{
		Name: "posts",
		Fields: []FieldDefinition{
			{Name: "", Type: FieldTypeText},
		},
	}
	if err := schema.Validate(); err == nil {
		t.Error("Validate() should return error for empty field name")
	}
}

func TestSchema_Validate_IndexEmptyName(t *testing.T) {
	schema := &Schema{
		Name:   "posts",
		Fields: []FieldDefinition{{Name: "title", Type: FieldTypeText}},
		Indexes: []IndexDefinition{
			{Name: "", Columns: []string{"title"}},
		},
	}
	if err := schema.Validate(); err == nil {
		t.Error("Validate() should return error for index with empty name")
	}
}

func TestSchema_Validate_IndexEmptyColumns(t *testing.T) {
	schema := &Schema{
		Name:   "posts",
		Fields: []FieldDefinition{{Name: "title", Type: FieldTypeText}},
		Indexes: []IndexDefinition{
			{Name: "idx_posts", Columns: []string{}},
		},
	}
	if err := schema.Validate(); err == nil {
		t.Error("Validate() should return error for index with empty columns")
	}
}
