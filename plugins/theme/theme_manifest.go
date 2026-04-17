package theme

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// SupportedEngines defines the rendering engines supported by the theme plugin.
var SupportedEngines = map[string]bool{
	"gotemplate": true,
	"lua":        true,
	"react":      true,
}

// ThemeManifest represents the manifest for an individual theme (theme.yaml).
// This is distinct from the plugin manifest — each installed theme has its own
// theme.yaml defining metadata and rendering engine configuration.
type ThemeManifest struct {
	Name          string                 `yaml:"name" json:"name"`
	Version       string                 `yaml:"version" json:"version"`
	Author        string                 `yaml:"author" json:"author"`
	Description   string                 `yaml:"description" json:"description"`
	Engine        string                 `yaml:"engine" json:"engine"` // "gotemplate", "lua", "react"
	ArouteVersion string                 `yaml:"aroute_version" json:"aroute_version"`
	Settings      map[string]interface{} `yaml:"settings" json:"settings"`
}

// Validate checks that the theme manifest has all required fields.
func (m *ThemeManifest) Validate() error {
	if m.Name == "" {
		return fmt.Errorf("theme manifest: name is required")
	}
	if m.Engine == "" {
		return fmt.Errorf("theme manifest: engine is required")
	}
	if !SupportedEngines[m.Engine] {
		return fmt.Errorf("theme manifest: unsupported engine %q (supported: gotemplate, lua, react)", m.Engine)
	}
	return nil
}

// LoadThemeManifest loads and validates a ThemeManifest from a theme.yaml file.
func LoadThemeManifest(path string) (*ThemeManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read theme manifest: %w", err)
	}

	var m ThemeManifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse theme manifest: %w", err)
	}

	if err := m.Validate(); err != nil {
		return nil, err
	}

	return &m, nil
}
