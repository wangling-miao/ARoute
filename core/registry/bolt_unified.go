package registry

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	bolt "go.etcd.io/bbolt"
)

const (
	// Bucket prefix for route data, one bucket per route type.
	routeBucketPrefix = "routes:"

	// Bucket for forward dependency index (route -> its dependents).
	depsForwardBucket = "deps:forward"

	// Bucket for reverse dependency index (route -> its dependencies).
	depsReverseBucket = "deps:reverse"

	// Schema version tracking bucket.
	metaBucket = "meta"
)

// allRouteTypes lists every recognized route type for bucket creation.
var allRouteTypes = []RouteType{
	RouteTypePlugin,
	RouteTypeService,
	RouteTypeHook,
	RouteTypeTheme,
	RouteTypeMiddleware,
	RouteTypeLicense,
}

// BoltUnifiedRegistry implements UnifiedRegistry using bbolt as the persistent backend.
// Routes are stored in type-partitioned buckets. Dependency indexes are maintained
// in separate forward and reverse buckets for O(1) lookups.
type BoltUnifiedRegistry struct {
	mu     sync.RWMutex
	db     *bolt.DB
	closed bool
}

// NewBoltUnifiedRegistry creates a new unified registry backed by bbolt.
func NewBoltUnifiedRegistry(dbPath string) (*BoltUnifiedRegistry, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, fmt.Errorf("create registry directory: %w", err)
	}

	db, err := bolt.Open(dbPath, 0600, &bolt.Options{Timeout: 5 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("open bbolt database: %w", err)
	}

	// Create all required buckets
	err = db.Update(func(tx *bolt.Tx) error {
		for _, rt := range allRouteTypes {
			if _, err := tx.CreateBucketIfNotExists([]byte(routeBucketPrefix + string(rt))); err != nil {
				return fmt.Errorf("create bucket %s: %w", routeBucketPrefix+string(rt), err)
			}
		}
		if _, err := tx.CreateBucketIfNotExists([]byte(depsForwardBucket)); err != nil {
			return fmt.Errorf("create deps forward bucket: %w", err)
		}
		if _, err := tx.CreateBucketIfNotExists([]byte(depsReverseBucket)); err != nil {
			return fmt.Errorf("create deps reverse bucket: %w", err)
		}
		if _, err := tx.CreateBucketIfNotExists([]byte(metaBucket)); err != nil {
			return fmt.Errorf("create meta bucket: %w", err)
		}
		return nil
	})
	if err != nil {
		db.Close()
		return nil, err
	}

	 reg := &BoltUnifiedRegistry{db: db}

	// Auto-migrate from legacy "plugins" bucket if it exists.
	if err := reg.migrateLegacyBucket(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate legacy data: %w", err)
	}

	return reg, nil
}

// migrateLegacyBucket checks for the old "plugins" bucket and migrates
// its entries into the new type-partitioned format. After migration the
// old bucket is deleted.
func (r *BoltUnifiedRegistry) migrateLegacyBucket() error {
	return r.db.Update(func(tx *bolt.Tx) error {
		old := tx.Bucket([]byte("plugins"))
		if old == nil {
			return nil
		}

		target := tx.Bucket([]byte(routeBucketName(RouteTypePlugin)))
		if target == nil {
			return nil
		}

		migrated := 0
		cursor := old.Cursor()
		for k, v := cursor.First(); k != nil; k, v = cursor.Next() {
			var entry PluginEntry
			if err := json.Unmarshal(v, &entry); err != nil {
				continue
			}

			route := RouteFromPluginEntry(&entry)
			route.RegisteredAt = time.Now()
			route.UpdatedAt = time.Now()

			data, err := json.Marshal(route)
			if err != nil {
				continue
			}

			key := routeKey(route.Domain, route.Name)
			if target.Get(key) == nil {
				if err := target.Put(key, data); err != nil {
					continue
				}
				migrated++
			}
		}

		if migrated > 0 {
			_ = tx.DeleteBucket([]byte("plugins"))
		}

		return nil
	})
}

// PluginStartupOrder returns the plugin names in dependency-resolved startup order.
// It uses the cross-type topological sort but only returns plugin-type entries.
func (r *BoltUnifiedRegistry) PluginStartupOrder() ([]string, error) {
	routes, err := r.ResolutionOrder()
	if err != nil {
		return nil, err
	}

	var names []string
	for _, route := range routes {
		if route.Type == RouteTypePlugin {
			names = append(names, route.Name)
		}
	}
	return names, nil
}

// routeBucketName returns the bucket name for a given route type.
func routeBucketName(rt RouteType) string {
	return routeBucketPrefix + string(rt)
}

// routeKey builds the storage key for a route within its type bucket.
func routeKey(domain, name string) []byte {
	return []byte(domain + "/" + name)
}

// Register adds a route to the routing table.
func (r *BoltUnifiedRegistry) Register(route *Route) error {
	if route == nil {
		return &RouteError{Op: "register", Msg: "route cannot be nil"}
	}

	if err := route.Validate(); err != nil {
		return &RouteError{
			RouteRef: route.Ref(),
			Op:       "register",
			Msg:      fmt.Sprintf("invalid route: %v", err),
		}
	}

	// Check trust boundaries for declared dependencies
	for _, dep := range route.Requires {
		if err := r.CheckTrustBoundary(route.TrustLevel, dep); err != nil {
			return err
		}
	}

	now := time.Now()
	route.RegisteredAt = now
	route.UpdatedAt = now
	if route.State == "" {
		route.State = RouteStateInactive
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return ErrUnifiedRegistryClosed
	}

	return r.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(routeBucketName(route.Type)))
		if bucket == nil {
			return &RouteError{
				RouteRef: route.Ref(),
				Op:       "register",
				Msg:      "bucket not found",
			}
		}

		key := routeKey(route.Domain, route.Name)
		if bucket.Get(key) != nil {
			return &RouteError{
				RouteRef: route.Ref(),
				Op:       "register",
				Msg:      "route already exists",
			}
		}

		data, err := json.Marshal(route)
		if err != nil {
			return fmt.Errorf("serialize route: %w", err)
		}

		if err := bucket.Put(key, data); err != nil {
			return err
		}

		// Update dependency indexes
		return r.updateDependencyIndexes(tx, route)
	})
}

// Get retrieves a route by its composite key.
func (r *BoltUnifiedRegistry) Get(routeType RouteType, domain, name string) (*Route, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.closed {
		return nil, ErrRegistryClosed
	}

	var route Route
	err := r.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(routeBucketName(routeType)))
		if bucket == nil {
			return &RouteError{
				RouteRef: RouteRef{Type: routeType, Domain: domain, Name: name},
				Op:       "get",
				Msg:      "route not found",
			}
		}

		data := bucket.Get(routeKey(domain, name))
		if data == nil {
			return &RouteError{
				RouteRef: RouteRef{Type: routeType, Domain: domain, Name: name},
				Op:       "get",
				Msg:      "route not found",
			}
		}

		return json.Unmarshal(data, &route)
	})
	if err != nil {
		return nil, err
	}

	return &route, nil
}

// List returns routes matching optional filters.
func (r *BoltUnifiedRegistry) List(opts ListOptions) ([]*Route, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.closed {
		return nil, ErrRegistryClosed
	}

	var routes []*Route

	err := r.db.View(func(tx *bolt.Tx) error {
		if opts.Type != nil {
			return r.listBucket(tx, routeBucketName(*opts.Type), opts, &routes)
		}
		for _, rt := range allRouteTypes {
			if err := r.listBucket(tx, routeBucketName(rt), opts, &routes); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	if routes == nil {
		routes = []*Route{}
	}
	return routes, nil
}

func (r *BoltUnifiedRegistry) listBucket(tx *bolt.Tx, bucketName string, opts ListOptions, routes *[]*Route) error {
	bucket := tx.Bucket([]byte(bucketName))
	if bucket == nil {
		return nil
	}

	return bucket.ForEach(func(key, value []byte) error {
		var route Route
		if err := json.Unmarshal(value, &route); err != nil {
			return nil // skip malformed entries
		}
		if opts.Matches(&route) {
			*routes = append(*routes, &route)
		}
		return nil
	})
}

// Update modifies a route's payload while preserving state.
func (r *BoltUnifiedRegistry) Update(routeType RouteType, domain, name string, payload []byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return ErrUnifiedRegistryClosed
	}

	ref := RouteRef{Type: routeType, Domain: domain, Name: name}

	return r.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(routeBucketName(routeType)))
		if bucket == nil {
			return &RouteError{RouteRef: ref, Op: "update", Msg: "route not found"}
		}

		key := routeKey(domain, name)
		data := bucket.Get(key)
		if data == nil {
			return &RouteError{RouteRef: ref, Op: "update", Msg: "route not found"}
		}

		var route Route
		if err := json.Unmarshal(data, &route); err != nil {
			return fmt.Errorf("deserialize route: %w", err)
		}

		route.Payload = payload
		route.UpdatedAt = time.Now()

		newData, err := json.Marshal(&route)
		if err != nil {
			return fmt.Errorf("serialize route: %w", err)
		}

		return bucket.Put(key, newData)
	})
}

// Remove deletes a route and returns its cascading impact.
func (r *BoltUnifiedRegistry) Remove(routeType RouteType, domain, name string) (*RemovalImpact, error) {
	impact, err := r.Impact(routeType, domain, name)
	if err != nil {
		return nil, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return nil, ErrRegistryClosed
	}

	ref := RouteRef{Type: routeType, Domain: domain, Name: name}

	err = r.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(routeBucketName(routeType)))
		if bucket == nil {
			return &RouteError{RouteRef: ref, Op: "remove", Msg: "route not found"}
		}

		key := routeKey(domain, name)
		if bucket.Get(key) == nil {
			return &RouteError{RouteRef: ref, Op: "remove", Msg: "route not found"}
		}

		// Remove dependency indexes
		if err := r.removeDependencyIndexes(tx, ref); err != nil {
			return err
		}

		// Mark orphaned dependents
		for _, depRef := range impact.DirectlyAffected {
			r.markDependentOrphaned(tx, depRef)
		}

		return bucket.Delete(key)
	})

	return impact, err
}

// Enable sets a route's enabled state to true.
func (r *BoltUnifiedRegistry) Enable(routeType RouteType, domain, name string) error {
	return r.setEnabled(routeType, domain, name, true, RouteStateActive)
}

// Disable sets a route's enabled state to false.
func (r *BoltUnifiedRegistry) Disable(routeType RouteType, domain, name string) (*RemovalImpact, error) {
	impact, err := r.Impact(routeType, domain, name)
	if err != nil {
		return nil, err
	}

	if err := r.setEnabled(routeType, domain, name, false, RouteStateInactive); err != nil {
		return nil, err
	}

	return impact, nil
}

func (r *BoltUnifiedRegistry) setEnabled(routeType RouteType, domain, name string, enabled bool, state RouteState) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return ErrUnifiedRegistryClosed
	}

	ref := RouteRef{Type: routeType, Domain: domain, Name: name}

	return r.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(routeBucketName(routeType)))
		if bucket == nil {
			return &RouteError{RouteRef: ref, Op: "enable", Msg: "route not found"}
		}

		key := routeKey(domain, name)
		data := bucket.Get(key)
		if data == nil {
			return &RouteError{RouteRef: ref, Op: "enable", Msg: "route not found"}
		}

		var route Route
		if err := json.Unmarshal(data, &route); err != nil {
			return fmt.Errorf("deserialize route: %w", err)
		}

		if route.Enabled == enabled && route.State == state {
			return nil
		}

		route.Enabled = enabled
		route.State = state
		route.UpdatedAt = time.Now()

		newData, err := json.Marshal(&route)
		if err != nil {
			return fmt.Errorf("serialize route: %w", err)
		}

		return bucket.Put(key, newData)
	})
}

// Dependents returns all routes that depend on the given route.
func (r *BoltUnifiedRegistry) Dependents(ref RouteRef) ([]*Route, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.closed {
		return nil, ErrRegistryClosed
	}

	var refs []RouteRef
	err := r.db.View(func(tx *bolt.Tx) error {
		fwd := tx.Bucket([]byte(depsForwardBucket))
		if fwd == nil {
			return nil
		}
		data := fwd.Get([]byte(ref.String()))
		if data == nil {
			return nil
		}
		return json.Unmarshal(data, &refs)
	})
	if err != nil {
		return nil, err
	}

	var routes []*Route
	for _, depRef := range refs {
		route, err := r.getLocked(txRef{depRef.Type, depRef.Domain, depRef.Name})
		if err != nil {
			continue
		}
		routes = append(routes, route)
	}

	if routes == nil {
		routes = []*Route{}
	}
	return routes, nil
}

// Dependencies returns all routes the given route depends on.
func (r *BoltUnifiedRegistry) Dependencies(ref RouteRef) ([]*Route, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.closed {
		return nil, ErrRegistryClosed
	}

	var refs []RouteRef
	err := r.db.View(func(tx *bolt.Tx) error {
		rev := tx.Bucket([]byte(depsReverseBucket))
		if rev == nil {
			return nil
		}
		data := rev.Get([]byte(ref.String()))
		if data == nil {
			return nil
		}
		return json.Unmarshal(data, &refs)
	})
	if err != nil {
		return nil, err
	}

	var routes []*Route
	for _, depRef := range refs {
		route, err := r.getLocked(txRef{depRef.Type, depRef.Domain, depRef.Name})
		if err != nil {
			continue
		}
		routes = append(routes, route)
	}

	if routes == nil {
		routes = []*Route{}
	}
	return routes, nil
}

// txRef is a lightweight struct for internal lookups without holding the lock.
type txRef struct {
	Type   RouteType
	Domain string
	Name   string
}

// getLocked reads a route assuming the caller already holds the read lock.
func (r *BoltUnifiedRegistry) getLocked(ref txRef) (*Route, error) {
	var route Route
	err := r.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(routeBucketName(ref.Type)))
		if bucket == nil {
			return &RouteError{
				RouteRef: RouteRef{Type: ref.Type, Domain: ref.Domain, Name: ref.Name},
				Op:       "get", Msg: "route not found",
			}
		}
		data := bucket.Get(routeKey(ref.Domain, ref.Name))
		if data == nil {
			return &RouteError{
				RouteRef: RouteRef{Type: ref.Type, Domain: ref.Domain, Name: ref.Name},
				Op:       "get", Msg: "route not found",
			}
		}
		return json.Unmarshal(data, &route)
	})
	if err != nil {
		return nil, err
	}
	return &route, nil
}

// ResolutionOrder returns a topological ordering across all route types.
func (r *BoltUnifiedRegistry) ResolutionOrder() ([]*Route, error) {
	routes, err := r.List(ListOptions{})
	if err != nil {
		return nil, err
	}
	return ResolveOrder(routes)
}

// ValidateDependencies checks if all declared dependencies are satisfiable.
func (r *BoltUnifiedRegistry) ValidateDependencies(route *Route) ([]RouteRef, error) {
	var unsatisfied []RouteRef
	for _, dep := range route.Requires {
		_, err := r.Get(dep.Type, dep.Domain, dep.Name)
		if err != nil {
			unsatisfied = append(unsatisfied, dep)
		}
	}
	return unsatisfied, nil
}

// CheckTrustBoundary verifies that a consumer at a given trust level
// can access the referenced provider route.
func (r *BoltUnifiedRegistry) CheckTrustBoundary(consumer TrustLevel, provider RouteRef) error {
	providerRoute, err := r.Get(provider.Type, provider.Domain, provider.Name)
	if err != nil {
		// Provider doesn't exist yet; validate structurally
		return checkTrustLevel(consumer, TrustL1)
	}
	return checkTrustLevel(consumer, providerRoute.TrustLevel)
}

// HotSwap atomically replaces a route with a new version.
func (r *BoltUnifiedRegistry) HotSwap(routeType RouteType, domain, name string, newRoute *Route) error {
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

	// Check trust boundaries for new dependencies
	for _, dep := range newRoute.Requires {
		if err := r.CheckTrustBoundary(newRoute.TrustLevel, dep); err != nil {
			return err
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return ErrUnifiedRegistryClosed
	}

	ref := RouteRef{Type: routeType, Domain: domain, Name: name}
	oldKey := routeKey(domain, name)

	return r.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(routeBucketName(routeType)))
		if bucket == nil {
			return &RouteError{RouteRef: ref, Op: "hotswap", Msg: "route not found"}
		}

		oldData := bucket.Get(oldKey)
		if oldData == nil {
			return &RouteError{RouteRef: ref, Op: "hotswap", Msg: "route not found"}
		}

		var oldRoute Route
		if err := json.Unmarshal(oldData, &oldRoute); err != nil {
			return fmt.Errorf("deserialize old route: %w", err)
		}

		// Preserve registration timestamp and enabled state from old route
		newRoute.RegisteredAt = oldRoute.RegisteredAt
		newRoute.UpdatedAt = time.Now()
		newRoute.Enabled = oldRoute.Enabled

		// Remove old dependency indexes
		if err := r.removeDependencyIndexes(tx, ref); err != nil {
			return err
		}

		newData, err := json.Marshal(newRoute)
		if err != nil {
			return fmt.Errorf("serialize new route: %w", err)
		}

		// Handle key change if domain/name changed
		newKey := routeKey(newRoute.Domain, newRoute.Name)
		if string(newKey) != string(oldKey) {
			if err := bucket.Delete(oldKey); err != nil {
				return err
			}
		}

		if err := bucket.Put(newKey, newData); err != nil {
			return err
		}

		// Add new dependency indexes
		return r.updateDependencyIndexes(tx, newRoute)
	})
}

// Impact returns the cascading effect of removing a route without performing it.
func (r *BoltUnifiedRegistry) Impact(routeType RouteType, domain, name string) (*RemovalImpact, error) {
	ref := RouteRef{Type: routeType, Domain: domain, Name: name}

	dependents, err := r.Dependents(ref)
	if err != nil {
		return &RemovalImpact{}, nil
	}

	impact := &RemovalImpact{}

	for _, dep := range dependents {
		impact.DirectlyAffected = append(impact.DirectlyAffected, dep.Ref())

		// Compute transitive closure
		transitive, _ := r.computeTransitiveDependents(dep.Ref(), map[string]bool{ref.String(): true})
		impact.TransitivelyAffected = append(impact.TransitivelyAffected, transitive...)

		// Check if dependent will become orphaned
		remainingDeps, _ := r.Dependencies(dep.Ref())
		hasOtherProviders := false
		for _, d := range remainingDeps {
			if d.Ref().String() != ref.String() {
				hasOtherProviders = true
				break
			}
		}
		if !hasOtherProviders {
			impact.OrphanedRoutes = append(impact.OrphanedRoutes, dep.Ref())
		}
	}

	return impact, nil
}

// Close releases the database connection.
func (r *BoltUnifiedRegistry) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return nil
	}
	r.closed = true
	err := r.db.Close()
	r.db = nil
	return err
}

// --- Internal helpers ---

func (r *BoltUnifiedRegistry) updateDependencyIndexes(tx *bolt.Tx, route *Route) error {
	fwd := tx.Bucket([]byte(depsForwardBucket))
	rev := tx.Bucket([]byte(depsReverseBucket))
	if fwd == nil || rev == nil {
		return nil
	}

	refKey := []byte(route.Ref().String())

	// Store reverse index: this route -> its dependencies
	if len(route.Requires) > 0 {
		data, err := json.Marshal(route.Requires)
		if err != nil {
			return fmt.Errorf("serialize dependencies: %w", err)
		}
		if err := rev.Put(refKey, data); err != nil {
			return err
		}
	}

	// Update forward index: for each dependency, add this route as a dependent
	for _, dep := range route.Requires {
		depKey := []byte(dep.String())
		var dependents []RouteRef
		if data := fwd.Get(depKey); data != nil {
			_ = json.Unmarshal(data, &dependents)
		}
		dependents = append(dependents, route.Ref())
		data, err := json.Marshal(dependents)
		if err != nil {
			return err
		}
		if err := fwd.Put(depKey, data); err != nil {
			return err
		}
	}

	return nil
}

func (r *BoltUnifiedRegistry) removeDependencyIndexes(tx *bolt.Tx, ref RouteRef) error {
	fwd := tx.Bucket([]byte(depsForwardBucket))
	rev := tx.Bucket([]byte(depsReverseBucket))
	if fwd == nil || rev == nil {
		return nil
	}

	refKey := []byte(ref.String())

	// Get this route's dependencies to clean up forward index
	if data := rev.Get(refKey); data != nil {
		var deps []RouteRef
		if err := json.Unmarshal(data, &deps); err == nil {
			for _, dep := range deps {
				depKey := []byte(dep.String())
				if fwdData := fwd.Get(depKey); fwdData != nil {
					var dependents []RouteRef
					if err := json.Unmarshal(fwdData, &dependents); err == nil {
						filtered := dependents[:0]
						for _, d := range dependents {
							if d.String() != ref.String() {
								filtered = append(filtered, d)
							}
						}
						if len(filtered) == 0 {
							_ = fwd.Delete(depKey)
						} else {
						 newData, _ := json.Marshal(filtered)
							_ = fwd.Put(depKey, newData)
						}
					}
				}
			}
		}
	}

	// Remove reverse index
	_ = rev.Delete(refKey)

	return nil
}

func (r *BoltUnifiedRegistry) markDependentOrphaned(tx *bolt.Tx, ref RouteRef) {
	bucket := tx.Bucket([]byte(routeBucketName(ref.Type)))
	if bucket == nil {
		return
	}

	data := bucket.Get(routeKey(ref.Domain, ref.Name))
	if data == nil {
		return
	}

	var route Route
	if err := json.Unmarshal(data, &route); err != nil {
		return
	}

	route.State = RouteStateOrphaned
	route.UpdatedAt = time.Now()

	newData, err := json.Marshal(&route)
	if err != nil {
		return
	}
	_ = bucket.Put(routeKey(ref.Domain, ref.Name), newData)
}

func (r *BoltUnifiedRegistry) computeTransitiveDependents(ref RouteRef, visited map[string]bool) ([]RouteRef, error) {
	var result []RouteRef
	dependents, err := r.Dependents(ref)
	if err != nil {
		return result, err
	}

	for _, dep := range dependents {
		depStr := dep.Ref().String()
		if visited[depStr] {
			continue
		}
		visited[depStr] = true
		result = append(result, dep.Ref())
		transitive, _ := r.computeTransitiveDependents(dep.Ref(), visited)
		result = append(result, transitive...)
	}

	return result, nil
}
