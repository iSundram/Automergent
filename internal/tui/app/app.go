package app

// Core App model: struct, construction, Bubble Tea lifecycle.
// Moved verbatim from internal/tui/app.go.

import (
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"context"
	"fmt"
	"github.com/iSundram/Automergent/internal/agent"
	"github.com/iSundram/Automergent/internal/ai"
	"github.com/iSundram/Automergent/internal/config"
	"github.com/iSundram/Automergent/internal/mcp"
	"github.com/iSundram/Automergent/internal/session"
	"github.com/iSundram/Automergent/internal/tui/commands"
	"github.com/iSundram/Automergent/internal/tui/components"
	"github.com/iSundram/Automergent/internal/tui/keys"
	"github.com/iSundram/Automergent/internal/tui/render"
	"github.com/iSundram/Automergent/internal/tui/themes"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type agentEventMsg struct{ ev agent.Event }
type modelsFetchedMsg []ai.Model
type clearCtrlCStatusMsg struct{}
type clearEscArmMsg struct{}  // Expires the armed double-ESC clear window.
type hideDiffPaneMsg struct{} // Message to safely hide diff pane from main loop
type streamTickMsg struct{}   // Coalesced streaming render tick (~80ms)
type liveTickMsg struct{}     // 1-second tick for live time displays (elapsed, etc.)
type mcpEventMsg struct{ ev mcp.ServerEvent }
type mcpConfigChangeMsg struct{ ev mcp.ConfigChangeEvent }
type sessionsLoadedMsg struct {
	sessions []*session.Session
}
type projectApprovalMsg struct{ response agent.ConfirmationResponse }

type App struct {
	cfg               *config.Config
	lastBranchCheck   time.Time
	toasts            *components.Toasts
	reviewingProposal string
	questionnaire     *components.Questionnaire
	taskBoard         *components.TaskBoard
	dock              *components.BottomDock
	// inspector is the full-screen viewer for one background task. It replaces
	// the old habit of formatting a snapshot into the diff pane, which borrowed
	// the wrong chrome and could never update.
	inspector *components.Inspector
	// inspectorFilterMode routes printable keys into the inspector's filter
	// pattern instead of its command grammar.
	inspectorFilterMode bool
	// passthroughCards tracks "!command" runs whose terminal card is still in
	// flight, so shell status hooks can tell an inline passthrough (handled by
	// its own done message) from a real background command.
	passthroughCards map[string]bool
	// inspected is the dock entry currently shown in the inspector, so its
	// shortcuts act on the entry on screen rather than the dock's cursor row.
	inspected components.DockEntry
	// queueStrip shows what is waiting to be sent. The queue itself lives in
	// msgQueue; this is the only thing that ever made it visible.
	queueStrip      *components.QueueStrip
	zenMode         bool
	sendToProgram   func(tea.Msg)
	pendingAsk      *pendingAsk
	ag              *agent.Agent
	sess            *session.Session
	storage         *session.Storage
	persist         *session.PersistenceManager
	keys            *keys.Bindings
	styles          *themes.Styles
	theme           *themes.Theme
	conversation    components.Conversation
	diffPane        components.Diff
	input           components.Input
	header          components.Header
	statusBar       components.StatusBar
	spin            components.Spinner
	confirm         components.Confirm
	sessionBrowser  components.SessionBrowser
	artifactBrowser components.ArtifactBrowser

	// artifacts are agent-produced deliverables (plans, reviews, docs)
	// awaiting or carrying user review decisions; /artifact browses them.
	artifacts []artifactRecord

	// pendingPlanReview is the exit_plan_mode call waiting for the user's
	// decision in the artifact browser; planModePrev is the mode to restore
	// when a plan is approved.
	pendingPlanReview *pendingPlanReview
	planModePrev      string
	selector          components.SelectorOverlay
	selectorAction    func(index int)
	stats             components.Stats
	helpOverlay       components.HelpOverlay
	fullPage          components.FullPage
	pageViewer        components.PageViewer
	providerStudio    *components.ProviderStudio
	modelHub          *components.ModelHub
	fileTree          components.FileTree
	palette           components.CommandPalette
	width             int
	height            int
	thinking          bool
	showSessionPicker bool
	workDir           string
	// swallowNextKey drops the next key event delivered to the session browser
	// so the key that opened it (ctrl+s, or /sessions from the palette) cannot
	// be misinterpreted as a selection inside it.
	swallowNextKey bool
	statusMsg      string

	// commands is the app-wide command registry: the single source of truth
	// for slash-command dispatch, palette items and help documentation.
	commands *commands.Registry

	// dispatchDepth guards cross-command invocation (Host.DispatchCommand)
	// against runaway recursion; pendingDispatchCmds collects the tea.Cmds
	// produced by nested dispatches so they are executed exactly once.
	dispatchDepth       int
	dispatchActive      bool
	pendingDispatchCmds []tea.Cmd

	// Custom command hot-reload state (see refreshCustomCommands).
	customCmdCount int
	customWarnKey  string

	// Conversation rewind points (captured per agent turn) and extra
	// read-only search roots added via /add-dir.
	checkpoints     []conversationCheckpoint
	extraSearchDirs []string

	// fileWriteSnapshots captures a file's pre-write content at EventToolCall
	// so EventToolDone can diff before→after and open a diff-pane tab —
	// including writes that never asked for confirmation (accept-edits/auto
	// mode, always-allow grants) and newly created files. Keyed by tool ID.
	fileWriteSnapshots map[string]fileWriteSnapshot

	// goal is the active autonomy objective driving the continuation loop
	// (see goal.go). nil when no goal is set.
	goal *goalState

	// Memory consolidation state (see dream.go): last pass time, the
	// message count baseline it covered, and the in-flight flag.
	dreamLastAt      time.Time
	dreamMsgBaseline int
	dreamRunning     bool

	// workflowRuns is the recent pipeline run history (see workflow.go).
	workflowRuns []workflowRun

	// settledAgentRows records subagent live rows that reached a terminal
	// state, so the tick stops rewriting them (see agent_rows.go).
	settledAgentRows map[string]bool

	showFileTree  bool
	showHelp      bool
	focusMode     bool // When true, show Diff on top and Confirm on bottom
	ctx           context.Context
	cancel        context.CancelFunc
	initialPrompt string
	focus         string

	availableModels    []ai.Model
	fetchingModels     bool
	availableProviders []string
	streamedReply      bool
	browsing           bool
	lastCtrlCAt        time.Time
	askUserReplyCh     chan string

	// Bottom-chrome state. infoLine renders the `└─` hint line; the rest is
	// derived UI state consumed by uistate.go (see refreshChrome).
	infoLine components.InfoLine

	// escArmed/lastEscAt implement the double-ESC input clear.
	escArmed   bool
	lastEscAt  time.Time
	ctrlCArmed bool

	// msgQueue holds messages typed while a run is in flight.
	msgQueue []queuedMessage

	// MCP orchestrator for external tool servers.
	mcpOrch   *mcp.Orchestrator
	mcpEvents []MCPEventEntry

	// activeTool is the tool currently executing, "" when none is.
	activeTool string
	// runToolCount counts tools completed in the current (or last) run, so an
	// interruption can report how far it got.
	runToolCount int
	// sessionProblems counts diagnostics that blocked edits this session,
	// surfaced as the "N problems" chip in the status bar.
	sessionProblems int
	// permissionTool names the tool a visible permission prompt is asking about.
	permissionTool string
	// lastOutcome is the sticky result of the last run: one of the outcome*
	// constants in errorlog.go.
	lastOutcome string

	// API error history and live retry state backing /error and the footer's
	// retry indicator.
	apiErrors    []apiErrorRecord
	retrying     bool
	retryAttempt int
	retryMax     int
	retryCode    string
	retryDetail  string
	retryDelay   time.Duration
	retryDelayAt time.Time // when retryDelay was set, for live countdown

	// pendingDiffHide is set when confirmation completes and diff should be hidden
	pendingDiffHide bool

	pendingProjectPath string
	projectApprovalCh  chan agent.ConfirmationResponse

	// Streaming render coalescing + live telemetry.
	streamTickPending bool
	runChars          int       // streamed characters in the current run (~4 chars ≈ 1 token)
	runStart          time.Time // when the current run started
	spinLabel         string    // current activity verb (reading/editing/…); the stream tick decorates it with the "(12s • ↓ N tokens)" readout
	lastRunSummary    string    // settled readout of the last completed run, rendered in the spinner slot
	clockMinute       string    // last-painted minute of the footer clock, so it repaints only on rollover
}

func NewApp(cfg *config.Config, ag *agent.Agent, sess *session.Session, storage *session.Storage, persist *session.PersistenceManager, initialPrompt string, showSessionPicker bool, mcpOrch *mcp.Orchestrator) *App {
	theme := themes.Get(cfg.Theme)
	styles := themes.NewStyles(theme)
	render.SetTheme(theme)
	kb := keys.Get(cfg.Keybindings)
	ctx, cancel := context.WithCancel(context.Background())

	app := &App{
		cfg:                cfg,
		ag:                 ag,
		sess:               sess,
		storage:            storage,
		persist:            persist,
		keys:               kb,
		styles:             styles,
		theme:              theme,
		mcpOrch:            mcpOrch,
		conversation:       components.NewConversation(styles),
		diffPane:           components.NewDiff(styles),
		// Prompt history persists beside the sessions dir (typically
		// ~/.automergent/history.txt) so up-arrow recall survives restarts.
		input:              components.NewInput(styles).WithHistoryFile(filepath.Join(filepath.Dir(cfg.SessionDir), "history.txt")),
		header:             components.NewHeader(styles),
		statusBar:          components.NewStatusBar(styles),
		infoLine:           components.NewInfoLine(styles),
		toasts:             components.NewToasts(styles),
		questionnaire:      components.NewQuestionnaire(styles),
		dock:               components.NewBottomDock(styles),
		inspector:          components.NewInspector(styles),
		passthroughCards:   make(map[string]bool),
		settledAgentRows:   make(map[string]bool),
		fileWriteSnapshots: make(map[string]fileWriteSnapshot),
		queueStrip:         components.NewQueueStrip(styles),
		taskBoard:          components.NewTaskBoard(styles),
		spin:               components.NewSpinner(styles),
		confirm:            components.NewConfirm(styles),
		sessionBrowser:     components.NewSessionBrowser(styles),
		artifactBrowser:    components.NewArtifactBrowser(styles),
		selector:           components.NewSelectorOverlay(styles),
		stats:              components.NewStats(styles),
		helpOverlay:        components.NewHelpOverlay(styles),
		fullPage:           components.NewFullPage(styles),
		pageViewer:         components.NewPageViewer(styles),
		providerStudio:     components.NewProviderStudio(styles),
		modelHub:           components.NewModelHub(styles),
		fileTree:           components.NewFileTree(styles),
		palette:            components.NewCommandPalette(styles),
		ctx:                ctx,
		cancel:             cancel,
		initialPrompt:      initialPrompt,
		showSessionPicker:  showSessionPicker,
		statusMsg:          "Ready",
		focus:              "input",
		availableProviders: config.ProviderNames(),
		commands:           commands.Default(),
	}
	app.providerStudio.SetHost(app)
	app.modelHub.SetHost(app)
	sort.Strings(app.availableProviders)
	if wd, err := os.Getwd(); err == nil {
		app.workDir = wd
	}
	// Artifacts are session-scoped: restore this session's registry from its
	// metadata (nothing from other sessions shows up).
	app.loadArtifactsForSession()
	// Load markdown custom commands (project + user roots) before help rows
	// are derived, so they appear in the palette and help overlay.
	if n, warnings := commands.LoadProjectAndUserCommands(app.commands, app.workDir); n > 0 || len(warnings) > 0 {
		var msg string
		switch {
		case n > 0 && len(warnings) > 0:
			msg = fmt.Sprintf("Loaded %d custom command(s), %d skipped:\n%s", n, len(warnings), strings.Join(warnings, "\n"))
		case n > 0:
			msg = fmt.Sprintf("Loaded %d custom command(s) from .automergent/commands", n)
		default:
			msg = "Custom command problems:\n" + strings.Join(warnings, "\n")
		}
		app.conversation.AddMessage("system", msg, false)
	}
	// Wire MCP orchestrator event handlers for TUI notifications
	if app.mcpOrch != nil {
		app.mcpOrch.SetEventHandlers(
			// Server event handler: emit agent events for toasts
			func(ev mcp.ServerEvent) {
				if app.sendToProgram != nil {
					app.sendToProgram(mcpEventMsg{ev: ev})
				}
			},
			// Config change handler: log config reloads
			func(ev mcp.ConfigChangeEvent) {
				if app.sendToProgram != nil {
					app.sendToProgram(mcpConfigChangeMsg{ev: ev})
				}
			},
		)
	}

	app.helpOverlay.SetSlashCommands(app.commands.HelpRows())
	app.helpOverlay.SetSlashSections(app.commands.HelpSections())
	// Derive the input layer's sub-palette trigger set from the registry so
	// the two can never drift: every command with a SubPalette becomes a
	// trigger. (The static map in input.go seeds this; registration here is
	// idempotent.)
	components.SyncSubPaletteTriggers(app.commands.SubPaletteNames())
	app.syncCommandHints()
	app.header.SetModel(cfg.Model)
	app.header.SetProvider(cfg.Provider)
	app.header.SetEffort(cfg.Effort)
	if wd, err := os.Getwd(); err == nil {
		app.statusBar.SetWorkDir(wd)
	}
	// Normalise a legacy "edit" mode from a persisted config to its current
	// name so the chip and the approval gate agree on what mode this is.
	cfg.Mode = agent.CanonicalMode(cfg.Mode)
	if cfg.Mode == "" {
		cfg.Mode = "manual"
	}
	app.header.SetMode(cfg.Mode)
	app.statusBar.SetMode(cfg.Mode)
	app.header.SetPhase(string(agent.DetectPhase(sess.Messages)))
	app.header.SetTokens(sess.TotalInputTokens + sess.TotalOutputTokens)
	app.stats.InputTokens = sess.TotalInputTokens
	app.stats.OutputTokens = sess.TotalOutputTokens
	if len(sess.Messages) > 0 {
		// Direct CLI resume (`automergent -s <id>`) loads the session before
		// the TUI starts, so replay it here just as picker-based resume does.
		for _, message := range sess.Messages {
			app.replayMessage(message)
		}
	}
	// Initialize active token estimate
	app.updateActiveTokens()
	app.refreshChrome()
	// Set the welcome empty state. Width/height are 0 at construction time;
	// the state will be refreshed on the first WindowSizeMsg (resize event).
	app.conversation.SetEmptyState(components.WelcomeView)
	return app
}

func (a *App) Init() tea.Cmd {
	cmds := []tea.Cmd{
		// Clear screen on startup to ensure clean slate (alt screen will be enabled by View)
		func() tea.Msg { return tea.ClearScreen() },
		a.input.Focus(),
		a.spin.Tick(),
		a.fileTree.Load("."),
		// Detect the terminal's background color so the theme can adapt.
		func() tea.Msg { return tea.RequestBackgroundColor() },
		// Locate the user from their IP so the footer can show local time.
		func() tea.Msg { return resolveUserClock() },
		// Live tick for elapsed time displays (dock, taskboard, etc.)
		scheduleLiveTick(),
	}

	if a.showSessionPicker && a.storage != nil {
		cmds = append(cmds, func() tea.Msg {
			sessions, err := a.storage.List()
			if err != nil {
				return nil
			}
			return sessionsLoadedMsg{sessions: a.projectSessions(sessions)}
		})
	}
	if a.pendingProjectPath != "" {
		a.projectApprovalCh = make(chan agent.ConfirmationResponse, 1)
		a.confirm.ShowTrust("Trust this project folder?\n" + a.pendingProjectPath + "\nAutomergent can read this folder; edits and commands still require permission.")
		a.statusBar.SetPermission("project writes")
		a.confirm.SetReply(bridgeConfirmation(a.projectApprovalCh))
		cmds = append(cmds, func() tea.Msg {
			return projectApprovalMsg{response: <-a.projectApprovalCh}
		})
	} else if a.initialPrompt != "" {
		cmds = append(cmds, a.startAgent(a.initialPrompt))
	}

	return tea.Batch(cmds...)
}

// streamTickInterval is the coalescing cadence for streamed output. 80ms was
// 12.5 fps, slow enough that text arrived in visible chunks rather than
// flowing; 33ms is ~30 fps, which reads as continuous. The tick is
// self-terminating (it only reschedules while NeedsRender), and the streamer
// now renders one block per tick instead of the whole prefix, so the higher
// rate costs less work than the old one did.
const streamTickInterval = 33 * time.Millisecond

func scheduleStreamTick() tea.Cmd {
	return tea.Tick(streamTickInterval, func(time.Time) tea.Msg {
		return streamTickMsg{}
	})
}

const liveTickInterval = time.Second

func scheduleLiveTick() tea.Cmd {
	return tea.Tick(liveTickInterval, func(time.Time) tea.Msg {
		return liveTickMsg{}
	})
}

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)
	switch m := msg.(type) {
	case tea.MouseMsg:
		if a.palette.Visible() {
			palette, cmd := a.palette.Update(msg)
			a.palette = palette
			return a, cmd
		}
		if a.browsing {
			conversation, cmd := a.conversation.Update(msg)
			a.conversation = conversation
			return a, cmd
		}
		return a, nil
	case askSessionMsg:
		a.pendingAsk = m.pa
		a.questionnaire.Begin(m.pa.req, func(formatted string) {
			select {
			case m.pa.reply <- formatted:
			default:
			}
			close(m.pa.done)
		}, func() { close(m.pa.done) })
		a.statusBar.SetStatus("Awaiting your answers")
		return a, nil
	case components.ToastsTickMsg:
		if a.toasts != nil {
			updated, cmd := a.toasts.Update(msg)
			a.toasts = updated
			return a, cmd
		}
		return a, nil
	case tea.WindowSizeMsg:
		a.width, a.height = m.Width, m.Height
		a.layout()
		if a.toasts != nil {
			a.toasts.SetSize(m.Width)
		}
		if a.questionnaire != nil {
			a.questionnaire.SetSize(m.Width, m.Height)
		}
		a.taskBoard.SetSize(34, m.Height)
		a.dock.SetWidth(m.Width)
		if a.inspector != nil {
			a.inspector.SetSize(m.Width, m.Height)
		}
		if a.queueStrip != nil {
			a.queueStrip.SetWidth(m.Width)
		}
		// Refresh the welcome screen so it is correctly centered for the new size.
		a.conversation.SetEmptyState(components.WelcomeView)
		return a, nil
	case tea.KeyMsg:
		// The inspector is a fullscreen modal: it outranks every other key
		// consumer except a confirmation prompt (which can appear on top of it,
		// e.g. a permission request arriving while the user reads output).
		if a.inspector != nil && a.inspector.Visible() && !a.confirm.Visible() {
			if cmd, handled := a.handleInspectorKeys(m); handled {
				return a, cmd
			}
		}
		// Structured page viewer (full-page command output with actions)
		// captures keys while visible, ahead of the plain-text full page.
		if a.pageViewer.Visible() && !a.confirm.Visible() {
			// ESC resolves through the shared chain so the overlay closes the
			// same way every other full-screen surface does.
			if m.String() == "esc" {
				if cmd := a.handleEscape(); cmd != nil {
					cmds = append(cmds, cmd)
				}
				a.refreshChrome()
				return a, tea.Batch(cmds...)
			}
			var cmd tea.Cmd
			var handled bool
			a.pageViewer, handled, cmd = a.pageViewer.Update(m)
			if handled {
				cmds = append(cmds, cmd)
				return a, tea.Batch(cmds...)
			}
			return a, tea.Batch(append(cmds, cmd)...)
		}
		// Full-page overlay captures keys while visible.
		if a.fullPage.Visible() && !a.confirm.Visible() {
			if m.String() == "esc" {
				if cmd := a.handleEscape(); cmd != nil {
					cmds = append(cmds, cmd)
				}
				a.refreshChrome()
				return a, tea.Batch(cmds...)
			}
			var cmd tea.Cmd
			a.fullPage, cmd = a.fullPage.Update(m)
			return a, tea.Batch(append(cmds, cmd)...)
		}
		// Provider Studio captures keys while visible.
		if a.providerStudio.Visible() && !a.confirm.Visible() {
			var cmd tea.Cmd
			a.providerStudio, cmd = a.providerStudio.Update(m)
			return a, cmd
		}
		// Model Hub captures keys while visible.
		if a.modelHub.Visible() && !a.confirm.Visible() {
			var cmd tea.Cmd
			a.modelHub, cmd = a.modelHub.Update(m)
			return a, cmd
		}
		// The session browser owns the keyboard while visible: typing filters
		// its list, so printable keys must not leak into the prompt input.
		// (A confirmation prompt still outranks it.)
		if a.sessionBrowser.Visible() && !a.confirm.Visible() {
			sb, cmd := a.sessionBrowser.Update(m)
			a.sessionBrowser = sb
			if !a.sessionBrowser.Visible() {
				a.layout()
			}
			return a, cmd
		}
		// The artifact review browser owns the keyboard the same way.
		if a.artifactBrowser.Visible() && !a.confirm.Visible() {
			ab, cmd := a.artifactBrowser.Update(m)
			a.artifactBrowser = ab
			if !a.artifactBrowser.Visible() {
				// Closing the browser without deciding ends a pending plan
				// review rather than leaving the tool blocked.
				a.cancelPlanReview()
				a.layout()
			}
			return a, cmd
		}
		// Queue-strip actions (drop, pull back, highlight move) work while the
		// input is focused, so they are claimed before the textarea sees them.
		if a.focus == "input" && !a.browsing {
			if cmd, handled := a.handleQueueStripKeys(m); handled {
				return a, cmd
			}
		}
		// Bottom dock owns keys while focused.
		if a.dockFocusActive() {
			if cmd, handled := a.handleDockKeys(m); handled {
				return a, cmd
			}
			a.unfocusDock()
			return a, nil
		}
		// ↓ from the input drops into the background dock.
		if m.String() == "down" && !a.confirm.Visible() && !a.browsing && a.focus == "input" && a.input.Value() == "" {
			a.refreshDock()
			if a.focusDock() {
				return a, nil
			}
		}
		// TaskBoard navigation while visible and focus is not on the prompt.
		if a.taskBoard.Visible() && !a.confirm.Visible() && !a.browsing && a.focus != "input" {
			switch m.String() {
			case "j", "down":
				a.taskBoard.MoveFocus(1)
				return a, nil
			case "k", "up":
				a.taskBoard.MoveFocus(-1)
				return a, nil
			}
		}
		// Structured ask_user questionnaire captures keys while active.
		if a.questionnaire != nil && a.questionnaire.Visible() {
			a.questionnaire.Update(msg)
			return a, nil
		}
		// When diff is visible (fullscreen), route events to diff first
		if a.diffPane.Visible() && !a.confirm.Visible() && !a.questionnaire.Visible() {
			// Edit-review grammar takes priority while a proposal is shown.
			if a.reviewingProposal != "" && a.handleEditReviewKeys(m) {
				return a, tea.Batch(cmds...)
			}
			diff, cmd := a.diffPane.Update(msg)
			a.diffPane = diff
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
			// If diff closed itself, continue normal flow
			if !a.diffPane.Visible() {
				a.reviewingProposal = ""
				return a, tea.Batch(cmds...)
			}
			return a, tea.Batch(cmds...)
		}
		// When confirmation modal is visible, route key events only to the modal.
		if !a.confirm.Visible() && !a.questionnaire.Visible() {
			cmd = a.handleKey(m)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
	case tea.PasteMsg:
		if a.focus == "input" {
			inp, cmd := a.input.Update(msg)
			a.input = inp
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
	case components.PageActionMsg:
		// A full-page command's action shortcut was pressed: run the command
		// it names through the normal dispatch path, so pages can invoke
		// other commands (and their pages) the same way typed input does.
		if a.pageViewer.Visible() {
			input := "/" + m.Command
			if len(m.Args) > 0 {
				input += " " + strings.Join(m.Args, " ")
			}
			cmd := a.handleSlashCommand(input)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
			a.layout()
		}
		return a, tea.Batch(cmds...)
	case agentEventMsg:
		cmd = a.handleAgentEvent(m.ev)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	case shellNotificationMsg:
		// Inline "!command" runs resolve through their own passthroughDoneMsg;
		// the status hook toast is only for commands the user backgrounded.
		if a.passthroughCards[m.id] {
			break
		}
		cmd = a.handleShellNotification(m)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	case passthroughDoneMsg:
		cmd = a.handlePassthroughDone(m)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	case passthroughBackgroundedMsg:
		cmd = a.handlePassthroughBackgrounded(m)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	case agentNotificationMsg:
		cmd = a.handleAgentNotification(m)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	case spinner.TickMsg:
		sp, cmd := a.spin.Update(msg)
		a.spin = sp
		cmds = append(cmds, cmd)
	case streamTickMsg:
		a.streamTickPending = false
		a.conversation.RenderIfDirty()
		if a.thinking && !a.runStart.IsZero() {
			elapsed := time.Since(a.runStart)
			// The label carries the live verb (reading/editing/…) set by
			// tool events; the meta parenthetical carries the Claude
			// Code-style "(12s • ↓ 1.2k tokens)" clock-and-counter readout.
			label := a.spinLabel
			if label == "" {
				label = "generating"
			}
			a.spin.SetLabel(label)
			if toks := estimateTokens(a.runChars); toks > 0 {
				a.spin.SetMeta(fmt.Sprintf("%s • ↓ %s tokens", formatDuration(elapsed), compactTokenCount(toks)))
			} else {
				a.spin.SetMeta(formatDuration(elapsed))
			}
		}
		if a.conversation.NeedsRender() {
			a.streamTickPending = true
			cmds = append(cmds, scheduleStreamTick())
		}
	case liveTickMsg:
		// Re-render only when there are live time displays (background tasks).
		if a.dock != nil {
			a.refreshDock()
			if a.dock.HasContent() {
				a.layout()
			}
		}
		// Subagent live rows in the conversation refresh on the same tick.
		a.syncAgentRows()
		if a.inspector != nil && a.inspector.Visible() {
			// The inspector samples its source on every render; the tick is what
			// makes live output scroll while nothing else is happening.
			a.layout()
		}
		// Keep retry countdown and elapsed info line ticking live.
		if a.retrying || a.thinking {
			a.refreshChrome()
		}
		// The footer clock ticks on minute boundaries only: repaint when the
		// displayed time actually changed.
		if stamp := a.statusBar.ClockMinute(); stamp != "" && stamp != a.clockMinute {
			a.clockMinute = stamp
			a.refreshChrome()
		}
		cmds = append(cmds, scheduleLiveTick())
	case userClockMsg:
		a.statusBar.SetUserClock(m.tz, m.country)
		a.clockMinute = a.statusBar.ClockMinute()
		a.refreshChrome()
	case components.FileTreeLoadedMsg:
		a.fileTree.SetItems(m.Items)
	case components.SessionSelectedMsg:
		if m.Session != nil {
			// The browser's rows come from the lite session listing (no
			// messages); restore by id so the full transcript loads.
			if err := a.resumeSession(m.Session.ID); err != nil {
				a.statusBar.SetStatus("Session resume failed: " + err.Error())
			}
		}
	case components.SessionDeletedMsg:
		if m.Session != nil {
			if err := a.deleteSession(m.Session.ID); err != nil {
				a.statusBar.SetStatus("Delete failed: " + err.Error())
			}
		}
	case components.ArtifactDecisionMsg:
		// applyArtifactDecision resolves any waiting plan review itself.
		a.applyArtifactDecision(m.Path, m.Approved, m.Reason)
	case components.ArtifactCommentMsg:
		a.applyArtifactComment(m.Path, m.Text)
	case planReviewRequestedMsg:
		a.beginPlanReview(m.pr)
		a.refreshChrome()
	case components.ArtifactApproveAllMsg:
		a.applyArtifactApproveAll()
	case components.ArtifactOpenMsg:
		if cmd := a.openArtifactEditor(m.Path); cmd != nil {
			cmds = append(cmds, cmd)
		}
	case artifactEditorDoneMsg:
		a.handleArtifactEditorDone(m)
	case sessionTitledMsg:
		a.applySessionTitle(m)
	case components.SessionRenamedMsg:
		if m.Session != nil && strings.TrimSpace(m.Title) != "" {
			if err := a.renameStoredSession(m.Session, m.Title); err != nil {
				a.statusBar.SetStatus("Rename failed: " + err.Error())
			}
		}
	case components.SelectorSelectedMsg:
		if a.selectorAction != nil {
			a.selectorAction(m.Index)
			a.layout()
		}
	case modelsFetchedMsg:
		a.availableModels = m
		a.fetchingModels = false
		a.updatePalette()
	case providerTestMsg:
		if m.success {
			a.conversation.AddMessage("system", m.message, false)
			a.statusBar.SetStatus("Provider test passed")
		} else {
			a.conversation.AddMessage("assistant", m.message, true)
			a.statusBar.SetStatus("Provider test failed")
		}
	case sessionsLoadedMsg:
		a.sessionBrowser.SetSessions(m.sessions)
		a.sessionBrowser.SetCurrent(a.sess.ID)
		a.sessionBrowser.Show()
		a.layout()
		return a, nil
	case projectApprovalMsg:
		projectPath := a.pendingProjectPath
		a.pendingProjectPath = ""
		a.projectApprovalCh = nil
		if m.response.Allow {
			a.cfg.Security.AllowedWritePaths = appendUniquePath(a.cfg.Security.AllowedWritePaths, projectPath)
			if m.response.Always {
				if err := a.cfg.SaveIfLoaded(); err != nil {
					a.statusBar.SetStatus("Folder trusted (config save failed)")
				} else {
					a.statusBar.SetStatus("Folder trusted and remembered")
				}
			} else {
				a.statusBar.SetStatus("Folder trusted for this session")
			}
		} else {
			// Rejecting the trust warning quits instead of continuing
			// read-only: an untrusted folder must not be worked on at all.
			a.statusBar.ClearPermission()
			a.layout()
			return a, tea.Quit
		}
		a.statusBar.ClearPermission()
		a.layout()
		if a.initialPrompt != "" {
			return a, a.startAgent(a.initialPrompt)
		}
		return a, a.input.Focus()
	case clearCtrlCStatusMsg:
		// The arm window expired: a later Ctrl+C starts over rather than
		// quitting on a press the user made minutes ago.
		a.ctrlCArmed = false
		if !a.thinking && a.lastOutcome == outcomeNone {
			a.statusBar.SetStatus("Ready")
		}
		a.refreshChrome()
	case clearEscArmMsg:
		// Same expiry discipline for the double-ESC clear: once the window
		// lapses the "press esc again" hint must stop advertising an action
		// that would no longer happen.
		if a.escArmed {
			a.disarmEscape()
			a.refreshChrome()
		}
	case hideDiffPaneMsg:
		// Safely hide diff pane from main event loop (not from goroutine)
		if a.diffPane.Visible() {
			a.diffPane.Toggle()
			a.layout()
		}
	case forkedAgentDoneMsg:
		a.handleForkedAgentDone(m)
	case workflowProgressMsg:
		a.conversation.AddMessage("system", m.text, false)
		a.statusBar.SetStatus("Workflow")
	case dreamDoneMsg:
		a.conversation.AddMessage("system", m.text, false)
		a.statusBar.SetStatus("Memory consolidated")
	case mcpEventMsg:
		a.handleMCPEvent(m.ev)
	case mcpConfigChangeMsg:
		a.handleMCPConfigChange(m.ev)
	case tea.BackgroundColorMsg:
		a.handleBackgroundColor(m.Color)
	}
	if a.sessionBrowser.Visible() {
		if a.swallowNextKey {
			a.swallowNextKey = false
		} else {
			sb, cmd := a.sessionBrowser.Update(msg)
			a.sessionBrowser = sb
			cmds = append(cmds, cmd)
			if !a.sessionBrowser.Visible() {
				a.layout()
			}
		}
	}
	if a.selector.Visible() {
		if km, ok := msg.(tea.KeyMsg); ok && km.String() == "enter" {
			if reason := a.selector.SelectedDisabledReason(); reason != "" {
				a.statusBar.SetStatus(reason)
			}
		}
		sel, cmd := a.selector.Update(msg)
		a.selector = sel
		cmds = append(cmds, cmd)
		if !a.selector.Visible() {
			a.layout()
		}
		return a, tea.Batch(cmds...)
	}
	if a.confirm.Visible() {
		c, cmd := a.confirm.Update(msg)
		a.confirm = c
		cmds = append(cmds, cmd)
		if !a.confirm.Visible() {
			a.statusBar.ClearPermission()
			a.permissionTool = ""
			// Check if we need to hide diff pane after confirmation
			if a.pendingDiffHide && a.diffPane.Visible() {
				a.diffPane.Toggle()
				a.pendingDiffHide = false
			}
			a.layout()
			a.refreshChrome()
		}
	}
	return a, tea.Batch(cmds...)
}
