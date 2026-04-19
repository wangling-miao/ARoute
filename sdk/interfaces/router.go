package interfaces

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// RouteRegistrar defines the interface for route registration.
// Plugins use this service (provided by the HTTP plugin via ServiceContainer)
// to register their HTTP routes without direct import of the http plugin.
type RouteRegistrar interface {
	// Register registers a route pattern with handler.
	Register(pattern string, handler interface{})

	// Route creates a sub-router for a path prefix.
	Route(pattern string, fn func(r chi.Router))

	// Group creates a new route group with shared middleware.
	Group(fn func(r chi.Router))

	// Mount mounts another router at a path prefix.
	Mount(pattern string, router chi.Router)

	// Use collects middleware to be applied before the HTTP server starts.
	// Middlewares are applied as outer handlers wrapping the mux,
	// avoiding chi's constraint that middleware must precede routes.
	Use(middlewares ...func(http.Handler) http.Handler)

	// Middlewares returns all collected middleware functions.
	Middlewares() []func(http.Handler) http.Handler
}
