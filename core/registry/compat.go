package registry

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/wangling-miao/aroute/core"
)

const defaultDomain = "aroute"

// PluginEntryFromRoute creates a PluginEntry from a Route.
// Used for backward compatibility with existing callers.
func PluginEntryFromRoute(route *Route) *PluginEntry {
	var payload PluginPayload
	if len(route.Payload) > 0 {
		_ = json.Unmarshal(route.Payload, &payload)
	}

	manifest := core.Manifest{
		Name:         route.Name,
		Version:      route.Version,
		Description:  payload.Description,
		Author:       payload.Author,
		License:      payload.License,
		Engine:       route.Engine,
		Trust:        route.TrustLevel.String(),
		Capabilities: append([]string(nil), route.Capabilities...),
		Publisher:    firstNonEmpty(route.Publisher, payload.Publisher),
		Digest:       firstNonEmpty(route.Digest, payload.Digest),
		Signature:    firstNonEmpty(route.Signature, payload.Signature),
		Resources:    route.Resources,
		Runtime:      route.Runtime,
		Homepage:     payload.Homepage,
		Repository:   payload.Repository,
		Keywords:     payload.Keywords,
	}

	for _, req := range route.Requires {
		manifest.Requires = append(manifest.Requires, req.Name)
	}
	for _, prov := range route.Provides {
		manifest.Provides = append(manifest.Provides, prov.Name)
	}

	return &PluginEntry{
		Manifest:         manifest,
		Enabled:          route.Enabled,
		DiscoveredPath:   route.DiscoveredPath,
		TrustLevel:       route.TrustLevel.String(),
		EffectiveTrust:   route.EffectiveTrustLevel.String(),
		RiskScore:        route.RiskScore,
		TrustState:       string(route.TrustState),
		Capabilities:     append([]string(nil), route.Capabilities...),
		CapabilityGrants: append([]string(nil), route.CapabilityGrants...),
		LastDecision:     route.LastDecision,
		PolicyRevision:   route.PolicyRevision,
	}
}

// RouteFromPluginEntry creates a Route from a PluginEntry.
func RouteFromPluginEntry(entry *PluginEntry) *Route {
	payload, _ := EncodePayload(PluginPayload{
		Description: entry.Manifest.Description,
		Author:      entry.Manifest.Author,
		License:     entry.Manifest.License,
		Homepage:    entry.Manifest.Homepage,
		Repository:  entry.Manifest.Repository,
		Keywords:    entry.Manifest.Keywords,
		Publisher:   entry.Manifest.Publisher,
		Digest:      entry.Manifest.Digest,
		Signature:   entry.Manifest.Signature,
	})

	var requires []RouteRef
	for _, req := range entry.Manifest.Requires {
		requires = append(requires, RouteRef{
			Type:   RouteTypePlugin,
			Domain: defaultDomain,
			Name:   req,
		})
	}

	var provides []RouteRef
	for _, prov := range entry.Manifest.Provides {
		provides = append(provides, RouteRef{
			Type:   RouteTypeService,
			Domain: defaultDomain,
			Name:   prov,
		})
	}

	trustLevel := trustLevelFromManifest(entry.Manifest)

	state := RouteStateInactive
	if entry.Enabled {
		state = RouteStateActive
	}

	return &Route{
		Type:                RouteTypePlugin,
		Domain:              defaultDomain,
		Name:                entry.Manifest.Name,
		Version:             entry.Manifest.Version,
		TrustLevel:          trustLevel,
		DeclaredTrustLevel:  trustLevel,
		EffectiveTrustLevel: trustLevel,
		Engine:              canonicalEngine(entry.Manifest.Engine),
		State:               state,
		Enabled:             entry.Enabled,
		Requires:            requires,
		Provides:            provides,
		Payload:             payload,
		Capabilities:        append([]string(nil), firstStringSlice(entry.Capabilities, entry.Manifest.Capabilities)...),
		CapabilityGrants:    append([]string(nil), firstStringSlice(entry.CapabilityGrants, entry.Manifest.Capabilities)...),
		RiskScore:           entry.RiskScore,
		TrustState:          trustStateOrDefault(entry.TrustState),
		LastDecision:        entry.LastDecision,
		PolicyRevision:      firstNonEmpty(entry.PolicyRevision, "builtin:v1"),
		Publisher:           entry.Manifest.Publisher,
		Digest:              entry.Manifest.Digest,
		Signature:           entry.Manifest.Signature,
		Resources:           entry.Manifest.Resources,
		Runtime:             entry.Manifest.Runtime,
		DiscoveredPath:      entry.DiscoveredPath,
		RegisteredAt:        time.Now(),
		UpdatedAt:           time.Now(),
	}
}

func firstStringSlice(values ...[]string) []string {
	for _, value := range values {
		if len(value) > 0 {
			return value
		}
	}
	return nil
}

func trustStateOrDefault(state string) TrustState {
	switch TrustState(state) {
	case TrustStateGuarded, TrustStatePendingReview, TrustStateQuarantined, TrustStateDisabled:
		return TrustState(state)
	default:
		return TrustStateAllow
	}
}

func canonicalEngine(engine string) string {
	switch engine {
	case "l1":
		return "native"
	case "l2":
		return "grpc"
	case "l3":
		return "wasm"
	default:
		return engine
	}
}

func trustLevelFromManifest(manifest core.Manifest) TrustLevel {
	switch manifest.Trust {
	case "L1", "l1":
		return TrustL1
	case "L2", "l2":
		return TrustL2
	case "L3", "l3":
		return TrustL3
	}
	switch manifest.Engine {
	case "wasm", "l3":
		return TrustL3
	case "grpc", "l2":
		return TrustL2
	default:
		return TrustL1
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

// LegacyRegistry wraps a UnifiedRegistry to satisfy the old Registry interface.
// This ensures zero breaking changes for existing callers.
type LegacyRegistry struct {
	inner UnifiedRegistry
}

// NewLegacyRegistry creates a Registry-compat wrapper around a UnifiedRegistry.
func NewLegacyRegistry(inner UnifiedRegistry) *LegacyRegistry {
	return &LegacyRegistry{inner: inner}
}

func (l *LegacyRegistry) Register(entry *PluginEntry) error {
	route := RouteFromPluginEntry(entry)
	return l.inner.Register(route)
}

func (l *LegacyRegistry) Get(name string) (*PluginEntry, error) {
	route, err := l.inner.Get(RouteTypePlugin, defaultDomain, name)
	if err != nil {
		return nil, err
	}
	return PluginEntryFromRoute(route), nil
}

func (l *LegacyRegistry) List() ([]*PluginEntry, error) {
	pluginType := RouteTypePlugin
	routes, err := l.inner.List(ListOptions{Type: &pluginType})
	if err != nil {
		return nil, err
	}

	entries := make([]*PluginEntry, len(routes))
	for i, route := range routes {
		entries[i] = PluginEntryFromRoute(route)
	}
	return entries, nil
}

func (l *LegacyRegistry) Update(name string, manifest core.Manifest) error {
	route, err := l.inner.Get(RouteTypePlugin, defaultDomain, name)
	if err != nil {
		return err
	}

	// Rebuild the route from the new manifest, preserving state
	newEntry := &PluginEntry{Manifest: manifest, Enabled: route.Enabled}
	newRoute := RouteFromPluginEntry(newEntry)
	newRoute.RegisteredAt = route.RegisteredAt

	return l.inner.HotSwap(RouteTypePlugin, defaultDomain, name, newRoute)
}

func (l *LegacyRegistry) Remove(name string) error {
	_, err := l.inner.Remove(RouteTypePlugin, defaultDomain, name)
	return err
}

func (l *LegacyRegistry) Enable(name string) error {
	return l.inner.Enable(RouteTypePlugin, defaultDomain, name)
}

func (l *LegacyRegistry) Disable(name string) error {
	_, err := l.inner.Disable(RouteTypePlugin, defaultDomain, name)
	return err
}

func (l *LegacyRegistry) Close() error {
	return l.inner.Close()
}

// LegacyDiscoveryRegistry wraps a UnifiedRegistry to satisfy the
// lifecycle manager's PluginRegistry interface.
type LegacyDiscoveryRegistry struct {
	inner UnifiedRegistry
}

// NewLegacyDiscoveryRegistry creates a lifecycle-compatible wrapper.
func NewLegacyDiscoveryRegistry(inner UnifiedRegistry) *LegacyDiscoveryRegistry {
	return &LegacyDiscoveryRegistry{inner: inner}
}

func (l *LegacyDiscoveryRegistry) List() ([]core.Manifest, error) {
	pluginType := RouteTypePlugin
	routes, err := l.inner.List(ListOptions{Type: &pluginType})
	if err != nil {
		return nil, err
	}

	manifests := make([]core.Manifest, len(routes))
	for i, route := range routes {
		entry := PluginEntryFromRoute(route)
		manifests[i] = entry.Manifest
	}
	return manifests, nil
}

func (l *LegacyDiscoveryRegistry) Get(name string) (core.Manifest, error) {
	route, err := l.inner.Get(RouteTypePlugin, defaultDomain, name)
	if err != nil {
		return core.Manifest{}, fmt.Errorf("plugin not found: %s", name)
	}
	entry := PluginEntryFromRoute(route)
	return entry.Manifest, nil
}

func (l *LegacyDiscoveryRegistry) IsEnabled(name string) (bool, error) {
	route, err := l.inner.Get(RouteTypePlugin, defaultDomain, name)
	if err != nil {
		return false, err
	}
	return route.Enabled, nil
}

func (l *LegacyDiscoveryRegistry) Enable(name string) error {
	return l.inner.Enable(RouteTypePlugin, defaultDomain, name)
}

func (l *LegacyDiscoveryRegistry) Disable(name string) error {
	_, err := l.inner.Disable(RouteTypePlugin, defaultDomain, name)
	return err
}
