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

	// Use appends middleware to the router.
	Use(middlewares ...func(http.Handler) http.Handler)
}
