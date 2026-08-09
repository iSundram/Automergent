package shell

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/iSundram/Automergent/internal/tools"
)

// RunnerTool executes shell commands.
type RunnerTool struct {
	timeout time.Duration
}

func NewRunnerTool(timeout time.Duration) *RunnerTool {
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	return &RunnerTool{timeout: timeout}
}

func (t *RunnerTool) Name() string        { return "run_command" }
func (t *RunnerTool) Description() string { return "Execute a shell command and return its output." }
func (t *RunnerTool) RequiresConfirmation(mode string) bool {
	return mode == "suggest" || mode == "auto-edit"
}

func (t *RunnerTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command": map[string]any{
				"type":        "string",
				"description": "Shell command to execute.",
			},
			"cwd": map[string]any{
				"type":        "string",
				"description": "Working directory for the command.",
			},
			"timeout": map[string]any{
				"type":        "string",
				"description": "Timeout, e.g. '30s'.",
			},
		},
		"required": []string{"command"},
	}
}

func (t *RunnerTool) Execute(ctx context.Context, args map[string]any) (tools.Result, error) {
	command, _ := args["command"].(string)
	cwd, _ := args["cwd"].(string)
	timeoutStr, _ := args["timeout"].(string)
	if command == "" {
		return tools.Result{IsError: true, Content: "command is required"}, nil
	}

	timeout := t.timeout
	if timeoutStr != "" {
		if d, err := time.ParseDuration(timeoutStr); err == nil {
			timeout = d
		}
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "-c", command)
	if cwd != "" {
		cmd.Dir = cwd
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	output := stdout.String()
	if stderr.Len() > 0 {
		output += "\n[stderr]\n" + stderr.String()
	}
	if err != nil {
		return tools.Result{
			IsError: true,
			Content: fmt.Sprintf("command failed: %v\n%s", err, output),
		}, nil
	}
	return tools.Result{Content: output}, nil
}
