package filesystem

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/iSundram/Automergent/internal/tools"
)

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
	path, ok := args["path"].(string)
	if !ok || path == "" {
		return tools.Result{IsError: true, Content: "path is required"}, nil
	}

	binary, err := isBinaryFile(path)
	if err != nil {
		// File might not exist — let os.ReadFile produce the error
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

	data, err := os.ReadFile(path)
	if err != nil {
		return tools.Result{IsError: true, Content: fmt.Sprintf("error reading file: %v", err)}, nil
	}

	content := string(data)

	// Optional line range filtering
	startLine, hasStart := args["start_line"].(float64)
	endLine, hasEnd := args["end_line"].(float64)
	if hasStart || hasEnd {
		lines := strings.Split(content, "\n")
		start := 1
		end := len(lines)
		if hasStart && int(startLine) >= 1 {
			start = int(startLine)
		}
		if hasEnd && int(endLine) >= 1 && int(endLine) < end {
			end = int(endLine)
		}
		if start > end {
			start = end
		}
		if start > len(lines) {
			start = len(lines)
		}
		if end > len(lines) {
			end = len(lines)
		}
		content = strings.Join(lines[start-1:end], "\n")
	}

	return tools.Result{Content: content}, nil
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
