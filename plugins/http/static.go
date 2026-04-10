// Package http provides static file serving functionality.
// It serves static files from configured directories (e.g., themes, uploads, admin UI).
package http

import (
	"net/http"
	"os"
	"path/filepath"

	"github.com/wangling-miao/aroute/core"
)

// setupStaticFiles configures static file serving from configured directories.
// It serves files from the static_dir configuration value.
func (p *Plugin) setupStaticFiles(ctx core.CoreContext) error {
	logger := ctx.Logger()
	config := ctx.Config()

	// Get static files directory
	staticDir := config.GetString("http.static_dir")
	if staticDir == "" {
		// Default to data directory + static
		staticDir = filepath.Join(ctx.DataDir(), "static")
	}

	// Check if static directory exists
	if _, err := os.Stat(staticDir); os.IsNotExist(err) {
		logger.Debug("Static directory does not exist, skipping static file setup", "dir", staticDir)
		return nil // Not an error - directory may be created later
	}

	// Serve static files from /static/ path
	fileServer := http.FileServer(http.Dir(staticDir))
	p.router.Handle("/static/*", http.StripPrefix("/static/", fileServer))

	logger.Info("Static file serving configured",
		"dir", staticDir,
		"path", "/static/",
	)

	return nil
}
