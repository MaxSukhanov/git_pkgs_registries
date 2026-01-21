package core

import (
	"github.com/git-pkgs/registries/internal/urlparser"
)

// ExtractRepoURL extracts a repository URL from various API response formats.
// Handles:
//   - Plain string: "https://github.com/user/repo"
//   - Map with url/git/http key: {"url": "...", "type": "git"}
//   - Array of strings or maps: tries first valid entry
func ExtractRepoURL(v interface{}) string {
	return extractRepoURL(v)
}

func extractRepoURL(v interface{}) string {
	if v == nil {
		return ""
	}

	switch r := v.(type) {
	case string:
		return urlparser.Parse(r)

	case map[string]interface{}:
		// Try common key names in order of preference
		for _, key := range []string{"url", "git", "http"} {
			if url, ok := r[key].(string); ok && url != "" {
				if parsed := urlparser.Parse(url); parsed != "" {
					return parsed
				}
			}
		}

	case []interface{}:
		for _, item := range r {
			if url := extractRepoURL(item); url != "" {
				return url
			}
		}

	case []string:
		for _, url := range r {
			if parsed := urlparser.Parse(url); parsed != "" {
				return parsed
			}
		}
	}

	return ""
}

// ExtractRepoURLWithFallback tries multiple values and returns the first valid repo URL.
func ExtractRepoURLWithFallback(values ...interface{}) string {
	for _, v := range values {
		if url := ExtractRepoURL(v); url != "" {
			return url
		}
	}
	return ""
}
