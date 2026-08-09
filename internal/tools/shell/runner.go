package shell

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/iSundram/Automergent/internal/tools"
)

// defaultSensitivePatterns lists env var name patterns that are stripped before
// executing shell commands to prevent credential leakage.
var defaultSensitivePatterns = []string{
	"*_SECRET", "*_PASSWORD", "*_TOKEN", "*_KEY",
	"AWS_*", "GEMINI_*",
	"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "NO_PROXY",
	"http_proxy", "https_proxy", "all_proxy", "no_proxy",
}

// filterEnv removes env vars whose names match any of the given glob patterns.
// It returns a new slice of env strings (KEY=VALUE) with matches removed.
func filterEnv(env []string, patterns []string) []string {
	filtered := env[:0:len(env)]
	for _, e := range env {
		name := e
		if idx := strings.IndexByte(e, '='); idx >= 0 {
			name = e[:idx]
		}
		blocked := false
		for _, pat := range patterns {
			if matchGlob(pat, name) {
				blocked = true
				break
			}
		}
		if !blocked {
			filtered = append(filtered, e)
		}
	}
	return filtered
}

// matchGlob implements simple shell-style glob matching (only * wildcard).
func matchGlob(pattern, name string) bool {
	if pattern == "*" {
		return true
	}
	// Use stdlib path matching via a simple recursive approach
	for len(pattern) > 0 {
		if pattern[0] == '*' {
			// Skip consecutive stars
			for len(pattern) > 0 && pattern[0] == '*' {
				pattern = pattern[1:]
			}
			if len(pattern) == 0 {
				return true
			}
			for i := 0; i <= len(name); i++ {
				if matchGlob(pattern, name[i:]) {
					return true
				}
			}
			return false
		}
		if len(name) == 0 || pattern[0] != name[0] {
			return false
		}
		pattern = pattern[1:]
		name = name[1:]
	}
	return len(name) == 0
}

// RunnerTool executes shell commands.
type RunnerTool struct {
	timeout          time.Duration
	stripEnvPatterns []string
}

func NewRunnerTool(timeout time.Duration) *RunnerTool {
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	return &RunnerTool{
		timeout:          timeout,
		stripEnvPatterns: defaultSensitivePatterns,
	}
}

func (t *RunnerTool) Name() string        { return "run_command" }
func (t *RunnerTool) Description() string { return "Execute a shell command and return its output." }
func (t *RunnerTool) RequiresConfirmation(mode string) bool {
	return mode == "plan" || mode == "edit"
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
	command, cmdOk := tools.StringArg(args, "command")
	cwd, _ := tools.StringArg(args, "cwd")
	timeoutStr, _ := tools.StringArg(args, "timeout")
	if !cmdOk || command == "" {
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

	// Strip sensitive environment variables before execution
	cmd.Env = filterEnv(os.Environ(), t.stripEnvPatterns)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	output := stdout.String()
	if stderr.Len() > 0 {
		output += "\n[stderr]\n" + stderr.String()
	}
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return tools.Result{
				IsError: true,
				Content: fmt.Sprintf("command timed out after %s", timeout),
			}, nil
		}
		return tools.Result{
			IsError: true,
			Content: fmt.Sprintf("command failed: %v\n%s", err, output),
		}, nil
	}
	return tools.Result{Content: output}, nil
}
