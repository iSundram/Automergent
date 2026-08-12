package filesystem

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/iSundram/Automergent/internal/tools"
)

// ViewFileTool provides a dedicated tool for viewing files with line numbers.
// It's optimized for AI agents that need to read specific portions of files.
type ViewFileTool struct{}

func (t *ViewFileTool) Name() string { return "view" }
func (t *ViewFileTool) Description() string {
	return `View file contents with line numbers, or list directory contents.
- If path is a file: returns content with line numbers prefixed (e.g., "1. ", "2. ")
- If path is a directory: lists files up to 2 levels deep
- Use view_range to read specific lines (e.g., [10, 20] for lines 10-20)
- Use view_range [start, -1] to read from start to end of file`
}
func (t *ViewFileTool) RequiresConfirmation(mode string) bool { return false }

func (t *ViewFileTool) EstimatedCost() tools.ToolCost {
	return tools.ToolCost{
		TokensApprox: 150,
		LatencyMs:    100,
		RiskLevel:    "low",
	}
}

func (t *ViewFileTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Absolute or relative path to file or directory.",
			},
			"view_range": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "integer"},
				"description": "Optional [start_line, end_line] range (1-indexed). Use -1 for end_line to read to EOF.",
			},
			"max_lines": map[string]any{
				"type":        "integer",
				"description": "Maximum number of lines to return (default: 500, max: 10000).",
			},
		},
		"required": []string{"path"},
	}
}

func (t *ViewFileTool) Execute(_ context.Context, args map[string]any) (tools.Result, error) {
	path, ok := tools.StringArg(args, "path")
	if !ok || path == "" {
		return tools.Result{IsError: true, Content: "path is required"}, nil
	}

	// Check if path exists
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return tools.Result{IsError: true, Content: fmt.Sprintf("path does not exist: %s", path)}, nil
		}
		return tools.Result{IsError: true, Content: fmt.Sprintf("error accessing path: %v", err)}, nil
	}

	// Handle directory listing
	if info.IsDir() {
		return listDirectoryTree(path, 2)
	}

	// Handle file viewing
	return viewFileWithLines(path, args)
}

func viewFileWithLines(path string, args map[string]any) (tools.Result, error) {
	// Check for binary file
	binary, err := isBinaryFile(path)
	if err != nil && !os.IsNotExist(err) {
		return tools.Result{IsError: true, Content: fmt.Sprintf("error checking file: %v", err)}, nil
	}
	if binary {
		return tools.Result{
			IsError: true,
			Content: fmt.Sprintf("file %s appears to be binary; use appropriate tools for binary files", path),
		}, nil
	}

	// Parse view_range
	var startLine, endLine int = 1, -1 // -1 means read to end
	if rangeVal, ok := args["view_range"]; ok {
		if rangeArr, ok := rangeVal.([]any); ok && len(rangeArr) >= 2 {
			if start, ok := toInt(rangeArr[0]); ok {
				startLine = start
			}
			if end, ok := toInt(rangeArr[1]); ok {
				endLine = end
			}
		}
	}

	// Parse max_lines
	maxLines := 500
	if n, ok := tools.ArgInt(args, "max_lines"); ok && n > 0 {
		if n > 10000 {
			n = 10000
		}
		maxLines = n
	}

	// Validate start line
	if startLine < 1 {
		startLine = 1
	}

	// Read file
	file, err := os.Open(path)
	if err != nil {
		return tools.Result{IsError: true, Content: fmt.Sprintf("error opening file: %v", err)}, nil
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	// Increase buffer size for long lines
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024) // 1MB max line length

	lineNum := 0
	linesRead := 0
	truncated := false

	// Capture lines in range
	for scanner.Scan() {
		lineNum++

		// Range check
		if lineNum >= startLine && (endLine == -1 || lineNum <= endLine) {
			if linesRead < maxLines {
				lines = append(lines, fmt.Sprintf("%d. %s", lineNum, scanner.Text()))
				linesRead++
			} else {
				truncated = true
			}
		}
	}

	// Continue scanning to get accurate total line count if we stopped early
	for scanner.Scan() {
		lineNum++
	}

	if err := scanner.Err(); err != nil {
		return tools.Result{IsError: true, Content: fmt.Sprintf("error reading file: %v", err)}, nil
	}

	if len(lines) == 0 {
		if startLine > lineNum {
			return tools.Result{
				Content: fmt.Sprintf("file has only %d lines, requested start line %d", lineNum, startLine),
			}, nil
		}
		return tools.Result{Content: "(empty file)"}, nil
	}

	result := strings.Join(lines, "\n")
	if truncated {
		result += fmt.Sprintf("\n\n... (truncated at %d lines, use view_range to see more)", maxLines)
	}

	summary := fmt.Sprintf("read %d-%d of %d lines", startLine, startLine+linesRead-1, lineNum)
	if linesRead == 0 {
		summary = "read 0 lines"
	}

	return tools.Result{
		Content: result,
		Summary: summary,
		Metadata: map[string]any{
			"total_lines": lineNum,
			"lines_shown": linesRead,
			"start_line":  startLine,
			"end_line":    endLine,
			"truncated":   truncated,
		},
	}, nil
}

func listDirectoryTree(root string, maxDepth int) (tools.Result, error) {
	var entries []string

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // Skip errors
		}

		// Get relative path
		relPath, _ := filepath.Rel(root, path)
		if relPath == "." {
			return nil
		}

		// Calculate depth
		depth := strings.Count(relPath, string(os.PathSeparator))
		if depth >= maxDepth {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip hidden files at root level
		if strings.HasPrefix(d.Name(), ".") && depth == 0 {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Format entry
		indent := strings.Repeat("  ", depth)
		if d.IsDir() {
			entries = append(entries, fmt.Sprintf("%s%s/", indent, d.Name()))
		} else {
			entries = append(entries, fmt.Sprintf("%s%s", indent, d.Name()))
		}

		return nil
	})

	if err != nil {
		return tools.Result{IsError: true, Content: fmt.Sprintf("error listing directory: %v", err)}, nil
	}

	if len(entries) == 0 {
		return tools.Result{Content: "(empty directory)", Summary: "listed 0 items"}, nil
	}

	return tools.Result{
		Content: strings.Join(entries, "\n"),
		Summary: fmt.Sprintf("listed %d items", len(entries)),
	}, nil
}

func toInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		return int(n), true
	case float64:
		return int(n), true
	case float32:
		return int(n), true
	}
	return 0, false
}
