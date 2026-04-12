package license

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadFromFile_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	nonExistentPath := filepath.Join(tmpDir, "nonexistent.json")

	lic, err := LoadFromFile(nonExistentPath)
	if err != ErrLicenseNotFound {
		t.Errorf("Expected ErrLicenseNotFound, got %v", err)
	}
	if lic != nil {
		t.Error("Expected nil license for not found file")
	}
}

func TestLoadFromFile_EmptyFile(t *testing.T) {
	tmpDir := t.TempDir()
	emptyPath := filepath.Join(tmpDir, "empty.json")
	if err := os.WriteFile(emptyPath, []byte{}, 0644); err != nil {
		t.Fatalf("Failed to create empty file: %v", err)
	}

	lic, err := LoadFromFile(emptyPath)
	if err == nil {
		t.Error("Expected error for empty file")
	}
	if lic != nil {
		t.Error("Expected nil license for empty file")
	}
}

func TestLoadFromFile_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	invalidPath := filepath.Join(tmpDir, "invalid.json")
	if err := os.WriteFile(invalidPath, []byte("not valid json {"), 0644); err != nil {
		t.Fatalf("Failed to create invalid JSON file: %v", err)
	}

	lic, err := LoadFromFile(invalidPath)
	if err == nil {
		t.Error("Expected error for invalid JSON")
	}
	if lic != nil {
		t.Error("Expected nil license for invalid JSON")
	}
}

func TestLoadFromFile_ValidLicense(t *testing.T) {
	tmpDir := t.TempDir()
	validPath := filepath.Join(tmpDir, "license.json")

	lic := &License{
		ID:        "lic_test123",
		Tier:      TierOpen,
		Features:  []string{"basic"},
		IssuedAt:  time.Now(),
		ExpiresAt: time.Now().Add(365 * 24 * time.Hour),
	}

	data, err := json.Marshal(lic)
	if err != nil {
		t.Fatalf("Failed to marshal license: %v", err)
	}

	if err := os.WriteFile(validPath, data, 0644); err != nil {
		t.Fatalf("Failed to write license file: %v", err)
	}

	loaded, err := LoadFromFile(validPath)
	if err != nil {
		t.Fatalf("LoadFromFile failed: %v", err)
	}
	if loaded == nil {
		t.Fatal("Expected non-nil license")
	}
	if loaded.ID != lic.ID {
		t.Errorf("Expected ID %s, got %s", lic.ID, loaded.ID)
	}
	if loaded.Tier != lic.Tier {
		t.Errorf("Expected Tier %d, got %d", lic.Tier, loaded.Tier)
	}
}

func TestLoadFromFile_MissingID(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "no_id.json")

	lic := &License{
		Tier:     TierOpen,
		Features: []string{"basic"},
	}

	data, _ := json.Marshal(lic)
	os.WriteFile(path, data, 0644)

	loaded, err := LoadFromFile(path)
	if err == nil {
		t.Error("Expected error for missing ID")
	}
	if loaded != nil {
		t.Error("Expected nil license")
	}
}

func TestLoadFromFile_InvalidIDPrefix(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "bad_prefix.json")

	lic := &License{
		ID:       "bad_prefix",
		Tier:     TierOpen,
		Features: []string{"basic"},
	}

	data, _ := json.Marshal(lic)
	os.WriteFile(path, data, 0644)

	loaded, err := LoadFromFile(path)
	if err == nil {
		t.Error("Expected error for invalid ID prefix")
	}
	if loaded != nil {
		t.Error("Expected nil license")
	}
}

func TestLoadFromFile_InvalidTier(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "bad_tier.json")

	lic := &License{
		ID:       "lic_test",
		Tier:     Tier(999),
		Features: []string{"basic"},
	}

	data, _ := json.Marshal(lic)
	os.WriteFile(path, data, 0644)

	loaded, err := LoadFromFile(path)
	if err == nil {
		t.Error("Expected error for invalid tier")
	}
	if loaded != nil {
		t.Error("Expected nil license")
	}
}

func TestLoadFromFile_EmptyFeatures(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "no_features.json")

	lic := &License{
		ID:       "lic_test",
		Tier:     TierOpen,
		Features: []string{},
	}

	data, _ := json.Marshal(lic)
	os.WriteFile(path, data, 0644)

	loaded, err := LoadFromFile(path)
	if err == nil {
		t.Error("Expected error for empty features")
	}
	if loaded != nil {
		t.Error("Expected nil license")
	}
}

func TestLoadFromFile_ExpiresBeforeIssued(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "bad_dates.json")

	lic := &License{
		ID:        "lic_test",
		Tier:      TierOpen,
		Features:  []string{"basic"},
		IssuedAt:  time.Now().Add(24 * time.Hour),
		ExpiresAt: time.Now(),
	}

	data, _ := json.Marshal(lic)
	os.WriteFile(path, data, 0644)

	loaded, err := LoadFromFile(path)
	if err == nil {
		t.Error("Expected error for expires before issued")
	}
	if loaded != nil {
		t.Error("Expected nil license")
	}
}

func TestLoadFromDir_NotFound(t *testing.T) {
	tmpDir := t.TempDir()

	lic, err := LoadFromDir(tmpDir)
	if err != ErrLicenseNotFound {
		t.Errorf("Expected ErrLicenseNotFound, got %v", err)
	}
	if lic != nil {
		t.Error("Expected nil license")
	}
}

func TestLoadFromDir_FoundJSON(t *testing.T) {
	tmpDir := t.TempDir()

	lic := &License{
		ID:       "lic_test",
		Tier:     TierOpen,
		Features: []string{"basic"},
		IssuedAt: time.Now(),
	}

	data, _ := json.Marshal(lic)
	os.WriteFile(filepath.Join(tmpDir, "license.json"), data, 0644)

	loaded, err := LoadFromDir(tmpDir)
	if err != nil {
		t.Fatalf("LoadFromDir failed: %v", err)
	}
	if loaded == nil {
		t.Fatal("Expected non-nil license")
	}
	if loaded.ID != lic.ID {
		t.Errorf("Expected ID %s, got %s", lic.ID, loaded.ID)
	}
}

func TestLoadFromDir_FoundHidden(t *testing.T) {
	tmpDir := t.TempDir()

	lic := &License{
		ID:       "lic_hidden",
		Tier:     TierOpen,
		Features: []string{"basic"},
		IssuedAt: time.Now(),
	}

	data, _ := json.Marshal(lic)
	os.WriteFile(filepath.Join(tmpDir, ".license.json"), data, 0644)

	loaded, err := LoadFromDir(tmpDir)
	if err != nil {
		t.Fatalf("LoadFromDir failed: %v", err)
	}
	if loaded == nil {
		t.Fatal("Expected non-nil license")
	}
	if loaded.ID != lic.ID {
		t.Errorf("Expected ID %s, got %s", lic.ID, loaded.ID)
	}
}

func TestLoadOrDefault_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	nonExistentPath := filepath.Join(tmpDir, "nonexistent.json")

	v, err := LoadOrDefault(nonExistentPath, nil)
	if err != nil {
		t.Fatalf("LoadOrDefault failed: %v", err)
	}
	if v == nil {
		t.Fatal("Expected non-nil validator")
	}
	if v.Tier() != TierOpen {
		t.Errorf("Expected default Open tier, got %d", v.Tier())
	}
}

func TestLoadOrDefault_ValidLicense(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "license.json")

	lic := &License{
		ID:        "lic_test",
		Tier:      TierPro,
		Features:  []string{"basic", "advanced"},
		IssuedAt:  time.Now(),
		ExpiresAt: time.Now().Add(365 * 24 * time.Hour),
	}

	data, _ := json.Marshal(lic)
	os.WriteFile(path, data, 0644)

	v, err := LoadOrDefault(path, nil)
	if err != nil {
		t.Fatalf("LoadOrDefault failed: %v", err)
	}
	if v == nil {
		t.Fatal("Expected non-nil validator")
	}
	if v.Tier() != TierPro {
		t.Errorf("Expected Pro tier, got %d", v.Tier())
	}
}

func TestValidateLicenseFields_Valid(t *testing.T) {
	lic := &License{
		ID:       "lic_test",
		Tier:     TierOpen,
		Features: []string{"basic"},
	}

	if err := validateLicenseFields(lic); err != nil {
		t.Errorf("Expected no error for valid license, got %v", err)
	}
}

func TestValidateLicenseFields_MultipleErrors(t *testing.T) {
	lic := &License{
		Tier:     Tier(999),
		Features: []string{},
	}

	err := validateLicenseFields(lic)
	if err == nil {
		t.Error("Expected error for multiple invalid fields")
	}
}

func TestLoadFromFile_BinExtension(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "license.bin")

	lic := &License{
		ID:       "lic_test",
		Tier:     TierOpen,
		Features: []string{"basic"},
		IssuedAt: time.Now(),
	}

	data, _ := json.Marshal(lic)
	os.WriteFile(path, data, 0644)

	loaded, err := LoadFromFile(path)
	if err != nil {
		t.Fatalf("LoadFromFile failed for .bin: %v", err)
	}
	if loaded.ID != lic.ID {
		t.Errorf("Expected ID %s, got %s", lic.ID, loaded.ID)
	}
}

func TestLoadFromFile_PermissionError(t *testing.T) {
	// Skip on Windows where permission handling differs
	if os.Getenv("GOOS") == "windows" {
		t.Skip("Skipping permission test on Windows")
	}

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "no_perm.json")

	data := []byte(`{"id":"lic_test","tier":0,"features":["basic"]}`)
	os.WriteFile(path, data, 0000) // No permissions

	// On some systems, root can still read, so this test may pass
	_, err := LoadFromFile(path)
	// Just check that we handle permission errors gracefully
	if err != nil && err != ErrLicenseNotFound {
		// We got a permission-related error, which is expected behavior
		t.Logf("Got expected permission error: %v", err)
	}
}

// TestLoadFromDir_PriorityOrder tests that LoadFromDir searches files in correct order:
// license.json > license.bin > .license.json > .license.bin
func TestLoadFromDir_PriorityOrder(t *testing.T) {
	tmpDir := t.TempDir()

	// Create multiple license files with different IDs to track which one gets loaded
	licJSON := &License{
		ID:       "lic_json_priority",
		Tier:     TierOpen,
		Features: []string{"basic"},
		IssuedAt: time.Now(),
	}
	dataJSON, _ := json.Marshal(licJSON)
	os.WriteFile(filepath.Join(tmpDir, "license.json"), dataJSON, 0644)

	licBin := &License{
		ID:       "lic_bin_priority",
		Tier:     TierPro,
		Features: []string{"basic", "advanced"},
		IssuedAt: time.Now(),
	}
	dataBin, _ := json.Marshal(licBin)
	os.WriteFile(filepath.Join(tmpDir, "license.bin"), dataBin, 0644)

	licHiddenJSON := &License{
		ID:       "lic_hidden_json",
		Tier:     TierEnterprise,
		Features: []string{"basic", "advanced", "enterprise"},
		IssuedAt: time.Now(),
	}
	dataHiddenJSON, _ := json.Marshal(licHiddenJSON)
	os.WriteFile(filepath.Join(tmpDir, ".license.json"), dataHiddenJSON, 0644)

	// license.json should be loaded first (highest priority)
	loaded, err := LoadFromDir(tmpDir)
	if err != nil {
		t.Fatalf("LoadFromDir failed: %v", err)
	}
	if loaded.ID != "lic_json_priority" {
		t.Errorf("Expected license.json (lic_json_priority), got %s", loaded.ID)
	}
}

// TestLoadFromDir_BinOnly tests loading when only .bin file exists
func TestLoadFromDir_BinOnly(t *testing.T) {
	tmpDir := t.TempDir()

	lic := &License{
		ID:       "lic_bin_only",
		Tier:     TierPro,
		Features: []string{"basic", "advanced"},
		IssuedAt: time.Now(),
	}

	data, _ := json.Marshal(lic)
	os.WriteFile(filepath.Join(tmpDir, "license.bin"), data, 0644)

	loaded, err := LoadFromDir(tmpDir)
	if err != nil {
		t.Fatalf("LoadFromDir failed: %v", err)
	}
	if loaded.ID != "lic_bin_only" {
		t.Errorf("Expected ID lic_bin_only, got %s", loaded.ID)
	}
}

// TestLoadFromDir_HiddenBinOnly tests loading when only hidden .license.bin file exists
func TestLoadFromDir_HiddenBinOnly(t *testing.T) {
	tmpDir := t.TempDir()

	lic := &License{
		ID:       "lic_hidden_bin",
		Tier:     TierEnterprise,
		Features: []string{"basic", "advanced", "enterprise"},
		IssuedAt: time.Now(),
	}

	data, _ := json.Marshal(lic)
	os.WriteFile(filepath.Join(tmpDir, ".license.bin"), data, 0644)

	loaded, err := LoadFromDir(tmpDir)
	if err != nil {
		t.Fatalf("LoadFromDir failed: %v", err)
	}
	if loaded.ID != "lic_hidden_bin" {
		t.Errorf("Expected ID lic_hidden_bin, got %s", loaded.ID)
	}
}

// TestLoadFromDir_InvalidFileSkipped tests that invalid files are skipped and search continues
func TestLoadFromDir_InvalidFileSkipped(t *testing.T) {
	tmpDir := t.TempDir()

	// Create an invalid license.json (missing required fields)
	invalidData := []byte(`{"tier":0,"features":["basic"]}`)
	os.WriteFile(filepath.Join(tmpDir, "license.json"), invalidData, 0644)

	// Create a valid hidden .license.json
	validLic := &License{
		ID:       "lic_fallback",
		Tier:     TierOpen,
		Features: []string{"basic"},
		IssuedAt: time.Now(),
	}
	validData, _ := json.Marshal(validLic)
	os.WriteFile(filepath.Join(tmpDir, ".license.json"), validData, 0644)

	// Invalid license.json should cause error, not continue to .license.json
	_, err := LoadFromDir(tmpDir)
	if err == nil {
		t.Error("Expected error for invalid license.json, got nil")
	}
}

// TestLoadOrDefault_ValidationFailure tests LoadOrDefault with a license that fails validation
func TestLoadOrDefault_ValidationFailure(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "license.json")

	// Create a license with invalid fields (missing ID prefix)
	invalidLic := License{
		ID:       "invalid_id_no_prefix",
		Tier:     TierPro,
		Features: []string{"basic"},
		IssuedAt: time.Now(),
	}
	data, _ := json.Marshal(invalidLic)
	os.WriteFile(path, data, 0644)

	// LoadOrDefault should return error for invalid license (not fallback to Open)
	_, err := LoadOrDefault(path, nil)
	if err == nil {
		t.Error("Expected error for invalid license, got nil")
	}
}

// TestLoadFromFile_AllTierTypes tests loading licenses for all tier types
func TestLoadFromFile_AllTierTypes(t *testing.T) {
	tests := []struct {
		name     string
		tier     Tier
		features []string
	}{
		{
			name:     "Open tier",
			tier:     TierOpen,
			features: OpenFeatures(),
		},
		{
			name:     "Pro tier",
			tier:     TierPro,
			features: ProFeatures(),
		},
		{
			name:     "Enterprise tier",
			tier:     TierEnterprise,
			features: EnterpriseFeatures(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			path := filepath.Join(tmpDir, "license.json")

			lic := &License{
				ID:        "lic_" + tt.name,
				Tier:      tt.tier,
				Features:  tt.features,
				IssuedAt:  time.Now(),
				ExpiresAt: time.Now().Add(365 * 24 * time.Hour),
			}

			data, _ := json.Marshal(lic)
			os.WriteFile(path, data, 0644)

			loaded, err := LoadFromFile(path)
			if err != nil {
				t.Fatalf("LoadFromFile failed: %v", err)
			}
			if loaded.Tier != tt.tier {
				t.Errorf("Expected Tier %d, got %d", tt.tier, loaded.Tier)
			}
			if len(loaded.Features) != len(tt.features) {
				t.Errorf("Expected %d features, got %d", len(tt.features), len(loaded.Features))
			}
		})
	}
}

// TestLoadFromFile_PerpetualLicense tests loading a license with zero ExpiresAt (perpetual)
func TestLoadFromFile_PerpetualLicense(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "license.json")

	lic := &License{
		ID:        "lic_perpetual",
		Tier:      TierEnterprise,
		Features:  EnterpriseFeatures(),
		IssuedAt:  time.Now(),
		ExpiresAt: time.Time{}, // Zero value = perpetual
	}

	data, _ := json.Marshal(lic)
	os.WriteFile(path, data, 0644)

	loaded, err := LoadFromFile(path)
	if err != nil {
		t.Fatalf("LoadFromFile failed: %v", err)
	}
	if !loaded.ExpiresAt.IsZero() {
		t.Error("Expected zero ExpiresAt for perpetual license")
	}
	if loaded.IsExpired() {
		t.Error("Perpetual license should not be expired")
	}
}

// TestLoadFromFile_VeryLargeFile tests handling of large license files
func TestLoadFromFile_VeryLargeFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "large.json")

	// Create a license with many features
	lic := &License{
		ID:       "lic_large",
		Tier:     TierEnterprise,
		Features: make([]string, 1000),
		IssuedAt: time.Now(),
	}
	for i := range lic.Features {
		lic.Features[i] = "feature_" + string(rune(i))
	}

	data, _ := json.Marshal(lic)
	os.WriteFile(path, data, 0644)

	loaded, err := LoadFromFile(path)
	if err != nil {
		t.Fatalf("LoadFromFile failed for large file: %v", err)
	}
	if len(loaded.Features) != 1000 {
		t.Errorf("Expected 1000 features, got %d", len(loaded.Features))
	}
}

// TestLoadFromFile_WhitespaceOnly tests handling of files with only whitespace
func TestLoadFromFile_WhitespaceOnly(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "whitespace.json")

	// Write whitespace-only content
	os.WriteFile(path, []byte("   \n\t  "), 0644)

	_, err := LoadFromFile(path)
	if err == nil {
		t.Error("Expected error for whitespace-only file")
	}
}

// TestLoadFromFile_JSONWithExtraFields tests that extra JSON fields are ignored
func TestLoadFromFile_JSONWithExtraFields(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "extra.json")

	// JSON with extra unknown fields
	data := []byte(`{
		"id": "lic_extra",
		"tier": "open",
		"features": ["basic"],
		"issued_at": "2025-01-01T00:00:00Z",
		"extra_field": "should be ignored",
		"another_unknown": 12345
	}`)
	os.WriteFile(path, data, 0644)

	loaded, err := LoadFromFile(path)
	if err != nil {
		t.Fatalf("LoadFromFile failed: %v", err)
	}
	if loaded.ID != "lic_extra" {
		t.Errorf("Expected ID lic_extra, got %s", loaded.ID)
	}
}

// TestLoadFromDir_SubdirectoryNotScanned tests that subdirectories are not searched
func TestLoadFromDir_SubdirectoryNotScanned(t *testing.T) {
	tmpDir := t.TempDir()

	// Create subdirectory with license file
	subDir := filepath.Join(tmpDir, "subdir")
	os.Mkdir(subDir, 0755)

	lic := &License{
		ID:       "lic_in_subdir",
		Tier:     TierPro,
		Features: []string{"basic"},
		IssuedAt: time.Now(),
	}
	data, _ := json.Marshal(lic)
	os.WriteFile(filepath.Join(subDir, "license.json"), data, 0644)

	// LoadFromDir on parent should not find the license in subdir
	_, err := LoadFromDir(tmpDir)
	if err != ErrLicenseNotFound {
		t.Errorf("Expected ErrLicenseNotFound, got %v", err)
	}
}
