package tools

import (
	"sort"
	"strings"
	"time"
)

// ToolMeta carries the per-tool system-prompt material and lifecycle knobs.
// It is the single place a tool explains itself: how to use it, when to use
// it, what to avoid, and how it should be validated and timed. Sections are
// assembled into the system prompt from the LIVE registry, so a tool that is
// not registered contributes nothing and a newly registered tool is
// documented automatically.
type ToolMeta struct {
	// Category groups tools in the prompt: read, edit, shell, git, web,
	// agents, verify, memory, interaction, lsp, db, security, planning.
	Category string

	// DisplayName is the human-facing name used in prompt headings.
	DisplayName string

	// Usage holds multi-line operational guidance appended verbatim under
	// the tool's heading (the long Claude-Code-style block).
	Usage string

	// WhenToUse gives routing guidance versus sibling tools.
	WhenToUse string

	// WhenNotTo states preferred alternatives and anti-patterns.
	WhenNotTo string

	// Examples holds good/bad call pairs rendered as short illustrations.
	Examples [][2]string

	// Aliases maps a model family (see ModelFamily) to an alternative name
	// for that family, e.g. "gemini3" -> "read_file (Gemini tool calling)".
	// Only Gemini families are populated; other providers are not targeted.
	Aliases map[string]string

	// UsageByFamily appends extra operational guidance only when the active
	// model matches the family key ("gemini3" today). Lets Gemini-specific
	// tuning ship without polluting other turns.
	UsageByFamily map[string]string

	// InjectOrder orders tools within their category section (lower first).
	InjectOrder int

	// Timeout bounds a single execution; zero means package default.
	Timeout time.Duration

	// StrictArgs rejects unknown/coerced arguments instead of best-effort
	// coercion. Malformed calls surface as likely-hallucinated calls.
	StrictArgs bool

	// PartialParse marks tools whose args are usable while the model is
	// still streaming them (leading keys are sufficient to render a card).
	PartialParse bool
}

// MetaProvider is an OPTIONAL interface: tools adopt per-tool prompting by
// adding a Meta() method. Nothing breaks for tools that do not — MetaOf
// falls back to inferred defaults built from the core Tool interface.
type MetaProvider interface {
	Meta() *ToolMeta
}

// Meta on BaseTool gives every embedder the default implementation up front,
// so adopting custom metadata is a plain method override, never an
// interface migration. Returning nil means "infer my meta".
func (b *BaseTool) Meta() *ToolMeta { return nil }

// MetaOf resolves the effective ToolMeta for a tool: explicit metadata when
// provided, inferred defaults otherwise. The returned value is always
// non-nil and always carries Name/Category/DisplayName.
func MetaOf(t Tool) *ToolMeta {
	if m, ok := t.(MetaProvider); ok && m != nil {
		if meta := m.Meta(); meta != nil {
			fillMetaDefaults(meta, t)
			return meta
		}
	}
	meta := &ToolMeta{}
	fillMetaDefaults(meta, t)
	return meta
}

func fillMetaDefaults(meta *ToolMeta, t Tool) {
	if meta.Category == "" {
		meta.Category = InferCategory(t.Name())
	}
	if meta.DisplayName == "" {
		meta.DisplayName = InferDisplayName(t.Name())
	}
	if meta.Usage == "" {
		if desc := strings.TrimSpace(t.Description()); desc != "" {
			meta.Usage = desc
		}
	}
}

// ModelFamily classifies a configured model id into a coarse family used for
// per-family prompt variants. Targeting is Gemini-first: the current Gemini 3
// generation gets its own family so tool documentation can be tuned for it;
// older Gemini models form a second family; everything else collapses to
// "default" (other providers are served by their own working stacks and are
// deliberately not special-cased here). Never returns an empty string.
func ModelFamily(model string) string {
	m := strings.ToLower(model)
	if IsGemini3(m) {
		return "gemini3"
	}
	if strings.Contains(m, "gemini") {
		return "gemini"
	}
	return "default"
}

// IsGemini3 reports whether a model id belongs to the new Gemini 3 family
// (gemini-3*, gemini_3*, "Gemini 3", flash/pro/lite/ultra 3.x variants).
func IsGemini3(model string) bool {
	m := strings.ToLower(strings.TrimSpace(model))
	if m == "" {
		return false
	}
	m = strings.ReplaceAll(m, " ", "-")
	m = strings.ReplaceAll(m, "_", "-")
	for _, sep := range []string{"gemini-3", "gemini3"} {
		if strings.Contains(m, sep) {
			return true
		}
	}
	// Guard against "gemini-30" style ids: require the next rune to be a
	// separator, a digit-less boundary, or part of a 3.x version.
	return false
}

// AliasFor returns the family-specific historical name for a tool, if any.
func (m *ToolMeta) AliasFor(family string) string {
	if m == nil || len(m.Aliases) == 0 {
		return ""
	}
	if alias, ok := m.Aliases[family]; ok {
		return alias
	}
	return m.Aliases["default"]
}

// CategoryOrder fixes the section ordering of categories in assembled prompts:
// discovery (read/search) before mutation (edit) before execution/verification
// (shell/git), then peripheral categories.
var CategoryOrder = []string{
	"read", "search", "edit", "shell", "git", "web",
	"lsp", "db", "security", "agents", "verify", "memory",
	"planning", "interaction", "general",
}

// CategoryRank returns the sort rank of a category; unknown categories sink.
func CategoryRank(category string) int {
	for i, c := range CategoryOrder {
		if c == category {
			return i
		}
	}
	return len(CategoryOrder)
}

// InferCategory guesses a tool's category from its registered name. Tools
// with real metadata never hit the heuristics; untagged ones still group
// sensibly instead of collapsing into one undifferentiated list.
func InferCategory(name string) string {
	n := strings.ToLower(name)
	switch {
	case n == "view" || strings.HasPrefix(n, "read") || strings.HasPrefix(n, "list_dir"):
		return "read"
	case strings.HasPrefix(n, "glob"), strings.HasPrefix(n, "grep"), n == "search":
		return "search"
	case strings.HasPrefix(n, "write"), strings.HasPrefix(n, "edit"), strings.HasPrefix(n, "create"),
		strings.HasPrefix(n, "delete"), strings.HasPrefix(n, "move"), strings.HasPrefix(n, "copy"),
		strings.HasPrefix(n, "apply_patch"), strings.HasPrefix(n, "multi_edit"), strings.HasPrefix(n, "sed"):
		return "edit"
	case strings.HasPrefix(n, "git_"):
		return "git"
	case n == "bash" || n == "run_command" || strings.HasSuffix(n, "_shell") ||
		n == "wait" || n == "command_status":
		return "shell"
	case strings.HasPrefix(n, "web_"):
		return "web"
	case strings.HasPrefix(n, "lsp_"):
		return "lsp"
	case n == "sql":
		return "db"
	case strings.HasPrefix(n, "secrets_scan"), strings.HasPrefix(n, "dependency_audit"):
		return "security"
	case n == "task" || n == "read_agent" || n == "list_agents":
		return "agents"
	case strings.HasPrefix(n, "todo_") || strings.HasPrefix(n, "context_") ||
		n == "task_list" || n == "task_get" || n == "task_update" || n == "memory_write":
		return "memory"
	case strings.HasPrefix(n, "plan"), n == "replan":
		return "planning"
	case n == "ask_user" || n == "notify" || n == "finish":
		return "interaction"
	default:
		return "general"
	}
}

// InferDisplayName converts snake_case tool names to Title Words for prompt
// headings, e.g. read_file -> "Read file".
func InferDisplayName(name string) string {
	parts := strings.Split(strings.ToLower(name), "_")
	for i, p := range parts {
		if p != "" {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, " ")
}

// SortToolsForPrompt orders tools deterministically for prompt assembly:
// category order, then explicit InjectOrder, then name. Determinism matters —
// unstable ordering would churn provider-side prompt caches every turn.
func SortToolsForPrompt(tools []Tool) []Tool {
	out := make([]Tool, len(tools))
	copy(out, tools)
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		ma, mb := MetaOf(a), MetaOf(b)
		if ra, rb := CategoryRank(ma.Category), CategoryRank(mb.Category); ra != rb {
			return ra < rb
		}
		if ma.InjectOrder != mb.InjectOrder {
			return ma.InjectOrder < mb.InjectOrder
		}
		return a.Name() < b.Name()
	})
	return out
}
