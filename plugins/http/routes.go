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
	// Handle registers a route pattern with an http.Handler.
	Handle(pattern string, handler http.Handler)

	// HandleFunc registers a route pattern with an http.HandlerFunc.
	HandleFunc(pattern string, handler http.HandlerFunc)

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

// Handle registers a route pattern with an http.Handler.
func (r *routeRegistrar) Handle(pattern string, handler http.Handler) {
	r.router.Handle(pattern, handler)
}

// HandleFunc registers a route pattern with an http.HandlerFunc.
func (r *routeRegistrar) HandleFunc(pattern string, handler http.HandlerFunc) {
	r.router.HandleFunc(pattern, handler)
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
