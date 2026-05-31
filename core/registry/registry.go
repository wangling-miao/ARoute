// Package registry provides plugin registration and discovery functionality.
package registry

import (
	"errors"

	"github.com/wangling-miao/aroute/core"
)

// PluginEntry represents a registered plugin with its manifest and state.
type PluginEntry struct {
	// Manifest contains the plugin metadata (name, version, dependencies, etc.)
	Manifest core.Manifest `json:"manifest"`

	// Enabled indicates whether the plugin is active.
	// Disabled plugins are discovered but not loaded during startup.
	Enabled bool `json:"enabled"`

	// DiscoveredPath is the filesystem path where the plugin was found.
	// Empty for programmatically registered plugins.
	DiscoveredPath string `json:"discovered_path,omitempty"`

	// Trust metadata is populated by the unified registry and policy engine.
	TrustLevel       string         `json:"trust_level,omitempty"`
	EffectiveTrust   string         `json:"effective_trust,omitempty"`
	RiskScore        int            `json:"risk_score,omitempty"`
	TrustState       string         `json:"trust_state,omitempty"`
	Capabilities     []string       `json:"capabilities,omitempty"`
	CapabilityGrants []string       `json:"capability_grants,omitempty"`
	LastDecision     *TrustDecision `json:"last_decision,omitempty"`
	PolicyRevision   string         `json:"policy_revision,omitempty"`
}

// Registry defines the interface for plugin registration and discovery.
// Implementations must be safe for concurrent use.
//
// Thread safety: All methods must be safe to call from multiple goroutines.
// bbolt supports multiple concurrent readers but only one writer at a time.
type Registry interface {
	// Register adds a new plugin to the registry.
	// Returns an error if a plugin with the same name already exists.
	//
	// The plugin is registered with enabled=true by default.
	// The manifest is validated before registration.
	//
	// Thread safety: Serialized write operation.
	Register(entry *PluginEntry) error

	// Get retrieves a plugin entry by name.
	// Returns ErrPluginNotFound if no plugin with that name exists.
	//
	// Thread safety: Concurrent read operation.
	Get(name string) (*PluginEntry, error)

	// List returns all registered plugins.
	// Returns an empty slice if no plugins are registered (not nil).
	//
	// Thread safety: Concurrent read operation.
	// The returned list is a snapshot at the time of the call.
	List() ([]*PluginEntry, error)

	// Update modifies a plugin's manifest while preserving its enabled state.
	// Returns ErrPluginNotFound if the plugin doesn't exist.
	//
	// Common use case: upgrading a plugin to a new version while keeping
	// its enabled/disabled status.
	//
	// Thread safety: Serialized write operation.
	Update(name string, manifest core.Manifest) error

	// Remove deletes a plugin entry from the registry.
	// Returns ErrPluginNotFound if the plugin doesn't exist.
	// After removal, the plugin can be re-registered as a new plugin.
	//
	// Thread safety: Serialized write operation.
	Remove(name string) error

	// Enable sets a plugin's enabled state to true.
	// Returns ErrPluginNotFound if the plugin doesn't exist.
	// If the plugin is already enabled, this is a no-op (no error).
	//
	// Thread safety: Serialized write operation.
	Enable(name string) error

	// Disable sets a plugin's enabled state to false.
	// Returns ErrPluginNotFound if the plugin doesn't exist.
	// If the plugin is already disabled, this is a no-op (no error).
	//
	// Thread safety: Serialized write operation.
	Disable(name string) error

	// Close releases any resources used by the registry.
	// After Close, all other methods will return errors.
	Close() error
}

// Discovery defines the interface for plugin discovery.
// Discovery implementations scan filesystem locations to find plugins.
type Discovery interface {
	// Discover scans for plugins and returns their manifest paths.
	// Returns a map of plugin name to absolute path of manifest file.
	//
	// Thread safety: Concurrent read operation.
	Discover() (map[string]string, error)
}

// Errors returned by the registry.
var (
	// ErrPluginNotFound indicates the requested plugin does not exist.
	ErrPluginNotFound = &PluginError{Op: "get", Msg: "plugin not found"}

	// ErrPluginExists indicates a plugin with that name already exists.
	ErrPluginExists = &PluginError{Op: "register", Msg: "plugin already exists"}

	// ErrRegistryClosed indicates the registry has been closed.
	ErrRegistryClosed = &PluginError{Op: "registry", Msg: "registry is closed"}
)

// PluginError represents an error from registry operations.
type PluginError struct {
	PluginName string
	Op         string // Operation that failed: "register", "get", "update", etc.
	Msg        string
}

func (e *PluginError) Error() string {
	if e.PluginName != "" {
		return "registry: " + e.Op + " plugin " + e.PluginName + ": " + e.Msg
	}
	return "registry: " + e.Op + ": " + e.Msg
}

func (e *PluginError) Unwrap() error {
	return nil
}

// IsPluginNotFound returns true if err is ErrPluginNotFound or wraps it.
func IsPluginNotFound(err error) bool {
	if e, ok := errors.AsType[*PluginError](err); ok {
		return e.Op == "get" && e.Msg == "plugin not found"
	}
	return false
}

// IsPluginExists returns true if err is ErrPluginExists or wraps it.
func IsPluginExists(err error) bool {
	if e, ok := errors.AsType[*PluginError](err); ok {
		return e.Op == "register" && e.Msg == "plugin already exists"
	}
	return false
}
