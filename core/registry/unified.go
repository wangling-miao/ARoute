package registry

import "errors"

// UnifiedRegistry is the central routing table for all ARoute entities.
// It manages routes of all types (plugin, service, hook, theme, middleware, license)
// with cross-type dependency resolution, trust-level enforcement, and atomic hot-swap.
//
// Thread safety: All methods must be safe for concurrent use.
type UnifiedRegistry interface {
	// Register adds a route to the routing table.
	// Validates schema based on RouteType, checks for conflicts,
	// enforces trust-level constraints, and updates the dependency graph.
	Register(route *Route) error

	// Get retrieves a route by its composite key.
	Get(routeType RouteType, domain, name string) (*Route, error)

	// List returns routes matching optional filters.
	List(opts ListOptions) ([]*Route, error)

	// Update modifies a route's payload while preserving state.
	Update(routeType RouteType, domain, name string, payload []byte) error

	// Remove deletes a route, computing cascading impact first.
	Remove(routeType RouteType, domain, name string) (*RemovalImpact, error)

	// Enable sets a route's enabled state to true and state to active.
	Enable(routeType RouteType, domain, name string) error

	// Disable sets a route's enabled state to false and computes cascading impact.
	Disable(routeType RouteType, domain, name string) (*RemovalImpact, error)

	// Dependents returns all routes that depend on the given route.
	Dependents(ref RouteRef) ([]*Route, error)

	// Dependencies returns all routes the given route depends on.
	Dependencies(ref RouteRef) ([]*Route, error)

	// ResolutionOrder returns a topological ordering across all route types,
	// respecting trust-level boundaries.
	ResolutionOrder() ([]*Route, error)

	// ValidateDependencies checks if all declared dependencies are satisfiable.
	ValidateDependencies(route *Route) ([]RouteRef, error)

	// CheckTrustBoundary verifies that a consumer at a given trust level
	// can access the referenced provider route.
	CheckTrustBoundary(consumer TrustLevel, provider RouteRef) error

	// HotSwap atomically replaces a route with a new version,
	// triggering cascading state updates for all dependents.
	HotSwap(routeType RouteType, domain, name string, newRoute *Route) error

	// Impact returns the cascading effect of disabling or removing a route
	// without performing the action.
	Impact(routeType RouteType, domain, name string) (*RemovalImpact, error)

	// Close releases any resources used by the registry.
	Close() error
}

// ListOptions provides filtering for List operations.
type ListOptions struct {
	Type       *RouteType
	Domain     string
	Enabled    *bool
	TrustLevel *TrustLevel
	State      *RouteState
}

// Matches checks if a route matches all non-nil filters.
func (o ListOptions) Matches(r *Route) bool {
	if o.Type != nil && r.Type != *o.Type {
		return false
	}
	if o.Domain != "" && r.Domain != o.Domain {
		return false
	}
	if o.Enabled != nil && r.Enabled != *o.Enabled {
		return false
	}
	if o.TrustLevel != nil && r.TrustLevel != *o.TrustLevel {
		return false
	}
	if o.State != nil && r.State != *o.State {
		return false
	}
	return true
}

// RemovalImpact describes the cascading effect of disabling/removing a route.
type RemovalImpact struct {
	DirectlyAffected     []RouteRef
	TransitivelyAffected []RouteRef
	OrphanedRoutes       []RouteRef
	TrustViolations      []string
	RiskReasons          []string
	RecommendedActions   []string
}

// HasImpact returns true if the removal affects any other routes.
func (ri *RemovalImpact) HasImpact() bool {
	return len(ri.DirectlyAffected) > 0 ||
		len(ri.TransitivelyAffected) > 0 ||
		len(ri.OrphanedRoutes) > 0 ||
		len(ri.TrustViolations) > 0 ||
		len(ri.RiskReasons) > 0 ||
		len(ri.RecommendedActions) > 0
}

// Errors returned by the unified registry.
var (
	ErrRouteNotFound         = &RouteError{Op: "get", Msg: "route not found"}
	ErrRouteExists           = &RouteError{Op: "register", Msg: "route already exists"}
	ErrUnifiedRegistryClosed = &RouteError{Op: "registry", Msg: "registry is closed"}
	ErrTrustViolation        = &RouteError{Op: "trust", Msg: "trust boundary violation"}
	ErrDependencyCycle       = &RouteError{Op: "resolve", Msg: "dependency cycle detected"}
)

// RouteError represents an error from unified registry operations.
type RouteError struct {
	RouteRef RouteRef
	Op       string
	Msg      string
}

func (e *RouteError) Error() string {
	if e.RouteRef.Name != "" {
		return "registry: " + e.Op + " " + e.RouteRef.String() + ": " + e.Msg
	}
	return "registry: " + e.Op + ": " + e.Msg
}

// IsRouteNotFound returns true if err is ErrRouteNotFound or wraps it.
func IsRouteNotFound(err error) bool {
	var re *RouteError
	if errors.As(err, &re) {
		return re.Op == "get" && re.Msg == "route not found"
	}
	return false
}

// IsRouteExists returns true if err is ErrRouteExists or wraps it.
func IsRouteExists(err error) bool {
	var re *RouteError
	if errors.As(err, &re) {
		return re.Op == "register" && re.Msg == "route already exists"
	}
	return false
}
