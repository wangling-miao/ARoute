package integration

import (
	"context"
	"testing"
	"time"

	"github.com/wangling-miao/aroute/sdk/interfaces"
)

// TestE2E_ContentTypeBuilder_CreateAndCRUD tests creating a custom content type
// via the API, verifying the table is created, performing CRUD on the new type,
// and then deleting the content type.
func TestE2E_ContentTypeBuilder_CreateAndCRUD(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	contentSvc := env.getContentService(t)
	dbSvc := env.getDatabaseService(t)

	ctName := "product"
	ctDisplayName := "Product"

	// Step 1: Create a custom content type "product"
	t.Run("create_content_type", func(t *testing.T) {
		ct, err := contentSvc.CreateContentType(ctx, &interfaces.ContentType{
			Name:        ctName,
			DisplayName: ctDisplayName,
			Description: "A product for the e-commerce catalog",
			Fields: []interfaces.Field{
				{
					Name:        "title",
					DisplayName: "Product Name",
					Type:        "text",
					Required:    true,
				},
				{
					Name:        "price",
					DisplayName: "Price",
					Type:        "number",
					Required:    true,
					ValidationRules: map[string]interface{}{
						"min": float64(0),
					},
				},
				{
					Name:        "description",
					DisplayName: "Description",
					Type:        "text",
				},
				{
					Name:        "in_stock",
					DisplayName: "In Stock",
					Type:        "boolean",
					DefaultValue: true,
				},
				{
					Name:        "sku",
					DisplayName: "SKU",
					Type:        "text",
					Unique:      true,
					Index:       true,
				},
			},
		})
		if err != nil {
			t.Fatalf("create content type: %v", err)
		}
		if ct.Name != ctName {
			t.Errorf("name = %q, want %q", ct.Name, ctName)
		}
		if len(ct.Fields) != 5 {
			t.Errorf("fields count = %d, want 5", len(ct.Fields))
		}
		t.Logf("Created content type: %s (table: %s)", ct.Name, ct.TableName)
	})

	// Step 2: Verify the table was created in the database
	t.Run("verify_table_created", func(t *testing.T) {
		rows, err := dbSvc.Query(ctx, "SELECT name FROM sqlite_master WHERE type='table' AND name LIKE '%product%'")
		if err != nil {
			t.Fatalf("query tables: %v", err)
		}
		defer rows.Close()

		found := false
		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err == nil {
				t.Logf("Found table: %s", name)
				found = true
			}
		}
		if !found {
			t.Error("expected product table to be created")
		}
	})

	// Step 3: Verify content type is retrievable
	t.Run("get_content_type", func(t *testing.T) {
		ct, err := contentSvc.GetContentType(ctx, ctName)
		if err != nil {
			t.Fatalf("get content type: %v", err)
		}
		if ct.DisplayName != ctDisplayName {
			t.Errorf("display name = %q, want %q", ct.DisplayName, ctDisplayName)
		}
	})

	// Step 4: List content types should include our custom type
	t.Run("list_content_types", func(t *testing.T) {
		types, err := contentSvc.ListContentTypes(ctx)
		if err != nil {
			t.Fatalf("list content types: %v", err)
		}
		found := false
		for _, ct := range types {
			if ct.Name == ctName {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("content type %q not found in list of %d types", ctName, len(types))
		}
	})

	// Step 5: CRUD operations on the new content type
	var productID string

	t.Run("create_product", func(t *testing.T) {
		product, err := contentSvc.Create(ctx, ctName, map[string]interface{}{
			"title":       "Widget Pro",
			"price":       29.99,
			"description": "A premium widget",
			"in_stock":    true,
			"sku":         "WGT-PRO-001",
		})
		if err != nil {
			t.Fatalf("create product: %v", err)
		}
		productID = product.ID
		if product.Title != "Widget Pro" {
			t.Errorf("title = %q, want Widget Pro", product.Title)
		}
		t.Logf("Created product: id=%s", productID)
	})

	t.Run("get_product", func(t *testing.T) {
		product, err := contentSvc.GetByID(ctx, productID)
		if err != nil {
			t.Fatalf("get product: %v", err)
		}
		if product.ID != productID {
			t.Errorf("ID mismatch: %q vs %q", product.ID, productID)
		}
	})

	t.Run("update_product", func(t *testing.T) {
		updated, err := contentSvc.Update(ctx, productID, map[string]interface{}{
			"title": "Widget Pro V2",
			"price": 39.99,
		})
		if err != nil {
			t.Fatalf("update product: %v", err)
		}
		if updated.Title != "Widget Pro V2" {
			t.Errorf("title = %q, want Widget Pro V2", updated.Title)
		}
	})

	t.Run("list_products", func(t *testing.T) {
		page, err := contentSvc.List(ctx, ctName, &interfaces.ListQuery{
			Page:    1,
			PerPage: 10,
		})
		if err != nil {
			t.Fatalf("list products: %v", err)
		}
		if page.Meta.Total < 1 {
			t.Errorf("expected at least 1 product, got %d", page.Meta.Total)
		}
	})

	t.Run("delete_product", func(t *testing.T) {
		if err := contentSvc.Delete(ctx, productID); err != nil {
			t.Fatalf("delete product: %v", err)
		}
	})

	// Step 6: Create more products for filter/sort testing
	t.Run("bulk_create", func(t *testing.T) {
		products := []map[string]interface{}{
			{"title": "Basic Widget", "price": 9.99, "sku": "WGT-BAS-001", "in_stock": true},
			{"title": "Deluxe Widget", "price": 49.99, "sku": "WGT-DLX-001", "in_stock": false},
			{"title": "Mini Widget", "price": 4.99, "sku": "WGT-MIN-001", "in_stock": true},
		}
		for _, p := range products {
			_, err := contentSvc.Create(ctx, ctName, p)
			if err != nil {
				t.Fatalf("create product %s: %v", p["title"], err)
			}
		}
	})

	t.Run("filter_products", func(t *testing.T) {
		page, err := contentSvc.List(ctx, ctName, &interfaces.ListQuery{
			Page:    1,
			PerPage: 10,
			Filters: map[string]interface{}{"in_stock": true},
		})
		if err != nil {
			t.Fatalf("filter products: %v", err)
		}
		t.Logf("In-stock products: %d", page.Meta.Total)
	})

	t.Run("sort_products_by_price", func(t *testing.T) {
		page, err := contentSvc.List(ctx, ctName, &interfaces.ListQuery{
			Page:    1,
			PerPage: 10,
			Sort:    "price",
			Order:   "asc",
		})
		if err != nil {
			t.Fatalf("sort products: %v", err)
		}
		t.Logf("Products sorted by price: total=%d", page.Meta.Total)
	})

	t.Run("paginate_products", func(t *testing.T) {
		page1, err := contentSvc.List(ctx, ctName, &interfaces.ListQuery{
			Page:    1,
			PerPage: 2,
		})
		if err != nil {
			t.Fatalf("paginate page 1: %v", err)
		}
		if page1.Meta.Total < 3 {
			t.Errorf("expected total >= 3, got %d", page1.Meta.Total)
		}
		if !page1.Meta.HasNext {
			t.Error("expected HasNext=true")
		}

		page2, err := contentSvc.List(ctx, ctName, &interfaces.ListQuery{
			Page:    2,
			PerPage: 2,
		})
		if err != nil {
			t.Fatalf("paginate page 2: %v", err)
		}
		if !page2.Meta.HasPrev {
			t.Error("expected HasPrev=true on page 2")
		}
	})

	// Step 7: Delete the custom content type
	t.Run("delete_content_type", func(t *testing.T) {
		if err := contentSvc.DeleteContentType(ctx, ctName); err != nil {
			t.Fatalf("delete content type: %v", err)
		}
		t.Log("Content type deleted successfully")
	})

	// Verify deletion
	t.Run("verify_content_type_deleted", func(t *testing.T) {
		_, err := contentSvc.GetContentType(ctx, ctName)
		if err == nil {
			t.Error("expected error getting deleted content type")
		}
	})
}

// TestE2E_ContentTypeBuilder_FieldValidation tests that field validation
// rules are enforced when creating content with custom content types.
func TestE2E_ContentTypeBuilder_FieldValidation(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	contentSvc := env.getContentService(t)

	// Create a content type with strict validation
	ctName := "validated_item"
	_, err := contentSvc.CreateContentType(ctx, &interfaces.ContentType{
		Name:        ctName,
		DisplayName: "Validated Item",
		Fields: []interfaces.Field{
			{
				Name:     "title",
				Type:     "text",
				Required: true,
				ValidationRules: map[string]interface{}{
					"minLength": float64(3),
					"maxLength": float64(100),
				},
			},
			{
				Name:     "code",
				Type:     "text",
				Required: true,
				Unique:   true,
			},
		},
	})
	if err != nil {
		t.Fatalf("create content type: %v", err)
	}
	t.Cleanup(func() {
		contentSvc.DeleteContentType(context.Background(), ctName)
	})

	// Create with valid data
	t.Run("valid_data", func(t *testing.T) {
		item, err := contentSvc.Create(ctx, ctName, map[string]interface{}{
			"title": "Valid Title",
			"code":  "VAL-001",
		})
		if err != nil {
			t.Fatalf("create valid item: %v", err)
		}
		if item.ID == "" {
			t.Error("expected non-empty ID")
		}
	})

	// Create with missing required field
	t.Run("missing_required", func(t *testing.T) {
		_, err := contentSvc.Create(ctx, ctName, map[string]interface{}{
			"code": "VAL-002",
		})
		if err == nil {
			t.Error("expected error for missing required field 'title'")
		}
		t.Logf("Missing required field error (expected): %v", err)
	})

	// Create with duplicate unique field
	t.Run("duplicate_unique", func(t *testing.T) {
		_, err := contentSvc.Create(ctx, ctName, map[string]interface{}{
			"title": "Another Title",
			"code":  "VAL-001", // duplicate
		})
		if err == nil {
			t.Error("expected error for duplicate unique field 'code'")
		}
		t.Logf("Duplicate unique field error (expected): %v", err)
	})
}

// TestE2E_ContentTypeBuilder_VersionHistory tests that content version
// history is maintained for custom content types.
func TestE2E_ContentTypeBuilder_VersionHistory(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	contentSvc := env.getContentService(t)

	ctName := "versioned_doc"
	_, err := contentSvc.CreateContentType(ctx, &interfaces.ContentType{
		Name:        ctName,
		DisplayName: "Versioned Document",
		Fields: []interfaces.Field{
			{Name: "title", Type: "text", Required: true},
			{Name: "body", Type: "text"},
		},
	})
	if err != nil {
		t.Fatalf("create content type: %v", err)
	}
	t.Cleanup(func() {
		contentSvc.DeleteContentType(context.Background(), ctName)
	})

	// Create a document
	doc, err := contentSvc.Create(ctx, ctName, map[string]interface{}{
		"title": "Version Test v1",
		"body":  "Initial body",
	})
	if err != nil {
		t.Fatalf("create doc: %v", err)
	}
	if doc.Version != 1 {
		t.Errorf("initial version = %d, want 1", doc.Version)
	}

	// Update it multiple times
	for i := 2; i <= 3; i++ {
		updated, err := contentSvc.Update(ctx, doc.ID, map[string]interface{}{
			"title": "Version Test v" + string(rune('0'+i)),
			"body":  "Updated body",
		})
		if err != nil {
			t.Fatalf("update doc v%d: %v", i, err)
		}
		if updated.Version != i {
			t.Errorf("version after update %d = %d, want %d", i, updated.Version, i)
		}
	}

	// Wait for async operations
	time.Sleep(100 * time.Millisecond)
}
