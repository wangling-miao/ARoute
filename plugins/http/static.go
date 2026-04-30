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
// It serves files from the static_dir configuration value and the uploads directory.
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
	} else {
		// Serve static files from /static/ path
		fileServer := http.FileServer(http.Dir(staticDir))
		p.router.Handle("/static/*", http.StripPrefix("/static/", fileServer))

		logger.Info("Static file serving configured",
			"dir", staticDir,
			"path", "/static/",
		)
	}

	// Serve uploaded media files from /uploads/ path.
	// Media plugin stores files in {plugin_data}/media/uploads/
	pluginDataDir := filepath.Dir(ctx.DataDir()) // parent of http plugin's data dir → plugin_data/
	uploadsDir := filepath.Join(pluginDataDir, "media", "uploads")
	if _, err := os.Stat(uploadsDir); os.IsNotExist(err) {
		if err := os.MkdirAll(uploadsDir, 0755); err != nil {
			logger.Warn("Failed to create uploads directory", "dir", uploadsDir, "error", err)
		}
	}
	uploadsServer := http.FileServer(http.Dir(uploadsDir))
	p.router.Handle("/uploads/*", http.StripPrefix("/uploads/", uploadsServer))

	logger.Info("Uploads file serving configured",
		"dir", uploadsDir,
		"path", "/uploads/",
	)

	return nil
}
