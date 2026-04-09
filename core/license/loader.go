package license

import (
	"crypto/ecdsa"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
)

// LoadFromFile loads a license from a file path.
// Supports two formats:
//   - .json: Standard JSON license file containing the license data with signature.
//     The entire file is parsed as a License struct.
//   - .bin: Binary format containing the JSON license data (same structure, different extension).
//
// Returns ErrLicenseNotFound if the file does not exist.
// Returns ErrLicenseInvalidFormat if the file content cannot be parsed.
func LoadFromFile(path string) (*License, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrLicenseNotFound
		}
		return nil, fmt.Errorf("license: read file %s: %w", path, err)
	}

	if len(data) == 0 {
		return nil, fmt.Errorf("license: empty license file %s: %w", path, ErrLicenseInvalidFormat)
	}

	var lic License
	if err := json.Unmarshal(data, &lic); err != nil {
		return nil, fmt.Errorf("license: parse license file %s: %w", path, ErrLicenseInvalidFormat)
	}

	if err := validateLicenseFields(&lic); err != nil {
		return nil, fmt.Errorf("license: invalid license in %s: %w", path, err)
	}

	return &lic, nil
}

// LoadFromDir loads a license from a directory by searching for known license file names.
// Searches for: license.json, license.bin, .license.json, .license.bin (in order).
// Returns ErrLicenseNotFound if no license file is found.
func LoadFromDir(dir string) (*License, error) {
	candidates := []string{
		"license.json",
		"license.bin",
		".license.json",
		".license.bin",
	}

	for _, name := range candidates {
		path := dir + "/" + name
		lic, err := LoadFromFile(path)
		if err == nil {
			return lic, nil
		}
		if err == ErrLicenseNotFound {
			continue
		}
		return nil, err
	}

	return nil, ErrLicenseNotFound
}

// LoadOrDefault loads a license from the given path.
// If the file does not exist, returns a default Open tier license.
// If the license has expired, falls back to Open tier per spec.
// This is the primary entry point for license initialization at startup.
func LoadOrDefault(path string, pubKey *ecdsa.PublicKey) (*Validator, error) {
	lic, err := LoadFromFile(path)
	if err != nil {
		if err == ErrLicenseNotFound {
			slog.Info("no license file found, defaulting to open tier", "path", path)
			return NewValidator(nil, pubKey), nil
		}
		return nil, err
	}

	v := NewValidator(lic, pubKey)
	if err := v.Validate(); err != nil {
		return nil, fmt.Errorf("license: validation failed: %w", err)
	}

	return v, nil
}

func validateLicenseFields(lic *License) error {
	var errs []error

	if lic.ID == "" {
		errs = append(errs, fmt.Errorf("id is required"))
	}

	if lic.ID != "" && !strings.HasPrefix(lic.ID, "lic_") {
		errs = append(errs, fmt.Errorf("id must start with 'lic_' prefix"))
	}

	if lic.Tier != TierOpen && lic.Tier != TierPro && lic.Tier != TierEnterprise {
		errs = append(errs, fmt.Errorf("invalid tier %d", lic.Tier))
	}

	if len(lic.Features) == 0 {
		errs = append(errs, fmt.Errorf("features cannot be empty"))
	}

	if !lic.IssuedAt.IsZero() && !lic.ExpiresAt.IsZero() && lic.ExpiresAt.Before(lic.IssuedAt) {
		errs = append(errs, fmt.Errorf("expires_at cannot be before issued_at"))
	}

	if len(errs) > 0 {
		return fmt.Errorf("%v", errs)
	}

	return nil
}

// Sentinel errors for license operations.
var (
	ErrLicenseNotFound      = fmt.Errorf("license: file not found")
	ErrLicenseInvalidFormat = fmt.Errorf("license: invalid format")
)
