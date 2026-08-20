package command

// Default returns the default command registry with all commands registered.
func Default() *Registry {
	r := NewRegistry()

	// --- AI & Model ---
	r.MustRegister(Command{
		Name:        "model",
		Aliases:     []string{},
		Description: "Switch AI model",
		Category:    "AI & Model",
		Icon:        "󰊕",
		Usage:       "[name|reset]",
		Immediate:   false,
	}, handleModel)

	r.MustRegister(Command{
		Name:        "provider",
		Aliases:     []string{},
		Description: "Switch AI provider",
		Category:    "AI & Model",
		Icon:        "󰒋",
		Usage:       "[name] [model]",
		Immediate:   false,
	}, handleProvider)

	r.MustRegister(Command{
		Name:        "mode",
		Aliases:     []string{},
		Description: "Change agent mode",
		Category:    "AI & Model",
		Icon:        "󰒓",
		Usage:       "<edit|plan>",
		Immediate:   false,
	}, handleMode)

	r.MustRegister(Command{
		Name:        "context",
		Aliases:     []string{"tokens"},
		Description: "Show model and context usage",
		Category:    "AI & Model",
		Icon:        "󰚩",
		Usage:       "[detail]",
		Immediate:   true,
	}, handleContext)

	// --- Project ---
	r.MustRegister(Command{
		Name:        "tree",
		Aliases:     []string{"files"},
		Description: "Toggle project file tree",
		Category:    "Project",
		Icon:        "󰙅",
		Usage:       "",
		Immediate:   true,
	}, handleTree)

	r.MustRegister(Command{
		Name:        "diff",
		Aliases:     []string{"changes"},
		Description: "Review workspace changes",
		Category:    "Project",
		Icon:        "󰈙",
		Usage:       "",
		Immediate:   true,
	}, handleDiff)

	r.MustRegister(Command{
		Name:        "lsp",
		Aliases:     []string{"diagnostics"},
		Description: "Toggle project diagnostics",
		Category:    "Project",
		Icon:        "󰒓",
		Usage:       "",
		Immediate:   true,
	}, handleLSP)

	r.MustRegister(Command{
		Name:        "search",
		Aliases:     []string{},
		Description: "Search workspace content",
		Category:    "Project",
		Icon:        "󰍉",
		Usage:       "<query>",
		Immediate:   false,
	}, handleSearch)

	// --- Workflow ---
	r.MustRegister(Command{
		Name:        "run",
		Aliases:     []string{},
		Description: "Run a project command",
		Category:    "Workflow",
		Icon:        "󰆍",
		Usage:       "<command>",
		Immediate:   false,
	}, handleRun)

	r.MustRegister(Command{
		Name:        "test",
		Aliases:     []string{},
		Description: "Detect and run project tests",
		Category:    "Workflow",
		Icon:        "󰙨",
		Usage:       "[target]",
		Immediate:   true,
	}, handleTest)

	r.MustRegister(Command{
		Name:        "build",
		Aliases:     []string{},
		Description: "Detect and build the project",
		Category:    "Workflow",
		Icon:        "󰒋",
		Usage:       "[target]",
		Immediate:   true,
	}, handleBuild)

	r.MustRegister(Command{
		Name:        "review",
		Aliases:     []string{},
		Description: "Toggle detailed change review",
		Category:    "Workflow",
		Icon:        "󰄬",
		Usage:       "",
		Immediate:   true,
	}, handleReview)

	r.MustRegister(Command{
		Name:        "cancel",
		Aliases:     []string{"stop"},
		Description: "Cancel the active request",
		Category:    "Workflow",
		Icon:        "󰅙",
		Usage:       "",
		Immediate:   true,
	}, handleCancel)

	// --- Configuration ---
	r.MustRegister(Command{
		Name:        "api-key",
		Aliases:     []string{},
		Description: "Set active provider API key",
		Category:    "Configuration",
		Icon:        "󰌆",
		Usage:       "<value>",
		Immediate:   false,
	}, handleAPIKey)

	r.MustRegister(Command{
		Name:        "base-url",
		Aliases:     []string{},
		Description: "Set active provider base URL",
		Category:    "Configuration",
		Icon:        "󰖟",
		Usage:       "<url>",
		Immediate:   false,
	}, handleBaseURL)

	r.MustRegister(Command{
		Name:        "effort",
		Aliases:     []string{},
		Description: "Set or show provider thinking effort",
		Category:    "Configuration",
		Icon:        "󰓅",
		Usage:       "[minimal|low|medium|high]",
		Immediate:   false,
	}, handleEffort)

	r.MustRegister(Command{
		Name:        "provider-api-key",
		Aliases:     []string{},
		Description: "Set an AI provider API key",
		Category:    "Configuration",
		Icon:        "󰌋",
		Usage:       "<provider> <value>",
		Immediate:   false,
	}, handleProviderAPIKey)

	r.MustRegister(Command{
		Name:        "provider-base-url",
		Aliases:     []string{},
		Description: "Set an AI provider base URL",
		Category:    "Configuration",
		Icon:        "󰌷",
		Usage:       "<provider> <url>",
		Immediate:   false,
	}, handleProviderBaseURL)

	// --- System ---
	r.MustRegister(Command{
		Name:        "stats",
		Aliases:     []string{},
		Description: "Show session statistics",
		Category:    "System",
		Icon:        "󰄪",
		Usage:       "",
		Immediate:   true,
	}, handleStats)

	r.MustRegister(Command{
		Name:        "help",
		Aliases:     []string{},
		Description: "Open keyboard and command help",
		Category:    "System",
		Icon:        "󰋖",
		Usage:       "",
		Immediate:   true,
	}, handleHelp)

	r.MustRegister(Command{
		Name:        "quit",
		Aliases:     []string{"exit"},
		Description: "Exit Automergent",
		Category:    "System",
		Icon:        "󰗼",
		Usage:       "",
		Immediate:   true,
	}, handleQuit)

	// --- New commands ---
	r.MustRegister(Command{
		Name:        "theme",
		Aliases:     []string{},
		Description: "Switch or list UI themes",
		Category:    "Configuration",
		Icon:        "󰏘",
		Usage:       "[name]",
		Immediate:   false,
	}, handleTheme)

	r.MustRegister(Command{
		Name:        "keybindings",
		Aliases:     []string{"keys"},
		Description: "Switch or list keybinding schemes",
		Category:    "Configuration",
		Icon:        "󰌌",
		Usage:       "[default|vim|emacs]",
		Immediate:   false,
	}, handleKeybindings)

	r.MustRegister(Command{
		Name:        "compact",
		Aliases:     []string{},
		Description: "Compact context to fit token budget",
		Category:    "AI & Model",
		Icon:        "󰕳",
		Usage:       "",
		Immediate:   true,
	}, handleCompact)

	// --- Session-owned commands (metadata only, no handlers here) ---
	// These are handled by the session layer in app.go and MUST NOT be edited.
	// /new, /sessions, /session, /resume, /clear, /reset, /export, /approvals
	r.Register(Command{
		Name:        "new",
		Aliases:     []string{},
		Description: "Start a fresh session",
		Category:    "Session",
		Icon:        "󰐕",
		Usage:       "",
		Immediate:   true,
	}, nil)

	r.Register(Command{
		Name:        "sessions",
		Aliases:     []string{"session"},
		Description: "Browse previous sessions",
		Category:    "Session",
		Icon:        "󰆓",
		Usage:       "",
		Immediate:   true,
	}, nil)

	r.Register(Command{
		Name:        "resume",
		Aliases:     []string{},
		Description: "Browse and resume a session",
		Category:    "Session",
		Icon:        "󰑐",
		Usage:       "[id]",
		Immediate:   true,
	}, nil)

	r.Register(Command{
		Name:        "clear",
		Aliases:     []string{},
		Description: "Clear the conversation view",
		Category:    "Session",
		Icon:        "󰃢",
		Usage:       "",
		Immediate:   true,
	}, nil)

	r.Register(Command{
		Name:        "reset",
		Aliases:     []string{},
		Description: "Reset current session history",
		Category:    "Session",
		Icon:        "󰑓",
		Usage:       "",
		Immediate:   true,
	}, nil)

	r.Register(Command{
		Name:        "export",
		Aliases:     []string{},
		Description: "Export conversation as Markdown",
		Category:    "Session",
		Icon:        "󰈇",
		Usage:       "[path]",
		Immediate:   true,
	}, nil)

	r.Register(Command{
		Name:        "approvals",
		Aliases:     []string{},
		Description: "View or revoke always-allow tool approvals",
		Category:    "Session",
		Icon:        "󰌑",
		Usage:       "[revoke <index>]",
		Immediate:   true,
	}, nil)

	return r
}