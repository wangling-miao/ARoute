package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wangling-miao/aroute/sdk/interfaces"
)

func sampleContentType(name, slug, displayName, desc string, fields []interfaces.Field) interfaces.ContentType {
	return interfaces.ContentType{
		ID:          "ct-" + name,
		Name:        name,
		Slug:        slug,
		DisplayName: displayName,
		Description: desc,
		Fields:      fields,
		TableName:   name + "s",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
}

func ptrContentType(ct interfaces.ContentType) *interfaces.ContentType {
	return &ct
}

func TestGenerateOpenAPISpec_EmptyContentTypes(t *testing.T) {
	spec := GenerateOpenAPISpec(nil)

	assert.Equal(t, "3.0.3", spec.OpenAPI)
	assert.Equal(t, "ARoute CMS REST API", spec.Info.Title)
	assert.Equal(t, "1.0.0", spec.Info.Version)
	assert.Empty(t, spec.Paths)
	assert.NotNil(t, spec.Components)
	assert.NotNil(t, spec.Components.Schemas)
	assert.NotNil(t, spec.Components.SecuritySchemes)
}

func TestGenerateOpenAPISpec_SingleContentType(t *testing.T) {
	ct := sampleContentType("post", "posts", "Posts", "Blog posts", []interfaces.Field{
		{Name: "title", DisplayName: "Title", Type: "text", Required: true},
		{Name: "body", DisplayName: "Body", Type: "richtext"},
	})

	spec := GenerateOpenAPISpec([]interfaces.ContentType{ct})

	assert.Contains(t, spec.Paths, "/api/v1/posts")
	assert.Contains(t, spec.Paths, "/api/v1/posts/{id}")
	assert.Contains(t, spec.Components.Schemas, "Post")
}

func TestGenerateOpenAPISpec_MultipleContentTypes(t *testing.T) {
	ct1 := sampleContentType("post", "posts", "Posts", "Blog posts", []interfaces.Field{
		{Name: "title", DisplayName: "Title", Type: "text"},
	})
	ct2 := sampleContentType("category", "categories", "Categories", "Content categories", []interfaces.Field{
		{Name: "name", DisplayName: "Name", Type: "text"},
	})

	spec := GenerateOpenAPISpec([]interfaces.ContentType{ct1, ct2})

	assert.Contains(t, spec.Paths, "/api/v1/posts")
	assert.Contains(t, spec.Paths, "/api/v1/posts/{id}")
	assert.Contains(t, spec.Paths, "/api/v1/categories")
	assert.Contains(t, spec.Paths, "/api/v1/categories/{id}")
	assert.Contains(t, spec.Components.Schemas, "Post")
	assert.Contains(t, spec.Components.Schemas, "Category")
}

func TestGenerateOpenAPISpec_FieldTypeMapping(t *testing.T) {
	tests := []struct {
		name         string
		fieldType    string
		expectedType string
		extraCheck   func(t *testing.T, schema map[string]interface{})
	}{
		{
			name: "text", fieldType: "text",
			expectedType: "string",
		},
		{
			name: "number", fieldType: "number",
			expectedType: "number",
		},
		{
			name: "boolean", fieldType: "boolean",
			expectedType: "boolean",
		},
		{
			name: "date", fieldType: "date",
			expectedType: "string",
			extraCheck: func(t *testing.T, schema map[string]interface{}) {
				assert.Equal(t, "date", schema["format"])
			},
		},
		{
			name: "datetime", fieldType: "datetime",
			expectedType: "string",
			extraCheck: func(t *testing.T, schema map[string]interface{}) {
				assert.Equal(t, "date-time", schema["format"])
			},
		},
		{
			name: "relation", fieldType: "relation",
			expectedType: "string",
			extraCheck: func(t *testing.T, schema map[string]interface{}) {
				assert.Equal(t, true, schema["x-relation"])
			},
		},
		{
			name: "media", fieldType: "media",
			expectedType: "string",
			extraCheck: func(t *testing.T, schema map[string]interface{}) {
				assert.Equal(t, true, schema["x-media"])
			},
		},
		{
			name: "json", fieldType: "json",
			expectedType: "object",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ct := sampleContentType("test_ct", "test", "Test", "test", []interfaces.Field{
				{Name: "field1", DisplayName: "Field1", Type: tt.fieldType},
			})

			spec := GenerateOpenAPISpec([]interfaces.ContentType{ct})
			schema := spec.Components.Schemas["TestCt"]

			obj, ok := schema.(map[string]interface{})
			require.True(t, ok)

			props, ok := obj["properties"].(map[string]interface{})
			require.True(t, ok)

			data, ok := props["data"].(map[string]interface{})
			require.True(t, ok)

			dataProps, ok := data["properties"].(map[string]interface{})
			require.True(t, ok)

			field1, ok := dataProps["field1"].(map[string]interface{})
			require.True(t, ok)

			assert.Equal(t, tt.expectedType, field1["type"])
			if tt.extraCheck != nil {
				tt.extraCheck(t, field1)
			}
		})
	}
}

func TestGenerateOpenAPISpec_SpecStructure(t *testing.T) {
	spec := GenerateOpenAPISpec(nil)

	assert.Equal(t, "3.0.3", spec.OpenAPI)
	assert.Equal(t, "ARoute CMS REST API", spec.Info.Title)
	assert.Equal(t, "1.0.0", spec.Info.Version)
	assert.Equal(t, "Auto-generated REST API for ARoute CMS content types", spec.Info.Description)
	require.Len(t, spec.Servers, 1)
	assert.Equal(t, "/", spec.Servers[0].URL)
	assert.Equal(t, "Current server", spec.Servers[0].Description)
}

func TestGenerateOpenAPISpec_SecurityScheme(t *testing.T) {
	spec := GenerateOpenAPISpec(nil)

	require.NotNil(t, spec.Components)
	require.NotNil(t, spec.Components.SecuritySchemes)

	bearerAuth, ok := spec.Components.SecuritySchemes["bearerAuth"].(map[string]interface{})
	require.True(t, ok)

	assert.Equal(t, "http", bearerAuth["type"])
	assert.Equal(t, "bearer", bearerAuth["scheme"])
	assert.Equal(t, "JWT", bearerAuth["bearerFormat"])
	assert.Contains(t, bearerAuth["description"], "JWT authentication")

	apiKeyAuth, ok := spec.Components.SecuritySchemes["apiKeyAuth"].(map[string]interface{})
	require.True(t, ok)

	assert.Equal(t, "apiKey", apiKeyAuth["type"])
	assert.Equal(t, "header", apiKeyAuth["in"])
	assert.Equal(t, "X-API-Key", apiKeyAuth["name"])
	assert.Contains(t, apiKeyAuth["description"], "API key")
}

func TestGenerateOpenAPISpec_CollectionPathOperations(t *testing.T) {
	ct := sampleContentType("post", "posts", "Posts", "Blog posts", []interfaces.Field{
		{Name: "title", DisplayName: "Title", Type: "text"},
	})

	spec := GenerateOpenAPISpec([]interfaces.ContentType{ct})

	path := spec.Paths["/api/v1/posts"]
	assert.NotNil(t, path.Get, "collection path should have GET")
	assert.NotNil(t, path.Post, "collection path should have POST")
	assert.Nil(t, path.Put, "collection path should NOT have PUT")
	assert.Nil(t, path.Delete, "collection path should NOT have DELETE")
}

func TestGenerateOpenAPISpec_ItemPathOperations(t *testing.T) {
	ct := sampleContentType("post", "posts", "Posts", "Blog posts", []interfaces.Field{
		{Name: "title", DisplayName: "Title", Type: "text"},
	})

	spec := GenerateOpenAPISpec([]interfaces.ContentType{ct})

	path := spec.Paths["/api/v1/posts/{id}"]
	assert.NotNil(t, path.Get, "item path should have GET")
	assert.NotNil(t, path.Put, "item path should have PUT")
	assert.NotNil(t, path.Delete, "item path should have DELETE")
	assert.Nil(t, path.Post, "item path should NOT have POST")
}

func TestGenerateOpenAPISpec_OperationIDs(t *testing.T) {
	ct := sampleContentType("post", "posts", "Posts", "Blog posts", []interfaces.Field{})

	spec := GenerateOpenAPISpec([]interfaces.ContentType{ct})

	collection := spec.Paths["/api/v1/posts"]
	assert.Equal(t, "listPost", collection.Get.OperationID)
	assert.Equal(t, "createPost", collection.Post.OperationID)

	item := spec.Paths["/api/v1/posts/{id}"]
	assert.Equal(t, "getPost", item.Get.OperationID)
	assert.Equal(t, "updatePost", item.Put.OperationID)
	assert.Equal(t, "deletePost", item.Delete.OperationID)
}

func TestGenerateOpenAPISpec_ListQueryParameters(t *testing.T) {
	ct := sampleContentType("post", "posts", "Posts", "Blog posts", []interfaces.Field{})

	spec := GenerateOpenAPISpec([]interfaces.ContentType{ct})

	listOp := spec.Paths["/api/v1/posts"].Get
	require.NotEmpty(t, listOp.Parameters)

	paramNames := make([]string, len(listOp.Parameters))
	for i, p := range listOp.Parameters {
		paramNames[i] = p.Name
		assert.Equal(t, "query", p.In)
	}

	assert.Contains(t, paramNames, "sort")
	assert.Contains(t, paramNames, "order")
	assert.Contains(t, paramNames, "page")
	assert.Contains(t, paramNames, "per_page")
	assert.Contains(t, paramNames, "fields")
	assert.Contains(t, paramNames, "expand")
	assert.Contains(t, paramNames, "search")
	assert.NotContains(t, paramNames, "filter")
}

func TestGenerateOpenAPISpec_IDPathParameter(t *testing.T) {
	ct := sampleContentType("post", "posts", "Posts", "Blog posts", []interfaces.Field{})

	spec := GenerateOpenAPISpec([]interfaces.ContentType{ct})

	getOp := spec.Paths["/api/v1/posts/{id}"].Get
	require.Len(t, getOp.Parameters, 1)
	assert.Equal(t, "id", getOp.Parameters[0].Name)
	assert.Equal(t, "path", getOp.Parameters[0].In)
	assert.True(t, getOp.Parameters[0].Required)
}

func TestGenerateOpenAPISpec_ComponentSchemaHasSystemFields(t *testing.T) {
	ct := sampleContentType("post", "posts", "Posts", "Blog posts", []interfaces.Field{
		{Name: "title", DisplayName: "Title", Type: "text"},
	})

	spec := GenerateOpenAPISpec([]interfaces.ContentType{ct})
	schema := spec.Components.Schemas["Post"].(map[string]interface{})
	props := schema["properties"].(map[string]interface{})

	systemFields := []string{
		"id", "content_type", "title", "slug", "status",
		"author_id", "version", "published_at", "created_at", "updated_at", "data",
	}
	for _, f := range systemFields {
		assert.Contains(t, props, f, "component schema should contain system field: %s", f)
	}
}

func TestGenerateOpenAPISpec_FieldDescriptionPropagated(t *testing.T) {
	ct := sampleContentType("post", "posts", "Posts", "Blog posts", []interfaces.Field{
		{Name: "title", DisplayName: "Title", Type: "text", Description: "The post title"},
	})

	spec := GenerateOpenAPISpec([]interfaces.ContentType{ct})
	schema := spec.Components.Schemas["Post"].(map[string]interface{})
	props := schema["properties"].(map[string]interface{})
	data := props["data"].(map[string]interface{})
	dataProps := data["properties"].(map[string]interface{})
	titleField := dataProps["title"].(map[string]interface{})

	assert.Equal(t, "The post title", titleField["description"])
}

func TestGenerateOpenAPISpec_SecurityOnOperations(t *testing.T) {
	ct := sampleContentType("post", "posts", "Posts", "Blog posts", []interfaces.Field{})

	spec := GenerateOpenAPISpec([]interfaces.ContentType{ct})

	ops := []*OpenAPIOperation{
		spec.Paths["/api/v1/posts"].Get,
		spec.Paths["/api/v1/posts"].Post,
		spec.Paths["/api/v1/posts/{id}"].Get,
		spec.Paths["/api/v1/posts/{id}"].Put,
		spec.Paths["/api/v1/posts/{id}"].Delete,
	}

	for _, op := range ops {
		require.NotNil(t, op.Security, "operation %s should have security", op.OperationID)
		require.NotEmpty(t, op.Security)
		assert.Contains(t, op.Security[0], "bearerAuth")
	}
}

func TestGenerateOpenAPISpec_RequestBodyOnWriteOps(t *testing.T) {
	ct := sampleContentType("post", "posts", "Posts", "Blog posts", []interfaces.Field{})

	spec := GenerateOpenAPISpec([]interfaces.ContentType{ct})

	require.NotNil(t, spec.Paths["/api/v1/posts"].Post.RequestBody)
	assert.True(t, spec.Paths["/api/v1/posts"].Post.RequestBody.Required)

	require.NotNil(t, spec.Paths["/api/v1/posts/{id}"].Put.RequestBody)
	assert.True(t, spec.Paths["/api/v1/posts/{id}"].Put.RequestBody.Required)
}

// ---------------------------------------------------------------------------
// TestHandleDocs
// ---------------------------------------------------------------------------

type mockContentSvcForDocs struct {
	types []*interfaces.ContentType
}

func (m *mockContentSvcForDocs) Create(ctx context.Context, contentType string, data map[string]interface{}) (*interfaces.Content, error) {
	return nil, nil
}
func (m *mockContentSvcForDocs) GetByID(ctx context.Context, id string) (*interfaces.Content, error) {
	return nil, nil
}
func (m *mockContentSvcForDocs) Update(ctx context.Context, id string, data map[string]interface{}) (*interfaces.Content, error) {
	return nil, nil
}
func (m *mockContentSvcForDocs) Delete(ctx context.Context, id string) error { return nil }
func (m *mockContentSvcForDocs) List(ctx context.Context, contentType string, query *interfaces.ListQuery) (*interfaces.Page, error) {
	return nil, nil
}
func (m *mockContentSvcForDocs) GetContentType(ctx context.Context, name string) (*interfaces.ContentType, error) {
	return nil, nil
}
func (m *mockContentSvcForDocs) CreateContentType(ctx context.Context, ct *interfaces.ContentType) (*interfaces.ContentType, error) {
	return nil, nil
}
func (m *mockContentSvcForDocs) UpdateContentType(ctx context.Context, name string, ct *interfaces.ContentType) (*interfaces.ContentType, error) {
	return nil, nil
}
func (m *mockContentSvcForDocs) DeleteContentType(ctx context.Context, name string) error { return nil }
func (m *mockContentSvcForDocs) ListContentTypes(ctx context.Context) ([]*interfaces.ContentType, error) {
	return m.types, nil
}

func TestHandleDocs_Returns200WithValidJSON(t *testing.T) {
	h := NewHandler(&mockContentSvcForDocs{
		types: []*interfaces.ContentType{
			ptrContentType(sampleContentType("post", "posts", "Posts", "Blog posts", []interfaces.Field{})),
		},
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/docs", nil)

	h.handleDocs(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var spec map[string]interface{}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&spec))
	assert.Equal(t, "3.0.3", spec["openapi"])
}

func TestHandleDocs_ContentTypeIsJSON(t *testing.T) {
	h := NewHandler(&mockContentSvcForDocs{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/docs", nil)

	h.handleDocs(rec, req)

	ct := rec.Header().Get("Content-Type")
	assert.Contains(t, ct, "application/json")
}

func TestHandleDocs_ContainsRequiredFields(t *testing.T) {
	h := NewHandler(&mockContentSvcForDocs{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/docs", nil)

	h.handleDocs(rec, req)

	var spec map[string]interface{}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&spec))

	_, hasOpenAPI := spec["openapi"]
	_, hasInfo := spec["info"]
	_, hasPaths := spec["paths"]

	assert.True(t, hasOpenAPI, "spec should have 'openapi' field")
	assert.True(t, hasInfo, "spec should have 'info' field")
	assert.True(t, hasPaths, "spec should have 'paths' field")
}

// ---------------------------------------------------------------------------
// TestPascalCase
// ---------------------------------------------------------------------------

func TestPascalCase_SnakeCase(t *testing.T) {
	assert.Equal(t, "BlogPost", pascalCase("blog_post"))
}

func TestPascalCase_KebabCase(t *testing.T) {
	assert.Equal(t, "MyContentType", pascalCase("my-content-type"))
}

func TestPascalCase_SingleWord(t *testing.T) {
	assert.Equal(t, "Post", pascalCase("post"))
}

func TestPascalCase_EmptyString(t *testing.T) {
	assert.Equal(t, "", pascalCase(""))
}

// ---------------------------------------------------------------------------
// TestFieldTypeToJSONSchema
// ---------------------------------------------------------------------------

func TestFieldTypeToJSONSchema_AllSupportedTypes(t *testing.T) {
	tests := []struct {
		name         string
		fieldType    string
		expectedType string
		extraCheck   func(t *testing.T, schema map[string]interface{})
	}{
		{"text", "text", "string", nil},
		{"markdown", "markdown", "string", nil},
		{"richtext", "richtext", "string", nil},
		{"email", "email", "string", nil},
		{"url", "url", "string", nil},
		{"slug", "slug", "string", nil},
		{"color", "color", "string", nil},
		{"enum", "enum", "string", nil},
		{"number", "number", "number", nil},
		{"boolean", "boolean", "boolean", nil},
		{"date", "date", "string", func(t *testing.T, s map[string]interface{}) {
			assert.Equal(t, "date", s["format"])
		}},
		{"datetime", "datetime", "string", func(t *testing.T, s map[string]interface{}) {
			assert.Equal(t, "date-time", s["format"])
		}},
		{"relation", "relation", "string", func(t *testing.T, s map[string]interface{}) {
			assert.Equal(t, true, s["x-relation"])
		}},
		{"media", "media", "string", func(t *testing.T, s map[string]interface{}) {
			assert.Equal(t, true, s["x-media"])
		}},
		{"json", "json", "object", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schema := fieldTypeToJSONSchema(tt.fieldType)
			assert.Equal(t, tt.expectedType, schema["type"])
			if tt.extraCheck != nil {
				tt.extraCheck(t, schema)
			}
		})
	}
}

func TestFieldTypeToJSONSchema_UnknownType_DefaultsToString(t *testing.T) {
	schema := fieldTypeToJSONSchema("unknown_weird_type")
	assert.Equal(t, "string", schema["type"])
}

// ---------------------------------------------------------------------------
// C1: TestHandleDocsUI / TestDocsUIHandler
// ---------------------------------------------------------------------------

func TestHandleDocsUI_ReturnsHTMLWithSwaggerUI(t *testing.T) {
	h := NewHandler(nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/docs", nil)

	h.handleDocsUI(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Header().Get("Content-Type"), "text/html")
	body := rec.Body.String()
	assert.Contains(t, body, "swagger-ui")
	assert.Contains(t, body, "swagger-ui-bundle.js")
	assert.Contains(t, body, "/api/v1/openapi.json")
}

func TestDocsUIHandler_DefaultSwagger(t *testing.T) {
	h := NewHandler(nil)
	handler := h.docsUIHandler("swagger")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/docs", nil)
	handler(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "swagger-ui")
}

func TestDocsUIHandler_RedocUI(t *testing.T) {
	h := NewHandler(nil)
	handler := h.docsUIHandler("redoc")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/docs", nil)
	handler(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Header().Get("Content-Type"), "text/html")
	body := rec.Body.String()
	assert.Contains(t, body, "redoc")
	assert.Contains(t, body, "redoc.standalone.js")
	assert.Contains(t, body, "/api/v1/openapi.json")
	assert.NotContains(t, body, "swagger-ui")
}

func TestDocsUIHandler_EmptyString_DefaultsToSwagger(t *testing.T) {
	h := NewHandler(nil)
	handler := h.docsUIHandler("")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/docs", nil)
	handler(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "swagger-ui")
}

func TestHandleDocsUI_ContainsRequiredHTMLElements(t *testing.T) {
	h := NewHandler(nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/docs", nil)

	h.handleDocsUI(rec, req)

	body := rec.Body.String()
	assert.Contains(t, body, "<!DOCTYPE html>")
	assert.Contains(t, body, "<html>")
	assert.Contains(t, body, "</html>")
	assert.Contains(t, body, "<head>")
	assert.Contains(t, body, "<body>")
}
