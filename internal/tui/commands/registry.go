package commands

// Default returns the default command registry with all commands registered.
// This is the single source of truth: palette, help and dispatch are all
// derived from it.
func Default() *Registry {
	r := NewRegistry()

	// --- AI & Model ---
	r.MustRegister(Command{
		Name:             "model",
		Description:      "Switch, list, or manage models for the active provider",
		Category:         "AI & Model",
		Icon:             "󰊕",
		ArgsHint:         "[name|list|add|remove|refresh|reset]",
		Tier:             TierSecondary,
		Type:             CmdFullPage,
		FullPageTitle:    "Model Hub",
		SubPalette:       "model",
		SubCommands: []SubCommand{
			{Name: "list", Description: "List available models", Handler: handleModel},
			{Name: "add", Description: "Add a custom model", ArgsHint: "<name>", Handler: handleModel},
			{Name: "remove", Description: "Remove a custom model", ArgsHint: "<name>", Handler: handleModel},
			{Name: "refresh", Description: "Refresh model list from provider", Handler: handleModel},
			{Name: "reset", Description: "Reset models to defaults", Handler: handleModel},
		},
		SupportsHeadless: true,
	}, handleModel)

	r.MustRegister(Command{
		Name:             "provider",
		Description:      "Manage AI providers (switch, setup, test, fallback)",
		Category:         "AI & Model",
		Icon:             "󰒋",
		ArgsHint:         "[use|list|setup|test|set|unset|backend|fallback] ...",
		Tier:             TierSecondary,
		Type:             CmdFullPage,
		FullPageTitle:    "Provider Studio",
		SubPalette:       "provider",
		SubCommands: []SubCommand{
			{Name: "use", Description: "Switch active provider", ArgsHint: "<name>", Handler: handleProvider},
			{Name: "list", Description: "List configured providers", Handler: handleProvider},
			{Name: "setup", Description: "Setup a provider", ArgsHint: "<name>", Handler: handleProvider},
			{Name: "test", Description: "Test provider connectivity", ArgsHint: "<name>", Handler: handleProvider},
			{Name: "set", Description: "Set provider config", ArgsHint: "<key> <value>", Handler: handleProvider},
			{Name: "unset", Description: "Unset provider config", ArgsHint: "<key>", Handler: handleProvider},
			{Name: "backend", Description: "Manage provider backend", Handler: handleProvider},
			{Name: "fallback", Description: "Manage fallback chain", ArgsHint: "[add|remove|list]", Handler: handleProvider},
		},
		Completion: func(h Host, partial string) []string {
			cands := []string{"use", "list", "setup", "test", "set", "unset", "backend", "fallback"}
			if partial == "" {
				return cands
			}
			var out []string
			for _, c := range cands {
				if len(partial) <= len(c) && c[:len(partial)] == partial {
					out = append(out, c)
				}
			}
			return out
		},
		SupportsHeadless: true,
	}, handleProvider)

	r.MustRegister(Command{
		Name:             "mode",
		Description:      "Change agent mode",
		Category:         "AI & Model",
		Icon:             "󰒓",
		ArgsHint:         "<edit|plan>",
		Tier:             TierSecondary,
		SubPalette:       "mode",
		SubCommands: []SubCommand{
			{Name: "edit", Description: "Edit mode (manual approval)", Handler: handleMode},
			{Name: "plan", Description: "Plan mode (read-only)", Handler: handleMode},
		},
		Completion: func(h Host, partial string) []string {
			cands := []string{"edit", "plan"}
			if partial == "" {
				return cands
			}
			var out []string
			for _, c := range cands {
				if len(partial) <= len(c) && c[:len(partial)] == partial {
					out = append(out, c)
				}
			}
			return out
		},
		SupportsHeadless: true,
	}, handleMode)

	r.MustRegister(Command{
		Name:             "context",
		Aliases:          []string{"tokens"},
		Description:      "Show model and context usage",
		Category:         "AI & Model",
		Icon:             "󰚩",
		ArgsHint:         "[detail]",
		Tier:             TierTertiary,
		Immediate:        true,
		SupportsHeadless: true,
	}, handleContext)

	r.MustRegister(Command{
		Name:             "compact",
		Description:      "Compact context to fit token budget",
		Category:         "AI & Model",
		Icon:             "󰕳",
		Tier:             TierSecondary,
		Type:             CmdPrompt,
		Immediate:        true,
		SupportsHeadless: true,
		WhenToUse:        "When the conversation is close to the context budget",
		PromptTemplate:   "Compact the current conversation context. Summarize key points, decisions, and file references so far. Preserve all important context while reducing token count.",
	}, handleCompact)

	// --- Session ---
	r.MustRegister(Command{
		Name:             "new",
		Description:      "Start a fresh session",
		Category:         "Session",
		Icon:             "󰐕",
		Tier:             TierPrimary,
		Immediate:        true,
		SupportsHeadless: true,
	}, handleNew)

	r.MustRegister(Command{
		Name:             "sessions",
		Aliases:          []string{"session"},
		Description:      "Browse previous sessions",
		Category:         "Session",
		Icon:             "󰆓",
		Tier:             TierPrimary,
		Type:             CmdFullPage,
		FullPageTitle:    "Sessions",
		Immediate:        true,
	}, handleSessions)

	r.MustRegister(Command{
		Name:             "resume",
		Description:      "Browse and resume a session",
		Category:         "Session",
		Icon:             "󰑐",
		ArgsHint:         "[id]",
		Tier:             TierSecondary,
		Immediate:        true,
		SupportsHeadless: true,
	}, handleResume)

	r.MustRegister(Command{
		Name:             "clear",
		Description:      "Clear the conversation view",
		Category:         "Session",
		Icon:             "󰃢",
		Tier:             TierPrimary,
		Immediate:        true,
		SupportsHeadless: true,
	}, handleClear)

	r.MustRegister(Command{
		Name:             "reset",
		Description:      "Reset current session history",
		Category:         "Session",
		Icon:             "󰑓",
		Tier:             TierSecondary,
		Immediate:        true,
		SupportsHeadless: true,
	}, handleReset)

	r.MustRegister(Command{
		Name:             "export",
		Description:      "Export conversation as Markdown",
		Category:         "Session",
		Icon:             "󰈇",
		ArgsHint:         "[path]",
		Tier:             TierSecondary,
		Immediate:        true,
		SupportsHeadless: true,
	}, handleExport)

	r.MustRegister(Command{
		Name:             "permissions",
		Aliases:          []string{"approvals"},
		Description:      "View or revoke tool permissions and write-path rules",
		Category:         "Session",
		Icon:             "󰌑",
		ArgsHint:         "[revoke <index>]",
		Tier:             TierSecondary,
		Immediate:        true,
		SupportsHeadless: true,
	}, handlePermissions)

	r.MustRegister(Command{
		Name:             "rewind",
		Description:      "List or restore conversation checkpoints",
		Category:         "Session",
		Icon:             "󰤄",
		ArgsHint:         "[index]",
		Tier:             TierSecondary,
		Immediate:        true,
		SupportsHeadless: true,
	}, handleRewind)

	r.MustRegister(Command{
		Name:             "branch",
		Description:      "Fork this conversation into a new session",
		Category:         "Session",
		Icon:             "󰘬",
		ArgsHint:         "<name>",
		Tier:             TierTertiary,
		SupportsHeadless: true,
	}, handleBranch)

	r.MustRegister(Command{
		Name:             "summary",
		Description:      "Generate an LLM summary of the session",
		Category:         "Session",
		Icon:             "󰑝",
		ArgsHint:         "[emphasis]",
		Tier:             TierTertiary,
		Type:             CmdPrompt,
		SupportsHeadless: true,
		WhenToUse:        "When a thorough written recap of goals, changes and open items is needed",
		PromptTemplate:   "Generate a comprehensive summary of this session. Include: goals discussed, key decisions made, files modified, outstanding issues, and next steps.$ARGUMENTS",
	}, handleSummary)

	r.MustRegister(Command{
		Name:             "cost",
		Aliases:          []string{"usage"},
		Description:      "Show token and cost usage",
		Category:         "System",
		Icon:             "󰌧",
		Tier:             TierTertiary,
		Type:             CmdFullPage,
		FullPageTitle:    "Cost & Usage",
		Immediate:        true,
		SupportsHeadless: true,
	}, handleCost)

	r.MustRegister(Command{
		Name:             "config",
		Description:      "Show effective settings at a glance",
		Category:         "Configuration",
		Icon:             "󰒔",
		Tier:             TierTertiary,
		Type:             CmdFullPage,
		FullPageTitle:    "Configuration",
		Immediate:        true,
		SupportsHeadless: true,
	}, handleConfig)

	r.MustRegister(Command{
		Name:             "rename",
		Description:      "Rename the current session",
		Category:         "Session",
		Icon:             "󰘎",
		ArgsHint:         "<title>",
		Tier:             TierTertiary,
		SupportsHeadless: true,
	}, handleRename)

	r.MustRegister(Command{
		Name:             "recap",
		Description:      "Summarize the session so far",
		Category:         "Session",
		Icon:             "󰭗",
		Tier:             TierSecondary,
		Type:             CmdPrompt,
		Immediate:        true,
		SupportsHeadless: true,
		WhenToUse:        "To catch up on what happened in this session",
		PromptTemplate:   "Recap what has happened in this session so far. List the key points, decisions, and current state.",
	}, handleRecap)

	// --- Project ---
	r.MustRegister(Command{
		Name:        "tree",
		Aliases:     []string{"files"},
		Description: "Toggle project file tree",
		Category:    "Project",
		Icon:        "󰙅",
		Tier:        TierSecondary,
		Immediate:   true,
		Current:     func(h Host) bool { return h.ShowingFileTree() },
	}, handleTree)

	r.MustRegister(Command{
		Name:        "diff",
		Aliases:     []string{"changes"},
		Description: "Review workspace changes",
		Category:    "Project",
		Icon:        "󰈙",
		Tier:        TierPrimary,
		Type:        CmdFullPage,
		FullPageTitle: "Diff",
		Immediate:   true,
		Current:     func(h Host) bool { return h.DiffPaneVisible() },
	}, handleDiff)

	r.MustRegister(Command{
		Name:             "search",
		Description:      "Search workspace content",
		Category:         "Project",
		Icon:             "󰍉",
		ArgsHint:         "<query>",
		Tier:             TierSecondary,
		SupportsHeadless: true,
		Completion: func(h Host, partial string) []string {
			// Suggest recent search dirs and common terms.
			dirs := h.ExtraSearchDirs()
			if partial == "" {
				return dirs
			}
			var out []string
			for _, d := range dirs {
				if len(partial) <= len(d) && d[:len(partial)] == partial {
					out = append(out, d)
				}
			}
			return out
		},
	}, handleSearch)

	r.MustRegister(Command{
		Name:             "init",
		Description:      "Create AUTOMERGENT.md project memory",
		Category:         "Project",
		Icon:             "󰚝",
		Tier:             TierPrimary,
		Type:             CmdPrompt,
		Immediate:        true,
		SupportsHeadless: true,
		WhenToUse:        "When a project has no AUTOMERGENT.md yet",
		PromptTemplate:   "Initialize this project by creating an AUTOMERGENT.md file. Analyze the project structure, dependencies, and conventions. Write a comprehensive guide for AI assistants working on this codebase. Include: project overview, build system, test commands, coding style, key files and directories, and common patterns.",
	}, handleInit)

	r.MustRegister(Command{
		Name:             "context-files",
		Description:      "Show files touched this session",
		Category:         "Project",
		Icon:             "󰈔",
		Tier:             TierTertiary,
		Type:             CmdFullPage,
		FullPageTitle:    "Context Files",
		Immediate:        true,
		SupportsHeadless: true,
	}, handleFiles)

	r.MustRegister(Command{
		Name:             "add-dir",
		Description:      "Add an extra search root for /search",
		Category:         "Project",
		Icon:             "󰉖",
		ArgsHint:         "<path>",
		Tier:             TierTertiary,
		SupportsHeadless: true,
		Completion: func(h Host, partial string) []string {
			// Suggest extra search dirs.
			dirs := h.ExtraSearchDirs()
			if partial == "" {
				return dirs
			}
			var out []string
			for _, d := range dirs {
				if len(partial) <= len(d) && d[:len(partial)] == partial {
					out = append(out, d)
				}
			}
			return out
		},
	}, handleAddDir)

	// --- Workflow ---
	r.MustRegister(Command{
		Name:             "run",
		Description:      "Run a project command",
		Category:         "Workflow",
		Icon:             "󰆍",
		ArgsHint:         "<command>",
		Tier:             TierSecondary,
		SubPalette:       "run",
		SupportsHeadless: true,
	}, handleRun)

	r.MustRegister(Command{
		Name:             "test",
		Description:      "Detect and run project tests",
		Category:         "Workflow",
		Icon:             "󰙨",
		ArgsHint:         "[target]",
		Tier:             TierPrimary,
		Type:             CmdPrompt,
		Immediate:        true,
		SupportsHeadless: true,
		WhenToUse:        "To verify changes against the project test suite",
		PromptTemplate:   "Detect the project's test command and run the test suite using the shell tool. Request permission before execution.$ARGUMENTS",
	}, handleTest)

	r.MustRegister(Command{
		Name:             "build",
		Description:      "Detect and build the project",
		Category:         "Workflow",
		Icon:             "󰒋",
		ArgsHint:         "[target]",
		Tier:             TierSecondary,
		Type:             CmdPrompt,
		Immediate:        true,
		SupportsHeadless: true,
		PromptTemplate:   "Detect the project's build command and build the project using the shell tool. Request permission before execution.$ARGUMENTS",
	}, handleBuild)

	r.MustRegister(Command{
		Name:             "commit",
		Description:      "Commit pending changes via the agent",
		Category:         "Workflow",
		Icon:             "󰊢",
		ArgsHint:         "[focus]",
		Tier:             TierPrimary,
		Type:             CmdPrompt,
		SubPalette:       "commit",
		Immediate:        true,
		SupportsHeadless: true,
		WhenToUse:        "To turn current workspace changes into a proper git commit",
		PromptTemplate:   "Create a git commit for the pending workspace changes.\n1. Run `git status` and `git diff` (staged and unstaged) to understand the change.\n2. Draft a concise message in the repository's existing style (check `git log`).\n3. Stage only files relevant to this change and commit. Never push.\n4. If the diff is empty or unrelated files are mixed in, ask before proceeding.\nFocus: $ARGUMENTS",
	}, handleCommit)

	r.MustRegister(Command{
		Name:             "review",
		Description:      "Review changes or a pull request",
		Category:         "Workflow",
		Icon:             "󰤒",
		Aliases:          []string{"pr"},
		ArgsHint:         "[ref|#PR]",
		Tier:             TierPrimary,
		Type:             CmdPrompt,
		SubPalette:       "review",
		SupportsHeadless: true,
		WhenToUse:        "To get severity-ordered findings on pending changes or a PR",
		PromptTemplate:   "Perform a careful code review.\nTarget: $ARGUMENTS\nReport findings grouped by severity: blocking, should-fix, nit. For each: file:line, issue, suggested fix.\nCheck correctness, edge cases, error handling, security, and test coverage.\nDo not modify any files unless explicitly asked.",
	}, handleReview)

	r.MustRegister(Command{
		Name:             "security-review",
		Description:      "Security-focused review of pending changes",
		Category:         "Workflow",
		Icon:             "󰢽",
		ArgsHint:         "[focus]",
		Tier:             TierSecondary,
		Type:             CmdPrompt,
		Immediate:        true,
		SupportsHeadless: true,
		PromptTemplate:   "Perform a security-focused code review of the current changes. Check for: injection vulnerabilities, authentication/authorization issues, data exposure, insecure defaults, dependency vulnerabilities, and OWASP top 10. Report findings with severity levels.$ARGUMENTS",
	}, handleSecurityReview)

	r.MustRegister(Command{
		Name:             "issue",
		Description:      "Create a GitHub issue via gh",
		Category:         "Workflow",
		Icon:             "󰿚",
		ArgsHint:         "<title>",
		Tier:             TierTertiary,
		SupportsHeadless: true,
	}, handleIssue)

	r.MustRegister(Command{
		Name:             "pr-comments",
		Description:      "Fetch and summarize PR review comments",
		Category:         "Workflow",
		Icon:             "󰣏",
		ArgsHint:         "<PR number or URL>",
		Tier:             TierTertiary,
		SupportsHeadless: true,
	}, handlePRComments)

	r.MustRegister(Command{
		Name:        "review-mode",
		Description: "Toggle detailed change review",
		Category:    "Workflow",
		Icon:        "󰄬",
		Tier:        TierSecondary,
		Immediate:   true,
		Current:     func(h Host) bool { return h.IsReviewMode() },
	}, handleReviewMode)

	r.MustRegister(Command{
		Name:             "cancel",
		Aliases:          []string{"stop"},
		Description:      "Cancel the active request",
		Category:         "Workflow",
		Icon:             "󰅙",
		Tier:             TierSecondary,
		Immediate:        true,
		SupportsHeadless: true,
		Enabled:          func(h Host) bool { return h.Thinking() },
		DisabledReason:   func(h Host) string { return "No active request" },
	}, handleCancel)

	// --- Configuration ---
	r.MustRegister(Command{
		Name:             "api-key",
		Description:      "Set active provider API key",
		Category:         "Configuration",
		Icon:             "󰌆",
		ArgsHint:         "<value>",
		Tier:             TierSecondary,
		Sensitive:        true,
		SupportsHeadless: true,
	}, handleAPIKey)

	r.MustRegister(Command{
		Name:             "base-url",
		Description:      "Set active provider base URL",
		Category:         "Configuration",
		Icon:             "󰖟",
		ArgsHint:         "<url>",
		Tier:             TierTertiary,
		SupportsHeadless: true,
	}, handleBaseURL)

	r.MustRegister(Command{
		Name:             "effort",
		Description:      "Set or show provider thinking effort",
		Category:         "Configuration",
		Icon:             "󰓅",
		ArgsHint:         "[minimal|low|medium|high]",
		Tier:             TierSecondary,
		SubPalette:       "effort",
		SupportsHeadless: true,
	}, handleEffort)

	r.MustRegister(Command{
		Name:             "provider-api-key",
		Description:      "Set an AI provider API key",
		Category:         "Configuration",
		Icon:             "󰌋",
		ArgsHint:         "<provider> <value>",
		Tier:             TierTertiary,
		Sensitive:        true,
		SupportsHeadless: true,
	}, handleProviderAPIKey)

	r.MustRegister(Command{
		Name:             "provider-base-url",
		Description:      "Set an AI provider base URL",
		Category:         "Configuration",
		Icon:             "󰌷",
		ArgsHint:         "<provider> <url>",
		Tier:             TierTertiary,
		SupportsHeadless: true,
	}, handleProviderBaseURL)

	r.MustRegister(Command{
		Name:             "theme",
		Description:      "Switch or list UI themes",
		Category:         "Configuration",
		Icon:             "󰏘",
		ArgsHint:         "[name]",
		Tier:             TierSecondary,
		SubPalette:       "theme",
		SupportsHeadless: true,
	}, handleTheme)

	r.MustRegister(Command{
		Name:             "keybindings",
		Aliases:          []string{"keys"},
		Description:      "Switch or list keybinding schemes",
		Category:         "Configuration",
		Icon:             "󰌌",
		ArgsHint:         "[default|vim|emacs]",
		Tier:             TierSecondary,
		SubPalette:       "keybindings",
		SupportsHeadless: true,
	}, handleKeybindings)

	r.MustRegister(Command{
		Name:             "memory",
		Description:      "Show project and memory file locations",
		Category:         "Configuration",
		Icon:             "󰋞",
		Tier:             TierTertiary,
		Type:             CmdFullPage,
		FullPageTitle:    "Memory",
		Immediate:        true,
		SupportsHeadless: true,
	}, handleMemory)

	// --- System ---
	r.MustRegister(Command{
		Name:             "env",
		Description:      "Show runtime environment details",
		Category:         "System",
		Icon:             "󰟀",
		Tier:             TierTertiary,
		Type:             CmdFullPage,
		FullPageTitle:    "Environment",
		Immediate:        true,
		SupportsHeadless: true,
	}, handleEnv)

	r.MustRegister(Command{
		Name:             "version",
		Description:      "Show the Automergent version",
		Category:         "System",
		Icon:             "󰬐",
		Tier:             TierTertiary,
		Immediate:        true,
		SupportsHeadless: true,
	}, handleVersion)

	r.MustRegister(Command{
		Name:             "doctor",
		Description:      "Check configuration, provider and storage health",
		Category:         "System",
		Icon:             "󰨙",
		Tier:             TierPrimary,
		Type:             CmdFullPage,
		FullPageTitle:    "Doctor",
		Immediate:        true,
		SupportsHeadless: true,
	}, handleDoctor)

	r.MustRegister(Command{
		Name:        "stats",
		Description: "Show session statistics",
		Category:    "System",
		Icon:        "󰄪",
		Tier:        TierTertiary,
		Type:        CmdFullPage,
		FullPageTitle: "Statistics",
		Immediate:   true,
	}, handleStats)

	r.MustRegister(Command{
		Name:             "error",
		Aliases:          []string{"errors"},
		Description:      "Show recorded API errors and retries",
		Category:         "System",
		Icon:             "󰀦",
		ArgsHint:         "[clear|<n>]",
		Tier:             TierTertiary,
		Type:             CmdFullPage,
		FullPageTitle:    "Errors",
		Immediate:        true,
		SupportsHeadless: true,
		WhenToUse:        "After a request failed or took unusually long",
	}, handleErrors)

	r.MustRegister(Command{
		Name:        "help",
		Description: "Open keyboard and command help",
		Category:    "System",
		Icon:        "󰋖",
		Tier:        TierSecondary,
		Type:        CmdFullPage,
		FullPageTitle: "Help",
		Immediate:   true,
	}, handleHelp)

	r.MustRegister(Command{
		Name:        "quit",
		Aliases:     []string{"exit"},
		Description: "Exit Automergent",
		Category:    "System",
		Icon:        "󰗼",
		Tier:        TierSecondary,
		Immediate:   true,
	}, handleQuit)

	// --- MCP ---
	r.MustRegister(Command{
		Name:        "mcp",
		Description: "Manage MCP servers, tools, resources, and prompts",
		Category:    "MCP",
		Icon:        "󰌠",
		ArgsHint:    "[sub-command]",
		Tier:        TierSecondary,
		SubCommands: []SubCommand{
			{Name: "list", Description: "List all MCP servers", Handler: handleMCP},
			{Name: "enable", Description: "Enable an MCP server", ArgsHint: "<name>", Handler: handleMCP},
			{Name: "disable", Description: "Disable an MCP server", ArgsHint: "<name>", Handler: handleMCP},
			{Name: "reconnect", Description: "Reconnect MCP servers", Handler: handleMCP},
			{Name: "refresh", Description: "Refresh MCP tools and resources", Handler: handleMCP},
			{Name: "tools", Description: "List MCP tools", Handler: handleMCP},
			{Name: "resources", Description: "List MCP resources", Handler: handleMCP},
			{Name: "prompts", Description: "List MCP prompts", Handler: handleMCP},
			{Name: "events", Description: "Show MCP events", Handler: handleMCP},
			{Name: "cache", Description: "Manage MCP cache", Handler: handleMCP},
		},
		SupportsHeadless: true,
	}, handleMCP)

	// --- Knowledge ---
	r.MustRegister(Command{
		Name:        "skills",
		Description: "Browse available skills",
		Category:    "Knowledge",
		Icon:        "󰚩",
		Tier:        TierSecondary,
		Type:        CmdFullPage,
		FullPageTitle: "Skills",
		Immediate:   true,
	}, handleSkills)

	r.MustRegister(Command{
		Name:        "agents",
		Description: "Browse available agents",
		Category:    "Knowledge",
		Icon:        "󰧑",
		Tier:        TierSecondary,
		Type:        CmdFullPage,
		FullPageTitle: "Agents",
		Immediate:   true,
	}, handleAgents)

	r.MustRegister(Command{
		Name:        "tldr",
		Aliases:     []string{"explain"},
		Description: "Explain current code concisely",
		Category:    "Knowledge",
		Icon:        "󰋗",
		ArgsHint:    "[target]",
		Tier:        TierTertiary,
		Type:        CmdPrompt,
		PromptTemplate: "Explain the following concisely (tldr). Cover what it does, key edge cases, and risks. Target: $ARGUMENTS",
	}, handleTldr)

	// --- Workflow extras ---
	r.MustRegister(Command{
		Name:        "directory",
		Aliases:     []string{"dirs"},
		Description: "Manage extra search directories",
		Category:    "Project",
		Icon:        "󰉖",
		ArgsHint:    "[add <path>|show]",
		Tier:        TierSecondary,
		SubCommands: []SubCommand{
			{Name: "add", Description: "Add a search directory", ArgsHint: "<path>", Handler: handleDirectory},
			{Name: "show", Description: "Show extra search directories", Handler: handleDirectory},
		},
		Completion: func(h Host, partial string) []string {
			// Suggest directory sub-commands.
			cands := []string{"add", "show"}
			if partial == "" {
				return cands
			}
			var out []string
			for _, c := range cands {
				if len(partial) <= len(c) && c[:len(partial)] == partial {
					out = append(out, c)
				}
			}
			return out
		},
	}, handleDirectory)

	r.MustRegister(Command{
		Name:        "plan",
		Description: "Enter plan mode (read-only analysis)",
		Category:    "Workflow",
		Icon:        "󰈙",
		ArgsHint:    "[focus|copy]",
		Tier:        TierPrimary,
		Type:        CmdPrompt,
		SubCommands: []SubCommand{
			{Name: "copy", Description: "Copy current plan to clipboard", Handler: handlePlan},
		},
		PromptTemplate: "Enter plan mode. Read-only analysis before making changes. Outline approach, files to touch, and confirm before editing. Focus: $ARGUMENTS",
		WhenToUse: "Before making non-trivial changes",
	}, handlePlan)

	r.MustRegister(Command{
		Name:        "goal",
		Description: "Set or manage the thread goal",
		Category:    "Session",
		Icon:        "󰘧",
		ArgsHint:    "[clear|edit|pause|resume|<objective>]",
		Tier:        TierSecondary,
		SubCommands: []SubCommand{
			{Name: "clear", Description: "Clear the thread goal", Handler: handleGoal},
			{Name: "edit", Description: "Edit the thread goal", Handler: handleGoal},
			{Name: "pause", Description: "Pause the thread goal", Handler: handleGoal},
			{Name: "resume", Description: "Resume the thread goal", Handler: handleGoal},
		},
		Completion: func(h Host, partial string) []string {
			cands := []string{"clear", "edit", "pause", "resume"}
			if partial == "" {
				return cands
			}
			var out []string
			for _, c := range cands {
				if len(partial) <= len(c) && c[:len(partial)] == partial {
					out = append(out, c)
				}
			}
			return out
		},
	}, handleGoal)

	r.MustRegister(Command{
		Name:        "feedback",
		Aliases:     []string{"bug"},
		Description: "Send feedback or file an issue",
		Category:    "System",
		Icon:        "󰊤",
		ArgsHint:    "[message]",
		Tier:        TierTertiary,
	}, handleFeedback)

	r.MustRegister(Command{
		Name:        "copy",
		Description: "Copy last assistant message",
		Category:    "Session",
		Icon:        "󰅍",
		Tier:        TierTertiary,
		Immediate:   true,
	}, handleCopy)

	// --- Commands meta ---
	r.MustRegister(Command{
		Name:        "commands",
		Description: "Manage custom slash commands",
		Category:    "System",
		Icon:        "󰘳",
		ArgsHint:    "[list|reload]",
		Tier:        TierSecondary,
		SubCommands: []SubCommand{
			{Name: "list", Description: "List all custom commands", Handler: handleCommandsList},
			{Name: "reload", Description: "Reload custom commands from disk", Handler: handleCommandsReload},
		},
		Completion: func(h Host, partial string) []string {
			candidates := []string{"list", "reload"}
			if partial == "" {
				return candidates
			}
			var out []string
			for _, c := range candidates {
				if len(partial) <= len(c) && c[:len(partial)] == partial {
					out = append(out, c)
				}
			}
			return out
		},
	}, handleCommandsList)

	return r
}
