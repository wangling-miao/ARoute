package interfaces

import (
	"errors"
	"fmt"
)

// Common error types used across the CMS.
// These errors can be used by plugins and core for consistent error handling.
var (
	// ErrNotFound indicates that the requested resource was not found.
	ErrNotFound = errors.New("resource not found")

	// ErrUnauthorized indicates that the request lacks valid authentication credentials.
	ErrUnauthorized = errors.New("unauthorized")

	// ErrForbidden indicates that the authenticated user lacks permission for the operation.
	ErrForbidden = errors.New("forbidden")

	// ErrValidation indicates that the request data failed validation.
	ErrValidation = errors.New("validation failed")

	// ErrConflict indicates a conflict with the current state (e.g., duplicate unique value).
	ErrConflict = errors.New("resource conflict")

	// ErrBadRequest indicates that the request is malformed or invalid.
	ErrBadRequest = errors.New("bad request")

	// ErrInternal indicates an internal server error.
	ErrInternal = errors.New("internal server error")
)

// ValidationError represents a field-level validation error.
type ValidationError struct {
	// Field is the field name that failed validation.
	Field string `json:"field"`

	// Message is the validation error message.
	Message string `json:"message"`

	// Code is an optional error code for programmatic handling.
	Code string `json:"code,omitempty"`
}

// Error implements the error interface.
func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// ValidationErrors is a collection of validation errors.
type ValidationErrors struct {
	// Errors is the list of field-level validation errors.
	Errors []*ValidationError `json:"errors"`
}

// Error implements the error interface.
func (e *ValidationErrors) Error() string {
	if len(e.Errors) == 0 {
		return "validation failed"
	}
	return fmt.Sprintf("validation failed: %d errors", len(e.Errors))
}

// Add adds a new validation error to the collection.
// If the receiver is nil, Add is a no-op to prevent panics.
func (e *ValidationErrors) Add(field, message string, code ...string) {
	if e == nil {
		return
	}
	ve := &ValidationError{
		Field:   field,
		Message: message,
	}
	if len(code) > 0 {
		ve.Code = code[0]
	}
	e.Errors = append(e.Errors, ve)
}

// HasErrors returns true if there are any validation errors.
func (e *ValidationErrors) HasErrors() bool {
	return len(e.Errors) > 0
}

// Unwrap returns ErrValidation when there are collected errors,
// enabling errors.Is(err, ErrValidation) to match *ValidationErrors.
func (e *ValidationErrors) Unwrap() error {
	if e.HasErrors() {
		return ErrValidation
	}
	return nil
}

// NewValidationErrors creates a new ValidationErrors instance.
func NewValidationErrors() *ValidationErrors {
	return &ValidationErrors{
		Errors: make([]*ValidationError, 0),
	}
}
