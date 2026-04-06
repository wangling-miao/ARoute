package interfaces

import (
	"context"
)

// SearchService defines full-text search and indexing operations.
// It handles Bleve-based search with gse Chinese tokenizer integration,
// automatic indexing on content changes, and faceted search.
type SearchService interface {
	// Index adds or updates a content item in the search index.
	Index(ctx context.Context, contentType string, content *Content) error

	// Remove removes a content item from the search index.
	Remove(ctx context.Context, id string) error

	// Search performs a full-text search across content.
	// Supports filtering by content type, pagination, and returning highlighted fragments.
	Search(ctx context.Context, query *SearchQuery) (*SearchResponse, error)

	// Rebuild clears and rebuilds the entire search index from database.
	Rebuild(ctx context.Context) error

	// GetFacets returns aggregated counts for specified fields (e.g., by category, author).
	GetFacets(ctx context.Context, contentType string, fields []string) (map[string]map[string]int64, error)
}

// SearchQuery contains search parameters.
type SearchQuery struct {
	// Query is the search query string.
	Query string `json:"query"`

	// ContentTypes is a list of content types to search (empty means all).
	ContentTypes []string `json:"content_types,omitempty"`

	// Filters is a map of field name to value for filtering.
	Filters map[string]interface{} `json:"filters,omitempty"`

	// Page is the page number (1-indexed).
	Page int `json:"page"`

	// PerPage is the number of results per page.
	PerPage int `json:"per_page"`

	// Highlight indicates whether to return highlighted fragments.
	Highlight bool `json:"highlight"`

	// Fields is the list of fields to return (sparse fieldsets).
	Fields []string `json:"fields,omitempty"`
}

// SearchResponse contains search results and metadata.
type SearchResponse struct {
	// Hits is the list of search result items.
	Hits []*SearchResult `json:"hits"`

	// Total is the total number of matching documents.
	Total int64 `json:"total"`

	// Page is the current page number.
	Page int `json:"page"`

	// PerPage is the number of items per page.
	PerPage int `json:"per_page"`

	// Facets contains aggregated counts for requested fields.
	Facets map[string]map[string]int64 `json:"facets,omitempty"`
}
