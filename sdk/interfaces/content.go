package interfaces

import (
	"context"
)

// ContentService defines content management operations for dynamic content types.
// It handles CRUD operations for both built-in and custom content types,
// field validation, version history, and slug generation.
type ContentService interface {
	// Create creates a new content item of the specified content type.
	// It validates fields, generates slugs, emits content.{type}.created event,
	// and returns the created content with generated ID.
	Create(ctx context.Context, contentType string, data map[string]interface{}) (*Content, error)

	// GetByID retrieves a content item by its ID.
	// Returns ErrNotFound if the content doesn't exist.
	GetByID(ctx context.Context, id string) (*Content, error)

	// Update modifies an existing content item.
	// It validates fields, creates version history, emits content.{type}.updated event.
	Update(ctx context.Context, id string, data map[string]interface{}) (*Content, error)

	// Delete soft-deletes a content item by setting deleted_at timestamp.
	// Emits content.{type}.deleted event.
	Delete(ctx context.Context, id string) error

	// List retrieves a paginated list of content items for a content type.
	// Supports filtering, sorting, pagination, and field selection.
	List(ctx context.Context, contentType string, query *ListQuery) (*Page, error)

	// GetContentType retrieves a content type definition by name.
	// Returns ErrNotFound if the content type doesn't exist.
	GetContentType(ctx context.Context, name string) (*ContentType, error)

	// CreateContentType creates a new dynamic content type.
	// It stores the definition in Schema Registry and calls DDL Engine to create the table.
	CreateContentType(ctx context.Context, ct *ContentType) (*ContentType, error)

	// UpdateContentType modifies an existing content type definition.
	// It updates Schema Registry and uses DDL Engine for schema migrations.
	UpdateContentType(ctx context.Context, name string, ct *ContentType) (*ContentType, error)

	// DeleteContentType removes a content type and its associated table.
	// This is a destructive operation that deletes all content of this type.
	DeleteContentType(ctx context.Context, name string) error
}
