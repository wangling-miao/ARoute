// Package http provides route registration services for other plugins.
// It exposes a RouteRegistrar interface through the ServiceContainer,
// allowing plugins to register their own routes without direct import.
package http

import (
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

	// Use appends middleware to the router.
	// Middleware is applied to all routes registered after this call.
	Use(middlewares ...func(http.Handler) http.Handler)
}

// routeRegistrar implements the RouteRegistrar interface.
type routeRegistrar struct {
	router chi.Router
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
	// Chi router accepts http.HandlerFunc
	if h, ok := handler.(http.HandlerFunc); ok {
		r.router.HandleFunc(pattern, h)
	} else if h, ok := handler.(func(http.ResponseWriter, *http.Request)); ok {
		r.router.HandleFunc(pattern, h)
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

// Use appends middleware to the router.
func (r *routeRegistrar) Use(middlewares ...func(http.Handler) http.Handler) {
	r.router.Use(middlewares...)
}
