// Package http provides CORS (Cross-Origin Resource Sharing) configuration.
// It reads CORS settings from configuration and applies them to the router.
package http

import (
	"github.com/go-chi/cors"
	"github.com/wangling-miao/aroute/core"
)

// setupCORS configures CORS middleware based on configuration.
// Reads allowed origins, methods, and headers from config.
func (p *Plugin) setupCORS(ctx core.CoreContext) {
	logger := ctx.Logger()
	config := ctx.Config()

	// Get CORS configuration
	allowedOrigins := config.GetStringSlice("http.cors.allowed_origins")
	if len(allowedOrigins) == 0 {
		allowedOrigins = []string{"*"} // Default: allow all origins
	}

	allowedMethods := config.GetStringSlice("http.cors.allowed_methods")
	if len(allowedMethods) == 0 {
		allowedMethods = []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}
	}

	allowedHeaders := config.GetStringSlice("http.cors.allowed_headers")
	if len(allowedHeaders) == 0 {
		allowedHeaders = []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"}
	}

	exposedHeaders := config.GetStringSlice("http.cors.exposed_headers")
	if len(exposedHeaders) == 0 {
		exposedHeaders = []string{"Link"}
	}

	maxAge := config.GetInt("http.cors.max_age")
	if maxAge == 0 {
		maxAge = 300 // 5 minutes default
	}

	allowCredentials := config.GetBool("http.cors.allow_credentials")

	// Apply CORS middleware
	corsMiddleware := cors.Handler(cors.Options{
		AllowedOrigins:   allowedOrigins,
		AllowedMethods:   allowedMethods,
		AllowedHeaders:   allowedHeaders,
		ExposedHeaders:   exposedHeaders,
		AllowCredentials: allowCredentials,
		MaxAge:           maxAge,
	})

	p.router.Use(corsMiddleware)

	logger.Debug("CORS middleware configured",
		"origins", allowedOrigins,
		"methods", allowedMethods,
		"headers", allowedHeaders,
		"max_age", maxAge,
		"credentials", allowCredentials,
	)
}
