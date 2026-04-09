package license

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func TestTierString(t *testing.T) {
	tests := []struct {
		tier Tier
		want string
	}{
		{TierOpen, "open"},
		{TierPro, "pro"},
		{TierEnterprise, "enterprise"},
		{Tier(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.tier.String(); got != tt.want {
			t.Errorf("Tier(%d).String() = %q, want %q", tt.tier, got, tt.want)
		}
	}
}

func TestParseTier(t *testing.T) {
	tests := []struct {
		input string
		want  Tier
		err   bool
	}{
		{"open", TierOpen, false},
		{"pro", TierPro, false},
		{"enterprise", TierEnterprise, false},
		{"invalid", TierOpen, true},
		{"", TierOpen, true},
	}
	for _, tt := range tests {
		got, err := ParseTier(tt.input)
		if tt.err {
			if err == nil {
				t.Errorf("ParseTier(%q): expected error, got nil", tt.input)
			}
		} else {
			if err != nil {
				t.Errorf("ParseTier(%q): unexpected error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("ParseTier(%q) = %d, want %d", tt.input, got, tt.want)
			}
		}
	}
}

func TestTierJSON(t *testing.T) {
	tests := []struct {
		tier Tier
		json string
	}{
		{TierOpen, `"open"`},
		{TierPro, `"pro"`},
		{TierEnterprise, `"enterprise"`},
	}
	for _, tt := range tests {
		data, err := json.Marshal(tt.tier)
		if err != nil {
			t.Fatalf("Marshal(%d): %v", tt.tier, err)
		}
		if string(data) != tt.json {
			t.Errorf("Marshal(%d) = %s, want %s", tt.tier, data, tt.json)
		}

		var parsed Tier
		if err := json.Unmarshal(data, &parsed); err != nil {
			t.Fatalf("Unmarshal(%s): %v", tt.json, err)
		}
		if parsed != tt.tier {
			t.Errorf("Unmarshal(%s) = %d, want %d", tt.json, parsed, tt.tier)
		}
	}
}

func TestLicenseIsExpired(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name    string
		license *License
		expired bool
	}{
		{
			name:    "perpetual (zero ExpiresAt)",
			license: &License{ExpiresAt: time.Time{}},
			expired: false,
		},
		{
			name:    "not yet expired",
			license: &License{ExpiresAt: now.Add(24 * time.Hour)},
			expired: false,
		},
		{
			name:    "already expired",
			license: &License{ExpiresAt: now.Add(-24 * time.Hour)},
			expired: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.license.IsExpired(); got != tt.expired {
				t.Errorf("IsExpired() = %v, want %v", got, tt.expired)
			}
		})
	}
}

func TestLicenseHasFeature(t *testing.T) {
	lic := &License{
		Features: []string{"plugin:l1-native", "auth:jwt", "api:rest"},
	}

	tests := []struct {
		feature string
		want    bool
	}{
		{"plugin:l1-native", true},
		{"auth:jwt", true},
		{"api:rest", true},
		{"plugin:l2-grpc", false},
		{"nonexistent", false},
	}

	for _, tt := range tests {
		if got := lic.HasFeature(tt.feature); got != tt.want {
			t.Errorf("HasFeature(%q) = %v, want %v", tt.feature, got, tt.want)
		}
	}
}

func TestECDSASignatureVerification(t *testing.T) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	lic := &License{
		ID:        "lic_test1234567890",
		Tier:      TierPro,
		Features:  ProFeatures(),
		IssuedAt:  time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		ExpiresAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		IssuedTo:  "test@example.com",
	}

	// Sign the license data
	unsignedData, err := lic.unsignedData()
	if err != nil {
		t.Fatalf("unsignedData: %v", err)
	}
	hash := sha256Sum(unsignedData)
	r, s, err := ecdsa.Sign(rand.Reader, privateKey, hash[:])
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}

	sig := make([]byte, 64)
	rBytes := r.Bytes()
	sBytes := s.Bytes()
	copy(sig[32-len(rBytes):32], rBytes)
	copy(sig[64-len(sBytes):64], sBytes)
	lic.Signature = base64.StdEncoding.EncodeToString(sig)

	// Verify with correct public key
	if err := lic.VerifySignature(&privateKey.PublicKey); err != nil {
		t.Errorf("VerifySignature with correct key: %v", err)
	}

	// Verify with wrong public key
	wrongKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err := lic.VerifySignature(&wrongKey.PublicKey); err == nil {
		t.Error("VerifySignature with wrong key: expected error, got nil")
	}

	// Verify with nil public key
	if err := lic.VerifySignature(nil); err == nil {
		t.Error("VerifySignature with nil key: expected error, got nil")
	}
}

func TestTamperDetection(t *testing.T) {
	privateKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)

	lic := &License{
		ID:        "lic_test1234567890",
		Tier:      TierPro,
		Features:  ProFeatures(),
		IssuedAt:  time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		ExpiresAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		IssuedTo:  "test@example.com",
	}

	unsignedData, _ := lic.unsignedData()
	hash := sha256Sum(unsignedData)
	r, s, _ := ecdsa.Sign(rand.Reader, privateKey, hash[:])
	sig := make([]byte, 64)
	rBytes := r.Bytes()
	sBytes := s.Bytes()
	copy(sig[32-len(rBytes):32], rBytes)
	copy(sig[64-len(sBytes):64], sBytes)
	lic.Signature = base64.StdEncoding.EncodeToString(sig)

	// Tamper with the tier
	lic.Tier = TierEnterprise
	if err := lic.VerifySignature(&privateKey.PublicKey); err == nil {
		t.Error("Tampered tier: expected verification to fail, got nil")
	}

	// Restore and verify works
	lic.Tier = TierPro
	if err := lic.VerifySignature(&privateKey.PublicKey); err != nil {
		t.Errorf("Restored license: expected verification to pass, got %v", err)
	}

	// Tamper with IssuedTo
	lic.IssuedTo = "hacker@evil.com"
	if err := lic.VerifySignature(&privateKey.PublicKey); err == nil {
		t.Error("Tampered IssuedTo: expected verification to fail, got nil")
	}
}

func TestDefaultOpenTier(t *testing.T) {
	v := NewValidator(nil, nil)

	if v.Tier() != TierOpen {
		t.Errorf("Default tier = %v, want TierOpen", v.Tier())
	}

	if v.IsExpired() {
		t.Error("Default license should never expire")
	}

	if err := v.Validate(); err != nil {
		t.Errorf("Default Open tier validation: %v", err)
	}

	// All Open features should be available
	openFeatures := OpenFeatures()
	for _, f := range openFeatures {
		if !v.IsFeatureAllowed(f) {
			t.Errorf("Open tier should allow feature %q", f)
		}
	}

	// Pro features should NOT be available
	if v.IsFeatureAllowed("plugin:l2-grpc") {
		t.Error("Open tier should not allow plugin:l2-grpc")
	}
}

func TestValidatorWithProLicense(t *testing.T) {
	lic := &License{
		ID:        "lic_pro1234567890",
		Tier:      TierPro,
		Features:  ProFeatures(),
		IssuedAt:  time.Now().Add(-24 * time.Hour),
		ExpiresAt: time.Now().Add(365 * 24 * time.Hour),
		IssuedTo:  "pro@example.com",
		Signature: "", // No signature for this test
	}

	v := NewValidator(lic, nil)

	if v.Tier() != TierPro {
		t.Errorf("Tier = %v, want TierPro", v.Tier())
	}

	// Pro features should be available
	if !v.IsFeatureAllowed("plugin:l2-grpc") {
		t.Error("Pro tier should allow plugin:l2-grpc")
	}

	// Enterprise features should NOT be available
	if v.IsFeatureAllowed("multi-site") {
		t.Error("Pro tier should not allow multi-site")
	}

	// Open features should still be available
	if !v.IsFeatureAllowed("auth:jwt") {
		t.Error("Pro tier should allow auth:jwt (Open feature)")
	}
}

func TestValidatorExpired(t *testing.T) {
	lic := &License{
		ID:        "lic_expired12345678",
		Tier:      TierPro,
		Features:  ProFeatures(),
		IssuedAt:  time.Now().Add(-48 * time.Hour),
		ExpiresAt: time.Now().Add(-24 * time.Hour),
		IssuedTo:  "expired@example.com",
		Signature: "",
	}

	v := NewValidator(lic, nil)

	if !v.IsExpired() {
		t.Error("Expired license should report as expired via IsExpired()")
	}

	// Per spec: expired license falls back to Open tier on Validate()
	if err := v.Validate(); err != nil {
		t.Errorf("Validate() should succeed with fallback, got error: %v", err)
	}

	if v.Tier() != TierOpen {
		t.Errorf("After Validate(), expired license should fall back to Open tier, got %v", v.Tier())
	}

	// After fallback, only Open features should be available
	if v.IsFeatureAllowed("plugin:l2-grpc") {
		t.Error("After expiry fallback, Pro features should not be available")
	}

	if !v.IsFeatureAllowed("auth:jwt") {
		t.Error("After expiry fallback, Open features should still be available")
	}
}

func TestFeatureTier(t *testing.T) {
	tests := []struct {
		feature string
		want    Tier
	}{
		// Open features
		{"auth:jwt", TierOpen},
		{"plugin:l1-native", TierOpen},
		{"content:crud", TierOpen},

		// Pro features
		{"plugin:l2-grpc", TierPro},
		{"theme:react-ssr", TierPro},

		// Enterprise features
		{"multi-site", TierEnterprise},
		{"auth:sso", TierEnterprise},

		// Unknown features default to Open
		{"unknown:feature", TierOpen},
	}

	for _, tt := range tests {
		if got := FeatureTier(tt.feature); got != tt.want {
			t.Errorf("FeatureTier(%q) = %v, want %v", tt.feature, got, tt.want)
		}
	}
}

func TestLoadFromFile(t *testing.T) {
	tmpDir := t.TempDir()

	lic := &License{
		ID:        "lic_file123456789",
		Tier:      TierPro,
		Features:  ProFeatures(),
		IssuedAt:  time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		ExpiresAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		IssuedTo:  "file@example.com",
		Signature: "dGVzdA==", // placeholder
	}

	data, _ := json.MarshalIndent(lic, "", "  ")
	jsonPath := filepath.Join(tmpDir, "license.json")
	os.WriteFile(jsonPath, data, 0644)

	loaded, err := LoadFromFile(jsonPath)
	if err != nil {
		t.Fatalf("LoadFromFile: %v", err)
	}

	if loaded.ID != lic.ID {
		t.Errorf("ID = %q, want %q", loaded.ID, lic.ID)
	}
	if loaded.Tier != lic.Tier {
		t.Errorf("Tier = %v, want %v", loaded.Tier, lic.Tier)
	}
	if loaded.IssuedTo != lic.IssuedTo {
		t.Errorf("IssuedTo = %q, want %q", loaded.IssuedTo, lic.IssuedTo)
	}

	// Non-existent file
	_, err = LoadFromFile(filepath.Join(tmpDir, "nonexistent.json"))
	if err != ErrLicenseNotFound {
		t.Errorf("LoadFromFile nonexistent: got %v, want ErrLicenseNotFound", err)
	}
}

func TestLoadFromDir(t *testing.T) {
	tmpDir := t.TempDir()

	// No license files → ErrLicenseNotFound
	_, err := LoadFromDir(tmpDir)
	if err != ErrLicenseNotFound {
		t.Errorf("LoadFromDir empty: got %v, want ErrLicenseNotFound", err)
	}

	// Create license.json
	lic := &License{
		ID:        "lic_dir1234567890",
		Tier:      TierEnterprise,
		Features:  EnterpriseFeatures(),
		IssuedAt:  time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		ExpiresAt: time.Time{},
		IssuedTo:  "dir@example.com",
		Signature: "",
	}
	data, _ := json.MarshalIndent(lic, "", "  ")
	os.WriteFile(filepath.Join(tmpDir, "license.json"), data, 0644)

	loaded, err := LoadFromDir(tmpDir)
	if err != nil {
		t.Fatalf("LoadFromDir: %v", err)
	}
	if loaded.Tier != TierEnterprise {
		t.Errorf("Tier = %v, want TierEnterprise", loaded.Tier)
	}
}

func TestLoadOrDefault(t *testing.T) {
	tmpDir := t.TempDir()

	// No file → default Open tier
	v, err := LoadOrDefault(filepath.Join(tmpDir, "license.json"), nil)
	if err != nil {
		t.Fatalf("LoadOrDefault no file: %v", err)
	}
	if v.Tier() != TierOpen {
		t.Errorf("Default tier = %v, want TierOpen", v.Tier())
	}

	// Valid file → Pro tier
	lic := &License{
		ID:        "lic_default12345678",
		Tier:      TierPro,
		Features:  ProFeatures(),
		IssuedAt:  time.Now().Add(-24 * time.Hour),
		ExpiresAt: time.Now().Add(365 * 24 * time.Hour),
		IssuedTo:  "default@example.com",
		Signature: "",
	}
	data, _ := json.MarshalIndent(lic, "", "  ")
	os.WriteFile(filepath.Join(tmpDir, "license.json"), data, 0644)

	v, err = LoadOrDefault(filepath.Join(tmpDir, "license.json"), nil)
	if err != nil {
		t.Fatalf("LoadOrDefault with file: %v", err)
	}
	if v.Tier() != TierPro {
		t.Errorf("Loaded tier = %v, want TierPro", v.Tier())
	}

	// Expired license → falls back to Open tier per spec
	expiredLic := &License{
		ID:        "lic_expired1234567890",
		Tier:      TierPro,
		Features:  ProFeatures(),
		IssuedAt:  time.Now().Add(-48 * time.Hour),
		ExpiresAt: time.Now().Add(-24 * time.Hour),
		IssuedTo:  "expired@example.com",
		Signature: "",
	}
	data, _ = json.MarshalIndent(expiredLic, "", "  ")
	os.WriteFile(filepath.Join(tmpDir, "license.json"), data, 0644)

	v, err = LoadOrDefault(filepath.Join(tmpDir, "license.json"), nil)
	if err != nil {
		t.Fatalf("LoadOrDefault with expired license: expected fallback to Open, got error: %v", err)
	}
	if v.Tier() != TierOpen {
		t.Errorf("Expired license should fall back to Open tier, got %v", v.Tier())
	}
}

func TestValidateLicenseFields(t *testing.T) {
	tests := []struct {
		name    string
		lic     *License
		wantErr bool
	}{
		{
			name: "valid license",
			lic: &License{
				ID:       "lic_valid123456789",
				Tier:     TierPro,
				Features: ProFeatures(),
				IssuedAt: time.Now(),
			},
			wantErr: false,
		},
		{
			name: "missing id",
			lic: &License{
				Tier:     TierOpen,
				Features: OpenFeatures(),
			},
			wantErr: true,
		},
		{
			name: "invalid id prefix",
			lic: &License{
				ID:       "invalid_id",
				Tier:     TierOpen,
				Features: OpenFeatures(),
			},
			wantErr: true,
		},
		{
			name: "empty features",
			lic: &License{
				ID:       "lic_empty123456789",
				Tier:     TierOpen,
				Features: []string{},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateLicenseFields(tt.lic)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateLicenseFields() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestLicenseInfo(t *testing.T) {
	// Default Open tier → no expiry
	v := NewValidator(nil, nil)
	info := v.LicenseInfo()
	if info.Tier != TierOpen {
		t.Errorf("LicenseInfo.Tier = %v, want TierOpen", info.Tier)
	}
	if info.ExpiresAt != nil {
		t.Errorf("LicenseInfo.ExpiresAt = %v, want nil for perpetual", info.ExpiresAt)
	}

	// Pro license with expiry
	lic := &License{
		ID:        "lic_pro1234567890",
		Tier:      TierPro,
		Features:  ProFeatures(),
		IssuedAt:  time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		ExpiresAt: time.Date(2027, 12, 31, 23, 59, 59, 0, time.UTC),
		IssuedTo:  "pro@example.com",
	}
	v2 := NewValidator(lic, nil)
	info2 := v2.LicenseInfo()
	if info2.Tier != TierPro {
		t.Errorf("LicenseInfo.Tier = %v, want TierPro", info2.Tier)
	}
	if info2.ExpiresAt == nil {
		t.Error("LicenseInfo.ExpiresAt = nil, want non-nil for expiring license")
	}
	if info2.ExpiresAt.Year() != 2027 {
		t.Errorf("LicenseInfo.ExpiresAt.Year = %d, want 2027", info2.ExpiresAt.Year())
	}
}

func TestOpenFeaturesConsistency(t *testing.T) {
	features := OpenFeatures()
	seen := make(map[string]bool)
	for _, f := range features {
		if seen[f] {
			t.Errorf("Duplicate Open feature: %q", f)
		}
		seen[f] = true
	}
}

func TestProFeaturesSuperset(t *testing.T) {
	openFeatures := OpenFeatures()
	proFeatures := ProFeatures()
	for _, f := range openFeatures {
		if !slices.Contains(proFeatures, f) {
			t.Errorf("Pro features missing Open feature: %q", f)
		}
	}
}

func sha256Sum(data []byte) [32]byte {
	return sha256.Sum256(data)
}
