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
			m := readOnlyPlusFinish("bash", "run_command", "write_shell")
			for _, name := range []string{"edit_file", "write_file", "create_file", "multi_edit"} {
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
	"edit":  {Name: "edit"},
	"agent": {Name: "agent"},
	"":      {Name: ""},
}

// currentMode resolves the active mode spec from cfg.Mode each turn, so
// runtime switches (TUI /mode) take effect without agent restarts.
func (a *Agent) currentMode() *ModeSpec {
	name := ""
	if a.cfg != nil {
		name = strings.ToLower(strings.TrimSpace(a.cfg.Mode))
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
	name := strings.ToLower(strings.TrimSpace(a.cfg.Mode))
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
func IsValid(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "edit", "plan":
		return true
	default:
		return false
	}
}

// AllModes returns the user-selectable modes in canonical order.
func AllModes() []string {
	return []string{"edit", "plan"}
}
