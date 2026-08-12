package filesystem

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// isPathBlocked checks if a path matches any of the blocked patterns.
// Blocked patterns can use glob syntax (e.g., ".git/**", "~/.ssh/**").
// Returns true if the path is blocked, along with the matching pattern.
func isPathBlocked(path string, blockedPatterns []string) (bool, string) {
	if len(blockedPatterns) == 0 {
		return false, ""
	}

	// Resolve symlinks and make absolute to prevent bypass attempts
	absPath, err := filepath.Abs(path)
	if err != nil {
		// If we can't resolve, treat as potentially dangerous and block
		return true, "unresolvable path"
	}

	// Resolve symlinks
	resolvedPath, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		// If symlink resolution fails (e.g., path doesn't exist yet),
		// use the absolute path for checking
		resolvedPath = absPath
	}

	// Expand ~ to home directory in blocked patterns
	homeDir, _ := os.UserHomeDir()

	for _, pattern := range blockedPatterns {
		// Expand ~ in pattern
		expandedPattern := pattern
		if strings.HasPrefix(pattern, "~/") && homeDir != "" {
			expandedPattern = filepath.Join(homeDir, pattern[2:])
		}

		// Check for exact match or glob pattern match
		if matched, err := filepath.Match(expandedPattern, resolvedPath); err == nil && matched {
			return true, pattern
		}

		// Check for directory prefix match (for patterns like ".git/**")
		if strings.HasSuffix(expandedPattern, "/**") {
			prefix := strings.TrimSuffix(expandedPattern, "/**")
			absPrefix, err := filepath.Abs(prefix)
			if err == nil {
				// Check if resolvedPath is inside the blocked directory
				rel, err := filepath.Rel(absPrefix, resolvedPath)
				if err == nil && !strings.HasPrefix(rel, "..") {
					return true, pattern
				}
			}
		}

		// Check for simple prefix match
		absPattern, err := filepath.Abs(expandedPattern)
		if err == nil {
			if strings.HasPrefix(resolvedPath, absPattern) {
				return true, pattern
			}
		}
	}

	return false, ""
}

// validateWritePath checks if writing to the given path is allowed based on
// the blocked and allowed write path configurations.
// Returns an error if the path is blocked.
func validateWritePath(path string, blockedPaths []string, allowedPaths []string) error {
	// Check allowed paths first - if specified and path matches, allow
	if len(allowedPaths) > 0 {
		allowed := false
		absPath, err := filepath.Abs(path)
		if err != nil {
			return fmt.Errorf("cannot resolve path: %s", path)
		}

		homeDir, _ := os.UserHomeDir()

		for _, pattern := range allowedPaths {
			expandedPattern := pattern
			if strings.HasPrefix(pattern, "~/") && homeDir != "" {
				expandedPattern = filepath.Join(homeDir, pattern[2:])
			}

			// Handle glob patterns like "src/**"
			if strings.HasSuffix(pattern, "/**") {
				prefix := strings.TrimSuffix(expandedPattern, "/**")
				absPrefix, err := filepath.Abs(prefix)
				if err == nil {
					rel, err := filepath.Rel(absPrefix, absPath)
					if err == nil && !strings.HasPrefix(rel, "..") {
						allowed = true
						break
					}
				}
			} else {
				// Exact or glob match
				absPattern, err := filepath.Abs(expandedPattern)
				if err == nil {
					if matched, err := filepath.Match(absPattern, absPath); err == nil && matched {
						allowed = true
						break
					}
					// Also check prefix match
					if strings.HasPrefix(absPath, absPattern+string(filepath.Separator)) || absPath == absPattern {
						allowed = true
						break
					}
				}
			}
		}

		if !allowed {
			return fmt.Errorf("path not in allowed write paths: %s", path)
		}
	}

	// Check blocked paths
	if blocked, pattern := isPathBlocked(path, blockedPaths); blocked {
		return fmt.Errorf("writing to this path is blocked by security policy (pattern: %s): %s", pattern, path)
	}

	return nil
}
