package prompt

import (
	"fmt"
	"sort"
	"strings"

	"github.com/iSundram/Automergent/internal/shared"
	"github.com/iSundram/Automergent/internal/tools"
)

// Per-tool prompt guidance. There are TWO sources, in priority order:
//
//  1. The live registry: each tool may implement Meta() (*tools.ToolMeta)
//     carrying Category/WhenToUse/WhenNotTo/Usage/Examples, rendered by
//     RenderToolSections. That is the primary, self-documenting system —
//     new tools document themselves.
//  2. This fallback table, for tools that have not adopted Meta() yet and
//     for entries the registry meta does not cover. Agent definitions may
//     override any entry via ToolPrompts (agent wins over both).
//
// RenderToolPrompts below renders the fallback layer for the tools offered
// in the current phase; RenderToolSections renders the registry layer. Both
// layers are bounded to offered tools so phases never see guidance for
// tools they cannot call.

var defaultToolPrompts = map[string]shared.ToolPromptConfig{
	"bash": {
		PreExecution: "Execute shell commands. Prefer non-interactive flags. Use absolute paths.",
		Rules: []string{
			"Never pipe curl/wget downloads straight into a shell interpreter",
			"Chain dependent commands with && so failure stops the chain",
			"Check exit codes; a zero-output success is still a success",
		},
	},
	"read_file": {
		PreExecution: "Read files to understand code. Use offset/limit for large files.",
		Rules: []string{
			"Do not read entire large files — use offset/limit",
			"Locate the right region with grep/glob before reading",
		},
	},
	"edit_file": {
		PreExecution: "Make minimal, focused edits. Match exact indentation.",
		Rules: []string{
			"Read the file (or the target region) before editing",
			"Use replaceAll only for renames",
			"Preserve existing code style; no new comments unless asked",
		},
	},
	"write_file": {
		PreExecution: "Create new files. Follow existing project patterns.",
		Rules: []string{
			"Check whether the file already exists first",
			"Match the project's structure and conventions",
			"Prefer editing an existing file over creating a new one",
		},
	},
	"multi_edit": {
		PreExecution: "Apply several edits to one file in a single call.",
		Rules: []string{
			"Order edits bottom-up so earlier edits do not shift later context",
			"Read the file first; every old_string must match exactly once",
		},
	},
	"glob": {
		PreExecution: "Find files by pattern. Use ** for recursive matches.",
		Rules: []string{
			"Prefer a narrow pattern over listing everything and filtering",
		},
	},
	"grep": {
		PreExecution: "Search file content with a regex. Use the include filter to narrow by file type.",
		Rules: []string{
			"Start narrow (symbol name, exact string) before broadening",
			"Files-with-matches mode first; pull content lines only when needed",
		},
	},
	"list_directory": {
		PreExecution: "List a directory's entries to orient yourself.",
		Rules: []string{
			"Use it to orient; use grep/glob once you know what you're looking for",
		},
	},
	"task": {
		PreExecution: "Delegate to a subagent. Provide a complete, self-contained prompt.",
		Rules: []string{
			"One task per subagent; include the relevant file paths and context",
			"Use background mode for parallel independent work, then collect results",
			"Prefer the specialized agent (explore/review) when the task matches",
		},
	},
	"batch_task": {
		PreExecution: "Launch several subagents in parallel with one call.",
		Rules: []string{
			"Only independent tasks — parallel agents cannot see each other's work",
			"Every prompt must stand alone; results are collected with read_agent",
		},
	},
	"read_agent": {
		PreExecution: "Collect results from background subagents.",
		Rules: []string{
			"Use wait=true when you need the result before continuing",
		},
	},
	"list_agents": {
		PreExecution: "List running and completed subagents.",
	},
	"todo_write": {
		PreExecution: "Create and update the task todo list.",
		Rules: []string{
			"Mark an item in_progress when you start it, completed the moment it's done",
			"Never batch completions",
		},
	},
	"todo_list": {
		PreExecution: "Read the current todo list state.",
	},
	"web_search": {
		PreExecution: "Search the web for current information.",
		Rules: []string{
			"Use for current events and version-specific facts, not codebase questions",
		},
	},
	"web_fetch": {
		PreExecution: "Fetch a specific URL's content.",
		Rules: []string{
			"Use for documentation pages, not as a substitute for user requests",
			"Follow cross-host redirects with a new fetch",
		},
	},
	"wait": {
		PreExecution: "Block until a background shell finishes.",
	},
	"ask_user": {
		PreExecution: "Ask the user a question when only they can decide.",
		Rules: []string{
			"Offer concrete options instead of open-ended questions",
			"Only for decisions you cannot resolve with tools",
		},
	},
	"git_status": {
		PreExecution: "Show the working tree status.",
	},
	"git_diff": {
		PreExecution: "Show staged and unstaged changes.",
	},
	"git_log": {
		PreExecution: "Show recent commit history.",
		Rules: []string{"Match the repo's commit message style when writing new ones"},
	},
	"git_add": {
		PreExecution: "Stage specific files.",
		Rules: []string{"Stage only the files that belong to the change"},
	},
	"git_commit": {
		PreExecution: "Create a commit.",
		Rules: []string{
			"Only commit when the user asks",
			"Never commit secrets or credential files",
		},
	},
	"git_branch": {
		PreExecution: "Create or list branches.",
	},
	"git_checkout": {
		PreExecution: "Switch branches or restore files.",
	},
	"git_stash": {
		PreExecution: "Stash or restore working tree changes.",
	},
}

// RenderToolPrompts renders the fallback tool-guidance layer for the given
// offered tool names, skipping any tool that already documents itself via
// Meta() in the registry (the registry layer owns those — guidance must not
// appear twice). Agent-specific ToolPrompts override the fallback entries.
// Output is deterministic (tools sorted by name) so the prompt prefix stays
// cache-stable.
func RenderToolPrompts(offered []string, agentOverrides map[string]shared.ToolPromptConfig) string {
	return RenderToolPromptsFromRegistry(nil, offered, agentOverrides)
}

// RenderToolPromptsFromRegistry is RenderToolPrompts with a registry: tools
// present in the registry AND carrying explicit Meta() documentation are
// skipped here because RenderToolSections already renders them.
func RenderToolPromptsFromRegistry(reg *tools.Registry, offered []string, agentOverrides map[string]shared.ToolPromptConfig) string {
	if len(offered) == 0 {
		return ""
	}

	selfDocumented := map[string]bool{}
	if reg != nil {
		for _, t := range reg.All() {
			if m, ok := t.(tools.MetaProvider); ok && m != nil {
				if meta := m.Meta(); meta != nil {
					selfDocumented[t.Name()] = true
				}
			}
		}
	}

	names := make([]string, 0, len(offered))
	for _, name := range offered {
		if selfDocumented[name] {
			continue
		}
		if _, ok := defaultToolPrompts[name]; ok {
			names = append(names, name)
		} else if _, ok := agentOverrides[name]; ok {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return ""
	}
	sort.Strings(names)

	var sections []string
	for _, name := range names {
		config := defaultToolPrompts[name]
		if override, ok := agentOverrides[name]; ok {
			config = override
		}
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("### %s\n", name))
		if config.PreExecution != "" {
			sb.WriteString(config.PreExecution + "\n")
		}
		if len(config.Rules) > 0 {
			sb.WriteString("Rules:\n")
			for _, rule := range config.Rules {
				sb.WriteString(fmt.Sprintf("- %s\n", rule))
			}
		}
		sections = append(sections, strings.TrimRight(sb.String(), "\n"))
	}
	return strings.Join(sections, "\n\n")
}
