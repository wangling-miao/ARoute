package registry

// ComputeImpact computes the transitive closure of dependent routes
// starting from the given route, using pure route data without database access.
func ComputeImpact(route *Route, allRoutes []*Route) *RemovalImpact {
	impact := &RemovalImpact{}
	if route.TrustState == TrustStateQuarantined || route.TrustState == TrustStateDisabled || route.State == RouteStateQuarantined {
		impact.RiskReasons = append(impact.RiskReasons, "source route is isolated or disabled by trust policy")
		impact.RecommendedActions = append(impact.RecommendedActions, "re-evaluate dependents and downgrade to guarded if alternate providers are unavailable")
	}

	// Build forward dependency map: provider -> dependents
	dependentsOf := make(map[string][]*Route)
	for _, r := range allRoutes {
		for _, dep := range r.Requires {
			dependentsOf[dep.String()] = append(dependentsOf[dep.String()], r)
		}
	}

	// BFS to find all transitive dependents
	visited := make(map[string]bool)
	queue := []string{route.Ref().String()}
	visited[route.Ref().String()] = true

	directSet := make(map[string]bool)
	for _, d := range dependentsOf[route.Ref().String()] {
		directSet[d.Ref().String()] = true
	}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		for _, dep := range dependentsOf[current] {
			depKey := dep.Ref().String()
			if visited[depKey] {
				continue
			}
			visited[depKey] = true

			// Check if this dependent has other providers for each of its requirements
			hasOtherProviders := false
			for _, req := range dep.Requires {
				if req.String() == current {
					continue
				}
				for _, r := range allRoutes {
					if r.Ref().String() != current && r.Enabled {
						for _, p := range r.Provides {
							if p.String() == req.String() {
								hasOtherProviders = true
								break
							}
						}
					}
					if hasOtherProviders {
						break
					}
				}
			}

			if !hasOtherProviders {
				impact.OrphanedRoutes = append(impact.OrphanedRoutes, dep.Ref())
				impact.RiskReasons = append(impact.RiskReasons, dep.Ref().String()+" loses its only trusted provider")
				impact.RecommendedActions = append(impact.RecommendedActions, "mark "+dep.Ref().String()+" as orphaned")
			} else if route.TrustState == TrustStateQuarantined || route.State == RouteStateQuarantined {
				impact.RiskReasons = append(impact.RiskReasons, dep.Ref().String()+" is reachable from quarantined route")
				impact.RecommendedActions = append(impact.RecommendedActions, "mark "+dep.Ref().String()+" as guarded")
			}

			if directSet[depKey] {
				impact.DirectlyAffected = append(impact.DirectlyAffected, dep.Ref())
			} else {
				impact.TransitivelyAffected = append(impact.TransitivelyAffected, dep.Ref())
			}

			queue = append(queue, depKey)
		}
	}

	return impact
}
