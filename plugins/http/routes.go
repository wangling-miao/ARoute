// Package http provides route registration services for other plugins.
// It exposes a RouteRegistrar interface through the ServiceContainer,
// allowing plugins to register their own routes without direct import.
package http

import (
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// RouteRegistrar defines the interface for route registration.
// Plugins use this service to register their HTTP routes.
type RouteRegistrar interface {
	// Register registers a route pattern with handler.
	// Pattern: HTTP method and path (e.g., "GET /api/users")
	Register(pattern string, handler interface{})

	// Route creates a sub-router for a path prefix.
	// Useful for organizing routes by domain.
	Route(pattern string, fn func(r chi.Router))

	// Group creates a new route group with shared middleware.
	// Middleware applied to the group affects all routes in it.
	Group(fn func(r chi.Router))

	// Mount mounts another router at a path prefix.
	// Useful for mounting sub-applications or API versions.
	Mount(pattern string, router chi.Router)

	// Use collects middleware to be applied to all routes.
	// Middleware is collected during the init phase and applied
	// before the HTTP server starts, avoiding chi's "middleware before routes" constraint.
	Use(middlewares ...func(http.Handler) http.Handler)

	// Middlewares returns all collected middleware functions.
	// Called by the HTTP plugin during Start to apply cross-plugin middleware.
	Middlewares() []func(http.Handler) http.Handler
}

// routeRegistrar implements the RouteRegistrar interface.
type routeRegistrar struct {
	router      chi.Router
	middlewares []func(http.Handler) http.Handler
}

// NewRouteRegistrar creates a new RouteRegistrar instance.
func NewRouteRegistrar(router chi.Router) RouteRegistrar {
	return &routeRegistrar{
		router: router,
	}
}

// Register registers a route pattern with handler.
// Supports standard chi patterns: "GET /path", "POST /path", etc.
func (r *routeRegistrar) Register(pattern string, handler interface{}) {
	if h, ok := handler.(http.HandlerFunc); ok {
		r.router.HandleFunc(pattern, h)
	} else if h, ok := handler.(func(http.ResponseWriter, *http.Request)); ok {
		r.router.HandleFunc(pattern, h)
	} else {
		log.Printf("[http] WARNING: Register() received unsupported handler type %T, ignoring", handler)
	}
}

// Route creates a sub-router for a path prefix.
func (r *routeRegistrar) Route(pattern string, fn func(r chi.Router)) {
	r.router.Route(pattern, fn)
}

// Group creates a new route group with shared middleware.
func (r *routeRegistrar) Group(fn func(r chi.Router)) {
	r.router.Group(fn)
}

// Mount mounts another router at a path prefix.
func (r *routeRegistrar) Mount(pattern string, router chi.Router) {
	r.router.Mount(pattern, router)
}

// Use collects middleware to be applied later.
// Middleware is collected during init phase and applied as an outer
// wrapper before the HTTP server starts. This avoids chi's constraint
// that all middleware must be defined before routes on a mux.
func (r *routeRegistrar) Use(middlewares ...func(http.Handler) http.Handler) {
	r.middlewares = append(r.middlewares, middlewares...)
}

// Middlewares returns all collected middleware functions.
func (r *routeRegistrar) Middlewares() []func(http.Handler) http.Handler {
	return r.middlewares
}
