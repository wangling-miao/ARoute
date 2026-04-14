package lifecycle

import (
	"context"
	"errors"
	"testing"

	"github.com/wangling-miao/aroute/core"
)

// TestCycleDetection tests various circular dependency patterns.
func TestCycleDetection(t *testing.T) {
	t.Run("simple two-node cycle", func(t *testing.T) {
		registry := newMockRegistry()
		loader := newMockLoader()

		pluginA := newMockPlugin("plugin-a", "1.0.0")
		pluginB := newMockPlugin("plugin-b", "1.0.0")
		pluginA.manifest.Requires = []string{"plugin-b"}
		pluginB.manifest.Requires = []string{"plugin-a"}

		loader.factory["plugin-a"] = pluginA
		loader.factory["plugin-b"] = pluginB

		registry.plugins["plugin-a"] = *pluginA.manifest
		registry.plugins["plugin-b"] = *pluginB.manifest
		registry.enabled["plugin-a"] = true
		registry.enabled["plugin-b"] = true

		manager := newTestManager(registry, loader)
		err := manager.LoadAll(context.Background())
		if err != nil {
			t.Fatalf("LoadAll failed: %v", err)
		}

		err = manager.Start(context.Background())
		if err == nil {
			t.Error("expected cycle detection error")
		}

		var depErr *DependencyError
		if !errors.As(err, &depErr) {
			t.Errorf("expected DependencyError, got %T: %v", err, err)
		} else {
			if len(depErr.CyclePath) == 0 {
				t.Error("expected cycle path in error")
			}
			// Verify both plugins are in cycle path
			foundA, foundB := false, false
			for _, name := range depErr.CyclePath {
				if name == "plugin-a" {
					foundA = true
				}
				if name == "plugin-b" {
					foundB = true
				}
			}
			if !foundA || !foundB {
				t.Errorf("expected both plugin-a and plugin-b in cycle path, got %v", depErr.CyclePath)
			}
		}
	})

	t.Run("three-node cycle", func(t *testing.T) {
		registry := newMockRegistry()
		loader := newMockLoader()

		pluginA := newMockPlugin("plugin-a", "1.0.0")
		pluginB := newMockPlugin("plugin-b", "1.0.0")
		pluginC := newMockPlugin("plugin-c", "1.0.0")
		pluginA.manifest.Requires = []string{"plugin-b"}
		pluginB.manifest.Requires = []string{"plugin-c"}
		pluginC.manifest.Requires = []string{"plugin-a"}

		loader.factory["plugin-a"] = pluginA
		loader.factory["plugin-b"] = pluginB
		loader.factory["plugin-c"] = pluginC

		registry.plugins["plugin-a"] = *pluginA.manifest
		registry.plugins["plugin-b"] = *pluginB.manifest
		registry.plugins["plugin-c"] = *pluginC.manifest
		registry.enabled["plugin-a"] = true
		registry.enabled["plugin-b"] = true
		registry.enabled["plugin-c"] = true

		manager := newTestManager(registry, loader)
		err := manager.LoadAll(context.Background())
		if err != nil {
			t.Fatalf("LoadAll failed: %v", err)
		}

		err = manager.Start(context.Background())
		if err == nil {
			t.Error("expected cycle detection error")
		}

		var depErr *DependencyError
		if !errors.As(err, &depErr) {
			t.Errorf("expected DependencyError, got %T", err)
		} else {
			if len(depErr.CyclePath) != 3 {
				t.Errorf("expected 3 plugins in cycle path, got %d: %v", len(depErr.CyclePath), depErr.CyclePath)
			}
		}
	})

	t.Run("self-referential cycle", func(t *testing.T) {
		registry := newMockRegistry()
		loader := newMockLoader()

		pluginA := newMockPlugin("plugin-a", "1.0.0")
		pluginA.manifest.Requires = []string{"plugin-a"}

		loader.factory["plugin-a"] = pluginA
		registry.plugins["plugin-a"] = *pluginA.manifest
		registry.enabled["plugin-a"] = true

		manager := newTestManager(registry, loader)
		err := manager.LoadAll(context.Background())
		if err != nil {
			t.Fatalf("LoadAll failed: %v", err)
		}

		err = manager.Start(context.Background())
		if err == nil {
			t.Error("expected cycle detection error for self-reference")
		}

		if _, ok := errors.AsType[*DependencyError](err); !ok {
			t.Errorf("expected DependencyError, got %T", err)
		}
	})

	t.Run("complex cycle with non-cycling plugins", func(t *testing.T) {
		registry := newMockRegistry()
		loader := newMockLoader()

		// plugin-d depends on plugin-a, but is outside the cycle
		pluginA := newMockPlugin("plugin-a", "1.0.0")
		pluginB := newMockPlugin("plugin-b", "1.0.0")
		pluginC := newMockPlugin("plugin-c", "1.0.0")
		pluginD := newMockPlugin("plugin-d", "1.0.0")

		pluginA.manifest.Requires = []string{"plugin-b"}
		pluginB.manifest.Requires = []string{"plugin-c"}
		pluginC.manifest.Requires = []string{"plugin-a"}
		pluginD.manifest.Requires = []string{"plugin-a"}

		loader.factory["plugin-a"] = pluginA
		loader.factory["plugin-b"] = pluginB
		loader.factory["plugin-c"] = pluginC
		loader.factory["plugin-d"] = pluginD

		registry.plugins["plugin-a"] = *pluginA.manifest
		registry.plugins["plugin-b"] = *pluginB.manifest
		registry.plugins["plugin-c"] = *pluginC.manifest
		registry.plugins["plugin-d"] = *pluginD.manifest
		registry.enabled["plugin-a"] = true
		registry.enabled["plugin-b"] = true
		registry.enabled["plugin-c"] = true
		registry.enabled["plugin-d"] = true

		manager := newTestManager(registry, loader)
		err := manager.LoadAll(context.Background())
		if err != nil {
			t.Fatalf("LoadAll failed: %v", err)
		}

		err = manager.Start(context.Background())
		if err == nil {
			t.Error("expected cycle detection error")
		}

		if _, ok := errors.AsType[*DependencyError](err); !ok {
			t.Errorf("expected DependencyError, got %T", err)
		}
	})
}

// TestBuildStartupOrder tests the buildStartupOrder function with various dependency graphs.
func TestBuildStartupOrder(t *testing.T) {
	t.Run("missing dependency error", func(t *testing.T) {
		registry := newMockRegistry()
		loader := newMockLoader()

		pluginA := newMockPlugin("plugin-a", "1.0.0")
		pluginA.manifest.Requires = []string{"nonexistent-plugin"}

		loader.factory["plugin-a"] = pluginA
		registry.plugins["plugin-a"] = *pluginA.manifest
		registry.enabled["plugin-a"] = true

		manager := newTestManager(registry, loader)
		err := manager.LoadAll(context.Background())
		if err != nil {
			t.Fatalf("LoadAll failed: %v", err)
		}

		err = manager.Start(context.Background())
		if err == nil {
			t.Error("expected error for missing dependency")
		}

		var depErr *DependencyError
		if !errors.As(err, &depErr) {
			t.Errorf("expected DependencyError, got %T", err)
		} else {
			if depErr.PluginName != "plugin-a" {
				t.Errorf("expected PluginName plugin-a, got %s", depErr.PluginName)
			}
			if depErr.Dependency != "nonexistent-plugin" {
				t.Errorf("expected Dependency nonexistent-plugin, got %s", depErr.Dependency)
			}
			if depErr.Message != "dependency not found" {
				t.Errorf("expected message 'dependency not found', got %s", depErr.Message)
			}
		}
	})

	t.Run("After ordering respected", func(t *testing.T) {
		registry := newMockRegistry()
		loader := newMockLoader()

		pluginA := newMockPlugin("plugin-a", "1.0.0")
		pluginB := newMockPlugin("plugin-b", "1.0.0")
		pluginC := newMockPlugin("plugin-c", "1.0.0")

		// plugin-b and plugin-c should start after plugin-a (no hard dependency)
		pluginB.manifest.After = []string{"plugin-a"}
		pluginC.manifest.After = []string{"plugin-a", "plugin-b"}

		loader.factory["plugin-a"] = pluginA
		loader.factory["plugin-b"] = pluginB
		loader.factory["plugin-c"] = pluginC

		registry.plugins["plugin-a"] = *pluginA.manifest
		registry.plugins["plugin-b"] = *pluginB.manifest
		registry.plugins["plugin-c"] = *pluginC.manifest
		registry.enabled["plugin-a"] = true
		registry.enabled["plugin-b"] = true
		registry.enabled["plugin-c"] = true

		manager := newTestManager(registry, loader)
		err := manager.LoadAll(context.Background())
		if err != nil {
			t.Fatalf("LoadAll failed: %v", err)
		}

		err = manager.Start(context.Background())
		if err != nil {
			t.Fatalf("Start failed: %v", err)
		}

		order := manager.order
		aIndex := indexOf(order, "plugin-a")
		bIndex := indexOf(order, "plugin-b")
		cIndex := indexOf(order, "plugin-c")

		if aIndex >= bIndex {
			t.Errorf("plugin-a (index %d) should come before plugin-b (index %d)", aIndex, bIndex)
		}
		if bIndex >= cIndex {
			t.Errorf("plugin-b (index %d) should come before plugin-c (index %d)", bIndex, cIndex)
		}
	})

	t.Run("mixed Requires and After ordering", func(t *testing.T) {
		registry := newMockRegistry()
		loader := newMockLoader()

		pluginA := newMockPlugin("plugin-a", "1.0.0")
		pluginB := newMockPlugin("plugin-b", "1.0.0")
		pluginC := newMockPlugin("plugin-c", "1.0.0")
		pluginD := newMockPlugin("plugin-d", "1.0.0")

		// plugin-b requires plugin-a (hard dependency)
		pluginB.manifest.Requires = []string{"plugin-a"}
		// plugin-c comes after plugin-a (soft ordering)
		pluginC.manifest.After = []string{"plugin-a"}
		// plugin-d requires plugin-b and comes after plugin-c
		pluginD.manifest.Requires = []string{"plugin-b"}
		pluginD.manifest.After = []string{"plugin-c"}

		loader.factory["plugin-a"] = pluginA
		loader.factory["plugin-b"] = pluginB
		loader.factory["plugin-c"] = pluginC
		loader.factory["plugin-d"] = pluginD

		registry.plugins["plugin-a"] = *pluginA.manifest
		registry.plugins["plugin-b"] = *pluginB.manifest
		registry.plugins["plugin-c"] = *pluginC.manifest
		registry.plugins["plugin-d"] = *pluginD.manifest
		registry.enabled["plugin-a"] = true
		registry.enabled["plugin-b"] = true
		registry.enabled["plugin-c"] = true
		registry.enabled["plugin-d"] = true

		manager := newTestManager(registry, loader)
		err := manager.LoadAll(context.Background())
		if err != nil {
			t.Fatalf("LoadAll failed: %v", err)
		}

		err = manager.Start(context.Background())
		if err != nil {
			t.Fatalf("Start failed: %v", err)
		}

		order := manager.order
		aIndex := indexOf(order, "plugin-a")
		bIndex := indexOf(order, "plugin-b")
		cIndex := indexOf(order, "plugin-c")
		dIndex := indexOf(order, "plugin-d")

		// Verify ordering constraints
		if aIndex >= bIndex {
			t.Errorf("plugin-a (index %d) should come before plugin-b (index %d)", aIndex, bIndex)
		}
		if aIndex >= cIndex {
			t.Errorf("plugin-a (index %d) should come before plugin-c (index %d)", aIndex, cIndex)
		}
		if bIndex >= dIndex {
			t.Errorf("plugin-b (index %d) should come before plugin-d (index %d)", bIndex, dIndex)
		}
		if cIndex >= dIndex {
			t.Errorf("plugin-c (index %d) should come before plugin-d (index %d)", cIndex, dIndex)
		}
	})

	t.Run("no dependencies - any order is valid", func(t *testing.T) {
		registry := newMockRegistry()
		loader := newMockLoader()

		pluginA := newMockPlugin("plugin-a", "1.0.0")
		pluginB := newMockPlugin("plugin-b", "1.0.0")
		pluginC := newMockPlugin("plugin-c", "1.0.0")

		loader.factory["plugin-a"] = pluginA
		loader.factory["plugin-b"] = pluginB
		loader.factory["plugin-c"] = pluginC

		registry.plugins["plugin-a"] = *pluginA.manifest
		registry.plugins["plugin-b"] = *pluginB.manifest
		registry.plugins["plugin-c"] = *pluginC.manifest
		registry.enabled["plugin-a"] = true
		registry.enabled["plugin-b"] = true
		registry.enabled["plugin-c"] = true

		manager := newTestManager(registry, loader)
		err := manager.LoadAll(context.Background())
		if err != nil {
			t.Fatalf("LoadAll failed: %v", err)
		}

		err = manager.Start(context.Background())
		if err != nil {
			t.Fatalf("Start failed: %v", err)
		}

		// All plugins should be active
		stateA, _ := manager.GetState("plugin-a")
		stateB, _ := manager.GetState("plugin-b")
		stateC, _ := manager.GetState("plugin-c")

		if stateA != core.StateActive {
			t.Errorf("plugin-a state = %v, want %v", stateA, core.StateActive)
		}
		if stateB != core.StateActive {
			t.Errorf("plugin-b state = %v, want %v", stateB, core.StateActive)
		}
		if stateC != core.StateActive {
			t.Errorf("plugin-c state = %v, want %v", stateC, core.StateActive)
		}
	})

	t.Run("After references missing plugin ignored", func(t *testing.T) {
		registry := newMockRegistry()
		loader := newMockLoader()

		pluginA := newMockPlugin("plugin-a", "1.0.0")
		// After references nonexistent plugin - should be ignored (not an error)
		pluginA.manifest.After = []string{"nonexistent-plugin"}

		loader.factory["plugin-a"] = pluginA
		registry.plugins["plugin-a"] = *pluginA.manifest
		registry.enabled["plugin-a"] = true

		manager := newTestManager(registry, loader)
		err := manager.LoadAll(context.Background())
		if err != nil {
			t.Fatalf("LoadAll failed: %v", err)
		}

		// Start should succeed - After refs to missing plugins are ignored
		err = manager.Start(context.Background())
		if err != nil {
			t.Errorf("Start should succeed when After references missing plugin: %v", err)
		}

		state, _ := manager.GetState("plugin-a")
		if state != core.StateActive {
			t.Errorf("plugin-a state = %v, want %v", state, core.StateActive)
		}
	})

	t.Run("dependency with version constraint", func(t *testing.T) {
		registry := newMockRegistry()
		loader := newMockLoader()

		pluginA := newMockPlugin("plugin-a", "1.0.0")
		pluginB := newMockPlugin("plugin-b", "1.0.0")
		// Requires with version constraint
		pluginB.manifest.Requires = []string{"plugin-a@^1.0.0"}

		loader.factory["plugin-a"] = pluginA
		loader.factory["plugin-b"] = pluginB

		registry.plugins["plugin-a"] = *pluginA.manifest
		registry.plugins["plugin-b"] = *pluginB.manifest
		registry.enabled["plugin-a"] = true
		registry.enabled["plugin-b"] = true

		manager := newTestManager(registry, loader)
		err := manager.LoadAll(context.Background())
		if err != nil {
			t.Fatalf("LoadAll failed: %v", err)
		}

		err = manager.Start(context.Background())
		if err != nil {
			t.Fatalf("Start failed: %v", err)
		}

		order := manager.order
		aIndex := indexOf(order, "plugin-a")
		bIndex := indexOf(order, "plugin-b")

		if aIndex >= bIndex {
			t.Errorf("plugin-a (index %d) should come before plugin-b (index %d)", aIndex, bIndex)
		}
	})
}

// TestRetryFunctionality tests the Retry method comprehensively.
func TestRetryFunctionality(t *testing.T) {
	t.Run("retry plugin not found", func(t *testing.T) {
		registry := newMockRegistry()
		loader := newMockLoader()

		manager := newTestManager(registry, loader)

		err := manager.Retry(context.Background(), "nonexistent")
		if err == nil {
			t.Error("expected error for nonexistent plugin")
		}
		if err != ErrPluginNotFound {
			t.Errorf("expected ErrPluginNotFound, got %v", err)
		}
	})

	t.Run("retry fails when dependency not active", func(t *testing.T) {
		registry := newMockRegistry()
		loader := newMockLoader()

		pluginA := newMockPlugin("plugin-a", "1.0.0")
		pluginA.initErr = errors.New("init failed")
		pluginB := newMockPlugin("plugin-b", "1.0.0")
		pluginB.manifest.Requires = []string{"plugin-a"}

		loader.factory["plugin-a"] = pluginA
		loader.factory["plugin-b"] = pluginB

		registry.plugins["plugin-a"] = *pluginA.manifest
		registry.plugins["plugin-b"] = *pluginB.manifest
		registry.enabled["plugin-a"] = true
		registry.enabled["plugin-b"] = true

		manager := newTestManager(registry, loader)
		_ = manager.LoadAll(context.Background())
		_ = manager.Start(context.Background())

		// Both should be failed
		stateA, _ := manager.GetState("plugin-a")
		stateB, _ := manager.GetState("plugin-b")
		if stateA != core.StateFailed {
			t.Fatalf("plugin-a state = %v, want %v", stateA, core.StateFailed)
		}
		if stateB != core.StateFailed {
			t.Fatalf("plugin-b state = %v, want %v", stateB, core.StateFailed)
		}

		// Fix plugin-a, retry it first
		pluginA.initErr = nil
		err := manager.Retry(context.Background(), "plugin-a")
		if err != nil {
			t.Errorf("retry plugin-a should succeed: %v", err)
		}

		stateA, _ = manager.GetState("plugin-a")
		if stateA != core.StateActive {
			t.Errorf("plugin-a state after retry = %v, want %v", stateA, core.StateActive)
		}

		// Now retry plugin-b (dependency is active)
		err = manager.Retry(context.Background(), "plugin-b")
		if err != nil {
			t.Errorf("retry plugin-b should succeed: %v", err)
		}

		stateB, _ = manager.GetState("plugin-b")
		if stateB != core.StateActive {
			t.Errorf("plugin-b state after retry = %v, want %v", stateB, core.StateActive)
		}
	})

	t.Run("retry fails again on startup", func(t *testing.T) {
		registry := newMockRegistry()
		loader := newMockLoader()

		pluginA := newMockPlugin("plugin-a", "1.0.0")
		pluginA.initErr = errors.New("init failed")

		loader.factory["plugin-a"] = pluginA
		registry.plugins["plugin-a"] = *pluginA.manifest
		registry.enabled["plugin-a"] = true

		manager := newTestManager(registry, loader)
		_ = manager.LoadAll(context.Background())
		_ = manager.Start(context.Background())

		state, _ := manager.GetState("plugin-a")
		if state != core.StateFailed {
			t.Fatalf("plugin-a state = %v, want %v", state, core.StateFailed)
		}

		// Fix init but add start error
		pluginA.initErr = nil
		pluginA.startErr = errors.New("start failed")

		err := manager.Retry(context.Background(), "plugin-a")
		if err == nil {
			t.Error("expected retry to fail with start error")
		}

		state, _ = manager.GetState("plugin-a")
		if state != core.StateFailed {
			t.Errorf("plugin-a state after failed retry = %v, want %v", state, core.StateFailed)
		}

		// LoadError should be set
		info := manager.plugins["plugin-a"]
		if info.LoadError == nil {
			t.Error("expected LoadError to be set after failed retry")
		}
	})

	t.Run("retry with dependency still failed", func(t *testing.T) {
		registry := newMockRegistry()
		loader := newMockLoader()

		pluginA := newMockPlugin("plugin-a", "1.0.0")
		pluginA.initErr = errors.New("init failed")
		pluginB := newMockPlugin("plugin-b", "1.0.0")
		pluginB.manifest.Requires = []string{"plugin-a"}

		loader.factory["plugin-a"] = pluginA
		loader.factory["plugin-b"] = pluginB

		registry.plugins["plugin-a"] = *pluginA.manifest
		registry.plugins["plugin-b"] = *pluginB.manifest
		registry.enabled["plugin-a"] = true
		registry.enabled["plugin-b"] = true

		manager := newTestManager(registry, loader)
		_ = manager.LoadAll(context.Background())
		_ = manager.Start(context.Background())

		// plugin-b is failed due to dependency
		stateB, _ := manager.GetState("plugin-b")
		if stateB != core.StateFailed {
			t.Fatalf("plugin-b state = %v, want %v", stateB, core.StateFailed)
		}

		// Retry plugin-b while dependency is still failed
		err := manager.Retry(context.Background(), "plugin-b")
		if err == nil {
			t.Error("expected error when retrying with failed dependency")
		}

		var depErr *DependencyError
		if !errors.As(err, &depErr) {
			t.Errorf("expected DependencyError, got %T", err)
		} else {
			if depErr.Dependency != "plugin-a" {
				t.Errorf("expected dependency plugin-a, got %s", depErr.Dependency)
			}
		}
	})

	t.Run("retry state transitions correct", func(t *testing.T) {
		registry := newMockRegistry()
		loader := newMockLoader()
		eventBus := &mockEventBus{}

		pluginA := newMockPlugin("plugin-a", "1.0.0")
		pluginA.initErr = errors.New("init failed")

		loader.factory["plugin-a"] = pluginA
		registry.plugins["plugin-a"] = *pluginA.manifest
		registry.enabled["plugin-a"] = true

		manager := NewManager(registry, loader, eventBus, nil, nil)
		_ = manager.LoadAll(context.Background())
		_ = manager.Start(context.Background())

		// Clear event bus history
		eventBus.emitted = nil

		pluginA.initErr = nil
		err := manager.Retry(context.Background(), "plugin-a")
		if err != nil {
			t.Errorf("retry should succeed: %v", err)
		}

		// Check state transition events
		var stateChanges []StateChangeEventData
		for _, e := range eventBus.emitted {
			data, ok := e.Data["data"].(StateChangeEventData)
			if ok {
				stateChanges = append(stateChanges, data)
			}
		}

		// Expected: Failed -> Resolved -> Starting -> Active
		expectedTransitions := []struct {
			oldState core.PluginState
			newState core.PluginState
		}{
			{core.StateFailed, core.StateResolved},
			{core.StateResolved, core.StateStarting},
			{core.StateStarting, core.StateActive},
		}

		if len(stateChanges) < len(expectedTransitions) {
			t.Errorf("expected at least %d state changes, got %d", len(expectedTransitions), len(stateChanges))
		}

		for i, expected := range expectedTransitions {
			if i >= len(stateChanges) {
				break
			}
			if stateChanges[i].OldState != expected.oldState {
				t.Errorf("state change %d: oldState = %v, want %v", i, stateChanges[i].OldState, expected.oldState)
			}
			if stateChanges[i].NewState != expected.newState {
				t.Errorf("state change %d: newState = %v, want %v", i, stateChanges[i].NewState, expected.newState)
			}
		}
	})
}

// TestEnableErrorPaths tests Enable method error scenarios.
func TestEnableErrorPaths(t *testing.T) {
	t.Run("enable plugin not found", func(t *testing.T) {
		registry := newMockRegistry()
		loader := newMockLoader()

		manager := newTestManager(registry, loader)

		err := manager.Enable(context.Background(), "nonexistent")
		if err == nil {
			t.Error("expected error for nonexistent plugin")
		}
		if err != ErrPluginNotFound {
			t.Errorf("expected ErrPluginNotFound, got %v", err)
		}
	})

	t.Run("enable plugin in wrong state - active", func(t *testing.T) {
		registry := newMockRegistry()
		loader := newMockLoader()

		pluginA := newMockPlugin("plugin-a", "1.0.0")
		loader.factory["plugin-a"] = pluginA
		registry.plugins["plugin-a"] = *pluginA.manifest
		registry.enabled["plugin-a"] = true

		manager := newTestManager(registry, loader)
		_ = manager.LoadAll(context.Background())
		_ = manager.Start(context.Background())

		// Plugin is already active
		state, _ := manager.GetState("plugin-a")
		if state != core.StateActive {
			t.Fatalf("plugin-a state = %v, want %v", state, core.StateActive)
		}

		err := manager.Enable(context.Background(), "plugin-a")
		if err == nil {
			t.Error("expected error when enabling already active plugin")
		}

		var stateErr *StateError
		if !errors.As(err, &stateErr) {
			t.Errorf("expected StateError, got %T", err)
		} else {
			if stateErr.CurrentState != core.StateActive {
				t.Errorf("expected current state Active, got %v", stateErr.CurrentState)
			}
		}
	})

	t.Run("enable plugin in wrong state - registered", func(t *testing.T) {
		registry := newMockRegistry()
		loader := newMockLoader()

		pluginA := newMockPlugin("plugin-a", "1.0.0")
		loader.factory["plugin-a"] = pluginA
		registry.plugins["plugin-a"] = *pluginA.manifest
		registry.enabled["plugin-a"] = true

		manager := newTestManager(registry, loader)
		_ = manager.LoadAll(context.Background())

		// Plugin is in Registered state (not started yet)
		state, _ := manager.GetState("plugin-a")
		if state != core.StateRegistered {
			t.Fatalf("plugin-a state = %v, want %v", state, core.StateRegistered)
		}

		err := manager.Enable(context.Background(), "plugin-a")
		if err == nil {
			t.Error("expected error when enabling plugin in Registered state")
		}

		if _, ok := errors.AsType[*StateError](err); !ok {
			t.Errorf("expected StateError, got %T", err)
		}
	})

	t.Run("enable plugin with dependency not active", func(t *testing.T) {
		registry := newMockRegistry()
		loader := newMockLoader()

		pluginA := newMockPlugin("plugin-a", "1.0.0")
		pluginB := newMockPlugin("plugin-b", "1.0.0")
		pluginB.manifest.Requires = []string{"plugin-a"}

		loader.factory["plugin-a"] = pluginA
		loader.factory["plugin-b"] = pluginB

		registry.plugins["plugin-a"] = *pluginA.manifest
		registry.plugins["plugin-b"] = *pluginB.manifest
		registry.enabled["plugin-a"] = true
		registry.enabled["plugin-b"] = true

		manager := newTestManager(registry, loader)
		_ = manager.LoadAll(context.Background())
		_ = manager.Start(context.Background())

		// Disable both
		_ = manager.Disable(context.Background(), "plugin-b")
		_ = manager.Disable(context.Background(), "plugin-a")

		stateB, _ := manager.GetState("plugin-b")
		if stateB != core.StateStopped {
			t.Fatalf("plugin-b state = %v, want %v", stateB, core.StateStopped)
		}

		// Try to enable plugin-b when plugin-a is still stopped
		err := manager.Enable(context.Background(), "plugin-b")
		if err == nil {
			t.Error("expected error when enabling with inactive dependency")
		}

		var depErr *DependencyError
		if !errors.As(err, &depErr) {
			t.Errorf("expected DependencyError, got %T", err)
		} else {
			if depErr.Dependency != "plugin-a" {
				t.Errorf("expected dependency plugin-a, got %s", depErr.Dependency)
			}
		}
	})

	t.Run("enable fails on start", func(t *testing.T) {
		registry := newMockRegistry()
		loader := newMockLoader()

		pluginA := newMockPlugin("plugin-a", "1.0.0")
		pluginA.startErr = errors.New("start failed")

		loader.factory["plugin-a"] = pluginA
		registry.plugins["plugin-a"] = *pluginA.manifest
		registry.enabled["plugin-a"] = true

		manager := newTestManager(registry, loader)
		_ = manager.LoadAll(context.Background())

		// Manually set state to Stopped (simulating a disabled state)
		manager.plugins["plugin-a"].State = core.StateStopped

		err := manager.Enable(context.Background(), "plugin-a")
		if err == nil {
			t.Error("expected error when enable fails on start")
		}

		state, _ := manager.GetState("plugin-a")
		if state != core.StateFailed {
			t.Errorf("plugin-a state after failed enable = %v, want %v", state, core.StateFailed)
		}
	})

	t.Run("enable from failed state fails due to state machine", func(t *testing.T) {
		registry := newMockRegistry()
		loader := newMockLoader()

		pluginA := newMockPlugin("plugin-a", "1.0.0")
		pluginA.initErr = errors.New("init failed")

		loader.factory["plugin-a"] = pluginA
		registry.plugins["plugin-a"] = *pluginA.manifest
		registry.enabled["plugin-a"] = true

		manager := newTestManager(registry, loader)
		_ = manager.LoadAll(context.Background())
		_ = manager.Start(context.Background())

		state, _ := manager.GetState("plugin-a")
		if state != core.StateFailed {
			t.Fatalf("plugin-a state = %v, want %v", state, core.StateFailed)
		}

		pluginA.initErr = nil

		// Enable from Failed state fails because Failed->Starting is invalid
		err := manager.Enable(context.Background(), "plugin-a")
		if err == nil {
			t.Error("expected error when enabling from Failed state")
		}

		if _, ok := errors.AsType[*StateError](err); !ok {
			t.Errorf("expected StateError, got %T", err)
		}
	})
}

// TestDisableErrorPaths tests Disable method error scenarios.
func TestDisableErrorPaths(t *testing.T) {
	t.Run("disable plugin not found", func(t *testing.T) {
		registry := newMockRegistry()
		loader := newMockLoader()

		manager := newTestManager(registry, loader)

		err := manager.Disable(context.Background(), "nonexistent")
		if err == nil {
			t.Error("expected error for nonexistent plugin")
		}
		if err != ErrPluginNotFound {
			t.Errorf("expected ErrPluginNotFound, got %v", err)
		}
	})

	t.Run("disable plugin not active", func(t *testing.T) {
		registry := newMockRegistry()
		loader := newMockLoader()

		pluginA := newMockPlugin("plugin-a", "1.0.0")
		loader.factory["plugin-a"] = pluginA
		registry.plugins["plugin-a"] = *pluginA.manifest
		registry.enabled["plugin-a"] = true

		manager := newTestManager(registry, loader)
		_ = manager.LoadAll(context.Background())

		state, _ := manager.GetState("plugin-a")
		if state != core.StateRegistered {
			t.Fatalf("plugin-a state = %v, want %v", state, core.StateRegistered)
		}

		err := manager.Disable(context.Background(), "plugin-a")
		if err == nil {
			t.Error("expected error when disabling non-active plugin")
		}

		var stateErr *StateError
		if !errors.As(err, &stateErr) {
			t.Errorf("expected StateError, got %T", err)
		} else {
			if stateErr.CurrentState != core.StateRegistered {
				t.Errorf("expected current state Registered, got %v", stateErr.CurrentState)
			}
		}
	})

	t.Run("disable plugin in failed state", func(t *testing.T) {
		registry := newMockRegistry()
		loader := newMockLoader()

		pluginA := newMockPlugin("plugin-a", "1.0.0")
		pluginA.initErr = errors.New("init failed")

		loader.factory["plugin-a"] = pluginA
		registry.plugins["plugin-a"] = *pluginA.manifest
		registry.enabled["plugin-a"] = true

		manager := newTestManager(registry, loader)
		_ = manager.LoadAll(context.Background())
		_ = manager.Start(context.Background())

		state, _ := manager.GetState("plugin-a")
		if state != core.StateFailed {
			t.Fatalf("plugin-a state = %v, want %v", state, core.StateFailed)
		}

		err := manager.Disable(context.Background(), "plugin-a")
		if err == nil {
			t.Error("expected error when disabling Failed plugin")
		}

		if _, ok := errors.AsType[*StateError](err); !ok {
			t.Errorf("expected StateError, got %T", err)
		}
	})

	t.Run("disable fails on stop error", func(t *testing.T) {
		registry := newMockRegistry()
		loader := newMockLoader()

		pluginA := newMockPlugin("plugin-a", "1.0.0")
		pluginA.stopErr = errors.New("stop failed")

		loader.factory["plugin-a"] = pluginA
		registry.plugins["plugin-a"] = *pluginA.manifest
		registry.enabled["plugin-a"] = true

		manager := newTestManager(registry, loader)
		_ = manager.LoadAll(context.Background())
		_ = manager.Start(context.Background())

		state, _ := manager.GetState("plugin-a")
		if state != core.StateActive {
			t.Fatalf("plugin-a state = %v, want %v", state, core.StateActive)
		}

		err := manager.Disable(context.Background(), "plugin-a")
		if err == nil {
			t.Error("expected error when stop fails")
		}

		state, _ = manager.GetState("plugin-a")
		if state != core.StateFailed {
			t.Errorf("plugin-a state after failed disable = %v, want %v", state, core.StateFailed)
		}
	})
}

// TestHasFailedDependencies tests the hasFailedDependencies helper function.
func TestHasFailedDependencies(t *testing.T) {
	t.Run("no failed dependencies", func(t *testing.T) {
		info := &PluginLoadInfo{
			Manifest: &core.Manifest{
				Name:     "test-plugin",
				Requires: []string{"dep-a", "dep-b"},
			},
		}
		failedPlugins := []string{"other-plugin"}

		if hasFailedDependencies(info, failedPlugins) {
			t.Error("expected false when no required deps are in failed list")
		}
	})

	t.Run("one failed dependency", func(t *testing.T) {
		info := &PluginLoadInfo{
			Manifest: &core.Manifest{
				Name:     "test-plugin",
				Requires: []string{"dep-a", "dep-b"},
			},
		}
		failedPlugins := []string{"dep-a"}

		if !hasFailedDependencies(info, failedPlugins) {
			t.Error("expected true when one required dep is failed")
		}
	})

	t.Run("multiple failed dependencies", func(t *testing.T) {
		info := &PluginLoadInfo{
			Manifest: &core.Manifest{
				Name:     "test-plugin",
				Requires: []string{"dep-a", "dep-b"},
			},
		}
		failedPlugins := []string{"dep-a", "dep-b"}

		if !hasFailedDependencies(info, failedPlugins) {
			t.Error("expected true when all required deps are failed")
		}
	})

	t.Run("empty requires list", func(t *testing.T) {
		info := &PluginLoadInfo{
			Manifest: &core.Manifest{
				Name:     "test-plugin",
				Requires: []string{},
			},
		}
		failedPlugins := []string{"any-plugin"}

		if hasFailedDependencies(info, failedPlugins) {
			t.Error("expected false for plugin with no requirements")
		}
	})

	t.Run("dependency with version constraint", func(t *testing.T) {
		info := &PluginLoadInfo{
			Manifest: &core.Manifest{
				Name:     "test-plugin",
				Requires: []string{"dep-a@^1.0.0"},
			},
		}
		failedPlugins := []string{"dep-a"}

		if !hasFailedDependencies(info, failedPlugins) {
			t.Error("expected true when versioned dep is failed")
		}
	})
}

// TestGetStateErrors tests GetState error handling.
func TestGetStateErrors(t *testing.T) {
	t.Run("get state for nonexistent plugin", func(t *testing.T) {
		registry := newMockRegistry()
		loader := newMockLoader()

		manager := newTestManager(registry, loader)

		state, err := manager.GetState("nonexistent")
		if err == nil {
			t.Error("expected error for nonexistent plugin")
		}
		if err != ErrPluginNotFound {
			t.Errorf("expected ErrPluginNotFound, got %v", err)
		}
		if state != core.StateFailed {
			t.Errorf("expected StateFailed return value, got %v", state)
		}
	})
}

// TestGetPluginReturnsNil tests GetPlugin for nonexistent plugins.
func TestGetPluginReturnsNil(t *testing.T) {
	t.Run("get nonexistent plugin returns nil", func(t *testing.T) {
		registry := newMockRegistry()
		loader := newMockLoader()

		manager := newTestManager(registry, loader)

		plugin := manager.GetPlugin("nonexistent")
		if plugin != nil {
			t.Error("expected nil for nonexistent plugin")
		}
	})
}

// TestStopErrors tests Stop method error handling.
func TestStopErrors(t *testing.T) {
	t.Run("stop collects multiple errors", func(t *testing.T) {
		registry := newMockRegistry()
		loader := newMockLoader()

		pluginA := newMockPlugin("plugin-a", "1.0.0")
		pluginA.stopErr = errors.New("stop failed")
		pluginB := newMockPlugin("plugin-b", "1.0.0")
		pluginB.manifest.Requires = []string{"plugin-a"}
		pluginB.stopErr = errors.New("stop failed too")

		loader.factory["plugin-a"] = pluginA
		loader.factory["plugin-b"] = pluginB

		registry.plugins["plugin-a"] = *pluginA.manifest
		registry.plugins["plugin-b"] = *pluginB.manifest
		registry.enabled["plugin-a"] = true
		registry.enabled["plugin-b"] = true

		manager := newTestManager(registry, loader)
		_ = manager.LoadAll(context.Background())
		_ = manager.Start(context.Background())

		err := manager.Stop(context.Background())
		if err == nil {
			t.Error("expected combined error for multiple stop failures")
		}

		// Error message should contain both plugin names
		errMsg := err.Error()
		if !containsString(errMsg, "plugin-a") {
			t.Errorf("error message should contain plugin-a: %s", errMsg)
		}
		if !containsString(errMsg, "plugin-b") {
			t.Errorf("error message should contain plugin-b: %s", errMsg)
		}

		// Both should be in Failed state
		stateA, _ := manager.GetState("plugin-a")
		stateB, _ := manager.GetState("plugin-b")
		if stateA != core.StateFailed {
			t.Errorf("plugin-a state = %v, want %v", stateA, core.StateFailed)
		}
		if stateB != core.StateFailed {
			t.Errorf("plugin-b state = %v, want %v", stateB, core.StateFailed)
		}
	})

	t.Run("stop with empty order", func(t *testing.T) {
		registry := newMockRegistry()
		loader := newMockLoader()

		manager := newTestManager(registry, loader)
		_ = manager.LoadAll(context.Background())

		// No plugins were started, order is empty
		err := manager.Stop(context.Background())
		if err != nil {
			t.Errorf("Stop should succeed with empty order: %v", err)
		}
	})

	t.Run("stop processes failed state plugins", func(t *testing.T) {
		registry := newMockRegistry()
		loader := newMockLoader()

		pluginA := newMockPlugin("plugin-a", "1.0.0")
		pluginA.initErr = errors.New("init failed")
		pluginB := newMockPlugin("plugin-b", "1.0.0")

		loader.factory["plugin-a"] = pluginA
		loader.factory["plugin-b"] = pluginB

		registry.plugins["plugin-a"] = *pluginA.manifest
		registry.plugins["plugin-b"] = *pluginB.manifest
		registry.enabled["plugin-a"] = true
		registry.enabled["plugin-b"] = true

		manager := newTestManager(registry, loader)
		_ = manager.LoadAll(context.Background())
		_ = manager.Start(context.Background())

		stateA, _ := manager.GetState("plugin-a")
		if stateA != core.StateFailed {
			t.Fatalf("plugin-a state = %v, want %v", stateA, core.StateFailed)
		}

		_ = manager.Stop(context.Background())

		// plugin-a stop IS called because Failed->Stopping is valid per stateMachine
		if !pluginA.stopCalled {
			t.Error("plugin-a Stop should be called - Failed->Stopping is valid")
		}
		// plugin-b stop should be called
		if !pluginB.stopCalled {
			t.Error("plugin-b Stop should be called")
		}

		// plugin-a transitions Failed->Stopping->Stopped
		stateA, _ = manager.GetState("plugin-a")
		if stateA != core.StateStopped {
			t.Errorf("plugin-a state after stop = %v, want %v", stateA, core.StateStopped)
		}
	})
}

// TestLoadAllErrors tests LoadAll error paths.
func TestLoadAllErrors(t *testing.T) {
	t.Run("load plugin failure sets failed state", func(t *testing.T) {
		registry := newMockRegistry()
		loader := &mockLoaderWithError{}

		registry.plugins["plugin-a"] = core.Manifest{Name: "plugin-a", Version: "1.0.0", Engine: "native"}
		registry.enabled["plugin-a"] = true

		manager := newTestManager(registry, loader)
		err := manager.LoadAll(context.Background())
		// LoadAll should succeed even if individual plugins fail
		if err != nil {
			t.Errorf("LoadAll should succeed when plugin fails to load: %v", err)
		}

		// Plugin should be in Failed state
		state, _ := manager.GetState("plugin-a")
		if state != core.StateFailed {
			t.Errorf("plugin-a state = %v, want %v", state, core.StateFailed)
		}

		// LoadError should be set
		info := manager.plugins["plugin-a"]
		if info.LoadError == nil {
			t.Error("expected LoadError to be set")
		}
	})
}

// mockLoaderWithError returns error on Load.
type mockLoaderWithError struct{}

func (l *mockLoaderWithError) Load(manifest core.Manifest) (core.Plugin, error) {
	return nil, errors.New("load failed")
}

// TestCanTransition tests the canTransition method on ManagerImpl.
func TestCanTransitionMethod(t *testing.T) {
	t.Run("canTransition via manager", func(t *testing.T) {
		registry := newMockRegistry()
		loader := newMockLoader()
		manager := newTestManager(registry, loader)

		// Create plugin info in different states
		info := &PluginLoadInfo{
			State: core.StateRegistered,
		}

		// Valid transition
		if !manager.canTransition(info, core.StateResolved) {
			t.Error("expected transition from Registered to Resolved to be valid")
		}

		// Invalid transition
		if manager.canTransition(info, core.StateActive) {
			t.Error("expected transition from Registered to Active to be invalid")
		}
	})
}

// TestAllStateTransitions tests all possible state transitions systematically.
func TestAllStateTransitions(t *testing.T) {
	tests := []struct {
		name     string
		from     core.PluginState
		to       core.PluginState
		expected bool
	}{
		// Valid transitions from Registered
		{"Registered->Resolved", core.StateRegistered, core.StateResolved, true},
		{"Registered->Failed", core.StateRegistered, core.StateFailed, true},
		{"Registered->Starting", core.StateRegistered, core.StateStarting, false},
		{"Registered->Active", core.StateRegistered, core.StateActive, false},
		{"Registered->Stopping", core.StateRegistered, core.StateStopping, false},
		{"Registered->Stopped", core.StateRegistered, core.StateStopped, false},

		// Valid transitions from Resolved
		{"Resolved->Starting", core.StateResolved, core.StateStarting, true},
		{"Resolved->Failed", core.StateResolved, core.StateFailed, true},
		{"Resolved->Stopped", core.StateResolved, core.StateStopped, true},
		{"Resolved->Active", core.StateResolved, core.StateActive, false},

		// Valid transitions from Starting
		{"Starting->Active", core.StateStarting, core.StateActive, true},
		{"Starting->Failed", core.StateStarting, core.StateFailed, true},
		{"Starting->Stopping", core.StateStarting, core.StateStopping, false},

		// Valid transitions from Active
		{"Active->Stopping", core.StateActive, core.StateStopping, true},
		{"Active->Failed", core.StateActive, core.StateFailed, false},
		{"Active->Stopped", core.StateActive, core.StateStopped, false},

		// Valid transitions from Stopping
		{"Stopping->Stopped", core.StateStopping, core.StateStopped, true},
		{"Stopping->Failed", core.StateStopping, core.StateFailed, true},
		{"Stopping->Active", core.StateStopping, core.StateActive, false},

		// Valid transitions from Stopped
		{"Stopped->Resolved", core.StateStopped, core.StateResolved, true},
		{"Stopped->Starting", core.StateStopped, core.StateStarting, true},
		{"Stopped->Failed", core.StateStopped, core.StateFailed, true},
		{"Stopped->Active", core.StateStopped, core.StateActive, false},

		// Valid transitions from Failed
		{"Failed->Stopping", core.StateFailed, core.StateStopping, true},
		{"Failed->Stopped", core.StateFailed, core.StateStopped, true},
		{"Failed->Resolved", core.StateFailed, core.StateResolved, false},
		{"Failed->Starting", core.StateFailed, core.StateStarting, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := canTransition(tt.from, tt.to)
			if result != tt.expected {
				t.Errorf("canTransition(%v, %v) = %v, want %v", tt.from, tt.to, result, tt.expected)
			}
		})
	}
}

// TestStartPluginWithNilCtxFactory tests startPlugin with nil ctxFactory.
func TestStartPluginWithNilCtxFactory(t *testing.T) {
	registry := newMockRegistry()
	loader := newMockLoader()

	pluginA := newMockPlugin("plugin-a", "1.0.0")
	loader.factory["plugin-a"] = pluginA
	registry.plugins["plugin-a"] = *pluginA.manifest
	registry.enabled["plugin-a"] = true

	// Manager with nil ctxFactory
	manager := NewManager(registry, loader, nil, nil, nil)

	err := manager.LoadAll(context.Background())
	if err != nil {
		t.Fatalf("LoadAll failed: %v", err)
	}

	err = manager.Start(context.Background())
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Init should still be called with nil CoreContext
	if !pluginA.initCalled {
		t.Error("expected Init to be called even with nil ctxFactory")
	}
}

// TestErrorTypes tests the Error() methods of StateError and DependencyError.
func TestStateError_Error(t *testing.T) {
	err := &StateError{
		PluginName:   "test-plugin",
		CurrentState: core.StateRegistered,
		TargetState:  core.StateActive,
		Message:      "invalid transition",
	}

	msg := err.Error()
	if !containsString(msg, "test-plugin") {
		t.Errorf("Error() should contain plugin name: %s", msg)
	}
	if !containsString(msg, "registered") {
		t.Errorf("Error() should contain current state: %s", msg)
	}
	if !containsString(msg, "active") {
		t.Errorf("Error() should contain target state: %s", msg)
	}
	if !containsString(msg, "invalid transition") {
		t.Errorf("Error() should contain message: %s", msg)
	}
}

func TestDependencyError_Error_WithCyclePath(t *testing.T) {
	err := &DependencyError{
		PluginName: "plugin-a",
		Dependency: "plugin-b",
		Message:    "cycle found",
		CyclePath:  []string{"plugin-a", "plugin-b", "plugin-a"},
	}

	msg := err.Error()
	if !containsString(msg, "cycle detected") {
		t.Errorf("Error() should mention cycle: %s", msg)
	}
	if !containsString(msg, "plugin-a") {
		t.Errorf("Error() should contain cycle path: %s", msg)
	}
}

func TestDependencyError_Error_WithoutCyclePath(t *testing.T) {
	err := &DependencyError{
		PluginName: "plugin-a",
		Dependency: "plugin-b",
		Message:    "not active",
	}

	msg := err.Error()
	if !containsString(msg, "plugin-a") {
		t.Errorf("Error() should contain plugin name: %s", msg)
	}
	if !containsString(msg, "plugin-b") {
		t.Errorf("Error() should contain dependency name: %s", msg)
	}
	if !containsString(msg, "not active") {
		t.Errorf("Error() should contain message: %s", msg)
	}
	if containsString(msg, "cycle detected") {
		t.Errorf("Error() should not mention 'cycle detected' without CyclePath: %s", msg)
	}
}

func TestLoadAll_RegistryListError(t *testing.T) {
	registry := &errorListRegistry{listErr: errors.New("registry unavailable")}
	loader := newMockLoader()

	manager := newTestManager(registry, loader)
	err := manager.LoadAll(context.Background())
	if err == nil {
		t.Error("LoadAll should fail when registry.List() fails")
	}
	if !containsString(err.Error(), "registry unavailable") {
		t.Errorf("Error should wrap registry error: %v", err)
	}
}

type errorListRegistry struct {
	mockRegistry
	listErr error
}

func (r *errorListRegistry) List() ([]core.Manifest, error) {
	return nil, r.listErr
}

// Helper function to check if string contains substring.
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
