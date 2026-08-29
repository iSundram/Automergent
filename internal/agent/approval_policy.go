package agent

import (
	"strings"

	"github.com/iSundram/Automergent/internal/ai"
	"github.com/iSundram/Automergent/internal/tools"
)

// Approval modes decide WHEN the user is asked to confirm a tool call, as
// opposed to the capability masks in modes.go which decide WHICH tools exist.
//
// Why this is a tri-state rather than a bool: tools implement
// RequiresConfirmation(mode string) and several compare the mode literal
// (internal/tools/filesystem/write.go and internal/tools/shell/async.go both
// test `mode == "edit"`). Under a mode name those tools have never heard of,
// they answer "no confirmation needed" — which would silently auto-approve
// writes and shell commands. ApprovalDefault therefore preserves the legacy
// path exactly for legacy modes, and only the new approval modes take an
// explicit decision here.
type ApprovalDecision int

const (
	// ApprovalDefault defers to the tool's own RequiresConfirmation(mode).
	ApprovalDefault ApprovalDecision = iota
	// ApprovalAsk always prompts the user, whatever the tool would say.
	ApprovalAsk
	// ApprovalAuto skips the prompt entirely.
	ApprovalAuto
)

// String renders a decision for logs and test failures.
func (d ApprovalDecision) String() string {
	switch d {
	case ApprovalAsk:
		return "ask"
	case ApprovalAuto:
		return "auto"
	default:
		return "default"
	}
}

// ApprovalFor decides whether a tool call needs user confirmation under the
// given mode.
//
// Destructive and unrecognized-shell calls always stop for the user, in every
// mode: "auto" means "don't interrupt me for routine work", never "delete
// things without telling me".
func ApprovalFor(mode string, tc ai.ToolCall, t tools.Tool) ApprovalDecision {
	switch canonicalMode(mode) {
	case "manual":
		if isReadOnlyCall(tc, t) {
			return ApprovalAuto
		}
		return ApprovalAsk

	case "accept-edits":
		if isReadOnlyCall(tc, t) {
			return ApprovalAuto
		}
		if requiresUserAttention(tc, t) {
			return ApprovalAsk
		}
		if tools.MetaOf(t).Category == "edit" {
			return ApprovalAuto
		}
		// Shell, web, git, database and anything else uncategorised keeps its
		// prompt: those reach outside the workspace or rewrite history.
		return ApprovalAsk

	case "auto":
		if requiresUserAttention(tc, t) {
			return ApprovalAsk
		}
		return ApprovalAuto

	default:
		// plan, agent, "" and any unknown value keep the pre-existing
		// behaviour: the tool decides.
		return ApprovalDefault
	}
}

// isReadOnlyCall reports whether a call only observes state. It trusts the
// tool's own IsReadOnly, then falls back to the category table so a tool that
// reports conservatively still doesn't cost the user a decision for a read.
func isReadOnlyCall(tc ai.ToolCall, t tools.Tool) bool {
	if t.IsReadOnly(tc.Args) {
		return true
	}
	switch tools.MetaOf(t).Category {
	case "read", "search", "lsp", "memory", "planning", "interaction", "agents":
		return true
	}
	return false
}

// requiresUserAttention reports whether a call must be confirmed no matter how
// permissive the mode is: destructive operations, and shell commands whose
// shape we cannot generalize into a safe grant.
func requiresUserAttention(tc ai.ToolCall, t tools.Tool) bool {
	if t.IsDestructive(tc.Args) {
		return true
	}
	if isShellCall(tc.Name, t) && CommandIsDangerous(tc.Name, tc.Args) {
		return true
	}
	return false
}

// isShellCall reports whether a tool call executes a shell command, so the
// dangerous-command check is only applied where it has a command to inspect.
func isShellCall(name string, t tools.Tool) bool {
	if tools.MetaOf(t).Category == "shell" {
		return true
	}
	n := strings.ToLower(name)
	return n == "bash" || n == "run_shell_command"
}

// needsConfirmation applies the active mode's approval policy to a tool call,
// falling back to the tool's own judgement for legacy modes.
func (a *Agent) needsConfirmation(tc ai.ToolCall, t tools.Tool) bool {
	mode := ""
	if a.cfg != nil {
		mode = a.cfg.Mode
	}
	switch ApprovalFor(mode, tc, t) {
	case ApprovalAuto:
		return false
	case ApprovalAsk:
		return true
	default:
		return t.RequiresConfirmation(mode)
	}
}
