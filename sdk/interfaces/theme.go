package interfaces

import (
	"context"
)

// ThemeService defines theme management and rendering operations.
// It supports multiple rendering engines (Go template, Lua, React SSR)
// and handles theme activation, asset serving, and template rendering.
type ThemeService interface {
	// Render renders a template with the given data and returns HTML.
	// The engine is selected based on the theme's configuration.
	Render(ctx context.Context, templateName string, data map[string]interface{}) (string, error)

	// GetActiveTheme returns the currently active theme name.
	GetActiveTheme(ctx context.Context) (string, error)

	// SetActiveTheme changes the active theme (hot swap without restart).
	SetActiveTheme(ctx context.Context, name string) error

	// ListThemes returns all available theme names.
	ListThemes(ctx context.Context) ([]string, error)

	// ThemeMeta returns metadata for a theme by slug.
	// Returns nil if the theme is not found.
	ThemeMeta(slug string) map[string]string

	// InstallTheme installs a theme from a directory path or archive.
	InstallTheme(ctx context.Context, sourcePath string) error

	// ReloadThemes rescans the themes directory for newly added themes
	// without requiring a service restart.
	ReloadThemes() error
}
