package core

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func TestPluginState_String(t *testing.T) {
	tests := []struct {
		state    PluginState
		expected string
	}{
		{StateRegistered, "registered"},
		{StateResolved, "resolved"},
		{StateStarting, "starting"},
		{StateActive, "active"},
		{StateStopping, "stopping"},
		{StateStopped, "stopped"},
		{StateFailed, "failed"},
		{PluginState(999), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.state.String(); got != tt.expected {
				t.Errorf("PluginState.String() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestBasePlugin(t *testing.T) {
	plugin := NewBasePlugin("test-plugin", "1.0.0")

	if got := plugin.Name(); got != "test-plugin" {
		t.Errorf("Name() = %v, want %v", got, "test-plugin")
	}

	if got := plugin.Version(); got != "1.0.0" {
		t.Errorf("Version() = %v, want %v", got, "1.0.0")
	}

	ctx := context.Background()
	coreCtx := NewCoreContext(ctx, nil, nil, nil, nil, "/data", "/plugins")

	if err := plugin.Init(coreCtx); err != nil {
		t.Errorf("Init() error = %v, want nil", err)
	}

	if err := plugin.Start(); err != nil {
		t.Errorf("Start() error = %v, want nil", err)
	}

	if err := plugin.Stop(); err != nil {
		t.Errorf("Stop() error = %v, want nil", err)
	}
}

func TestManifest_Validate(t *testing.T) {
	tests := []struct {
		name     string
		manifest *Manifest
		wantErr  bool
	}{
		{
			name: "valid manifest",
			manifest: &Manifest{
				Name:    "test-plugin",
				Version: "1.0.0",
				Engine:  "native",
				License: "MIT",
			},
			wantErr: false,
		},
		{
			name: "missing name",
			manifest: &Manifest{
				Version: "1.0.0",
				Engine:  "native",
			},
			wantErr: true,
		},
		{
			name: "missing version",
			manifest: &Manifest{
				Name:   "test-plugin",
				Engine: "native",
			},
			wantErr: true,
		},
		{
			name: "invalid version format",
			manifest: &Manifest{
				Name:    "test-plugin",
				Version: "invalid",
				Engine:  "native",
			},
			wantErr: true,
		},
		{
			name: "invalid engine type",
			manifest: &Manifest{
				Name:    "test-plugin",
				Version: "1.0.0",
				Engine:  "invalid",
			},
			wantErr: true,
		},
		{
			name: "valid with dependencies",
			manifest: &Manifest{
				Name:     "test-plugin",
				Version:  "1.0.0",
				Engine:   "native",
				Requires: []string{"http@^1.0.0", "database@~1.2.3"},
				After:    []string{"auth"},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.manifest.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Manifest.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestLoadManifest_YAML(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name    string
		content string
		wantErr bool
	}{
		{
			name: "valid yaml manifest",
			content: `name: test-plugin
version: 1.0.0
description: A test plugin
author: Test Author
license: MIT
engine: native
requires:
  - http@^1.0.0
provides:
  - test-service
`,
			wantErr: false,
		},
		{
			name: "invalid yaml syntax",
			content: `name: test-plugin
version: [invalid
`,
			wantErr: true,
		},
		{
			name: "missing required field",
			content: `description: Missing name and version
`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manifestPath := filepath.Join(tmpDir, "manifest.yaml")
			if err := os.WriteFile(manifestPath, []byte(tt.content), 0644); err != nil {
				t.Fatalf("Failed to write manifest file: %v", err)
			}

			manifest, err := LoadManifest(manifestPath)
			if (err != nil) != tt.wantErr {
				t.Errorf("LoadManifest() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && manifest != nil {
				if manifest.Name != "test-plugin" && !tt.wantErr {
					t.Errorf("LoadManifest() name = %v, want test-plugin", manifest.Name)
				}
			}
		})
	}
}

func TestLoadManifest_JSON(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name    string
		content string
		wantErr bool
	}{
		{
			name:    "valid json manifest",
			content: `{"name":"test-plugin","version":"1.0.0","description":"A test plugin","author":"Test Author","license":"MIT","engine":"native"}`,
			wantErr: false,
		},
		{
			name:    "invalid json syntax",
			content: `{"name":"test-plugin","version":invalid}`,
			wantErr: true,
		},
		{
			name:    "missing required field",
			content: `{"description":"Missing name and version"}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manifestPath := filepath.Join(tmpDir, "manifest.json")
			if err := os.WriteFile(manifestPath, []byte(tt.content), 0644); err != nil {
				t.Fatalf("Failed to write manifest file: %v", err)
			}

			_, err := LoadManifest(manifestPath)
			if (err != nil) != tt.wantErr {
				t.Errorf("LoadManifest() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestParseDependency(t *testing.T) {
	tests := []struct {
		input          string
		wantName       string
		wantConstraint string
		wantErr        bool
	}{
		{"http", "http", "*", false},
		{"http@^1.0.0", "http", "^1.0.0", false},
		{"database@~2.3.4", "database", "~2.3.4", false},
		{"plugin@>=1.0.0", "plugin", ">=1.0.0", false},
		{"plugin@", "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			name, constraint, err := ParseDependency(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseDependency() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if name != tt.wantName {
					t.Errorf("ParseDependency() name = %v, want %v", name, tt.wantName)
				}
				if constraint != tt.wantConstraint {
					t.Errorf("ParseDependency() constraint = %v, want %v", constraint, tt.wantConstraint)
				}
			}
		})
	}
}

func TestValidateSemver(t *testing.T) {
	tests := []struct {
		version string
		wantErr bool
	}{
		{"1.0.0", false},
		{"2.3.4", false},
		{"1.0.0-beta.1", false},
		{"1.0", true},
		{"1", true},
		{"invalid", true},
		{"1.0.0.0", true},
	}

	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			err := validateSemver(tt.version)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateSemver() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestCoreContext(t *testing.T) {
	ctx := context.Background()
	coreCtx := NewCoreContext(ctx, nil, nil, nil, nil, "/data", "/plugins")

	if coreCtx.Context() != ctx {
		t.Errorf("Context() returned wrong context")
	}

	if coreCtx.DataDir() != "/data" {
		t.Errorf("DataDir() = %v, want /data", coreCtx.DataDir())
	}

	if coreCtx.PluginDir() != "/plugins" {
		t.Errorf("PluginDir() = %v, want /plugins", coreCtx.PluginDir())
	}
}

func TestEngineType_String(t *testing.T) {
	tests := []struct {
		engine EngineType
		want   string
	}{
		{EngineL1Native, "native"},
		{EngineL2GRPC, "grpc"},
		{EngineL3Wasm, "wasm"},
		{EngineType(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.engine.String(); got != tt.want {
				t.Errorf("EngineType(%d).String() = %q, want %q", tt.engine, got, tt.want)
			}
		})
	}
}

func TestParseEngine(t *testing.T) {
	tests := []struct {
		input   string
		want    EngineType
		wantErr bool
	}{
		{"native", EngineL1Native, false},
		{"l1", EngineL1Native, false},
		{"grpc", EngineL2GRPC, false},
		{"l2", EngineL2GRPC, false},
		{"wasm", EngineL3Wasm, false},
		{"l3", EngineL3Wasm, false},
		{"invalid", EngineType(0), true},
		{"", EngineType(0), true},
		{"WASM", EngineType(0), true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseEngine(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseEngine(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("ParseEngine(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestCoreContext_Logger(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	coreCtx := NewCoreContext(ctx, nil, nil, nil, logger, "/data", "/plugins")

	if coreCtx.Logger() != logger {
		t.Error("Logger() should return the provided logger")
	}
}

func TestCoreContext_NilLogger(t *testing.T) {
	ctx := context.Background()
	coreCtx := NewCoreContext(ctx, nil, nil, nil, nil, "/data", "/plugins")

	if coreCtx.Logger() != nil {
		t.Error("Logger() should return nil when nil is provided")
	}
}

func TestValidateSemver_AdditionalCases(t *testing.T) {
	tests := []struct {
		version string
		wantErr bool
	}{
		{"", true},
		{"1.0.0+build", false},
		{"1.0.0-alpha+build", false},
		{"1.0.0-beta.1", false},
		{"1.0.0-rc.1", false},
		{"v1.0.0", true},
	}

	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			err := validateSemver(tt.version)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateSemver(%q) error = %v, wantErr %v", tt.version, err, tt.wantErr)
			}
		})
	}
}

func TestValidateDependencyConstraint(t *testing.T) {
	tests := []struct {
		input   string
		wantErr bool
	}{
		{"http@^1.0.0", false},
		{"http@~1.2.3", false},
		{"http@>=1.0.0", false},
		{"http@1.2.3", false},
		{"http", false},
		{"", true},
		{"http@", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			err := validateDependencyConstraint(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateDependencyConstraint(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestIsValidConstraint(t *testing.T) {
	tests := []struct {
		constraint string
		want       bool
	}{
		{"^1.2.3", true},
		{"~1.2.3", true},
		{">=1.0.0", true},
		{">1.0.0", true},
		{"<=1.0.0", true},
		{"<1.0.0", true},
		{"1.2.3", true},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.constraint, func(t *testing.T) {
			if got := isValidConstraint(tt.constraint); got != tt.want {
				t.Errorf("isValidConstraint(%q) = %v, want %v", tt.constraint, got, tt.want)
			}
		})
	}
}

func TestManifest_Validate_InvalidName(t *testing.T) {
	manifest := &Manifest{
		Name:    "INVALID",
		Version: "1.0.0",
		Engine:  "native",
	}
	if err := manifest.Validate(); err == nil {
		t.Error("Validate() should fail for uppercase name")
	}
}

func TestManifest_Validate_DuplicateProvides(t *testing.T) {
	manifest := &Manifest{
		Name:     "test-plugin",
		Version:  "1.0.0",
		Engine:   "native",
		Provides: []string{"http", "http"},
	}
	if err := manifest.Validate(); err == nil {
		t.Error("Validate() should fail for duplicate provides")
	}
}

func TestManifest_Provides(t *testing.T) {
	manifest := &Manifest{
		Name:     "test-plugin",
		Version:  "1.0.0",
		Engine:   "native",
		Provides: []string{"http", "router", "middleware"},
	}

	if len(manifest.Provides) != 3 {
		t.Errorf("len(Provides) = %v, want 3", len(manifest.Provides))
	}

	expected := map[string]bool{
		"http":       false,
		"router":     false,
		"middleware": false,
	}

	for _, svc := range manifest.Provides {
		if _, ok := expected[svc]; !ok {
			t.Errorf("Unexpected service in Provides: %v", svc)
		}
		expected[svc] = true
	}

	for svc, found := range expected {
		if !found {
			t.Errorf("Missing service in Provides: %v", svc)
		}
	}
}
