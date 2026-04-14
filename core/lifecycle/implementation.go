package lifecycle

import (
	"context"
	"fmt"
	"strings"

	"github.com/wangling-miao/aroute/core"
	"github.com/wangling-miao/aroute/core/events"
)

type StateChangeEventData struct {
	PluginName string
	OldState   core.PluginState
	NewState   core.PluginState
}

func (m *ManagerImpl) setState(info *PluginLoadInfo, pluginName string, newState core.PluginState) {
	oldState := info.State
	info.State = newState

	if m.eventBus != nil {
		eventData := StateChangeEventData{
			PluginName: pluginName,
			OldState:   oldState,
			NewState:   newState,
		}
		m.eventBus.Emit(context.Background(), events.Event{
			Topic: "lifecycle.plugin." + pluginName + ".stateChanged",
			Data:  map[string]interface{}{"data": eventData},
		})
	}
}

func (m *ManagerImpl) LoadAll(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	manifests, err := m.registry.List()
	if err != nil {
		return fmt.Errorf("list plugins from registry: %w", err)
	}

	for _, manifest := range manifests {
		enabled, err := m.registry.IsEnabled(manifest.Name)
		if err != nil {
			return fmt.Errorf("check enabled status for %s: %w", manifest.Name, err)
		}

		if !enabled {
			continue
		}

		if _, exists := m.plugins[manifest.Name]; exists {
			continue
		}

		plugin, err := m.pluginLoader.Load(manifest)
		if err != nil {
			m.plugins[manifest.Name] = &PluginLoadInfo{
				Manifest:  &manifest,
				State:     core.StateFailed,
				LoadError: err,
			}
			continue
		}

		m.plugins[manifest.Name] = &PluginLoadInfo{
			Plugin:   plugin,
			Manifest: &manifest,
			State:    core.StateRegistered,
		}
	}

	return nil
}

func (m *ManagerImpl) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	order, err := m.buildStartupOrder()
	if err != nil {
		return err
	}

	m.order = order

	var failedPlugins []string

	for _, pluginName := range order {
		info, exists := m.plugins[pluginName]
		if !exists {
			continue
		}

		if !m.canTransition(info, core.StateResolved) {
			if info.State == core.StateFailed {
				failedPlugins = append(failedPlugins, pluginName)
			}
			continue
		}

		m.setState(info, pluginName, core.StateResolved)

		if hasFailedDependencies(info, failedPlugins) {
			m.setState(info, pluginName, core.StateFailed)
			info.LoadError = fmt.Errorf("dependency failed")
			failedPlugins = append(failedPlugins, pluginName)
			continue
		}

		if !m.canTransition(info, core.StateStarting) {
			m.setState(info, pluginName, core.StateFailed)
			info.LoadError = fmt.Errorf("invalid state transition to starting")
			failedPlugins = append(failedPlugins, pluginName)
			continue
		}

		m.setState(info, pluginName, core.StateStarting)

		if err := m.startPlugin(ctx, pluginName, info); err != nil {
			m.setState(info, pluginName, core.StateFailed)
			info.LoadError = err
			failedPlugins = append(failedPlugins, pluginName)
			continue
		}

		m.setState(info, pluginName, core.StateActive)
	}

	if len(failedPlugins) > 0 {
		return fmt.Errorf("plugins failed to start: %s", strings.Join(failedPlugins, ", "))
	}

	return nil
}

func (m *ManagerImpl) Stop(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.order) == 0 {
		return nil
	}

	var errors []string

	reverseOrder := make([]string, len(m.order))
	for i, name := range m.order {
		reverseOrder[len(m.order)-1-i] = name
	}

	for _, pluginName := range reverseOrder {
		info, exists := m.plugins[pluginName]
		if !exists {
			continue
		}

		if info.State != core.StateActive && info.State != core.StateFailed {
			continue
		}

		if !m.canTransition(info, core.StateStopping) {
			continue
		}

		m.setState(info, pluginName, core.StateStopping)

		if err := info.Plugin.Stop(); err != nil {
			errors = append(errors, fmt.Sprintf("%s: %v", pluginName, err))
			m.setState(info, pluginName, core.StateFailed)
			info.LoadError = err
			continue
		}

		m.setState(info, pluginName, core.StateStopped)
	}

	if len(errors) > 0 {
		return fmt.Errorf("errors during shutdown: %s", strings.Join(errors, "; "))
	}

	return nil
}

func (m *ManagerImpl) Enable(ctx context.Context, pluginName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	info, exists := m.plugins[pluginName]
	if !exists {
		return ErrPluginNotFound
	}

	if info.State != core.StateStopped && info.State != core.StateFailed {
		return &StateError{
			PluginName:   pluginName,
			CurrentState: info.State,
			TargetState:  core.StateStarting,
			Message:      "plugin must be stopped or failed to enable",
		}
	}

	for _, dep := range info.Manifest.Requires {
		depName, _, err := core.ParseDependency(dep)
		if err != nil {
			continue
		}

		depInfo, exists := m.plugins[depName]
		if !exists || depInfo.State != core.StateActive {
			return &DependencyError{
				PluginName: pluginName,
				Dependency: depName,
				Message:    "dependency not active",
			}
		}
	}

	if info.State == core.StateStopped {
		if !m.canTransition(info, core.StateResolved) {
			return &StateError{
				PluginName:   pluginName,
				CurrentState: info.State,
				TargetState:  core.StateResolved,
				Message:      "cannot transition to resolved",
			}
		}
		m.setState(info, pluginName, core.StateResolved)
	}

	if !m.canTransition(info, core.StateStarting) {
		return &StateError{
			PluginName:   pluginName,
			CurrentState: info.State,
			TargetState:  core.StateStarting,
			Message:      "cannot transition to starting",
		}
	}
	m.setState(info, pluginName, core.StateStarting)

	if err := m.startPlugin(ctx, pluginName, info); err != nil {
		m.setState(info, pluginName, core.StateFailed)
		info.LoadError = err
		return fmt.Errorf("start plugin: %w", err)
	}

	m.setState(info, pluginName, core.StateActive)
	return nil
}

func (m *ManagerImpl) Disable(ctx context.Context, pluginName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	info, exists := m.plugins[pluginName]
	if !exists {
		return ErrPluginNotFound
	}

	if info.State != core.StateActive {
		return &StateError{
			PluginName:   pluginName,
			CurrentState: info.State,
			TargetState:  core.StateStopping,
			Message:      "plugin must be active to disable",
		}
	}

	for _, otherName := range m.order {
		otherInfo, exists := m.plugins[otherName]
		if !exists || otherName == pluginName {
			continue
		}

		if otherInfo.State != core.StateActive {
			continue
		}

		for _, dep := range otherInfo.Manifest.Requires {
			depName, _, _ := core.ParseDependency(dep)
			if depName == pluginName {
				return &DependencyError{
					PluginName: pluginName,
					Dependency: otherName,
					Message:    "active dependent exists",
				}
			}
		}
	}

	m.setState(info, pluginName, core.StateStopping)

	if err := info.Plugin.Stop(); err != nil {
		m.setState(info, pluginName, core.StateFailed)
		info.LoadError = err
		return fmt.Errorf("stop plugin: %w", err)
	}

	m.setState(info, pluginName, core.StateStopped)

	if m.container != nil && info.Manifest != nil {
		for _, svc := range info.Manifest.Provides {
			_ = m.container.Unregister(svc)
		}
	}

	return nil
}

func (m *ManagerImpl) Retry(ctx context.Context, pluginName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	info, exists := m.plugins[pluginName]
	if !exists {
		return ErrPluginNotFound
	}

	if info.State != core.StateFailed {
		return &StateError{
			PluginName:   pluginName,
			CurrentState: info.State,
			TargetState:  core.StateResolved,
			Message:      "retry is only valid for Failed state",
		}
	}

	for _, dep := range info.Manifest.Requires {
		depName, _, err := core.ParseDependency(dep)
		if err != nil {
			continue
		}

		depInfo, exists := m.plugins[depName]
		if !exists || depInfo.State != core.StateActive {
			return &DependencyError{
				PluginName: pluginName,
				Dependency: depName,
				Message:    "dependency not active",
			}
		}
	}

	m.setState(info, pluginName, core.StateResolved)
	m.setState(info, pluginName, core.StateStarting)

	if err := m.startPlugin(ctx, pluginName, info); err != nil {
		m.setState(info, pluginName, core.StateFailed)
		info.LoadError = err
		return fmt.Errorf("start plugin: %w", err)
	}

	m.setState(info, pluginName, core.StateActive)
	return nil
}

func (m *ManagerImpl) GetState(pluginName string) (core.PluginState, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	info, exists := m.plugins[pluginName]
	if !exists {
		return core.StateFailed, ErrPluginNotFound
	}

	return info.State, nil
}

func (m *ManagerImpl) GetPlugin(pluginName string) core.Plugin {
	m.mu.RLock()
	defer m.mu.RUnlock()

	info, exists := m.plugins[pluginName]
	if !exists {
		return nil
	}

	return info.Plugin
}

func (m *ManagerImpl) ListPlugins() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	names := make([]string, 0, len(m.plugins))
	for name := range m.plugins {
		names = append(names, name)
	}
	return names
}

func (m *ManagerImpl) canTransition(info *PluginLoadInfo, target core.PluginState) bool {
	return canTransition(info.State, target)
}

func (m *ManagerImpl) startPlugin(ctx context.Context, pluginName string, info *PluginLoadInfo) error {
	var coreCtx core.CoreContext
	if m.ctxFactory != nil {
		coreCtx = m.ctxFactory(ctx, pluginName)
	}

	if err := info.Plugin.Init(coreCtx); err != nil {
		return fmt.Errorf("init: %w", err)
	}

	if err := info.Plugin.Start(); err != nil {
		return fmt.Errorf("start: %w", err)
	}

	return nil
}

func hasFailedDependencies(info *PluginLoadInfo, failedPlugins []string) bool {
	for _, dep := range info.Manifest.Requires {
		depName, _, _ := core.ParseDependency(dep)
		for _, failed := range failedPlugins {
			if failed == depName {
				return true
			}
		}
	}
	return false
}

func (m *ManagerImpl) buildStartupOrder() ([]string, error) {
	graph := make(map[string][]string)
	inDegree := make(map[string]int)

	for name, info := range m.plugins {
		if _, exists := inDegree[name]; !exists {
			inDegree[name] = 0
		}

		for _, dep := range info.Manifest.Requires {
			depName, _, err := core.ParseDependency(dep)
			if err != nil {
				continue
			}

			if _, exists := m.plugins[depName]; !exists {
				return nil, &DependencyError{
					PluginName: name,
					Dependency: depName,
					Message:    "dependency not found",
				}
			}

			graph[depName] = append(graph[depName], name)
			inDegree[name]++
		}

		for _, dep := range info.Manifest.After {
			depName, _, err := core.ParseDependency(dep)
			if err != nil {
				continue
			}

			if _, exists := m.plugins[depName]; !exists {
				continue
			}

			graph[depName] = append(graph[depName], name)
			inDegree[name]++
		}
	}

	var queue []string
	for name, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, name)
		}
	}

	var order []string
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		order = append(order, current)

		for _, neighbor := range graph[current] {
			inDegree[neighbor]--
			if inDegree[neighbor] == 0 {
				queue = append(queue, neighbor)
			}
		}
	}

	if len(order) != len(m.plugins) {
		var cycle []string
		for name, degree := range inDegree {
			if degree > 0 {
				cycle = append(cycle, name)
			}
		}
		return nil, &DependencyError{
			CyclePath: cycle,
		}
	}

	return order, nil
}
