package filesystem

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"syscall"

	"github.com/iSundram/Automergent/internal/config"
	"github.com/iSundram/Automergent/internal/diagnostics"
	"github.com/iSundram/Automergent/internal/tools"
)

// showDiagHeader reports whether the diagnostics header should be included
// in read results. It defaults to on when no configuration is available.
func (t *ReadFileTool) showDiagHeader() bool {
	if t.cfg != nil {
		return t.cfg.Diagnostics.ShowInRead
	}
	return true
}

// isBinaryFile reports whether the file at path appears to be binary.
// It reads up to 8 KB and checks for null bytes or non-text MIME type.
func isBinaryFile(path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()

	buf := make([]byte, 8192)
	n, err := f.Read(buf)
	if err != nil && n == 0 {
		return false, err
	}
	buf = buf[:n]

	// Null bytes are a strong indicator of binary content.
	if bytes.ContainsRune(buf, 0) {
		return true, nil
	}

	// Use http.DetectContentType for MIME-based detection.
	contentType := http.DetectContentType(buf)
	if strings.HasPrefix(contentType, "text/") {
		return false, nil
	}
	switch contentType {
	case "application/json", "application/xml", "application/x-yaml",
		"application/javascript", "application/x-sh":
		return false, nil
	}
	return true, nil
}

// ReadFileTool reads the contents of a file.
type ReadFileTool struct {
	tools.BaseTool
	cfg *config.Config
}

// NewReadFileTool builds a ReadFileTool honoring the given configuration
// (diagnostics display settings).
func NewReadFileTool(cfg *config.Config) *ReadFileTool {
	return &ReadFileTool{cfg: cfg}
}

func (t *ReadFileTool) Name() string                               { return "read_file" }
func (t *ReadFileTool) Description() string {
	return "Read a file from disk, returning line-numbered content. Optionally bounded by start_line/end_line."
}
func (t *ReadFileTool) RequiresConfirmation(mode string) bool      { return false }
func (t *ReadFileTool) IsConcurrencySafe(args map[string]any) bool { return true }
func (t *ReadFileTool) IsReadOnly(args map[string]any) bool        { return true }
func (t *ReadFileTool) IsDestructive(args map[string]any) bool     { return false }
func (t *ReadFileTool) EstimatedCost() tools.ToolCost {
	return tools.ToolCost{TokensApprox: 100, LatencyMs: 50, RiskLevel: "low"}
}

// Meta documents read_file in the system prompt.
func (t *ReadFileTool) Meta() *tools.ToolMeta {
	return &tools.ToolMeta{
		Category:     "read",
		DisplayName:  "Read file",
		InjectOrder:  10,
		PartialParse: true,
		UsageByFamily: map[string]string{
			"gemini3": "Gemini 3 batches parallel calls well: issue several independent read_file calls in one turn instead of sequencing them.",
		},
		WhenToUse: "Any time you need file contents: before editing, to trace symbols across files, or to verify an edit landed. Batch multiple independent reads in one turn.",
		WhenNotTo: "Do not use shell `cat`/`head`/`tail` for this. To list a directory use `list_directory`; to find files use `glob`/`grep`. Do not re-read a file listed under 'Contents already loaded'.",
		Usage: "Returns numbered lines from `path`, optionally bounded by `start_line`/`end_line`.\n" +
			"Prefer narrow ranges on huge files; results are ghosted when enormous.\n" +
			"If the file exists but is empty you will receive 'File is empty'.",
		Examples: [][2]string{
			{"read_file {\"path\": \"internal/tools/tool.go\"} before editing it", "editing blind and hoping old_str matches"},
		},
	}
}

func (t *ReadFileTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Absolute or relative path to the file.",
			},
			"start_line": map[string]any{
				"type":        "integer",
				"description": "First line to read (1-based, inclusive). Omit to read from start.",
			},
			"end_line": map[string]any{
				"type":        "integer",
				"description": "Last line to read (1-based, inclusive). Omit to read to end.",
			},
		},
		"required": []string{"path"},
	}
}

func (t *ReadFileTool) Execute(_ context.Context, args map[string]any) (tools.Result, error) {
	path, ok := tools.StringArg(args, "path")
	if !ok || path == "" {
		return tools.Result{IsError: true, Content: "path is required"}, nil
	}

	binary, err := isBinaryFile(path)
	if err != nil {
		// File might not exist — let the open below produce the error
		if !os.IsNotExist(err) {
			return tools.Result{IsError: true, Content: fmt.Sprintf("error checking file: %v", err)}, nil
		}
	}
	if binary {
		return tools.Result{
			IsError: true,
			Content: fmt.Sprintf("file %s appears to be binary; reading binary files is not supported", path),
		}, nil
	}

	// Open without following symlinks to avoid TOCTOU through symlink swaps.
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		if err == syscall.ELOOP {
			return tools.Result{IsError: true, Content: fmt.Sprintf("refusing to open symlink: %s", path)}, nil
		}
		return tools.Result{IsError: true, Content: fmt.Sprintf("error reading file: %v", err)}, nil
	}
	file := os.NewFile(uintptr(fd), path)
	defer file.Close()

	// Read all lines; diagnostics operate on the whole file even when a
	// range is requested, so the header always reflects the file's health.
	var all []string
	scanner := bufio.NewScanner(file)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024) // 1MB max line length
	for scanner.Scan() {
		all = append(all, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return tools.Result{IsError: true, Content: fmt.Sprintf("error reading file: %v", err)}, nil
	}

	var header strings.Builder
	if diags := diagnostics.Analyze(path, strings.Join(all, "\n")); len(diags) > 0 && t.showDiagHeader() {
		errCount, warnCount := 0, 0
		for _, d := range diags {
			switch d.Severity {
			case "error":
				errCount++
			case "warning":
				warnCount++
			}
		}
		header.WriteString("═══════════════════════════════════\n")
		header.WriteString(fmt.Sprintf("[DIAGNOSTICS: %d error(s), %d warning(s)]\n", errCount, warnCount))
		// Errors first, then warnings, then info/hints — capped so a badly
		// broken generated file cannot flood the context window.
		const maxDiagLines = 20
		shown := 0
		for _, sev := range []string{"error", "warning"} {
			for _, d := range diags {
				if shown >= maxDiagLines {
					break
				}
				if d.Severity != sev {
					continue
				}
				label := "ERROR"
				if sev == "warning" {
					label = "WARN "
				}
				header.WriteString(fmt.Sprintf("%s Line %d: %s - %s\n", label, d.Line, d.Code, d.Message))
				shown++
			}
		}
		if shown < len(diags) {
			header.WriteString(fmt.Sprintf("... (%d more diagnostics not shown)\n", len(diags)-shown))
		}
		if recoveryMsg := diagnostics.RecoveryMessage(path, strings.Join(all, "\n")); recoveryMsg != "" {
			header.WriteString("\n")
			header.WriteString(recoveryMsg)
		}
		header.WriteString("═══════════════════════════════════\n")
	}

	// Optional line range filtering (accepts JSON int or float).
	total := len(all)
	start, end := 1, total
	if n, ok := tools.ArgInt(args, "start_line"); ok && n >= 1 {
		start = n
	}
	if n, ok := tools.ArgInt(args, "end_line"); ok && n >= 1 && n < end {
		end = n
	}
	if start > total {
		return tools.Result{
			Content: header.String() + fmt.Sprintf("file has only %d lines, requested start line %d", total, start),
			Summary: "read 0 lines",
		}, nil
	}
	if end < start {
		end = start
	}

	// Render numbered lines with a hard cap so huge files cannot flood the
	// context window in a single call.
	const maxLines = 10000
	truncated := false
	if end-start+1 > maxLines {
		end = start + maxLines - 1
		truncated = true
	}

	var lines []string
	for i := start; i <= end; i++ {
		lines = append(lines, fmt.Sprintf("%d. %s", i, all[i-1]))
	}
	result := header.String() + strings.Join(lines, "\n")
	if truncated {
		result += fmt.Sprintf("\n\n... (truncated at %d lines, use start_line to continue)", maxLines)
	}
	if len(lines) == 0 {
		result = header.String() + "(empty file)"
	}

	return tools.Result{
		Content: result,
		Summary: fmt.Sprintf("read %d-%d of %d lines", start, start+len(lines)-1, total),
		Metadata: map[string]any{
			"total_lines": total,
			"lines_shown": len(lines),
			"start_line":  start,
			"end_line":    end,
			"truncated":   truncated,
		},
	}, nil
}

// ListDirectoryTool lists the files in a directory.
type ListDirectoryTool struct{}

func (t *ListDirectoryTool) Name() string                               { return "list_directory" }
func (t *ListDirectoryTool) Description() string                        { return "List files and directories at a path." }
func (t *ListDirectoryTool) RequiresConfirmation(mode string) bool      { return false }
func (t *ListDirectoryTool) IsConcurrencySafe(args map[string]any) bool { return true }
func (t *ListDirectoryTool) IsReadOnly(args map[string]any) bool        { return true }
func (t *ListDirectoryTool) IsDestructive(args map[string]any) bool     { return false }
func (t *ListDirectoryTool) EstimatedCost() tools.ToolCost {
	return tools.ToolCost{TokensApprox: 50, LatencyMs: 20, RiskLevel: "low"}
}

func (t *ListDirectoryTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Directory path to list.",
			},
		},
		"required": []string{"path"},
	}
}

func (t *ListDirectoryTool) Execute(_ context.Context, args map[string]any) (tools.Result, error) {
	path, ok := tools.StringArg(args, "path")
	if !ok || path == "" {
		path = "."
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return tools.Result{IsError: true, Content: fmt.Sprintf("error listing directory: %v", err)}, nil
	}
	var result string
	for _, e := range entries {
		if e.IsDir() {
			result += e.Name() + "/\n"
		} else {
			result += e.Name() + "\n"
		}
	}
	return tools.Result{Content: result}, nil
}
