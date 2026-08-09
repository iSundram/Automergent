package filesystem

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/iSundram/Automergent/internal/tools"
)

// GrepTool searches for a pattern in files.
type GrepTool struct{}

func (t *GrepTool) Name() string                          { return "grep" }
func (t *GrepTool) Description() string                   { return "Search for a pattern in files." }
func (t *GrepTool) RequiresConfirmation(mode string) bool { return false }

func (t *GrepTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"pattern": map[string]any{"type": "string", "description": "String to search for."},
			"path":    map[string]any{"type": "string", "description": "Directory or file to search."},
			"glob":    map[string]any{"type": "string", "description": "File glob pattern, e.g. *.go"},
		},
		"required": []string{"pattern"},
	}
}

func (t *GrepTool) Execute(_ context.Context, args map[string]any) (tools.Result, error) {
	pattern, _ := args["pattern"].(string)
	root, _ := args["path"].(string)
	glob, _ := args["glob"].(string)
	if root == "" {
		root = "."
	}
	if pattern == "" {
		return tools.Result{IsError: true, Content: "pattern is required"}, nil
	}

	var matches []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if d.Name() == ".git" || d.Name() == "node_modules" || d.Name() == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if glob != "" {
			matched, _ := filepath.Match(glob, d.Name())
			if !matched {
				return nil
			}
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		lines := strings.Split(string(data), "\n")
		for i, line := range lines {
			if strings.Contains(line, pattern) {
				matches = append(matches, fmt.Sprintf("%s:%d: %s", path, i+1, line))
				if len(matches) >= 100 {
					return fmt.Errorf("limit reached")
				}
			}
		}
		return nil
	})
	if err != nil && err.Error() != "limit reached" {
		return tools.Result{IsError: true, Content: fmt.Sprintf("walk error: %v", err)}, nil
	}

	if len(matches) == 0 {
		return tools.Result{Content: "no matches found"}, nil
	}
	return tools.Result{Content: strings.Join(matches, "\n")}, nil
}
