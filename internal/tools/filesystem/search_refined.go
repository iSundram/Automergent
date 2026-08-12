package filesystem

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/iSundram/Automergent/internal/tools"
)

// RefinedSearchTool provides advanced contextual search across the codebase.
type RefinedSearchTool struct{}

func (t *RefinedSearchTool) Name() string { return "search" }

func (t *RefinedSearchTool) Description() string {
	return `Search for patterns in the codebase with rich context. 
Returns matches with surrounding lines (above and below) to help understand the code context.`
}

func (t *RefinedSearchTool) RequiresConfirmation(mode string) bool { return false }

func (t *RefinedSearchTool) EstimatedCost() tools.ToolCost {
	return tools.ToolCost{
		TokensApprox: 250,
		LatencyMs:    600,
		RiskLevel:    "low",
	}
}

func (t *RefinedSearchTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query":         map[string]any{"type": "string", "description": "The search query (regex supported)"},
			"path":          map[string]any{"type": "string", "description": "Directory to search in (default: '.')"},
			"context_lines": map[string]any{"type": "integer", "description": "Number of lines to show above and below (default: 2)"},
			"ignore_case":   map[string]any{"type": "boolean", "description": "Ignore case (default: true)"},
			"max_results":   map[string]any{"type": "integer", "description": "Max number of files to return results for (default: 10)"},
		},
		"required": []string{"query"},
	}
}

func (t *RefinedSearchTool) Execute(ctx context.Context, args map[string]any) (tools.Result, error) {
	query, _ := tools.StringArg(args, "query")
	root, _ := tools.StringArg(args, "path")
	if root == "" {
		root = "."
	}

	contextLines := 2
	if cl, ok := tools.ArgInt(args, "context_lines"); ok {
		contextLines = cl
	}

	ignoreCase := true
	if ic, ok := args["ignore_case"].(bool); ok {
		ignoreCase = ic
	}

	maxFiles := 10
	if mf, ok := tools.ArgInt(args, "max_results"); ok {
		maxFiles = mf
	}

	pattern := query
	if ignoreCase {
		pattern = "(?i)" + pattern
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return tools.Result{IsError: true, Content: "invalid regex: " + err.Error()}, nil
	}

	var output strings.Builder
	filesProcessed := 0
	matchesFound := 0

	err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || filesProcessed >= maxFiles {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" || d.Name() == "node_modules" || d.Name() == "vendor" || d.Name() == "dist" || d.Name() == "bin" {
				return filepath.SkipDir
			}
			return nil
		}

		// Basic binary check
		isBinary, _ := isBinaryFile(path)
		if isBinary {
			return nil
		}

		matches := t.searchInFile(path, re, contextLines)
		if len(matches) > 0 {
			output.WriteString(fmt.Sprintf("─── %s ───\n", path))
			for _, m := range matches {
				output.WriteString(m + "\n")
				matchesFound++
			}
			output.WriteString("\n")
			filesProcessed++
		}
		return nil
	})

	if output.Len() == 0 {
		return tools.Result{Content: "No matches found for query: " + query}, nil
	}

	resText := output.String()
	if filesProcessed >= maxFiles {
		resText += fmt.Sprintf("... (more results in other files, reached limit of %d files)\n", maxFiles)
	}

	return tools.Result{
		Content: resText,
		Summary: fmt.Sprintf("found %d matches in %d files", matchesFound, filesProcessed),
	}, nil
}

func (t *RefinedSearchTool) searchInFile(path string, re *regexp.Regexp, context int) []string {
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	var results []string
	lastEmittedLine := -1

	for i, line := range lines {
		if re.MatchString(line) {
			start := i - context
			if start < 0 {
				start = 0
			}
			if start <= lastEmittedLine {
				start = lastEmittedLine + 1
			}

			end := i + context
			if end >= len(lines) {
				end = len(lines) - 1
			}

			// Header for each hunk if there was a gap
			if lastEmittedLine != -1 && start > lastEmittedLine+1 {
				results = append(results, "  ...")
			}

			for j := start; j <= end; j++ {
				prefix := "  "
				if j == i {
					prefix = "> "
				}
				results = append(results, fmt.Sprintf("%4d | %s%s", j+1, prefix, lines[j]))
			}
			lastEmittedLine = end
		}
	}
	return results
}
