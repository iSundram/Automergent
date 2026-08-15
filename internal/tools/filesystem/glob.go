package filesystem

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/iSundram/Automergent/internal/tools"
)

// GlobTool provides fast file pattern matching using glob patterns.
type GlobTool struct{}

func (t *GlobTool) Name() string { return "glob" }
func (t *GlobTool) Description() string {
	return `Fast file pattern matching using glob patterns.
- Supports * (any chars in segment), ** (any chars across segments), ? (single char), {a,b} (alternatives)
- Examples: "**/*.go", "src/**/*.ts", "*.{js,jsx}", "test_*.py"
- Returns matching file paths
- Use for finding files by name; use grep for searching contents`
}
func (t *GlobTool) RequiresConfirmation(mode string) bool { return false }

func (t *GlobTool) EstimatedCost() tools.ToolCost {
	return tools.ToolCost{
		TokensApprox: 100,
		LatencyMs:    200,
		RiskLevel:    "low",
	}
}

func (t *GlobTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"pattern": map[string]any{
				"type":        "string",
				"description": "Glob pattern to match files (e.g., '**/*.go', 'src/**/*.ts').",
			},
			"path": map[string]any{
				"type":        "string",
				"description": "Directory to search in. Defaults to current directory.",
			},
			"max_results": map[string]any{
				"type":        "integer",
				"description": "Maximum number of results (default: 1000).",
			},
			"include_hidden": map[string]any{
				"type":        "boolean",
				"description": "Include hidden files/directories (default: false).",
			},
		},
		"required": []string{"pattern"},
	}
}

func (t *GlobTool) Execute(_ context.Context, args map[string]any) (tools.Result, error) {
	pattern, ok := tools.StringArg(args, "pattern")
	if !ok || pattern == "" {
		return tools.Result{IsError: true, Content: "pattern is required"}, nil
	}

	root, _ := tools.StringArg(args, "path")
	if root == "" {
		root = "."
	}

	maxResults := 1000
	if n, ok := tools.ArgInt(args, "max_results"); ok && n > 0 {
		maxResults = n
	}

	includeHidden := false
	if v, ok := tools.ArgBool(args, "include_hidden"); ok {
		includeHidden = v
	}

	// Check if pattern contains **
	hasDoublestar := strings.Contains(pattern, "**")

	var matches []string
	truncated := false

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip hidden files/dirs unless requested (never skip the root itself)
		if !includeHidden && path != root && strings.HasPrefix(d.Name(), ".") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip common large directories
		if d.IsDir() {
			switch d.Name() {
			case "node_modules", "vendor", ".git", "__pycache__", ".venv", "venv":
				return filepath.SkipDir
			}
		}

		// Only match files (not directories)
		if d.IsDir() {
			return nil
		}

		// Get relative path for matching
		relPath, _ := filepath.Rel(root, path)

		// Match pattern
		var matched bool
		if hasDoublestar || strings.Contains(pattern, "/") {
			matched = matchDoublestar(pattern, relPath)
		} else {
			matched, _ = filepath.Match(pattern, filepath.Base(relPath))
		}

		if matched {
			if len(matches) >= maxResults {
				truncated = true
				return filepath.SkipAll
			}
			matches = append(matches, relPath)
		}

		return nil
	})

	if err != nil && err != filepath.SkipAll {
		return tools.Result{IsError: true, Content: fmt.Sprintf("error walking directory: %v", err)}, nil
	}

	if len(matches) == 0 {
		return tools.Result{Content: "no matches found"}, nil
	}

	result := strings.Join(matches, "\n")
	if truncated {
		result += fmt.Sprintf("\n\n... (truncated at %d results)", maxResults)
	}

	return tools.Result{
		Content: result,
		Summary: fmt.Sprintf("found %d files", len(matches)),
		Metadata: map[string]any{
			"count":     len(matches),
			"truncated": truncated,
		},
	}, nil
}

// matchDoublestar matches a pattern with ** support against a path.
func matchDoublestar(pattern, path string) bool {
	// Normalize separators
	pattern = filepath.ToSlash(pattern)
	path = filepath.ToSlash(path)

	return doMatch(pattern, path)
}

const maxGlobDepth = 256

func doMatch(pattern, path string) bool {
	return doMatchDepth(pattern, path, 0)
}

func doMatchDepth(pattern, path string, depth int) bool {
	return doMatchDepthEx(pattern, path, depth, false)
}

func doMatchDepthEx(pattern, path string, depth int, starCrossesSlash bool) bool {
	if depth > maxGlobDepth {
		return false
	}
	for len(pattern) > 0 {
		switch {
		case strings.HasPrefix(pattern, "**"):
			// ** matches zero or more path segments
			pattern = strings.TrimPrefix(pattern, "**")
			pattern = strings.TrimPrefix(pattern, "/")

			if pattern == "" {
				return true
			}

			// Try matching rest of pattern at every position
			for i := 0; i <= len(path); i++ {
				if doMatchDepthEx(pattern, path[i:], depth+1, true) {
					return true
				}
				// Skip to next segment
				idx := strings.Index(path[i:], "/")
				if idx == -1 {
					break
				}
				i += idx
			}
			return false

		case strings.HasPrefix(pattern, "*"):
			// * matches any chars, optionally across / if starCrossesSlash
			pattern = pattern[1:]
			if pattern == "" {
				if starCrossesSlash {
					return true
				}
				return !strings.Contains(path, "/")
			}

			// Find next literal char in pattern
			for i := 0; i <= len(path); i++ {
				if !starCrossesSlash && i > 0 && path[i-1] == '/' {
					break // * can't cross /
				}
				if doMatchDepthEx(pattern, path[i:], depth+1, starCrossesSlash) {
					return true
				}
			}
			return false

		case strings.HasPrefix(pattern, "?"):
			// ? matches single char except /
			if len(path) == 0 || path[0] == '/' {
				return false
			}
			pattern = pattern[1:]
			path = path[1:]

		case strings.HasPrefix(pattern, "{"):
			// Handle {a,b,c} alternatives
			end := strings.Index(pattern, "}")
			if end == -1 {
				return false
			}
			alts := strings.Split(pattern[1:end], ",")
			rest := pattern[end+1:]

			for _, alt := range alts {
				if doMatchDepthEx(alt+rest, path, depth+1, starCrossesSlash) {
					return true
				}
			}
			return false

		default:
			// Literal character match
			if len(path) == 0 || pattern[0] != path[0] {
				return false
			}
			pattern = pattern[1:]
			path = path[1:]
		}
	}

	return len(path) == 0
}
