package app

import (
	"fmt"
	"strings"
)

// Activity verbs — the spinner and status line describe WHAT the agent is
// doing right now, not just that it is busy. Tool calls map to concrete
// verbs; phase transitions map to arc verbs. The verbs are lowercase and
// read naturally after the spinner glyph ("⣳ reading…").
//
// The activity word is ALSO what the post-run summary uses ("worked for
// 12s"), so the vocabulary deliberately stays human.

// setSpinVerb records the current activity verb and applies it to the
// spinner. An empty verb resets to the streaming default ("generating").
// The stream tick decorates the label with throughput and elapsed time, so
// it reads the recorded verb rather than parsing it back out of the label.
func (a *App) setSpinVerb(verb string) {
	if verb == "" {
		verb = "generating"
	}
	a.spinLabel = verb
	a.spin.SetLabel(verb)
}

// verbForTool maps a tool name to its activity verb.
func verbForTool(tool string) string {
	switch tool {
	case "read_file", "list_directory":
		return "reading"
	case "grep", "glob":
		return "searching"
	case "edit_file", "write_file", "multi_edit":
		return "editing"
	case "bash", "write_shell", "stop_shell", "read_shell":
		return "running"
	case "task", "batch_task":
		return "delegating"
	case "read_agent", "list_agents":
		return "collecting"
	case "web_search":
		return "searching the web"
	case "web_fetch":
		return "fetching"
	case "todo_write", "todo_list":
		return "planning"
	case "git_diff", "git_status", "git_log":
		return "inspecting git"
	case "git_add", "git_commit", "git_branch", "git_checkout", "git_stash":
		return "committing"
	case "ask_user":
		return "waiting for you"
	case "wait":
		return "waiting"
	case "lsp_diagnostics":
		return "checking diagnostics"
	default:
		return "working"
	}
}

// verbForPhase maps an arc phase to its activity verb.
func verbForPhase(phase string) string {
	switch strings.ToLower(strings.TrimSpace(phase)) {
	case "init":
		return "initiating"
	case "explore":
		return "exploring"
	case "plan":
		return "planning"
	case "build":
		return "building"
	default:
		return "working"
	}
}

// verbForStatus derives an activity verb from a free-form agent status
// string, so phase and pipeline events ("phase explore: …", "init: …")
// drive the spinner without a new event type.
func verbForStatus(s string) (string, bool) {
	lower := strings.ToLower(s)
	for _, phase := range []string{"init", "explore", "plan", "build"} {
		if strings.HasPrefix(lower, "phase "+phase) {
			return verbForPhase(phase), true
		}
	}
	switch {
	case strings.HasPrefix(lower, "init"):
		return "initiating", true
	case strings.Contains(lower, "compacting"), strings.Contains(lower, "compaction"):
		return "compacting context", true
	case strings.Contains(lower, "retry"):
		return "retrying", true
	}
	return "", false
}

// formatDuration renders a run duration the way the status line needs it:
// seconds under a minute, minutes:seconds beyond, whole minutes past ten.
func formatDuration(d interface{ Seconds() float64 }) string {
	secs := int(d.Seconds())
	switch {
	case secs < 60:
		return fmt.Sprintf("%ds", secs)
	case secs < 600:
		return fmt.Sprintf("%dm%02ds", secs/60, secs%60)
	default:
		return fmt.Sprintf("%dm", secs/60)
	}
}

// estimateTokens converts streamed characters to an approximate token count.
// ~4 characters per token is the same heuristic Claude Code's spinner uses
// for its live "↓ N tokens" counter.
func estimateTokens(chars int) int {
	if chars < 4 {
		return 0
	}
	return chars / 4
}

// compactTokenCount renders a token count for the spinner readout: "842",
// "1.2k", "3.4M".
func compactTokenCount(n int) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 10_000:
		return fmt.Sprintf("%.0fk", float64(n)/1_000)
	case n >= 1_000:
		return fmt.Sprintf("%.1fk", float64(n)/1_000)
	default:
		return fmt.Sprintf("%d", n)
	}
}
