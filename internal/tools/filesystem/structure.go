package filesystem

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/iSundram/Automergent/internal/tools"
)

// StructureTool provides a comprehensive project directory structure overview.
type StructureTool struct{}

func (t *StructureTool) Name() string { return "structure" }

func (t *StructureTool) Description() string {
	return `Display the project structure as a tree. 
Supports depth limiting, items-per-directory limiting, and automatic ignoring of common heavy folders.`
}

func (t *StructureTool) RequiresConfirmation(mode string) bool { return false }

func (t *StructureTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path":            map[string]any{"type": "string", "description": "The root path to start from (default: '.')"},
			"max_depth":       map[string]any{"type": "integer", "description": "How deep to recurse (default: 4)"},
			"items_per_dir":   map[string]any{"type": "integer", "description": "Max files/dirs to show per folder (default: 50)"},
			"ignore_patterns": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Custom patterns to ignore"},
			"show_hidden":     map[string]any{"type": "boolean", "description": "Show hidden files/folders (default: false)"},
		},
	}
}

func (t *StructureTool) Execute(ctx context.Context, args map[string]any) (tools.Result, error) {
	rootPath, _ := tools.StringArg(args, "path")
	if rootPath == "" {
		rootPath = "."
	}

	maxDepth := 4
	if d, ok := tools.ArgInt(args, "max_depth"); ok {
		maxDepth = d
	}

	itemsPerDir := 50
	if l, ok := tools.ArgInt(args, "items_per_dir"); ok {
		itemsPerDir = l
	}

	showHidden := false
	if sh, ok := args["show_hidden"].(bool); ok {
		showHidden = sh
	}

	customIgnores, _ := args["ignore_patterns"].([]any)
	ignoreMap := map[string]bool{
		".git":         true,
		"node_modules": true,
		"vendor":       true,
		"bin":          true,
		"obj":          true,
		".next":        true,
		"dist":         true,
		"build":        true,
	}
	for _, p := range customIgnores {
		if s, ok := p.(string); ok {
			ignoreMap[s] = true
		}
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Structure of %s (depth: %d):\n\n", rootPath, maxDepth))

	err := t.walk(rootPath, "", 0, maxDepth, itemsPerDir, ignoreMap, showHidden, &sb)
	if err != nil {
		return tools.Result{IsError: true, Content: err.Error()}, nil
	}

	return tools.Result{Content: sb.String(), Summary: fmt.Sprintf("mapped %s", rootPath)}, nil
}

func (t *StructureTool) walk(path, indent string, depth, maxDepth, itemsPerDir int, ignores map[string]bool, showHidden bool, sb *strings.Builder) error {
	if depth > maxDepth {
		return nil
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return err
	}

	// Filter and Sort
	var filtered []os.DirEntry
	for _, e := range entries {
		name := e.Name()
		if !showHidden && strings.HasPrefix(name, ".") && name != "." && name != ".." && name != ".automergent.md" && name != ".env.example" {
			if name != ".github" && name != ".gitignore" { // Special exceptions
				continue
			}
		}
		if ignores[name] {
			continue
		}
		filtered = append(filtered, e)
	}

	sort.Slice(filtered, func(i, j int) bool {
		if filtered[i].IsDir() != filtered[j].IsDir() {
			return filtered[i].IsDir()
		}
		return filtered[i].Name() < filtered[j].Name()
	})

	// Process
	count := 0
	for i, e := range filtered {
		if count >= itemsPerDir {
			sb.WriteString(fmt.Sprintf("%s└── ... (%d more items hidden)\n", indent, len(filtered)-count))
			break
		}

		isLast := i == len(filtered)-1 || count == itemsPerDir-1
		connector := "├── "
		newIndent := indent + "│   "
		if isLast {
			connector = "└── "
			newIndent = indent + "    "
		}

		name := e.Name()
		if e.IsDir() {
			sb.WriteString(fmt.Sprintf("%s%s%s/\n", indent, connector, name))
			err := t.walk(filepath.Join(path, name), newIndent, depth+1, maxDepth, itemsPerDir, ignores, showHidden, sb)
			if err != nil {
				return err
			}
		} else {
			sb.WriteString(fmt.Sprintf("%s%s%s\n", indent, connector, name))
		}
		count++
	}

	return nil
}
