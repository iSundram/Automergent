package agent

import (
	"strings"

	"github.com/iSundram/Automergent/internal/ai"
)

// ModeSpec is a capability mask + injected guidance for one working mode.
// Modes never change the persona — they constrain WHICH tools are physically
// available and add mode-specific protocol guidance to the system prompt.
type ModeSpec struct {
	Name string
	// AllowedTools: nil means "everything registered". Non-nil intersects.
	AllowedTools map[string]bool
	PromptBlock  string
}

var readOnlyPlusFinish = func(extra ...string) map[string]bool {
	m := map[string]bool{
		"read_file": true, "view": true, "list_directory": true,
		"grep": true, "glob": true, "search": true,
		"lsp_diagnostics": true, "list_shells": true, "read_shell": true,
		"web_search": true, "web_fetch": true,
		"todo_list": true, "todo_next": true, "todo_write": true,
		"ask_user": true, "wait": true,
	}
	for _, name := range extra {
		m[name] = true
	}
	return m
}

// knownModes maps cfg.Mode values (set via /mode or --mode) onto specs.
var knownModes = map[string]*ModeSpec{
	"plan": {
		Name:         "plan",
		AllowedTools: readOnlyPlusFinish(),
		PromptBlock: "# Mode: Plan\n" +
			"You are planning, not implementing. Research the codebase, then produce a structured plan:\n" +
			"objective, findings (files/symbols), proposed changes per file, risks, and verification steps.\n" +
			"Do not edit files in this mode. End with `finish` and a plan the user can approve.\n",
	},
	"review": {
		Name:         "review",
		AllowedTools: readOnlyPlusFinish("task", "read_agent", "list_agents"),
		PromptBlock: "# Mode: Review\n" +
			"Perform an adversarial review of recent changes (git_diff) or the requested code.\n" +
			"Actively try to find bugs, security issues, and contract violations; verify claims by running read-only checks.\n" +
			"Report findings ranked by severity with file:line references. Do not edit files in this mode.\n",
	},
	"debug": {
		Name: "debug",
		AllowedTools: func() map[string]bool {
			m := readOnlyPlusFinish("bash", "write_shell")
			for _, name := range []string{"edit_file", "write_file", "multi_edit"} {
				m[name] = true
			}
			return m
		}(),
		PromptBlock: "# Mode: Debug\n" +
			"Diagnose systematically: reproduce first (run the failing command), isolate with narrow probes,\n" +
			"state the root-cause hypothesis BEFORE fixing, then apply the minimal fix and re-run the reproduction to confirm.\n",
	},
	"triage": {
		Name: "triage",
		AllowedTools: map[string]bool{
			"task": true, "read_agent": true, "list_agents": true,
			"todo_write": true, "todo_list": true, "todo_next": true,
			"ask_user": true,
		},
		PromptBlock: "# Mode: Triage (orchestrator)\n" +
			"You coordinate; subagents execute. Your ONLY tools spawn and track subagents — you cannot touch files directly.\n" +
			"Delegate every actionable request to a `task` subagent with a complete, self-contained prompt including relevant context slices.\n" +
			"When subagent results arrive, review them and either dispatch follow-ups or finish. Never redo a subagent's work yourself.\n",
	},
	// Approval modes carry no capability mask — they differ in WHEN the user
	// is asked, not in which tools exist. The gate lives in ApprovalFor.
	"manual": {
		Name: "manual",
		PromptBlock: "# Mode: Manual approval\n" +
			"Every action that touches the workspace or the network is approved by the user first.\n" +
			"Batch related work into single tool calls where you can — each call costs the user a decision.\n",
	},
	"accept-edits": {
		Name: "accept-edits",
		PromptBlock: "# Mode: Accept edits\n" +
			"File edits apply without asking; shell commands, network access and git still need approval.\n" +
			"Keep edits reviewable: small, focused changes the user can scan in the diff afterwards.\n",
	},
	"auto": {
		Name: "auto",
		PromptBlock: "# Mode: Auto\n" +
			"You may act without per-action approval. Destructive or unrecognized shell commands still stop for the user.\n" +
			"Verify as you go — the user is not gating each step, so an unchecked mistake compounds.\n",
	},
	"edit":  {Name: "edit"},
	"agent": {Name: "agent"},
	"":      {Name: ""},
}

// canonicalMode resolves a user-supplied or persisted mode name to its
// canonical form. "edit" is the pre-approval-modes name for what is now
// "manual"; configs and `--mode edit` written before the rename keep working.
func canonicalMode(name string) string {
	n := strings.ToLower(strings.TrimSpace(name))
	if n == "edit" {
		return "manual"
	}
	return n
}

// CanonicalMode exposes canonicalMode to callers outside this package (the TUI
// resolves the persisted config value before displaying it).
func CanonicalMode(name string) string { return canonicalMode(name) }

// currentMode resolves the active mode spec from cfg.Mode each turn, so
// runtime switches (TUI /mode) take effect without agent restarts.
func (a *Agent) currentMode() *ModeSpec {
	name := ""
	if a.cfg != nil {
		name = canonicalMode(a.cfg.Mode)
	}
	if spec, ok := knownModes[name]; ok && len(spec.AllowedTools) > 0 {
		return spec
	}
	return nil // default/full-capability modes carry no mask
}

// modePromptBlock returns the system-prompt block for the active mode.
func (a *Agent) modePromptBlock() string {
	if a.cfg == nil {
		return ""
	}
	name := canonicalMode(a.cfg.Mode)
	if spec, ok := knownModes[name]; ok {
		return spec.PromptBlock
	}
	return ""
}

// applyModeMask filters schemas down to the active mode's capability mask.
func applyModeMask(schemas []ai.ToolSchema, spec *ModeSpec) []ai.ToolSchema {
	if spec == nil || spec.AllowedTools == nil {
		return schemas
	}
	out := make([]ai.ToolSchema, 0, len(schemas))
	for _, s := range schemas {
		if spec.AllowedTools[s.Name] {
			out = append(out, s)
		}
	}
	return out
}

// IsValid reports whether a mode name is a user-selectable approval mode.
// These are the canonical persistable modes surfaced by /mode and --mode;
// richer internal capability masks (review/debug/triage) exist in knownModes
// but are not part of the public mode switch.
//
// "edit" remains valid as the legacy alias for "manual" so configs and command
// lines written before the approval modes landed keep working.
func IsValid(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "manual", "accept-edits", "auto", "plan", "edit":
		return true
	default:
		return false
	}
}

// AllModes returns the user-selectable modes in canonical order, ordered from
// most to least gated. The legacy "edit" alias is deliberately absent: it is
// accepted as input but never offered as a choice.
func AllModes() []string {
	return []string{"manual", "accept-edits", "auto", "plan"}
}

// NextMode returns the mode after the given one in AllModes order, wrapping
// around. This backs the shift+tab mode cycle in the TUI.
func NextMode(current string) string {
	all := AllModes()
	canonical := canonicalMode(current)
	for i, m := range all {
		if m == canonical {
			return all[(i+1)%len(all)]
		}
	}
	return all[0]
}

// ModeDescription returns a one-line explanation of a mode for the /mode
// palette and help output.
func ModeDescription(name string) string {
	switch canonicalMode(name) {
	case "manual":
		return "Confirm every action that writes or reaches the network"
	case "accept-edits":
		return "Apply file edits automatically; confirm shell, web and git"
	case "auto":
		return "Act without asking, except destructive commands"
	case "plan":
		return "Research and plan only — no edits"
	default:
		return ""
	}
}
