package content

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wangling-miao/aroute/core"
	"github.com/wangling-miao/aroute/core/ddl"
	"github.com/wangling-miao/aroute/core/events"
	"github.com/wangling-miao/aroute/plugins/database"
	"github.com/wangling-miao/aroute/sdk/interfaces"
)

var testCounter int64

func nextTestDBName() string {
	n := atomic.AddInt64(&testCounter, 1)
	return fmt.Sprintf("content_test_%d", n)
}

func setupTestService(t *testing.T) *Service {
	t.Helper()

	dbName := nextTestDBName()
	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s?mode=memory&cache=shared&_pragma=foreign_keys(ON)", dbName))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	dbSvc := database.NewService(db, database.DriverSQLite)
	store := NewStore(dbSvc)

	ctx := context.Background()
	if err := store.CreateTables(ctx); err != nil {
		t.Fatalf("create tables: %v", err)
	}

	eb := &events.EventBus{}
	svc := NewService(store, eb, slog.Default())

	if err := svc.InitializeBuiltInContentTypes(ctx); err != nil {
		t.Fatalf("init built-in content types: %v", err)
	}

	return svc
}

func TestInitializeBuiltInContentTypes(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	builtins := []string{"page", "post", "category", "tag"}
	for _, name := range builtins {
		ct, err := svc.GetContentType(ctx, name)
		if err != nil {
			t.Fatalf("expected content type '%s' to exist, got error: %v", name, err)
		}
		if ct.Name != name {
			t.Errorf("expected name %s, got %s", name, ct.Name)
		}
		if ct.TableName == "" {
			t.Errorf("content type '%s' has empty table name", name)
		}
	}

	if err := svc.InitializeBuiltInContentTypes(ctx); err != nil {
		t.Fatalf("re-initialize built-in types should be idempotent: %v", err)
	}
}

func TestCreateAndGetContent(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	created, err := svc.Create(ctx, "page", map[string]interface{}{
		"title": "Hello World",
		"body":  "<p>Test content</p>",
	})
	if err != nil {
		t.Fatalf("create page: %v", err)
	}

	if created.ID == "" {
		t.Fatal("expected non-empty ID")
	}
	if created.ContentType != "page" {
		t.Errorf("expected content_type=page, got %s", created.ContentType)
	}
	if created.Title != "Hello World" {
		t.Errorf("expected title 'Hello World', got %s", created.Title)
	}
	if created.Slug == "" {
		t.Fatal("expected auto-generated slug")
	}
	if created.Status != "draft" {
		t.Errorf("expected default status 'draft', got %s", created.Status)
	}
	if created.Version != 1 {
		t.Errorf("expected version 1, got %d", created.Version)
	}

	got, err := svc.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("get by id: %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("expected ID %s, got %s", created.ID, got.ID)
	}
}

func TestUpdateContent(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	created, err := svc.Create(ctx, "page", map[string]interface{}{
		"title": "Original Title",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	updated, err := svc.Update(ctx, created.ID, map[string]interface{}{
		"title": "Updated Title",
		"body":  "<p>Updated content</p>",
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}

	if updated.Title != "Updated Title" {
		t.Errorf("expected title 'Updated Title', got %s", updated.Title)
	}
	if updated.Version != 2 {
		t.Errorf("expected version 2, got %d", updated.Version)
	}

	got, err := svc.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("get after update: %v", err)
	}
	if got.Version != 2 {
		t.Errorf("expected persisted version 2, got %d", got.Version)
	}
}

func TestDeleteContent(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	created, err := svc.Create(ctx, "page", map[string]interface{}{
		"title": "To Be Deleted",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := svc.Delete(ctx, created.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	_, err = svc.GetByID(ctx, created.ID)
	if err != interfaces.ErrNotFound {
		t.Errorf("expected ErrNotFound after delete, got: %v", err)
	}
}

func TestListContent(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		_, err := svc.Create(ctx, "page", map[string]interface{}{
			"title": fmt.Sprintf("Page %d", i),
		})
		if err != nil {
			t.Fatalf("create page %d: %v", i, err)
		}
	}

	page, err := svc.List(ctx, "page", &interfaces.ListQuery{
		Page:    1,
		PerPage: 3,
	})
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	items, ok := page.Data.([]map[string]interface{})
	if !ok {
		t.Fatal("expected []map[string]interface{}")
	}
	if len(items) != 3 {
		t.Errorf("expected 3 items, got %d", len(items))
	}
	if page.Meta.Total != 5 {
		t.Errorf("expected total 5, got %d", page.Meta.Total)
	}
	if !page.Meta.HasNext {
		t.Error("expected HasNext=true")
	}
	if page.Meta.HasPrev {
		t.Error("expected HasPrev=false on first page")
	}
}

func TestListContentPagination(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		_, err := svc.Create(ctx, "page", map[string]interface{}{
			"title": fmt.Sprintf("Page %d", i),
		})
		if err != nil {
			t.Fatalf("create: %v", err)
		}
	}

	page2, err := svc.List(ctx, "page", &interfaces.ListQuery{
		Page:    2,
		PerPage: 3,
	})
	if err != nil {
		t.Fatalf("list page 2: %v", err)
	}

	items2, ok := page2.Data.([]map[string]interface{})
	if !ok {
		t.Fatal("expected []map[string]interface{}")
	}
	if len(items2) != 2 {
		t.Errorf("expected 2 items on page 2, got %d", len(items2))
	}
	if !page2.Meta.HasPrev {
		t.Error("expected HasPrev=true on page 2")
	}
}

func TestListContentFilter(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	_, err := svc.Create(ctx, "page", map[string]interface{}{
		"title":  "Published Page",
		"status": "published",
	})
	if err != nil {
		t.Fatalf("create published: %v", err)
	}

	_, err = svc.Create(ctx, "page", map[string]interface{}{
		"title": "Draft Page",
	})
	if err != nil {
		t.Fatalf("create draft: %v", err)
	}

	page, err := svc.List(ctx, "page", &interfaces.ListQuery{
		Filters: map[string]interface{}{"status": "published"},
	})
	if err != nil {
		t.Fatalf("list with filter: %v", err)
	}

	items, ok := page.Data.([]map[string]interface{})
	if !ok {
		t.Fatal("expected []map[string]interface{}")
	}
	if page.Meta.Total != 1 {
		t.Errorf("expected 1 published page, got %d", page.Meta.Total)
	}
	if len(items) != 1 {
		t.Errorf("expected 1 item, got %d", len(items))
	}
}

func TestDraftPublishWorkflow(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	created, err := svc.Create(ctx, "post", map[string]interface{}{
		"title": "My Blog Post",
		"body":  "<p>Content</p>",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.Status != "draft" {
		t.Errorf("expected initial status draft, got %s", created.Status)
	}

	published, err := svc.Update(ctx, created.ID, map[string]interface{}{
		"title":  "My Blog Post",
		"status": "published",
	})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if published.Status != "published" {
		t.Errorf("expected status published, got %s", published.Status)
	}
	if published.PublishedAt == nil {
		t.Error("expected published_at to be set")
	}
}

func TestVersionHistory(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	created, err := svc.Create(ctx, "page", map[string]interface{}{
		"title": "V1 Title",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	_, err = svc.Update(ctx, created.ID, map[string]interface{}{
		"title": "V2 Title",
	})
	if err != nil {
		t.Fatalf("update v2: %v", err)
	}

	_, err = svc.Update(ctx, created.ID, map[string]interface{}{
		"title": "V3 Title",
	})
	if err != nil {
		t.Fatalf("update v3: %v", err)
	}

	got, err := svc.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Version != 3 {
		t.Errorf("expected version 3, got %d", got.Version)
	}
}

func TestSlugGeneration(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Hello World", "hello-world"},
		{"Hello World!", "hello-world"},
		{"  Multiple   Spaces  ", "multiple-spaces"},
		{"UPPERCASE Title", "uppercase-title"},
		{"Special #@$ Characters!", "special-characters"},
		{"Hello-World", "hello-world"},
		{"a & b | c", "a-b-c"},
		{"  ", ""},
		{"Test123", "test123"},
	}

	for _, tt := range tests {
		got := GenerateSlug(tt.input)
		if got != tt.expected {
			t.Errorf("GenerateSlug(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestUniqueSlugGeneration(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	ct, err := svc.GetContentType(ctx, "page")
	if err != nil {
		t.Fatalf("get content type: %v", err)
	}

	slug1, err := svc.GenerateUniqueSlug(ctx, ct, "Hello World")
	if err != nil {
		t.Fatalf("generate unique slug 1: %v", err)
	}
	if slug1 != "hello-world" {
		t.Errorf("expected 'hello-world', got %q", slug1)
	}

	_, err = svc.Create(ctx, "page", map[string]interface{}{
		"title": "Hello World",
	})
	if err != nil {
		t.Fatalf("create page with slug: %v", err)
	}

	slug2, err := svc.GenerateUniqueSlug(ctx, ct, "Hello World")
	if err != nil {
		t.Fatalf("generate unique slug 2: %v", err)
	}
	if slug2 != "hello-world-2" {
		t.Errorf("expected 'hello-world-2', got %q", slug2)
	}
}

func TestFieldValidationRequired(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	_, err := svc.Create(ctx, "page", map[string]interface{}{})
	if err == nil {
		t.Fatal("expected validation error for missing required fields")
	}

	verrs, ok := err.(*interfaces.ValidationErrors)
	if !ok {
		t.Fatalf("expected *ValidationErrors, got %T", err)
	}

	found := false
	for _, ve := range verrs.Errors {
		if ve.Field == "title" && ve.Code == "required" {
			found = true
		}
	}
	if !found {
		t.Error("expected required validation error for 'title'")
	}
}

func TestFieldValidationMinMaxLength(t *testing.T) {
	v := &FieldValidator{}
	ct := &interfaces.ContentType{
		Name: "test",
		Fields: []interfaces.Field{
			{
				Name:            "name",
				Type:            "text",
				Required:        true,
				ValidationRules: map[string]interface{}{"minLength": float64(3), "maxLength": float64(10)},
			},
		},
	}

	err := v.Validate(context.Background(), ct, map[string]interface{}{
		"name": "ab",
	})
	if err == nil {
		t.Fatal("expected min length validation error")
	}

	err = v.Validate(context.Background(), ct, map[string]interface{}{
		"name": "this is way too long",
	})
	if err == nil {
		t.Fatal("expected max length validation error")
	}

	err = v.Validate(context.Background(), ct, map[string]interface{}{
		"name": "valid",
	})
	if err != nil {
		t.Fatalf("expected valid, got: %v", err)
	}
}

func TestFieldValidationPattern(t *testing.T) {
	v := &FieldValidator{}
	ct := &interfaces.ContentType{
		Name: "test",
		Fields: []interfaces.Field{
			{
				Name:     "code",
				Type:     "text",
				Required: true,
				ValidationRules: map[string]interface{}{
					"pattern": "^[A-Z]{3}$",
				},
			},
		},
	}

	err := v.Validate(context.Background(), ct, map[string]interface{}{
		"code": "abc",
	})
	if err == nil {
		t.Fatal("expected pattern validation error")
	}

	err = v.Validate(context.Background(), ct, map[string]interface{}{
		"code": "ABC",
	})
	if err != nil {
		t.Fatalf("expected valid, got: %v", err)
	}
}

func TestFieldValidationEnum(t *testing.T) {
	v := &FieldValidator{}
	ct := &interfaces.ContentType{
		Name: "test",
		Fields: []interfaces.Field{
			{
				Name:     "status",
				Type:     "enum",
				Required: true,
				ValidationRules: map[string]interface{}{
					"enum": []string{"draft", "published", "archived"},
				},
			},
		},
	}

	err := v.Validate(context.Background(), ct, map[string]interface{}{
		"status": "invalid",
	})
	if err == nil {
		t.Fatal("expected enum validation error")
	}

	err = v.Validate(context.Background(), ct, map[string]interface{}{
		"status": "draft",
	})
	if err != nil {
		t.Fatalf("expected valid, got: %v", err)
	}
}

func TestFieldValidationEmail(t *testing.T) {
	v := &FieldValidator{}
	ct := &interfaces.ContentType{
		Name: "test",
		Fields: []interfaces.Field{
			{Name: "email", Type: "email", Required: true},
		},
	}

	err := v.Validate(context.Background(), ct, map[string]interface{}{
		"email": "not-an-email",
	})
	if err == nil {
		t.Fatal("expected email validation error")
	}

	err = v.Validate(context.Background(), ct, map[string]interface{}{
		"email": "user@example.com",
	})
	if err != nil {
		t.Fatalf("expected valid, got: %v", err)
	}
}

func TestFieldValidationURL(t *testing.T) {
	v := &FieldValidator{}
	ct := &interfaces.ContentType{
		Name: "test",
		Fields: []interfaces.Field{
			{Name: "website", Type: "url", Required: true},
		},
	}

	err := v.Validate(context.Background(), ct, map[string]interface{}{
		"website": "not-a-url",
	})
	if err == nil {
		t.Fatal("expected url validation error")
	}

	err = v.Validate(context.Background(), ct, map[string]interface{}{
		"website": "https://example.com",
	})
	if err != nil {
		t.Fatalf("expected valid, got: %v", err)
	}
}

func TestFieldValidationColor(t *testing.T) {
	v := &FieldValidator{}
	ct := &interfaces.ContentType{
		Name: "test",
		Fields: []interfaces.Field{
			{Name: "color", Type: "color", Required: true},
		},
	}

	err := v.Validate(context.Background(), ct, map[string]interface{}{
		"color": "red",
	})
	if err == nil {
		t.Fatal("expected color validation error")
	}

	err = v.Validate(context.Background(), ct, map[string]interface{}{
		"color": "#FF0000",
	})
	if err != nil {
		t.Fatalf("expected valid, got: %v", err)
	}
}

func TestFieldValidationNumber(t *testing.T) {
	v := &FieldValidator{}
	ct := &interfaces.ContentType{
		Name: "test",
		Fields: []interfaces.Field{
			{
				Name:     "age",
				Type:     "number",
				Required: true,
				ValidationRules: map[string]interface{}{
					"min": float64(0),
					"max": float64(150),
				},
			},
		},
	}

	err := v.Validate(context.Background(), ct, map[string]interface{}{
		"age": -1,
	})
	if err == nil {
		t.Fatal("expected min validation error")
	}

	err = v.Validate(context.Background(), ct, map[string]interface{}{
		"age": 200,
	})
	if err == nil {
		t.Fatal("expected max validation error")
	}

	err = v.Validate(context.Background(), ct, map[string]interface{}{
		"age": 25,
	})
	if err != nil {
		t.Fatalf("expected valid, got: %v", err)
	}
}

func TestFieldValidationSlug(t *testing.T) {
	v := &FieldValidator{}
	ct := &interfaces.ContentType{
		Name: "test",
		Fields: []interfaces.Field{
			{Name: "slug", Type: "slug", Required: true},
		},
	}

	err := v.Validate(context.Background(), ct, map[string]interface{}{
		"slug": "INVALID SLUG!",
	})
	if err == nil {
		t.Fatal("expected slug validation error")
	}

	err = v.Validate(context.Background(), ct, map[string]interface{}{
		"slug": "valid-slug-123",
	})
	if err != nil {
		t.Fatalf("expected valid, got: %v", err)
	}
}

func TestFieldValidationDate(t *testing.T) {
	v := &FieldValidator{}
	ct := &interfaces.ContentType{
		Name: "test",
		Fields: []interfaces.Field{
			{Name: "born", Type: "date", Required: true},
		},
	}

	err := v.Validate(context.Background(), ct, map[string]interface{}{
		"born": "not-a-date",
	})
	if err == nil {
		t.Fatal("expected date validation error")
	}

	err = v.Validate(context.Background(), ct, map[string]interface{}{
		"born": "2024-01-15",
	})
	if err != nil {
		t.Fatalf("expected valid, got: %v", err)
	}
}

func TestFieldValidationJSON(t *testing.T) {
	v := &FieldValidator{}
	ct := &interfaces.ContentType{
		Name: "test",
		Fields: []interfaces.Field{
			{Name: "metadata", Type: "json"},
		},
	}

	err := v.Validate(context.Background(), ct, map[string]interface{}{
		"metadata": "{invalid json",
	})
	if err == nil {
		t.Fatal("expected json validation error")
	}

	err = v.Validate(context.Background(), ct, map[string]interface{}{
		"metadata": map[string]interface{}{"key": "value"},
	})
	if err != nil {
		t.Fatalf("expected valid map, got: %v", err)
	}
}

func TestFieldValidationMultipleErrors(t *testing.T) {
	v := &FieldValidator{}
	ct := &interfaces.ContentType{
		Name: "test",
		Fields: []interfaces.Field{
			{Name: "title", Type: "text", Required: true},
			{Name: "slug", Type: "slug", Required: true},
			{Name: "email", Type: "email", Required: true},
		},
	}

	err := v.Validate(context.Background(), ct, map[string]interface{}{})
	if err == nil {
		t.Fatal("expected validation error")
	}

	verrs, ok := err.(*interfaces.ValidationErrors)
	if !ok {
		t.Fatalf("expected *ValidationErrors, got %T", err)
	}
	if len(verrs.Errors) != 3 {
		t.Errorf("expected 3 validation errors, got %d", len(verrs.Errors))
	}
}

func TestCreateContentType(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	ct, err := svc.CreateContentType(ctx, &interfaces.ContentType{
		Name:        "product",
		DisplayName: "Product",
		Description: "A product",
		Fields: []interfaces.Field{
			{Name: "name", DisplayName: "Name", Type: "text", Required: true},
			{Name: "price", DisplayName: "Price", Type: "number"},
		},
	})
	if err != nil {
		t.Fatalf("create content type: %v", err)
	}

	if ct.TableName != "content_products" {
		t.Errorf("expected table name 'content_products', got %s", ct.TableName)
	}

	got, err := svc.GetContentType(ctx, "product")
	if err != nil {
		t.Fatalf("get content type: %v", err)
	}
	if got.Name != "product" {
		t.Errorf("expected name 'product', got %s", got.Name)
	}

	_, err = svc.Create(ctx, "product", map[string]interface{}{
		"name":  "Widget",
		"price": 9.99,
	})
	if err != nil {
		t.Fatalf("create product: %v", err)
	}
}

func TestUpdateContentType(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	_, err := svc.CreateContentType(ctx, &interfaces.ContentType{
		Name:        "article",
		DisplayName: "Article",
		Fields: []interfaces.Field{
			{Name: "title", Type: "text", Required: true},
		},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	updated, err := svc.UpdateContentType(ctx, "article", &interfaces.ContentType{
		DisplayName: "Blog Article",
		Description: "Updated description",
		Fields: []interfaces.Field{
			{Name: "title", Type: "text", Required: true},
			{Name: "subtitle", Type: "text"},
		},
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.DisplayName != "Blog Article" {
		t.Errorf("expected display name 'Blog Article', got %s", updated.DisplayName)
	}
}

func TestDeleteContentType(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	_, err := svc.CreateContentType(ctx, &interfaces.ContentType{
		Name:        "temp",
		DisplayName: "Temporary",
		Fields: []interfaces.Field{
			{Name: "name", Type: "text", Required: true},
		},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := svc.DeleteContentType(ctx, "temp"); err != nil {
		t.Fatalf("delete: %v", err)
	}

	_, err = svc.GetContentType(ctx, "temp")
	if err == nil {
		t.Fatal("expected error getting deleted content type")
	}
}

func TestRelationField(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	_, err := svc.Create(ctx, "category", map[string]interface{}{
		"name": "Technology",
	})
	if err != nil {
		t.Fatalf("create category: %v", err)
	}

	pages, err := svc.List(ctx, "category", &interfaces.ListQuery{})
	if err != nil {
		t.Fatalf("list categories: %v", err)
	}
	items, ok := pages.Data.([]map[string]interface{})
	if !ok || len(items) != 1 {
		t.Fatalf("expected 1 category")
	}
	if items[0]["name"] != "Technology" {
		t.Errorf("expected name 'Technology', got %v", items[0]["name"])
	}
}

func TestSoftDeleteExcludesFromList(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	_, err := svc.Create(ctx, "page", map[string]interface{}{
		"title": "Page 1",
	})
	if err != nil {
		t.Fatalf("create page 1: %v", err)
	}

	p2, err := svc.Create(ctx, "page", map[string]interface{}{
		"title": "Page 2",
	})
	if err != nil {
		t.Fatalf("create page 2: %v", err)
	}

	if err := svc.Delete(ctx, p2.ID); err != nil {
		t.Fatalf("delete page 2: %v", err)
	}

	page, err := svc.List(ctx, "page", &interfaces.ListQuery{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if page.Meta.Total != 1 {
		t.Errorf("expected 1 page after soft delete, got %d", page.Meta.Total)
	}
}

func TestCreatePost(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	post, err := svc.Create(ctx, "post", map[string]interface{}{
		"title":   "My First Post",
		"body":    "<p>Content here</p>",
		"excerpt": "First post excerpt",
	})
	if err != nil {
		t.Fatalf("create post: %v", err)
	}
	if post.ContentType != "post" {
		t.Errorf("expected content_type=post, got %s", post.ContentType)
	}
	if post.Slug != "my-first-post" {
		t.Errorf("expected slug 'my-first-post', got %s", post.Slug)
	}
}

func TestCreateTag(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	tag, err := svc.Create(ctx, "tag", map[string]interface{}{
		"name": "Go Programming",
	})
	if err != nil {
		t.Fatalf("create tag: %v", err)
	}
	if tag.Slug != "go-programming" {
		t.Errorf("expected slug 'go-programming', got %s", tag.Slug)
	}
}

func TestGetByIDNotFound(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	_, err := svc.GetByID(ctx, "nonexistent-id")
	if err != interfaces.ErrNotFound {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

func TestAutoSlugFromName(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	tag, err := svc.Create(ctx, "tag", map[string]interface{}{
		"name": "Test Tag",
	})
	if err != nil {
		t.Fatalf("create tag: %v", err)
	}
	if tag.Slug != "test-tag" {
		t.Errorf("expected auto-generated slug 'test-tag', got %s", tag.Slug)
	}
}

func TestFieldValidationBoolean(t *testing.T) {
	v := &FieldValidator{}
	ctx := context.Background()

	tests := []struct {
		name    string
		value   interface{}
		wantErr bool
	}{
		{"bool true", true, false},
		{"bool false", false, false},
		{"string true", "true", false},
		{"string false", "false", false},
		{"string 1", "1", false},
		{"string 0", "0", false},
		{"string yes", "yes", true},
		{"string no", "no", true},
		{"int value", 123, false},
		{"float64 value", float64(1.0), false},
		{"int64 value", int64(1), false},
		{"invalid string", "maybe", true},
		{"nil value", nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ct := &interfaces.ContentType{
				Name: "test",
				Fields: []interfaces.Field{
					{Name: "active", Type: "boolean", Required: true},
				},
			}
			data := map[string]interface{}{"active": tt.value}
			err := v.Validate(ctx, ct, data)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate(active=%v) error = %v, wantErr %v", tt.value, err, tt.wantErr)
			}
		})
	}
}

func TestFieldValidationDatetime(t *testing.T) {
	v := &FieldValidator{}
	ctx := context.Background()

	tests := []struct {
		name    string
		value   interface{}
		wantErr bool
	}{
		{"valid ISO datetime with T", "2024-01-15T10:30:00Z", false},
		{"valid ISO datetime with space", "2024-01-15 10:30:00", false},
		{"invalid no T or space", "not-a-datetime", true},
		{"non-string value", 12345, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ct := &interfaces.ContentType{
				Name: "test",
				Fields: []interfaces.Field{
					{Name: "published_at", Type: "datetime", Required: true},
				},
			}
			data := map[string]interface{}{"published_at": tt.value}
			err := v.Validate(ctx, ct, data)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate(published_at=%v) error = %v, wantErr %v", tt.value, err, tt.wantErr)
			}
		})
	}
}

func TestFieldValidationMedia(t *testing.T) {
	v := &FieldValidator{}
	ctx := context.Background()

	tests := []struct {
		name    string
		value   interface{}
		wantErr bool
	}{
		{"valid URL string", "https://example.com/image.png", false},
		{"empty string not required", "", false},
		{"nil value", nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ct := &interfaces.ContentType{
				Name: "test",
				Fields: []interfaces.Field{
					{Name: "image", Type: "media"},
				},
			}
			data := map[string]interface{}{"image": tt.value}
			err := v.Validate(ctx, ct, data)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate(image=%v) error = %v, wantErr %v", tt.value, err, tt.wantErr)
			}
		})
	}
}

func TestFieldValidationRelation(t *testing.T) {
	v := &FieldValidator{}
	ctx := context.Background()

	tests := []struct {
		name           string
		relationConfig *interfaces.RelationConfig
		wantErr        bool
		errCode        string
	}{
		{
			name:           "nil relation config",
			relationConfig: nil,
			wantErr:        false,
		},
		{
			name: "empty target content type",
			relationConfig: &interfaces.RelationConfig{
				TargetContentType: "",
			},
			wantErr: true,
			errCode: "invalid_relation",
		},
		{
			name: "valid relation",
			relationConfig: &interfaces.RelationConfig{
				TargetContentType: "category",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ct := &interfaces.ContentType{
				Name: "test",
				Fields: []interfaces.Field{
					{
						Name:           "parent",
						Type:           "relation",
						RelationConfig: tt.relationConfig,
					},
				},
			}
			data := map[string]interface{}{"parent": "some-id"}
			err := v.Validate(ctx, ct, data)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && err != nil {
				verrs, ok := err.(*interfaces.ValidationErrors)
				if !ok {
					t.Fatalf("expected *ValidationErrors, got %T", err)
				}
				if len(verrs.Errors) == 0 || verrs.Errors[0].Code != tt.errCode {
					t.Errorf("expected error code %s, got %v", tt.errCode, verrs.Errors)
				}
			}
		})
	}
}

func TestFieldValidationMarkdown(t *testing.T) {
	v := &FieldValidator{}
	ctx := context.Background()

	ct := &interfaces.ContentType{
		Name: "test",
		Fields: []interfaces.Field{
			{Name: "content", Type: "markdown", Required: true},
		},
	}

	err := v.Validate(ctx, ct, map[string]interface{}{
		"content": "# Hello World",
	})
	if err != nil {
		t.Fatalf("expected valid markdown, got: %v", err)
	}

	err = v.Validate(ctx, ct, map[string]interface{}{
		"content": 123,
	})
	if err == nil {
		t.Fatal("expected type mismatch error for non-string markdown")
	}
}

func TestFieldValidationRichtext(t *testing.T) {
	v := &FieldValidator{}
	ctx := context.Background()

	ct := &interfaces.ContentType{
		Name: "test",
		Fields: []interfaces.Field{
			{Name: "body", Type: "richtext", Required: true},
		},
	}

	err := v.Validate(ctx, ct, map[string]interface{}{
		"body": "<p>Hello</p>",
	})
	if err != nil {
		t.Fatalf("expected valid richtext, got: %v", err)
	}
}

func TestToInt(t *testing.T) {
	tests := []struct {
		name    string
		input   interface{}
		want    int
		wantErr bool
	}{
		{"int", 42, 42, false},
		{"int64", int64(99), 99, false},
		{"float64", float64(3.14), 3, false},
		{"string valid", "42", 42, false},
		{"string invalid", "not-a-number", 0, true},
		{"unsupported type", []int{1, 2}, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := toInt(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("toInt(%v) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("toInt(%v) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestToFloat64(t *testing.T) {
	tests := []struct {
		name   string
		input  interface{}
		want   float64
		wantOK bool
	}{
		{"int", 42, 42.0, true},
		{"int64", int64(100), 100.0, true},
		{"float64", 3.14, 3.14, true},
		{"string valid", "3.14", 3.14, true},
		{"string integer", "42", 42.0, true},
		{"string invalid", "not-a-float", 0, false},
		{"unsupported type", []int{1}, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := toFloat64(tt.input)
			if ok != tt.wantOK {
				t.Errorf("toFloat64(%v) ok = %v, want %v", tt.input, ok, tt.wantOK)
			}
			if tt.wantOK && got != tt.want {
				t.Errorf("toFloat64(%v) = %f, want %f", tt.input, got, tt.want)
			}
		})
	}
}

func TestFieldValidationTypeMismatches(t *testing.T) {
	v := &FieldValidator{}
	ctx := context.Background()

	tests := []struct {
		name      string
		fieldType string
		value     interface{}
		wantErr   bool
	}{
		{"text non-string", "text", 123, true},
		{"number non-numeric", "number", "abc", true},
		{"date non-string", "date", 12345, true},
		{"datetime non-string", "datetime", 12345, true},
		{"email non-string", "email", 12345, true},
		{"url non-string", "url", 12345, true},
		{"slug non-string", "slug", 12345, true},
		{"enum non-string", "enum", 12345, true},
		{"color non-string", "color", 12345, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ct := &interfaces.ContentType{
				Name: "test",
				Fields: []interfaces.Field{
					{Name: "field", Type: tt.fieldType, Required: true},
				},
			}
			data := map[string]interface{}{"field": tt.value}
			err := v.Validate(ctx, ct, data)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate(field=%v, type=%s) error = %v, wantErr %v", tt.value, tt.fieldType, err, tt.wantErr)
			}
		})
	}
}

func TestIsEmpty(t *testing.T) {
	tests := []struct {
		name  string
		value interface{}
		want  bool
	}{
		{"nil", nil, true},
		{"empty string", "", true},
		{"non-empty string", "hello", false},
		{"empty slice", []interface{}{}, true},
		{"non-empty slice", []interface{}{1}, false},
		{"empty map", map[string]interface{}{}, true},
		{"non-empty map", map[string]interface{}{"a": 1}, false},
		{"int zero", 0, false},
		{"bool false", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isEmpty(tt.value)
			if got != tt.want {
				t.Errorf("isEmpty(%v) = %v, want %v", tt.value, got, tt.want)
			}
		})
	}
}

func TestFieldValidationEnumEdgeCases(t *testing.T) {
	v := &FieldValidator{}
	ctx := context.Background()

	t.Run("enum with []interface{}", func(t *testing.T) {
		ct := &interfaces.ContentType{
			Name: "test",
			Fields: []interfaces.Field{
				{
					Name:     "status",
					Type:     "enum",
					Required: true,
					ValidationRules: map[string]interface{}{
						"enum": []interface{}{"draft", "published"},
					},
				},
			},
		}
		err := v.Validate(ctx, ct, map[string]interface{}{"status": "draft"})
		if err != nil {
			t.Fatalf("expected valid, got: %v", err)
		}

		err = v.Validate(ctx, ct, map[string]interface{}{"status": "invalid"})
		if err == nil {
			t.Fatal("expected enum validation error")
		}
	})

	t.Run("enum with no validation rules", func(t *testing.T) {
		ct := &interfaces.ContentType{
			Name: "test",
			Fields: []interfaces.Field{
				{Name: "status", Type: "enum", Required: true},
			},
		}
		err := v.Validate(ctx, ct, map[string]interface{}{"status": "anything"})
		if err != nil {
			t.Fatalf("expected no error without validation rules, got: %v", err)
		}
	})

	t.Run("enum with no enum key in rules", func(t *testing.T) {
		ct := &interfaces.ContentType{
			Name: "test",
			Fields: []interfaces.Field{
				{
					Name:     "status",
					Type:     "enum",
					Required: true,
					ValidationRules: map[string]interface{}{
						"other": "value",
					},
				},
			},
		}
		err := v.Validate(ctx, ct, map[string]interface{}{"status": "anything"})
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
	})
}

func TestFieldValidationTextEdgeCases(t *testing.T) {
	v := &FieldValidator{}
	ctx := context.Background()

	t.Run("pattern with empty string", func(t *testing.T) {
		ct := &interfaces.ContentType{
			Name: "test",
			Fields: []interfaces.Field{
				{
					Name:     "code",
					Type:     "text",
					Required: true,
					ValidationRules: map[string]interface{}{
						"pattern": "",
					},
				},
			},
		}
		err := v.Validate(ctx, ct, map[string]interface{}{"code": "anything"})
		if err != nil {
			t.Fatalf("expected valid with empty pattern, got: %v", err)
		}
	})

	t.Run("pattern non-string value", func(t *testing.T) {
		ct := &interfaces.ContentType{
			Name: "test",
			Fields: []interfaces.Field{
				{
					Name:     "code",
					Type:     "text",
					Required: true,
					ValidationRules: map[string]interface{}{
						"pattern": 123,
					},
				},
			},
		}
		err := v.Validate(ctx, ct, map[string]interface{}{"code": "anything"})
		if err != nil {
			t.Fatalf("expected valid with non-string pattern, got: %v", err)
		}
	})

	t.Run("nil validation rules", func(t *testing.T) {
		ct := &interfaces.ContentType{
			Name: "test",
			Fields: []interfaces.Field{
				{Name: "code", Type: "text", Required: true},
			},
		}
		err := v.Validate(ctx, ct, map[string]interface{}{"code": "anything"})
		if err != nil {
			t.Fatalf("expected valid with nil rules, got: %v", err)
		}
	})
}

func TestFieldValidationJSONNonStringTypes(t *testing.T) {
	v := &FieldValidator{}
	ctx := context.Background()

	ct := &interfaces.ContentType{
		Name: "test",
		Fields: []interfaces.Field{
			{Name: "data", Type: "json"},
		},
	}

	// []interface{} should pass without error
	err := v.Validate(ctx, ct, map[string]interface{}{
		"data": []interface{}{1, 2, 3},
	})
	if err != nil {
		t.Fatalf("expected valid for []interface{}, got: %v", err)
	}

	err = v.Validate(ctx, ct, map[string]interface{}{
		"data": 123,
	})
	if err != nil {
		t.Fatalf("expected no error for non-json-compatible type, got: %v", err)
	}
}

func TestGetVersions(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	created, err := svc.Create(ctx, "page", map[string]interface{}{
		"title": "V1",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	_, err = svc.Update(ctx, created.ID, map[string]interface{}{
		"title": "V2",
	})
	if err != nil {
		t.Fatalf("update v2: %v", err)
	}

	_, err = svc.Update(ctx, created.ID, map[string]interface{}{
		"title": "V3",
	})
	if err != nil {
		t.Fatalf("update v3: %v", err)
	}

	versions, err := svc.store.GetVersions(ctx, "page", created.ID, 10, 0)
	if err != nil {
		t.Fatalf("GetVersions: %v", err)
	}
	if len(versions) != 2 {
		t.Errorf("expected 2 versions, got %d", len(versions))
	}
}

func TestGetVersion(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	created, err := svc.Create(ctx, "page", map[string]interface{}{
		"title": "Original",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	_, err = svc.Update(ctx, created.ID, map[string]interface{}{
		"title": "Updated",
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}

	ver, err := svc.store.GetVersion(ctx, "page", created.ID, 1)
	if err != nil {
		t.Fatalf("GetVersion: %v", err)
	}
	if ver["version_number"] == nil {
		t.Error("expected version_number in result")
	}
}

func TestGetVersionNotFound(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	created, err := svc.Create(ctx, "page", map[string]interface{}{
		"title": "Test",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	_, err = svc.store.GetVersion(ctx, "page", created.ID, 999)
	if err != interfaces.ErrNotFound {
		t.Errorf("expected ErrNotFound for non-existent version, got: %v", err)
	}
}

func TestGetNextSlug(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	ct, err := svc.GetContentType(ctx, "page")
	if err != nil {
		t.Fatalf("get content type: %v", err)
	}

	slug, err := svc.store.GetNextSlug(ctx, ct.TableName, "hello-world")
	if err != nil {
		t.Fatalf("GetNextSlug (no existing): %v", err)
	}
	if slug != "hello-world" {
		t.Errorf("expected 'hello-world', got %q", slug)
	}

	// Create a page with slug "hello-world"
	_, err = svc.Create(ctx, "page", map[string]interface{}{
		"title": "Hello World",
	})
	if err != nil {
		t.Fatalf("create page: %v", err)
	}

	slug2, err := svc.store.GetNextSlug(ctx, ct.TableName, "hello-world")
	if err != nil {
		t.Fatalf("GetNextSlug (with existing): %v", err)
	}
	if slug2 != "hello-world-2" {
		t.Errorf("expected 'hello-world-2', got %q", slug2)
	}
}

func TestCreateWithInvalidContentType(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	_, err := svc.Create(ctx, "nonexistent_type", map[string]interface{}{
		"title": "Test",
	})
	if err == nil {
		t.Fatal("expected error creating with invalid content type")
	}
}

func TestListWithInvalidContentType(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	_, err := svc.List(ctx, "nonexistent_type", &interfaces.ListQuery{})
	if err == nil {
		t.Fatal("expected error listing with invalid content type")
	}
}

func TestDeleteNonExistentContent(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	err := svc.Delete(ctx, "nonexistent-id")
	if err == nil {
		t.Fatal("expected error deleting non-existent content")
	}
}

func TestUpdateNonExistentContent(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	_, err := svc.Update(ctx, "nonexistent-id", map[string]interface{}{
		"title": "Test",
	})
	if err == nil {
		t.Fatal("expected error updating non-existent content")
	}
}

func TestListContentNilQuery(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	_, err := svc.Create(ctx, "page", map[string]interface{}{
		"title": "Test Page",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	page, err := svc.List(ctx, "page", nil)
	if err != nil {
		t.Fatalf("list with nil query: %v", err)
	}
	if page.Meta.Total != 1 {
		t.Errorf("expected total 1, got %d", page.Meta.Total)
	}
	if page.Meta.Page != 1 {
		t.Errorf("expected default page 1, got %d", page.Meta.Page)
	}
	if page.Meta.PerPage != 20 {
		t.Errorf("expected default perPage 20, got %d", page.Meta.PerPage)
	}
}

func TestListContentPerPageOverflow(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	_, err := svc.Create(ctx, "page", map[string]interface{}{
		"title": "Test",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	page, err := svc.List(ctx, "page", &interfaces.ListQuery{
		PerPage: 200,
	})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if page.Meta.PerPage != 100 {
		t.Errorf("expected perPage capped to 100, got %d", page.Meta.PerPage)
	}
}

func TestCreateContentTypeEmptyName(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	_, err := svc.CreateContentType(ctx, &interfaces.ContentType{
		Name: "",
	})
	if err == nil {
		t.Fatal("expected error for empty content type name")
	}
}

func TestCreateContentTypeDuplicate(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	_, err := svc.CreateContentType(ctx, &interfaces.ContentType{
		Name:        "dup",
		DisplayName: "First",
		Fields:      []interfaces.Field{{Name: "title", Type: "text", Required: true}},
	})
	if err != nil {
		t.Fatalf("first create: %v", err)
	}

	_, err = svc.CreateContentType(ctx, &interfaces.ContentType{
		Name:        "dup",
		DisplayName: "Second",
		Fields:      []interfaces.Field{{Name: "title", Type: "text", Required: true}},
	})
	if err == nil {
		t.Fatal("expected error for duplicate content type name")
	}
}

func TestUpdateContentTypeNotFound(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	_, err := svc.UpdateContentType(ctx, "nonexistent", &interfaces.ContentType{
		DisplayName: "Test",
	})
	if err == nil {
		t.Fatal("expected error updating non-existent content type")
	}
}

func TestDeleteContentTypeNotFound(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	err := svc.DeleteContentType(ctx, "nonexistent")
	if err == nil {
		t.Fatal("expected error deleting non-existent content type")
	}
}

func TestEmitEventNilEventBus(t *testing.T) {
	store := setupTestService(t).store
	svc := NewService(store, nil, slog.Default())
	ctx := context.Background()

	// Create should not panic with nil event bus
	ct := &interfaces.ContentType{
		Name:        "nilbus_test",
		DisplayName: "NilBus Test",
		TableName:   "content_nilbus_tests",
		Fields:      []interfaces.Field{{Name: "title", Type: "text", Required: true}},
	}
	if err := store.CreateContentType(ctx, ct); err != nil {
		t.Fatalf("create content type: %v", err)
	}
	if err := svc.createContentTable(ctx, ct); err != nil {
		t.Fatalf("create table: %v", err)
	}

	// This exercises emitEvent with nil events
	_, err := svc.Create(ctx, "nilbus_test", map[string]interface{}{
		"title": "Test",
	})
	if err != nil {
		t.Fatalf("create with nil event bus: %v", err)
	}
}

func TestCheckUniqueFields(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	// page has slug with Unique: true
	_, err := svc.Create(ctx, "page", map[string]interface{}{
		"title": "First Page",
	})
	if err != nil {
		t.Fatalf("create first page: %v", err)
	}

	// Create another page with a unique slug - should work fine
	_, err = svc.Create(ctx, "page", map[string]interface{}{
		"title": "Second Page",
		"slug":  "second-page",
	})
	if err != nil {
		t.Fatalf("create second page with unique slug: %v", err)
	}
}

func TestMapFieldTypeToDDL(t *testing.T) {
	tests := []struct {
		fieldType string
		expected  ddl.FieldType
	}{
		{"text", ddl.FieldTypeText},
		{"number", ddl.FieldTypeNumber},
		{"boolean", ddl.FieldTypeBoolean},
		{"date", ddl.FieldTypeDatetime},
		{"datetime", ddl.FieldTypeDatetime},
		{"json", ddl.FieldTypeJSON},
		{"relation", ddl.FieldTypeRelation},
		{"markdown", ddl.FieldTypeText},
		{"richtext", ddl.FieldTypeText},
		{"email", ddl.FieldTypeText},
		{"url", ddl.FieldTypeText},
		{"slug", ddl.FieldTypeText},
		{"enum", ddl.FieldTypeText},
		{"color", ddl.FieldTypeText},
		{"media", ddl.FieldTypeText},
		{"unknown", ddl.FieldTypeText},
	}

	for _, tt := range tests {
		t.Run(tt.fieldType, func(t *testing.T) {
			got := mapFieldTypeToDDL(tt.fieldType)
			if got != tt.expected {
				t.Errorf("mapFieldTypeToDDL(%q) = %q, want %q", tt.fieldType, got, tt.expected)
			}
		})
	}
}

func TestResolveTargetTable(t *testing.T) {
	svc := setupTestService(t)

	table := resolveTargetTable(svc.store, "page")
	if table != "content_pages" {
		t.Errorf("expected 'content_pages', got %q", table)
	}

	table = resolveTargetTable(svc.store, "unknown_type")
	expected := "content_unknown_types"
	if table != expected {
		t.Errorf("expected %q, got %q", expected, table)
	}
}

func TestNilTime(t *testing.T) {
	t.Run("nil time", func(t *testing.T) {
		result := nilTime(nil)
		if result != nil {
			t.Errorf("expected nil, got %v", result)
		}
	})

	t.Run("non-nil time", func(t *testing.T) {
		now := time.Now().UTC()
		result := nilTime(&now)
		s, ok := result.(string)
		if !ok {
			t.Fatalf("expected string, got %T", result)
		}
		if s == "" {
			t.Error("expected non-empty time string")
		}
	})
}

func TestToIntValue(t *testing.T) {
	tests := []struct {
		name       string
		input      interface{}
		defaultVal int
		want       int
	}{
		{"int", 42, 0, 42},
		{"int64", int64(99), 0, 99},
		{"float64", float64(7.5), 0, 7},
		{"json.Number", json.Number("123"), 0, 123},
		{"string uses default", "not-a-number", 5, 5},
		{"nil uses default", nil, 10, 10},
		{"unsupported type uses default", []int{}, 3, 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toIntValue(tt.input, tt.defaultVal)
			if got != tt.want {
				t.Errorf("toIntValue(%v, %d) = %d, want %d", tt.input, tt.defaultVal, got, tt.want)
			}
		})
	}
}

func TestPluginNew(t *testing.T) {
	p := New()
	if p == nil {
		t.Fatal("expected non-nil plugin")
	}
	if p.Name() == "" {
		t.Error("expected non-empty plugin name")
	}
}

func TestAutoGenerateSlugEdgeCases(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	t.Run("slug already provided", func(t *testing.T) {
		ct, _ := svc.GetContentType(ctx, "page")
		data := map[string]interface{}{
			"slug": "custom-slug",
		}
		svc.autoGenerateSlug(ctx, ct, data)
		if data["slug"] != "custom-slug" {
			t.Errorf("expected slug to remain 'custom-slug', got %v", data["slug"])
		}
	})

	t.Run("no title or name", func(t *testing.T) {
		ct, _ := svc.GetContentType(ctx, "page")
		data := map[string]interface{}{}
		svc.autoGenerateSlug(ctx, ct, data)
		if _, hasSlug := data["slug"]; hasSlug {
			t.Error("expected no slug generated when no title/name")
		}
	})

	t.Run("name used as fallback", func(t *testing.T) {
		ct, _ := svc.GetContentType(ctx, "tag")
		data := map[string]interface{}{
			"name": "Test Tag Name",
		}
		svc.autoGenerateSlug(ctx, ct, data)
		if data["slug"] != "test-tag-name" {
			t.Errorf("expected slug from name, got %v", data["slug"])
		}
	})

	t.Run("title used, sets title in data", func(t *testing.T) {
		ct, _ := svc.GetContentType(ctx, "page")
		data := map[string]interface{}{
			"title": "My Page Title",
		}
		svc.autoGenerateSlug(ctx, ct, data)
		if data["slug"] != "my-page-title" {
			t.Errorf("expected slug 'my-page-title', got %v", data["slug"])
		}
	})
}

func TestMergeForUpdate(t *testing.T) {
	svc := setupTestService(t)

	current := map[string]interface{}{
		"title":   "Old Title",
		"version": float64(1),
		"status":  "draft",
	}

	updates := map[string]interface{}{
		"title":  "New Title",
		"status": "published",
	}

	merged := svc.mergeForUpdate(current, updates)
	if merged["title"] != "New Title" {
		t.Errorf("expected 'New Title', got %v", merged["title"])
	}
	if merged["version"] != float64(1) {
		t.Errorf("expected version 1, got %v", merged["version"])
	}
	if merged["status"] != "published" {
		t.Errorf("expected 'published', got %v", merged["status"])
	}
}

func TestIsSystemField(t *testing.T) {
	svc := setupTestService(t)

	systemFields := []string{"id", "content_type", "title", "slug", "created_by",
		"updated_by", "status", "published_at", "created_at", "updated_at", "deleted_at", "version"}

	for _, f := range systemFields {
		if !svc.isSystemField(f) {
			t.Errorf("expected %q to be a system field", f)
		}
	}

	if svc.isSystemField("custom_field") {
		t.Error("expected 'custom_field' to NOT be a system field")
	}
}

func TestCreateContentTypeWithVariousFields(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	_, err := svc.CreateContentType(ctx, &interfaces.ContentType{
		Name:        "event",
		DisplayName: "Event",
		Fields: []interfaces.Field{
			{Name: "title", Type: "text", Required: true},
			{Name: "active", Type: "boolean"},
			{Name: "event_date", Type: "datetime"},
			{Name: "image", Type: "media"},
			{Name: "content", Type: "markdown"},
			{Name: "notes", Type: "richtext"},
			{Name: "metadata", Type: "json"},
			{Name: "priority", Type: "number"},
			{Name: "code", Type: "slug"},
		},
	})
	if err != nil {
		t.Fatalf("create content type with various fields: %v", err)
	}

	created, err := svc.Create(ctx, "event", map[string]interface{}{
		"title":      "Test Event",
		"active":     true,
		"event_date": "2024-06-15T10:00:00Z",
		"priority":   5,
	})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}
	if created.ContentType != "event" {
		t.Errorf("expected content_type=event, got %s", created.ContentType)
	}
}

func TestSnapshotVersionViaUpdate(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	created, err := svc.Create(ctx, "page", map[string]interface{}{
		"title": "Snapshot Test",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Update creates a version snapshot
	_, err = svc.Update(ctx, created.ID, map[string]interface{}{
		"title": "Snapshot Updated",
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}

	// Verify the snapshot was created
	versions, err := svc.store.GetVersions(ctx, "page", created.ID, 10, 0)
	if err != nil {
		t.Fatalf("GetVersions: %v", err)
	}
	if len(versions) != 1 {
		t.Errorf("expected 1 version snapshot, got %d", len(versions))
	}
}

func TestCreateWithExplicitStatus(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	created, err := svc.Create(ctx, "page", map[string]interface{}{
		"title":  "Published Page",
		"status": "published",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.Status != "published" {
		t.Errorf("expected status 'published', got %s", created.Status)
	}
}

func TestGetByIDWithMultipleContentTypes(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	page, err := svc.Create(ctx, "page", map[string]interface{}{
		"title": "Test Page",
	})
	if err != nil {
		t.Fatalf("create page: %v", err)
	}

	post, err := svc.Create(ctx, "post", map[string]interface{}{
		"title": "Test Post",
	})
	if err != nil {
		t.Fatalf("create post: %v", err)
	}

	gotPage, err := svc.GetByID(ctx, page.ID)
	if err != nil {
		t.Fatalf("get page: %v", err)
	}
	if gotPage.ContentType != "page" {
		t.Errorf("expected content_type=page, got %s", gotPage.ContentType)
	}

	gotPost, err := svc.GetByID(ctx, post.ID)
	if err != nil {
		t.Fatalf("get post: %v", err)
	}
	if gotPost.ContentType != "post" {
		t.Errorf("expected content_type=post, got %s", gotPost.ContentType)
	}
}

func TestListContentSortOrder(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	_, err := svc.Create(ctx, "page", map[string]interface{}{"title": "Alpha"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.Create(ctx, "page", map[string]interface{}{"title": "Beta"})
	if err != nil {
		t.Fatal(err)
	}

	page, err := svc.List(ctx, "page", &interfaces.ListQuery{
		Sort:    "title",
		Order:   "asc",
		PerPage: 10,
	})
	if err != nil {
		t.Fatalf("list sorted: %v", err)
	}

	items, ok := page.Data.([]map[string]interface{})
	if !ok {
		t.Fatal("expected []map[string]interface{}")
	}
	if len(items) < 2 {
		t.Fatalf("expected at least 2 items, got %d", len(items))
	}
	if items[0]["title"] != "Alpha" {
		t.Errorf("expected first item 'Alpha', got %v", items[0]["title"])
	}
}

type contentMockServiceContainer struct {
	dbSvc interfaces.DatabaseService
}

func (m *contentMockServiceContainer) Get(target interface{}) error {
	rv := reflect.ValueOf(target)
	if rv.Kind() != reflect.Ptr {
		return fmt.Errorf("target must be a pointer")
	}
	rv.Elem().Set(reflect.ValueOf(m.dbSvc))
	return nil
}

func (m *contentMockServiceContainer) Provide(_ interface{}) error            { return nil }
func (m *contentMockServiceContainer) GetNamed(_ string, _ interface{}) error { return nil }
func (m *contentMockServiceContainer) Unregister(_ interface{}) error         { return nil }
func (m *contentMockServiceContainer) Has(_ interface{}) bool                 { return true }
func (m *contentMockServiceContainer) Keys() []string                         { return nil }

type contentMockEventBus struct{}

func (m *contentMockEventBus) SubscribeFilter(_ string, _ int, _ events.FilterHandler) string {
	return ""
}
func (m *contentMockEventBus) SubscribeBroadcast(_ string, _ events.BroadcastHandler) string {
	return ""
}
func (m *contentMockEventBus) Emit(_ context.Context, _ events.Event) {}
func (m *contentMockEventBus) DispatchFilter(_ context.Context, event *events.Event) (*events.Event, error) {
	return event, nil
}
func (m *contentMockEventBus) Unsubscribe(_ string) {}

func newContentMockCoreContext(dbSvc interfaces.DatabaseService) core.CoreContext {
	return core.NewCoreContext(
		context.Background(),
		&contentMockServiceContainer{dbSvc: dbSvc},
		&contentMockEventBus{},
		nil,
		slog.Default(),
		"",
		"",
	)
}

func TestPluginInitStartStop(t *testing.T) {
	dbName := nextTestDBName()
	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s?mode=memory&cache=shared&_pragma=foreign_keys(ON)", dbName))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	dbSvc := database.NewService(db, database.DriverSQLite)
	coreCtx := newContentMockCoreContext(dbSvc)

	p := New()

	if err := p.Init(coreCtx); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if err := p.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	if err := p.Start(); err != nil {
		t.Fatalf("Start (already running): %v", err)
	}

	if err := p.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	if err := p.Stop(); err != nil {
		t.Fatalf("Stop (already stopped): %v", err)
	}
}

func TestHardDelete(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	content, err := svc.Create(ctx, "post", map[string]interface{}{
		"title":  "To Hard Delete",
		"status": "draft",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	_, err = svc.Update(ctx, content.ID, map[string]interface{}{
		"title": "Updated Before Delete",
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}

	if err := svc.HardDelete(ctx, content.ID); err != nil {
		t.Fatalf("hard delete: %v", err)
	}

	_, err = svc.GetByID(ctx, content.ID)
	if err == nil {
		t.Fatal("expected not found after hard delete")
	}

	versions, err := svc.store.GetVersions(ctx, "post", content.ID, 10, 0)
	if err != nil {
		t.Fatalf("get versions: %v", err)
	}
	if len(versions) != 0 {
		t.Errorf("expected 0 versions after hard delete, got %d", len(versions))
	}
}

func TestHardDeleteNotFound(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	err := svc.HardDelete(ctx, "nonexistent-id")
	if err == nil {
		t.Fatal("expected error for hard delete of nonexistent content")
	}
}

func TestRestore(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	content, err := svc.Create(ctx, "page", map[string]interface{}{
		"title": "To Restore",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	ct, _ := svc.GetContentType(ctx, "page")

	if err := svc.store.DeleteContent(ctx, ct.TableName, content.ID); err != nil {
		t.Fatalf("soft delete via store: %v", err)
	}

	if err := svc.store.RestoreContent(ctx, ct.TableName, content.ID); err != nil {
		t.Fatalf("restore via store: %v", err)
	}

	if err := svc.Restore(ctx, content.ID); err != nil {
		t.Fatalf("Restore on already-restored should still succeed or be idempotent: %v", err)
	}
}

func TestRestoreNotFound(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	err := svc.Restore(ctx, "nonexistent-id")
	if err == nil {
		t.Fatal("expected error for restore of nonexistent content")
	}
}

func TestValidateFieldName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid simple", "title", false},
		{"valid underscore", "my_field", false},
		{"valid alphanumeric", "field123", false},
		{"valid mixed", "my_field_2", false},
		{"invalid dash", "my-field", true},
		{"invalid space", "my field", true},
		{"invalid special char", "field!", true},
		{"empty string", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateFieldName(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateFieldName(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestJunctionTableName(t *testing.T) {
	svc := setupTestService(t)

	field := interfaces.Field{
		Name: "tags",
		Type: "relation",
		RelationConfig: &interfaces.RelationConfig{
			TargetContentType: "tag",
			RelationType:      "many-to-many",
		},
	}

	got := svc.junctionTableName("content_posts", field)
	if got != "content_posts_tag" {
		t.Errorf("expected 'content_posts_tag', got %q", got)
	}

	fieldWithThrough := interfaces.Field{
		Name: "categories",
		Type: "relation",
		RelationConfig: &interfaces.RelationConfig{
			TargetContentType: "category",
			RelationType:      "many-to-many",
			ThroughTable:      "custom_junction",
		},
	}

	got = svc.junctionTableName("content_posts", fieldWithThrough)
	if got != "custom_junction" {
		t.Errorf("expected 'custom_junction', got %q", got)
	}
}

func TestToSliceOfStrings(t *testing.T) {
	tests := []struct {
		name  string
		input interface{}
		want  []string
	}{
		{"[]string", []string{"a", "b"}, []string{"a", "b"}},
		{"[]interface{} strings", []interface{}{"x", "y"}, []string{"x", "y"}},
		{"[]interface{} mixed", []interface{}{"a", 1, true}, []string{"a", "1", "true"}},
		{"nil", nil, nil},
		{"string", "hello", nil},
		{"int", 42, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toSliceOfStrings(tt.input)
			if len(got) != len(tt.want) {
				t.Fatalf("expected %v, got %v", tt.want, got)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("index %d: expected %q, got %q", i, tt.want[i], got[i])
				}
			}
		})
	}
}

func TestGenerateSlugUnicode(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Hello World", "hello-world"},
		{"Test--Multiple---Dashes", "test-multiple-dashes"},
		{"  trim spaces  ", "trim-spaces"},
		{"中文测试", "中文测试"},
		{"Hello 世界 World", "hello-世界-world"},
		{"UPPERCASE", "uppercase"},
		{"a---b", "a-b"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := GenerateSlugUnicode(tt.input)
			if got != tt.want {
				t.Errorf("GenerateSlugUnicode(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestStoreHardDeleteContent(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	content, err := svc.Create(ctx, "page", map[string]interface{}{
		"title": "Store Hard Delete Test",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	ct, _ := svc.GetContentType(ctx, "page")

	if err := svc.store.HardDeleteContent(ctx, ct.TableName, content.ID); err != nil {
		t.Fatalf("HardDeleteContent: %v", err)
	}

	_, err = svc.store.GetContent(ctx, ct.TableName, content.ID)
	if err == nil {
		t.Error("expected content to be gone after hard delete")
	}
}

func TestStoreHardDeleteContentNotFound(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	ct, _ := svc.GetContentType(ctx, "page")

	err := svc.store.HardDeleteContent(ctx, ct.TableName, "nonexistent-id")
	if err == nil {
		t.Fatal("expected error for hard delete of nonexistent content")
	}
}

func TestStoreRestoreContent(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	content, err := svc.Create(ctx, "page", map[string]interface{}{
		"title": "Restore Test",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	ct, _ := svc.GetContentType(ctx, "page")

	if err := svc.store.DeleteContent(ctx, ct.TableName, content.ID); err != nil {
		t.Fatalf("soft delete: %v", err)
	}

	if err := svc.store.RestoreContent(ctx, ct.TableName, content.ID); err != nil {
		t.Fatalf("RestoreContent: %v", err)
	}

	row, err := svc.store.GetContent(ctx, ct.TableName, content.ID)
	if err != nil {
		t.Fatalf("GetContent after restore: %v", err)
	}
	if row["deleted_at"] != nil {
		t.Errorf("expected deleted_at nil, got %v", row["deleted_at"])
	}
}

func TestStoreRestoreContentNotFound(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	ct, _ := svc.GetContentType(ctx, "page")

	err := svc.store.RestoreContent(ctx, ct.TableName, "nonexistent-id")
	if err == nil {
		t.Fatal("expected error for restore of nonexistent content")
	}
}

func TestDeleteVersionsByContentID(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	content, err := svc.Create(ctx, "page", map[string]interface{}{
		"title": "Version Delete Test",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	_, err = svc.Update(ctx, content.ID, map[string]interface{}{"title": "V2"})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	_, err = svc.Update(ctx, content.ID, map[string]interface{}{"title": "V3"})
	if err != nil {
		t.Fatalf("update: %v", err)
	}

	versions, _ := svc.store.GetVersions(ctx, "page", content.ID, 10, 0)
	if len(versions) == 0 {
		t.Fatal("expected some versions before delete")
	}

	if err := svc.store.DeleteVersionsByContentID(ctx, "page", content.ID); err != nil {
		t.Fatalf("DeleteVersionsByContentID: %v", err)
	}

	versions, _ = svc.store.GetVersions(ctx, "page", content.ID, 10, 0)
	if len(versions) != 0 {
		t.Errorf("expected 0 versions after delete, got %d", len(versions))
	}
}

func TestCreateJunctionTable(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	err := svc.store.CreateJunctionTable(ctx, "test_junction", "post", "tag")
	if err != nil {
		t.Fatalf("CreateJunctionTable: %v", err)
	}

	err = svc.store.CreateJunctionTable(ctx, "test_junction", "post", "tag")
	if err != nil {
		t.Fatalf("CreateJunctionTable idempotent: %v", err)
	}
}

func TestInsertAndGetJunctionRows(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	err := svc.store.CreateJunctionTable(ctx, "jt_posts_tags", "post", "tag")
	if err != nil {
		t.Fatalf("CreateJunctionTable: %v", err)
	}

	sourceID := "post-123"
	targetIDs := []string{"tag-1", "tag-2", "tag-3"}

	err = svc.store.InsertJunctionRows(ctx, "jt_posts_tags", "post", "tag", sourceID, targetIDs)
	if err != nil {
		t.Fatalf("InsertJunctionRows: %v", err)
	}

	ids, err := svc.store.GetJunctionIDs(ctx, "jt_posts_tags", "post", "tag", sourceID)
	if err != nil {
		t.Fatalf("GetJunctionIDs: %v", err)
	}
	if len(ids) != 3 {
		t.Fatalf("expected 3 IDs, got %d", len(ids))
	}

	found := map[string]bool{}
	for _, id := range ids {
		found[id] = true
	}
	for _, want := range targetIDs {
		if !found[want] {
			t.Errorf("missing target ID %q", want)
		}
	}
}

func TestDeleteJunctionRows(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	err := svc.store.CreateJunctionTable(ctx, "jt_posts_tags2", "post", "tag")
	if err != nil {
		t.Fatalf("CreateJunctionTable: %v", err)
	}

	sourceID := "post-456"
	err = svc.store.InsertJunctionRows(ctx, "jt_posts_tags2", "post", "tag", sourceID, []string{"tag-a", "tag-b"})
	if err != nil {
		t.Fatalf("InsertJunctionRows: %v", err)
	}

	err = svc.store.DeleteJunctionRows(ctx, "jt_posts_tags2", "post", sourceID)
	if err != nil {
		t.Fatalf("DeleteJunctionRows: %v", err)
	}

	ids, err := svc.store.GetJunctionIDs(ctx, "jt_posts_tags2", "post", "tag", sourceID)
	if err != nil {
		t.Fatalf("GetJunctionIDs: %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("expected 0 IDs after delete, got %d", len(ids))
	}
}

func TestGetJunctionIDsEmpty(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	err := svc.store.CreateJunctionTable(ctx, "jt_empty", "post", "tag")
	if err != nil {
		t.Fatalf("CreateJunctionTable: %v", err)
	}

	ids, err := svc.store.GetJunctionIDs(ctx, "jt_empty", "post", "tag", "nonexistent")
	if err != nil {
		t.Fatalf("GetJunctionIDs: %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("expected 0 IDs for nonexistent source, got %d", len(ids))
	}
}

func TestParseFilter(t *testing.T) {
	tests := []struct {
		name     string
		field    string
		value    interface{}
		wantLike string
		wantArgs int
	}{
		{"equals", "status", "draft", "\"status\" = ?", 1},
		{"gte", "price_gte", 50, "\"price\" >= ?", 1},
		{"lte", "price_lte", 100, "\"price\" <= ?", 1},
		{"gt", "price_gt", 10, "\"price\" > ?", 1},
		{"lt", "price_lt", 99, "\"price\" < ?", 1},
		{"contains", "title_contains", "hello", "\"title\" LIKE ?", 1},
		{"ne", "status_ne", "draft", "\"status\" != ?", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clause, args := parseFilter(tt.field, tt.value)
			if !containsStr(clause, tt.wantLike) {
				t.Errorf("parseFilter(%q) clause = %q, want containing %q", tt.field, clause, tt.wantLike)
			}
			if len(args) != tt.wantArgs {
				t.Errorf("parseFilter(%q) args count = %d, want %d", tt.field, len(args), tt.wantArgs)
			}
		})
	}
}

func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		(len(s) > 0 && len(sub) > 0 && findSubstr(s, sub)))
}

func findSubstr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestListWithRangeFilters(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	_, err := svc.CreateContentType(ctx, &interfaces.ContentType{
		Name:        "product",
		DisplayName: "Product",
		TableName:   "content_products",
		Fields: []interfaces.Field{
			{Name: "title", Type: "text", Required: true},
			{Name: "price", Type: "number"},
		},
	})
	if err != nil {
		t.Fatalf("CreateContentType: %v", err)
	}

	svc.Create(ctx, "product", map[string]interface{}{"title": "Cheap", "price": 10})
	svc.Create(ctx, "product", map[string]interface{}{"title": "Medium", "price": 50})
	svc.Create(ctx, "product", map[string]interface{}{"title": "Expensive", "price": 100})

	tests := []struct {
		name      string
		filterKey string
		filterVal interface{}
		wantCount int
	}{
		{"gte 50", "price_gte", 50, 2},
		{"lte 50", "price_lte", 50, 2},
		{"gt 10", "price_gt", 10, 2},
		{"lt 100", "price_lt", 100, 2},
		{"ne 50", "price_ne", 50, 2},
		{"equals 50", "price", 50, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			page, err := svc.List(ctx, "product", &interfaces.ListQuery{
				Filters: map[string]interface{}{tt.filterKey: tt.filterVal},
				PerPage: 100,
			})
			if err != nil {
				t.Fatalf("List: %v", err)
			}
			items, ok := page.Data.([]map[string]interface{})
			if !ok {
				t.Fatal("expected []map[string]interface{}")
			}
			if len(items) != tt.wantCount {
				t.Errorf("filter %s=%v: expected %d items, got %d", tt.filterKey, tt.filterVal, tt.wantCount, len(items))
			}
		})
	}
}

func TestListWithContainsFilter(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	svc.Create(ctx, "page", map[string]interface{}{"title": "Hello World"})
	svc.Create(ctx, "page", map[string]interface{}{"title": "Goodbye World"})
	svc.Create(ctx, "page", map[string]interface{}{"title": "Test Page"})

	page, err := svc.List(ctx, "page", &interfaces.ListQuery{
		Filters: map[string]interface{}{"title_contains": "World"},
		PerPage: 100,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	items, ok := page.Data.([]map[string]interface{})
	if !ok {
		t.Fatal("expected []map[string]interface{}")
	}
	if len(items) != 2 {
		t.Errorf("expected 2 items containing 'World', got %d", len(items))
	}
}

func TestListIncludeDeleted(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	svc.Create(ctx, "page", map[string]interface{}{"title": "Delete Me"})
	svc.Create(ctx, "page", map[string]interface{}{"title": "Keep Me"})

	all, _ := svc.List(ctx, "page", &interfaces.ListQuery{PerPage: 100})
	allItems := all.Data.([]map[string]interface{})

	for _, item := range allItems {
		if item["title"] == "Delete Me" {
			svc.Delete(ctx, item["id"].(string))
			break
		}
	}

	page, err := svc.List(ctx, "page", &interfaces.ListQuery{
		Filters: map[string]interface{}{"_include_deleted": true, "status": "draft"},
		PerPage: 100,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	items, ok := page.Data.([]map[string]interface{})
	if !ok {
		t.Fatal("expected []map[string]interface{}")
	}
	if len(items) < 2 {
		t.Errorf("expected at least 2 items with _include_deleted, got %d", len(items))
	}
}

func TestDDLTypeToSQL(t *testing.T) {
	tests := []struct {
		input ddl.FieldType
		want  string
	}{
		{ddl.FieldTypeNumber, "REAL"},
		{ddl.FieldTypeBoolean, "INTEGER"},
		{ddl.FieldTypeDatetime, "TEXT"},
		{ddl.FieldTypeJSON, "TEXT"},
		{ddl.FieldTypeText, "TEXT"},
	}

	for _, tt := range tests {
		t.Run(string(tt.input), func(t *testing.T) {
			got := ddlTypeToSQL(tt.input)
			if got != tt.want {
				t.Errorf("ddlTypeToSQL(%s) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestProtectBuiltInFields(t *testing.T) {
	svc := setupTestService(t)

	existingPage, _ := svc.GetContentType(context.Background(), "page")

	updatedPage := *existingPage
	updatedPage.Fields = existingPage.Fields[1:]

	err := svc.protectBuiltInFields(existingPage, &updatedPage)
	if err == nil {
		t.Error("expected error when removing field from built-in type")
	}

	customCT := &interfaces.ContentType{
		Name: "custom",
		Fields: []interfaces.Field{
			{Name: "title", Type: "text"},
			{Name: "body", Type: "text"},
		},
	}
	customUpdated := &interfaces.ContentType{
		Name: "custom",
		Fields: []interfaces.Field{
			{Name: "title", Type: "text"},
		},
	}

	err = svc.protectBuiltInFields(customCT, customUpdated)
	if err != nil {
		t.Errorf("expected no error for custom type, got: %v", err)
	}

	err = svc.protectBuiltInFields(existingPage, existingPage)
	if err != nil {
		t.Errorf("expected no error when same fields, got: %v", err)
	}
}

func TestExtractManyToManyData(t *testing.T) {
	svc := setupTestService(t)

	ct := &interfaces.ContentType{
		Name:      "article",
		TableName: "content_articles",
		Fields: []interfaces.Field{
			{Name: "title", Type: "text"},
			{Name: "tags", Type: "relation", RelationConfig: &interfaces.RelationConfig{
				TargetContentType: "tag",
				RelationType:      "many-to-many",
			}},
			{Name: "category", Type: "relation", RelationConfig: &interfaces.RelationConfig{
				TargetContentType: "category",
				RelationType:      "one-to-many",
			}},
		},
	}

	data := map[string]interface{}{
		"title":    "Test Article",
		"tags":     []interface{}{"tag-1", "tag-2"},
		"category": "cat-1",
	}

	m2mData := svc.extractManyToManyData(ct, data)

	if _, exists := data["tags"]; exists {
		t.Error("expected 'tags' to be removed from data")
	}
	if _, exists := data["category"]; !exists {
		t.Error("expected 'category' (one-to-many) to remain in data")
	}
	if len(m2mData) != 1 {
		t.Fatalf("expected 1 m2m field, got %d", len(m2mData))
	}
	if m2mData["tags"] == nil {
		t.Error("expected 'tags' in m2mData")
	}
}

func TestInsertManyToManyRows(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	ct := &interfaces.ContentType{
		Name:      "article",
		TableName: "content_articles_test",
		Fields: []interfaces.Field{
			{Name: "tags", Type: "relation", RelationConfig: &interfaces.RelationConfig{
				TargetContentType: "tag",
				RelationType:      "many-to-many",
			}},
		},
	}

	junctionTable := svc.junctionTableName(ct.TableName, ct.Fields[0])
	err := svc.store.CreateJunctionTable(ctx, junctionTable, "article", "tag")
	if err != nil {
		t.Fatalf("CreateJunctionTable: %v", err)
	}

	m2mData := map[string]interface{}{
		"tags": []interface{}{"tag-1", "tag-2"},
	}

	svc.insertManyToManyRows(ctx, ct, "article-123", m2mData)

	ids, err := svc.store.GetJunctionIDs(ctx, junctionTable, "article", "tag", "article-123")
	if err != nil {
		t.Fatalf("GetJunctionIDs: %v", err)
	}
	if len(ids) != 2 {
		t.Errorf("expected 2 junction IDs, got %d", len(ids))
	}
}

func TestUpdateManyToManyRows(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	ct := &interfaces.ContentType{
		Name:      "article2",
		TableName: "content_articles2",
		Fields: []interfaces.Field{
			{Name: "tags", Type: "relation", RelationConfig: &interfaces.RelationConfig{
				TargetContentType: "tag",
				RelationType:      "many-to-many",
			}},
		},
	}

	junctionTable := svc.junctionTableName(ct.TableName, ct.Fields[0])
	err := svc.store.CreateJunctionTable(ctx, junctionTable, "article2", "tag")
	if err != nil {
		t.Fatalf("CreateJunctionTable: %v", err)
	}

	svc.store.InsertJunctionRows(ctx, junctionTable, "article2", "tag", "art-1", []string{"old-tag"})

	m2mData := map[string]interface{}{
		"tags": []interface{}{"new-tag-1", "new-tag-2"},
	}

	svc.updateManyToManyRows(ctx, ct, "art-1", m2mData)

	ids, err := svc.store.GetJunctionIDs(ctx, junctionTable, "article2", "tag", "art-1")
	if err != nil {
		t.Fatalf("GetJunctionIDs: %v", err)
	}
	if len(ids) != 2 {
		t.Errorf("expected 2 junction IDs after update, got %d", len(ids))
	}
}

func TestDeleteManyToManyRows(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	ct := &interfaces.ContentType{
		Name:      "article3",
		TableName: "content_articles3",
		Fields: []interfaces.Field{
			{Name: "tags", Type: "relation", RelationConfig: &interfaces.RelationConfig{
				TargetContentType: "tag",
				RelationType:      "many-to-many",
			}},
		},
	}

	junctionTable := svc.junctionTableName(ct.TableName, ct.Fields[0])
	svc.store.CreateJunctionTable(ctx, junctionTable, "article3", "tag")
	svc.store.InsertJunctionRows(ctx, junctionTable, "article3", "tag", "art-del", []string{"t1", "t2"})

	svc.deleteManyToManyRows(ctx, ct, "art-del")

	ids, err := svc.store.GetJunctionIDs(ctx, junctionTable, "article3", "tag", "art-del")
	if err != nil {
		t.Fatalf("GetJunctionIDs: %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("expected 0 junction IDs after delete, got %d", len(ids))
	}
}

func TestValidateRelationWithStore(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	svc.Create(ctx, "tag", map[string]interface{}{
		"name": "Existing Tag",
	})

	tags, _ := svc.List(ctx, "tag", &interfaces.ListQuery{PerPage: 1})
	tagItems := tags.Data.([]map[string]interface{})
	if len(tagItems) == 0 {
		t.Fatal("need at least one tag")
	}
	tagID := tagItems[0]["id"].(string)

	ct := &interfaces.ContentType{
		Name: "post",
		Fields: []interfaces.Field{
			{
				Name: "related_tag",
				Type: "relation",
				RelationConfig: &interfaces.RelationConfig{
					TargetContentType: "tag",
					RelationType:      "one-to-many",
				},
			},
		},
	}

	err := svc.validator.Validate(ctx, ct, map[string]interface{}{
		"related_tag": tagID,
	})
	if err != nil {
		t.Errorf("expected valid for existing tag, got: %v", err)
	}

	err = svc.validator.Validate(ctx, ct, map[string]interface{}{
		"related_tag": "nonexistent-id",
	})
	if err == nil {
		t.Error("expected error for nonexistent tag reference")
	}
}

func TestValidateRelationCircular(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	ct := &interfaces.ContentType{
		Name: "category",
		Fields: []interfaces.Field{
			{
				Name: "parent",
				Type: "relation",
				RelationConfig: &interfaces.RelationConfig{
					TargetContentType: "category",
					RelationType:      "one-to-many",
				},
			},
		},
	}

	err := svc.validator.Validate(ctx, ct, map[string]interface{}{
		"parent": "some-id",
	})
	if err == nil {
		t.Error("expected error for self-referencing relation")
	}
}

func TestValidateRelationArrayWithStore(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	svc.Create(ctx, "tag", map[string]interface{}{"name": "Tag A"})
	svc.Create(ctx, "tag", map[string]interface{}{"name": "Tag B"})

	tags, _ := svc.List(ctx, "tag", &interfaces.ListQuery{PerPage: 100})
	tagItems := tags.Data.([]map[string]interface{})
	if len(tagItems) < 2 {
		t.Fatal("need at least 2 tags")
	}

	ct := &interfaces.ContentType{
		Name: "post",
		Fields: []interfaces.Field{
			{
				Name: "tags",
				Type: "relation",
				RelationConfig: &interfaces.RelationConfig{
					TargetContentType: "tag",
					RelationType:      "many-to-many",
				},
			},
		},
	}

	err := svc.validator.Validate(ctx, ct, map[string]interface{}{
		"tags": []interface{}{tagItems[0]["id"].(string), "nonexistent"},
	})
	if err == nil {
		t.Error("expected validation error for nonexistent tag in array")
	}
}

func TestValidateUnsupportedFieldType(t *testing.T) {
	v := &FieldValidator{}
	ctx := context.Background()

	ct := &interfaces.ContentType{
		Name: "test",
		Fields: []interfaces.Field{
			{Name: "xml_data", Type: "xml"},
		},
	}

	err := v.Validate(ctx, ct, map[string]interface{}{
		"xml_data": "<root/>",
	})
	if err == nil {
		t.Fatal("expected error for unsupported field type 'xml'")
	}
}

func TestPruneVersions(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	content, err := svc.Create(ctx, "page", map[string]interface{}{
		"title": "Prune Test",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	for i := 0; i < 5; i++ {
		_, err = svc.Update(ctx, content.ID, map[string]interface{}{
			"title": fmt.Sprintf("Version %d", i+2),
		})
		if err != nil {
			t.Fatalf("update %d: %v", i, err)
		}
	}

	versions, _ := svc.store.GetVersions(ctx, "page", content.ID, 100, 0)
	if len(versions) != 5 {
		t.Fatalf("expected 5 versions before prune, got %d", len(versions))
	}

	err = svc.store.PruneVersions(ctx, "page", content.ID, 3)
	if err != nil {
		t.Fatalf("PruneVersions: %v", err)
	}

	versions, _ = svc.store.GetVersions(ctx, "page", content.ID, 100, 0)
	if len(versions) != 3 {
		t.Errorf("expected 3 versions after prune, got %d", len(versions))
	}
}

func TestPruneVersionsBelowMax(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	content, err := svc.Create(ctx, "page", map[string]interface{}{
		"title": "Prune Below Max",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	err = svc.store.PruneVersions(ctx, "page", content.ID, 50)
	if err != nil {
		t.Fatalf("PruneVersions (below max): %v", err)
	}
}

func TestCreateJunctionTablesForNewFields(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	oldCT := &interfaces.ContentType{
		Name:      "article",
		TableName: "content_articles_jt",
		Fields: []interfaces.Field{
			{Name: "title", Type: "text"},
		},
	}

	newCT := &interfaces.ContentType{
		Name:      "article",
		TableName: "content_articles_jt",
		Fields: []interfaces.Field{
			{Name: "title", Type: "text"},
			{Name: "tags", Type: "relation", RelationConfig: &interfaces.RelationConfig{
				TargetContentType: "tag",
				RelationType:      "many-to-many",
			}},
		},
	}

	svc.createJunctionTablesForNewFields(ctx, oldCT, newCT)

	junctionTable := svc.junctionTableName(newCT.TableName, newCT.Fields[1])
	ids, err := svc.store.GetJunctionIDs(ctx, junctionTable, "article", "tag", "test-id")
	if err != nil {
		t.Fatalf("GetJunctionIDs on new junction table: %v", err)
	}
	if ids == nil {
		ids = []string{}
	}
}

func TestValidateMedia(t *testing.T) {
	v := &FieldValidator{}
	ctx := context.Background()

	ct := &interfaces.ContentType{
		Name: "test",
		Fields: []interfaces.Field{
			{Name: "image", Type: "media"},
		},
	}

	err := v.Validate(ctx, ct, map[string]interface{}{
		"image": map[string]interface{}{"url": "https://example.com/img.png"},
	})
	if err != nil {
		t.Errorf("expected media to pass, got: %v", err)
	}

	err = v.Validate(ctx, ct, map[string]interface{}{
		"image": nil,
	})
	if err != nil {
		t.Errorf("expected nil media to pass, got: %v", err)
	}
}

func TestValidateRelationNilConfig(t *testing.T) {
	v := &FieldValidator{}
	verrs := interfaces.NewValidationErrors()

	field := &interfaces.Field{
		Name:           "rel",
		Type:           "relation",
		RelationConfig: nil,
	}

	v.validateRelation("test", field, "some-value", "rel", verrs)
	if verrs.HasErrors() {
		t.Error("expected no errors for nil RelationConfig")
	}
}

func TestValidateRelationMissingTarget(t *testing.T) {
	v := &FieldValidator{}
	verrs := interfaces.NewValidationErrors()

	field := &interfaces.Field{
		Name: "rel",
		Type: "relation",
		RelationConfig: &interfaces.RelationConfig{
			TargetContentType: "",
		},
	}

	v.validateRelation("test", field, "val", "rel", verrs)
	if !verrs.HasErrors() {
		t.Error("expected error for missing target_content_type")
	}
}

func TestContentTypeSlugGeneration(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	ct, err := svc.CreateContentType(ctx, &interfaces.ContentType{
		Name:        "review",
		DisplayName: "Review",
		Fields: []interfaces.Field{
			{Name: "title", Type: "text", Required: true},
		},
	})
	if err != nil {
		t.Fatalf("CreateContentType: %v", err)
	}

	if ct.Slug == "" {
		t.Error("expected auto-generated slug")
	}

	ct2, err := svc.CreateContentType(ctx, &interfaces.ContentType{
		Name:        "testimonial",
		DisplayName: "Testimonial",
		Slug:        "custom-slug",
		Fields: []interfaces.Field{
			{Name: "title", Type: "text", Required: true},
		},
	})
	if err != nil {
		t.Fatalf("CreateContentType with explicit slug: %v", err)
	}
	if ct2.Slug != "custom-slug" {
		t.Errorf("expected slug 'custom-slug', got %q", ct2.Slug)
	}
}

func TestContentTypeSlugUniqueness(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	_, err := svc.CreateContentType(ctx, &interfaces.ContentType{
		Name:        "widget",
		DisplayName: "Widget",
		Slug:        "widgets",
		Fields: []interfaces.Field{
			{Name: "title", Type: "text", Required: true},
		},
	})
	if err != nil {
		t.Fatalf("CreateContentType: %v", err)
	}

	_, err = svc.CreateContentType(ctx, &interfaces.ContentType{
		Name:        "gadget",
		DisplayName: "Gadget",
		Slug:        "widgets",
		Fields: []interfaces.Field{
			{Name: "title", Type: "text", Required: true},
		},
	})
	if err == nil {
		t.Error("expected error for duplicate slug")
	}
}
