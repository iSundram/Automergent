// Package git provides the native git tool suite. Commands are executed with
// fixed argv templates (exec.Command("git", args...)) — user input never
// reaches a shell, so injection is structurally impossible.
package git

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/iSundram/Automergent/internal/tools"
)

const (
	maxOutputChars = 8000
	gitTimeout     = 60 * time.Second
)

// RegisterAll registers the git tool suite.
func RegisterAll(reg *tools.Registry) {
	if reg == nil {
		return
	}
	reg.Register(&statusTool{})
	reg.Register(&diffTool{})
	reg.Register(&logTool{})
	reg.Register(&addTool{})
	reg.Register(&commitTool{})
	reg.Register(&branchTool{})
	reg.Register(&checkoutTool{})
	reg.Register(&stashTool{})
}

// runGit executes one bounded git invocation.
func runGit(ctx context.Context, args ...string) tools.Result {
	ctx, cancel := context.WithTimeout(ctx, gitTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	out, err := cmd.CombinedOutput()
	content := string(out)
	if len(content) > maxOutputChars {
		content = content[:maxOutputChars] + "\n… [truncated]"
	}
	if err != nil {
		msg := strings.TrimSpace(content)
		if msg == "" {
			msg = err.Error()
		}
		return tools.Result{IsError: true, Content: fmt.Sprintf("git %s: %s", args[0], msg)}
	}
	return tools.Result{Content: content, Summary: strings.TrimSpace(firstLine(content))}
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

func meta(name, whenToUse, whenNotTo string) *tools.ToolMeta {
	return &tools.ToolMeta{
		Category:    "git",
		DisplayName: name,
		InjectOrder: 10,
		WhenToUse:   whenToUse,
		WhenNotTo:   whenNotTo,
	}
}

// --- read-only: status ---

type statusTool struct{ tools.BaseTool }

func (*statusTool) Name() string        { return "git_status" }
func (*statusTool) Description() string { return "Show the working tree status." }
func (*statusTool) Schema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{}}
}
func (*statusTool) RequiresConfirmation(string) bool      { return false }
func (*statusTool) IsConcurrencySafe(map[string]any) bool { return true }
func (*statusTool) IsReadOnly(map[string]any) bool        { return true }
func (*statusTool) IsDestructive(map[string]any) bool     { return false }
func (*statusTool) EstimatedCost() tools.ToolCost {
	return tools.ToolCost{TokensApprox: 150, LatencyMs: 100, RiskLevel: "low"}
}
func (*statusTool) Meta() *tools.ToolMeta {
	return meta("Git status", "Check what changed before staging or committing; also verifies you are inside a repository.", "")
}

func (t *statusTool) Execute(ctx context.Context, args map[string]any) (tools.Result, error) {
	res := runGit(ctx, "status", "--porcelain=v1", "--branch")
	if res.Content == "" && !res.IsError {
		res.Content = "(clean)"
		res.Summary = "clean"
	}
	return res, nil
}

// --- read-only: diff ---

type diffTool struct{ tools.BaseTool }

func (*diffTool) Name() string        { return "git_diff" }
func (*diffTool) Description() string { return "Show unstaged or staged changes as a unified diff." }
func (*diffTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"staged": map[string]any{"type": "boolean", "description": "Diff the index instead of the working tree"},
			"path":   map[string]any{"type": "string", "description": "Restrict the diff to one path"},
		},
	}
}
func (*diffTool) RequiresConfirmation(string) bool      { return false }
func (*diffTool) IsConcurrencySafe(map[string]any) bool { return true }
func (*diffTool) IsReadOnly(map[string]any) bool        { return true }
func (*diffTool) IsDestructive(map[string]any) bool     { return false }
func (*diffTool) EstimatedCost() tools.ToolCost {
	return tools.ToolCost{TokensApprox: 300, LatencyMs: 100, RiskLevel: "low"}
}
func (*diffTool) Meta() *tools.ToolMeta {
	return meta("Git diff", "Review your edits before committing — always diff before you commit.", "")
}

func (t *diffTool) Execute(ctx context.Context, args map[string]any) (tools.Result, error) {
	gitArgs := []string{"diff"}
	staged, _ := args["staged"].(bool)
	if staged {
		gitArgs = append(gitArgs, "--cached")
	}
	if p, ok := tools.StringArg(args, "path"); ok && p != "" {
		gitArgs = append(gitArgs, "--", p)
	}
	res := runGit(ctx, gitArgs...)
	if res.Content == "" && !res.IsError {
		res.Content = "(no changes)"
		res.Summary = "clean"
	}
	return res, nil
}

// --- read-only: log ---

type logTool struct{ tools.BaseTool }

func (*logTool) Name() string        { return "git_log" }
func (*logTool) Description() string { return "Show recent commit history." }
func (*logTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"limit": map[string]any{"type": "integer", "description": "Number of commits (default 20)"},
		},
	}
}
func (*logTool) RequiresConfirmation(string) bool      { return false }
func (*logTool) IsConcurrencySafe(map[string]any) bool { return true }
func (*logTool) IsReadOnly(map[string]any) bool        { return true }
func (*logTool) IsDestructive(map[string]any) bool     { return false }
func (*logTool) EstimatedCost() tools.ToolCost {
	return tools.ToolCost{TokensApprox: 250, LatencyMs: 100, RiskLevel: "low"}
}
func (*logTool) Meta() *tools.ToolMeta {
	return meta("Git log", "Inspect history for context: recent changes, blame-adjacent archaeology, release boundaries.", "")
}

func (t *logTool) Execute(ctx context.Context, args map[string]any) (tools.Result, error) {
	limit := 20
	if v, ok := args["limit"].(float64); ok && v > 0 && v <= 200 {
		limit = int(v)
	}
	format := "--pretty=format:%h %ad %an%n  %s%d%n"
	return runGit(ctx, "log", fmt.Sprintf("-n=%d", limit), format, "--date=short"), nil
}

// --- staging: add ---

type addTool struct{ tools.BaseTool }

func (*addTool) Name() string        { return "git_add" }
func (*addTool) Description() string { return "Stage files for the next commit." }
func (*addTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"paths": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "Paths to stage; use [\".\"] for everything (prefer explicit paths)",
			},
		},
		"required": []string{"paths"},
	}
}
func (*addTool) RequiresConfirmation(string) bool      { return false }
func (*addTool) IsConcurrencySafe(map[string]any) bool { return false }
func (*addTool) IsReadOnly(map[string]any) bool        { return false }
func (*addTool) EstimatedCost() tools.ToolCost {
	return tools.ToolCost{TokensApprox: 80, LatencyMs: 80, RiskLevel: "low"}
}

func (t *addTool) IsDestructive(args map[string]any) bool {
	return false // staging is reversible via git reset
}

func (t *addTool) Meta() *tools.ToolMeta {
	return meta("Git stage", "Stage exactly the files your change touched before git_commit. Prefer explicit paths over \".\".", "")
}

func (t *addTool) Execute(ctx context.Context, args map[string]any) (tools.Result, error) {
	raw, ok := args["paths"].([]any)
	if !ok || len(raw) == 0 {
		return tools.Result{IsError: true, Content: "git_add requires a non-empty `paths` array"}, nil
	}
	gitArgs := append([]string{"add", "--"}, toStrings(raw)...)
	return runGit(ctx, gitArgs...), nil
}

func toStrings(raw []any) []string {
	out := make([]string, 0, len(raw))
	for _, r := range raw {
		if s, ok := r.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	return out
}

// --- commit ---

type commitTool struct{ tools.BaseTool }

func (*commitTool) Name() string        { return "git_commit" }
func (*commitTool) Description() string { return "Create a commit from staged changes." }
func (*commitTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"message": map[string]any{"type": "string", "description": "Concise commit message matching repo style"},
		},
		"required": []string{"message"},
	}
}
func (*commitTool) RequiresConfirmation(string) bool      { return true }
func (*commitTool) IsConcurrencySafe(map[string]any) bool { return false }
func (*commitTool) IsReadOnly(map[string]any) bool        { return false }
func (*commitTool) IsDestructive(map[string]any) bool     { return false }
func (*commitTool) EstimatedCost() tools.ToolCost {
	return tools.ToolCost{TokensApprox: 100, LatencyMs: 150, RiskLevel: "medium"}
}
func (*commitTool) Meta() *tools.ToolMeta {
	return meta("Git commit", "After staging and reviewing a diff. Write a concise message that matches the repository's existing style.",
		"Do not bypass hooks or verification (no --no-verify); do not commit secrets.")
}

func (t *commitTool) Execute(ctx context.Context, args map[string]any) (tools.Result, error) {
	msg, ok := tools.StringArg(args, "message")
	if !ok || strings.TrimSpace(msg) == "" {
		return tools.Result{IsError: true, Content: "git_commit requires `message`"}, nil
	}
	return runGit(ctx, "commit", "-m", msg), nil
}

// --- branch ---

type branchTool struct{ tools.BaseTool }

func (*branchTool) Name() string        { return "git_branch" }
func (*branchTool) Description() string { return "List branches, or create one." }
func (*branchTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{"type": "string", "description": "Branch name to create (omit to list)"},
		},
	}
}
func (*branchTool) RequiresConfirmation(mode string) bool { return mode == "edit" || mode == "plan" }
func (*branchTool) IsConcurrencySafe(map[string]any) bool { return false }
func (*branchTool) IsReadOnly(map[string]any) bool        { return false }
func (*branchTool) IsDestructive(map[string]any) bool     { return false }
func (*branchTool) EstimatedCost() tools.ToolCost {
	return tools.ToolCost{TokensApprox: 80, LatencyMs: 80, RiskLevel: "low"}
}
func (*branchTool) Meta() *tools.ToolMeta {
	return meta("Git branch", "List branches for context, or create a working branch before starting a multi-file change.", "")
}

func (t *branchTool) Execute(ctx context.Context, args map[string]any) (tools.Result, error) {
	name, hasName := tools.StringArg(args, "name")
	if !hasName || name == "" {
		return runGit(ctx, "branch", "--list", "--all"), nil
	}
	return runGit(ctx, "branch", name), nil
}

// --- checkout ---

type checkoutTool struct{ tools.BaseTool }

func (*checkoutTool) Name() string { return "git_checkout" }
func (*checkoutTool) Description() string {
	return "Switch branches; optionally create the target first."
}
func (*checkoutTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"target": map[string]any{"type": "string", "description": "Branch (or commit-ish) to switch to"},
			"create": map[string]any{"type": "boolean", "description": "Create target as a new branch (-b)"},
		},
		"required": []string{"target"},
	}
}
func (*checkoutTool) RequiresConfirmation(string) bool      { return true }
func (*checkoutTool) IsConcurrencySafe(map[string]any) bool { return false }
func (*checkoutTool) IsReadOnly(map[string]any) bool        { return false }
func (*checkoutTool) IsDestructive(map[string]any) bool     { return false }
func (*checkoutTool) EstimatedCost() tools.ToolCost {
	return tools.ToolCost{TokensApprox: 100, LatencyMs: 120, RiskLevel: "medium"}
}
func (*checkoutTool) Meta() *tools.ToolMeta {
	return meta("Git checkout", "Switch to an existing or new branch. Warn the user if uncommitted work might be disturbed.", "")
}

func (t *checkoutTool) Execute(ctx context.Context, args map[string]any) (tools.Result, error) {
	target, _ := tools.StringArg(args, "target")
	if target == "" {
		return tools.Result{IsError: true, Content: "git_checkout requires `target`"}, nil
	}
	create, _ := args["create"].(bool)
	gitArgs := []string{"checkout"}
	if create {
		gitArgs = append(gitArgs, "-b")
	}
	gitArgs = append(gitArgs, target)
	return runGit(ctx, gitArgs...), nil
}

// --- stash ---

type stashTool struct{ tools.BaseTool }

func (*stashTool) Name() string { return "git_stash" }
func (*stashTool) Description() string {
	return "Stash changes (push), restore them (pop), or list stashes."
}
func (*stashTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action":  map[string]any{"type": "string", "enum": []string{"list", "push", "pop"}},
			"message": map[string]any{"type": "string", "description": "Optional message for push"},
		},
		"required": []string{"action"},
	}
}
func (*stashTool) RequiresConfirmation(mode string) bool { return mode == "edit" || mode == "plan" }
func (*stashTool) IsConcurrencySafe(map[string]any) bool { return false }
func (*stashTool) IsReadOnly(map[string]any) bool        { return false }
func (*stashTool) IsDestructive(map[string]any) bool     { return false }
func (*stashTool) EstimatedCost() tools.ToolCost {
	return tools.ToolCost{TokensApprox: 90, LatencyMs: 90, RiskLevel: "low"}
}
func (*stashTool) Meta() *tools.ToolMeta {
	return meta("Git stash", "Shelve in-progress work when switching context mid-task; pop restores it.", "pop can conflict — read its output carefully.")
}

func (t *stashTool) Execute(ctx context.Context, args map[string]any) (tools.Result, error) {
	action, _ := tools.StringArg(args, "action")
	switch action {
	case "list":
		return runGit(ctx, "stash", "list"), nil
	case "push":
		gitArgs := []string{"stash", "push"}
		if m, ok := tools.StringArg(args, "message"); ok && m != "" {
			gitArgs = append(gitArgs, "-m", m)
		}
		return runGit(ctx, gitArgs...), nil
	case "pop":
		return runGit(ctx, "stash", "pop"), nil
	default:
		return tools.Result{IsError: true, Content: "git_stash: action must be list|push|pop"}, nil
	}
}
