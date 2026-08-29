package components

// Single source of truth for how every registered tool presents itself in the
// conversation log: display name, accent color, which family renderer draws
// its body, which argument supplies the headline subject, and whether
// consecutive calls collapse into one card.
//
// A tool missing from toolSpecs still renders sensibly: specFor falls back to
// the same inference the system prompt uses (tools.InferCategory /
// tools.InferDisplayName), so registering a new tool never requires a TUI
// change — it only unlocks a nicer card.

import (
	"image/color"

	"github.com/iSundram/Automergent/internal/tools"
	"github.com/iSundram/Automergent/internal/tui/themes"
)

// toolFamily selects the body renderer for a tool.
type toolFamily int

const (
	familyGeneric      toolFamily = iota // unknown tools: call line + summary
	familyRead                           // read_file, view
	familyList                           // list_directory, glob
	familySearch                         // grep, search
	familyEdit                           // write_file, edit_file, multi_edit
	familyFileOp                         // reserved: legacy file op tools (removed)
	familyTerminal                       // bash
	familyShellSession                   // read_shell, write_shell, stop_shell, wait
	familyShellList                      // list_shells
	familyWeb                            // web_fetch, web_search
	familyDiagnostics                    // lsp_diagnostics
	familySecurity                       // secrets_scan, dependency_audit
	familyData                           // sql
	familyAgent                          // task, read_agent, list_agents, agent_control
	familyPlan                           // plan, replan
	familyTodo                           // todo_write, todo_list, todo_next
	familyContext                        // context_bucket_*, context_get_*
	familyInteraction                    // ask_user, notify
)

// accentFn resolves a theme slot lazily so specs stay theme-independent.
type accentFn func(*themes.Theme) color.Color

func aBlue(t *themes.Theme) color.Color    { return t.Blue }
func aGreen(t *themes.Theme) color.Color   { return t.Green }
func aYellow(t *themes.Theme) color.Color  { return t.Yellow }
func aRed(t *themes.Theme) color.Color     { return t.Red }
func aMagenta(t *themes.Theme) color.Color { return t.Magenta }
func aCyan(t *themes.Theme) color.Color    { return t.Cyan }
func aAccent(t *themes.Theme) color.Color  { return t.Accent }
func aMuted(t *themes.Theme) color.Color   { return t.Muted }

// toolSpec is the display contract for one tool.
type toolSpec struct {
	// Display is the name shown on the call line ("Grep", "Secrets scan").
	Display string

	// Accent colors the tool name so the log has per-family color rhythm.
	Accent accentFn

	// Family picks the body renderer.
	Family toolFamily

	// Subject lists argument keys tried in order for the headline subject.
	Subject []string

	// Groups marks tools whose consecutive calls collapse into one card.
	Groups bool
}

// toolSpecs covers every tool registered in cmd/automergent/main.go,
// internal/tools/taskstate.go and internal/agent/agent.go.
var toolSpecs = map[string]toolSpec{
	// --- read ---------------------------------------------------------
	"read_file": {"Read", aBlue, familyRead, []string{"path"}, true},

	// --- list ---------------------------------------------------------
	"list_directory": {"List", aBlue, familyList, []string{"path"}, true},
	"glob":           {"Glob", aBlue, familyList, []string{"pattern"}, true},

	// --- search -------------------------------------------------------
	"grep":   {"Grep", aMagenta, familySearch, []string{"pattern"}, true},

	// --- edit ---------------------------------------------------------
	"write_file": {"Write", aGreen, familyEdit, []string{"path"}, false},
	"edit_file":  {"Edit", aYellow, familyEdit, []string{"path"}, false},
	"multi_edit": {"Multi-edit", aYellow, familyEdit, []string{"path"}, false},

	// --- terminal -----------------------------------------------------
	"bash": {"Bash", aYellow, familyTerminal, []string{"command"}, false},

	// --- shell sessions -----------------------------------------------
	"read_shell":  {"Read shell", aYellow, familyShellSession, []string{"shell_id"}, false},
	"write_shell": {"Write shell", aYellow, familyShellSession, []string{"shell_id"}, false},
	"stop_shell":  {"Stop shell", aRed, familyShellSession, []string{"shell_id"}, false},
	"wait":        {"Wait", aMuted, familyShellSession, []string{"seconds"}, false},
	"list_shells": {"Shells", aYellow, familyShellList, nil, false},

	// --- web ----------------------------------------------------------
	"web_fetch":  {"Fetch", aMagenta, familyWeb, []string{"url"}, true},
	"web_search": {"Web search", aMagenta, familyWeb, []string{"query"}, true},

	// --- diagnostics --------------------------------------------------
	"lsp_diagnostics": {"Diagnostics", aCyan, familyDiagnostics, []string{"file"}, true},

	// --- security -----------------------------------------------------
	"secrets_scan":     {"Secrets scan", aRed, familySecurity, []string{"path"}, false},
	"dependency_audit": {"Audit", aYellow, familySecurity, []string{"path"}, false},

	// --- data ---------------------------------------------------------
	"sql": {"SQL", aBlue, familyData, []string{"query"}, false},

	// --- agents -------------------------------------------------------
	"task":          {"Task", aAccent, familyAgent, []string{"description", "name", "prompt"}, false},
	"read_agent":    {"Read agent", aAccent, familyAgent, []string{"agent_id"}, false},
	"list_agents":   {"Agents", aAccent, familyAgent, nil, false},
	"agent_control": {"Agent", aAccent, familyAgent, []string{"action"}, false},

	// --- planning -----------------------------------------------------
	"plan":   {"Plan", aAccent, familyPlan, []string{"request"}, false},
	"replan": {"Replan", aAccent, familyPlan, []string{"feedback"}, false},

	// --- todos --------------------------------------------------------
	"todo_write": {"Todos", aAccent, familyTodo, []string{"action"}, false},
	"todo_list":  {"Todos", aAccent, familyTodo, []string{"status_filter"}, false},

	// --- context bookkeeping ------------------------------------------
	"context_bucket_get":    {"Context", aMuted, familyContext, []string{"key", "bucket"}, true},
	"context_bucket_set":    {"Context", aMuted, familyContext, []string{"key"}, true},
	"context_bucket_delete": {"Context", aMuted, familyContext, []string{"key"}, true},
	"context_get":           {"Context", aMuted, familyContext, []string{"what"}, true},

	// --- interaction --------------------------------------------------
	"ask_user": {"Ask", aAccent, familyInteraction, []string{"question"}, false},
	"notify":   {"Notify", aCyan, familyInteraction, []string{"message", "title"}, false},
}

// categoryFamily maps an inferred tool category onto the closest family, so an
// unregistered tool lands in a real renderer instead of the generic dump.
var categoryFamily = map[string]toolFamily{
	"read":        familyRead,
	"search":      familySearch,
	"edit":        familyEdit,
	"shell":       familyTerminal,
	"web":         familyWeb,
	"lsp":         familyDiagnostics,
	"db":          familyData,
	"security":    familySecurity,
	"agents":      familyAgent,
	"memory":      familyTodo,
	"planning":    familyPlan,
	"interaction": familyInteraction,
}

// categoryAccent gives inferred tools the same color rhythm as declared ones.
var categoryAccent = map[string]accentFn{
	"read":        aBlue,
	"search":      aMagenta,
	"edit":        aYellow,
	"shell":       aYellow,
	"web":         aMagenta,
	"lsp":         aCyan,
	"db":          aBlue,
	"security":    aRed,
	"agents":      aAccent,
	"memory":      aAccent,
	"planning":    aAccent,
	"interaction": aCyan,
}

// specFor resolves the display contract for a tool name. Unknown names are
// inferred from the same helpers the prompt assembler uses, so the log
// degrades gracefully rather than falling off a cliff.
func specFor(name string) toolSpec {
	if spec, ok := toolSpecs[name]; ok {
		return spec
	}
	category := tools.InferCategory(name)
	spec := toolSpec{
		Display: tools.InferDisplayName(name),
		Accent:  aAccent,
		Family:  familyGeneric,
		Subject: []string{"path", "command", "pattern", "query", "url"},
	}
	if fam, ok := categoryFamily[category]; ok {
		spec.Family = fam
	}
	if accent, ok := categoryAccent[category]; ok {
		spec.Accent = accent
	}
	return spec
}

// groupKeyFor keys the run detector that collapses consecutive calls. Tools
// group per FAMILY, so read_file and list_directory merge into one
// "Read 3 files" card while an edit between them breaks the run.
func groupKeyFor(name string) string {
	spec := specFor(name)
	if !spec.Groups {
		// Ungrouped tools get a unique-per-name key so they never merge.
		return "solo:" + name
	}
	return "fam:" + familyName(spec.Family)
}

// groupsFor reports whether consecutive calls of a tool collapse.
func groupsFor(name string) bool { return specFor(name).Groups }

// ToolDisplayName is the exported display name for a tool, so permission
// prompts and status lines name a tool exactly as its card does.
func ToolDisplayName(name string) string { return specFor(name).Display }

// familyName gives each family a stable string for grouping keys and tests.
func familyName(f toolFamily) string {
	switch f {
	case familyRead:
		return "read"
	case familyList:
		return "list"
	case familySearch:
		return "search"
	case familyEdit:
		return "edit"
	case familyFileOp:
		return "fileop"
	case familyTerminal:
		return "terminal"
	case familyShellSession:
		return "shellsession"
	case familyShellList:
		return "shelllist"
	case familyWeb:
		return "web"
	case familyDiagnostics:
		return "diagnostics"
	case familySecurity:
		return "security"
	case familyData:
		return "data"
	case familyAgent:
		return "agent"
	case familyPlan:
		return "plan"
	case familyTodo:
		return "todo"
	case familyContext:
		return "context"
	case familyInteraction:
		return "interaction"
	default:
		return "generic"
	}
}
