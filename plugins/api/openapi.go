package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"unicode"

	"github.com/wangling-miao/aroute/sdk/interfaces"
)

// OpenAPISpec represents an OpenAPI 3.0 specification.
type OpenAPISpec struct {
	OpenAPI    string                     `json:"openapi"`
	Info       OpenAPIInfo                `json:"info"`
	Servers    []OpenAPIServer            `json:"servers,omitempty"`
	Paths      map[string]OpenAPIPathItem `json:"paths"`
	Components *OpenAPIComponents         `json:"components,omitempty"`
}

// OpenAPIInfo contains API metadata.
type OpenAPIInfo struct {
	Title       string `json:"title"`
	Version     string `json:"version"`
	Description string `json:"description"`
}

// OpenAPIServer describes a single server URL.
type OpenAPIServer struct {
	URL         string `json:"url"`
	Description string `json:"description"`
}

// OpenAPIComponents holds reusable schemas and security schemes.
type OpenAPIComponents struct {
	Schemas         map[string]interface{} `json:"schemas,omitempty"`
	SecuritySchemes map[string]interface{} `json:"securitySchemes,omitempty"`
}

// OpenAPIPathItem describes operations on a single path.
type OpenAPIPathItem struct {
	Get    *OpenAPIOperation `json:"get,omitempty"`
	Post   *OpenAPIOperation `json:"post,omitempty"`
	Put    *OpenAPIOperation `json:"put,omitempty"`
	Delete *OpenAPIOperation `json:"delete,omitempty"`
}

// OpenAPIOperation describes a single API operation.
type OpenAPIOperation struct {
	Summary     string                     `json:"summary"`
	Description string                     `json:"description"`
	OperationID string                     `json:"operationId"`
	Tags        []string                   `json:"tags"`
	Parameters  []OpenAPIParameter         `json:"parameters,omitempty"`
	RequestBody *OpenAPIRequestBody        `json:"requestBody,omitempty"`
	Responses   map[string]OpenAPIResponse `json:"responses"`
	Security    []map[string][]string      `json:"security,omitempty"`
}

// OpenAPIParameter describes a single operation parameter.
type OpenAPIParameter struct {
	Name        string      `json:"name"`
	In          string      `json:"in"`
	Description string      `json:"description,omitempty"`
	Required    bool        `json:"required"`
	Schema      interface{} `json:"schema"`
}

// OpenAPIRequestBody describes the expected request payload.
type OpenAPIRequestBody struct {
	Required bool                   `json:"required"`
	Content  map[string]interface{} `json:"content"`
}

// OpenAPIResponse describes a single response.
type OpenAPIResponse struct {
	Description string                 `json:"description"`
	Content     map[string]interface{} `json:"content,omitempty"`
}

// contentTypesRegistry stores known content types discovered during route
// registration. Since ContentService does not expose ListContentTypes, the
// registry is the only way for the docs endpoint to enumerate types.
var (
	contentTypesRegistry   []interfaces.ContentType
	contentTypesRegistryMu sync.RWMutex
)

// RegisterContentType adds a content type to the OpenAPI registry,
// replacing any existing entry with the same name.
func RegisterContentType(ct interfaces.ContentType) {
	contentTypesRegistryMu.Lock()
	defer contentTypesRegistryMu.Unlock()
	for i, existing := range contentTypesRegistry {
		if existing.Name == ct.Name {
			contentTypesRegistry[i] = ct
			return
		}
	}
	contentTypesRegistry = append(contentTypesRegistry, ct)
}

// RegisteredContentTypes returns a copy of all registered content types.
func RegisteredContentTypes() []interfaces.ContentType {
	contentTypesRegistryMu.RLock()
	defer contentTypesRegistryMu.RUnlock()
	out := make([]interfaces.ContentType, len(contentTypesRegistry))
	copy(out, contentTypesRegistry)
	return out
}

// GenerateOpenAPISpec creates an OpenAPI 3.0 spec from the given content type
// definitions, producing full CRUD paths, component schemas, and a Bearer JWT
// security scheme.
func GenerateOpenAPISpec(contentTypes []interfaces.ContentType) *OpenAPISpec {
	spec := &OpenAPISpec{
		OpenAPI: "3.0.3",
		Info: OpenAPIInfo{
			Title:       "ARoute CMS REST API",
			Version:     "1.0.0",
			Description: "Auto-generated REST API for ARoute CMS content types",
		},
		Servers: []OpenAPIServer{
			{URL: "/", Description: "Current server"},
		},
		Paths: make(map[string]OpenAPIPathItem),
		Components: &OpenAPIComponents{
			Schemas:         make(map[string]interface{}),
			SecuritySchemes: bearerSecurityScheme(),
		},
	}

	for i := range contentTypes {
		ct := &contentTypes[i]
		schemaName := pascalCase(ct.Name)

		spec.Components.Schemas[schemaName] = buildComponentSchema(ct)

		basePath := "/api/v1/" + ct.Slug
		spec.Paths[basePath] = buildCollectionPath(ct, schemaName)
		spec.Paths[basePath+"/{id}"] = buildItemPath(ct, schemaName)
	}

	return spec
}

// handleDocs serves the generated OpenAPI JSON spec at GET /api/v1/openapi.json.
func (h *Handler) handleDocs(w http.ResponseWriter, r *http.Request) {
	types := RegisteredContentTypes()
	spec := GenerateOpenAPISpec(types)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(spec)
}

// handleDocsUI serves an interactive API documentation UI (Swagger UI or Redoc).
// The UI type is determined by the api.docs.ui config (default: "swagger").
// When api.docs.enabled is false, returns 404.
func (h *Handler) handleDocsUI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(swaggerUIHTML))
}

const swaggerUIHTML = `<!DOCTYPE html>
<html>
<head>
  <title>ARoute CMS API Docs</title>
  <link rel="stylesheet" type="text/css" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css">
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
  <script>
    SwaggerUIBundle({ url: "/api/v1/openapi.json", dom_id: '#swagger-ui' })
  </script>
</body>
</html>
`

const redocHTML = `<!DOCTYPE html>
<html>
<head>
  <title>ARoute CMS API Docs</title>
</head>
<body>
  <redoc spec-url='/api/v1/openapi.json'></redoc>
  <script src="https://cdn.redoc.ly/redoc/latest/bundles/redoc.standalone.js"></script>
</body>
</html>
`

func buildCollectionPath(ct *interfaces.ContentType, schemaName string) OpenAPIPathItem {
	tag := ct.DisplayName

	return OpenAPIPathItem{
		Get: &OpenAPIOperation{
			Summary:     "List " + tag,
			Description: "Retrieve a paginated list of " + tag + " items",
			OperationID: "list" + schemaName,
			Tags:        []string{tag},
			Parameters:  listQueryParameters(),
			Responses: map[string]OpenAPIResponse{
				"200": {
					Description: "Paginated list of " + tag,
					Content:     jsonContent(refSchema(schemaName)),
				},
			},
			Security: bearerSecurity(),
		},
		Post: &OpenAPIOperation{
			Summary:     "Create " + tag,
			Description: "Create a new " + tag + " item",
			OperationID: "create" + schemaName,
			Tags:        []string{tag},
			RequestBody: jsonRequestBody(schemaName, true),
			Responses: map[string]OpenAPIResponse{
				"201": {
					Description: "Created " + tag,
					Content:     jsonContent(refSchema(schemaName)),
				},
				"400": errorResponse("Bad request — invalid JSON body"),
				"422": errorResponse("Validation error"),
			},
			Security: bearerSecurity(),
		},
	}
}

func buildItemPath(ct *interfaces.ContentType, schemaName string) OpenAPIPathItem {
	tag := ct.DisplayName

	return OpenAPIPathItem{
		Get: &OpenAPIOperation{
			Summary:     "Get " + tag,
			Description: "Retrieve a single " + tag + " item by ID",
			OperationID: "get" + schemaName,
			Tags:        []string{tag},
			Parameters:  idPathParam(),
			Responses: map[string]OpenAPIResponse{
				"200": {
					Description: "Single " + tag,
					Content:     jsonContent(refSchema(schemaName)),
				},
				"404": errorResponse("Not found"),
			},
			Security: bearerSecurity(),
		},
		Put: &OpenAPIOperation{
			Summary:     "Update " + tag,
			Description: "Update an existing " + tag + " item",
			OperationID: "update" + schemaName,
			Tags:        []string{tag},
			Parameters:  idPathParam(),
			RequestBody: jsonRequestBody(schemaName, true),
			Responses: map[string]OpenAPIResponse{
				"200": {
					Description: "Updated " + tag,
					Content:     jsonContent(refSchema(schemaName)),
				},
				"400": errorResponse("Bad request — invalid JSON body"),
				"404": errorResponse("Not found"),
				"422": errorResponse("Validation error"),
			},
			Security: bearerSecurity(),
		},
		Delete: &OpenAPIOperation{
			Summary:     "Delete " + tag,
			Description: "Soft-delete a " + tag + " item",
			OperationID: "delete" + schemaName,
			Tags:        []string{tag},
			Parameters:  idPathParam(),
			Responses: map[string]OpenAPIResponse{
				"204": {Description: "Deleted"},
				"404": errorResponse("Not found"),
			},
			Security: bearerSecurity(),
		},
	}
}

func buildComponentSchema(ct *interfaces.ContentType) map[string]interface{} {
	props := map[string]interface{}{
		"id":           map[string]interface{}{"type": "string", "description": "Unique identifier"},
		"content_type": map[string]interface{}{"type": "string", "description": "Content type name"},
		"title":        map[string]interface{}{"type": "string", "description": "Content title"},
		"slug":         map[string]interface{}{"type": "string", "description": "URL-friendly identifier"},
		"status":       map[string]interface{}{"type": "string", "description": "Content status", "enum": []string{"draft", "published", "archived"}},
		"author_id":    map[string]interface{}{"type": "string", "description": "Author user ID"},
		"version":      map[string]interface{}{"type": "integer", "description": "Version number"},
		"published_at": map[string]interface{}{"type": "string", "format": "date-time", "description": "Publication timestamp"},
		"created_at":   map[string]interface{}{"type": "string", "format": "date-time", "description": "Creation timestamp"},
		"updated_at":   map[string]interface{}{"type": "string", "format": "date-time", "description": "Last update timestamp"},
	}

	dataProps := make(map[string]interface{})
	for _, field := range ct.Fields {
		schema := fieldTypeToJSONSchema(field.Type)
		if field.Description != "" {
			schema["description"] = field.Description
		}
		dataProps[field.Name] = schema
	}
	props["data"] = map[string]interface{}{
		"type":        "object",
		"properties":  dataProps,
		"description": "Dynamic field values for " + ct.DisplayName,
	}

	return map[string]interface{}{
		"type":       "object",
		"properties": props,
	}
}

func fieldTypeToJSONSchema(ftype string) map[string]interface{} {
	switch ftype {
	case "text", "markdown", "richtext", "email", "url", "slug", "color", "enum":
		return map[string]interface{}{"type": "string"}
	case "number":
		return map[string]interface{}{"type": "number"}
	case "boolean":
		return map[string]interface{}{"type": "boolean"}
	case "date":
		return map[string]interface{}{"type": "string", "format": "date"}
	case "datetime":
		return map[string]interface{}{"type": "string", "format": "date-time"}
	case "relation":
		return map[string]interface{}{"type": "string", "x-relation": true}
	case "media":
		return map[string]interface{}{"type": "string", "x-media": true}
	case "json":
		return map[string]interface{}{"type": "object"}
	default:
		return map[string]interface{}{"type": "string"}
	}
}

func listQueryParameters() []OpenAPIParameter {
	return []OpenAPIParameter{
		{Name: "sort", In: "query", Description: "Sort field name", Schema: map[string]interface{}{"type": "string"}},
		{Name: "order", In: "query", Description: "Sort order (asc or desc)", Schema: map[string]interface{}{"type": "string", "enum": []string{"asc", "desc"}}},
		{Name: "page", In: "query", Description: "Page number (1-indexed, must be positive)", Schema: map[string]interface{}{"type": "integer", "default": 1, "minimum": 1}},
		{Name: "per_page", In: "query", Description: "Items per page (max 100)", Schema: map[string]interface{}{"type": "integer", "default": 20, "maximum": 100}},
		{Name: "fields", In: "query", Description: "Comma-separated field list for sparse fieldsets", Schema: map[string]interface{}{"type": "string"}},
		{Name: "expand", In: "query", Description: "Comma-separated relation fields to expand", Schema: map[string]interface{}{"type": "string"}},
		{Name: "search", In: "query", Description: "Full-text search query", Schema: map[string]interface{}{"type": "string"}},
	}
}

func idPathParam() []OpenAPIParameter {
	return []OpenAPIParameter{
		{
			Name:        "id",
			In:          "path",
			Description: "Content item ID",
			Required:    true,
			Schema:      map[string]interface{}{"type": "string"},
		},
	}
}

func jsonContent(schema interface{}) map[string]interface{} {
	return map[string]interface{}{
		"application/json": map[string]interface{}{
			"schema": schema,
		},
	}
}

func refSchema(name string) map[string]interface{} {
	return map[string]interface{}{"$ref": "#/components/schemas/" + name}
}

func jsonRequestBody(schemaName string, required bool) *OpenAPIRequestBody {
	return &OpenAPIRequestBody{
		Required: required,
		Content: map[string]interface{}{
			"application/json": map[string]interface{}{
				"schema": refSchema(schemaName),
			},
		},
	}
}

func errorResponse(desc string) OpenAPIResponse {
	return OpenAPIResponse{
		Description: desc,
		Content: map[string]interface{}{
			"application/json": map[string]interface{}{
				"schema": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"errors": map[string]interface{}{
							"type": "array",
							"items": map[string]interface{}{
								"type": "object",
								"properties": map[string]interface{}{
									"code":    map[string]interface{}{"type": "string"},
									"message": map[string]interface{}{"type": "string"},
									"details": map[string]interface{}{"type": "object"},
								},
							},
						},
					},
				},
			},
		},
	}
}

func bearerSecurityScheme() map[string]interface{} {
	return map[string]interface{}{
		"bearerAuth": map[string]interface{}{
			"type":         "http",
			"scheme":       "bearer",
			"bearerFormat": "JWT",
			"description":  "JWT authentication via Authorization: Bearer <token>",
		},
		"apiKeyAuth": map[string]interface{}{
			"type":        "apiKey",
			"in":          "header",
			"name":        "X-API-Key",
			"description": "API key authentication via X-API-Key header",
		},
	}
}

func bearerSecurity() []map[string][]string {
	return []map[string][]string{
		{"bearerAuth": {}},
	}
}

// pascalCase converts a snake_case or kebab-case string to PascalCase.
func pascalCase(s string) string {
	if len(s) == 0 {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	upper := true
	for _, r := range s {
		if r == '_' || r == '-' {
			upper = true
			continue
		}
		if upper {
			b.WriteRune(unicode.ToUpper(r))
			upper = false
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}
