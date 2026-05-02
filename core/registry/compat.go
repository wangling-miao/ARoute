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
		Name:        route.Name,
		Version:     route.Version,
		Description: payload.Description,
		Author:      payload.Author,
		License:     payload.License,
		Engine:      route.Engine,
		Homepage:    payload.Homepage,
		Repository:  payload.Repository,
		Keywords:    payload.Keywords,
	}

	for _, req := range route.Requires {
		manifest.Requires = append(manifest.Requires, req.Name)
	}
	for _, prov := range route.Provides {
		manifest.Provides = append(manifest.Provides, prov.Name)
	}

	return &PluginEntry{
		Manifest:       manifest,
		Enabled:        route.Enabled,
		DiscoveredPath: route.DiscoveredPath,
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

	trustLevel := TrustL1
	if entry.Manifest.Engine == "wasm" || entry.Manifest.Engine == "l3" {
		trustLevel = TrustL3
	} else if entry.Manifest.Engine == "grpc" || entry.Manifest.Engine == "l2" {
		trustLevel = TrustL2
	}

	state := RouteStateInactive
	if entry.Enabled {
		state = RouteStateActive
	}

	return &Route{
		Type:          RouteTypePlugin,
		Domain:        defaultDomain,
		Name:          entry.Manifest.Name,
		Version:       entry.Manifest.Version,
		TrustLevel:    trustLevel,
		Engine:        entry.Manifest.Engine,
		State:         state,
		Enabled:       entry.Enabled,
		Requires:      requires,
		Provides:      provides,
		Payload:       payload,
		DiscoveredPath: entry.DiscoveredPath,
		RegisteredAt:  time.Now(),
		UpdatedAt:     time.Now(),
	}
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
