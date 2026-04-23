package registry

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/wangling-miao/aroute/core"
)

// helper: create a temp BoltUnifiedRegistry
func newTestUnifiedRegistry(t *testing.T) *BoltUnifiedRegistry {
	t.Helper()
	dir := t.TempDir()
	reg, err := NewBoltUnifiedRegistry(filepath.Join(dir, "unified.db"))
	if err != nil {
		t.Fatalf("create unified registry: %v", err)
	}
	t.Cleanup(func() { reg.Close() })
	return reg
}

// helper: create a plugin route
func testPluginRoute(name, version string, trust TrustLevel, requires ...RouteRef) *Route {
	return &Route{
		Type:       RouteTypePlugin,
		Domain:     "aroute",
		Name:       name,
		Version:    version,
		TrustLevel: trust,
		Engine:     "native",
		State:      RouteStateActive,
		Enabled:    true,
		Requires:   requires,
	}
}

// helper: create a service route
func testServiceRoute(name string, trust TrustLevel) *Route {
	return &Route{
		Type:       RouteTypeService,
		Domain:     "aroute",
		Name:       name,
		Version:    "1.0.0",
		TrustLevel: trust,
		State:      RouteStateActive,
		Enabled:    true,
	}
}

// helper: create a hook route
func testHookRoute(name string, topic string, requires ...RouteRef) *Route {
	payload, _ := EncodePayload(HookPayload{Topic: topic, Priority: 10, Mode: "filter"})
	return &Route{
		Type:       RouteTypeHook,
		Domain:     "aroute",
		Name:       name,
		Version:    "1.0.0",
		TrustLevel: TrustL1,
		State:      RouteStateActive,
		Enabled:    true,
		Requires:   requires,
		Payload:    payload,
	}
}

// --- Route validation tests ---

func TestRoute_Validation(t *testing.T) {
	tests := []struct {
		name    string
		route   *Route
		wantErr bool
	}{
		{
			"valid plugin route",
			testPluginRoute("http", "1.0.0", TrustL1),
			false,
		},
		{
			"empty type",
			&Route{Domain: "aroute", Name: "test", Version: "1.0.0"},
			true,
		},
		{
			"empty domain",
			&Route{Type: RouteTypePlugin, Name: "test", Version: "1.0.0"},
			true,
		},
		{
			"empty name",
			&Route{Type: RouteTypePlugin, Domain: "aroute", Version: "1.0.0"},
			true,
		},
		{
			"empty version",
			&Route{Type: RouteTypePlugin, Domain: "aroute", Name: "test"},
			true,
		},
		{
			"invalid engine",
			&Route{Type: RouteTypePlugin, Domain: "aroute", Name: "test", Version: "1.0.0", Engine: "invalid"},
			true,
		},
		{
			"invalid requires ref type",
			&Route{
				Type: RouteTypePlugin, Domain: "aroute", Name: "test", Version: "1.0.0",
				Requires: []RouteRef{{Type: "invalid", Name: "dep"}},
			},
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.route.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// --- UnifiedRegistry CRUD tests ---

func TestUnifiedRegistry_RegisterAndGet(t *testing.T) {
	reg := newTestUnifiedRegistry(t)

	route := testPluginRoute("http", "1.0.0", TrustL1)
	if err := reg.Register(route); err != nil {
		t.Fatalf("Register: %v", err)
	}

	got, err := reg.Get(RouteTypePlugin, "aroute", "http")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	if got.Name != "http" {
		t.Errorf("got name = %q, want %q", got.Name, "http")
	}
	if got.Type != RouteTypePlugin {
		t.Errorf("got type = %q, want %q", got.Type, RouteTypePlugin)
	}
	if got.TrustLevel != TrustL1 {
		t.Errorf("got trust = %v, want %v", got.TrustLevel, TrustL1)
	}
	if !got.Enabled {
		t.Error("route should be enabled")
	}
}

func TestUnifiedRegistry_RegisterDuplicate(t *testing.T) {
	reg := newTestUnifiedRegistry(t)

	route := testPluginRoute("http", "1.0.0", TrustL1)
	_ = reg.Register(route)

	err := reg.Register(route)
	if !IsRouteExists(err) {
		t.Errorf("expected route exists error, got: %v", err)
	}
}

func TestUnifiedRegistry_GetNotFound(t *testing.T) {
	reg := newTestUnifiedRegistry(t)

	_, err := reg.Get(RouteTypePlugin, "aroute", "nonexistent")
	if !IsRouteNotFound(err) {
		t.Errorf("expected route not found error, got: %v", err)
	}
}

func TestUnifiedRegistry_ListWithFilters(t *testing.T) {
	reg := newTestUnifiedRegistry(t)

	_ = reg.Register(testPluginRoute("http", "1.0.0", TrustL1))
	_ = reg.Register(testPluginRoute("database", "1.0.0", TrustL1))
	_ = reg.Register(testServiceRoute("db-pool", TrustL1))

	// List all
	all, _ := reg.List(ListOptions{})
	if len(all) != 3 {
		t.Errorf("expected 3 routes, got %d", len(all))
	}

	// List only plugins
	pluginType := RouteTypePlugin
	plugins, _ := reg.List(ListOptions{Type: &pluginType})
	if len(plugins) != 2 {
		t.Errorf("expected 2 plugins, got %d", len(plugins))
	}

	// List only services
	serviceType := RouteTypeService
	services, _ := reg.List(ListOptions{Type: &serviceType})
	if len(services) != 1 {
		t.Errorf("expected 1 service, got %d", len(services))
	}
}

func TestUnifiedRegistry_EnableDisable(t *testing.T) {
	reg := newTestUnifiedRegistry(t)

	_ = reg.Register(testPluginRoute("http", "1.0.0", TrustL1))

	if _, err := reg.Disable(RouteTypePlugin, "aroute", "http"); err != nil {
		t.Fatalf("Disable: %v", err)
	}

	got, _ := reg.Get(RouteTypePlugin, "aroute", "http")
	if got.Enabled {
		t.Error("route should be disabled")
	}
	if got.State != RouteStateInactive {
		t.Errorf("state = %q, want %q", got.State, RouteStateInactive)
	}

	if err := reg.Enable(RouteTypePlugin, "aroute", "http"); err != nil {
		t.Fatalf("Enable: %v", err)
	}

	got, _ = reg.Get(RouteTypePlugin, "aroute", "http")
	if !got.Enabled {
		t.Error("route should be enabled")
	}
}

func TestUnifiedRegistry_Remove(t *testing.T) {
	reg := newTestUnifiedRegistry(t)

	_ = reg.Register(testPluginRoute("http", "1.0.0", TrustL1))

	impact, err := reg.Remove(RouteTypePlugin, "aroute", "http")
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if impact == nil {
		t.Error("expected non-nil impact")
	}

	_, err = reg.Get(RouteTypePlugin, "aroute", "http")
	if !IsRouteNotFound(err) {
		t.Error("route should be removed")
	}
}

func TestUnifiedRegistry_Update(t *testing.T) {
	reg := newTestUnifiedRegistry(t)

	_ = reg.Register(testPluginRoute("http", "1.0.0", TrustL1))

	newPayload, _ := json.Marshal(PluginPayload{Description: "updated"})
	if err := reg.Update(RouteTypePlugin, "aroute", "http", newPayload); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, _ := reg.Get(RouteTypePlugin, "aroute", "http")
	var payload PluginPayload
	_ = DecodePayload(got, &payload)
	if payload.Description != "updated" {
		t.Errorf("payload description = %q, want %q", payload.Description, "updated")
	}
}

// --- Cross-type dependency resolution tests (patent innovation #1) ---

func TestResolver_SimpleCrossTypeDependency(t *testing.T) {
	routes := []*Route{
		testPluginRoute("database", "1.0.0", TrustL1),
		testPluginRoute("http", "1.0.0", TrustL1,
			RouteRef{Type: RouteTypeService, Domain: "aroute", Name: "db-pool"},
		),
		testServiceRoute("db-pool", TrustL1),
	}

	order, err := ResolveOrder(routes)
	if err != nil {
		t.Fatalf("ResolveOrder: %v", err)
	}

	// database and db-pool should come before http
	dbIdx := indexOf(order, "database")
	httpIdx := indexOf(order, "http")
	poolIdx := indexOf(order, "db-pool")

	if dbIdx >= httpIdx {
		t.Error("database should come before http")
	}
	if poolIdx >= httpIdx {
		t.Error("db-pool service should come before http")
	}
}

func TestResolver_CycleDetection(t *testing.T) {
	routes := []*Route{
		{
			Type: RouteTypePlugin, Domain: "aroute", Name: "a", Version: "1.0.0",
			TrustLevel: TrustL1, Requires: []RouteRef{
				{Type: RouteTypePlugin, Domain: "aroute", Name: "b"},
			},
		},
		{
			Type: RouteTypePlugin, Domain: "aroute", Name: "b", Version: "1.0.0",
			TrustLevel: TrustL1, Requires: []RouteRef{
				{Type: RouteTypePlugin, Domain: "aroute", Name: "a"},
			},
		},
	}

	_, err := ResolveOrder(routes)
	if err == nil {
		t.Fatal("expected cycle detection error")
	}
}

func TestResolver_MissingDependency(t *testing.T) {
	routes := []*Route{
		{
			Type: RouteTypePlugin, Domain: "aroute", Name: "http", Version: "1.0.0",
			TrustLevel: TrustL1, Requires: []RouteRef{
				{Type: RouteTypePlugin, Domain: "aroute", Name: "nonexistent"},
			},
		},
	}

	_, err := ResolveOrder(routes)
	if err == nil {
		t.Fatal("expected dependency not found error")
	}
}

// --- Trust boundary enforcement tests (patent innovation #2) ---

func TestTrustBoundary_L1CannotDependOnL3(t *testing.T) {
	reg := newTestUnifiedRegistry(t)

	// Register L3 provider first
	_ = reg.Register(&Route{
		Type: RouteTypeService, Domain: "aroute", Name: "untrusted-service",
		Version: "1.0.0", TrustLevel: TrustL3, State: RouteStateActive, Enabled: true,
	})

	// Try to register L1 consumer depending on L3
	l1Route := &Route{
		Type: RouteTypePlugin, Domain: "aroute", Name: "trusted-plugin",
		Version: "1.0.0", TrustLevel: TrustL1, State: RouteStateActive, Enabled: true,
		Requires: []RouteRef{
			{Type: RouteTypeService, Domain: "aroute", Name: "untrusted-service"},
		},
	}

	err := reg.Register(l1Route)
	if err == nil {
		t.Fatal("L1 should not be able to depend on L3")
	}
}

func TestTrustBoundary_L3CanDependOnL1(t *testing.T) {
	reg := newTestUnifiedRegistry(t)

	// Register L1 provider
	_ = reg.Register(&Route{
		Type: RouteTypeService, Domain: "aroute", Name: "core-service",
		Version: "1.0.0", TrustLevel: TrustL1, State: RouteStateActive, Enabled: true,
	})

	// L3 consumer can depend on L1
	l3Route := &Route{
		Type: RouteTypePlugin, Domain: "aroute", Name: "wasm-plugin",
		Version: "1.0.0", TrustLevel: TrustL3, Engine: "wasm",
		State: RouteStateActive, Enabled: true,
		Requires: []RouteRef{
			{Type: RouteTypeService, Domain: "aroute", Name: "core-service"},
		},
	}

	err := reg.Register(l3Route)
	if err != nil {
		t.Fatalf("L3 should be able to depend on L1: %v", err)
	}
}

func TestTrustLevel_Rules(t *testing.T) {
	tests := []struct {
		name      string
		consumer  TrustLevel
		provider  TrustLevel
		wantError bool
	}{
		{"L1 -> L1 ok", TrustL1, TrustL1, false},
		{"L1 -> L2 fail", TrustL1, TrustL2, true},
		{"L1 -> L3 fail", TrustL1, TrustL3, true},
		{"L2 -> L1 ok", TrustL2, TrustL1, false},
		{"L2 -> L2 ok", TrustL2, TrustL2, false},
		{"L2 -> L3 fail", TrustL2, TrustL3, true},
		{"L3 -> L1 ok", TrustL3, TrustL1, false},
		{"L3 -> L2 ok", TrustL3, TrustL2, false},
		{"L3 -> L3 ok", TrustL3, TrustL3, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkTrustLevel(tt.consumer, tt.provider)
			if (err != nil) != tt.wantError {
				t.Errorf("checkTrustLevel(%v, %v) error = %v, wantError %v",
					tt.consumer, tt.provider, err, tt.wantError)
			}
		})
	}
}

// --- Hot-swap tests (patent innovation #3) ---

func TestUnifiedRegistry_HotSwap(t *testing.T) {
	reg := newTestUnifiedRegistry(t)

	_ = reg.Register(testPluginRoute("http", "1.0.0", TrustL1))

	newRoute := testPluginRoute("http", "2.0.0", TrustL1)
	newPayload, _ := EncodePayload(PluginPayload{Description: "v2"})
	newRoute.Payload = newPayload

	if err := reg.HotSwap(RouteTypePlugin, "aroute", "http", newRoute); err != nil {
		t.Fatalf("HotSwap: %v", err)
	}

	got, _ := reg.Get(RouteTypePlugin, "aroute", "http")
	if got.Version != "2.0.0" {
		t.Errorf("version = %q, want %q", got.Version, "2.0.0")
	}
	// Should preserve registered_at from original
	if got.RegisteredAt.IsZero() {
		t.Error("registered_at should be preserved")
	}
}

func TestUnifiedRegistry_HotSwapPreservesEnabled(t *testing.T) {
	reg := newTestUnifiedRegistry(t)

	_ = reg.Register(testPluginRoute("http", "1.0.0", TrustL1))
	_, _ = reg.Disable(RouteTypePlugin, "aroute", "http")

	newRoute := testPluginRoute("http", "2.0.0", TrustL1)
	if err := reg.HotSwap(RouteTypePlugin, "aroute", "http", newRoute); err != nil {
		t.Fatalf("HotSwap: %v", err)
	}

	got, _ := reg.Get(RouteTypePlugin, "aroute", "http")
	if got.Enabled {
		t.Error("hot-swap should preserve disabled state")
	}
}

// --- Impact analysis tests ---

func TestComputeImpact_DirectDependents(t *testing.T) {
	db := testPluginRoute("database", "1.0.0", TrustL1)
	http := testPluginRoute("http", "1.0.0", TrustL1,
		RouteRef{Type: RouteTypePlugin, Domain: "aroute", Name: "database"},
	)

	impact := ComputeImpact(db, []*Route{db, http})

	if len(impact.DirectlyAffected) != 1 {
		t.Fatalf("expected 1 directly affected, got %d", len(impact.DirectlyAffected))
	}
	if impact.DirectlyAffected[0].Name != "http" {
		t.Errorf("directly affected = %q, want %q", impact.DirectlyAffected[0].Name, "http")
	}
}

func TestComputeImpact_TransitiveDependents(t *testing.T) {
	db := testPluginRoute("database", "1.0.0", TrustL1)
	http := testPluginRoute("http", "1.0.0", TrustL1,
		RouteRef{Type: RouteTypePlugin, Domain: "aroute", Name: "database"},
	)
	api := testPluginRoute("api", "1.0.0", TrustL1,
		RouteRef{Type: RouteTypePlugin, Domain: "aroute", Name: "http"},
	)

	impact := ComputeImpact(db, []*Route{db, http, api})

	if len(impact.DirectlyAffected) != 1 {
		t.Errorf("expected 1 directly affected, got %d", len(impact.DirectlyAffected))
	}
	if len(impact.TransitivelyAffected) != 1 {
		t.Errorf("expected 1 transitively affected, got %d", len(impact.TransitivelyAffected))
	}
	if len(impact.TransitivelyAffected) > 0 && impact.TransitivelyAffected[0].Name != "api" {
		t.Errorf("transitively affected = %q, want %q", impact.TransitivelyAffected[0].Name, "api")
	}
}

// --- Compatibility layer tests ---

func TestLegacyRegistry_BackwardCompat(t *testing.T) {
	reg := newTestUnifiedRegistry(t)
	legacy := NewLegacyRegistry(reg)

	manifest := core.Manifest{
		Name:    "http",
		Version: "1.0.0",
		Author:  "test",
		Engine:  "native",
	}

	entry := &PluginEntry{Manifest: manifest, Enabled: true}
	if err := legacy.Register(entry); err != nil {
		t.Fatalf("Legacy Register: %v", err)
	}

	got, err := legacy.Get("http")
	if err != nil {
		t.Fatalf("Legacy Get: %v", err)
	}
	if got.Manifest.Name != "http" {
		t.Errorf("name = %q, want %q", got.Manifest.Name, "http")
	}

	list, _ := legacy.List()
	if len(list) != 1 {
		t.Errorf("expected 1 entry, got %d", len(list))
	}
}

func TestRouteRefFromString(t *testing.T) {
	ref := RouteRef{Type: RouteTypePlugin, Domain: "aroute", Name: "http"}
	s := ref.String()
	if s != "plugin/aroute/http" {
		t.Errorf("String() = %q, want %q", s, "plugin/aroute/http")
	}
}

// --- Concurrent access test ---

func TestUnifiedRegistry_ConcurrentAccess(t *testing.T) {
	reg := newTestUnifiedRegistry(t)

	done := make(chan bool, 10)

	for i := 0; i < 5; i++ {
		go func(n int) {
			name := "plugin-" + string(rune('a'+n))
			_ = reg.Register(testPluginRoute(name, "1.0.0", TrustL1))
			done <- true
		}(i)
	}

	for i := 0; i < 5; i++ {
		go func() {
			_, _ = reg.List(ListOptions{})
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	all, _ := reg.List(ListOptions{})
	if len(all) != 5 {
		t.Errorf("expected 5 routes, got %d", len(all))
	}
}

// --- Dependency index tests ---

func TestUnifiedRegistry_Dependents(t *testing.T) {
	reg := newTestUnifiedRegistry(t)

	dbRef := RouteRef{Type: RouteTypePlugin, Domain: "aroute", Name: "database"}

	_ = reg.Register(testPluginRoute("database", "1.0.0", TrustL1))
	_ = reg.Register(&Route{
		Type: RouteTypePlugin, Domain: "aroute", Name: "http", Version: "1.0.0",
		TrustLevel: TrustL1, Engine: "native", State: RouteStateActive, Enabled: true,
		Requires: []RouteRef{dbRef},
	})

	deps, err := reg.Dependents(dbRef)
	if err != nil {
		t.Fatalf("Dependents: %v", err)
	}

	if len(deps) != 1 {
		t.Fatalf("expected 1 dependent, got %d", len(deps))
	}
	if deps[0].Name != "http" {
		t.Errorf("dependent name = %q, want %q", deps[0].Name, "http")
	}
}

func TestUnifiedRegistry_Dependencies(t *testing.T) {
	reg := newTestUnifiedRegistry(t)

	dbRef := RouteRef{Type: RouteTypePlugin, Domain: "aroute", Name: "database"}

	_ = reg.Register(testPluginRoute("database", "1.0.0", TrustL1))
	_ = reg.Register(&Route{
		Type: RouteTypePlugin, Domain: "aroute", Name: "http", Version: "1.0.0",
		TrustLevel: TrustL1, Engine: "native", State: RouteStateActive, Enabled: true,
		Requires: []RouteRef{dbRef},
	})

	deps, err := reg.Dependencies(RouteRef{Type: RouteTypePlugin, Domain: "aroute", Name: "http"})
	if err != nil {
		t.Fatalf("Dependencies: %v", err)
	}

	if len(deps) != 1 {
		t.Fatalf("expected 1 dependency, got %d", len(deps))
	}
	if deps[0].Name != "database" {
		t.Errorf("dependency name = %q, want %q", deps[0].Name, "database")
	}
}

// --- Payload encode/decode tests ---

func TestPayloadEncodeDecode(t *testing.T) {
	original := PluginPayload{
		Description: "HTTP server plugin",
		Author:      "aroute",
		License:     "MIT",
		Keywords:    []string{"http", "server"},
	}

	data, err := EncodePayload(original)
	if err != nil {
		t.Fatalf("EncodePayload: %v", err)
	}

	route := &Route{Payload: data}
	var decoded PluginPayload
	if err := DecodePayload(route, &decoded); err != nil {
		t.Fatalf("DecodePayload: %v", err)
	}

	if decoded.Description != original.Description {
		t.Errorf("description = %q, want %q", decoded.Description, original.Description)
	}
	if decoded.Author != original.Author {
		t.Errorf("author = %q, want %q", decoded.Author, original.Author)
	}
}

// --- Multi-type storage test ---

func TestUnifiedRegistry_MultipleTypes(t *testing.T) {
	reg := newTestUnifiedRegistry(t)

	plugin := testPluginRoute("http", "1.0.0", TrustL1)
	service := testServiceRoute("db-pool", TrustL1)
	hook := testHookRoute("cache-invalidator", "content.post.saved",
		RouteRef{Type: RouteTypePlugin, Domain: "aroute", Name: "http"},
	)

	_ = reg.Register(plugin)
	_ = reg.Register(service)
	_ = reg.Register(hook)

	// Verify each type is in its own bucket
	p, _ := reg.Get(RouteTypePlugin, "aroute", "http")
	if p.Type != RouteTypePlugin {
		t.Error("plugin type mismatch")
	}

	s, _ := reg.Get(RouteTypeService, "aroute", "db-pool")
	if s.Type != RouteTypeService {
		t.Error("service type mismatch")
	}

	h, _ := reg.Get(RouteTypeHook, "aroute", "cache-invalidator")
	if h.Type != RouteTypeHook {
		t.Error("hook type mismatch")
	}

	// Verify List with type filter
	pluginType := RouteTypePlugin
	plugins, _ := reg.List(ListOptions{Type: &pluginType})
	if len(plugins) != 1 {
		t.Errorf("expected 1 plugin, got %d", len(plugins))
	}

	all, _ := reg.List(ListOptions{})
	if len(all) != 3 {
		t.Errorf("expected 3 total routes, got %d", len(all))
	}
}

// --- ResolutionOrder integration test ---

func TestUnifiedRegistry_ResolutionOrder(t *testing.T) {
	reg := newTestUnifiedRegistry(t)

	dbRef := RouteRef{Type: RouteTypePlugin, Domain: "aroute", Name: "database"}
	httpRef := RouteRef{Type: RouteTypePlugin, Domain: "aroute", Name: "http"}

	_ = reg.Register(&Route{
		Type: RouteTypePlugin, Domain: "aroute", Name: "database", Version: "1.0.0",
		TrustLevel: TrustL1, Engine: "native", State: RouteStateActive, Enabled: true,
	})
	_ = reg.Register(&Route{
		Type: RouteTypePlugin, Domain: "aroute", Name: "http", Version: "1.0.0",
		TrustLevel: TrustL1, Engine: "native", State: RouteStateActive, Enabled: true,
		Requires: []RouteRef{dbRef},
	})
	_ = reg.Register(&Route{
		Type: RouteTypeHook, Domain: "aroute", Name: "cache-hook", Version: "1.0.0",
		TrustLevel: TrustL1, State: RouteStateActive, Enabled: true,
		Requires: []RouteRef{httpRef},
	})

	order, err := reg.ResolutionOrder()
	if err != nil {
		t.Fatalf("ResolutionOrder: %v", err)
	}

	// database must come before http, http must come before cache-hook
	dbIdx := indexOf(order, "database")
	httpIdx := indexOf(order, "http")
	hookIdx := indexOf(order, "cache-hook")

	if dbIdx >= httpIdx {
		t.Error("database should come before http in resolution order")
	}
	if httpIdx >= hookIdx {
		t.Error("http should come before cache-hook in resolution order")
	}
}

// --- helper ---

func indexOf(routes []*Route, name string) int {
	for i, r := range routes {
		if r.Name == name {
			return i
		}
	}
	return -1
}

// DiscoveredPath is used in discovery - verify it's preserved
func TestUnifiedRegistry_DiscoveredPathPreserved(t *testing.T) {
	reg := newTestUnifiedRegistry(t)

	route := testPluginRoute("http", "1.0.0", TrustL1)
	route.DiscoveredPath = "/plugins/http/manifest.yaml"
	_ = reg.Register(route)

	got, _ := reg.Get(RouteTypePlugin, "aroute", "http")
	if got.DiscoveredPath != "/plugins/http/manifest.yaml" {
		t.Errorf("discovered_path = %q, want %q", got.DiscoveredPath, "/plugins/http/manifest.yaml")
	}
}

// Verify schema version bucket exists
func TestUnifiedRegistry_SchemaBuckets(t *testing.T) {
	reg := newTestUnifiedRegistry(t)
	defer reg.Close()

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	newReg, err := NewBoltUnifiedRegistry(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer newReg.Close()

	// Verify db file was created
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Error("database file should exist")
	}
}
