package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// writeJSON tests
// ---------------------------------------------------------------------------

func TestWriteJSON_Structure(t *testing.T) {
	rr := httptest.NewRecorder()
	writeJSON(rr, http.StatusOK, map[string]string{"hello": "world"})

	var resp APIResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))

	dataBytes, err := json.Marshal(resp.Data)
	require.NoError(t, err)
	assert.Equal(t, `{"hello":"world"}`, string(dataBytes))

	// Meta is now always present as an empty map
	require.NotNil(t, resp.Meta)
	metaBytes, _ := json.Marshal(resp.Meta)
	assert.Equal(t, `{}`, string(metaBytes))
}

func TestWriteJSON_ContentType(t *testing.T) {
	rr := httptest.NewRecorder()
	writeJSON(rr, http.StatusOK, "test")

	ct := rr.Header().Get("Content-Type")
	assert.Equal(t, "application/json; charset=utf-8", ct)
}

func TestWriteJSON_StatusCode(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
	}{
		{"200 OK", http.StatusOK},
		{"201 Created", http.StatusCreated},
		{"202 Accepted", http.StatusAccepted},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			writeJSON(rr, tt.statusCode, nil)
			assert.Equal(t, tt.statusCode, rr.Code)
		})
	}
}

// ---------------------------------------------------------------------------
// writeJSONWithMeta tests
// ---------------------------------------------------------------------------

func TestWriteJSONWithMeta_Structure(t *testing.T) {
	rr := httptest.NewRecorder()
	meta := PageMeta{
		TotalCount: 42,
		Page:       2,
		PerPage:    10,
		TotalPages: 5,
	}
	writeJSONWithMeta(rr, http.StatusOK, []string{"a", "b"}, meta)

	var resp APIResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))

	dataBytes, _ := json.Marshal(resp.Data)
	assert.Equal(t, `["a","b"]`, string(dataBytes))

	require.NotNil(t, resp.Meta)
	// Meta is interface{}, need to re-marshal to check fields
	metaBytes, err := json.Marshal(resp.Meta)
	require.NoError(t, err)
	var gotMeta PageMeta
	require.NoError(t, json.Unmarshal(metaBytes, &gotMeta))
	assert.Equal(t, int64(42), gotMeta.TotalCount)
	assert.Equal(t, 2, gotMeta.Page)
	assert.Equal(t, 10, gotMeta.PerPage)
	assert.Equal(t, 5, gotMeta.TotalPages)
}

func TestWriteJSONWithMeta_MetaFields(t *testing.T) {
	rr := httptest.NewRecorder()
	meta := PageMeta{
		TotalCount: 100,
		Page:       3,
		PerPage:    25,
		TotalPages: 4,
	}
	writeJSONWithMeta(rr, http.StatusOK, nil, meta)

	var resp APIResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	require.NotNil(t, resp.Meta)

	metaBytes, err := json.Marshal(resp.Meta)
	require.NoError(t, err)
	var gotMeta PageMeta
	require.NoError(t, json.Unmarshal(metaBytes, &gotMeta))

	assert.Equal(t, int64(100), gotMeta.TotalCount)
	assert.Equal(t, 3, gotMeta.Page)
	assert.Equal(t, 25, gotMeta.PerPage)
	assert.Equal(t, 4, gotMeta.TotalPages)
}

func TestWriteJSONWithMeta_ContentTypeAndStatus(t *testing.T) {
	rr := httptest.NewRecorder()
	writeJSONWithMeta(rr, http.StatusOK, nil, PageMeta{})

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "application/json; charset=utf-8", rr.Header().Get("Content-Type"))
}

func TestWriteJSONWithMeta_XTotalCountHeader(t *testing.T) {
	rr := httptest.NewRecorder()
	meta := PageMeta{TotalCount: 42, Page: 1, PerPage: 10, TotalPages: 5}
	writeJSONWithMeta(rr, http.StatusOK, nil, meta)

	assert.Equal(t, "42", rr.Header().Get("X-Total-Count"))
}

// ---------------------------------------------------------------------------
// writeError tests
// ---------------------------------------------------------------------------

func TestWriteError_Structure(t *testing.T) {
	rr := httptest.NewRecorder()
	writeError(rr, http.StatusBadRequest, "BAD_REQUEST", "invalid input")

	var envelope ErrorsEnvelope
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &envelope))
	require.Len(t, envelope.Errors, 1)

	assert.Equal(t, "BAD_REQUEST", envelope.Errors[0].Code)
	assert.Equal(t, "invalid input", envelope.Errors[0].Message)
}

func TestWriteError_ContentType(t *testing.T) {
	rr := httptest.NewRecorder()
	writeError(rr, http.StatusInternalServerError, "INTERNAL_ERROR", "boom")

	ct := rr.Header().Get("Content-Type")
	assert.Equal(t, "application/json; charset=utf-8", ct)
}

func TestWriteError_StatusCode(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
	}{
		{"400", http.StatusBadRequest},
		{"404", http.StatusNotFound},
		{"422", http.StatusUnprocessableEntity},
		{"500", http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			writeError(rr, tt.statusCode, "CODE", "msg")
			assert.Equal(t, tt.statusCode, rr.Code)
		})
	}
}

func TestWriteError_DetailsIsEmptyMap(t *testing.T) {
	rr := httptest.NewRecorder()
	writeError(rr, http.StatusNotFound, "NOT_FOUND", "resource not found")

	var envelope ErrorsEnvelope
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &envelope))
	require.Len(t, envelope.Errors, 1)

	// Details is always present as an empty map for writeError
	detailsBytes, _ := json.Marshal(envelope.Errors[0].Details)
	assert.Equal(t, `{}`, string(detailsBytes))
}

// ---------------------------------------------------------------------------
// writeErrorWithDetails tests
// ---------------------------------------------------------------------------

func TestWriteErrorWithDetails_Structure(t *testing.T) {
	rr := httptest.NewRecorder()
	details := map[string]interface{}{"field": "email"}
	writeErrorWithDetails(rr, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "invalid email", details)

	var envelope ErrorsEnvelope
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &envelope))
	require.Len(t, envelope.Errors, 1)

	err := envelope.Errors[0]
	assert.Equal(t, "VALIDATION_ERROR", err.Code)
	assert.Equal(t, "invalid email", err.Message)

	detailsMap, ok := err.Details.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "email", detailsMap["field"])
}

// ---------------------------------------------------------------------------
// writeErrors tests (multiple errors)
// ---------------------------------------------------------------------------

func TestWriteErrors_MultipleErrors(t *testing.T) {
	rr := httptest.NewRecorder()
	errs := []APIError{
		{Code: "VALIDATION_ERROR", Message: "title is required", Details: map[string]interface{}{"field": "title"}},
		{Code: "VALIDATION_ERROR", Message: "body is too short", Details: map[string]interface{}{"field": "body"}},
	}
	writeErrors(rr, http.StatusUnprocessableEntity, errs...)

	var envelope ErrorsEnvelope
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &envelope))
	require.Len(t, envelope.Errors, 2)

	assert.Equal(t, "VALIDATION_ERROR", envelope.Errors[0].Code)
	assert.Equal(t, "title is required", envelope.Errors[0].Message)
	assert.Equal(t, "VALIDATION_ERROR", envelope.Errors[1].Code)
	assert.Equal(t, "body is too short", envelope.Errors[1].Message)
}

func TestWriteErrors_SingleError(t *testing.T) {
	rr := httptest.NewRecorder()
	writeErrors(rr, http.StatusNotFound, APIError{Code: "NOT_FOUND", Message: "gone", Details: map[string]interface{}{}})

	var envelope ErrorsEnvelope
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &envelope))
	require.Len(t, envelope.Errors, 1)
	assert.Equal(t, "NOT_FOUND", envelope.Errors[0].Code)
}

// ---------------------------------------------------------------------------
// ErrorsEnvelope tests
// ---------------------------------------------------------------------------

func TestErrorsEnvelope_JSONFormat(t *testing.T) {
	envelope := ErrorsEnvelope{
		Errors: []APIError{
			{Code: "BAD_REQUEST", Message: "bad", Details: map[string]interface{}{}},
		},
	}
	b, err := json.Marshal(envelope)
	require.NoError(t, err)

	var raw map[string]interface{}
	require.NoError(t, json.Unmarshal(b, &raw))

	errorsArr, ok := raw["errors"].([]interface{})
	require.True(t, ok)
	require.Len(t, errorsArr, 1)
}

// ---------------------------------------------------------------------------
// PageMeta and APIResponse type tests (ensuring JSON tags work correctly)
// ---------------------------------------------------------------------------

func TestAPIResponse_MetaAlwaysIncluded(t *testing.T) {
	resp := APIResponse{Data: "hello"}
	b, err := json.Marshal(resp)
	require.NoError(t, err)

	var raw map[string]interface{}
	require.NoError(t, json.Unmarshal(b, &raw))
	assert.Equal(t, "hello", raw["data"])
	_, hasMeta := raw["meta"]
	assert.True(t, hasMeta, "meta should always be present in JSON output")
}

func TestAPIResponse_IncludeMetaWhenSet(t *testing.T) {
	resp := APIResponse{
		Data: "hello",
		Meta: PageMeta{TotalCount: 5, Page: 1, PerPage: 10, TotalPages: 1},
	}
	b, err := json.Marshal(resp)
	require.NoError(t, err)

	var raw map[string]interface{}
	require.NoError(t, json.Unmarshal(b, &raw))
	assert.Equal(t, "hello", raw["data"])
	_, hasMeta := raw["meta"]
	assert.True(t, hasMeta)

	metaRaw, ok := raw["meta"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, float64(5), metaRaw["total_count"])
}

func TestPageMeta_JSONTags(t *testing.T) {
	meta := PageMeta{
		TotalCount: 42,
		Page:       2,
		PerPage:    10,
		TotalPages: 5,
		Warnings:   []string{"deprecated param"},
	}
	b, err := json.Marshal(meta)
	require.NoError(t, err)

	var raw map[string]interface{}
	require.NoError(t, json.Unmarshal(b, &raw))

	assert.Contains(t, raw, "total_count")
	assert.Contains(t, raw, "page")
	assert.Contains(t, raw, "per_page")
	assert.Contains(t, raw, "total_pages")
	assert.Contains(t, raw, "warnings")
}
