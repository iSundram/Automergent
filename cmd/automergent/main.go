package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/iSundram/Automergent/internal/agent"
	aiPkg "github.com/iSundram/Automergent/internal/ai"
	googleProvider "github.com/iSundram/Automergent/internal/ai/google"
	"github.com/iSundram/Automergent/internal/cache"
	"github.com/iSundram/Automergent/internal/config"
	diagcache "github.com/iSundram/Automergent/internal/diagnostics/cache"
	"github.com/iSundram/Automergent/internal/debug"
	automergentErrors "github.com/iSundram/Automergent/internal/errors"
	"github.com/iSundram/Automergent/internal/git"
	"github.com/iSundram/Automergent/internal/mcp"
	"github.com/iSundram/Automergent/internal/agent/mcpbridge"
	planningPkg "github.com/iSundram/Automergent/internal/planning"
	"github.com/iSundram/Automergent/internal/session"
	"github.com/iSundram/Automergent/internal/tools"
	toolsAgent "github.com/iSundram/Automergent/internal/tools/agent"
	toolsDB "github.com/iSundram/Automergent/internal/tools/database"
	toolsFS "github.com/iSundram/Automergent/internal/tools/filesystem"
	toolsInteraction "github.com/iSundram/Automergent/internal/tools/interaction"
	toolsLSP "github.com/iSundram/Automergent/internal/tools/lsp"
	toolsSecurity "github.com/iSundram/Automergent/internal/tools/security"
	toolsCtxinfo "github.com/iSundram/Automergent/internal/tools/ctxinfo"
	toolsMcpres "github.com/iSundram/Automergent/internal/tools/mcpres"
	toolsPlanmode "github.com/iSundram/Automergent/internal/tools/planmode"
	toolsSkills "github.com/iSundram/Automergent/internal/tools/skills"
	toolsShell "github.com/iSundram/Automergent/internal/tools/shell"
	toolsWeb "github.com/iSundram/Automergent/internal/tools/web"
	"github.com/iSundram/Automergent/internal/tui"
	"github.com/iSundram/Automergent/internal/tui/themes"
	"github.com/iSundram/Automergent/internal/version"
)

var flags config.CLIFlags
var cfgFile string

type outputFormat string

const (
	outputFormatText       outputFormat = "text"
	outputFormatJSON       outputFormat = "json"
	outputFormatStreamJSON outputFormat = "stream-json"
)

type headlessRunners struct {
	text       func(context.Context, *agent.Agent, *session.Session, string) error
	json       func(context.Context, *agent.Agent, *session.Session, string) error
	streamJSON func(context.Context, *agent.Agent, *session.Session, string) error
}

type structuredToolCall struct {
	ID      string         `json:"id,omitempty"`
	Name    string         `json:"name,omitempty"`
	Context string         `json:"context,omitempty"`
	Args    map[string]any `json:"args,omitempty"`
}

type structuredToolResult struct {
	ID         string `json:"id,omitempty"`
	Name       string `json:"name,omitempty"`
	Context    string `json:"context,omitempty"`
	Status     string `json:"status,omitempty"`
	DurationMS int64  `json:"duration_ms,omitempty"`
	Output     string `json:"output,omitempty"`
}

type structuredError struct {
	Code     string         `json:"code"`
	Category string         `json:"category"`
	Message  string         `json:"message"`
	Details  map[string]any `json:"details,omitempty"`
}

type structuredEvent struct {
	Type       string                `json:"type"`
	Version    string                `json:"version"`
	Timestamp  time.Time             `json:"timestamp"`
	SessionID  string                `json:"session_id"`
	Seq        int                   `json:"seq"`
	TaskID     string                `json:"task_id,omitempty"`
	Phase      string                `json:"phase,omitempty"`
	Progress   *float64              `json:"progress_pct,omitempty"`
	ETASec     *int64                `json:"eta_sec,omitempty"`
	Log        string                `json:"log,omitempty"`
	Content    string                `json:"content,omitempty"`
	ToolCall   *structuredToolCall   `json:"tool_call,omitempty"`
	ToolResult *structuredToolResult `json:"tool_result,omitempty"`
	Error      *structuredError      `json:"error,omitempty"`
}

type longTaskStatus struct {
	TaskID     string
	Phase      string
	Progress   *float64
	ETASec     *int64
	Log        string
	Message    string
	Structured bool
}

type jsonHeadlessOutput struct {
	Version string            `json:"version"`
	Success bool              `json:"success"`
	Result  string            `json:"result"`
	Events  []structuredEvent `json:"events"`
	Usage   struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
	TimingMS int64 `json:"timing_ms"`
	Session  struct {
		ID string `json:"id"`
	} `json:"session"`
	Error *structuredError `json:"error"`
}

var defaultHeadlessRunners = headlessRunners{
	text:       runHeadlessText,
	json:       runHeadlessJSON,
	streamJSON: runHeadlessStreamJSON,
}

func main() {
	// Graceful shutdown: cancel context on SIGINT / SIGTERM.
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := rootCmd.ExecuteContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(int(automergentErrors.ExitCodeForError(err)))
	}
}

var rootCmd = &cobra.Command{
	Use:   "automergent [prompt]",
	Short: "Automergent – AI coding agent for the terminal",
	Long: `Automergent is an AI-powered coding agent that helps you write, edit,
and understand code directly in your terminal.`,
	Version: version.Version,
	Args:    cobra.ArbitraryArgs,
	RunE:    run,
}

func init() {
	cobra.OnInitialize(initConfig)

	f := rootCmd.Flags()
	f.StringVar(&cfgFile, "config", "", "config file (default ~/.automergent/config.yaml)")
	f.StringVarP(&flags.Prompt, "prompt", "p", "", "Non-interactive: run this prompt and exit")
	f.StringVar(&flags.Provider, "provider", "", "AI provider (google)")
	f.StringVarP(&flags.Model, "model", "m", "", "Model name")
	f.StringVar(&flags.Mode, "mode", "", "Approval mode: edit, plan")
	f.StringVar(&flags.Theme, "theme", "", "Color theme: catppuccin, dracula, modern")
	f.StringVar(&flags.Keybindings, "keybindings", "", "Key bindings: default, vim, emacs")
	f.StringVarP(&flags.Output, "output", "o", "text", "Output format: text | json (final envelope) | stream-json (NDJSON events)")
	f.BoolVar(&flags.NoTUI, "no-tui", false, "Disable TUI, write output to stdout")
	f.BoolVar(&flags.Stdin, "stdin", false, "Read prompt from stdin")
	f.BoolVar(&flags.NoColor, "no-color", false, "Disable color output")
	f.BoolVarP(&flags.Quiet, "quiet", "q", false, "Suppress non-essential output")
	f.BoolVar(&flags.Verbose, "verbose", false, "Enable verbose logging")
	f.BoolVar(&flags.NoAnimation, "no-animation", false, "Disable animations")
	f.StringVarP(&flags.Session, "session", "s", "", "Resume a specific session ID or name")
	f.BoolVar(&flags.Resume, "resume", false, "Open session picker to resume a chat")
	f.BoolVar(&flags.NewSession, "new-session", false, "Start a new session")
	f.StringVar(&flags.SessionDir, "session-dir", "", "Session storage directory")
	f.StringSliceVarP(&flags.ContextFiles, "context", "c", nil, "Extra context files to include")
	f.StringSliceVarP(&flags.Files, "file", "f", nil, "Files to include in context")
	f.BoolVar(&flags.NoSandbox, "no-sandbox", false, "Disable OS sandboxing")
	f.StringVar(&flags.Sandbox, "sandbox", "", "Sandbox type: auto | macos | docker | namespaces | off")
	f.StringVar(&flags.APIKey, "api-key", "", "API key (overrides env var)")
	f.StringVar(&flags.BaseURL, "base-url", "", "Custom API base URL")
	f.BoolVar(&flags.NoContext, "no-context", false, "Skip loading AUTOMERGENT.md files")
	f.StringVar(&flags.Layout, "layout", "", "TUI layout: auto | split | single")
}

func initConfig() {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		home, _ := os.UserHomeDir()
		viper.AddConfigPath(home + "/.automergent")
	}
	viper.SetEnvPrefix("AUTOMERGENT")
	viper.AutomaticEnv()
	_ = viper.ReadInConfig()
}

func run(cmd *cobra.Command, args []string) error {
	cfg := config.Default()
	if err := decodeConfigFromViper(cfg); err != nil {
		return fmt.Errorf("decode config: %w", err)
	}
	cfg.ConfigFile = viper.ConfigFileUsed()
	cfg.ApplyFlags(&flags)
	applyProjectDefaults(cfg, cmd)
	format := parseOutputFormat(cfg.Output)
	cfg.Output = string(format)

	// Initialize the persistent diagnostics cache (best-effort — analysis
	// falls back to the in-memory cache when the store cannot be opened).
	if cfg.Diagnostics.Enabled {
		if err := diagcache.InitGlobalCache("", 24*time.Hour); err != nil {
			// A read-only or corrupt cache directory must not stop the app.
			_ = err
		}
	}

	// Save config if critical settings were changed via flags to persist as last used.
	// Only save if a config file was actually loaded (avoid creating fresh config.yaml).
	configFileWasLoaded := viper.ConfigFileUsed() != ""
	if configFileWasLoaded && (flags.Provider != "" || flags.Model != "" || flags.APIKey != "" || flags.BaseURL != "") {
		_ = cfg.SaveIfLoaded()
	}

	// Resolve API keys from environment if not set
	resolveAPIKeysFromEnv(cfg)

	// Build prompt from --prompt flag, trailing args, or --stdin
	prompt := flags.Prompt
	if prompt == "" && len(args) > 0 {
		prompt = strings.Join(args, " ")
	}
	if prompt == "" && flags.Stdin {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("read stdin: %w", err)
		}
		prompt = strings.TrimSpace(string(data))
	}

	// When a prompt is supplied via flag/stdin, default to headless (no-tui)
	if prompt != "" && !cmd.Flags().Changed("no-tui") {
		cfg.NoTUI = true
	}

	// Determine if we should save config on exit (only for explicit user changes)
	shouldSaveConfig := flags.Provider != "" || flags.Model != "" || flags.APIKey != "" || flags.BaseURL != ""

	projectApprovalPath, needsProjectApproval := projectApprovalRequired(cfg)
	if !needsProjectApproval {
		projectApprovalPath = ""
	} else if cfg.NoTUI && prompt == "" && !flags.Stdin {
		promptProjectAllowedCLI(cfg, projectApprovalPath)
		projectApprovalPath = ""
	} else if cfg.NoTUI {
		projectApprovalPath = ""
	} else {
		// Interactive mode: gate workspace access before the TUI starts.
		approved, remember, err := tui.RunProjectApproval(cfg, projectApprovalPath)
		if err != nil {
			return fmt.Errorf("project approval: %w", err)
		}
		if !approved {
			if !flags.Quiet {
				fmt.Fprintln(os.Stderr, "Access to this folder was not approved.")
			}
			return nil
		}
		cfg.Security.AllowedWritePaths = appendUniquePath(cfg.Security.AllowedWritePaths, projectApprovalPath)
		if remember {
			_ = cfg.SaveIfLoaded()
		}
		projectApprovalPath = ""
	}

	// Setup session
	storage, err := session.NewStorage(cfg.SessionDir)
	if err != nil {
		return fmt.Errorf("session storage: %w", err)
	}

	// Crash-recovery persistence (state.json + recovery.json + autosave).
	pm, err := session.NewPersistenceManager(cfg.SessionDir)
	if err != nil {
		return fmt.Errorf("persistence manager: %w", err)
	}

	// Prune old sessions in the background to keep disk usage bounded.
	go func() {
		var maxAge time.Duration
		if cfg.MaxSessionAge != "" {
			maxAge, _ = time.ParseDuration(cfg.MaxSessionAge)
		}
		_ = storage.Prune(cfg.MaxSessions, maxAge)
	}()

	var sess *session.Session
	if flags.Session != "" {
		// Prefer a crash-recovery point for this session, then state.json,
		// then the session file on disk.
		sess, err = pm.ResumeSession(flags.Session, storage)
		if err != nil {
			return fmt.Errorf("load session %s: %w", flags.Session, err)
		}
		// Restore session-specific provider/model if not overridden by flags.
		if flags.Provider == "" && sess.Provider != "" {
			cfg.Provider = sess.Provider
		}
		if flags.Model == "" && sess.Model != "" {
			cfg.Model = sess.Model
		}
		// Save resumed session's settings as new default (only if provider/model changed)
		if flags.Provider != "" || flags.Model != "" {
			_ = cfg.SaveIfLoaded()
		}
	} else {
		sess = session.New()
		if prompt != "" {
			sess.Title = prompt
		} else {
			sess.Title = "New conversation"
		}
	}

	// Ensure session reflects the current provider/model.
	sess.Provider = cfg.Provider
	sess.Model = cfg.Model
	if wd, err := os.Getwd(); err == nil {
		sess.WorkDir = wd
	}

	// Build tool registry
	reg := tools.NewRegistry()

	// Filesystem tools
	reg.Register(toolsFS.NewReadFileTool(cfg))
	reg.Register(toolsFS.NewWriteFileTool(cfg))
	reg.Register(toolsFS.NewEditFileTool(cfg))
	reg.Register(toolsFS.NewNotebookEditTool(cfg))
	reg.Register(toolsFS.NewArtifactTool(cfg))
	reg.Register(&toolsFS.ListDirectoryTool{})
	reg.Register(&toolsFS.GlobTool{})
	reg.Register(&toolsFS.GrepTool{})

	// Shell tools (async-capable)
	reg.Register(toolsShell.NewAsyncRunnerTool(0))
	reg.Register(&toolsShell.ReadShellTool{})
	reg.Register(&toolsShell.WriteShellTool{})
	reg.Register(&toolsShell.StopShellTool{})
	reg.Register(&toolsShell.ListShellsTool{})

	// Web tools
	reg.Register(toolsWeb.NewFetchTool())
	reg.Register(toolsWeb.NewSearchTool())

	// LSP tools
	reg.Register(&toolsLSP.DiagnosticsTool{})

	// Security tools
	reg.Register(&toolsSecurity.SecretsScanTool{})
	reg.Register(&toolsSecurity.DependencyAuditTool{})

	// Database tools
	reg.Register(toolsDB.GetSQLTool())

	// Agent/sub-agent tools
	reg.Register(&toolsAgent.TaskTool{})
	reg.Register(&toolsAgent.ReadAgentTool{})
	reg.Register(&toolsAgent.ListAgentsTool{})
	reg.Register(planningPkg.NewTool("."))
	reg.Register(planningPkg.NewReplanTool("."))

	// Plan-mode, skill and context-inspection tools (UI hooks are installed
	// by the TUI when it starts; the tools degrade gracefully without them).
	reg.Register(&toolsPlanmode.EnterPlanModeTool{})
	reg.Register(&toolsPlanmode.ExitPlanModeTool{})
	reg.Register(&toolsPlanmode.VerifyPlanExecutionTool{})
	reg.Register(&toolsSkills.DiscoverSkillsTool{})
	reg.Register(&toolsSkills.SkillTool{})
	reg.Register(&toolsCtxinfo.CtxInspectTool{})

	// Skills: project skills override user skills (later dirs win).
	skillDirs := []string{}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		skillDirs = append(skillDirs, filepath.Join(home, ".automergent", "skills"))
	}
	if wd, err := os.Getwd(); err == nil {
		skillDirs = append(skillDirs, filepath.Join(wd, ".automergent", "skills"))
	}
	toolsSkills.SetDirs(skillDirs...)

	// MCP tools
	var mcpOrch *mcp.Orchestrator
	var mcpBr *mcpbridge.Bridge
	if len(cfg.MCP.Servers) > 0 {
		mcpCfg := mcp.FromAppConfig(cfg.MCP)
		mcpOrch = mcp.NewOrchestrator(mcpCfg)
		mcpBr = mcpbridge.New(mcpOrch)
		// Start orchestrator in background (non-blocking)
		go func() {
			if err := mcpOrch.Start(context.Background()); err != nil {
				if cfg.Verbose || cfg.Debug.Enabled {
					fmt.Fprintf(os.Stderr, "[MCP] orchestrator start: %v\n", err)
				}
			}
			// Register MCP tools into the registry
			mcpBr.Sync(reg)
			// MCP resource tools (list/read published resources).
			reg.Register(toolsMcpres.NewListTool(mcpOrch))
			reg.Register(toolsMcpres.NewReadTool(mcpOrch))
		}()
		// Watch config for hot-reload (best-effort)
		if cfg.ConfigFile != "" {
			go mcpOrch.WatchConfig(cfg.ConfigFile)
		}
	}

	// Interaction tools
	// NotifyTool will be registered after agent creation so it can emit events to the UI.

	// Get AI provider
	provider, err := resolveProvider(cfg)
	if err != nil {
		return fmt.Errorf("provider: %w", err)
	}

	// Build agent
	ag := agent.New(cfg, provider, sess, reg)

	// Background subagent completions reach the model as <task-notification>
	// messages instead of waiting for a read_agent poll.
	ag.EnableSubagentNotifications()

	// Shell watchdog events (interactive-prompt stalls, output-cap kills)
	// reach the model the same way, and the shell's persistent working
	// directory starts from the launch directory.
	if wd, err := os.Getwd(); err == nil {
		toolsShell.SetOriginalCwd(wd)
	}
	toolsShell.RegisterModelNotification(func(message string) {
		ag.Steer(message)
	})

	// Wire MCP bridge to agent for event emission
	if mcpBr != nil {
		mcpBr.SetAgent(ag)
		// Re-sync tools now that agent exists (for events)
		go func() {
			time.Sleep(2 * time.Second) // Give orchestrator time to connect
			mcpBr.SyncWithEvents(reg)
		}()
	}

	// Graph-backed context operations are registered after the agent exists so
	// they share the same persistent task/bucket store as orchestration.
	contextTools := ag.RegisterContextTools()
	if cfg.Verbose || cfg.Debug.Enabled {
		fmt.Fprintf(os.Stderr, "[CONTEXT] config_file=%q context_enabled=%v reason=%q\n",
			cfg.ConfigFile, contextTools.Enabled, contextTools.Reason)
	}
	ag.SetSessionPersist(func() {
		// The TUI can switch sessions after startup. Persist the agent's
		// current session rather than the pointer captured during launch.
		current := ag.Session()
		if current == nil {
			return
		}
		_ = storage.Save(current)
		pm.SetSession(current)
		_ = pm.SaveRecoveryPoint()
	})

	// Register NotifyTool now that we have an agent to emit UI events.
	reg.Register(toolsInteraction.NewNotifyTool(func(level string, title string, message string) error {
		// Use agent events to surface notifications in the TUI
		ag.Emit(agent.EventNotify, map[string]any{"level": level, "title": title, "message": message})
		return nil
	}))

	// Set the main agent as the executor for sub-agent tools.
	toolsAgent.GetAgentManager().SetExecutor(ag)
	toolsAgent.GetAgentManager().RegisterCompletionHook(func(n toolsAgent.AgentNotification) {
		if n.Status == toolsAgent.AgentStatusCompleted {
			ag.Emit(agent.EventNotify, map[string]any{
				"level":   "info",
				"title":   "Background agent complete",
				"message": fmt.Sprintf("%s (%s) finished in %s", n.AgentID, n.Type, n.Duration.Truncate(time.Second)),
			})
			return
		}
		if n.Status == toolsAgent.AgentStatusFailed {
			ag.Emit(agent.EventNotify, map[string]any{
				"level":   "error",
				"title":   "Background agent failed",
				"message": fmt.Sprintf("%s failed: %s", n.AgentID, n.ErrMessage),
			})
		}
	})
	toolsShell.GetManager().RegisterStatusHook(func(n toolsShell.SessionNotification) {
		switch n.Status {
		case toolsShell.SessionStatusCompleted:
			ag.Emit(agent.EventNotify, map[string]any{
				"level":   "info",
				"title":   "Background shell complete",
				"message": fmt.Sprintf("%s finished (exit %d)", n.ID, n.ExitCode),
			})
		case toolsShell.SessionStatusFailed:
			ag.Emit(agent.EventNotify, map[string]any{
				"level":   "error",
				"title":   "Background shell failed",
				"message": fmt.Sprintf("%s failed (exit %d)", n.ID, n.ExitCode),
			})
		}
	})
	// Re-register ask_user with agent-aware responder for TUI
	reg.Register(toolsInteraction.NewAskUserTool(func(question string) (string, error) {
		if cfg.NoTUI {
			fmt.Fprintf(os.Stdout, "\n[ask_user] %s\n> ", question)
			reader := bufio.NewReader(os.Stdin)
			answer, err := reader.ReadString('\n')
			if err != nil {
				return "", err
			}
			return strings.TrimSpace(answer), nil
		}

		// TUI mode: emit event and wait for response
		ch := make(chan string, 1)
		ag.Emit(agent.EventAskUser, map[string]any{
			"question": question,
			"reply":    ch,
		})
		select {
		case res := <-ch:
			return res, nil
		case <-time.After(time.Hour): // Safety timeout
			return "", fmt.Errorf("user response timeout")
		}
	}))

	// Track workspace context for crash recovery and future editor resume.
	if wd, err := os.Getwd(); err == nil {
		pm.SetWorkDir(wd)
		pm.SetProjectPath(wd)
		pm.SetGitBranch(detectGitBranch(wd))
	}
	pm.SetSession(sess)
	pm.StartAutoSave(30)

	// Save session on exit; only save config if explicitly modified via flags
	defer func() {
		current := ag.Session()
		if current == nil {
			current = sess
		}
		_ = storage.Save(current)
		pm.SetSession(current)
		_ = pm.Save()
		pm.StopAutoSave()
		if shouldSaveConfig {
			_ = cfg.SaveIfLoaded()
		}
		if !flags.Quiet && (format == outputFormatText) {
			printComprehensiveExitMessage(current)
		}
	}()

	if cfg.NoTUI {
		return runHeadless(cmd.Context(), ag, sess, prompt, format)
	}
	if format != outputFormatText {
		return fmt.Errorf("--output=%s requires --no-tui mode", format)
	}

	showPicker := flags.Resume && flags.Session == ""
	return tui.Run(cfg, ag, sess, storage, pm, prompt, showPicker, projectApprovalPath, mcpOrch)
}

func applyProjectDefaults(cfg *config.Config, cmd *cobra.Command) {
	if cmd.Flags().Changed("session-dir") {
		return
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	cfg.SessionDir = filepath.Join(home, ".automergent", "sessions")
	_ = os.MkdirAll(filepath.Join(home, ".automergent"), 0o700)
}

// detectGitBranch returns the current git branch of dir, or "" when dir is
// not inside a repository or git is unavailable.
func detectGitBranch(dir string) string {
	branch, err := git.CurrentBranch(context.Background(), dir)
	if err != nil {
		return ""
	}
	return branch
}

func parseOutputFormat(raw string) outputFormat {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case string(outputFormatJSON):
		return outputFormatJSON
	case string(outputFormatStreamJSON):
		return outputFormatStreamJSON
	case string(outputFormatText):
		return outputFormatText
	default:
		return outputFormatText
	}
}

func runHeadless(ctx context.Context, ag *agent.Agent, sess *session.Session, prompt string, format outputFormat) error {
	return runHeadlessWithRunners(ctx, ag, sess, prompt, format, defaultHeadlessRunners)
}

func runHeadlessWithRunners(ctx context.Context, ag *agent.Agent, sess *session.Session, prompt string, format outputFormat, runners headlessRunners) error {
	switch format {
	case outputFormatJSON:
		return runners.json(ctx, ag, sess, prompt)
	case outputFormatStreamJSON:
		return runners.streamJSON(ctx, ag, sess, prompt)
	default:
		return runners.text(ctx, ag, sess, prompt)
	}
}

func runHeadlessWithConsumer(ctx context.Context, ag *agent.Agent, prompt string, consume func(agent.Event)) error {
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		events := ag.Events()
		for {
			select {
			case ev := <-events:
				consume(ev)
			case <-stop:
				return
			}
		}
	}()

	runErr := ag.Run(ctx, prompt)
	close(stop)
	wg.Wait()
	drainPendingEvents(ag.Events(), consume)
	return runErr
}

func drainPendingEvents(events <-chan agent.Event, consume func(agent.Event)) {
	for {
		select {
		case ev := <-events:
			consume(ev)
		default:
			return
		}
	}
}

func runHeadlessText(ctx context.Context, ag *agent.Agent, _ *session.Session, prompt string) error {
	if prompt == "" {
		return fmt.Errorf("prompt required in no-tui mode")
	}
	sawError := false
	runErr := runHeadlessWithConsumer(ctx, ag, prompt, func(ev agent.Event) {
		switch ev.Type {
		case agent.EventToken:
			if tok, ok := ev.Payload.(string); ok {
				fmt.Print(tok)
			}
		case agent.EventStatus:
			status := parseLongTaskStatus(ev.Payload)
			if !status.Structured {
				break
			}
			renderLongTaskStatusText(os.Stderr, status)
		case agent.EventToolCall:
			if te, ok := ev.Payload.(agent.ToolCallEvent); ok {
				reasons := ""
				if len(te.Decision.Reasons) > 0 {
					codes := make([]string, 0, len(te.Decision.Reasons))
					for _, reason := range te.Decision.Reasons {
						codes = append(codes, string(reason.Code))
					}
					reasons = fmt.Sprintf(" reasons=%s", strings.Join(codes, ","))
				}
				fmt.Fprintf(os.Stderr, "\n[tool:start] %s id=%s strategy=%s%s\n", te.Name, te.ID, te.Decision.Strategy, reasons)
			} else if tc, ok := ev.Payload.(aiPkg.ToolCall); ok {
				fmt.Fprintf(os.Stderr, "\n[tool: %s]\n", tc.Name)
			}
		case agent.EventToolDone:
			if td, ok := ev.Payload.(agent.ToolDoneEvent); ok {
				state := "ok"
				if td.Result.IsError {
					state = "error"
				}
				fmt.Fprintf(os.Stderr, "\n[tool:%s] %s id=%s dur=%s\n", state, td.Name, td.ID, td.Duration.Round(time.Millisecond))
			}
		case agent.EventDone:
			fmt.Println()
		case agent.EventError:
			sawError = true
			printTextStructuredError(structuredErrorFromPayload(ev.Payload, nil))
		}
	})
	if runErr != nil && !sawError {
		printTextStructuredError(structuredErrorFromPayload(runErr, nil))
	}
	return runErr
}

func runHeadlessJSON(ctx context.Context, ag *agent.Agent, sess *session.Session, prompt string) error {
	return runHeadlessJSONToWriter(ctx, ag, sess, prompt, os.Stdout)
}

func runHeadlessJSONToWriter(ctx context.Context, ag *agent.Agent, sess *session.Session, prompt string, w io.Writer) error {
	start := time.Now()
	if prompt == "" {
		out := jsonHeadlessOutput{
			Version:  "v1",
			Success:  false,
			Result:   "",
			Events:   []structuredEvent{},
			TimingMS: 0,
			Error: &structuredError{
				Code:     "invalid_args",
				Category: "validation",
				Message:  "prompt required in no-tui mode",
				Details:  map[string]any{"source": "headless"},
			},
		}
		if sess != nil {
			out.Session.ID = sess.ID
		}
		if err := json.NewEncoder(w).Encode(out); err != nil {
			return fmt.Errorf("encode json output: %w", err)
		}
		return fmt.Errorf("prompt required in no-tui mode")
	}

	var (
		mu          sync.Mutex
		events      []structuredEvent
		tokens      strings.Builder
		finalResult string
		firstErr    *structuredError
	)
	events = append(events, structuredEvent{
		Type:      "init",
		Version:   "v1",
		Timestamp: time.Now().UTC(),
		SessionID: func() string {
			if sess != nil {
				return sess.ID
			}
			return ""
		}(),
		Seq: 1,
	})

	runErr := runHeadlessWithConsumer(ctx, ag, prompt, func(ev agent.Event) {
		mapped := mapStructuredEvent(ev)

		mu.Lock()
		defer mu.Unlock()
		mapped.Version = "v1"
		if sess != nil {
			mapped.SessionID = sess.ID
		}
		mapped.Seq = len(events) + 1
		events = append(events, mapped)
		if mapped.Type == "token" {
			tokens.WriteString(mapped.Content)
		}
		if mapped.Type == "done" && mapped.Content != "" {
			finalResult = mapped.Content
		}
		if mapped.Error != nil && firstErr == nil {
			firstErr = mapped.Error
		}
	})

	mu.Lock()
	defer mu.Unlock()
	if finalResult == "" {
		finalResult = tokens.String()
	}
	if runErr != nil && firstErr == nil {
		errModel := structuredErrorFromPayload(runErr, nil)
		firstErr = &errModel
	}

	payload := jsonHeadlessOutput{
		Version:  "v1",
		Success:  runErr == nil && firstErr == nil,
		Result:   finalResult,
		Events:   events,
		TimingMS: time.Since(start).Milliseconds(),
		Error:    firstErr,
	}
	if sess != nil {
		payload.Session.ID = sess.ID
		payload.Usage.InputTokens = sess.TotalInputTokens
		payload.Usage.OutputTokens = sess.TotalOutputTokens
	}

	if err := json.NewEncoder(w).Encode(payload); err != nil {
		return fmt.Errorf("encode json output: %w", err)
	}

	return runErr
}

func runHeadlessStreamJSON(ctx context.Context, ag *agent.Agent, sess *session.Session, prompt string) error {
	if prompt == "" {
		emitStreamEvent(structuredEvent{
			Type:      "error",
			Version:   "v1",
			Timestamp: time.Now().UTC(),
			Error: &structuredError{
				Code:     "invalid_args",
				Category: "validation",
				Message:  "prompt required in no-tui mode",
				Details:  map[string]any{"source": "headless"},
			},
		})
		return fmt.Errorf("prompt required in no-tui mode")
	}
	var sawError bool
	var sawDone bool
	seq := 0
	seq++
	emitStreamEvent(structuredEvent{
		Type:    "init",
		Version: "v1",
		Seq:     seq,
		SessionID: func() string {
			if sess != nil {
				return sess.ID
			}
			return ""
		}(),
		Timestamp: time.Now().UTC(),
	})
	runErr := runHeadlessWithConsumer(ctx, ag, prompt, func(ev agent.Event) {
		mapped := mapStructuredEvent(ev)
		if mapped.Error != nil {
			sawError = true
		}
		if mapped.Type == "done" {
			sawDone = true
		}
		seq++
		mapped.Version = "v1"
		mapped.Seq = seq
		if sess != nil {
			mapped.SessionID = sess.ID
		}
		emitStreamEvent(mapped)
	})
	if runErr != nil && !sawError {
		emitStreamEvent(structuredEvent{
			Type:    "error",
			Version: "v1",
			Seq:     seq + 1,
			SessionID: func() string {
				if sess != nil {
					return sess.ID
				}
				return ""
			}(),
			Error:     ptrStructuredError(structuredErrorFromPayload(runErr, nil)),
			Timestamp: time.Now().UTC(),
		})
		seq++
	}
	if !sawDone {
		emitStreamEvent(structuredEvent{
			Type:    "done",
			Version: "v1",
			Seq:     seq + 1,
			SessionID: func() string {
				if sess != nil {
					return sess.ID
				}
				return ""
			}(),
			Timestamp: time.Now().UTC(),
		})
	}
	return runErr
}

func emitStreamEvent(ev structuredEvent) {
	encoded, err := json.Marshal(ev)
	if err != nil {
		fallback, _ := json.Marshal(structuredEvent{
			Type:    "error",
			Version: "v1",
			Error: &structuredError{
				Code:     "internal_error",
				Category: "internal",
				Message:  "failed to serialize event",
				Details:  map[string]any{"source": "emitStreamEvent"},
			},
			Timestamp: time.Now().UTC(),
		})
		fmt.Fprintln(os.Stdout, string(fallback))
		return
	}
	fmt.Fprintln(os.Stdout, string(encoded))
}

func mapStructuredEvent(ev agent.Event) structuredEvent {
	out := structuredEvent{
		Type:      "status",
		Timestamp: time.Now().UTC(),
	}
	switch ev.Type {
	case agent.EventToken:
		out.Type = "token"
		if content, ok := ev.Payload.(string); ok {
			out.Content = content
		}
	case agent.EventDone:
		out.Type = "done"
		if content, ok := ev.Payload.(string); ok {
			out.Content = content
		}
	case agent.EventThought, agent.EventThinking:
		out.Type = "status"
		if content, ok := ev.Payload.(string); ok {
			out.Content = content
		}
	case agent.EventStatus:
		out.Type = "status"
		status := parseLongTaskStatus(ev.Payload)
		if status.Message != "" {
			out.Content = status.Message
		} else if content, ok := ev.Payload.(string); ok {
			out.Content = content
		}
		out.TaskID = status.TaskID
		out.Phase = status.Phase
		out.Progress = status.Progress
		out.ETASec = status.ETASec
		out.Log = status.Log
	case agent.EventToolCall:
		out.Type = "tool_call"
		if te, ok := ev.Payload.(agent.ToolCallEvent); ok {
			out.ToolCall = &structuredToolCall{
				ID:      te.ID,
				Name:    te.Name,
				Context: te.Context,
				Args:    te.Args,
			}
		} else if tc, ok := ev.Payload.(aiPkg.ToolCall); ok {
			out.ToolCall = &structuredToolCall{
				ID:   tc.ID,
				Name: tc.Name,
				Args: tc.Args,
			}
		}
	case agent.EventToolDone:
		out.Type = "tool_result"
		if td, ok := ev.Payload.(agent.ToolDoneEvent); ok {
			status := "success"
			if td.Result.IsError {
				status = "error"
			}
			output := td.Result.Summary
			if output == "" {
				output = td.Result.Content
			}
			out.ToolResult = &structuredToolResult{
				ID:         td.ID,
				Name:       td.Name,
				Context:    td.Context,
				Status:     status,
				DurationMS: td.Duration.Round(time.Millisecond).Milliseconds(),
				Output:     output,
			}
		}
	case agent.EventError:
		out.Type = "error"
		out.Error = ptrStructuredError(structuredErrorFromPayload(ev.Payload, nil))
	default:
		out.Type = "status"
		if content, ok := ev.Payload.(string); ok {
			out.Content = content
		}
	}
	return out
}

func parseLongTaskStatus(payload any) longTaskStatus {
	status := longTaskStatus{}
	switch v := payload.(type) {
	case string:
		status.Message = v
		return status
	case agent.LongTaskStatus:
		status.Structured = true
		status.TaskID = v.TaskID
		status.Phase = v.Phase
		progress := v.ProgressPct
		status.Progress = &progress
		if v.ETASec > 0 {
			eta := v.ETASec
			status.ETASec = &eta
		}
		status.Log = v.Log
		status.Message = v.Message
		return status
	case map[string]any:
		status.Structured = true
		if val, ok := v["task_id"].(string); ok {
			status.TaskID = val
		}
		if val, ok := v["phase"].(string); ok {
			status.Phase = val
		}
		if val, ok := v["message"].(string); ok {
			status.Message = val
		}
		if val, ok := v["log"].(string); ok {
			status.Log = val
		}
		if val, ok := asFloat(v["progress_pct"]); ok {
			status.Progress = &val
		}
		if val, ok := asInt64(v["eta_sec"]); ok {
			status.ETASec = &val
		}
		return status
	default:
		return status
	}
}

func asFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	default:
		return 0, false
	}
}

func asInt64(v any) (int64, bool) {
	switch n := v.(type) {
	case int:
		return int64(n), true
	case int32:
		return int64(n), true
	case int64:
		return n, true
	case float64:
		return int64(n), true
	case float32:
		return int64(n), true
	default:
		return 0, false
	}
}

func formatLongTaskStatusText(status longTaskStatus) string {
	parts := make([]string, 0, 5)
	if status.TaskID != "" {
		parts = append(parts, fmt.Sprintf("task=%s", status.TaskID))
	}
	if status.Phase != "" {
		parts = append(parts, fmt.Sprintf("phase=%s", status.Phase))
	}
	if status.Progress != nil {
		parts = append(parts, fmt.Sprintf("progress=%.1f%%", *status.Progress))
	}
	if status.ETASec != nil {
		parts = append(parts, fmt.Sprintf("eta=%ds", *status.ETASec))
	}
	if status.Message != "" {
		parts = append(parts, status.Message)
	}
	return strings.TrimSpace(strings.Join(parts, " "))
}

func renderLongTaskStatusText(w io.Writer, status longTaskStatus) {
	if status.Log != "" {
		if status.Phase != "" {
			fmt.Fprintf(w, "\n[log] [%s] %s\n", status.Phase, status.Log)
		} else {
			fmt.Fprintf(w, "\n[log] %s\n", status.Log)
		}
	}
	if rendered := formatLongTaskStatusText(status); rendered != "" {
		fmt.Fprintf(w, "\n[progress] %s\n", rendered)
	}
}

func classifyErrorCode(err error) string {
	return classifyErrorCodeMessage(errorMessage(err))
}

func classifyErrorCodeMessage(msg string) string {
	if msg == "" {
		return "internal_error"
	}
	lower := strings.ToLower(msg)
	contains := func(parts ...string) bool {
		for _, part := range parts {
			if strings.Contains(lower, part) {
				return true
			}
		}
		return false
	}
	if contains("auth", "authentication", "unauthorized", "forbidden", "api key", "credential", "401", "403") {
		return "auth_failed"
	}
	code := automergentErrors.ErrorCategoryForError(fmt.Errorf("%s", msg))
	if code == "" {
		return "internal_error"
	}
	return code
}

func classifyErrorCategory(code string) string {
	switch code {
	case "invalid_args":
		return "validation"
	case "auth_failed":
		return "auth"
	case "provider_error":
		return "provider"
	case "tool_error":
		return "tool"
	case "context_limit":
		return "context"
	default:
		return "internal"
	}
}

func structuredErrorFromPayload(payload any, fallback error) structuredError {
	switch v := payload.(type) {
	case structuredError:
		return fillErrorDefaults(v)
	case *structuredError:
		if v != nil {
			return fillErrorDefaults(*v)
		}
	case *automergentErrors.AutomergentError:
		return structuredErrorFromAutomergentError(v)
	case error:
		if oce := automergentErrors.GetAutomergentError(v); oce != nil {
			return structuredErrorFromAutomergentError(oce)
		}
		return structuredErrorFromMessage(v.Error(), fmt.Sprintf("%T", v))
	case string:
		return structuredErrorFromMessage(v, "string")
	case nil:
		if fallback != nil {
			return structuredErrorFromPayload(fallback, nil)
		}
		return structuredErrorFromMessage("unknown error", "nil")
	default:
		return structuredErrorFromMessage(fmt.Sprintf("%v", v), fmt.Sprintf("%T", v))
	}
	if fallback != nil {
		return structuredErrorFromPayload(fallback, nil)
	}
	return structuredErrorFromMessage("unknown error", "nil")
}

func structuredErrorFromMessage(message, payloadType string) structuredError {
	code := classifyErrorCodeMessage(message)
	return fillErrorDefaults(structuredError{
		Code:     code,
		Category: classifyErrorCategory(code),
		Message:  message,
		Details: map[string]any{
			"raw":          message,
			"payload_type": payloadType,
		},
	})
}

func structuredErrorFromAutomergentError(oce *automergentErrors.AutomergentError) structuredError {
	if oce == nil {
		return structuredErrorFromMessage("unknown error", "nil")
	}
	message := oce.Message
	if message == "" && oce.Err != nil {
		message = oce.Err.Error()
	}
	if message == "" {
		message = "unknown error"
	}
	details := map[string]any{}
	for k, v := range oce.Context {
		details[k] = v
	}
	if oce.Operation != "" {
		details["operation"] = oce.Operation
	}
	if oce.Resource != "" {
		details["resource"] = oce.Resource
	}
	if oce.RequestID != "" {
		details["request_id"] = oce.RequestID
	}
	if oce.Suggestion != "" {
		details["suggestion"] = oce.Suggestion
	}
	if oce.Err != nil {
		details["raw"] = oce.Err.Error()
	}
	return fillErrorDefaults(structuredError{
		Code:     strings.ToLower(string(oce.Code)),
		Category: string(oce.Category),
		Message:  message,
		Details:  details,
	})
}

func fillErrorDefaults(in structuredError) structuredError {
	if in.Code == "" {
		in.Code = "internal_error"
	}
	if in.Category == "" {
		in.Category = classifyErrorCategory(in.Code)
	}
	if in.Message == "" {
		in.Message = "unknown error"
	}
	if len(in.Details) == 0 {
		in.Details = nil
	}
	return in
}

func errorMessage(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func ptrStructuredError(e structuredError) *structuredError {
	out := e
	return &out
}

func printTextStructuredError(e structuredError) {
	encodedDetails, err := json.Marshal(e.Details)
	if err != nil {
		encodedDetails = []byte(`{"marshal_error":"failed to serialize details"}`)
	}
	fmt.Fprintf(os.Stderr, "\nError: code=%s category=%s message=%q details=%s\n", e.Code, e.Category, e.Message, encodedDetails)
}

func resolveProvider(cfg *config.Config) (aiPkg.Provider, error) {
	pc := cfg.Providers[cfg.Provider]
	enablePromptCache := shouldEnablePromptCache(cfg, cfg.Provider)
	aiCfg := aiPkg.ProviderConfig{
		APIKey:             pc.APIKey,
		BaseURL:            pc.BaseURL,
		DefaultModel:       cfg.Model,
		OrgID:              pc.OrgID,
		Project:            pc.Project,
		Location:           pc.Location,
		Effort:             pc.Effort,
		PromptCacheEnabled: &enablePromptCache,
	}

	var provider aiPkg.Provider
	switch cfg.Provider {
	case "google":
		provider = googleProvider.New(aiCfg)
	default:
		// Check global registry
		if p, ok := aiPkg.Get(cfg.Provider); ok {
			provider = p
		} else {
			return nil, fmt.Errorf("unknown provider %q", cfg.Provider)
		}
	}

	if shouldWrapPromptCacheProvider(cfg, provider) {
		provider = cache.NewCachingProvider(provider, cache.NewPromptCache())
	}

	// Wrap with debug provider if debug mode is enabled
	if cfg.Debug.Enabled {
		sessionID := debug.NewSessionID()
		logger, err := debug.NewLogger(cfg.Debug, sessionID)
		if err != nil {
			return nil, fmt.Errorf("failed to create debug logger: %w", err)
		}
		if logger != nil {
			provider = debug.NewDebugProvider(provider, logger)
		}
	}

	return provider, nil
}

func shouldEnablePromptCache(cfg *config.Config, provider string) bool {
	if !cfg.Cache.Prompt.Enabled {
		return false
	}
	switch provider {
	default:
		return false
	}
}

func shouldWrapPromptCacheProvider(cfg *config.Config, provider aiPkg.Provider) bool {
	return shouldEnablePromptCache(cfg, provider.Name())
}

func resolveAPIKeysFromEnv(cfg *config.Config) {
	envMap := map[string]string{
		"google": "GEMINI_API_KEY",
	}
	if cfg.Providers == nil {
		cfg.Providers = map[string]config.ProviderConfig{}
	}
	for provider, envVar := range envMap {
		if val := os.Getenv(envVar); val != "" {
			pc, exists := cfg.Providers[provider]
			// Only set from env if not already present in config (manual set)
			if !exists || pc.APIKey == "" {
				pc.APIKey = val
				cfg.Providers[provider] = pc
			}
		}
	}
}

func decodeConfigFromViper(cfg *config.Config) error {
	// Unmarshal directly into the config struct using viper's built-in support.
	// This handles mapstructure tags correctly.
	if err := viper.Unmarshal(cfg); err != nil {
		return fmt.Errorf("viper unmarshal: %w", err)
	}
	// Keep this feature flag explicit. Viper's decode behavior can vary when a
	// config file is absent, when an environment key is present, or when a
	// previously-used Viper instance is reset. The default must remain intact
	// unless the user actually supplied the key.
	return nil
}

func printComprehensiveExitMessage(sess *session.Session) {
	fmt.Println(formatComprehensiveExitMessage(sess))
}

func formatComprehensiveExitMessage(sess *session.Session) string {
	duration := time.Since(sess.CreatedAt).Round(time.Second)
	toolCount := 0
	for _, m := range sess.Messages {
		if m.Role == aiPkg.RoleTool {
			toolCount++
		}
	}

	tips := []string{
		"Use /compress to keep your context window lean.",
		"Type /sessions to browse, search, rename and resume previous chats.",
		"Use /diff to toggle the diff view during a session.",
		"The /stats command shows detailed token information.",
		"Automergent supports many models; try /provider to switch.",
	}
	tip := tips[time.Now().Unix()%int64(len(tips))]

	accent := lipgloss.NewStyle().Foreground(lipgloss.Color("255")).Bold(true)
	textStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	command := lipgloss.NewStyle().Foreground(lipgloss.Color("153")).Bold(true)
	success := lipgloss.NewStyle().Foreground(lipgloss.Color("114"))
	// The banner prints after the TUI has torn down, so there is no live theme to
	// read. Building styles from the default theme keeps the mark identical to the
	// one the header and the conversation label just showed.
	brand := themes.NewStyles(themes.Modern())

	const width = 72
	rule := dim.Render(strings.Repeat("─", width))
	shortID := sess.ID
	if len(shortID) > 8 {
		shortID = shortID[:8] + "…"
	}
	field := func(label, value string) string {
		return " " + dim.Width(12).Render(label) + textStyle.Render(value)
	}

	stats := fmt.Sprintf(" Messages %-6d  Tools %-6d  Tokens %d in / %d out",
		len(sess.Messages), toolCount, sess.TotalInputTokens, sess.TotalOutputTokens)

	return strings.Join([]string{
		"",
		" " + brand.BrandMark() + accent.Render(themes.BrandName) + "   " + dim.Render("SESSION COMPLETE"),
		rule,
		"",
		field("Session", shortID),
		field("Title", titleOrFallback(sess)),
		field("Duration", duration.String()),
		"",
		" " + dim.Render("Resume this session"),
		"   " + command.Render("amt -s "+sess.ID),
		"   " + command.Render("automergent -s "+sess.ID),
		"",
		textStyle.Render(stats),
		"",
		" " + dim.Render("Tip") + "  " + dim.Italic(true).Render(tip),
		"",
		rule,
		" " + success.Render("󰄬 Session saved") + "  " + dim.Render("See you next time."),
		"",
	}, "\n")
}

// titleOrFallback gives the exit banner a session name: the stored title, or
// the first line of the opening user message when the session is untitled.
func titleOrFallback(sess *session.Session) string {
	if t := strings.TrimSpace(sess.Title); t != "" {
		return t
	}
	for _, m := range sess.Messages {
		if m.Role != aiPkg.RoleUser {
			continue
		}
		line := strings.TrimSpace(m.PlaintextForHistory())
		if idx := strings.IndexAny(line, "\n"); idx >= 0 {
			line = line[:idx]
		}
		if len(line) > 60 {
			line = line[:57] + "..."
		}
		if line != "" {
			return line
		}
	}
	return "Untitled session"
}

// appendUniquePath appends path to paths unless already present.
func appendUniquePath(paths []string, path string) []string {
	for _, existing := range paths {
		if existing == path {
			return paths
		}
	}
	return append(paths, path)
}

func projectApprovalRequired(cfg *config.Config) (string, bool) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", false
	}
	absCwd, err := filepath.Abs(cwd)
	if err != nil {
		return "", false
	}

	allowed := false
	for _, p := range cfg.Security.AllowedWritePaths {
		absP, err := filepath.Abs(p)
		if err == nil && (absCwd == absP || strings.HasPrefix(absCwd, absP+string(filepath.Separator))) {
			allowed = true
			break
		}
	}
	return absCwd, !allowed
}

func promptProjectAllowedCLI(cfg *config.Config, projectPath string) {
	fmt.Fprintf(os.Stderr, "This project is not in allowed directory (%s). Do you want to add it? [y/N]: ", projectPath)
	reader := bufio.NewReader(os.Stdin)
	resp, err := reader.ReadString('\n')
	if err == nil {
		resp = strings.TrimSpace(strings.ToLower(resp))
		if resp == "y" || resp == "yes" {
			cfg.Security.AllowedWritePaths = append(cfg.Security.AllowedWritePaths, projectPath)
			_ = cfg.SaveIfLoaded()
			fmt.Fprintln(os.Stderr, "Project added to allowed directories.")
		}
	}
}
