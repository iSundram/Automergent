package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/iSundram/Automergent/internal/ai"
	"github.com/iSundram/Automergent/internal/config"
	"github.com/iSundram/Automergent/internal/tools"
)

const defaultHookTimeout = 10 * time.Second

// runPreToolHooks executes configured PreToolUse hooks. A non-zero exit from
// any hook vetoes the call; its stderr (or stdout) becomes the block reason.
func (a *Agent) runPreToolHooks(ctx context.Context, tc ai.ToolCall) (blocked bool, reason string) {
	return a.runToolHooks(ctx, tc, nil, a.cfg.Hooks.PreToolUse, "PreToolUse")
}

// runPostToolHooks executes PostToolUse hooks. Failures are surfaced as
// notifications and never affect the tool result.
func (a *Agent) runPostToolHooks(ctx context.Context, tc ai.ToolCall, res tools.Result) {
	_, _ = a.runToolHooks(ctx, tc, &res, a.cfg.Hooks.PostToolUse, "PostToolUse")
}

func (a *Agent) runToolHooks(ctx context.Context, tc ai.ToolCall, res *tools.Result, hooks []config.Hook, phase string) (blocked bool, reason string) {
	if len(hooks) == 0 || tc.Name == "" {
		return false, ""
	}
	env := a.hookEnv(tc, res)
	for _, hook := range hooks {
		command := strings.TrimSpace(hook.Command)
		if command == "" {
			continue
		}
		timeout := defaultHookTimeout
		if hook.Timeout != "" {
			if d, err := time.ParseDuration(hook.Timeout); err == nil && d > 0 {
				timeout = d
			}
		}
		hookCtx, cancel := context.WithTimeout(ctx, timeout)
		cmd := exec.CommandContext(hookCtx, "/bin/sh", "-c", command)
		cmd.Env = append(os.Environ(), env...)
		out, err := cmd.CombinedOutput()
		cancel()
		output := strings.TrimSpace(string(out))
		switch {
		case ctx.Err() != nil:
			// Parent cancelled: stop everything, treat as veto for pre-hooks.
			return true, "cancelled"
		case err != nil && phase == "PreToolUse":
			msg := output
			if msg == "" {
				msg = err.Error()
			}
			a.Emit(EventNotify, map[string]any{
				"level":   "warn",
				"title":   "hook",
				"message": fmt.Sprintf("%s hook %q blocked %s: %s", phase, hookLabel(hook), tc.Name, firstLineStr(msg)),
			})
			return true, fmt.Sprintf("blocked by hook %q: %s", hookLabel(hook), msg)
		case err != nil:
			a.Emit(EventNotify, map[string]any{
				"level":   "warn",
				"title":   "hook",
				"message": fmt.Sprintf("%s hook %q failed on %s: %s", phase, hookLabel(hook), tc.Name, firstLineStr(output)),
			})
		}
	}
	return false, ""
}

func hookLabel(hook config.Hook) string {
	if strings.TrimSpace(hook.Name) != "" {
		return hook.Name
	}
	return "unnamed"
}

// hookEnv builds the env var set handed to every hook invocation.
func (a *Agent) hookEnv(tc ai.ToolCall, res *tools.Result) []string {
	argsJSON, _ := json.Marshal(tc.Args)
	env := []string{
		"AUTOMERGENT_TOOL_NAME=" + tc.Name,
		"AUTOMERGENT_TOOL_ARGS=" + string(argsJSON),
		"AUTOMERGENT_PROJECT_DIR=" + a.workDir,
	}
	if res != nil {
		truncated := res.Content
		if len(truncated) > 4096 {
			truncated = truncated[:4096]
		}
		flag := "false"
		if res.IsError {
			flag = "true"
		}
		env = append(env,
			"AUTOMERGENT_TOOL_RESULT="+truncated,
			"AUTOMERGENT_TOOL_ERROR="+flag,
		)
	}
	return env
}

func firstLineStr(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
