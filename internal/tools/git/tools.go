package git

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/iSundram/Automergent/internal/tools"
)

const (
	minStatusLineLen          = 2   // minimum chars for git status short line (X Y)
	defaultCommitHashShortLen = 7   // characters to show for short commit hash
	defaultLogCount           = 10  // default number of commits to show
	defaultBlameRange         = 100 // default number of lines for blame when end not set
)

// StatusTool returns the current git status.
type StatusTool struct{}

func (t *StatusTool) Name() string                               { return "git_status" }
func (t *StatusTool) Description() string                        { return "Get the current git status of the repository." }
func (t *StatusTool) RequiresConfirmation(mode string) bool      { return false }
func (t *StatusTool) IsConcurrencySafe(args map[string]any) bool { return true }
func (t *StatusTool) IsReadOnly(args map[string]any) bool        { return true }
func (t *StatusTool) IsDestructive(args map[string]any) bool     { return false }
func (t *StatusTool) EstimatedCost() tools.ToolCost {
	return tools.ToolCost{
		TokensApprox: 100,
		LatencyMs:    50,
		RiskLevel:    "low",
	}
}

func (t *StatusTool) Schema() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}
}

func (t *StatusTool) Execute(ctx context.Context, _ map[string]any) (tools.Result, error) {
	res, err := runGit(ctx, "status", "--short")
	if err != nil {
		return res, err
	}

	lines := strings.Split(res.Content, "\n")
	staged := 0
	modified := 0
	untracked := 0

	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if len(line) < minStatusLineLen {
			continue
		}
		if line[0] != ' ' && line[0] != '?' {
			staged++
		}
		if line[1] != ' ' {
			modified++
		}
		if strings.HasPrefix(line, "??") {
			untracked++
		}
	}

	summary := "clean"
	// determine if there were any non-empty lines
	hasAny := false
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			hasAny = true
			break
		}
	}
	if hasAny {
		summary = fmt.Sprintf("%d staged, %d modified", staged, modified)
		if untracked > 0 {
			summary += fmt.Sprintf(", %d untracked", untracked)
		}
	}
	res.Summary = summary
	return res, nil
}

// DiffTool shows the current diff.
type DiffTool struct{}

func (t *DiffTool) Name() string                               { return "git_diff" }
func (t *DiffTool) Description() string                        { return "Show the current git diff." }
func (t *DiffTool) RequiresConfirmation(mode string) bool      { return false }
func (t *DiffTool) IsConcurrencySafe(args map[string]any) bool { return true }
func (t *DiffTool) IsReadOnly(args map[string]any) bool        { return true }
func (t *DiffTool) IsDestructive(args map[string]any) bool     { return false }
func (t *DiffTool) EstimatedCost() tools.ToolCost {
	return tools.ToolCost{
		TokensApprox: 200,
		LatencyMs:    150,
		RiskLevel:    "low",
	}
}

func (t *DiffTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"file": map[string]any{"type": "string", "description": "Optional file to diff."},
		},
	}
}

func (t *DiffTool) Execute(ctx context.Context, args map[string]any) (tools.Result, error) {
	var res tools.Result
	var err error
	file, _ := tools.StringArg(args, "file")
	if file != "" {
		res, err = runGit(ctx, "diff", "--", file)
	} else {
		res, err = runGit(ctx, "diff")
	}

	if err != nil {
		return res, err
	}

	plus := strings.Count(res.Content, "\n+")
	minus := strings.Count(res.Content, "\n-")
	res.Summary = fmt.Sprintf("diff +%d -%d lines", plus, minus)
	return res, nil
}

// LogTool shows recent git log entries.
type LogTool struct{}

func (t *LogTool) Name() string                               { return "git_log" }
func (t *LogTool) Description() string                        { return "Show recent git commit history." }
func (t *LogTool) RequiresConfirmation(mode string) bool      { return false }
func (t *LogTool) IsConcurrencySafe(args map[string]any) bool { return true }
func (t *LogTool) IsReadOnly(args map[string]any) bool        { return true }
func (t *LogTool) IsDestructive(args map[string]any) bool     { return false }
func (t *LogTool) EstimatedCost() tools.ToolCost {
	return tools.ToolCost{
		TokensApprox: 150,
		LatencyMs:    100,
		RiskLevel:    "low",
	}
}

func (t *LogTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"n": map[string]any{"type": "integer", "description": "Number of commits to show."},
		},
	}
}

func (t *LogTool) Execute(ctx context.Context, args map[string]any) (tools.Result, error) {
	n := defaultLogCount
	if ni, ok := tools.ArgInt(args, "n"); ok && ni > 0 {
		n = ni
	}
	res, err := runGit(ctx, "log", fmt.Sprintf("--max-count=%d", n), "--oneline")
	if err != nil {
		return res, err
	}

	count := 0
	for _, l := range strings.Split(res.Content, "\n") {
		if strings.TrimSpace(l) != "" {
			count++
		}
	}
	res.Summary = fmt.Sprintf("read %d commits", count)
	return res, nil
}

func runGit(ctx context.Context, args ...string) (tools.Result, error) {
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		content := fmt.Sprintf("git %v: %v\n%s", args, err, stderr.String())
		return tools.Result{IsError: true, Content: content}, fmt.Errorf(content)
	}
	return tools.Result{Content: stdout.String()}, nil
}
