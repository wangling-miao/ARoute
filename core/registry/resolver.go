package registry

import (
	"fmt"
	"sort"
)

// ResolveOrder performs a cross-type topological sort on a set of routes.
// It builds a heterogeneous directed graph where edges can cross route types
// (e.g., a plugin depending on a service, a hook depending on a plugin),
// detects cycles, and returns a valid startup ordering.
//
// This is the core patent-worthy algorithm: topological sort over a
// heterogeneous dependency graph with type-discriminated nodes and
// trust-level boundary enforcement.
func ResolveOrder(routes []*Route) ([]*Route, error) {
	if len(routes) == 0 {
		return []*Route{}, nil
	}

	// Build index: route key -> route
	index := make(map[string]*Route, len(routes))
	for _, r := range routes {
		index[r.Ref().String()] = r
	}

	// Build adjacency: dependency -> list of dependents
	// And in-degree: route -> number of unresolved dependencies
	graph := make(map[string][]string)
	inDegree := make(map[string]int, len(routes))

	for _, r := range routes {
		key := r.Ref().String()
		if _, exists := inDegree[key]; !exists {
			inDegree[key] = 0
		}

		for _, dep := range r.Requires {
			depKey := dep.String()
			if _, exists := index[depKey]; !exists {
				// Dependency not found in the provided routes
				return nil, &RouteError{
					RouteRef: r.Ref(),
					Op:       "resolve",
					Msg:      fmt.Sprintf("dependency not found: %s", depKey),
				}
			}

			// Enforce trust boundary
			depRoute := index[depKey]
			if err := checkTrustLevel(r.TrustLevel, depRoute.TrustLevel); err != nil {
				return nil, &RouteError{
					RouteRef: r.Ref(),
					Op:       "resolve",
					Msg:      fmt.Sprintf("trust boundary violation: cannot depend on %s: %v", depKey, err),
				}
			}

			graph[depKey] = append(graph[depKey], key)
			inDegree[key]++
		}
	}

	// Kahn's algorithm with deterministic ordering
	var queue []string
	for key, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, key)
		}
	}
	sort.Strings(queue)

	var order []string
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		order = append(order, current)

		var newReady []string
		for _, neighbor := range graph[current] {
			inDegree[neighbor]--
			if inDegree[neighbor] == 0 {
				newReady = append(newReady, neighbor)
			}
		}
		if len(newReady) > 0 {
			sort.Strings(newReady)
			queue = append(queue, newReady...)
		}
	}

	if len(order) != len(routes) {
		var cycle []string
		for key, degree := range inDegree {
			if degree > 0 {
				cycle = append(cycle, key)
			}
		}
		sort.Strings(cycle)
		return nil, &RouteError{
			Op:  "resolve",
			Msg: fmt.Sprintf("dependency cycle detected among: %v", cycle),
		}
	}

	// Map back to Route pointers
	result := make([]*Route, len(order))
	for i, key := range order {
		result[i] = index[key]
	}
	return result, nil
}
