package git

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/iSundram/Automergent/internal/config"
	"github.com/iSundram/Automergent/internal/tools"
)

// CommitTool creates a git commit.
type CommitTool struct {
	cfg *config.Config
}

// NewCommitTool creates a new CommitTool with config.
func NewCommitTool(cfg *config.Config) *CommitTool {
	return &CommitTool{cfg: cfg}
}

func (t *CommitTool) Name() string { return "git_commit" }
func (t *CommitTool) Description() string {
	return `Create a git commit with the specified message.
- Adds co-author trailer based on config (git.coAuthor: always/never/ask)
- Use --all to stage all changes before committing
- Use --amend to amend the previous commit
- Use co_author parameter to override config for this commit`
}
func (t *CommitTool) RequiresConfirmation(mode string) bool {
	return mode == "edit" || mode == "plan"
}

func (t *CommitTool) EstimatedCost() tools.ToolCost {
	return tools.ToolCost{
		TokensApprox: 100,
		LatencyMs:    100,
		RiskLevel:    "medium",
	}
}

func (t *CommitTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"message": map[string]any{
				"type":        "string",
				"description": "Commit message.",
			},
			"all": map[string]any{
				"type":        "boolean",
				"description": "Stage all changes before committing (like git commit -a).",
			},
			"amend": map[string]any{
				"type":        "boolean",
				"description": "Amend the previous commit.",
			},
			"no_verify": map[string]any{
				"type":        "boolean",
				"description": "Skip pre-commit and commit-msg hooks.",
			},
			"co_author": map[string]any{
				"type":        "boolean",
				"description": "Include Automergent co-author trailer. Overrides config setting.",
			},
		},
		"required": []string{"message"},
	}
}

func (t *CommitTool) Execute(ctx context.Context, args map[string]any) (tools.Result, error) {
	message, ok := tools.StringArg(args, "message")
	if !ok || message == "" {
		return tools.Result{IsError: true, Content: "message is required"}, nil
	}

	// Determine whether to add co-author
	addCoAuthor := t.shouldAddCoAuthor(args)
	if addCoAuthor {
		trailer := "Co-authored-by: Automergent <automergent-bot@users.noreply.github.com>"
		if t.cfg != nil {
			trailer = t.cfg.Git.CoAuthorTrailerValue()
		}
		message = message + "\n\n" + trailer
	}

	cmdArgs := []string{"commit", "-m", message}

	if all, ok := tools.ArgBool(args, "all"); ok && all {
		cmdArgs = append(cmdArgs, "-a")
	}
	if amend, ok := tools.ArgBool(args, "amend"); ok && amend {
		cmdArgs = append(cmdArgs, "--amend")
	}
	if noVerify, ok := tools.ArgBool(args, "no_verify"); ok && noVerify {
		cmdArgs = append(cmdArgs, "--no-verify")
	}

	res, err := runGit(ctx, cmdArgs...)
	if err != nil {
		return res, err
	}

	// Extract commit hash for summary
	// Format: [main a9a0329] message
	summary := "committed"
	parts := strings.Split(strings.TrimSpace(res.Content), " ")
	if len(parts) >= 2 {
		hash := strings.TrimSuffix(strings.TrimPrefix(parts[1], "["), "]")
		if len(hash) > defaultCommitHashShortLen {
			hash = hash[:defaultCommitHashShortLen]
		}
		summary = fmt.Sprintf("committed %s", hash)
	}
	if addCoAuthor {
		summary += " (co-authored)"
	}
	res.Summary = summary
	return res, nil
}

// shouldAddCoAuthor determines if co-author should be added based on config and args.
func (t *CommitTool) shouldAddCoAuthor(args map[string]any) bool {
	// Check if explicitly set in args (overrides config)
	if coAuthor, ok := tools.ArgBool(args, "co_author"); ok {
		return coAuthor
	}

	// Fall back to config
	if t.cfg == nil {
		return true // default to always if no config
	}

	mode := t.cfg.Git.CoAuthorMode()
	switch mode {
	case "always":
		return true
	case "never":
		return false
	case "ask":
		// When "ask", check if co_author was explicitly passed in args
		// If not, we default to true (the confirmation UI handles the asking)
		// The actual "ask" UI is handled by the TUI confirmation component
		return true
	default:
		return true
	}
}

// AddTool stages files for commit.
type AddTool struct{}

func (t *AddTool) Name() string { return "git_add" }
func (t *AddTool) Description() string {
	return `Stage files for commit.
- Use path to stage specific files/patterns
- Use all=true to stage all changes
- Use update=true to only stage modified/deleted files (not new files)`
}
func (t *AddTool) RequiresConfirmation(mode string) bool {
	return mode == "edit" || mode == "plan"
}

func (t *AddTool) EstimatedCost() tools.ToolCost {
	return tools.ToolCost{
		TokensApprox: 80,
		LatencyMs:    80,
		RiskLevel:    "low",
	}
}

func (t *AddTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "File or pattern to stage.",
			},
			"all": map[string]any{
				"type":        "boolean",
				"description": "Stage all changes (git add -A).",
			},
			"update": map[string]any{
				"type":        "boolean",
				"description": "Only stage modified/deleted files (git add -u).",
			},
		},
	}
}

func (t *AddTool) Execute(ctx context.Context, args map[string]any) (tools.Result, error) {
	path, _ := tools.StringArg(args, "path")
	all, _ := tools.ArgBool(args, "all")
	update, _ := tools.ArgBool(args, "update")

	cmdArgs := []string{"add"}

	if all {
		cmdArgs = append(cmdArgs, "-A")
	} else if update {
		cmdArgs = append(cmdArgs, "-u")
	} else if path != "" {
		cmdArgs = append(cmdArgs, path)
	} else {
		return tools.Result{IsError: true, Content: "specify path, all=true, or update=true"}, nil
	}

	result, err := runGit(ctx, cmdArgs...)
	if err != nil {
		return result, err
	}

	// Show what was staged
	statusResult, err := runGit(ctx, "status", "--short")
	if err != nil {
		// If we couldn't get status, return the add result and the error
		return result, err
	}

	stagedCount := 0
	for _, line := range strings.Split(statusResult.Content, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if len(line) >= minStatusLineLen && line[0] != ' ' && line[0] != '?' {
			stagedCount++
		}
	}

	return tools.Result{
		Content: fmt.Sprintf("staged files\n\n%s", statusResult.Content),
		Summary: fmt.Sprintf("staged +%d files", stagedCount),
	}, nil
}

// CheckoutTool switches branches or restores files.
type CheckoutTool struct{}

func (t *CheckoutTool) Name() string { return "git_checkout" }
func (t *CheckoutTool) Description() string {
	return `Switch branches or restore working tree files.
- Use branch to switch to a branch
- Use file to restore a specific file
- Use create=true with branch to create a new branch`
}
func (t *CheckoutTool) RequiresConfirmation(mode string) bool {
	return mode == "edit" || mode == "plan"
}

func (t *CheckoutTool) EstimatedCost() tools.ToolCost {
	return tools.ToolCost{
		TokensApprox: 100,
		LatencyMs:    150,
		RiskLevel:    "medium",
	}
}

func (t *CheckoutTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"branch": map[string]any{
				"type":        "string",
				"description": "Branch name to switch to.",
			},
			"file": map[string]any{
				"type":        "string",
				"description": "File to restore from HEAD.",
			},
			"create": map[string]any{
				"type":        "boolean",
				"description": "Create a new branch (git checkout -b).",
			},
			"ref": map[string]any{
				"type":        "string",
				"description": "Reference (commit, tag) to checkout file from.",
			},
		},
	}
}

func (t *CheckoutTool) Execute(ctx context.Context, args map[string]any) (tools.Result, error) {
	branch, _ := tools.StringArg(args, "branch")
	file, _ := tools.StringArg(args, "file")
	create, _ := tools.ArgBool(args, "create")
	ref, _ := tools.StringArg(args, "ref")

	if branch != "" {
		cmdArgs := []string{"checkout"}
		if create {
			cmdArgs = append(cmdArgs, "-b")
		}
		cmdArgs = append(cmdArgs, branch)
		return runGit(ctx, cmdArgs...)
	}

	if file != "" {
		cmdArgs := []string{"checkout"}
		if ref != "" {
			cmdArgs = append(cmdArgs, ref, "--")
		} else {
			cmdArgs = append(cmdArgs, "HEAD", "--")
		}
		cmdArgs = append(cmdArgs, file)
		return runGit(ctx, cmdArgs...)
	}

	return tools.Result{IsError: true, Content: "specify either branch or file"}, nil
}

// BranchTool manages git branches.
type BranchTool struct{}

func (t *BranchTool) Name() string { return "git_branch" }
func (t *BranchTool) Description() string {
	return `List, create, or delete branches.
- list (default): show all branches
- create: create a new branch
- delete: delete a branch`
}
func (t *BranchTool) RequiresConfirmation(mode string) bool {
	return mode == "edit" || mode == "plan"
}

func (t *BranchTool) EstimatedCost() tools.ToolCost {
	return tools.ToolCost{
		TokensApprox: 80,
		LatencyMs:    100,
		RiskLevel:    "medium",
	}
}

func (t *BranchTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"enum":        []string{"list", "create", "delete"},
				"description": "Action to perform.",
			},
			"name": map[string]any{
				"type":        "string",
				"description": "Branch name (for create/delete).",
			},
			"force": map[string]any{
				"type":        "boolean",
				"description": "Force delete unmerged branch (-D).",
			},
			"all": map[string]any{
				"type":        "boolean",
				"description": "Include remote branches in list.",
			},
		},
	}
}

func (t *BranchTool) Execute(ctx context.Context, args map[string]any) (tools.Result, error) {
	action := "list"
	if a, ok := tools.StringArg(args, "action"); ok {
		action = a
	}

	name, _ := tools.StringArg(args, "name")
	force, _ := tools.ArgBool(args, "force")
	all, _ := tools.ArgBool(args, "all")

	switch action {
	case "list":
		if all {
			return runGit(ctx, "branch", "-a")
		}
		return runGit(ctx, "branch")

	case "create":
		if name == "" {
			return tools.Result{IsError: true, Content: "name is required for create"}, nil
		}
		return runGit(ctx, "branch", name)

	case "delete":
		if name == "" {
			return tools.Result{IsError: true, Content: "name is required for delete"}, nil
		}
		flag := "-d"
		if force {
			flag = "-D"
		}
		return runGit(ctx, "branch", flag, name)

	default:
		return tools.Result{IsError: true, Content: fmt.Sprintf("unknown action: %s", action)}, nil
	}
}

// StashTool manages git stash.
type StashTool struct{}

func (t *StashTool) Name() string { return "git_stash" }
func (t *StashTool) Description() string {
	return `Stash changes or restore stashed changes.
- push: stash current changes
- pop: apply and remove last stash
- list: show all stashes
- apply: apply stash without removing
- drop: remove a stash`
}
func (t *StashTool) RequiresConfirmation(mode string) bool {
	return mode == "edit" || mode == "plan"
}

func (t *StashTool) EstimatedCost() tools.ToolCost {
	return tools.ToolCost{
		TokensApprox: 100,
		LatencyMs:    150,
		RiskLevel:    "medium",
	}
}

func (t *StashTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"enum":        []string{"push", "pop", "list", "apply", "drop"},
				"description": "Stash action to perform.",
			},
			"message": map[string]any{
				"type":        "string",
				"description": "Message for stash push.",
			},
			"index": map[string]any{
				"type":        "integer",
				"description": "Stash index for pop/apply/drop (default: 0).",
			},
			"include_untracked": map[string]any{
				"type":        "boolean",
				"description": "Include untracked files in stash.",
			},
		},
		"required": []string{"action"},
	}
}

func (t *StashTool) Execute(ctx context.Context, args map[string]any) (tools.Result, error) {
	action, _ := tools.StringArg(args, "action")
	message, _ := tools.StringArg(args, "message")
	index := 0
	if i, ok := tools.ArgInt(args, "index"); ok {
		index = i
	}
	includeUntracked, _ := tools.ArgBool(args, "include_untracked")

	stashRef := fmt.Sprintf("stash@{%d}", index)

	switch action {
	case "push":
		cmdArgs := []string{"stash", "push"}
		if includeUntracked {
			cmdArgs = append(cmdArgs, "-u")
		}
		if message != "" {
			cmdArgs = append(cmdArgs, "-m", message)
		}
		return runGit(ctx, cmdArgs...)

	case "pop":
		return runGit(ctx, "stash", "pop", stashRef)

	case "list":
		return runGit(ctx, "stash", "list")

	case "apply":
		return runGit(ctx, "stash", "apply", stashRef)

	case "drop":
		return runGit(ctx, "stash", "drop", stashRef)

	default:
		return tools.Result{IsError: true, Content: fmt.Sprintf("unknown action: %s", action)}, nil
	}
}

// BlameTool shows line-by-line authorship.
type BlameTool struct{}

func (t *BlameTool) Name() string                          { return "git_blame" }
func (t *BlameTool) Description() string                   { return "Show line-by-line authorship for a file." }
func (t *BlameTool) RequiresConfirmation(mode string) bool { return false }

func (t *BlameTool) EstimatedCost() tools.ToolCost {
	return tools.ToolCost{
		TokensApprox: 150,
		LatencyMs:    200,
		RiskLevel:    "low",
	}
}

func (t *BlameTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"file": map[string]any{
				"type":        "string",
				"description": "File to show blame for.",
			},
			"start_line": map[string]any{
				"type":        "integer",
				"description": "Start line number.",
			},
			"end_line": map[string]any{
				"type":        "integer",
				"description": "End line number.",
			},
		},
		"required": []string{"file"},
	}
}

func (t *BlameTool) Execute(ctx context.Context, args map[string]any) (tools.Result, error) {
	file, ok := tools.StringArg(args, "file")
	if !ok || file == "" {
		return tools.Result{IsError: true, Content: "file is required"}, nil
	}

	cmdArgs := []string{"blame"}

	startLine, hasStart := tools.ArgInt(args, "start_line")
	endLine, hasEnd := tools.ArgInt(args, "end_line")

	if hasStart || hasEnd {
		if !hasStart {
			startLine = 1
		}
		if !hasEnd {
			endLine = startLine + defaultBlameRange
		}
		cmdArgs = append(cmdArgs, fmt.Sprintf("-L%d,%d", startLine, endLine))
	}

	cmdArgs = append(cmdArgs, file)
	return runGit(ctx, cmdArgs...)
}

// ShowTool shows commit details.
type ShowTool struct{}

func (t *ShowTool) Name() string                          { return "git_show" }
func (t *ShowTool) Description() string                   { return "Show details of a commit, including diff." }
func (t *ShowTool) RequiresConfirmation(mode string) bool { return false }

func (t *ShowTool) EstimatedCost() tools.ToolCost {
	return tools.ToolCost{
		TokensApprox: 150,
		LatencyMs:    100,
		RiskLevel:    "low",
	}
}

func (t *ShowTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"ref": map[string]any{
				"type":        "string",
				"description": "Commit SHA, branch, or tag (default: HEAD).",
			},
			"file": map[string]any{
				"type":        "string",
				"description": "Show only changes to this file.",
			},
			"stat": map[string]any{
				"type":        "boolean",
				"description": "Show diffstat only (no full diff).",
			},
		},
	}
}

func (t *ShowTool) Execute(ctx context.Context, args map[string]any) (tools.Result, error) {
	ref := "HEAD"
	if r, ok := tools.StringArg(args, "ref"); ok && r != "" {
		ref = r
	}

	file, _ := tools.StringArg(args, "file")
	stat, _ := tools.ArgBool(args, "stat")

	cmdArgs := []string{"show"}
	if stat {
		cmdArgs = append(cmdArgs, "--stat")
	}
	cmdArgs = append(cmdArgs, ref)

	if file != "" {
		cmdArgs = append(cmdArgs, "--", file)
	}

	return runGit(ctx, cmdArgs...)
}
