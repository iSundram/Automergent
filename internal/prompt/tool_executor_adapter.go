package prompt

import (
	"context"
	"fmt"

	"github.com/iSundram/Automergent/internal/tools"
)

// ToolExecutorAdapter adapts tools.Registry to ToolExecutor interface.
type ToolExecutorAdapter struct {
	registry *tools.Registry
}

func NewToolExecutorAdapter(registry *tools.Registry) *ToolExecutorAdapter {
	return &ToolExecutorAdapter{registry: registry}
}

func (a *ToolExecutorAdapter) Glob(ctx context.Context, pattern, workingDir string) (string, error) {
	tool, ok := a.registry.Get("glob")
	if !ok {
		return "", fmt.Errorf("tool not found: glob")
	}
	result, err := tool.Execute(ctx, map[string]any{"pattern": pattern})
	if err != nil {
		return "", err
	}
	return result.Content, nil
}

func (a *ToolExecutorAdapter) Grep(ctx context.Context, pattern, workingDir string) (string, error) {
	tool, ok := a.registry.Get("grep")
	if !ok {
		return "", fmt.Errorf("tool not found: grep")
	}
	result, err := tool.Execute(ctx, map[string]any{"pattern": pattern})
	if err != nil {
		return "", err
	}
	return result.Content, nil
}

func (a *ToolExecutorAdapter) Read(ctx context.Context, path, workingDir string) (string, error) {
	tool, ok := a.registry.Get("read_file")
	if !ok {
		return "", fmt.Errorf("tool not found: read_file")
	}
	result, err := tool.Execute(ctx, map[string]any{"path": path})
	if err != nil {
		return "", err
	}
	return result.Content, nil
}

func (a *ToolExecutorAdapter) Bash(ctx context.Context, command, workingDir string) (string, error) {
	tool, ok := a.registry.Get("bash")
	if !ok {
		return "", fmt.Errorf("tool not found: bash")
	}
	result, err := tool.Execute(ctx, map[string]any{"command": command, "mode": "sync"})
	if err != nil {
		return "", err
	}
	return result.Content, nil
}
