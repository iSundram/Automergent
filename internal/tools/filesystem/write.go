package filesystem

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/iSundram/Automergent/internal/tools"
)

// WriteFileTool writes content to a file.
type WriteFileTool struct{}

func (t *WriteFileTool) Name() string { return "write_file" }
func (t *WriteFileTool) Description() string {
	return "Write content to a file, creating it if needed."
}
func (t *WriteFileTool) RequiresConfirmation(mode string) bool {
	return mode == "suggest"
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
	path, _ := args["path"].(string)
	content, _ := args["content"].(string)
	if path == "" {
		return tools.Result{IsError: true, Content: "path is required"}, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return tools.Result{IsError: true, Content: fmt.Sprintf("mkdir: %v", err)}, nil
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return tools.Result{IsError: true, Content: fmt.Sprintf("write: %v", err)}, nil
	}
	return tools.Result{Content: fmt.Sprintf("wrote %d bytes to %s", len(content), path)}, nil
}

// PatchFileTool applies a string replacement in a file.
type PatchFileTool struct{}

func (t *PatchFileTool) Name() string        { return "patch_file" }
func (t *PatchFileTool) Description() string { return "Replace a substring in a file." }
func (t *PatchFileTool) RequiresConfirmation(mode string) bool {
	return mode == "suggest"
}

func (t *PatchFileTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path":    map[string]any{"type": "string"},
			"old_str": map[string]any{"type": "string", "description": "Exact string to replace."},
			"new_str": map[string]any{"type": "string", "description": "Replacement string."},
		},
		"required": []string{"path", "old_str", "new_str"},
	}
}

func (t *PatchFileTool) Execute(_ context.Context, args map[string]any) (tools.Result, error) {
	path, _ := args["path"].(string)
	oldStr, _ := args["old_str"].(string)
	newStr, _ := args["new_str"].(string)
	if path == "" || oldStr == "" {
		return tools.Result{IsError: true, Content: "path and old_str are required"}, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return tools.Result{IsError: true, Content: fmt.Sprintf("read: %v", err)}, nil
	}
	original := string(data)
	var found bool
	result := ""
	for i := 0; i < len(original); {
		idx := indexOf(original[i:], oldStr)
		if idx < 0 {
			result += original[i:]
			break
		}
		result += original[i : i+idx]
		result += newStr
		i += idx + len(oldStr)
		found = true
		break // replace only first occurrence
	}
	if !found {
		return tools.Result{IsError: true, Content: "old_str not found in file"}, nil
	}
	if err := os.WriteFile(path, []byte(result), 0o644); err != nil {
		return tools.Result{IsError: true, Content: fmt.Sprintf("write: %v", err)}, nil
	}
	return tools.Result{Content: fmt.Sprintf("patched %s", path)}, nil
}

func indexOf(s, sub string) int {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
