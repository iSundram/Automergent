package filesystem

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/iSundram/Automergent/internal/config"
	"github.com/iSundram/Automergent/internal/diagnostics"
	"github.com/iSundram/Automergent/internal/tools"
)

// atomicWriteFile writes data to path atomically: write to a temp file in the
// same directory, sync, set permissions, then rename into place.
func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".automergent-tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() {
		tmp.Close()
		os.Remove(tmpPath)
	}()
	if _, err := tmp.Write(data); err != nil {
		return fmt.Errorf("write temp: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("sync temp: %w", err)
	}
	tmp.Close()
	if err := os.Chmod(tmpPath, perm); err != nil {
		return fmt.Errorf("chmod temp: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("rename: %w", err)
	}
	return nil
}

// WriteFileTool writes content to a file.
type WriteFileTool struct {
	cfg *config.Config
}

func NewWriteFileTool(cfg *config.Config) *WriteFileTool {
	return &WriteFileTool{cfg: cfg}
}

func (t *WriteFileTool) Name() string { return "write_file" }
func (t *WriteFileTool) Description() string {
	return "Write content to a file, creating it if needed."
}
func (t *WriteFileTool) RequiresConfirmation(mode string) bool {
	return mode == "edit" || mode == "plan"
}
func (t *WriteFileTool) IsConcurrencySafe(args map[string]any) bool {
	// Writing to files is not safe if the same file is being written elsewhere
	return false
}
func (t *WriteFileTool) IsReadOnly(args map[string]any) bool { return false }
func (t *WriteFileTool) IsDestructive(args map[string]any) bool {
	path, ok := tools.StringArg(args, "path")
	if !ok || path == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}
func (t *WriteFileTool) EstimatedCost() tools.ToolCost {
	return tools.ToolCost{TokensApprox: 200, LatencyMs: 100, RiskLevel: "medium"}
}

func (t *WriteFileTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path":    map[string]any{"type": "string", "description": "File path to write."},
			"content": map[string]any{"type": "string", "description": "Content to write."},
		},
		"required": []string{"path", "content"},
	}
}

func (t *WriteFileTool) Execute(_ context.Context, args map[string]any) (tools.Result, error) {
	path, ok := tools.StringArg(args, "path")
	if !ok || path == "" {
		return tools.Result{IsError: true, Content: "path is required"}, nil
	}
	content, ok := tools.StringArg(args, "content")
	if !ok {
		return tools.Result{IsError: true, Content: "content is required (string)"}, nil
	}

	// Validate path against security policy
	if t.cfg != nil {
		if err := validateWritePath(path, t.cfg.Security.BlockedWritePaths, t.cfg.Security.AllowedWritePaths); err != nil {
			return tools.Result{IsError: true, Content: err.Error()}, nil
		}
	}

	if err := atomicWriteFile(path, []byte(content), 0o644); err != nil {
		return tools.Result{IsError: true, Content: fmt.Sprintf("write: %v", err)}, nil
	}

	lineCount := strings.Count(content, "\n")
	if len(content) > 0 && !strings.HasSuffix(content, "\n") {
		lineCount++
	}

	// Build a simple "full add" diff
	var diff strings.Builder
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if i >= 10 { // Limit to 10 lines
			diff.WriteString(fmt.Sprintf("... (%d more lines)\n", len(lines)-10))
			break
		}
		diff.WriteString("+ " + line + "\n")
	}

	return tools.Result{
		Content: diff.String(),
		Summary: fmt.Sprintf("wrote +%d lines", lineCount),
	}, nil
}

// EditFileTool applies a string replacement in a file.
type EditFileTool struct {
	cfg *config.Config
}

func NewEditFileTool(cfg *config.Config) *EditFileTool {
	return &EditFileTool{cfg: cfg}
}

func (t *EditFileTool) Name() string        { return "edit_file" }
func (t *EditFileTool) Description() string { return "Replace a substring in a file." }

// Meta documents edit_file in the system prompt.
func (t *EditFileTool) Meta() *tools.ToolMeta {
	return &tools.ToolMeta{
		Category:    "edit",
		DisplayName: "Edit",
		InjectOrder: 10,
		WhenToUse:   "Localized changes to an existing file you have already read. Preserve the exact indentation of the surrounding code.",
		UsageByFamily: map[string]string{
			"gemini3": "Gemini 3: emit old_str/new_str as plain strings — never JSON-escape the backslashes twice; verify uniqueness before relying on replace_all.",
		},
		WhenNotTo: "Never write whole files with this; use `write_file` for full-content replacement or `create_file` for new paths. For several edits in one file, prefer `multi_edit` over repeated calls.",
		Usage: "Performs exact string replacement: `old_str` must match the file content EXACTLY, once.\n" +
			"The call FAILS if old_str is not unique — add more surrounding context to disambiguate, or set `replace_all` only when every occurrence should change.",
		Examples: [][2]string{
			{"edit_file {\"path\":\"a.go\",\"old_str\":\"func main() {\",\"new_str\":\"func main() {\\n\\tlog.SetFlags(0)\"}", "\"old_str\": \"{\" (matches dozens of places, call fails)"},
		},
	}
}
func (t *EditFileTool) RequiresConfirmation(mode string) bool {
	return mode == "edit" || mode == "plan"
}
func (t *EditFileTool) IsConcurrencySafe(args map[string]any) bool {
	// Editing is not safe if the same file is being edited elsewhere
	return false
}
func (t *EditFileTool) IsReadOnly(args map[string]any) bool { return false }
func (t *EditFileTool) IsDestructive(args map[string]any) bool {
	// Check if removing substantial code
	if oldStr, ok := tools.StringArg(args, "old_str"); ok {
		lines := strings.Count(oldStr, "\n")
		return lines > 10 // Removing more than 10 lines is destructive
	}
	return false
}
func (t *EditFileTool) EstimatedCost() tools.ToolCost {
	return tools.ToolCost{TokensApprox: 150, LatencyMs: 80, RiskLevel: "medium"}
}

func (t *EditFileTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{"type": "string"},
			"old_str": map[string]any{
				"type":        "string",
				"description": "Exact string to replace.",
			},
			"new_str": map[string]any{
				"type":        "string",
				"description": "Replacement string.",
			},
			"replace_all": map[string]any{
				"type":        "boolean",
				"description": "Replace all occurrences instead of the first occurrence.",
			},
		},
		"required": []string{"path", "old_str", "new_str"},
	}
}

func (t *EditFileTool) Execute(_ context.Context, args map[string]any) (tools.Result, error) {
	path, pathOk := tools.StringArg(args, "path")
	oldStr, oldOk := tools.StringArg(args, "old_str")
	newStr, newOk := tools.StringArg(args, "new_str")
	replaceAll := false
	if v, set := tools.ArgBool(args, "replace_all"); set {
		replaceAll = v
	}
	if !pathOk || path == "" || !oldOk || oldStr == "" || !newOk {
		return tools.Result{IsError: true, Content: "path, old_str, and new_str are required"}, nil
	}

	// Validate path against security policy
	if t.cfg != nil {
		if err := validateWritePath(path, t.cfg.Security.BlockedWritePaths, t.cfg.Security.AllowedWritePaths); err != nil {
			return tools.Result{IsError: true, Content: err.Error()}, nil
		}
	}

	// Get original file permissions before reading
	info, err := os.Stat(path)
	if err != nil {
		return tools.Result{IsError: true, Content: fmt.Sprintf("stat: %v", err)}, nil
	}
	originalPerm := info.Mode().Perm()

	data, err := os.ReadFile(path)
	if err != nil {
		return tools.Result{IsError: true, Content: fmt.Sprintf("read: %v", err)}, nil
	}
	original := string(data)
	if !strings.Contains(original, oldStr) {
		return tools.Result{IsError: true, Content: "old_str not found in file"}, nil
	}

	var result string
	replaced := 1
	if replaceAll {
		replaced = strings.Count(original, oldStr)
		result = strings.ReplaceAll(original, oldStr, newStr)
	} else {
		result = strings.Replace(original, oldStr, newStr, 1)
	}

	// NEW: Pre-flight validation with diagnostics
	beforeDiags := diagnostics.Analyze(path, original)
	afterDiags := diagnostics.Analyze(path, result)
	delta := diagnostics.Compare(beforeDiags, afterDiags)
	recoveryReport := diagnostics.RecoverDiagnostics(afterDiags)

	if delta.IntroducedCount > 0 {
		// Block the edit - it would introduce new errors
		var msg strings.Builder
		msg.WriteString("═══════════════════════════════════\n")
		msg.WriteString("[VALIDATION FAILED ✗]\n\n")
		msg.WriteString("Impact:\n")
		msg.WriteString(fmt.Sprintf("  Fixed:      %d error(s)\n", delta.FixedCount))
		msg.WriteString(fmt.Sprintf("  Introduced: %d NEW error(s)\n", delta.IntroducedCount))
		for _, d := range delta.Introduced {
			msg.WriteString(fmt.Sprintf("    Line %d: %s - %s\n", d.Line, d.Code, d.Message))
		}
		if recoveryReport.UserMessage != "" {
			msg.WriteString("\nRecovery guidance:\n")
			msg.WriteString(recoveryReport.UserMessage)
			msg.WriteString("\n")
		}
		msg.WriteString("\nChange was NOT applied. File unchanged.\n")
		msg.WriteString("Try again with a different edit.\n")
		msg.WriteString("═══════════════════════════════════\n")
		return tools.Result{IsError: true, Content: msg.String()}, nil
	}

	// Safe to write
	if err := atomicWriteFile(path, []byte(result), originalPerm); err != nil {
		return tools.Result{IsError: true, Content: fmt.Sprintf("write: %v", err)}, nil
	}

	oldLines := strings.Count(oldStr, "\n")
	newLines := strings.Count(newStr, "\n")

	totalAdded := newLines * replaced
	totalRemoved := oldLines * replaced

	// Build a simple diff snippet
	var diff strings.Builder
	diff.WriteString("═══════════════════════════════════\n")
	diff.WriteString("[VALIDATION PASSED ✓]\n\n")
	diff.WriteString("Impact:\n")
	diff.WriteString(fmt.Sprintf("  Fixed:      %d error(s)\n", delta.FixedCount))
	diff.WriteString(fmt.Sprintf("  Introduced: %d error(s)\n", delta.IntroducedCount))
	diff.WriteString(fmt.Sprintf("  Remaining:  %d error(s) in file\n", len(afterDiags)))
	diff.WriteString("\nChange has been applied to disk.\n")
	diff.WriteString("═══════════════════════════════════\n")

	return tools.Result{
		Content: diff.String(),
		Summary: fmt.Sprintf("applied +%d -%d lines", totalAdded, totalRemoved),
	}, nil
}

func pluralS(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
