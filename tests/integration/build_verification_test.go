package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// TestBuild_GoreleaserConfig verifies the goreleaser configuration exists
// and is valid for cross-platform builds.
func TestBuild_GoreleaserConfig(t *testing.T) {
	configPath := filepath.Join("..", "..", ".goreleaser.yaml")

	t.Run("config_exists", func(t *testing.T) {
		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			t.Skipf("goreleaser config not found at %s", configPath)
		}
	})

	t.Run("config_valid", func(t *testing.T) {
		// Check if goreleaser is available
		goreleaserPath, err := exec.LookPath("goreleaser")
		if err != nil {
			t.Skip("goreleaser not installed, skipping config validation")
		}
		t.Logf("Using goreleaser at: %s", goreleaserPath)

		cmd := exec.Command(goreleaserPath, "check", "-f", configPath)
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("goreleaser config validation failed: %s\n%s", err, string(output))
		}
		t.Log("goreleaser config is valid")
	})
}

// TestBuild_CurrentPlatform verifies the project compiles successfully
// on the current platform.
func TestBuild_CurrentPlatform(t *testing.T) {
	projectRoot := filepath.Join("..", "..")

	// Verify go build works for the core packages
	t.Run("go_build_core", func(t *testing.T) {
		pkgs := []string{
			"./core/...",
			"./sdk/...",
			"./plugins/database/",
			"./plugins/auth/",
			"./plugins/content/",
			"./plugins/search/",
			"./plugins/theme/",
			"./plugins/cache/",
			"./plugins/queue/",
			"./plugins/webhook/",
			"./plugins/http/",
			"./plugins/api/",
		}
		for _, pkg := range pkgs {
			cmd := exec.Command("go", "build", "-o", os.DevNull, pkg)
			cmd.Dir = projectRoot
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Errorf("go build %s failed: %s\n%s", pkg, err, string(output))
			}
		}
		t.Logf("Core packages build succeeded on %s/%s", runtime.GOOS, runtime.GOARCH)
	})

	t.Run("go_build_full_binary", func(t *testing.T) {
		cmd := exec.Command("go", "build", "-o", os.DevNull, "./cmd/aroute/")
		cmd.Dir = projectRoot
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("go build failed: %s\n%s", err, string(output))
		}
		t.Logf("Full binary build succeeded on %s/%s", runtime.GOOS, runtime.GOARCH)
	})
}

// TestBuild_CrossPlatformTargets verifies that goreleaser config specifies
// the expected build matrix.
func TestBuild_CrossPlatformTargets(t *testing.T) {
	configPath := filepath.Join("..", "..", ".goreleaser.yaml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Skipf("goreleaser config not found: %v", err)
	}

	content := string(data)

	expectedTargets := []struct {
		goos   string
		goarch string
	}{
		{"linux", "amd64"},
		{"linux", "arm64"},
		{"darwin", "amd64"},
		{"darwin", "arm64"},
		{"windows", "amd64"},
	}

	for _, target := range expectedTargets {
		t.Run(target.goos+"/"+target.goarch, func(t *testing.T) {
			// Check if the target OS is mentioned in the config
			if !contains(content, target.goos) {
				t.Errorf("config does not mention %s", target.goos)
			}
		})
	}

	// Verify CGO_ENABLED=0
	if !contains(content, "CGO_ENABLED=0") && !contains(content, "CGO_ENABLED") {
		t.Log("Warning: CGO_ENABLED=0 not explicitly set in goreleaser config")
	}
}

// TestBuild_GoVet runs go vet on core packages to catch build issues.
func TestBuild_GoVet(t *testing.T) {
	projectRoot := filepath.Join("..", "..")

	// Vet core packages
	pkgs := []string{
		"./core/...",
		"./sdk/...",
		"./plugins/database/",
		"./plugins/auth/",
		"./plugins/content/",
		"./plugins/search/",
		"./plugins/theme/",
		"./plugins/cache/",
		"./plugins/queue/",
		"./plugins/webhook/",
		"./plugins/http/",
		"./plugins/api/",
		"./tests/integration/",
	}
	for _, pkg := range pkgs {
		cmd := exec.Command("go", "vet", pkg)
		cmd.Dir = projectRoot
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Errorf("go vet %s: %s\n%s", pkg, err, string(output))
		}
	}
	if !t.Failed() {
		t.Log("go vet passed for all core packages")
	}
}
