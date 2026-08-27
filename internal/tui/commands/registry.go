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
		SupportsHeadless: true,
	}, handleModel)

	r.MustRegister(Command{
		Name:             "provider",
		Description:      "Manage AI providers (switch, setup, test, fallback)",
		Category:         "AI & Model",
		Icon:             "󰒋",
		ArgsHint:         "[use|list|setup|test|set|unset|backend|fallback] ...",
		SupportsHeadless: true,
	}, handleProvider)

	r.MustRegister(Command{
		Name:             "mode",
		Description:      "Change agent mode",
		Category:         "AI & Model",
		Icon:             "󰒓",
		ArgsHint:         "<edit|plan>",
		SupportsHeadless: true,
	}, handleMode)

	r.MustRegister(Command{
		Name:             "context",
		Aliases:          []string{"tokens"},
		Description:      "Show model and context usage",
		Category:         "AI & Model",
		Icon:             "󰚩",
		ArgsHint:         "[detail]",
		Immediate:        true,
		SupportsHeadless: true,
	}, handleContext)

	r.MustRegister(Command{
		Name:             "compact",
		Description:      "Compact context to fit token budget",
		Category:         "AI & Model",
		Icon:             "󰕳",
		Immediate:        true,
		SupportsHeadless: true,
		WhenToUse:        "When the conversation is close to the context budget",
	}, handleCompact)

	// --- Session ---
	r.MustRegister(Command{
		Name:             "new",
		Description:      "Start a fresh session",
		Category:         "Session",
		Icon:             "󰐕",
		Immediate:        true,
		SupportsHeadless: true,
	}, handleNew)

	r.MustRegister(Command{
		Name:        "sessions",
		Aliases:     []string{"session"},
		Description: "Browse previous sessions",
		Category:    "Session",
		Icon:        "󰆓",
		Immediate:   true,
	}, handleSessions)

	r.MustRegister(Command{
		Name:             "resume",
		Description:      "Browse and resume a session",
		Category:         "Session",
		Icon:             "󰑐",
		ArgsHint:         "[id]",
		Immediate:        true,
		SupportsHeadless: true,
	}, handleResume)

	r.MustRegister(Command{
		Name:             "clear",
		Description:      "Clear the conversation view",
		Category:         "Session",
		Icon:             "󰃢",
		Immediate:        true,
		SupportsHeadless: true,
	}, handleClear)

	r.MustRegister(Command{
		Name:             "reset",
		Description:      "Reset current session history",
		Category:         "Session",
		Icon:             "󰑓",
		Immediate:        true,
		SupportsHeadless: true,
	}, handleReset)

	r.MustRegister(Command{
		Name:             "export",
		Description:      "Export conversation as Markdown",
		Category:         "Session",
		Icon:             "󰈇",
		ArgsHint:         "[path]",
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
		Immediate:        true,
		SupportsHeadless: true,
	}, handlePermissions)

	r.MustRegister(Command{
		Name:             "rewind",
		Description:      "List or restore conversation checkpoints",
		Category:         "Session",
		Icon:             "󰤄",
		ArgsHint:         "[index]",
		Immediate:        true,
		SupportsHeadless: true,
	}, handleRewind)

	r.MustRegister(Command{
		Name:             "branch",
		Description:      "Fork this conversation into a new session",
		Category:         "Session",
		Icon:             "󰘬",
		ArgsHint:         "<name>",
		SupportsHeadless: true,
	}, handleBranch)

	r.MustRegister(Command{
		Name:             "summary",
		Description:      "Generate an LLM summary of the session",
		Category:         "Session",
		Icon:             "󰑝",
		ArgsHint:         "[emphasis]",
		SupportsHeadless: true,
		WhenToUse:        "When a thorough written recap of goals, changes and open items is needed",
	}, handleSummary)

	r.MustRegister(Command{
		Name:             "cost",
		Aliases:          []string{"usage"},
		Description:      "Show token and cost usage",
		Category:         "System",
		Icon:             "󰌧",
		Immediate:        true,
		SupportsHeadless: true,
	}, handleCost)

	r.MustRegister(Command{
		Name:             "config",
		Description:      "Show effective settings at a glance",
		Category:         "Configuration",
		Icon:             "󰒔",
		Immediate:        true,
		SupportsHeadless: true,
	}, handleConfig)

	r.MustRegister(Command{
		Name:             "rename",
		Description:      "Rename the current session",
		Category:         "Session",
		Icon:             "󰘎",
		ArgsHint:         "<title>",
		SupportsHeadless: true,
	}, handleRename)

	r.MustRegister(Command{
		Name:             "recap",
		Description:      "Summarize the session so far",
		Category:         "Session",
		Icon:             "󰭗",
		Immediate:        true,
		SupportsHeadless: true,
		WhenToUse:        "To catch up on what happened in this session",
	}, handleRecap)

	// --- Project ---
	r.MustRegister(Command{
		Name:        "tree",
		Aliases:     []string{"files"},
		Description: "Toggle project file tree",
		Category:    "Project",
		Icon:        "󰙅",
		Immediate:   true,
		Current:     func(h Host) bool { return h.ShowingFileTree() },
	}, handleTree)

	r.MustRegister(Command{
		Name:        "diff",
		Aliases:     []string{"changes"},
		Description: "Review workspace changes",
		Category:    "Project",
		Icon:        "󰈙",
		Immediate:   true,
		Current:     func(h Host) bool { return h.DiffPaneVisible() },
	}, handleDiff)

	r.MustRegister(Command{
		Name:             "search",
		Description:      "Search workspace content",
		Category:         "Project",
		Icon:             "󰍉",
		ArgsHint:         "<query>",
		SupportsHeadless: true,
	}, handleSearch)

	r.MustRegister(Command{
		Name:             "init",
		Description:      "Create AUTOMERGENT.md project memory",
		Category:         "Project",
		Icon:             "󰚝",
		Immediate:        true,
		SupportsHeadless: true,
		WhenToUse:        "When a project has no AUTOMERGENT.md yet",
	}, handleInit)

	r.MustRegister(Command{
		Name:             "context-files",
		Description:      "Show files touched this session",
		Category:         "Project",
		Icon:             "󰈔",
		Immediate:        true,
		SupportsHeadless: true,
	}, handleFiles)

	r.MustRegister(Command{
		Name:             "add-dir",
		Description:      "Add an extra search root for /search",
		Category:         "Project",
		Icon:             "󰉖",
		ArgsHint:         "<path>",
		SupportsHeadless: true,
	}, handleAddDir)

	// --- Workflow ---
	r.MustRegister(Command{
		Name:             "run",
		Description:      "Run a project command",
		Category:         "Workflow",
		Icon:             "󰆍",
		ArgsHint:         "<command>",
		SupportsHeadless: true,
	}, handleRun)

	r.MustRegister(Command{
		Name:             "test",
		Description:      "Detect and run project tests",
		Category:         "Workflow",
		Icon:             "󰙨",
		ArgsHint:         "[target]",
		Immediate:        true,
		SupportsHeadless: true,
		WhenToUse:        "To verify changes against the project test suite",
	}, handleTest)

	r.MustRegister(Command{
		Name:             "build",
		Description:      "Detect and build the project",
		Category:         "Workflow",
		Icon:             "󰒋",
		ArgsHint:         "[target]",
		Immediate:        true,
		SupportsHeadless: true,
	}, handleBuild)

	r.MustRegister(Command{
		Name:             "commit",
		Description:      "Commit pending changes via the agent",
		Category:         "Workflow",
		Icon:             "󰊢",
		ArgsHint:         "[focus]",
		Immediate:        true,
		SupportsHeadless: true,
		WhenToUse:        "To turn current workspace changes into a proper git commit",
	}, handleCommit)

	r.MustRegister(Command{
		Name:             "review",
		Description:      "Review changes or a pull request",
		Category:         "Workflow",
		Icon:             "󰤒",
		Aliases:          []string{"pr"},
		ArgsHint:         "[ref|#PR]",
		SupportsHeadless: true,
		WhenToUse:        "To get severity-ordered findings on pending changes or a PR",
	}, handleReview)

	r.MustRegister(Command{
		Name:             "security-review",
		Description:      "Security-focused review of pending changes",
		Category:         "Workflow",
		Icon:             "󰢽",
		ArgsHint:         "[focus]",
		Immediate:        true,
		SupportsHeadless: true,
	}, handleSecurityReview)

	r.MustRegister(Command{
		Name:             "issue",
		Description:      "Create a GitHub issue via gh",
		Category:         "Workflow",
		Icon:             "󰿚",
		ArgsHint:         "<title>",
		SupportsHeadless: true,
	}, handleIssue)

	r.MustRegister(Command{
		Name:             "pr-comments",
		Description:      "Fetch and summarize PR review comments",
		Category:         "Workflow",
		Icon:             "󰣏",
		ArgsHint:         "<PR number or URL>",
		SupportsHeadless: true,
	}, handlePRComments)

	r.MustRegister(Command{
		Name:        "review-mode",
		Description: "Toggle detailed change review",
		Category:    "Workflow",
		Icon:        "󰄬",
		Immediate:   true,
		Current:     func(h Host) bool { return h.IsReviewMode() },
	}, handleReviewMode)

	r.MustRegister(Command{
		Name:             "cancel",
		Aliases:          []string{"stop"},
		Description:      "Cancel the active request",
		Category:         "Workflow",
		Icon:             "󰅙",
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
		Sensitive:        true,
		SupportsHeadless: true,
	}, handleAPIKey)

	r.MustRegister(Command{
		Name:             "base-url",
		Description:      "Set active provider base URL",
		Category:         "Configuration",
		Icon:             "󰖟",
		ArgsHint:         "<url>",
		SupportsHeadless: true,
	}, handleBaseURL)

	r.MustRegister(Command{
		Name:             "effort",
		Description:      "Set or show provider thinking effort",
		Category:         "Configuration",
		Icon:             "󰓅",
		ArgsHint:         "[minimal|low|medium|high]",
		SupportsHeadless: true,
	}, handleEffort)

	r.MustRegister(Command{
		Name:             "provider-api-key",
		Description:      "Set an AI provider API key",
		Category:         "Configuration",
		Icon:             "󰌋",
		ArgsHint:         "<provider> <value>",
		Sensitive:        true,
		SupportsHeadless: true,
	}, handleProviderAPIKey)

	r.MustRegister(Command{
		Name:             "provider-base-url",
		Description:      "Set an AI provider base URL",
		Category:         "Configuration",
		Icon:             "󰌷",
		ArgsHint:         "<provider> <url>",
		SupportsHeadless: true,
	}, handleProviderBaseURL)

	r.MustRegister(Command{
		Name:             "theme",
		Description:      "Switch or list UI themes",
		Category:         "Configuration",
		Icon:             "󰏘",
		ArgsHint:         "[name]",
		SupportsHeadless: true,
	}, handleTheme)

	r.MustRegister(Command{
		Name:             "keybindings",
		Aliases:          []string{"keys"},
		Description:      "Switch or list keybinding schemes",
		Category:         "Configuration",
		Icon:             "󰌌",
		ArgsHint:         "[default|vim|emacs]",
		SupportsHeadless: true,
	}, handleKeybindings)

	r.MustRegister(Command{
		Name:             "memory",
		Description:      "Show project and memory file locations",
		Category:         "Configuration",
		Icon:             "󰋞",
		Immediate:        true,
		SupportsHeadless: true,
	}, handleMemory)

	// --- System ---
	r.MustRegister(Command{
		Name:             "env",
		Description:      "Show runtime environment details",
		Category:         "System",
		Icon:             "󰟀",
		Immediate:        true,
		SupportsHeadless: true,
	}, handleEnv)

	r.MustRegister(Command{
		Name:             "version",
		Description:      "Show the Automergent version",
		Category:         "System",
		Icon:             "󰬐",
		Immediate:        true,
		SupportsHeadless: true,
	}, handleVersion)

	r.MustRegister(Command{
		Name:             "doctor",
		Description:      "Check configuration, provider and storage health",
		Category:         "System",
		Icon:             "󰨙",
		Immediate:        true,
		SupportsHeadless: true,
	}, handleDoctor)
	r.MustRegister(Command{
		Name:        "stats",
		Description: "Show session statistics",
		Category:    "System",
		Icon:        "󰄪",
		Immediate:   true,
	}, handleStats)

	r.MustRegister(Command{
		Name:             "error",
		Aliases:          []string{"errors"},
		Description:      "Show recorded API errors and retries",
		Category:         "System",
		Icon:             "󰀦",
		ArgsHint:         "[clear|<n>]",
		Immediate:        true,
		SupportsHeadless: true,
		WhenToUse:        "After a request failed or took unusually long",
	}, handleErrors)

	r.MustRegister(Command{
		Name:        "help",
		Description: "Open keyboard and command help",
		Category:    "System",
		Icon:        "󰋖",
		Immediate:   true,
	}, handleHelp)

	r.MustRegister(Command{
		Name:        "quit",
		Aliases:     []string{"exit"},
		Description: "Exit Automergent",
		Category:    "System",
		Icon:        "󰗼",
		Immediate:   true,
	}, handleQuit)

	// --- MCP ---
	r.MustRegister(Command{
		Name:             "mcp",
		Description:      "Manage MCP servers, tools, resources, and prompts",
		Category:         "MCP",
		Icon:             "󰌠",
		ArgsHint:         "[tools|resources|prompts|enable|disable|reconnect|refresh|events|cache]",
		SupportsHeadless: true,
	}, handleMCP)

	return r
}
