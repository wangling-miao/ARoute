package api

import (
	"encoding/json"
	"net/http"
	"strconv"
)

// APIResponse is the unified success response envelope.
// Meta can be PageMeta (for lists) or a generic map (for single items).
type APIResponse struct {
	Data interface{} `json:"data"`
	Meta interface{} `json:"meta"`
}

// PageMeta contains pagination metadata for list responses.
type PageMeta struct {
	TotalCount int64    `json:"total_count"`
	Page       int      `json:"page"`
	PerPage    int      `json:"per_page"`
	TotalPages int      `json:"total_pages"`
	Warnings   []string `json:"warnings,omitempty"`
}

// APIError is the unified error object structure.
// Details is a free-form object carrying per-error context (e.g. {"field": "title"}).
type APIError struct {
	Code    string      `json:"code"`
	Message string      `json:"message"`
	Details interface{} `json:"details"`
}

// ErrorsEnvelope wraps one or more API errors in the {"errors": [...]} envelope.
type ErrorsEnvelope struct {
	Errors []APIError `json:"errors"`
}

// ErrorDetail provides field-level error information (kept for backward compatibility).
type ErrorDetail struct {
	Field   string `json:"field,omitempty"`
	Message string `json:"message"`
}

// writeJSON writes a JSON success response with the given status code.
func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	resp := APIResponse{Data: data, Meta: map[string]interface{}{}}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		http.Error(w, `{"errors":[{"code":"INTERNAL_ERROR","message":"failed to encode response","details":{}}]}`, http.StatusInternalServerError)
	}
}

// writeJSONWithMeta writes a JSON success response with pagination metadata
// and the X-Total-Count response header.
func writeJSONWithMeta(w http.ResponseWriter, status int, data interface{}, meta PageMeta) {
	resp := APIResponse{Data: data, Meta: meta}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Total-Count", strconv.FormatInt(meta.TotalCount, 10))
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		http.Error(w, `{"errors":[{"code":"INTERNAL_ERROR","message":"failed to encode response","details":{}}]}`, http.StatusInternalServerError)
	}
}

// writeError writes a JSON error response with a single error wrapped in the
// {"errors": [...]} envelope.
func writeError(w http.ResponseWriter, status int, code, message string) {
	writeErrors(w, status, APIError{Code: code, Message: message, Details: map[string]interface{}{}})
}

// writeErrorWithDetails writes a JSON error response with a single error that
// includes structured details.
func writeErrorWithDetails(w http.ResponseWriter, status int, code, message string, details interface{}) {
	writeErrors(w, status, APIError{Code: code, Message: message, Details: details})
}

// writeErrors writes a JSON error response with one or more errors in the
// {"errors": [...]} envelope.
func writeErrors(w http.ResponseWriter, status int, errs ...APIError) {
	envelope := ErrorsEnvelope{Errors: errs}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(envelope); err != nil {
		http.Error(w, `{"errors":[{"code":"INTERNAL_ERROR","message":"failed to encode error","details":{}}]}`, http.StatusInternalServerError)
	}
}
