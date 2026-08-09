package filesystem

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
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
			"pattern": map[string]any{"type": "string", "description": "Regex pattern to search for."},
			"path":    map[string]any{"type": "string", "description": "Directory or file to search."},
			"glob":    map[string]any{"type": "string", "description": "File glob pattern, e.g. *.go"},
			"ignore_case": map[string]any{
				"type":        "boolean",
				"description": "Case-insensitive regex search.",
			},
			"max_results": map[string]any{
				"type":        "integer",
				"description": "Maximum number of matching lines to return (default 100).",
			},
		},
		"required": []string{"pattern"},
	}
}

func (t *GrepTool) Execute(_ context.Context, args map[string]any) (tools.Result, error) {
	pattern, patOk := tools.StringArg(args, "pattern")
	root, _ := tools.StringArg(args, "path")
	glob, _ := tools.StringArg(args, "glob")
	ignoreCase := false
	if v, set := tools.ArgBool(args, "ignore_case"); set {
		ignoreCase = v
	}
	maxResults := 100
	if n, ok := tools.ArgInt(args, "max_results"); ok && n > 0 {
		maxResults = n
	}
	if root == "" {
		root = "."
	}
	if !patOk || pattern == "" {
		return tools.Result{IsError: true, Content: "pattern is required"}, nil
	}
	if ignoreCase {
		pattern = "(?i)" + pattern
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return tools.Result{IsError: true, Content: fmt.Sprintf("invalid regex pattern: %v", err)}, nil
	}

	var matches []string
	limitErr := fmt.Errorf("limit reached")
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if path == root {
				return err
			}
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
		if isBinary(data) {
			return nil
		}
		lines := strings.Split(string(data), "\n")
		for i, line := range lines {
			if re.MatchString(line) {
				matches = append(matches, fmt.Sprintf("%s:%d: %s", path, i+1, line))
				if len(matches) >= maxResults {
					return limitErr
				}
			}
		}
		return nil
	})
	if err != nil && err != limitErr {
		return tools.Result{IsError: true, Content: fmt.Sprintf("walk error: %v", err)}, nil
	}

	if len(matches) == 0 {
		return tools.Result{Content: "no matches found"}, nil
	}
	return tools.Result{Content: strings.Join(matches, "\n")}, nil
}

func isBinary(data []byte) bool {
	check := data
	if len(check) > 8192 {
		check = check[:8192]
	}
	for _, b := range check {
		if b == 0 {
			return true
		}
	}
	return false
}
