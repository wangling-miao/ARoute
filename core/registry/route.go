package registry

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"
)

// RouteType enumerates all addressable entity categories in the unified routing table.
type RouteType string

const (
	RouteTypePlugin     RouteType = "plugin"
	RouteTypeService    RouteType = "service"
	RouteTypeHook       RouteType = "hook"
	RouteTypeTheme      RouteType = "theme"
	RouteTypeMiddleware RouteType = "middleware"
	RouteTypeLicense    RouteType = "license"
)

// validRouteTypes is the set of recognized route types.
var validRouteTypes = map[RouteType]bool{
	RouteTypePlugin:     true,
	RouteTypeService:    true,
	RouteTypeHook:       true,
	RouteTypeTheme:      true,
	RouteTypeMiddleware: true,
	RouteTypeLicense:    true,
}

// TrustLevel determines the execution isolation boundary for a route.
type TrustLevel int

const (
	TrustL1 TrustLevel = iota // Native, in-process (official built-in plugins)
	TrustL2                   // gRPC subprocess (auth/pro plugins, reserved for Pro)
	TrustL3                   // WASM sandbox (third-party plugins, zero-trust)
)

func (t TrustLevel) String() string {
	switch t {
	case TrustL1:
		return "L1"
	case TrustL2:
		return "L2"
	case TrustL3:
		return "L3"
	default:
		return "unknown"
	}

}

// MarshalJSON implements json.Marshaler for TrustLevel.
func (t TrustLevel) MarshalJSON() ([]byte, error) {
	return json.Marshal(t.String())
}

// UnmarshalJSON implements json.Unmarshaler for TrustLevel.
func (t *TrustLevel) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	switch s {
	case "L1":
		*t = TrustL1
	case "L2":
		*t = TrustL2
	case "L3":
		*t = TrustL3
	default:
		return fmt.Errorf("invalid trust level: %s", s)
	}
	return nil
}

// RouteState represents the operational state of a route.
type RouteState string

const (
	RouteStateActive   RouteState = "active"
	RouteStateInactive RouteState = "inactive"
	RouteStateOrphaned RouteState = "orphaned"
	RouteStateError    RouteState = "error"
)

// RouteRef is a reference to another route, used in dependency declarations.
type RouteRef struct {
	Type   RouteType `json:"type"`
	Domain string    `json:"domain"`
	Name   string    `json:"name"`
}

// String returns a human-readable representation of a RouteRef.
func (r RouteRef) String() string {
	return string(r.Type) + "/" + r.Domain + "/" + r.Name
}

var domainRegex = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]*$`)
var nameRegexRoute = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// Route is the universal entry in the unified routing table.
// All addressable entities in ARoute (plugins, services, hooks, themes, middleware, license)
// are stored as Route records with a composite key: {Type}/{Domain}/{Name}.
type Route struct {
	Type    RouteType `json:"type"`
	Domain  string    `json:"domain"`
	Name    string    `json:"name"`
	Version string    `json:"version"`

	TrustLevel TrustLevel `json:"trust_level"`
	Engine     string     `json:"engine,omitempty"`

	State   RouteState `json:"state"`
	Enabled bool       `json:"enabled"`

	Requires []RouteRef        `json:"requires,omitempty"`
	Provides []RouteRef        `json:"provides,omitempty"`
	Payload  json.RawMessage   `json:"payload,omitempty"`

	DiscoveredPath string    `json:"discovered_path,omitempty"`
	RegisteredAt   time.Time `json:"registered_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// Key returns the composite storage key for this route.
func (r *Route) Key() string {
	return string(r.Type) + "/" + r.Domain + "/" + r.Name
}

// Ref returns a RouteRef pointing to this route.
func (r *Route) Ref() RouteRef {
	return RouteRef{Type: r.Type, Domain: r.Domain, Name: r.Name}
}

// Validate checks the route for required fields and valid values.
func (r *Route) Validate() error {
	var errs []error

	if !validRouteTypes[r.Type] {
		errs = append(errs, fmt.Errorf("invalid route type: %s", r.Type))
	}

	if r.Domain == "" {
		errs = append(errs, errors.New("domain is required"))
	} else if !domainRegex.MatchString(r.Domain) {
		errs = append(errs, fmt.Errorf("domain must match %s", domainRegex.String()))
	}

	if r.Name == "" {
		errs = append(errs, errors.New("name is required"))
	} else if !nameRegexRoute.MatchString(r.Name) {
		errs = append(errs, fmt.Errorf("name must match %s", nameRegexRoute.String()))
	}

	if r.Version == "" {
		errs = append(errs, errors.New("version is required"))
	}

	if r.Engine != "" && r.Engine != "native" && r.Engine != "grpc" && r.Engine != "wasm" {
		errs = append(errs, fmt.Errorf("engine must be 'native', 'grpc', or 'wasm', got %q", r.Engine))
	}

	for i, ref := range r.Requires {
		if !validRouteTypes[ref.Type] {
			errs = append(errs, fmt.Errorf("requires[%d]: invalid route type %s", i, ref.Type))
		}
		if ref.Name == "" {
			errs = append(errs, fmt.Errorf("requires[%d]: name is required", i))
		}
	}

	for i, ref := range r.Provides {
		if !validRouteTypes[ref.Type] {
			errs = append(errs, fmt.Errorf("provides[%d]: invalid route type %s", i, ref.Type))
		}
		if ref.Name == "" {
			errs = append(errs, fmt.Errorf("provides[%d]: name is required", i))
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

// --- Type-specific Payload Structures ---

// PluginPayload carries plugin-specific data in a Route's Payload field.
type PluginPayload struct {
	Description string   `json:"description"`
	Author      string   `json:"author"`
	License     string   `json:"license"`
	Keywords    []string `json:"keywords,omitempty"`
	Homepage    string   `json:"homepage,omitempty"`
	Repository  string   `json:"repository,omitempty"`
	Permissions []string `json:"permissions,omitempty"`
}

// ServicePayload carries service registration data.
type ServicePayload struct {
	InterfaceType string `json:"interface_type"`
	Singleton     bool   `json:"singleton"`
	Description   string `json:"description"`
}

// HookPayload carries event hook registration data.
type HookPayload struct {
	Topic     string `json:"topic"`
	Priority  int    `json:"priority"`
	Mode      string `json:"mode"`       // "filter" or "broadcast"
	HandlerID string `json:"handler_id"`
}

// ThemePayload carries theme-specific data.
type ThemePayload struct {
	ScriptEngine string           `json:"script_engine"`
	PreviewImage string           `json:"preview_image,omitempty"`
	MinVersion   string           `json:"min_version,omitempty"`
	Active       bool             `json:"active"`
	ConfigSchema json.RawMessage  `json:"config_schema,omitempty"`
}

// MiddlewarePayload carries middleware registration data.
type MiddlewarePayload struct {
	Layer    string `json:"layer"`
	Priority int    `json:"priority"`
	Global   bool   `json:"global"`
}

// LicensePayload carries license/feature-flag data.
type LicensePayload struct {
	Edition   string     `json:"edition"`
	Features  []string   `json:"features"`
	Signature string     `json:"signature,omitempty"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// EncodePayload marshals a payload struct into json.RawMessage.
func EncodePayload(v interface{}) (json.RawMessage, error) {
	return json.Marshal(v)
}

// DecodePayload unmarshals a Route's Payload into a target struct.
func DecodePayload(r *Route, target interface{}) error {
	if len(r.Payload) == 0 {
		return nil
	}
	return json.Unmarshal(r.Payload, target)
}
