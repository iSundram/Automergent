package filesystem

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
type WriteFileTool struct{}

func (t *WriteFileTool) Name() string { return "write_file" }
func (t *WriteFileTool) Description() string {
	return "Write content to a file, creating it if needed."
}
func (t *WriteFileTool) RequiresConfirmation(mode string) bool {
	return mode == "edit" || mode == "plan"
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
	if err := atomicWriteFile(path, []byte(content), 0o644); err != nil {
		return tools.Result{IsError: true, Content: fmt.Sprintf("write: %v", err)}, nil
	}
	return tools.Result{Content: fmt.Sprintf("wrote %d bytes to %s", len(content), path)}, nil
}

// PatchFileTool applies a string replacement in a file.
type PatchFileTool struct{}

func (t *PatchFileTool) Name() string        { return "patch_file" }
func (t *PatchFileTool) Description() string { return "Replace a substring in a file." }
func (t *PatchFileTool) RequiresConfirmation(mode string) bool {
	return mode == "edit" || mode == "plan"
}

func (t *PatchFileTool) Schema() map[string]any {
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

func (t *PatchFileTool) Execute(_ context.Context, args map[string]any) (tools.Result, error) {
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
	if err := atomicWriteFile(path, []byte(result), 0o644); err != nil {
		return tools.Result{IsError: true, Content: fmt.Sprintf("write: %v", err)}, nil
	}
	return tools.Result{Content: fmt.Sprintf("patched %s (%d replacement%s)", path, replaced, pluralS(replaced))}, nil
}

func pluralS(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
