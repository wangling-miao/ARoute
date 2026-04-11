// Package license provides the license subsystem for Aroute CMS,
// managing license validation, feature gating, and tier-based access control.
// Licenses are signed using ECDSA P-256 for authenticity and integrity.
package license

import (
	"crypto/ecdsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/big"
	"slices"
	"strings"
	"time"
)

// Tier represents the license tier level.
// Higher tiers include all features of lower tiers.
type Tier int

const (
	// TierOpen is the free and open-source tier.
	// All OSS features are available without a license file.
	TierOpen Tier = iota

	// TierPro is the commercial tier with additional features.
	// Includes everything in Open plus Pro-exclusive features.
	TierPro

	// TierEnterprise is the full-featured tier.
	// Includes everything in Pro plus Enterprise-exclusive features.
	TierEnterprise
)

// String returns the string representation of the Tier.
func (t Tier) String() string {
	switch t {
	case TierOpen:
		return "open"
	case TierPro:
		return "pro"
	case TierEnterprise:
		return "enterprise"
	default:
		return "unknown"
	}
}

// MarshalJSON implements json.Marshaler for Tier.
func (t Tier) MarshalJSON() ([]byte, error) {
	return json.Marshal(t.String())
}

// UnmarshalJSON implements json.Unmarshaler for Tier.
func (t *Tier) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	parsed, err := ParseTier(s)
	if err != nil {
		return err
	}
	*t = parsed
	return nil
}

// ParseTier parses a tier string and returns the corresponding Tier.
// Accepts: "open", "pro", "enterprise" (case-insensitive).
func ParseTier(s string) (Tier, error) {
	s = strings.ToLower(s)
	switch s {
	case "open":
		return TierOpen, nil
	case "pro":
		return TierPro, nil
	case "enterprise":
		return TierEnterprise, nil
	default:
		return TierOpen, fmt.Errorf("license: unknown tier %q (must be open, pro, or enterprise)", s)
	}
}

// License represents a parsed Aroute license with tier, features, expiration, and ECDSA P-256 signature.
type License struct {
	ID        string    `json:"id"`
	Tier      Tier      `json:"tier"`
	Features  []string  `json:"features"`
	IssuedAt  time.Time `json:"issued_at"`
	ExpiresAt time.Time `json:"expires_at"` // Zero value = perpetual (never expires)
	IssuedTo  string    `json:"issued_to"`
	Signature string    `json:"signature"` // Base64-encoded ECDSA P-256 signature over SHA-256(unsign​edData)
}

// IsExpired checks whether the license has expired.
// Returns true if ExpiresAt is in the past.
// A zero ExpiresAt means the license never expires (perpetual).
func (l *License) IsExpired() bool {
	if l.ExpiresAt.IsZero() {
		return false
	}
	return time.Now().After(l.ExpiresAt)
}

// HasFeature checks whether the license allows a specific feature.
func (l *License) HasFeature(feature string) bool {
	return slices.Contains(l.Features, feature)
}

// unsignedData returns the license data without the signature field,
// serialized as canonical JSON for signature verification.
func (l *License) unsignedData() ([]byte, error) {
	// Create a copy without the signature for hashing
	clone := *l
	clone.Signature = ""
	data, err := json.Marshal(clone)
	if err != nil {
		return nil, fmt.Errorf("license: marshal unsigned data: %w", err)
	}
	return data, nil
}

func (l *License) VerifySignature(pubKey *ecdsa.PublicKey) error {
	if pubKey == nil {
		return fmt.Errorf("license: public key is nil")
	}
	if l.Signature == "" {
		return fmt.Errorf("license: signature is empty")
	}

	data, err := l.unsignedData()
	if err != nil {
		return fmt.Errorf("license: compute unsigned data: %w", err)
	}
	hash := sha256.Sum256(data)

	sigBytes, err := base64.StdEncoding.DecodeString(l.Signature)
	if err != nil {
		return fmt.Errorf("license: decode base64 signature: %w", err)
	}

	if len(sigBytes) != 64 {
		return fmt.Errorf("license: invalid signature length %d, expected 64 bytes", len(sigBytes))
	}

	r := new(big.Int).SetBytes(sigBytes[:32])
	s := new(big.Int).SetBytes(sigBytes[32:])

	if !ecdsa.Verify(pubKey, hash[:], r, s) {
		return fmt.Errorf("license: signature verification failed (data may be tampered)")
	}

	return nil
}

// Validator manages license validation and feature gating. Safe for concurrent use after creation.
type Validator struct {
	license   *License
	publicKey *ecdsa.PublicKey
	features  map[string]bool
	openTier  *License
}

// NewValidator creates a Validator. If license is nil, defaults to Open tier.
// If pubKey is nil, signature verification is skipped (for testing).
func NewValidator(license *License, pubKey *ecdsa.PublicKey) *Validator {
	v := &Validator{
		publicKey: pubKey,
		features:  make(map[string]bool),
	}

	v.openTier = &License{
		ID:       "open-default",
		Tier:     TierOpen,
		Features: OpenFeatures(),
		IssuedAt: time.Now(),
		IssuedTo: "Aroute Open Source",
	}

	if license == nil {
		v.license = v.openTier
	} else {
		v.license = license
	}

	for _, f := range v.license.Features {
		v.features[f] = true
	}

	return v
}

// License returns the current active license.
func (v *Validator) License() *License {
	return v.license
}

// Tier returns the current license tier.
func (v *Validator) Tier() Tier {
	return v.license.Tier
}

// IsFeatureAllowed checks whether the current license allows a specific feature.
// Returns true if the feature is listed in the license's Features slice.
// Unknown feature names return false. Enterprise tier has all features.
func (v *Validator) IsFeatureAllowed(feature string) bool {
	return v.features[feature]
}

// IsExpired checks whether the current license has expired.
func (v *Validator) IsExpired() bool {
	return v.license.IsExpired()
}

// Validate checks the license for expiry and signature validity.
// If the license has expired, the Validator falls back to Open tier per spec.
// Expired licenses are handled by downgrading the active license to the
// default Open tier license, and Validate returns nil (no error).
func (v *Validator) Validate() error {
	if v.license.ID == "open-default" {
		return nil
	}

	if v.license.IsExpired() {
		slog.Warn("license expired, falling back to open tier", "id", v.license.ID, "expires_at", v.license.ExpiresAt.Format(time.RFC3339))
		v.license = v.openTier
		v.features = make(map[string]bool, len(v.openTier.Features))
		for _, f := range v.openTier.Features {
			v.features[f] = true
		}
		return nil
	}

	if v.publicKey != nil {
		if err := v.license.VerifySignature(v.publicKey); err != nil {
			return fmt.Errorf("license: validation failed: %w", err)
		}
	}

	return nil
}

// LicenseInfo returns the current license state for other components to query.
// Provides the current tier, allowed features, and expiry date (nil for perpetual).
func (v *Validator) LicenseInfo() LicenseInfo {
	return LicenseInfo{
		Tier:     v.license.Tier,
		Features: v.license.Features,
		ExpiresAt: func() *time.Time {
			if v.license.ExpiresAt.IsZero() {
				return nil
			}
			return new(v.license.ExpiresAt)
		}(),
	}
}

// LicenseInfo represents the current license state queryable by other components.
type LicenseInfo struct {
	Tier      Tier
	Features  []string
	ExpiresAt *time.Time
}

func OpenFeatures() []string {
	return []string{
		"plugin:l1-native", "plugin:l3-wasm", "plugin:hot-plug",
		"content:dynamic-ct", "content:crud", "content:versioning",
		"content:draft-publish", "content:slug", "content:field-validation",
		"media:upload", "media:local-storage", "media:thumbnails",
		"theme:go-template", "theme:lua",
		"search:fulltext", "search:chinese",
		"api:rest", "api:openapi",
		"auth:jwt", "auth:rbac", "auth:api-tokens",
		"cache:ristretto", "queue:in-process", "webhook:delivery",
		"ddl:schema-diff", "ddl:sqlite", "ddl:postgresql",
		"license:open-tier",
	}
}

func ProFeatures() []string {
	return append(OpenFeatures(),
		"plugin:l2-grpc", "theme:react-ssr", "search:facets",
		"cache:s3-storage", "webhook:retry-policy", "license:pro-tier",
	)
}

func EnterpriseFeatures() []string {
	return append(ProFeatures(),
		"multi-site", "auth:sso", "auth:ldap", "cluster", "license:enterprise-tier",
	)
}

func FeatureTier(feature string) Tier {
	proFeatures := map[string]bool{
		"plugin:l2-grpc": true, "theme:react-ssr": true, "search:facets": true,
		"cache:s3-storage": true, "webhook:retry-policy": true, "license:pro-tier": true,
	}
	if proFeatures[feature] {
		return TierPro
	}

	entFeatures := map[string]bool{
		"multi-site": true, "auth:sso": true, "auth:ldap": true,
		"cluster": true, "license:enterprise-tier": true,
	}
	if entFeatures[feature] {
		return TierEnterprise
	}

	return TierOpen
}
