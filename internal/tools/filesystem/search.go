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

// GrepTool searches for a pattern in files.
type GrepTool struct{}

func (t *GrepTool) Name() string { return "grep" }
func (t *GrepTool) Description() string {
	return `Search for patterns in file contents using regex.
- output_mode: "content" (show lines), "files_with_matches" (just paths), "count" (match counts)
- Use context_before/context_after (-B/-A) to show surrounding lines
- Use glob to filter file types (e.g., "*.go", "*.{ts,tsx}")
- Skips binary files, .git, node_modules, vendor directories`
}
func (t *GrepTool) RequiresConfirmation(mode string) bool { return false }

func (t *GrepTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"pattern": map[string]any{
				"type":        "string",
				"description": "Regex pattern to search for.",
			},
			"path": map[string]any{
				"type":        "string",
				"description": "Directory or file to search (default: current dir).",
			},
			"glob": map[string]any{
				"type":        "string",
				"description": "File glob pattern, e.g. '*.go', '*.{ts,tsx}'.",
			},
			"ignore_case": map[string]any{
				"type":        "boolean",
				"description": "Case-insensitive search (default: false).",
			},
			"output_mode": map[string]any{
				"type":        "string",
				"enum":        []string{"content", "files_with_matches", "count"},
				"description": "Output format: 'content' (matching lines), 'files_with_matches' (paths only), 'count' (match count per file).",
			},
			"context_before": map[string]any{
				"type":        "integer",
				"description": "Lines of context before each match (like grep -B).",
			},
			"context_after": map[string]any{
				"type":        "integer",
				"description": "Lines of context after each match (like grep -A).",
			},
			"multiline": map[string]any{
				"type":        "boolean",
				"description": "Enable multiline mode (patterns can span lines).",
			},
			"max_results": map[string]any{
				"type":        "integer",
				"description": "Maximum number of results (default: 100).",
			},
			"include_line_numbers": map[string]any{
				"type":        "boolean",
				"description": "Include line numbers in output (default: true for content mode).",
			},
		},
		"required": []string{"pattern"},
	}
}

func (t *GrepTool) Execute(_ context.Context, args map[string]any) (tools.Result, error) {
	pattern, patOk := tools.StringArg(args, "pattern")
	if !patOk || pattern == "" {
		return tools.Result{IsError: true, Content: "pattern is required"}, nil
	}

	root, _ := tools.StringArg(args, "path")
	if root == "" {
		root = "."
	}

	glob, _ := tools.StringArg(args, "glob")

	outputMode := "content"
	if mode, ok := tools.StringArg(args, "output_mode"); ok {
		outputMode = mode
	}

	ignoreCase := false
	if v, ok := tools.ArgBool(args, "ignore_case"); ok {
		ignoreCase = v
	}

	contextBefore := 0
	if n, ok := tools.ArgInt(args, "context_before"); ok && n > 0 {
		contextBefore = n
	}

	contextAfter := 0
	if n, ok := tools.ArgInt(args, "context_after"); ok && n > 0 {
		contextAfter = n
	}

	multiline := false
	if v, ok := tools.ArgBool(args, "multiline"); ok {
		multiline = v
	}

	maxResults := 100
	if n, ok := tools.ArgInt(args, "max_results"); ok && n > 0 {
		maxResults = n
	}

	showLineNumbers := outputMode == "content"
	if v, ok := tools.ArgBool(args, "include_line_numbers"); ok {
		showLineNumbers = v
	}

	// Build regex
	if ignoreCase {
		pattern = "(?i)" + pattern
	}
	if multiline {
		pattern = "(?s)" + pattern
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return tools.Result{IsError: true, Content: fmt.Sprintf("invalid regex: %v", err)}, nil
	}

	var results []string
	fileCounts := make(map[string]int)
	matchedFiles := make(map[string]bool)
	resultCount := 0
	truncated := false
	limitErr := fmt.Errorf("limit reached")

	err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if path == root {
				return err
			}
			return nil
		}

		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", "vendor", "__pycache__", ".venv", "venv":
				return filepath.SkipDir
			}
			return nil
		}

		// Apply glob filter
		if glob != "" {
			matched := matchGlobPattern(glob, d.Name())
			if !matched {
				return nil
			}
		}

		// Handle multiline search differently
		if multiline {
			return searchFileMultiline(path, re, outputMode, &results, fileCounts, matchedFiles, &resultCount, maxResults)
		}

		// Standard line-by-line search
		return searchFileByLine(path, re, outputMode, showLineNumbers, contextBefore, contextAfter, &results, fileCounts, matchedFiles, &resultCount, maxResults)
	})

	if err != nil && err != limitErr {
		return tools.Result{IsError: true, Content: fmt.Sprintf("walk error: %v", err)}, nil
	}
	truncated = resultCount >= maxResults

	// Format output based on mode
	var output string
	switch outputMode {
	case "files_with_matches":
		var files []string
		for f := range matchedFiles {
			files = append(files, f)
		}
		if len(files) == 0 {
			return tools.Result{Content: "no matches found"}, nil
		}
		output = strings.Join(files, "\n")

	case "count":
		if len(fileCounts) == 0 {
			return tools.Result{Content: "no matches found"}, nil
		}
		var counts []string
		for f, c := range fileCounts {
			counts = append(counts, fmt.Sprintf("%s:%d", f, c))
		}
		output = strings.Join(counts, "\n")

	default: // content
		if len(results) == 0 {
			return tools.Result{Content: "no matches found"}, nil
		}
		output = strings.Join(results, "\n")
	}

	if truncated {
		output += fmt.Sprintf("\n\n... (truncated at %d results)", maxResults)
	}

	summary := fmt.Sprintf("found %d matches across %d files", resultCount, len(matchedFiles))
	if resultCount == 0 {
		summary = "no matches found"
	}

	return tools.Result{
		Content: output,
		Summary: summary,
		Metadata: map[string]any{
			"files_matched": len(matchedFiles),
			"total_matches": resultCount,
			"truncated":     truncated,
		},
	}, nil
}

// pendingMatch tracks a match that needs after-context lines before being emitted
type pendingMatch struct {
	contextLines []string // lines to emit (before + match + after)
	afterNeeded  int      // how many more after-context lines needed
	matchLineNum int      // line number of the match
}

func searchFileByLine(path string, re *regexp.Regexp, outputMode string, showLineNumbers bool, contextBefore, contextAfter int, results *[]string, fileCounts map[string]int, matchedFiles map[string]bool, resultCount *int, maxResults int) error {
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()

	// Check for binary
	buf := make([]byte, 8192)
	n, _ := file.Read(buf)
	if isBinary(buf[:n]) {
		return nil
	}
	file.Seek(0, 0)

	scanner := bufio.NewScanner(file)
	var beforeBuffer []string  // rolling buffer of before-context lines
	var pending []pendingMatch // matches waiting for after-context
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		// Add this line as after-context to any pending matches
		for i := range pending {
			if pending[i].afterNeeded > 0 {
				if showLineNumbers {
					pending[i].contextLines = append(pending[i].contextLines, fmt.Sprintf("%s:%d+ %s", path, lineNum, line))
				} else {
					pending[i].contextLines = append(pending[i].contextLines, fmt.Sprintf("%s+ %s", path, line))
				}
				pending[i].afterNeeded--
			}
		}

		// Emit any matches that have collected enough after-context
		newPending := pending[:0]
		for _, pm := range pending {
			if pm.afterNeeded == 0 {
				*results = append(*results, strings.Join(pm.contextLines, "\n"))
			} else {
				newPending = append(newPending, pm)
			}
		}
		pending = newPending

		if re.MatchString(line) {
			matchedFiles[path] = true
			fileCounts[path]++

			if outputMode == "content" {
				// Build context output
				var contextLines []string

				// Before context from buffer
				start := len(beforeBuffer) - contextBefore
				if start < 0 {
					start = 0
				}
				for i := start; i < len(beforeBuffer); i++ {
					ln := lineNum - (len(beforeBuffer) - i)
					if showLineNumbers {
						contextLines = append(contextLines, fmt.Sprintf("%s:%d- %s", path, ln, beforeBuffer[i]))
					} else {
						contextLines = append(contextLines, fmt.Sprintf("%s- %s", path, beforeBuffer[i]))
					}
				}

				// Match line
				if showLineNumbers {
					contextLines = append(contextLines, fmt.Sprintf("%s:%d: %s", path, lineNum, line))
				} else {
					contextLines = append(contextLines, fmt.Sprintf("%s: %s", path, line))
				}

				// If we need after-context, defer this match
				if contextAfter > 0 {
					pending = append(pending, pendingMatch{
						contextLines: contextLines,
						afterNeeded:  contextAfter,
						matchLineNum: lineNum,
					})
				} else {
					*results = append(*results, strings.Join(contextLines, "\n"))
				}
			}

			(*resultCount)++
			if *resultCount >= maxResults {
				// Emit any pending matches before returning
				for _, pm := range pending {
					*results = append(*results, strings.Join(pm.contextLines, "\n"))
				}
				return fmt.Errorf("limit reached")
			}
		}

		// Maintain before-context buffer
		beforeBuffer = append(beforeBuffer, line)
		if len(beforeBuffer) > contextBefore {
			beforeBuffer = beforeBuffer[1:]
		}
	}

	// Emit any remaining pending matches (EOF before enough after-context)
	for _, pm := range pending {
		*results = append(*results, strings.Join(pm.contextLines, "\n"))
	}

	return nil
}

func searchFileMultiline(path string, re *regexp.Regexp, outputMode string, results *[]string, fileCounts map[string]int, matchedFiles map[string]bool, resultCount *int, maxResults int) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	if isBinary(data) {
		return nil
	}

	content := string(data)
	matches := re.FindAllStringIndex(content, -1)

	if len(matches) == 0 {
		return nil
	}

	matchedFiles[path] = true
	fileCounts[path] = len(matches)

	if outputMode == "content" {
		for _, match := range matches {
			start := match[0]
			end := match[1]

			// Find line boundaries
			lineStart := strings.LastIndex(content[:start], "\n") + 1
			lineEnd := strings.Index(content[end:], "\n")
			if lineEnd == -1 {
				lineEnd = len(content)
			} else {
				lineEnd += end
			}

			matchText := content[lineStart:lineEnd]
			*results = append(*results, fmt.Sprintf("%s: %s", path, matchText))

			(*resultCount)++
			if *resultCount >= maxResults {
				return fmt.Errorf("limit reached")
			}
		}
	} else {
		*resultCount += len(matches)
	}

	return nil
}

func matchGlobPattern(pattern, name string) bool {
	// Handle {a,b} patterns
	if strings.Contains(pattern, "{") {
		start := strings.Index(pattern, "{")
		end := strings.Index(pattern, "}")
		if end > start {
			prefix := pattern[:start]
			suffix := pattern[end+1:]
			alts := strings.Split(pattern[start+1:end], ",")
			for _, alt := range alts {
				if matchGlobPattern(prefix+alt+suffix, name) {
					return true
				}
			}
			return false
		}
	}
	matched, _ := filepath.Match(pattern, name)
	return matched
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
