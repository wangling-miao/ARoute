package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wangling-miao/aroute/sdk/interfaces"
)

// ---------------------------------------------------------------------------
// mock ContentService
// ---------------------------------------------------------------------------

type mockContentService struct {
	createFunc         func(ctx context.Context, contentType string, data map[string]interface{}) (*interfaces.Content, error)
	getByIDFunc        func(ctx context.Context, id string) (*interfaces.Content, error)
	updateFunc         func(ctx context.Context, id string, data map[string]interface{}) (*interfaces.Content, error)
	deleteFunc         func(ctx context.Context, id string) error
	listFunc           func(ctx context.Context, contentType string, query *interfaces.ListQuery) (*interfaces.Page, error)
	getContentTypeFunc func(ctx context.Context, name string) (*interfaces.ContentType, error)
	createCTFunc       func(ctx context.Context, ct *interfaces.ContentType) (*interfaces.ContentType, error)
	updateCTFunc       func(ctx context.Context, name string, ct *interfaces.ContentType) (*interfaces.ContentType, error)
	deleteCTFunc       func(ctx context.Context, name string) error
}

func (m *mockContentService) Create(ctx context.Context, contentType string, data map[string]interface{}) (*interfaces.Content, error) {
	if m.createFunc != nil {
		return m.createFunc(ctx, contentType, data)
	}
	return nil, nil
}

func (m *mockContentService) GetByID(ctx context.Context, id string) (*interfaces.Content, error) {
	if m.getByIDFunc != nil {
		return m.getByIDFunc(ctx, id)
	}
	return nil, nil
}

func (m *mockContentService) Update(ctx context.Context, id string, data map[string]interface{}) (*interfaces.Content, error) {
	if m.updateFunc != nil {
		return m.updateFunc(ctx, id, data)
	}
	return nil, nil
}

func (m *mockContentService) Delete(ctx context.Context, id string) error {
	if m.deleteFunc != nil {
		return m.deleteFunc(ctx, id)
	}
	return nil
}

func (m *mockContentService) List(ctx context.Context, contentType string, query *interfaces.ListQuery) (*interfaces.Page, error) {
	if m.listFunc != nil {
		return m.listFunc(ctx, contentType, query)
	}
	return nil, nil
}

func (m *mockContentService) GetContentType(ctx context.Context, name string) (*interfaces.ContentType, error) {
	if m.getContentTypeFunc != nil {
		return m.getContentTypeFunc(ctx, name)
	}
	return nil, nil
}

func (m *mockContentService) CreateContentType(ctx context.Context, ct *interfaces.ContentType) (*interfaces.ContentType, error) {
	if m.createCTFunc != nil {
		return m.createCTFunc(ctx, ct)
	}
	return nil, nil
}

func (m *mockContentService) UpdateContentType(ctx context.Context, name string, ct *interfaces.ContentType) (*interfaces.ContentType, error) {
	if m.updateCTFunc != nil {
		return m.updateCTFunc(ctx, name, ct)
	}
	return nil, nil
}

func (m *mockContentService) DeleteContentType(ctx context.Context, name string) error {
	if m.deleteCTFunc != nil {
		return m.deleteCTFunc(ctx, name)
	}
	return nil
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func sampleContent(id, ct string) *interfaces.Content {
	return &interfaces.Content{
		ID:          id,
		ContentType: ct,
		Title:       "Test " + id,
		Slug:        "test-" + id,
		Data:        map[string]interface{}{"body": "Hello"},
		AuthorID:    "user-1",
		Status:      "published",
		CreatedAt:   time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		UpdatedAt:   time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		Version:     1,
	}
}

func samplePage(contents []*interfaces.Content) *interfaces.Page {
	return &interfaces.Page{
		Data: contents,
		Meta: interfaces.PageMeta{
			Total:      int64(len(contents)),
			Page:       1,
			PerPage:    20,
			TotalPages: 1,
			HasPrev:    false,
			HasNext:    false,
		},
	}
}

func setupRouter(h *Handler) *chi.Mux {
	r := chi.NewRouter()
	r.Get("/api/v1/{contentType}", h.List)
	r.Get("/api/v1/{contentType}/{id}", h.Get)
	r.Post("/api/v1/{contentType}", h.Create)
	r.Put("/api/v1/{contentType}/{id}", h.Update)
	r.Delete("/api/v1/{contentType}/{id}", h.Delete)
	r.Get("/api/v1/content-types", h.ListContentTypes)
	return r
}

func doRequest(t *testing.T, router *chi.Mux, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var bodyReader io.Reader
	if body != "" {
		bodyReader = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, path, bodyReader)
	require.NoError(t, err)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	return rr
}

func decodeAPIResponse(t *testing.T, body []byte) APIResponse {
	t.Helper()
	var resp APIResponse
	require.NoError(t, json.Unmarshal(body, &resp))
	return resp
}

func decodeErrorsEnvelope(t *testing.T, body []byte) ErrorsEnvelope {
	t.Helper()
	var envelope ErrorsEnvelope
	require.NoError(t, json.Unmarshal(body, &envelope))
	return envelope
}

func firstAPIError(t *testing.T, body []byte) APIError {
	env := decodeErrorsEnvelope(t, body)
	require.NotEmpty(t, env.Errors, "expected at least one error")
	return env.Errors[0]
}

// ===========================================================================
// List handler tests
// ===========================================================================

func TestList_Success(t *testing.T) {
	mock := &mockContentService{
		getContentTypeFunc: func(_ context.Context, ct string) (*interfaces.ContentType, error) {
			assert.Equal(t, "post", ct)
			return &interfaces.ContentType{Name: "post"}, nil
		},
		listFunc: func(_ context.Context, ct string, _ *interfaces.ListQuery) (*interfaces.Page, error) {
			assert.Equal(t, "post", ct)
			c1 := sampleContent("id-1", "post")
			c2 := sampleContent("id-2", "post")
			return samplePage([]*interfaces.Content{c1, c2}), nil
		},
	}
	h := NewHandler(mock)
	router := setupRouter(h)

	rr := doRequest(t, router, http.MethodGet, "/api/v1/post", "")
	assert.Equal(t, http.StatusOK, rr.Code)

	var resp APIResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.NotNil(t, resp.Data)

	metaBytes, _ := json.Marshal(resp.Meta)
	var meta PageMeta
	require.NoError(t, json.Unmarshal(metaBytes, &meta))
	assert.Equal(t, int64(2), meta.TotalCount)
	assert.Equal(t, 1, meta.Page)
	assert.Equal(t, 20, meta.PerPage)
	assert.Equal(t, 1, meta.TotalPages)
}

func TestList_EmptyResult(t *testing.T) {
	mock := &mockContentService{
		getContentTypeFunc: func(_ context.Context, _ string) (*interfaces.ContentType, error) {
			return &interfaces.ContentType{Name: "post"}, nil
		},
		listFunc: func(_ context.Context, _ string, _ *interfaces.ListQuery) (*interfaces.Page, error) {
			return samplePage([]*interfaces.Content{}), nil
		},
	}
	h := NewHandler(mock)
	router := setupRouter(h)

	rr := doRequest(t, router, http.MethodGet, "/api/v1/post", "")
	assert.Equal(t, http.StatusOK, rr.Code)

	resp := decodeAPIResponse(t, rr.Body.Bytes())
	dataBytes, _ := json.Marshal(resp.Data)
	assert.Equal(t, "[]", string(dataBytes))
}

func TestList_WithQueryParams(t *testing.T) {
	var captured *interfaces.ListQuery
	mock := &mockContentService{
		getContentTypeFunc: func(_ context.Context, _ string) (*interfaces.ContentType, error) {
			return &interfaces.ContentType{Name: "post"}, nil
		},
		listFunc: func(_ context.Context, _ string, q *interfaces.ListQuery) (*interfaces.Page, error) {
			captured = q
			return samplePage([]*interfaces.Content{}), nil
		},
	}
	h := NewHandler(mock)
	router := setupRouter(h)

	path := "/api/v1/post?page=3&per_page=50&sort=title&order=desc&status=published&fields=id,title&expand=author&search=golang"
	rr := doRequest(t, router, http.MethodGet, path, "")
	assert.Equal(t, http.StatusOK, rr.Code)
	require.NotNil(t, captured)

	assert.Equal(t, 3, captured.Page)
	assert.Equal(t, 50, captured.PerPage)
	assert.Equal(t, "title", captured.Sort)
	assert.Equal(t, "desc", captured.Order)
	assert.Equal(t, "published", captured.Filters["status"])
	assert.Equal(t, []string{"id", "title"}, captured.Fields)
	assert.Equal(t, []string{"author"}, captured.Expand)
	assert.Equal(t, "golang", captured.Search)
}

func TestList_ErrNotFound(t *testing.T) {
	mock := &mockContentService{
		getContentTypeFunc: func(_ context.Context, ct string) (*interfaces.ContentType, error) {
			return nil, fmt.Errorf("wrap: %w", interfaces.ErrNotFound)
		},
		listFunc: func(_ context.Context, _ string, _ *interfaces.ListQuery) (*interfaces.Page, error) {
			return nil, fmt.Errorf("wrap: %w", interfaces.ErrNotFound)
		},
	}
	h := NewHandler(mock)
	router := setupRouter(h)

	rr := doRequest(t, router, http.MethodGet, "/api/v1/unknown", "")
	assert.Equal(t, http.StatusNotFound, rr.Code)

	apiErr := firstAPIError(t, rr.Body.Bytes())
	assert.Equal(t, "NOT_FOUND", apiErr.Code)
	assert.Contains(t, apiErr.Message, "not found")
}

func TestList_ContentTypeNotExist(t *testing.T) {
	mock := &mockContentService{
		getContentTypeFunc: func(_ context.Context, ct string) (*interfaces.ContentType, error) {
			return nil, interfaces.ErrNotFound
		},
	}
	h := NewHandler(mock)
	router := setupRouter(h)

	rr := doRequest(t, router, http.MethodGet, "/api/v1/nonexistent", "")
	assert.Equal(t, http.StatusNotFound, rr.Code)

	apiErr := firstAPIError(t, rr.Body.Bytes())
	assert.Equal(t, "NOT_FOUND", apiErr.Code)
	assert.Equal(t, "content type 'nonexistent' not found", apiErr.Message)
}

func TestList_ErrValidation(t *testing.T) {
	mock := &mockContentService{
		getContentTypeFunc: func(_ context.Context, _ string) (*interfaces.ContentType, error) {
			return &interfaces.ContentType{Name: "post"}, nil
		},
		listFunc: func(_ context.Context, _ string, _ *interfaces.ListQuery) (*interfaces.Page, error) {
			return nil, interfaces.ErrValidation
		},
	}
	h := NewHandler(mock)
	router := setupRouter(h)

	rr := doRequest(t, router, http.MethodGet, "/api/v1/post", "")
	assert.Equal(t, http.StatusUnprocessableEntity, rr.Code)

	apiErr := firstAPIError(t, rr.Body.Bytes())
	assert.Equal(t, "VALIDATION_ERROR", apiErr.Code)
}

func TestList_EmptyContentType(t *testing.T) {
	h := NewHandler(&mockContentService{})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/", nil)
	h.List(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	apiErr := firstAPIError(t, rr.Body.Bytes())
	assert.Equal(t, "BAD_REQUEST", apiErr.Code)
}

func TestList_GenericError(t *testing.T) {
	mock := &mockContentService{
		getContentTypeFunc: func(_ context.Context, _ string) (*interfaces.ContentType, error) {
			return &interfaces.ContentType{Name: "post"}, nil
		},
		listFunc: func(_ context.Context, _ string, _ *interfaces.ListQuery) (*interfaces.Page, error) {
			return nil, fmt.Errorf("database exploded")
		},
	}
	h := NewHandler(mock)
	router := setupRouter(h)

	rr := doRequest(t, router, http.MethodGet, "/api/v1/post", "")
	assert.Equal(t, http.StatusInternalServerError, rr.Code)

	apiErr := firstAPIError(t, rr.Body.Bytes())
	assert.Equal(t, "INTERNAL_ERROR", apiErr.Code)
	assert.Equal(t, "an unexpected error occurred", apiErr.Message)
}

// ===========================================================================
// Get handler tests
// ===========================================================================

func TestGet_Success(t *testing.T) {
	content := sampleContent("abc-123", "post")
	mock := &mockContentService{
		getByIDFunc: func(_ context.Context, id string) (*interfaces.Content, error) {
			assert.Equal(t, "abc-123", id)
			return content, nil
		},
	}
	h := NewHandler(mock)
	router := setupRouter(h)

	rr := doRequest(t, router, http.MethodGet, "/api/v1/post/abc-123", "")
	assert.Equal(t, http.StatusOK, rr.Code)

	resp := decodeAPIResponse(t, rr.Body.Bytes())
	dataBytes, _ := json.Marshal(resp.Data)
	var got interfaces.Content
	require.NoError(t, json.Unmarshal(dataBytes, &got))
	assert.Equal(t, "abc-123", got.ID)
	assert.Equal(t, "post", got.ContentType)
}

func TestGet_MissingID(t *testing.T) {
	h := NewHandler(&mockContentService{})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/post/", nil)
	h.Get(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	apiErr := firstAPIError(t, rr.Body.Bytes())
	assert.Equal(t, "BAD_REQUEST", apiErr.Code)
}

func TestGet_ErrNotFound(t *testing.T) {
	mock := &mockContentService{
		getByIDFunc: func(_ context.Context, _ string) (*interfaces.Content, error) {
			return nil, interfaces.ErrNotFound
		},
	}
	h := NewHandler(mock)
	router := setupRouter(h)

	rr := doRequest(t, router, http.MethodGet, "/api/v1/post/missing", "")
	assert.Equal(t, http.StatusNotFound, rr.Code)

	apiErr := firstAPIError(t, rr.Body.Bytes())
	assert.Equal(t, "NOT_FOUND", apiErr.Code)
}

func TestGet_GenericError(t *testing.T) {
	mock := &mockContentService{
		getByIDFunc: func(_ context.Context, _ string) (*interfaces.Content, error) {
			return nil, fmt.Errorf("db down")
		},
	}
	h := NewHandler(mock)
	router := setupRouter(h)

	rr := doRequest(t, router, http.MethodGet, "/api/v1/post/x", "")
	assert.Equal(t, http.StatusInternalServerError, rr.Code)

	apiErr := firstAPIError(t, rr.Body.Bytes())
	assert.Equal(t, "INTERNAL_ERROR", apiErr.Code)
}

// ===========================================================================
// Create handler tests
// ===========================================================================

func TestCreate_Success(t *testing.T) {
	created := sampleContent("new-1", "post")
	mock := &mockContentService{
		createFunc: func(_ context.Context, ct string, data map[string]interface{}) (*interfaces.Content, error) {
			assert.Equal(t, "post", ct)
			assert.Equal(t, "Hello", data["title"])
			return created, nil
		},
	}
	h := NewHandler(mock)
	router := setupRouter(h)

	body := `{"title":"Hello","body":"World"}`
	rr := doRequest(t, router, http.MethodPost, "/api/v1/post", body)

	assert.Equal(t, http.StatusCreated, rr.Code)
	assert.Equal(t, "/api/v1/post/new-1", rr.Header().Get("Location"))

	resp := decodeAPIResponse(t, rr.Body.Bytes())
	dataBytes, _ := json.Marshal(resp.Data)
	var got interfaces.Content
	require.NoError(t, json.Unmarshal(dataBytes, &got))
	assert.Equal(t, "new-1", got.ID)
}

func TestCreate_InvalidJSON(t *testing.T) {
	h := NewHandler(&mockContentService{})
	router := setupRouter(h)

	rr := doRequest(t, router, http.MethodPost, "/api/v1/post", "{invalid}")
	assert.Equal(t, http.StatusBadRequest, rr.Code)

	apiErr := firstAPIError(t, rr.Body.Bytes())
	assert.Equal(t, "INVALID_JSON", apiErr.Code)
	assert.Equal(t, "request body is not valid JSON", apiErr.Message)
}

func TestCreate_ErrValidation(t *testing.T) {
	mock := &mockContentService{
		createFunc: func(_ context.Context, _ string, _ map[string]interface{}) (*interfaces.Content, error) {
			return nil, interfaces.ErrValidation
		},
	}
	h := NewHandler(mock)
	router := setupRouter(h)

	rr := doRequest(t, router, http.MethodPost, "/api/v1/post", `{"title":"x"}`)
	assert.Equal(t, http.StatusUnprocessableEntity, rr.Code)

	apiErr := firstAPIError(t, rr.Body.Bytes())
	assert.Equal(t, "VALIDATION_ERROR", apiErr.Code)
}

func TestCreate_ErrConflict(t *testing.T) {
	mock := &mockContentService{
		createFunc: func(_ context.Context, _ string, _ map[string]interface{}) (*interfaces.Content, error) {
			return nil, interfaces.ErrConflict
		},
	}
	h := NewHandler(mock)
	router := setupRouter(h)

	rr := doRequest(t, router, http.MethodPost, "/api/v1/post", `{"title":"dup"}`)
	assert.Equal(t, http.StatusConflict, rr.Code)

	apiErr := firstAPIError(t, rr.Body.Bytes())
	assert.Equal(t, "CONFLICT", apiErr.Code)
}

func TestCreate_EmptyContentType(t *testing.T) {
	h := NewHandler(&mockContentService{})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/", strings.NewReader(`{"title":"x"}`))
	h.Create(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	apiErr := firstAPIError(t, rr.Body.Bytes())
	assert.Equal(t, "BAD_REQUEST", apiErr.Code)
}

// ===========================================================================
// Update handler tests
// ===========================================================================

func TestUpdate_MissingID(t *testing.T) {
	h := NewHandler(&mockContentService{})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/post/", strings.NewReader(`{"title":"x"}`))
	h.Update(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	apiErr := firstAPIError(t, rr.Body.Bytes())
	assert.Equal(t, "BAD_REQUEST", apiErr.Code)
}

func TestUpdate_Success(t *testing.T) {
	updated := sampleContent("upd-1", "post")
	updated.Title = "Updated Title"
	mock := &mockContentService{
		updateFunc: func(_ context.Context, id string, data map[string]interface{}) (*interfaces.Content, error) {
			assert.Equal(t, "upd-1", id)
			return updated, nil
		},
	}
	h := NewHandler(mock)
	router := setupRouter(h)

	rr := doRequest(t, router, http.MethodPut, "/api/v1/post/upd-1", `{"title":"Updated Title"}`)
	assert.Equal(t, http.StatusOK, rr.Code)

	resp := decodeAPIResponse(t, rr.Body.Bytes())
	dataBytes, _ := json.Marshal(resp.Data)
	var got interfaces.Content
	require.NoError(t, json.Unmarshal(dataBytes, &got))
	assert.Equal(t, "Updated Title", got.Title)
}

func TestUpdate_InvalidJSON(t *testing.T) {
	h := NewHandler(&mockContentService{})
	router := setupRouter(h)

	rr := doRequest(t, router, http.MethodPut, "/api/v1/post/1", "not-json")
	assert.Equal(t, http.StatusBadRequest, rr.Code)

	apiErr := firstAPIError(t, rr.Body.Bytes())
	assert.Equal(t, "INVALID_JSON", apiErr.Code)
}

func TestUpdate_ErrNotFound(t *testing.T) {
	mock := &mockContentService{
		updateFunc: func(_ context.Context, _ string, _ map[string]interface{}) (*interfaces.Content, error) {
			return nil, interfaces.ErrNotFound
		},
	}
	h := NewHandler(mock)
	router := setupRouter(h)

	rr := doRequest(t, router, http.MethodPut, "/api/v1/post/gone", `{"title":"x"}`)
	assert.Equal(t, http.StatusNotFound, rr.Code)
}

func TestUpdate_ValidationErrorsWithDetails(t *testing.T) {
	ve := interfaces.NewValidationErrors()
	ve.Add("title", "title is required")
	ve.Add("body", "body is too short")

	mock := &mockContentService{
		updateFunc: func(_ context.Context, _ string, _ map[string]interface{}) (*interfaces.Content, error) {
			return nil, ve
		},
	}
	h := NewHandler(mock)
	router := setupRouter(h)

	rr := doRequest(t, router, http.MethodPut, "/api/v1/post/1", `{"title":""}`)
	assert.Equal(t, http.StatusUnprocessableEntity, rr.Code)

	envelope := decodeErrorsEnvelope(t, rr.Body.Bytes())
	require.Len(t, envelope.Errors, 2)

	assert.Equal(t, "VALIDATION_ERROR", envelope.Errors[0].Code)
	assert.Equal(t, "title is required", envelope.Errors[0].Message)
	details0, ok := envelope.Errors[0].Details.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "title", details0["field"])

	assert.Equal(t, "VALIDATION_ERROR", envelope.Errors[1].Code)
	assert.Equal(t, "body is too short", envelope.Errors[1].Message)
	details1, ok := envelope.Errors[1].Details.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "body", details1["field"])
}

// ===========================================================================
// Delete handler tests
// ===========================================================================

func TestDelete_MissingID(t *testing.T) {
	h := NewHandler(&mockContentService{})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/post/", nil)
	h.Delete(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	apiErr := firstAPIError(t, rr.Body.Bytes())
	assert.Equal(t, "BAD_REQUEST", apiErr.Code)
}

func TestDelete_Success(t *testing.T) {
	mock := &mockContentService{
		deleteFunc: func(_ context.Context, id string) error {
			assert.Equal(t, "del-1", id)
			return nil
		},
	}
	h := NewHandler(mock)
	router := setupRouter(h)

	rr := doRequest(t, router, http.MethodDelete, "/api/v1/post/del-1", "")
	assert.Equal(t, http.StatusNoContent, rr.Code)
	assert.Empty(t, rr.Body.Bytes())
}

func TestDelete_ErrNotFound(t *testing.T) {
	mock := &mockContentService{
		deleteFunc: func(_ context.Context, _ string) error {
			return interfaces.ErrNotFound
		},
	}
	h := NewHandler(mock)
	router := setupRouter(h)

	rr := doRequest(t, router, http.MethodDelete, "/api/v1/post/ghost", "")
	assert.Equal(t, http.StatusNotFound, rr.Code)
}

// ===========================================================================
// ListContentTypes handler tests
// ===========================================================================

func TestListContentTypes_ReturnsEmptyArray(t *testing.T) {
	h := NewHandler(&mockContentService{})
	router := setupRouter(h)

	rr := doRequest(t, router, http.MethodGet, "/api/v1/content-types", "")
	assert.Equal(t, http.StatusOK, rr.Code)

	resp := decodeAPIResponse(t, rr.Body.Bytes())
	dataBytes, _ := json.Marshal(resp.Data)
	assert.Equal(t, "[]", string(dataBytes))
}

// ===========================================================================
// buildListQuery tests
// ===========================================================================

func TestBuildListQuery_Defaults(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/post", nil)
	q, warnings, err := buildListQuery(req)

	require.NoError(t, err)
	assert.Empty(t, warnings)
	assert.Equal(t, 1, q.Page)
	assert.Equal(t, 20, q.PerPage)
	assert.Equal(t, map[string]interface{}{}, q.Filters)
	assert.Equal(t, "created_at", q.Sort)
	assert.Equal(t, "desc", q.Order)
	assert.Empty(t, q.Fields)
	assert.Empty(t, q.Expand)
	assert.Empty(t, q.Search)
}

func TestBuildListQuery_CustomPageAndPerPage(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/post?page=5&per_page=30", nil)
	q, _, err := buildListQuery(req)

	require.NoError(t, err)
	assert.Equal(t, 5, q.Page)
	assert.Equal(t, 30, q.PerPage)
}

func TestBuildListQuery_PerPageCappedAt100(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/post?per_page=999", nil)
	q, _, err := buildListQuery(req)

	require.NoError(t, err)
	assert.Equal(t, 100, q.PerPage)
}

func TestBuildListQuery_InvalidPageReturnsError(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/post?page=abc", nil)
	_, _, err := buildListQuery(req)

	require.Error(t, err)
	assert.Equal(t, "page must be a positive integer", err.Error())
}

func TestBuildListQuery_ZeroPageReturnsError(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/post?page=0", nil)
	_, _, err := buildListQuery(req)

	require.Error(t, err)
	assert.Equal(t, "page must be a positive integer", err.Error())
}

func TestBuildListQuery_NegativePageReturnsError(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/post?page=-1", nil)
	_, _, err := buildListQuery(req)

	require.Error(t, err)
	assert.Equal(t, "page must be a positive integer", err.Error())
}

func TestBuildListQuery_IndividualFilterParams(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/post?status=published&price_gte=10", nil)
	q, _, err := buildListQuery(req)

	require.NoError(t, err)
	require.NotNil(t, q.Filters)
	assert.Equal(t, "published", q.Filters["status"])
	assert.Equal(t, "10", q.Filters["price_gte"])
}

func TestBuildListQuery_FilterCommaValues(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/post?tags=go,testing,api", nil)
	q, _, err := buildListQuery(req)

	require.NoError(t, err)
	tags, ok := q.Filters["tags"].([]string)
	require.True(t, ok)
	assert.Equal(t, []string{"go", "testing", "api"}, tags)
}

func TestBuildListQuery_FilterBooleanValues(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/post?featured=true", nil)
	q, _, err := buildListQuery(req)

	require.NoError(t, err)
	assert.Equal(t, true, q.Filters["featured"])
}

func TestBuildListQuery_FilterBooleanFalse(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/post?published=false", nil)
	q, _, err := buildListQuery(req)

	require.NoError(t, err)
	assert.Equal(t, false, q.Filters["published"])
}

func TestBuildListQuery_FieldsAndExpand(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/post?fields=id,title,slug&expand=author,tags", nil)
	q, _, err := buildListQuery(req)

	require.NoError(t, err)
	assert.Equal(t, []string{"id", "title", "slug"}, q.Fields)
	assert.Equal(t, []string{"author", "tags"}, q.Expand)
}

func TestBuildListQuery_SortAndOrder(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/post?sort=created_at&order=asc", nil)
	q, _, err := buildListQuery(req)

	require.NoError(t, err)
	assert.Equal(t, "created_at", q.Sort)
	assert.Equal(t, "asc", q.Order)
}

func TestBuildListQuery_Search(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/post?search=golang+testing", nil)
	q, _, err := buildListQuery(req)

	require.NoError(t, err)
	assert.Equal(t, "golang testing", q.Search)
}

func TestBuildListQuery_NestedExpandWarning(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/post?expand=author.posts", nil)
	q, warnings, err := buildListQuery(req)

	require.NoError(t, err)
	assert.Equal(t, []string{"author.posts"}, q.Expand)
	require.Len(t, warnings, 1)
	assert.Equal(t, "nested expansion is not supported", warnings[0])
}

func TestBuildListQuery_ContainsFilter(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/post?title_contains=golang", nil)
	q, _, err := buildListQuery(req)

	require.NoError(t, err)
	assert.Equal(t, "golang", q.Filters["title_contains"])
}

func TestBuildListQuery_GteLteFilters(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/post?price_gte=10&price_lte=100", nil)
	q, _, err := buildListQuery(req)

	require.NoError(t, err)
	assert.Equal(t, "10", q.Filters["price_gte"])
	assert.Equal(t, "100", q.Filters["price_lte"])
}

// ===========================================================================
// mapErrorToHTTP tests
// ===========================================================================

func TestMapError_ErrNotFound(t *testing.T) {
	rr := httptest.NewRecorder()
	mapErrorToHTTP(rr, interfaces.ErrNotFound)
	assert.Equal(t, http.StatusNotFound, rr.Code)

	apiErr := firstAPIError(t, rr.Body.Bytes())
	assert.Equal(t, "NOT_FOUND", apiErr.Code)
}

func TestMapError_ErrUnauthorized(t *testing.T) {
	rr := httptest.NewRecorder()
	mapErrorToHTTP(rr, interfaces.ErrUnauthorized)
	assert.Equal(t, http.StatusUnauthorized, rr.Code)

	apiErr := firstAPIError(t, rr.Body.Bytes())
	assert.Equal(t, "UNAUTHORIZED", apiErr.Code)
}

func TestMapError_ErrForbidden(t *testing.T) {
	rr := httptest.NewRecorder()
	mapErrorToHTTP(rr, interfaces.ErrForbidden)
	assert.Equal(t, http.StatusForbidden, rr.Code)

	apiErr := firstAPIError(t, rr.Body.Bytes())
	assert.Equal(t, "FORBIDDEN", apiErr.Code)
}

func TestMapError_ErrBadRequest(t *testing.T) {
	rr := httptest.NewRecorder()
	mapErrorToHTTP(rr, interfaces.ErrBadRequest)
	assert.Equal(t, http.StatusBadRequest, rr.Code)

	apiErr := firstAPIError(t, rr.Body.Bytes())
	assert.Equal(t, "BAD_REQUEST", apiErr.Code)
}

func TestMapError_ErrConflict(t *testing.T) {
	rr := httptest.NewRecorder()
	mapErrorToHTTP(rr, interfaces.ErrConflict)
	assert.Equal(t, http.StatusConflict, rr.Code)

	apiErr := firstAPIError(t, rr.Body.Bytes())
	assert.Equal(t, "CONFLICT", apiErr.Code)
}

func TestMapError_ValidationErrors(t *testing.T) {
	ve := interfaces.NewValidationErrors()
	ve.Add("email", "invalid email format")
	ve.Add("name", "name is required", "required")

	rr := httptest.NewRecorder()
	mapErrorToHTTP(rr, ve)

	assert.Equal(t, http.StatusUnprocessableEntity, rr.Code)

	envelope := decodeErrorsEnvelope(t, rr.Body.Bytes())
	require.Len(t, envelope.Errors, 2)

	assert.Equal(t, "VALIDATION_ERROR", envelope.Errors[0].Code)
	assert.Equal(t, "invalid email format", envelope.Errors[0].Message)
	details0, ok := envelope.Errors[0].Details.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "email", details0["field"])

	assert.Equal(t, "VALIDATION_ERROR", envelope.Errors[1].Code)
	assert.Equal(t, "name is required", envelope.Errors[1].Message)
	details1, ok := envelope.Errors[1].Details.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "name", details1["field"])
}

func TestMapError_ErrValidationPlain(t *testing.T) {
	rr := httptest.NewRecorder()
	mapErrorToHTTP(rr, interfaces.ErrValidation)

	assert.Equal(t, http.StatusUnprocessableEntity, rr.Code)

	apiErr := firstAPIError(t, rr.Body.Bytes())
	assert.Equal(t, "VALIDATION_ERROR", apiErr.Code)
	assert.Equal(t, "validation failed", apiErr.Message)
}

func TestMapError_UnknownError(t *testing.T) {
	rr := httptest.NewRecorder()
	mapErrorToHTTP(rr, fmt.Errorf("something unexpected"))

	assert.Equal(t, http.StatusInternalServerError, rr.Code)

	apiErr := firstAPIError(t, rr.Body.Bytes())
	assert.Equal(t, "INTERNAL_ERROR", apiErr.Code)
	assert.Equal(t, "an unexpected error occurred", apiErr.Message)
}

// ===========================================================================
// M5: ListContentTypes tests
// ===========================================================================

func TestListContentTypes_ReturnsRegisteredTypes(t *testing.T) {
	defer setTestRegistry([]interfaces.ContentType{
		sampleContentType("post", "posts", "Posts", "Blog posts", []interfaces.Field{}),
		sampleContentType("page", "pages", "Pages", "Static pages", []interfaces.Field{}),
	})()

	h := NewHandler(&mockContentService{})
	router := setupRouter(h)

	rr := doRequest(t, router, http.MethodGet, "/api/v1/content-types", "")
	assert.Equal(t, http.StatusOK, rr.Code)

	resp := decodeAPIResponse(t, rr.Body.Bytes())
	dataBytes, _ := json.Marshal(resp.Data)

	var types []map[string]interface{}
	require.NoError(t, json.Unmarshal(dataBytes, &types))
	assert.Len(t, types, 2)
}

// ===========================================================================
// M1: Sort field validation tests
// ===========================================================================

func TestList_InvalidSortField_Returns400(t *testing.T) {
	mock := &mockContentService{
		getContentTypeFunc: func(_ context.Context, _ string) (*interfaces.ContentType, error) {
			return &interfaces.ContentType{
				Name:   "post",
				Fields: []interfaces.Field{{Name: "title", Type: "text"}},
			}, nil
		},
	}
	h := NewHandler(mock)
	router := setupRouter(h)

	rr := doRequest(t, router, http.MethodGet, "/api/v1/post?sort=nonexistent_field", "")
	assert.Equal(t, http.StatusBadRequest, rr.Code)

	apiErr := firstAPIError(t, rr.Body.Bytes())
	assert.Equal(t, "BAD_REQUEST", apiErr.Code)
	assert.Contains(t, apiErr.Message, "unknown sort field: nonexistent_field")
}

func TestList_ValidSortField_CTField_Succeeds(t *testing.T) {
	var captured *interfaces.ListQuery
	mock := &mockContentService{
		getContentTypeFunc: func(_ context.Context, _ string) (*interfaces.ContentType, error) {
			return &interfaces.ContentType{
				Name:   "post",
				Fields: []interfaces.Field{{Name: "title", Type: "text"}},
			}, nil
		},
		listFunc: func(_ context.Context, _ string, q *interfaces.ListQuery) (*interfaces.Page, error) {
			captured = q
			return samplePage([]*interfaces.Content{}), nil
		},
	}
	h := NewHandler(mock)
	router := setupRouter(h)

	rr := doRequest(t, router, http.MethodGet, "/api/v1/post?sort=title&order=desc", "")
	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "title", captured.Sort)
	assert.Equal(t, "desc", captured.Order)
}

func TestList_ValidSortField_SystemField_Succeeds(t *testing.T) {
	mock := &mockContentService{
		getContentTypeFunc: func(_ context.Context, _ string) (*interfaces.ContentType, error) {
			return &interfaces.ContentType{Name: "post"}, nil
		},
		listFunc: func(_ context.Context, _ string, q *interfaces.ListQuery) (*interfaces.Page, error) {
			return samplePage([]*interfaces.Content{}), nil
		},
	}
	h := NewHandler(mock)
	router := setupRouter(h)

	rr := doRequest(t, router, http.MethodGet, "/api/v1/post?sort=created_at&order=asc", "")
	assert.Equal(t, http.StatusOK, rr.Code)
}

// ===========================================================================
// M4: Default sort order by field type tests
// ===========================================================================

func TestList_DefaultOrder_TextField_Asc(t *testing.T) {
	var captured *interfaces.ListQuery
	mock := &mockContentService{
		getContentTypeFunc: func(_ context.Context, _ string) (*interfaces.ContentType, error) {
			return &interfaces.ContentType{
				Name:   "post",
				Fields: []interfaces.Field{{Name: "title", Type: "text"}},
			}, nil
		},
		listFunc: func(_ context.Context, _ string, q *interfaces.ListQuery) (*interfaces.Page, error) {
			captured = q
			return samplePage([]*interfaces.Content{}), nil
		},
	}
	h := NewHandler(mock)
	router := setupRouter(h)

	rr := doRequest(t, router, http.MethodGet, "/api/v1/post?sort=title", "")
	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "title", captured.Sort)
	assert.Equal(t, "asc", captured.Order, "text fields should default to asc")
}

func TestList_DefaultOrder_NumberField_Desc(t *testing.T) {
	var captured *interfaces.ListQuery
	mock := &mockContentService{
		getContentTypeFunc: func(_ context.Context, _ string) (*interfaces.ContentType, error) {
			return &interfaces.ContentType{
				Name:   "product",
				Fields: []interfaces.Field{{Name: "price", Type: "number"}},
			}, nil
		},
		listFunc: func(_ context.Context, _ string, q *interfaces.ListQuery) (*interfaces.Page, error) {
			captured = q
			return samplePage([]*interfaces.Content{}), nil
		},
	}
	h := NewHandler(mock)
	router := setupRouter(h)

	rr := doRequest(t, router, http.MethodGet, "/api/v1/product?sort=price", "")
	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "price", captured.Sort)
	assert.Equal(t, "desc", captured.Order, "number fields should default to desc")
}

func TestList_DefaultOrder_DateField_Desc(t *testing.T) {
	var captured *interfaces.ListQuery
	mock := &mockContentService{
		getContentTypeFunc: func(_ context.Context, _ string) (*interfaces.ContentType, error) {
			return &interfaces.ContentType{
				Name:   "event",
				Fields: []interfaces.Field{{Name: "event_date", Type: "date"}},
			}, nil
		},
		listFunc: func(_ context.Context, _ string, q *interfaces.ListQuery) (*interfaces.Page, error) {
			captured = q
			return samplePage([]*interfaces.Content{}), nil
		},
	}
	h := NewHandler(mock)
	router := setupRouter(h)

	rr := doRequest(t, router, http.MethodGet, "/api/v1/event?sort=event_date", "")
	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "desc", captured.Order, "date fields should default to desc")
}

func TestList_DefaultOrder_SystemFieldCreatedAt_Desc(t *testing.T) {
	var captured *interfaces.ListQuery
	mock := &mockContentService{
		getContentTypeFunc: func(_ context.Context, _ string) (*interfaces.ContentType, error) {
			return &interfaces.ContentType{Name: "post"}, nil
		},
		listFunc: func(_ context.Context, _ string, q *interfaces.ListQuery) (*interfaces.Page, error) {
			captured = q
			return samplePage([]*interfaces.Content{}), nil
		},
	}
	h := NewHandler(mock)
	router := setupRouter(h)

	rr := doRequest(t, router, http.MethodGet, "/api/v1/post?sort=created_at", "")
	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "desc", captured.Order, "created_at system field should default to desc")
}

func TestList_DefaultOrder_SystemFieldTitle_Asc(t *testing.T) {
	var captured *interfaces.ListQuery
	mock := &mockContentService{
		getContentTypeFunc: func(_ context.Context, _ string) (*interfaces.ContentType, error) {
			return &interfaces.ContentType{Name: "post"}, nil
		},
		listFunc: func(_ context.Context, _ string, q *interfaces.ListQuery) (*interfaces.Page, error) {
			captured = q
			return samplePage([]*interfaces.Content{}), nil
		},
	}
	h := NewHandler(mock)
	router := setupRouter(h)

	rr := doRequest(t, router, http.MethodGet, "/api/v1/post?sort=title", "")
	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "asc", captured.Order, "title system field should default to asc")
}

func TestList_ExplicitOrderOverridesDefault(t *testing.T) {
	var captured *interfaces.ListQuery
	mock := &mockContentService{
		getContentTypeFunc: func(_ context.Context, _ string) (*interfaces.ContentType, error) {
			return &interfaces.ContentType{
				Name:   "post",
				Fields: []interfaces.Field{{Name: "title", Type: "text"}},
			}, nil
		},
		listFunc: func(_ context.Context, _ string, q *interfaces.ListQuery) (*interfaces.Page, error) {
			captured = q
			return samplePage([]*interfaces.Content{}), nil
		},
	}
	h := NewHandler(mock)
	router := setupRouter(h)

	rr := doRequest(t, router, http.MethodGet, "/api/v1/post?sort=title&order=desc", "")
	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "desc", captured.Order, "explicit order should override default")
}

// ===========================================================================
// M2: Unknown filter field warning tests
// ===========================================================================

func TestList_UnknownFilterField_RemovedWithWarning(t *testing.T) {
	var captured *interfaces.ListQuery
	mock := &mockContentService{
		getContentTypeFunc: func(_ context.Context, _ string) (*interfaces.ContentType, error) {
			return &interfaces.ContentType{
				Name:   "post",
				Fields: []interfaces.Field{{Name: "title", Type: "text"}},
			}, nil
		},
		listFunc: func(_ context.Context, _ string, q *interfaces.ListQuery) (*interfaces.Page, error) {
			captured = q
			return samplePage([]*interfaces.Content{}), nil
		},
	}
	h := NewHandler(mock)
	router := setupRouter(h)

	rr := doRequest(t, router, http.MethodGet, "/api/v1/post?nonexistent_field=value", "")
	assert.Equal(t, http.StatusOK, rr.Code)

	assert.Nil(t, captured.Filters["nonexistent_field"], "unknown filter should be removed")

	resp := decodeAPIResponse(t, rr.Body.Bytes())
	metaBytes, _ := json.Marshal(resp.Meta)
	var meta PageMeta
	require.NoError(t, json.Unmarshal(metaBytes, &meta))
	require.Contains(t, meta.Warnings, "unknown filter field: nonexistent_field")
}

func TestList_KnownFilterField_NotRemoved(t *testing.T) {
	var captured *interfaces.ListQuery
	mock := &mockContentService{
		getContentTypeFunc: func(_ context.Context, _ string) (*interfaces.ContentType, error) {
			return &interfaces.ContentType{
				Name:   "post",
				Fields: []interfaces.Field{{Name: "status", Type: "text"}},
			}, nil
		},
		listFunc: func(_ context.Context, _ string, q *interfaces.ListQuery) (*interfaces.Page, error) {
			captured = q
			return samplePage([]*interfaces.Content{}), nil
		},
	}
	h := NewHandler(mock)
	router := setupRouter(h)

	rr := doRequest(t, router, http.MethodGet, "/api/v1/post?status=published", "")
	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "published", captured.Filters["status"])
}

func TestList_FilterOnSystemField_Valid(t *testing.T) {
	var captured *interfaces.ListQuery
	mock := &mockContentService{
		getContentTypeFunc: func(_ context.Context, _ string) (*interfaces.ContentType, error) {
			return &interfaces.ContentType{Name: "post"}, nil
		},
		listFunc: func(_ context.Context, _ string, q *interfaces.ListQuery) (*interfaces.Page, error) {
			captured = q
			return samplePage([]*interfaces.Content{}), nil
		},
	}
	h := NewHandler(mock)
	router := setupRouter(h)

	rr := doRequest(t, router, http.MethodGet, "/api/v1/post?slug=test-slug", "")
	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "test-slug", captured.Filters["slug"])
}

// ===========================================================================
// M3: Unknown fields param warning tests
// ===========================================================================

func TestList_UnknownFieldsParam_RemovedWithWarning(t *testing.T) {
	var captured *interfaces.ListQuery
	mock := &mockContentService{
		getContentTypeFunc: func(_ context.Context, _ string) (*interfaces.ContentType, error) {
			return &interfaces.ContentType{
				Name:   "post",
				Fields: []interfaces.Field{{Name: "title", Type: "text"}},
			}, nil
		},
		listFunc: func(_ context.Context, _ string, q *interfaces.ListQuery) (*interfaces.Page, error) {
			captured = q
			return samplePage([]*interfaces.Content{}), nil
		},
	}
	h := NewHandler(mock)
	router := setupRouter(h)

	rr := doRequest(t, router, http.MethodGet, "/api/v1/post?fields=id,title,nonexistent", "")
	assert.Equal(t, http.StatusOK, rr.Code)

	assert.Equal(t, []string{"id", "title"}, captured.Fields, "unknown field should be removed, id always included")

	resp := decodeAPIResponse(t, rr.Body.Bytes())
	metaBytes, _ := json.Marshal(resp.Meta)
	var meta PageMeta
	require.NoError(t, json.Unmarshal(metaBytes, &meta))
	require.Contains(t, meta.Warnings, "unknown field: nonexistent")
}

func TestList_FieldsParam_AlwaysIncludesID(t *testing.T) {
	var captured *interfaces.ListQuery
	mock := &mockContentService{
		getContentTypeFunc: func(_ context.Context, _ string) (*interfaces.ContentType, error) {
			return &interfaces.ContentType{
				Name:   "post",
				Fields: []interfaces.Field{{Name: "title", Type: "text"}},
			}, nil
		},
		listFunc: func(_ context.Context, _ string, q *interfaces.ListQuery) (*interfaces.Page, error) {
			captured = q
			return samplePage([]*interfaces.Content{}), nil
		},
	}
	h := NewHandler(mock)
	router := setupRouter(h)

	rr := doRequest(t, router, http.MethodGet, "/api/v1/post?fields=title", "")
	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, []string{"id", "title"}, captured.Fields, "id should be prepended when missing")
}

func TestList_FieldsParam_IDAlreadyIncluded_NoDuplicate(t *testing.T) {
	var captured *interfaces.ListQuery
	mock := &mockContentService{
		getContentTypeFunc: func(_ context.Context, _ string) (*interfaces.ContentType, error) {
			return &interfaces.ContentType{
				Name:   "post",
				Fields: []interfaces.Field{{Name: "title", Type: "text"}},
			}, nil
		},
		listFunc: func(_ context.Context, _ string, q *interfaces.ListQuery) (*interfaces.Page, error) {
			captured = q
			return samplePage([]*interfaces.Content{}), nil
		},
	}
	h := NewHandler(mock)
	router := setupRouter(h)

	rr := doRequest(t, router, http.MethodGet, "/api/v1/post?fields=id,title", "")
	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, []string{"id", "title"}, captured.Fields)
}

// ===========================================================================
// C2: Expand on Single-Item GET tests
// ===========================================================================

func TestGet_WithExpand_ReturnsMeta(t *testing.T) {
	content := sampleContent("abc-123", "post")
	mock := &mockContentService{
		getByIDFunc: func(_ context.Context, id string) (*interfaces.Content, error) {
			return content, nil
		},
		getContentTypeFunc: func(_ context.Context, ct string) (*interfaces.ContentType, error) {
			return &interfaces.ContentType{
				Name: "post",
				Fields: []interfaces.Field{
					{Name: "author", Type: "relation"},
					{Name: "tags", Type: "relation"},
				},
			}, nil
		},
	}
	h := NewHandler(mock)
	router := setupRouter(h)

	rr := doRequest(t, router, http.MethodGet, "/api/v1/post/abc-123?expand=author,tags", "")
	assert.Equal(t, http.StatusOK, rr.Code)

	resp := decodeAPIResponse(t, rr.Body.Bytes())
	metaMap, ok := resp.Meta.(map[string]interface{})
	require.True(t, ok, "meta should be a map when expand is present")
	expandArr, ok := metaMap["expand"].([]interface{})
	require.True(t, ok)
	assert.Len(t, expandArr, 2)
	assert.Equal(t, "author", expandArr[0])
	assert.Equal(t, "tags", expandArr[1])
}

func TestGet_WithExpand_Nested_WarningInMeta(t *testing.T) {
	content := sampleContent("abc-123", "post")
	mock := &mockContentService{
		getByIDFunc: func(_ context.Context, id string) (*interfaces.Content, error) {
			return content, nil
		},
		getContentTypeFunc: func(_ context.Context, ct string) (*interfaces.ContentType, error) {
			return &interfaces.ContentType{Name: "post"}, nil
		},
	}
	h := NewHandler(mock)
	router := setupRouter(h)

	rr := doRequest(t, router, http.MethodGet, "/api/v1/post/abc-123?expand=author.posts", "")
	assert.Equal(t, http.StatusOK, rr.Code)

	resp := decodeAPIResponse(t, rr.Body.Bytes())
	metaMap, ok := resp.Meta.(map[string]interface{})
	require.True(t, ok)
	warnings, ok := metaMap["warnings"].([]interface{})
	require.True(t, ok)
	assert.Contains(t, warnings[0], "nested expansion is not supported")
}

func TestGet_WithExpand_UnknownRelation_WarningInMeta(t *testing.T) {
	content := sampleContent("abc-123", "post")
	mock := &mockContentService{
		getByIDFunc: func(_ context.Context, id string) (*interfaces.Content, error) {
			return content, nil
		},
		getContentTypeFunc: func(_ context.Context, ct string) (*interfaces.ContentType, error) {
			return &interfaces.ContentType{
				Name:   "post",
				Fields: []interfaces.Field{{Name: "author", Type: "relation"}},
			}, nil
		},
	}
	h := NewHandler(mock)
	router := setupRouter(h)

	rr := doRequest(t, router, http.MethodGet, "/api/v1/post/abc-123?expand=author,nonexistent", "")
	assert.Equal(t, http.StatusOK, rr.Code)

	resp := decodeAPIResponse(t, rr.Body.Bytes())
	metaMap, ok := resp.Meta.(map[string]interface{})
	require.True(t, ok)
	warnings, ok := metaMap["warnings"].([]interface{})
	require.True(t, ok)
	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], "unknown relation: nonexistent")
}

func TestGet_WithoutExpand_NoMeta(t *testing.T) {
	content := sampleContent("abc-123", "post")
	mock := &mockContentService{
		getByIDFunc: func(_ context.Context, id string) (*interfaces.Content, error) {
			return content, nil
		},
	}
	h := NewHandler(mock)
	router := setupRouter(h)

	rr := doRequest(t, router, http.MethodGet, "/api/v1/post/abc-123", "")
	assert.Equal(t, http.StatusOK, rr.Code)

	resp := decodeAPIResponse(t, rr.Body.Bytes())
	metaBytes, _ := json.Marshal(resp.Meta)
	assert.Equal(t, `{}`, string(metaBytes), "without expand, meta should be empty")
}
