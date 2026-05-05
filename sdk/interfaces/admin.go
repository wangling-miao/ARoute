package interfaces

import "context"

// AdminUISwitcher defines the interface for hot-swapping admin UI variants.
// The admin plugin implements this to allow switching between different
// pre-built admin interface builds without restarting the server.
type AdminUISwitcher interface {
	// GetActiveVariant returns the currently active admin UI variant name.
	GetActiveVariant(ctx context.Context) (string, error)

	// SetActiveVariant switches the admin UI to the specified variant.
	// The switch is atomic — new requests are served from the new variant.
	SetActiveVariant(ctx context.Context, variant string) error

	// ListVariants returns metadata for all available admin UI variants.
	ListVariants(ctx context.Context) ([]VariantInfo, error)
}

// VariantInfo holds metadata for an admin UI variant.
type VariantInfo struct {
	Variant     string `json:"variant"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
	Active      bool   `json:"active"`
}
