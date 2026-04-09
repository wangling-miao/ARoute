package ddl

import (
	"testing"

	"github.com/wangling-miao/aroute/sdk/interfaces"
)

// helper to create a pointer to an int
func intPtrDiff(i int) *int {
	return &i
}

// ---------------------------------------------------------------------------
// TestDiff_NilActual produces OpTableCreate
// ---------------------------------------------------------------------------

func TestDiff_NilActual(t *testing.T) {
	engine := NewDiffEngine()

	desired := &Schema{
		Name: "articles",
		Fields: []FieldDefinition{
			{Name: "title", Type: FieldTypeText},
			{Name: "body", Type: FieldTypeText},
		},
	}

	result, err := engine.Diff(desired, nil)
	if err != nil {
		t.Fatalf("Diff() returned unexpected error: %v", err)
	}

	if result.IsEmpty() {
		t.Fatal("DiffResult should not be empty when actual is nil")
	}

	if len(result.Operations) != 1 {
		t.Fatalf("expected 1 operation, got %d", len(result.Operations))
	}

	op := result.Operations[0]
	if op.Type != OpTableCreate {
		t.Errorf("expected OpTableCreate, got %q", op.Type)
	}
	if op.TableName != "articles" {
		t.Errorf("expected table name 'articles', got %q", op.TableName)
	}
	if op.Schema == nil || op.Schema.Name != "articles" {
		t.Error("OpTableCreate should carry the full desired schema")
	}
	if result.HasDestructiveOps {
		t.Error("OpTableCreate should not be marked as destructive")
	}
}

// ---------------------------------------------------------------------------
// TestDiff_IdenticalSchemas produces empty result
// ---------------------------------------------------------------------------

func TestDiff_IdenticalSchemas(t *testing.T) {
	engine := NewDiffEngine()

	desired := &Schema{
		Name: "posts",
		Fields: []FieldDefinition{
			{Name: "title", Type: FieldTypeText},
			{Name: "views", Type: FieldTypeNumber},
		},
	}

	actual := &interfaces.TableDefinition{
		Name: "posts",
		Columns: []interfaces.ColumnDefinition{
			{Name: "title", Type: "text"},
			{Name: "views", Type: "number"},
		},
		Indexes: nil,
	}

	result, err := engine.Diff(desired, actual)
	if err != nil {
		t.Fatalf("Diff() returned unexpected error: %v", err)
	}

	if !result.IsEmpty() {
		t.Errorf("expected empty result for identical schemas, got %d operations", len(result.Operations))
		for _, op := range result.Operations {
			t.Logf("  unexpected op: %s", op.String())
		}
	}
	if result.HasDestructiveOps {
		t.Error("expected no destructive ops for identical schemas")
	}
}

// ---------------------------------------------------------------------------
// TestDiff_NewColumns produces OpColumnAdd
// ---------------------------------------------------------------------------

func TestDiff_NewColumns(t *testing.T) {
	engine := NewDiffEngine()

	desired := &Schema{
		Name: "posts",
		Fields: []FieldDefinition{
			{Name: "title", Type: FieldTypeText},
			{Name: "body", Type: FieldTypeText},
			{Name: "author", Type: FieldTypeText},
		},
	}

	// actual only has "title" — "body" and "author" are new
	actual := &interfaces.TableDefinition{
		Name: "posts",
		Columns: []interfaces.ColumnDefinition{
			{Name: "title", Type: "text"},
		},
	}

	result, err := engine.Diff(desired, actual)
	if err != nil {
		t.Fatalf("Diff() returned unexpected error: %v", err)
	}

	addOps := filterOps(result.Operations, OpColumnAdd)
	if len(addOps) != 2 {
		t.Fatalf("expected 2 OpColumnAdd, got %d", len(addOps))
	}

	addedNames := opColumnNames(addOps)
	if !contains(addedNames, "body") {
		t.Error("expected OpColumnAdd for 'body'")
	}
	if !contains(addedNames, "author") {
		t.Error("expected OpColumnAdd for 'author'")
	}

	// Verify constraints are carried through
	for _, op := range addOps {
		if op.TableName != "posts" {
			t.Errorf("expected TableName 'posts', got %q", op.TableName)
		}
		if op.ColumnType == "" {
			t.Errorf("expected non-empty ColumnType for OpColumnAdd on %q", op.ColumnName)
		}
	}
}

// ---------------------------------------------------------------------------
// TestDiff_RemovedColumns produces OpColumnDrop (destructive)
// ---------------------------------------------------------------------------

func TestDiff_RemovedColumns(t *testing.T) {
	engine := NewDiffEngine()

	desired := &Schema{
		Name: "posts",
		Fields: []FieldDefinition{
			{Name: "title", Type: FieldTypeText},
		},
	}

	// actual has "title" and "legacy_field" — "legacy_field" is removed
	actual := &interfaces.TableDefinition{
		Name: "posts",
		Columns: []interfaces.ColumnDefinition{
			{Name: "title", Type: "text"},
			{Name: "legacy_field", Type: "text"},
		},
	}

	result, err := engine.Diff(desired, actual)
	if err != nil {
		t.Fatalf("Diff() returned unexpected error: %v", err)
	}

	dropOps := filterOps(result.Operations, OpColumnDrop)
	if len(dropOps) != 1 {
		t.Fatalf("expected 1 OpColumnDrop, got %d", len(dropOps))
	}
	if dropOps[0].ColumnName != "legacy_field" {
		t.Errorf("expected drop of 'legacy_field', got %q", dropOps[0].ColumnName)
	}
	if !result.HasDestructiveOps {
		t.Error("expected HasDestructiveOps=true when columns are dropped")
	}
}

// ---------------------------------------------------------------------------
// TestDiff_ModifiedColumnType produces OpColumnModify
// ---------------------------------------------------------------------------

func TestDiff_ModifiedColumnType(t *testing.T) {
	tests := []struct {
		name         string
		desiredType  FieldType
		actualType   string
		expectModify bool
	}{
		{
			name:         "type changed from text to number",
			desiredType:  FieldTypeNumber,
			actualType:   "text",
			expectModify: true,
		},
		{
			name:         "type unchanged (case-insensitive)",
			desiredType:  FieldTypeText,
			actualType:   "TEXT",
			expectModify: false,
		},
		{
			name:         "type unchanged (exact match)",
			desiredType:  FieldTypeText,
			actualType:   "text",
			expectModify: false,
		},
		{
			name:         "type changed from boolean to datetime",
			desiredType:  FieldTypeDatetime,
			actualType:   "boolean",
			expectModify: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine := NewDiffEngine()

			desired := &Schema{
				Name: "posts",
				Fields: []FieldDefinition{
					{Name: "field_a", Type: tt.desiredType},
				},
			}

			actual := &interfaces.TableDefinition{
				Name: "posts",
				Columns: []interfaces.ColumnDefinition{
					{Name: "field_a", Type: tt.actualType},
				},
			}

			result, err := engine.Diff(desired, actual)
			if err != nil {
				t.Fatalf("Diff() returned unexpected error: %v", err)
			}

			modifyOps := filterOps(result.Operations, OpColumnModify)
			gotModify := len(modifyOps) > 0

			if gotModify != tt.expectModify {
				t.Errorf("expected modify=%v, got modify=%v (modifyOps=%d)", tt.expectModify, gotModify, len(modifyOps))
			}

			if tt.expectModify && len(modifyOps) == 1 {
				op := modifyOps[0]
				if op.OldColumnType != tt.actualType {
					t.Errorf("OldColumnType = %q, want %q", op.OldColumnType, tt.actualType)
				}
				if op.ColumnType != string(tt.desiredType) {
					t.Errorf("ColumnType = %q, want %q", op.ColumnType, tt.desiredType)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestDiff_NewIndexes produces OpIndexAdd
// ---------------------------------------------------------------------------

func TestDiff_NewIndexes(t *testing.T) {
	engine := NewDiffEngine()

	desired := &Schema{
		Name: "posts",
		Fields: []FieldDefinition{
			{Name: "title", Type: FieldTypeText},
			{Name: "slug", Type: FieldTypeText},
		},
		Indexes: []IndexDefinition{
			{Name: "idx_posts_slug", Columns: []string{"slug"}, Unique: true},
			{Name: "idx_posts_title", Columns: []string{"title"}, Unique: false},
		},
	}

	actual := &interfaces.TableDefinition{
		Name: "posts",
		Columns: []interfaces.ColumnDefinition{
			{Name: "title", Type: "text"},
			{Name: "slug", Type: "text"},
		},
		Indexes: nil, // no indexes in actual
	}

	result, err := engine.Diff(desired, actual)
	if err != nil {
		t.Fatalf("Diff() returned unexpected error: %v", err)
	}

	addOps := filterOps(result.Operations, OpIndexAdd)
	if len(addOps) != 2 {
		t.Fatalf("expected 2 OpIndexAdd, got %d", len(addOps))
	}

	idxNames := opIndexNames(addOps)
	if !contains(idxNames, "idx_posts_slug") {
		t.Error("expected OpIndexAdd for 'idx_posts_slug'")
	}
	if !contains(idxNames, "idx_posts_title") {
		t.Error("expected OpIndexAdd for 'idx_posts_title'")
	}

	// Verify unique flag is carried through
	for _, op := range addOps {
		if op.IndexName == "idx_posts_slug" && !op.IndexUnique {
			t.Error("expected idx_posts_slug to be unique")
		}
		if op.IndexName == "idx_posts_title" && op.IndexUnique {
			t.Error("expected idx_posts_title to be non-unique")
		}
	}
}

// ---------------------------------------------------------------------------
// TestDiff_RemovedIndexes produces OpIndexDrop
// ---------------------------------------------------------------------------

func TestDiff_RemovedIndexes(t *testing.T) {
	engine := NewDiffEngine()

	desired := &Schema{
		Name: "posts",
		Fields: []FieldDefinition{
			{Name: "title", Type: FieldTypeText},
		},
		Indexes: nil, // no indexes in desired
	}

	actual := &interfaces.TableDefinition{
		Name: "posts",
		Columns: []interfaces.ColumnDefinition{
			{Name: "title", Type: "text"},
		},
		Indexes: []interfaces.IndexDefinition{
			{Name: "idx_posts_legacy", Columns: []string{"title"}, Unique: false},
		},
	}

	result, err := engine.Diff(desired, actual)
	if err != nil {
		t.Fatalf("Diff() returned unexpected error: %v", err)
	}

	dropOps := filterOps(result.Operations, OpIndexDrop)
	if len(dropOps) != 1 {
		t.Fatalf("expected 1 OpIndexDrop, got %d", len(dropOps))
	}
	if dropOps[0].IndexName != "idx_posts_legacy" {
		t.Errorf("expected drop of 'idx_posts_legacy', got %q", dropOps[0].IndexName)
	}
}

// ---------------------------------------------------------------------------
// TestDiff_FieldIndexAutoGenerate — field.Index=true auto-generates index
// ---------------------------------------------------------------------------

func TestDiff_FieldIndexAutoGenerate(t *testing.T) {
	engine := NewDiffEngine()

	desired := &Schema{
		Name: "posts",
		Fields: []FieldDefinition{
			{Name: "title", Type: FieldTypeText, Index: true},
			{Name: "slug", Type: FieldTypeText, Index: true, Constraints: &Constraints{Unique: true}},
		},
	}

	actual := &interfaces.TableDefinition{
		Name: "posts",
		Columns: []interfaces.ColumnDefinition{
			{Name: "title", Type: "text"},
			{Name: "slug", Type: "text"},
		},
		Indexes: nil,
	}

	result, err := engine.Diff(desired, actual)
	if err != nil {
		t.Fatalf("Diff() returned unexpected error: %v", err)
	}

	addOps := filterOps(result.Operations, OpIndexAdd)
	if len(addOps) != 2 {
		t.Fatalf("expected 2 auto-generated OpIndexAdd, got %d", len(addOps))
	}

	idxNames := opIndexNames(addOps)

	// Auto-generated index names follow pattern: idx_{schema}_{field}
	if !contains(idxNames, "idx_posts_title") {
		t.Error("expected auto-generated index 'idx_posts_title'")
	}
	if !contains(idxNames, "idx_posts_slug") {
		t.Error("expected auto-generated index 'idx_posts_slug'")
	}

	// Verify uniqueness: slug has Unique constraint, title does not
	for _, op := range addOps {
		if op.IndexName == "idx_posts_slug" && !op.IndexUnique {
			t.Error("expected idx_posts_slug to be unique (field has Unique constraint)")
		}
		if op.IndexName == "idx_posts_title" && op.IndexUnique {
			t.Error("expected idx_posts_title to be non-unique (field has no Unique constraint)")
		}
	}
}

// ---------------------------------------------------------------------------
// TestDiff_FieldIndexNotDuplicated — explicit index and field.Index=true
// should not produce duplicate OpIndexAdd
// ---------------------------------------------------------------------------

func TestDiff_FieldIndexNotDuplicated(t *testing.T) {
	engine := NewDiffEngine()

	desired := &Schema{
		Name: "posts",
		Fields: []FieldDefinition{
			{Name: "title", Type: FieldTypeText, Index: true},
		},
		Indexes: []IndexDefinition{
			// Explicit index with the same auto-generated name
			{Name: "idx_posts_title", Columns: []string{"title"}, Unique: false},
		},
	}

	actual := &interfaces.TableDefinition{
		Name: "posts",
		Columns: []interfaces.ColumnDefinition{
			{Name: "title", Type: "text"},
		},
		Indexes: nil,
	}

	result, err := engine.Diff(desired, actual)
	if err != nil {
		t.Fatalf("Diff() returned unexpected error: %v", err)
	}

	addOps := filterOps(result.Operations, OpIndexAdd)
	// The explicit index should take precedence; field.Index should NOT add a duplicate
	if len(addOps) != 1 {
		t.Fatalf("expected 1 OpIndexAdd (no duplicate), got %d", len(addOps))
	}
	if addOps[0].IndexName != "idx_posts_title" {
		t.Errorf("expected index name 'idx_posts_title', got %q", addOps[0].IndexName)
	}
}

// ---------------------------------------------------------------------------
// TestDiff_HasDestructiveOps correctly set
// ---------------------------------------------------------------------------

func TestDiff_HasDestructiveOps(t *testing.T) {
	tests := []struct {
		name               string
		desired            *Schema
		actual             *interfaces.TableDefinition
		expectDestructive  bool
		destructiveOpCount int
	}{
		{
			name: "no destructive ops — only additions",
			desired: &Schema{
				Name: "posts",
				Fields: []FieldDefinition{
					{Name: "title", Type: FieldTypeText},
					{Name: "body", Type: FieldTypeText},
				},
			},
			actual: &interfaces.TableDefinition{
				Name: "posts",
				Columns: []interfaces.ColumnDefinition{
					{Name: "title", Type: "text"},
				},
			},
			expectDestructive:  false,
			destructiveOpCount: 0,
		},
		{
			name: "destructive — column dropped",
			desired: &Schema{
				Name: "posts",
				Fields: []FieldDefinition{
					{Name: "title", Type: FieldTypeText},
				},
			},
			actual: &interfaces.TableDefinition{
				Name: "posts",
				Columns: []interfaces.ColumnDefinition{
					{Name: "title", Type: "text"},
					{Name: "deprecated_col", Type: "text"},
				},
			},
			expectDestructive:  true,
			destructiveOpCount: 1,
		},
		{
			name: "destructive — index dropped",
			desired: &Schema{
				Name: "posts",
				Fields: []FieldDefinition{
					{Name: "title", Type: FieldTypeText},
				},
			},
			actual: &interfaces.TableDefinition{
				Name: "posts",
				Columns: []interfaces.ColumnDefinition{
					{Name: "title", Type: "text"},
				},
				Indexes: []interfaces.IndexDefinition{
					{Name: "idx_posts_old", Columns: []string{"title"}, Unique: false},
				},
			},
			expectDestructive:  false, // OpIndexDrop is NOT destructive per IsDestructive()
			destructiveOpCount: 0,
		},
		{
			name: "mixed — column add + column drop",
			desired: &Schema{
				Name: "posts",
				Fields: []FieldDefinition{
					{Name: "title", Type: FieldTypeText},
					{Name: "summary", Type: FieldTypeText},
				},
			},
			actual: &interfaces.TableDefinition{
				Name: "posts",
				Columns: []interfaces.ColumnDefinition{
					{Name: "title", Type: "text"},
					{Name: "old_field", Type: "text"},
				},
			},
			expectDestructive:  true,
			destructiveOpCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine := NewDiffEngine()
			result, err := engine.Diff(tt.desired, tt.actual)
			if err != nil {
				t.Fatalf("Diff() returned unexpected error: %v", err)
			}

			if result.HasDestructiveOps != tt.expectDestructive {
				t.Errorf("HasDestructiveOps = %v, want %v", result.HasDestructiveOps, tt.expectDestructive)
			}

			destructive := result.GetDestructiveOperations()
			if len(destructive) != tt.destructiveOpCount {
				t.Errorf("GetDestructiveOperations() returned %d ops, want %d", len(destructive), tt.destructiveOpCount)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestDiffOperation_String — verify human-readable output
// ---------------------------------------------------------------------------

func TestDiffOperation_String(t *testing.T) {
	tests := []struct {
		name     string
		op       DiffOperation
		expected string
	}{
		{
			name: "OpTableCreate",
			op: DiffOperation{
				Type:      OpTableCreate,
				TableName: "articles",
			},
			expected: "create table articles",
		},
		{
			name: "OpTableDrop",
			op: DiffOperation{
				Type:      OpTableDrop,
				TableName: "articles",
			},
			expected: "drop table articles",
		},
		{
			name: "OpColumnAdd",
			op: DiffOperation{
				Type:       OpColumnAdd,
				TableName:  "articles",
				ColumnName: "title",
				ColumnType: "text",
			},
			expected: "add column articles.title (text)",
		},
		{
			name: "OpColumnDrop",
			op: DiffOperation{
				Type:       OpColumnDrop,
				TableName:  "articles",
				ColumnName: "legacy",
			},
			expected: "drop column articles.legacy",
		},
		{
			name: "OpColumnModify",
			op: DiffOperation{
				Type:          OpColumnModify,
				TableName:     "articles",
				ColumnName:    "count",
				ColumnType:    "number",
				OldColumnType: "text",
			},
			expected: "modify column articles.count from text to number",
		},
		{
			name: "OpIndexAdd non-unique",
			op: DiffOperation{
				Type:         OpIndexAdd,
				TableName:    "articles",
				IndexName:    "idx_articles_title",
				IndexColumns: []string{"title"},
				IndexUnique:  false,
			},
			expected: "add index idx_articles_title on articles ([title])",
		},
		{
			name: "OpIndexAdd unique",
			op: DiffOperation{
				Type:         OpIndexAdd,
				TableName:    "articles",
				IndexName:    "idx_articles_slug",
				IndexColumns: []string{"slug"},
				IndexUnique:  true,
			},
			expected: "add unique index idx_articles_slug on articles ([slug])",
		},
		{
			name: "OpIndexDrop",
			op: DiffOperation{
				Type:      OpIndexDrop,
				TableName: "articles",
				IndexName: "idx_articles_old",
			},
			expected: "drop index idx_articles_old on articles",
		},
		{
			name: "unknown operation type",
			op: DiffOperation{
				Type:      DiffOperationType("unknown_op"),
				TableName: "articles",
			},
			expected: "unknown operation: unknown_op",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.op.String()
			if got != tt.expected {
				t.Errorf("String() = %q, want %q", got, tt.expected)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestDiff_CombinedScenario — full diff with adds, drops, modifies
// ---------------------------------------------------------------------------

func TestDiff_CombinedScenario(t *testing.T) {
	engine := NewDiffEngine()

	desired := &Schema{
		Name: "posts",
		Fields: []FieldDefinition{
			{Name: "title", Type: FieldTypeText},
			{Name: "views", Type: FieldTypeNumber}, // changed from text → number
			{Name: "status", Type: FieldTypeText},  // new column
		},
		Indexes: []IndexDefinition{
			{Name: "idx_posts_status", Columns: []string{"status"}, Unique: false},
		},
	}

	actual := &interfaces.TableDefinition{
		Name: "posts",
		Columns: []interfaces.ColumnDefinition{
			{Name: "title", Type: "text"},
			{Name: "views", Type: "text"},          // type mismatch
			{Name: "deprecated_col", Type: "text"}, // removed column
		},
		Indexes: []interfaces.IndexDefinition{
			{Name: "idx_posts_old_index", Columns: []string{"deprecated_col"}, Unique: false},
		},
	}

	result, err := engine.Diff(desired, actual)
	if err != nil {
		t.Fatalf("Diff() returned unexpected error: %v", err)
	}

	addOps := filterOps(result.Operations, OpColumnAdd)
	modifyOps := filterOps(result.Operations, OpColumnModify)
	dropOps := filterOps(result.Operations, OpColumnDrop)
	idxAddOps := filterOps(result.Operations, OpIndexAdd)
	idxDropOps := filterOps(result.Operations, OpIndexDrop)

	if len(addOps) != 1 || addOps[0].ColumnName != "status" {
		t.Errorf("expected 1 OpColumnAdd for 'status', got %v", opColumnNames(addOps))
	}
	if len(modifyOps) != 1 || modifyOps[0].ColumnName != "views" {
		t.Errorf("expected 1 OpColumnModify for 'views', got %v", opColumnNames(modifyOps))
	}
	if len(dropOps) != 1 || dropOps[0].ColumnName != "deprecated_col" {
		t.Errorf("expected 1 OpColumnDrop for 'deprecated_col', got %v", opColumnNames(dropOps))
	}
	if len(idxAddOps) != 1 || idxAddOps[0].IndexName != "idx_posts_status" {
		t.Errorf("expected 1 OpIndexAdd for 'idx_posts_status', got %v", opIndexNames(idxAddOps))
	}
	if len(idxDropOps) != 1 || idxDropOps[0].IndexName != "idx_posts_old_index" {
		t.Errorf("expected 1 OpIndexDrop for 'idx_posts_old_index', got %v", opIndexNames(idxDropOps))
	}

	if !result.HasDestructiveOps {
		t.Error("expected HasDestructiveOps=true because of column drop")
	}

	destructive := result.GetDestructiveOperations()
	if len(destructive) != 1 {
		t.Errorf("expected 1 destructive operation, got %d", len(destructive))
	}
}

// ---------------------------------------------------------------------------
// TestDiff_ColumnConstraintsCarried — constraints are passed through to ops
// ---------------------------------------------------------------------------

func TestDiff_ColumnConstraintsCarried(t *testing.T) {
	engine := NewDiffEngine()

	desired := &Schema{
		Name: "users",
		Fields: []FieldDefinition{
			{
				Name: "email",
				Type: FieldTypeText,
				Constraints: &Constraints{
					Nullable:  false,
					Unique:    true,
					MaxLength: intPtrDiff(255),
				},
			},
		},
	}

	actual := &interfaces.TableDefinition{
		Name:    "users",
		Columns: []interfaces.ColumnDefinition{},
	}

	result, err := engine.Diff(desired, actual)
	if err != nil {
		t.Fatalf("Diff() returned unexpected error: %v", err)
	}

	addOps := filterOps(result.Operations, OpColumnAdd)
	if len(addOps) != 1 {
		t.Fatalf("expected 1 OpColumnAdd, got %d", len(addOps))
	}

	op := addOps[0]
	if op.Constraints == nil {
		t.Fatal("expected Constraints to be non-nil")
	}
	if op.Constraints.Unique != true {
		t.Error("expected Constraints.Unique = true")
	}
	if op.Constraints.MaxLength == nil || *op.Constraints.MaxLength != 255 {
		t.Error("expected Constraints.MaxLength = 255")
	}
}

// ---------------------------------------------------------------------------
// TestDiff_ExistingIndexNotReAdded — indexes present in actual are not re-added
// ---------------------------------------------------------------------------

func TestDiff_ExistingIndexNotReAdded(t *testing.T) {
	engine := NewDiffEngine()

	desired := &Schema{
		Name: "posts",
		Fields: []FieldDefinition{
			{Name: "title", Type: FieldTypeText},
		},
		Indexes: []IndexDefinition{
			{Name: "idx_posts_title", Columns: []string{"title"}, Unique: false},
		},
	}

	actual := &interfaces.TableDefinition{
		Name: "posts",
		Columns: []interfaces.ColumnDefinition{
			{Name: "title", Type: "text"},
		},
		Indexes: []interfaces.IndexDefinition{
			{Name: "idx_posts_title", Columns: []string{"title"}, Unique: false},
		},
	}

	result, err := engine.Diff(desired, actual)
	if err != nil {
		t.Fatalf("Diff() returned unexpected error: %v", err)
	}

	addOps := filterOps(result.Operations, OpIndexAdd)
	if len(addOps) != 0 {
		t.Errorf("expected 0 OpIndexAdd for existing index, got %d", len(addOps))
		for _, op := range addOps {
			t.Logf("  unexpected: %s", op.String())
		}
	}

	dropOps := filterOps(result.Operations, OpIndexDrop)
	if len(dropOps) != 0 {
		t.Errorf("expected 0 OpIndexDrop for existing index, got %d", len(dropOps))
	}
}

// ---------------------------------------------------------------------------
// TestDiff_TableNameFromTableNameField — Schema.TableName overrides Name
// ---------------------------------------------------------------------------

func TestDiff_TableNameFromTableNameField(t *testing.T) {
	engine := NewDiffEngine()

	desired := &Schema{
		Name:      "content",
		TableName: "cms_content",
		Fields: []FieldDefinition{
			{Name: "title", Type: FieldTypeText},
		},
	}

	result, err := engine.Diff(desired, nil)
	if err != nil {
		t.Fatalf("Diff() returned unexpected error: %v", err)
	}

	if len(result.Operations) != 1 {
		t.Fatalf("expected 1 operation, got %d", len(result.Operations))
	}

	if result.Operations[0].TableName != "cms_content" {
		t.Errorf("expected TableName 'cms_content', got %q", result.Operations[0].TableName)
	}
}

// ---------------------------------------------------------------------------
// TestDiffResult_IsEmpty on fresh result
// ---------------------------------------------------------------------------

func TestDiffResult_IsEmpty(t *testing.T) {
	result := NewDiffResult()
	if !result.IsEmpty() {
		t.Error("fresh DiffResult should be empty")
	}

	result.AddOperation(DiffOperation{Type: OpColumnAdd})
	if result.IsEmpty() {
		t.Error("DiffResult with operations should not be empty")
	}
}

// ---------------------------------------------------------------------------
// TestDiff_ColumnModifyWithConstraints — OpColumnModify carries constraints
// ---------------------------------------------------------------------------

func TestDiff_ColumnModifyWithConstraints(t *testing.T) {
	engine := NewDiffEngine()

	desired := &Schema{
		Name: "posts",
		Fields: []FieldDefinition{
			{
				Name: "views",
				Type: FieldTypeNumber,
				Constraints: &Constraints{
					Nullable: false,
					Min:      floatPtr(0),
				},
			},
		},
	}

	actual := &interfaces.TableDefinition{
		Name: "posts",
		Columns: []interfaces.ColumnDefinition{
			{Name: "views", Type: "text"}, // type changed
		},
	}

	result, err := engine.Diff(desired, actual)
	if err != nil {
		t.Fatalf("Diff() returned unexpected error: %v", err)
	}

	modifyOps := filterOps(result.Operations, OpColumnModify)
	if len(modifyOps) != 1 {
		t.Fatalf("expected 1 OpColumnModify, got %d", len(modifyOps))
	}

	op := modifyOps[0]
	if op.Constraints == nil {
		t.Fatal("expected Constraints to be non-nil on OpColumnModify")
	}
	if op.OldColumnType != "text" {
		t.Errorf("OldColumnType = %q, want %q", op.OldColumnType, "text")
	}
	if op.ColumnType != "number" {
		t.Errorf("ColumnType = %q, want %q", op.ColumnType, "number")
	}
}

// ---------------------------------------------------------------------------
// TestDiff_AutoIndexNotReAdded — field.Index=true with existing index in actual
// ---------------------------------------------------------------------------

func TestDiff_AutoIndexNotReAdded(t *testing.T) {
	engine := NewDiffEngine()

	desired := &Schema{
		Name: "posts",
		Fields: []FieldDefinition{
			{Name: "title", Type: FieldTypeText, Index: true},
		},
	}

	actual := &interfaces.TableDefinition{
		Name: "posts",
		Columns: []interfaces.ColumnDefinition{
			{Name: "title", Type: "text"},
		},
		Indexes: []interfaces.IndexDefinition{
			{Name: "idx_posts_title", Columns: []string{"title"}, Unique: false},
		},
	}

	result, err := engine.Diff(desired, actual)
	if err != nil {
		t.Fatalf("Diff() returned unexpected error: %v", err)
	}

	addOps := filterOps(result.Operations, OpIndexAdd)
	if len(addOps) != 0 {
		t.Errorf("expected 0 OpIndexAdd (index already exists), got %d", len(addOps))
		for _, op := range addOps {
			t.Logf("  unexpected: %s", op.String())
		}
	}
}

func filterOps(ops []DiffOperation, opType DiffOperationType) []DiffOperation {
	var filtered []DiffOperation
	for _, op := range ops {
		if op.Type == opType {
			filtered = append(filtered, op)
		}
	}
	return filtered
}

func opColumnNames(ops []DiffOperation) []string {
	names := make([]string, len(ops))
	for i, op := range ops {
		names[i] = op.ColumnName
	}
	return names
}

func opIndexNames(ops []DiffOperation) []string {
	names := make([]string, len(ops))
	for i, op := range ops {
		names[i] = op.IndexName
	}
	return names
}

func contains(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}

func floatPtr(f float64) *float64 {
	return &f
}
