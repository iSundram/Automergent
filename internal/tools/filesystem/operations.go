package filesystem

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/iSundram/Automergent/internal/tools"
)

// CreateFileTool creates a new file (fails if file already exists).
type CreateFileTool struct{}

func (t *CreateFileTool) Name() string { return "create_file" }
func (t *CreateFileTool) Description() string {
	return `Create a new file with the specified content.
- Fails if the file already exists (use write_file or edit_file for existing files)
- Creates parent directories if needed
- Use this to prevent accidental overwrites`
}
func (t *CreateFileTool) RequiresConfirmation(mode string) bool {
	return mode == "edit" || mode == "plan"
}

func (t *CreateFileTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Path for the new file.",
			},
			"content": map[string]any{
				"type":        "string",
				"description": "Content to write to the file.",
			},
		},
		"required": []string{"path", "content"},
	}
}

func (t *CreateFileTool) Execute(_ context.Context, args map[string]any) (tools.Result, error) {
	path, ok := tools.StringArg(args, "path")
	if !ok || path == "" {
		return tools.Result{IsError: true, Content: "path is required"}, nil
	}

	content, ok := tools.StringArg(args, "content")
	if !ok {
		return tools.Result{IsError: true, Content: "content is required"}, nil
	}

	// Check if file already exists
	if _, err := os.Stat(path); err == nil {
		return tools.Result{
			IsError: true,
			Content: fmt.Sprintf("file already exists: %s (use write_file or edit_file to modify existing files)", path),
		}, nil
	}

	// Create parent directories
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return tools.Result{IsError: true, Content: fmt.Sprintf("failed to create directories: %v", err)}, nil
	}

	// Write file atomically
	if err := atomicWriteFile(path, []byte(content), 0o644); err != nil {
		return tools.Result{IsError: true, Content: fmt.Sprintf("failed to create file: %v", err)}, nil
	}

	return tools.Result{
		Content: fmt.Sprintf("created %s (%d bytes)", path, len(content)),
	}, nil
}

// DeleteFileTool deletes a file or directory.
type DeleteFileTool struct{}

func (t *DeleteFileTool) Name() string { return "delete_file" }
func (t *DeleteFileTool) Description() string {
	return `Delete a file or directory.
- For directories, use recursive=true to delete non-empty directories
- Requires confirmation in edit/plan modes`
}
func (t *DeleteFileTool) RequiresConfirmation(mode string) bool {
	return true // Always require confirmation for deletes
}

func (t *DeleteFileTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Path to file or directory to delete.",
			},
			"recursive": map[string]any{
				"type":        "boolean",
				"description": "If true, delete directories recursively (default: false).",
			},
		},
		"required": []string{"path"},
	}
}

func (t *DeleteFileTool) Execute(_ context.Context, args map[string]any) (tools.Result, error) {
	path, ok := tools.StringArg(args, "path")
	if !ok || path == "" {
		return tools.Result{IsError: true, Content: "path is required"}, nil
	}

	recursive := false
	if v, ok := tools.ArgBool(args, "recursive"); ok {
		recursive = v
	}

	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return tools.Result{IsError: true, Content: fmt.Sprintf("path does not exist: %s", path)}, nil
		}
		return tools.Result{IsError: true, Content: fmt.Sprintf("error accessing path: %v", err)}, nil
	}

	if info.IsDir() {
		if recursive {
			if err := os.RemoveAll(path); err != nil {
				return tools.Result{IsError: true, Content: fmt.Sprintf("failed to delete directory: %v", err)}, nil
			}
			return tools.Result{Content: fmt.Sprintf("deleted directory (recursive): %s", path)}, nil
		}
		if err := os.Remove(path); err != nil {
			return tools.Result{
				IsError: true,
				Content: fmt.Sprintf("failed to delete directory (not empty? use recursive=true): %v", err),
			}, nil
		}
		return tools.Result{Content: fmt.Sprintf("deleted directory: %s", path)}, nil
	}

	if err := os.Remove(path); err != nil {
		return tools.Result{IsError: true, Content: fmt.Sprintf("failed to delete file: %v", err)}, nil
	}

	return tools.Result{Content: fmt.Sprintf("deleted file: %s", path)}, nil
}

// MoveFileTool moves or renames a file/directory.
type MoveFileTool struct{}

func (t *MoveFileTool) Name() string { return "move_file" }
func (t *MoveFileTool) Description() string {
	return `Move or rename a file or directory.
- Creates destination parent directories if needed
- Fails if destination already exists (use overwrite=true to replace)`
}
func (t *MoveFileTool) RequiresConfirmation(mode string) bool {
	return mode == "edit" || mode == "plan"
}

func (t *MoveFileTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"source": map[string]any{
				"type":        "string",
				"description": "Source path.",
			},
			"destination": map[string]any{
				"type":        "string",
				"description": "Destination path.",
			},
			"overwrite": map[string]any{
				"type":        "boolean",
				"description": "If true, overwrite existing destination (default: false).",
			},
		},
		"required": []string{"source", "destination"},
	}
}

func (t *MoveFileTool) Execute(_ context.Context, args map[string]any) (tools.Result, error) {
	source, ok := tools.StringArg(args, "source")
	if !ok || source == "" {
		return tools.Result{IsError: true, Content: "source is required"}, nil
	}

	dest, ok := tools.StringArg(args, "destination")
	if !ok || dest == "" {
		return tools.Result{IsError: true, Content: "destination is required"}, nil
	}

	overwrite := false
	if v, ok := tools.ArgBool(args, "overwrite"); ok {
		overwrite = v
	}

	// Check source exists
	if _, err := os.Stat(source); err != nil {
		if os.IsNotExist(err) {
			return tools.Result{IsError: true, Content: fmt.Sprintf("source does not exist: %s", source)}, nil
		}
		return tools.Result{IsError: true, Content: fmt.Sprintf("error accessing source: %v", err)}, nil
	}

	// Check destination
	if _, err := os.Stat(dest); err == nil && !overwrite {
		return tools.Result{
			IsError: true,
			Content: fmt.Sprintf("destination already exists: %s (use overwrite=true to replace)", dest),
		}, nil
	}

	// Create destination parent directories
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return tools.Result{IsError: true, Content: fmt.Sprintf("failed to create destination directory: %v", err)}, nil
	}

	if err := os.Rename(source, dest); err != nil {
		return tools.Result{IsError: true, Content: fmt.Sprintf("failed to move: %v", err)}, nil
	}

	return tools.Result{Content: fmt.Sprintf("moved %s -> %s", source, dest)}, nil
}

// CopyFileTool copies a file or directory.
type CopyFileTool struct{}

func (t *CopyFileTool) Name() string { return "copy_file" }
func (t *CopyFileTool) Description() string {
	return `Copy a file or directory.
- For directories, use recursive=true to copy contents
- Creates destination parent directories if needed`
}
func (t *CopyFileTool) RequiresConfirmation(mode string) bool {
	return mode == "edit" || mode == "plan"
}

func (t *CopyFileTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"source": map[string]any{
				"type":        "string",
				"description": "Source path.",
			},
			"destination": map[string]any{
				"type":        "string",
				"description": "Destination path.",
			},
			"recursive": map[string]any{
				"type":        "boolean",
				"description": "If true, copy directories recursively (default: false).",
			},
			"overwrite": map[string]any{
				"type":        "boolean",
				"description": "If true, overwrite existing files (default: false).",
			},
		},
		"required": []string{"source", "destination"},
	}
}

func (t *CopyFileTool) Execute(_ context.Context, args map[string]any) (tools.Result, error) {
	source, ok := tools.StringArg(args, "source")
	if !ok || source == "" {
		return tools.Result{IsError: true, Content: "source is required"}, nil
	}

	dest, ok := tools.StringArg(args, "destination")
	if !ok || dest == "" {
		return tools.Result{IsError: true, Content: "destination is required"}, nil
	}

	recursive := false
	if v, ok := tools.ArgBool(args, "recursive"); ok {
		recursive = v
	}

	overwrite := false
	if v, ok := tools.ArgBool(args, "overwrite"); ok {
		overwrite = v
	}

	info, err := os.Stat(source)
	if err != nil {
		if os.IsNotExist(err) {
			return tools.Result{IsError: true, Content: fmt.Sprintf("source does not exist: %s", source)}, nil
		}
		return tools.Result{IsError: true, Content: fmt.Sprintf("error accessing source: %v", err)}, nil
	}

	if info.IsDir() {
		if !recursive {
			return tools.Result{
				IsError: true,
				Content: "source is a directory; use recursive=true to copy directories",
			}, nil
		}
		count, err := copyDirRecursive(source, dest, overwrite)
		if err != nil {
			return tools.Result{IsError: true, Content: fmt.Sprintf("copy failed: %v", err)}, nil
		}
		return tools.Result{Content: fmt.Sprintf("copied directory %s -> %s (%d files)", source, dest, count)}, nil
	}

	// Copy single file
	if err := copyFile(source, dest, overwrite); err != nil {
		return tools.Result{IsError: true, Content: fmt.Sprintf("copy failed: %v", err)}, nil
	}

	return tools.Result{Content: fmt.Sprintf("copied %s -> %s", source, dest)}, nil
}

func copyFile(src, dst string, overwrite bool) error {
	// Check destination
	if _, err := os.Stat(dst); err == nil && !overwrite {
		return fmt.Errorf("destination already exists: %s", dst)
	}

	// Create parent directories
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}

	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open source: %w", err)
	}
	defer srcFile.Close()

	srcInfo, err := srcFile.Stat()
	if err != nil {
		return fmt.Errorf("stat source: %w", err)
	}

	dstFile, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, srcInfo.Mode())
	if err != nil {
		return fmt.Errorf("create destination: %w", err)
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return fmt.Errorf("copy: %w", err)
	}

	return dstFile.Sync()
}

func copyDirRecursive(src, dst string, overwrite bool) (int, error) {
	count := 0
	err := filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		relPath, _ := filepath.Rel(src, path)
		dstPath := filepath.Join(dst, relPath)

		if d.IsDir() {
			return os.MkdirAll(dstPath, 0o755)
		}

		if err := copyFile(path, dstPath, overwrite); err != nil {
			return err
		}
		count++
		return nil
	})

	return count, err
}
