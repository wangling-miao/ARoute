package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/wangling-miao/aroute/sdk/interfaces"
)

const (
	defaultPage    = 1
	defaultPerPage = 20
	maxPerPage     = 100
)

var reservedQueryParams = map[string]bool{
	"page":     true,
	"per_page": true,
	"sort":     true,
	"order":    true,
	"fields":   true,
	"expand":   true,
	"search":   true,
}

// Handler holds dependencies for REST API handlers.
type Handler struct {
	contentSvc interfaces.ContentService
	authSvc    interfaces.AuthService
}

// NewHandler creates a new Handler with the given ContentService.
func NewHandler(svc interfaces.ContentService) *Handler {
	return &Handler{contentSvc: svc}
}

// checkPerm checks if the authenticated user has the specified permission.
// Returns true if allowed, false if forbidden (writes error response).
func (h *Handler) checkPerm(w http.ResponseWriter, r *http.Request, resource, action string) bool {
	if h.authSvc == nil {
		return true // No auth service — allow all (public API mode).
	}
	claims := userClaimsFromRequest(r)
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return false
	}
	allowed, err := h.authSvc.HasPermission(r.Context(), claims.UserID, resource, action)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "permission check failed")
		return false
	}
	if !allowed {
		writeError(w, http.StatusForbidden, "FORBIDDEN", "insufficient permissions")
		return false
	}
	return true
}

// systemFields are valid for all content types.
var systemFields = map[string]bool{
	"id": true, "content_type": true, "title": true, "slug": true,
	"status": true, "author_id": true, "version": true,
	"published_at": true, "created_at": true, "updated_at": true,
}

// textFieldTypes default to ascending sort order.
var textFieldTypes = map[string]bool{
	"text": true, "markdown": true, "richtext": true, "email": true,
	"url": true, "slug": true, "color": true, "enum": true,
}

// descDefaultSystemFields default to descending sort order.
var descDefaultSystemFields = map[string]bool{
	"created_at": true, "updated_at": true, "published_at": true, "version": true,
}

// List handles GET /api/v1/{contentType} — paginated list of content items.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	contentType := chi.URLParam(r, "contentType")
	if contentType == "" {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "content type is required")
		return
	}

	ct, err := h.contentSvc.GetContentType(r.Context(), contentType)
	if err != nil {
		if errors.Is(err, interfaces.ErrNotFound) {
			writeError(w, http.StatusNotFound, "NOT_FOUND",
				fmt.Sprintf("content type '%s' not found", contentType))
			return
		}
		mapErrorToHTTP(w, err)
		return
	}

	ctFieldTypes := make(map[string]string)
	for _, f := range ct.Fields {
		ctFieldTypes[f.Name] = f.Type
	}
	validFields := make(map[string]bool, len(systemFields)+len(ct.Fields))
	for k := range systemFields {
		validFields[k] = true
	}
	for _, f := range ct.Fields {
		validFields[f.Name] = true
	}

	query, warnings, err := buildListQuery(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}

	// Author-scoped filtering: if user only has content.update_own (not content.read),
	// restrict results to content they created.
	if h.authSvc != nil {
		claims := userClaimsFromRequest(r)
		if claims != nil {
			canRead, _ := h.authSvc.HasPermission(r.Context(), claims.UserID, "content", "read")
			if !canRead {
				canOwn, _ := h.authSvc.HasPermission(r.Context(), claims.UserID, "content", "update_own")
				if canOwn {
					query.AuthorID = claims.UserID
				}
			}
		}
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}

	// M1: Validate sort field.
	if query.Sort != "" && !validFields[query.Sort] {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST",
			fmt.Sprintf("unknown sort field: %s", query.Sort))
		return
	}

	// M4: Default sort order by field type when sort is set but order is not.
	if query.Sort != "" && r.URL.Query().Get("order") == "" {
		if ft, ok := ctFieldTypes[query.Sort]; ok {
			if textFieldTypes[ft] {
				query.Order = "asc"
			} else {
				query.Order = "desc"
			}
		} else if descDefaultSystemFields[query.Sort] {
			query.Order = "desc"
		} else {
			query.Order = "asc"
		}
	}

	// M2: Unknown filter field warning — remove unknown filters.
	for key := range query.Filters {
		baseKey := key
		// Strip operator suffixes to get the base field name.
		for _, suffix := range []string{"_contains", "_gte", "_lte"} {
			if strings.HasSuffix(baseKey, suffix) {
				baseKey = strings.TrimSuffix(baseKey, suffix)
				break
			}
		}
		if !validFields[baseKey] {
			warnings = append(warnings, fmt.Sprintf("unknown filter field: %s", key))
			delete(query.Filters, key)
		}
	}

	// M3: Unknown fields param warning — remove unknown fields, ensure id is included.
	if len(query.Fields) > 0 {
		var valid []string
		hasID := false
		for _, f := range query.Fields {
			if !validFields[f] {
				warnings = append(warnings, fmt.Sprintf("unknown field: %s", f))
				continue
			}
			if f == "id" {
				hasID = true
			}
			valid = append(valid, f)
		}
		if !hasID {
			valid = append([]string{"id"}, valid...)
		}
		query.Fields = valid
	}

	page, err := h.contentSvc.List(r.Context(), contentType, query)
	if err != nil {
		mapErrorToHTTP(w, err)
		return
	}

	meta := PageMeta{
		TotalCount: page.Meta.Total,
		Page:       page.Meta.Page,
		PerPage:    page.Meta.PerPage,
		TotalPages: page.Meta.TotalPages,
		Warnings:   warnings,
	}
	writeJSONWithMeta(w, http.StatusOK, page.Data, meta)
}

// Get handles GET /api/v1/{contentType}/{id} — single content item.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "content id is required")
		return
	}

	content, err := h.contentSvc.GetByID(r.Context(), id)
	if err != nil {
		mapErrorToHTTP(w, err)
		return
	}

	expand := r.URL.Query().Get("expand")
	var meta map[string]interface{}
	if expand != "" {
		var warnings []string
		expandFields := strings.Split(expand, ",")
		validNames := make(map[string]bool)
		for _, ef := range expandFields {
			if strings.Contains(ef, ".") {
				warnings = append(warnings, "nested expansion is not supported")
				continue
			}
			validNames[ef] = true
		}

		ct, ctErr := h.contentSvc.GetContentType(r.Context(), content.ContentType)
		if ctErr == nil {
			ctFieldNames := make(map[string]bool, len(ct.Fields))
			for _, f := range ct.Fields {
				ctFieldNames[f.Name] = true
			}
			for ef := range validNames {
				if !ctFieldNames[ef] && !systemFields[ef] {
					warnings = append(warnings, fmt.Sprintf("unknown relation: %s", ef))
				}
			}
		}

		if len(warnings) > 0 {
			meta = map[string]interface{}{"warnings": warnings, "expand": expandFields}
		} else {
			meta = map[string]interface{}{"expand": expandFields}
		}
	}

	if meta != nil {
		resp := APIResponse{Data: content, Meta: meta}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(resp)
	} else {
		writeJSON(w, http.StatusOK, content)
	}
}

// Create handles POST /api/v1/{contentType} — create a new content item.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	contentType := chi.URLParam(r, "contentType")
	if contentType == "" {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "content type is required")
		return
	}

	defer r.Body.Close()
	var data map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", "request body is not valid JSON")
		return
	}

	content, err := h.contentSvc.Create(r.Context(), contentType, data)
	if err != nil {
		mapErrorToHTTP(w, err)
		return
	}

	w.Header().Set("Location", fmt.Sprintf("/api/v1/%s/%s", contentType, content.ID))
	writeJSON(w, http.StatusCreated, content)
}

// Update handles PUT /api/v1/{contentType}/{id} — update a content item.
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "content id is required")
		return
	}

	defer r.Body.Close()
	var data map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", "request body is not valid JSON")
		return
	}

	content, err := h.contentSvc.Update(r.Context(), id, data)
	if err != nil {
		mapErrorToHTTP(w, err)
		return
	}

	writeJSON(w, http.StatusOK, content)
}

// Delete handles DELETE /api/v1/{contentType}/{id} — delete a content item.
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "content id is required")
		return
	}

	if err := h.contentSvc.Delete(r.Context(), id); err != nil {
		mapErrorToHTTP(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ListContentTypes handles GET /api/v1/content-types.
func (h *Handler) ListContentTypes(w http.ResponseWriter, r *http.Request) {
	types, err := h.contentSvc.ListContentTypes(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list content types")
		return
	}
	if types == nil {
		types = []*interfaces.ContentType{}
	}
	writeJSON(w, http.StatusOK, types)
}

// GetContentType handles GET /api/v1/content-types/{name} — get a single content type.
func (h *Handler) GetContentType(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "content type name is required")
		return
	}

	ct, err := h.contentSvc.GetContentType(r.Context(), name)
	if err != nil {
		mapErrorToHTTP(w, err)
		return
	}

	writeJSON(w, http.StatusOK, ct)
}

// CreateContentType handles POST /api/v1/content-types — create a new content type.
func (h *Handler) CreateContentType(w http.ResponseWriter, r *http.Request) {
	if !h.checkPerm(w, r, "content_types", "create") {
		return
	}
	defer r.Body.Close()
	var ct interfaces.ContentType
	if err := json.NewDecoder(r.Body).Decode(&ct); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", "request body is not valid JSON")
		return
	}

	created, err := h.contentSvc.CreateContentType(r.Context(), &ct)
	if err != nil {
		mapErrorToHTTP(w, err)
		return
	}

	w.Header().Set("Location", fmt.Sprintf("/api/v1/content-types/%s", created.Name))
	writeJSON(w, http.StatusCreated, created)
}

// UpdateContentType handles PUT /api/v1/content-types/{name} — update a content type.
func (h *Handler) UpdateContentType(w http.ResponseWriter, r *http.Request) {
	if !h.checkPerm(w, r, "content_types", "update") {
		return
	}
	name := chi.URLParam(r, "name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "content type name is required")
		return
	}

	defer r.Body.Close()
	var ct interfaces.ContentType
	if err := json.NewDecoder(r.Body).Decode(&ct); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", "request body is not valid JSON")
		return
	}

	updated, err := h.contentSvc.UpdateContentType(r.Context(), name, &ct)
	if err != nil {
		mapErrorToHTTP(w, err)
		return
	}

	writeJSON(w, http.StatusOK, updated)
}

// DeleteContentType handles DELETE /api/v1/content-types/{name} — delete a content type.
func (h *Handler) DeleteContentType(w http.ResponseWriter, r *http.Request) {
	if !h.checkPerm(w, r, "content_types", "delete") {
		return
	}
	name := chi.URLParam(r, "name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "content type name is required")
		return
	}

	if err := h.contentSvc.DeleteContentType(r.Context(), name); err != nil {
		mapErrorToHTTP(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// buildListQuery constructs a ListQuery from request query parameters.
// Returns the query, any warnings, or an error for invalid pagination.
func buildListQuery(r *http.Request) (*interfaces.ListQuery, []string, error) {
	q := &interfaces.ListQuery{
		Page:    defaultPage,
		PerPage: defaultPerPage,
		Filters: make(map[string]interface{}),
	}

	var warnings []string

	if p := r.URL.Query().Get("page"); p != "" {
		v, err := strconv.Atoi(p)
		if err != nil || v <= 0 {
			return nil, nil, fmt.Errorf("page must be a positive integer")
		}
		q.Page = v
	}

	if pp := r.URL.Query().Get("per_page"); pp != "" {
		if v, err := strconv.Atoi(pp); err == nil && v > 0 {
			q.PerPage = min(v, maxPerPage)
		}
	}

	q.Sort = r.URL.Query().Get("sort")
	q.Order = r.URL.Query().Get("order")

	if q.Sort == "" {
		q.Sort = "created_at"
		q.Order = "desc"
	}

	if f := r.URL.Query().Get("fields"); f != "" {
		q.Fields = strings.Split(f, ",")
	}

	if e := r.URL.Query().Get("expand"); e != "" {
		expandFields := strings.Split(e, ",")
		for _, ef := range expandFields {
			if strings.Contains(ef, ".") {
				warnings = append(warnings, "nested expansion is not supported")
			}
		}
		q.Expand = expandFields
	}

	q.Search = r.URL.Query().Get("search")

	for key, values := range r.URL.Query() {
		if reservedQueryParams[key] || len(values) == 0 {
			continue
		}
		value := values[0]

		switch {
		case strings.HasSuffix(key, "_contains"):
			q.Filters[key] = value
		case strings.HasSuffix(key, "_gte"):
			q.Filters[key] = value
		case strings.HasSuffix(key, "_lte"):
			q.Filters[key] = value
		default:
			if strings.Contains(value, ",") {
				q.Filters[key] = strings.Split(value, ",")
			} else if value == "true" || value == "false" {
				q.Filters[key] = value == "true"
			} else {
				q.Filters[key] = value
			}
		}
	}

	return q, warnings, nil
}

// mapErrorToHTTP translates service errors to HTTP error responses.
func mapErrorToHTTP(w http.ResponseWriter, err error) {
	var validationErrs *interfaces.ValidationErrors

	switch {
	case errors.Is(err, interfaces.ErrNotFound):
		writeError(w, http.StatusNotFound, "NOT_FOUND", err.Error())
	case errors.Is(err, interfaces.ErrUnauthorized):
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", err.Error())
	case errors.Is(err, interfaces.ErrForbidden):
		writeError(w, http.StatusForbidden, "FORBIDDEN", err.Error())
	case errors.Is(err, interfaces.ErrBadRequest):
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
	case errors.Is(err, interfaces.ErrConflict):
		writeError(w, http.StatusConflict, "CONFLICT", err.Error())
	case errors.As(err, &validationErrs):
		apiErrs := make([]APIError, 0, len(validationErrs.Errors))
		for _, ve := range validationErrs.Errors {
			apiErrs = append(apiErrs, APIError{
				Code:    "VALIDATION_ERROR",
				Message: ve.Message,
				Details: map[string]interface{}{"field": ve.Field},
			})
		}
		writeErrors(w, http.StatusUnprocessableEntity, apiErrs...)
	case errors.Is(err, interfaces.ErrValidation):
		writeError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", err.Error())
	default:
		slog.Error("unexpected API error", "error", err)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "an unexpected error occurred")
	}
}
