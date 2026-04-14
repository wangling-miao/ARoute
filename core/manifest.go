package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

var nameRegex = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

type Manifest struct {
	Name        string   `yaml:"name" json:"name"`
	Version     string   `yaml:"version" json:"version"`
	Description string   `yaml:"description" json:"description"`
	Author      string   `yaml:"author" json:"author"`
	License     string   `yaml:"license" json:"license"`
	Engine      string   `yaml:"engine" json:"engine"`
	Requires    []string `yaml:"requires" json:"requires"`
	After       []string `yaml:"after" json:"after"`
	Provides    []string `yaml:"provides" json:"provides"`
	Homepage    string   `yaml:"homepage" json:"homepage,omitempty"`
	Repository  string   `yaml:"repository" json:"repository,omitempty"`
	Keywords    []string `yaml:"keywords" json:"keywords,omitempty"`
}

func (m *Manifest) Validate() error {
	var errs []error

	if m.Name == "" {
		errs = append(errs, errors.New("name is required"))
	} else if !nameRegex.MatchString(m.Name) {
		errs = append(errs, errors.New("name must be lowercase alphanumeric with hyphens, starting with a letter (^[a-z][a-z0-9-]*$)"))
	}

	if m.Version == "" {
		errs = append(errs, errors.New("version is required"))
	} else if err := validateSemver(m.Version); err != nil {
		errs = append(errs, fmt.Errorf("invalid version: %w", err))
	}

	if m.Engine == "" {
		errs = append(errs, errors.New("engine is required (must be 'native' or 'wasm')"))
	} else if _, err := ParseEngine(m.Engine); err != nil {
		errs = append(errs, err)
	}

	for _, req := range m.Requires {
		if err := validateDependencyConstraint(req); err != nil {
			errs = append(errs, fmt.Errorf("invalid requires constraint %q: %w", req, err))
		}
	}

	seen := make(map[string]bool)
	for _, p := range m.Provides {
		if seen[p] {
			errs = append(errs, fmt.Errorf("duplicate capability name %q in provides", p))
		}
		seen[p] = true
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

// ParseEngine parses an engine type string and returns the corresponding EngineType.
// Supports "native" and "l1" for L1Native, "wasm" and "l3" for L3Wasm.
func ParseEngine(s string) (EngineType, error) {
	switch s {
	case "native", "l1":
		return EngineL1Native, nil
	case "wasm", "l3":
		return EngineL3Wasm, nil
	default:
		return EngineType(0), fmt.Errorf("engine must be 'native' or 'wasm', got %q", s)
	}
}

// ParseDependency parses a dependency constraint string.
// Format: "name" or "name@version-constraint"
func ParseDependency(s string) (name, constraint string, err error) {
	parts := strings.SplitN(s, "@", 2)
	if len(parts) == 1 {
		return parts[0], "*", nil
	}
	name = parts[0]
	constraint = parts[1]
	if constraint == "" {
		return "", "", fmt.Errorf("empty version constraint")
	}
	return name, constraint, nil
}

// LoadManifest loads a manifest from a YAML or JSON file.
func LoadManifest(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}

	ext := ".yaml"
	if strings.HasSuffix(path, ".json") {
		ext = ".json"
	}

	return ParseManifest(data, ext)
}

// ParseManifest parses a manifest from raw bytes.
// The ext parameter should be ".yaml", ".yml", or ".json" to select the parser.
func ParseManifest(data []byte, ext string) (*Manifest, error) {
	var m Manifest
	switch ext {
	case ".yaml", ".yml":
		if err := yaml.Unmarshal(data, &m); err != nil {
			return nil, fmt.Errorf("parse yaml manifest: %w", err)
		}
	case ".json":
		if err := json.Unmarshal(data, &m); err != nil {
			return nil, fmt.Errorf("parse json manifest: %w", err)
		}
	default:
		return nil, fmt.Errorf("unsupported manifest format: %s", ext)
	}

	if err := m.Validate(); err != nil {
		return nil, err
	}

	return &m, nil
}

// validateSemver validates a semantic version string.
// Supports: MAJOR.MINOR.PATCH[-PRERELEASE][+BUILD]
func validateSemver(v string) error {
	if len(v) == 0 {
		return fmt.Errorf("version cannot be empty")
	}

	versionWithoutBuild := v
	if buildMetadataIdx := strings.Index(v, "+"); buildMetadataIdx >= 0 {
		versionWithoutBuild = v[:buildMetadataIdx]
	}

	parts := strings.SplitN(versionWithoutBuild, ".", 3)
	if len(parts) != 3 {
		return fmt.Errorf("version must have 3 parts (MAJOR.MINOR.PATCH)")
	}

	for partIdx, part := range parts {
		numericPart := part
		if partIdx == 2 {
			if prereleaseIdx := strings.Index(part, "-"); prereleaseIdx >= 0 {
				numericPart = part[:prereleaseIdx]
			}
		}

		if len(numericPart) == 0 {
			return fmt.Errorf("version part %d is empty", partIdx+1)
		}
		for _, char := range numericPart {
			if char < '0' || char > '9' {
				return fmt.Errorf("version part %d contains non-numeric characters", partIdx+1)
			}
		}
	}

	return nil
}

// validateDependencyConstraint validates a dependency constraint.
// Formats: "name@^1.2.3", "name@~1.2.3", "name@>=1.0.0", "name@1.2.3", "name"
func validateDependencyConstraint(s string) error {
	if s == "" {
		return fmt.Errorf("empty dependency")
	}
	_, constraint, err := ParseDependency(s)
	if err != nil {
		return err
	}
	if constraint != "*" && !isValidConstraint(constraint) {
		return fmt.Errorf("invalid version constraint: %s", constraint)
	}
	return nil
}

// isValidConstraint checks if a constraint syntax is valid.
func isValidConstraint(c string) bool {
	// Simplified validation - production would use hashicorp/go-version
	if len(c) == 0 {
		return false
	}
	// Accept: ^1.2.3, ~1.2.3, >=1.0.0, >1.0.0, <=1.0.0, <1.0.0, 1.2.3, *
	validOps := []string{"^", "~", ">=", ">", "<=", "<"}
	for _, op := range validOps {
		if strings.HasPrefix(c, op) {
			return true
		}
	}
	return true // Default to accepting any format for MVP
}
