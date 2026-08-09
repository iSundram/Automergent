package filesystem

import (
	"context"
	"fmt"
	"os"

	"github.com/iSundram/Automergent/internal/tools"
)

// ReadFileTool reads the contents of a file.
type ReadFileTool struct{}

func (t *ReadFileTool) Name() string                          { return "read_file" }
func (t *ReadFileTool) Description() string                   { return "Read the contents of a file from disk." }
func (t *ReadFileTool) RequiresConfirmation(mode string) bool { return false }

func (t *ReadFileTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Absolute or relative path to the file.",
			},
		},
		"required": []string{"path"},
	}
}

func (t *ReadFileTool) Execute(_ context.Context, args map[string]any) (tools.Result, error) {
	path, ok := args["path"].(string)
	if !ok || path == "" {
		return tools.Result{IsError: true, Content: "path is required"}, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return tools.Result{IsError: true, Content: fmt.Sprintf("error reading file: %v", err)}, nil
	}
	return tools.Result{Content: string(data)}, nil
}

// ListDirectoryTool lists the files in a directory.
type ListDirectoryTool struct{}

func (t *ListDirectoryTool) Name() string                          { return "list_directory" }
func (t *ListDirectoryTool) Description() string                   { return "List files and directories at a path." }
func (t *ListDirectoryTool) RequiresConfirmation(mode string) bool { return false }

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
	path, ok := args["path"].(string)
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
