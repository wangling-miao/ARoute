package registry

import "fmt"

// HotSwapResult describes the outcome of a hot-swap operation.
type HotSwapResult struct {
	Route          *Route
	AffectedRoutes []*RouteRef
	OrphanedRoutes []*RouteRef
}

// ValidateHotSwap checks whether a hot-swap can succeed without performing it.
// It validates the new route and checks that the old route exists.
func ValidateHotSwap(reg UnifiedRegistry, oldType RouteType, oldDomain, oldName string, newRoute *Route) error {
	if newRoute == nil {
		return &RouteError{Op: "hotswap", Msg: "new route cannot be nil"}
	}

	if err := newRoute.Validate(); err != nil {
		return &RouteError{
			RouteRef: newRoute.Ref(),
			Op:       "hotswap",
			Msg:      fmt.Sprintf("invalid route: %v", err),
		}
	}

	// Verify old route exists
	_, err := reg.Get(oldType, oldDomain, oldName)
	if err != nil {
		return &RouteError{
			RouteRef: RouteRef{Type: oldType, Domain: oldDomain, Name: oldName},
			Op:       "hotswap",
			Msg:      "route to replace not found",
		}
	}

	// Validate new dependencies
	unsatisfied, err := reg.ValidateDependencies(newRoute)
	if err != nil {
		return err
	}
	if len(unsatisfied) > 0 {
		var names []string
		for _, u := range unsatisfied {
			names = append(names, u.String())
		}
		return &RouteError{
			RouteRef: newRoute.Ref(),
			Op:       "hotswap",
			Msg:      fmt.Sprintf("unsatisfied dependencies: %v", names),
		}
	}

	// Check trust boundaries for new dependencies
	for _, dep := range newRoute.Requires {
		if err := reg.CheckTrustBoundary(newRoute.TrustLevel, dep); err != nil {
			return &RouteError{
				RouteRef: newRoute.Ref(),
				Op:       "hotswap",
				Msg:      fmt.Sprintf("trust boundary violation: %v", err),
			}
		}
	}

	return nil
}
