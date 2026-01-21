package core

import (
	"strings"

	"github.com/git-pkgs/spdx"
)

// ExtractLicense extracts a license string from various API response formats
// and normalizes it via SPDX. Handles:
//   - Plain string: "MIT"
//   - Map with type key: {"type": "MIT", "url": "..."}
//   - Array of strings or maps: ["MIT", "Apache-2.0"]
func ExtractLicense(v interface{}) string {
	raw := extractLicenseRaw(v)
	if raw == "" {
		return ""
	}
	normalized, _ := spdx.Normalize(raw)
	return normalized
}

// ExtractLicenseRaw extracts a license without SPDX normalization.
func ExtractLicenseRaw(v interface{}) string {
	return extractLicenseRaw(v)
}

func extractLicenseRaw(v interface{}) string {
	if v == nil {
		return ""
	}

	switch l := v.(type) {
	case string:
		return l

	case map[string]interface{}:
		// Try common key names
		for _, key := range []string{"type", "name", "spdx_id", "license"} {
			if t, ok := l[key].(string); ok && t != "" {
				return t
			}
		}

	case []interface{}:
		var licenses []string
		for _, item := range l {
			if license := extractLicenseRaw(item); license != "" {
				licenses = append(licenses, license)
			}
		}
		if len(licenses) > 0 {
			return strings.Join(licenses, " AND ")
		}

	case []string:
		if len(l) > 0 {
			return strings.Join(l, " AND ")
		}
	}

	return ""
}

// ExtractLicenseFromClassifiers extracts a license from Python classifiers.
// Looks for "License :: OSI Approved :: MIT License" style classifiers.
func ExtractLicenseFromClassifiers(classifiers []string) string {
	for _, classifier := range classifiers {
		if strings.HasPrefix(classifier, "License :: ") {
			parts := strings.Split(classifier, " :: ")
			if len(parts) > 0 {
				raw := parts[len(parts)-1]
				normalized, _ := spdx.Normalize(raw)
				return normalized
			}
		}
	}
	return ""
}

// NormalizeLicense normalizes a license string via SPDX.
func NormalizeLicense(license string) string {
	if license == "" {
		return ""
	}
	normalized, _ := spdx.Normalize(license)
	return normalized
}
