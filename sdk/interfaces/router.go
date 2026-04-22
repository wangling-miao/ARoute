package interfaces

import (
	"net/http"
)

// RouteRegistrar defines the interface for HTTP route registration.
// Plugins use this service (provided by the HTTP plugin via ServiceContainer)
// to register their HTTP routes using only standard library types.
type RouteRegistrar interface {
	// Handle registers a route pattern with an http.Handler.
	Handle(pattern string, handler http.Handler)

	// HandleFunc registers a route pattern with an http.HandlerFunc.
	HandleFunc(pattern string, handler http.HandlerFunc)

	// Use collects middleware to be applied before the HTTP server starts.
	Use(middlewares ...func(http.Handler) http.Handler)

	// Middlewares returns all collected middleware functions.
	Middlewares() []func(http.Handler) http.Handler
}
