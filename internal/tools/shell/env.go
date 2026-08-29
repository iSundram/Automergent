package shell

import (
	"strings"
)

// defaultSensitivePatterns lists env var name patterns that are stripped before
// executing shell commands to prevent credential leakage.
var defaultSensitivePatterns = []string{
	"*_SECRET", "*_PASSWORD", "*_TOKEN", "*_KEY",
	"AWS_*", "GEMINI_*",
	"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "NO_PROXY",
	"http_proxy", "https_proxy", "all_proxy", "no_proxy",
}

// filterEnv removes env vars whose names match any of the given glob patterns.
// It returns a new slice of env strings (KEY=VALUE) with matches removed.
func filterEnv(env []string, patterns []string) []string {
	filtered := env[:0:len(env)]
	for _, e := range env {
		name := e
		if idx := strings.IndexByte(e, '='); idx >= 0 {
			name = e[:idx]
		}
		blocked := false
		for _, pat := range patterns {
			if matchGlob(pat, name) {
				blocked = true
				break
			}
		}
		if !blocked {
			filtered = append(filtered, e)
		}
	}
	return filtered
}

// matchGlob implements simple shell-style glob matching (only * wildcard).
func matchGlob(pattern, name string) bool {
	if pattern == "*" {
		return true
	}
	// Use stdlib path matching via a simple recursive approach
	for len(pattern) > 0 {
		if pattern[0] == '*' {
			// Skip consecutive stars
			for len(pattern) > 0 && pattern[0] == '*' {
				pattern = pattern[1:]
			}
			if len(pattern) == 0 {
				return true
			}
			for i := 0; i <= len(name); i++ {
				if matchGlob(pattern, name[i:]) {
					return true
				}
			}
			return false
		}
		if len(name) == 0 || pattern[0] != name[0] {
			return false
		}
		pattern = pattern[1:]
		name = name[1:]
	}
	return len(name) == 0
}
