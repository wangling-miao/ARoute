package interfaces

import (
	"time"
)

// User represents a user account in the system.
type User struct {
	// ID is the unique user identifier (UUID or auto-increment).
	ID string `json:"id"`

	// Email is the user's email address (unique, used for login).
	Email string `json:"email"`

	// Username is the user's display name (unique).
	Username string `json:"username"`

	// PasswordHash is the bcrypt-hashed password (never returned in API responses).
	PasswordHash string `json:"-"`

	// Roles is the list of role names assigned to this user.
	Roles []string `json:"roles"`

	// CreatedAt is when the user account was created.
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt is when the user profile was last modified.
	UpdatedAt time.Time `json:"updated_at"`

	// LastLoginAt is when the user last logged in (nullable).
	LastLoginAt *time.Time `json:"last_login_at,omitempty"`

	// Status is the user account status (active, inactive, suspended).
	Status string `json:"status"`
}

// Role represents a user role in RBAC system.
type Role struct {
	// ID is the unique role identifier.
	ID string `json:"id"`

	// Name is the role name (e.g., "admin", "editor", "author", "viewer").
	Name string `json:"name"`

	// DisplayName is the human-readable role name.
	DisplayName string `json:"display_name"`

	// Description is a brief description of the role.
	Description string `json:"description"`

	// Permissions is the list of permission names assigned to this role.
	Permissions []string `json:"permissions"`

	// CreatedAt is when the role was created.
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt is when the role was last modified.
	UpdatedAt time.Time `json:"updated_at"`
}

// Permission represents a permission in RBAC system.
// A permission is a resource+action pair (e.g., "content:read", "content:write").
type Permission struct {
	// ID is the unique permission identifier.
	ID string `json:"id"`

	// Name is the permission name (e.g., "content:read", "user:manage").
	Name string `json:"name"`

	// Resource is the resource type (e.g., "content", "user", "media").
	Resource string `json:"resource"`

	// Action is the action type (e.g., "read", "write", "delete", "manage").
	Action string `json:"action"`

	// DisplayName is the human-readable permission name.
	DisplayName string `json:"display_name"`

	// Description is a brief description of what this permission allows.
	Description string `json:"description"`
}

// Content represents a content item in the CMS.
type Content struct {
	// ID is the unique content identifier (UUID or auto-increment).
	ID string `json:"id"`

	// ContentType is the content type name (e.g., "post", "page", "product").
	ContentType string `json:"content_type"`

	// Title is the content title (for list display).
	Title string `json:"title"`

	// Slug is the URL-friendly identifier (auto-generated from title).
	Slug string `json:"slug"`

	// Data is the actual content data (field values as map).
	// This map contains all field values defined by the Content Type.
	Data map[string]interface{} `json:"data"`

	// AuthorID is the ID of the user who created this content.
	AuthorID string `json:"author_id"`

	// Status is the content status ("draft", "published", "archived").
	Status string `json:"status"`

	// PublishedAt is when the content was published (nullable).
	PublishedAt *time.Time `json:"published_at,omitempty"`

	// CreatedAt is when the content was created.
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt is when the content was last modified.
	UpdatedAt time.Time `json:"updated_at"`

	// Version is the content version number (for version history).
	Version int `json:"version"`
}

// ContentType represents a dynamic content type definition.
type ContentType struct {
	// ID is the unique content type identifier.
	ID string `json:"id"`

	// Name is the content type name (e.g., "post", "page", "product").
	Name string `json:"name"`

	// DisplayName is the human-readable name (e.g., "Blog Post", "Product").
	DisplayName string `json:"display_name"`

	// Description is what this content type is for.
	Description string `json:"description"`

	// Fields is the list of field definitions for this content type.
	Fields []Field `json:"fields"`

	// TableName is the database table name for this content type.
	TableName string `json:"table_name"`

	// CreatedAt is when the content type was created.
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt is when the content type was last modified.
	UpdatedAt time.Time `json:"updated_at"`
}

// Field represents a field definition in a Content Type.
type Field struct {
	// Name is the field name (used as column name, snake_case).
	Name string `json:"name"`

	// DisplayName is the human-readable field name.
	DisplayName string `json:"display_name"`

	// Type is the field type (text, number, boolean, date, datetime, relation, media, json, markdown, richtext, email, url, slug, enum, color).
	Type string `json:"type"`

	// Required indicates whether this field is mandatory.
	Required bool `json:"required"`

	// Unique indicates whether this field value must be unique.
	Unique bool `json:"unique"`

	// DefaultValue is the default value for this field.
	DefaultValue interface{} `json:"default_value,omitempty"`

	// ValidationRules contains field-specific validation rules.
	// Keys: "minLength", "maxLength", "min", "max", "pattern", "enum"
	ValidationRules map[string]interface{} `json:"validation_rules,omitempty"`

	// RelationConfig contains configuration for relation fields.
	RelationConfig *RelationConfig `json:"relation_config,omitempty"`

	// Index indicates whether to create a database index on this field.
	Index bool `json:"index"`

	// Description is a brief description of this field.
	Description string `json:"description"`
}

// RelationConfig defines the relationship between content types.
type RelationConfig struct {
	// TargetContentType is the related content type name.
	TargetContentType string `json:"target_content_type"`

	// RelationType is the relationship type (one-to-one, one-to-many, many-to-many).
	RelationType string `json:"relation_type"`

	// ThroughTable is the junction table name (for many-to-many relations).
	ThroughTable string `json:"through_table,omitempty"`
}

// MediaFile represents a media file in the media library.
type MediaFile struct {
	// ID is the unique media file identifier.
	ID string `json:"id"`

	// Filename is the original file name (e.g., "photo.jpg").
	Filename string `json:"filename"`

	// MIMEType is the MIME type (e.g., "image/jpeg", "video/mp4").
	MIMEType string `json:"mime_type"`

	// Size is the file size in bytes.
	Size int64 `json:"size"`

	// Width is the image width in pixels (only for images).
	Width int `json:"width,omitempty"`

	// Height is the image height in pixels (only for images).
	Height int `json:"height,omitempty"`

	// StoragePath is the storage path (local path or S3 key).
	StoragePath string `json:"storage_path"`

	// StorageType is the storage backend ("local" or "s3").
	StorageType string `json:"storage_type"`

	// UploaderID is the ID of the user who uploaded this file.
	UploaderID string `json:"uploader_id"`

	// CreatedAt is when the file was uploaded.
	CreatedAt time.Time `json:"created_at"`

	// ThumbnailPath is the path to the generated thumbnail (optional).
	ThumbnailPath string `json:"thumbnail_path,omitempty"`
}

// SearchResult represents a search result item.
type SearchResult struct {
	// ID is the content ID.
	ID string `json:"id"`

	// ContentType is the content type name.
	ContentType string `json:"content_type"`

	// Title is the content title.
	Title string `json:"title"`

	// Excerpt is a snippet of the matching text (with highlighting).
	Excerpt string `json:"excerpt"`

	// Score is the search relevance score.
	Score float64 `json:"score"`

	// HighlightedFields contains highlighted field fragments.
	HighlightedFields map[string]string `json:"highlighted_fields"`

	// Data contains the actual content data.
	Data map[string]interface{} `json:"data"`
}

// Page represents a pagination wrapper for list results.
type Page struct {
	// Data is the list of items on the current page.
	Data interface{} `json:"data"`

	// Meta contains pagination metadata.
	Meta PageMeta `json:"meta"`
}

// PageMeta contains pagination metadata.
type PageMeta struct {
	// Total is the total number of items across all pages.
	Total int64 `json:"total"`

	// Page is the current page number (1-indexed).
	Page int `json:"page"`

	// PerPage is the number of items per page.
	PerPage int `json:"per_page"`

	// TotalPages is the total number of pages.
	TotalPages int `json:"total_pages"`

	// HasPrev indicates whether there is a previous page.
	HasPrev bool `json:"has_prev"`

	// HasNext indicates whether there is a next page.
	HasNext bool `json:"has_next"`
}

// ListQuery contains common parameters for list queries.
type ListQuery struct {
	// Page is the page number (1-indexed).
	Page int `json:"page"`

	// PerPage is the number of items per page.
	PerPage int `json:"per_page"`

	// Filters is a map of field name to filter value.
	Filters map[string]interface{} `json:"filters,omitempty"`

	// Sort is the sort field name.
	Sort string `json:"sort,omitempty"`

	// Order is the sort order ("asc" or "desc").
	Order string `json:"order,omitempty"`

	// Fields is the list of fields to return (sparse fieldsets).
	Fields []string `json:"fields,omitempty"`

	// Expand is the list of related resources to expand.
	Expand []string `json:"expand,omitempty"`

	// Search is the search query string (for full-text search).
	Search string `json:"search,omitempty"`
}
