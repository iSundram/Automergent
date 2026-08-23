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
	"github.com/iSundram/Automergent/internal/session"
	"github.com/iSundram/Automergent/internal/tui/commands"
	"github.com/iSundram/Automergent/internal/tui/components"
	"github.com/iSundram/Automergent/internal/tui/keys"
	"github.com/iSundram/Automergent/internal/tui/render"
	"github.com/iSundram/Automergent/internal/tui/themes"
	"os"
	"sort"
	"strings"
	"time"
)

type agentEventMsg struct{ ev agent.Event }
type modelsFetchedMsg []ai.Model
type clearCtrlCStatusMsg struct{}
type hideDiffPaneMsg struct{} // Message to safely hide diff pane from main loop
type streamTickMsg struct{}   // Coalesced streaming render tick (~80ms)
type sessionsLoadedMsg struct {
	sessions []*session.Session
}
type projectApprovalMsg struct{ response agent.ConfirmationResponse }
type coordinatorEventMsg struct {
	phase   string
	running bool
}

type App struct {
	cfg               *config.Config
	lastBranchCheck   time.Time
	toasts            *components.Toasts
	reviewingProposal string
	questionnaire     *components.Questionnaire
	taskBoard         *components.TaskBoard
	zenMode           bool
	sendToProgram     func(tea.Msg)
	pendingAsk        *pendingAsk
	ag                *agent.Agent
	sess              *session.Session
	storage           *session.Storage
	persist           *session.PersistenceManager
	keys              *keys.Bindings
	styles            *themes.Styles
	theme             *themes.Theme
	conversation      components.Conversation
	diffPane          components.Diff
	input             components.Input
	header            components.Header
	logo              components.Logo
	statusBar         components.StatusBar
	spin              components.Spinner
	confirm           components.Confirm
	coAuthorConfirm   components.CoAuthorConfirm
	sessionBrowser    components.SessionBrowser
	selector          components.SelectorOverlay
	selectorAction    func(index int)
	lspPanel          components.LSPPanel
	stats             components.Stats
	helpOverlay       components.HelpOverlay
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

	// Custom command hot-reload state (see refreshCustomCommands).
	customCmdCount int
	customWarnKey  string

	// Conversation rewind points (captured per agent turn) and extra
	// read-only search roots added via /add-dir.
	checkpoints     []conversationCheckpoint
	extraSearchDirs []string

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

	// pendingDiffHide is set when confirmation completes and diff should be hidden
	pendingDiffHide bool

	// pendingCommitToolCall stores the tool call when waiting for co-author confirmation
	pendingCommitToolCall *ai.ToolCall
	pendingCommitReplyCh  chan agent.ConfirmationResponse
	pendingProjectPath    string
	projectApprovalCh     chan agent.ConfirmationResponse

	// Streaming render coalescing + live telemetry.
	streamTickPending bool
	runTokens         int       // token events observed in the current run
	runStart          time.Time // when the current run started
	tokRate           int       // smoothed tokens/sec shown while thinking
}

func NewApp(cfg *config.Config, ag *agent.Agent, sess *session.Session, storage *session.Storage, persist *session.PersistenceManager, initialPrompt string, showSessionPicker bool) *App {
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
		conversation:       components.NewConversation(styles),
		diffPane:           components.NewDiff(styles),
		input:              components.NewInput(styles),
		header:             components.NewHeader(styles),
		logo:               components.NewLogo(styles),
		statusBar:          components.NewStatusBar(styles),
		toasts:             components.NewToasts(styles),
		questionnaire:      components.NewQuestionnaire(styles),
		taskBoard:          components.NewTaskBoard(styles),
		spin:               components.NewSpinner(styles),
		confirm:            components.NewConfirm(styles),
		coAuthorConfirm:    components.NewCoAuthorConfirm(styles, cfg),
		sessionBrowser:     components.NewSessionBrowser(styles),
		selector:           components.NewSelectorOverlay(styles),
		lspPanel:           components.NewLSPPanel(styles),
		stats:              components.NewStats(styles),
		helpOverlay:        components.NewHelpOverlay(styles),
		fileTree:           components.NewFileTree(styles),
		palette:            components.NewCommandPalette(styles),
		ctx:                ctx,
		cancel:             cancel,
		initialPrompt:      initialPrompt,
		showSessionPicker:  showSessionPicker,
		statusMsg:          "Ready",
		focus:              "input",
		availableProviders: []string{"google"},
		commands:           commands.Default(),
	}
	sort.Strings(app.availableProviders)
	if wd, err := os.Getwd(); err == nil {
		app.workDir = wd
	}
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
	app.helpOverlay.SetSlashCommands(app.commands.HelpRows())
	app.header.SetModel(cfg.Model)
	app.header.SetProvider(cfg.Provider)
	app.header.SetMode(cfg.Mode)
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

	// Start coordinator event listener if available
	cmds = append(cmds, a.startCoordinatorListener())

	return tea.Batch(cmds...)
}

func scheduleStreamTick() tea.Cmd {
	return tea.Tick(80*time.Millisecond, func(time.Time) tea.Msg {
		return streamTickMsg{}
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
		return a, nil
	case tea.KeyMsg:
		// TaskBoard navigation/actions when visible and not typing.
		if a.taskBoard.Visible() && !a.confirm.Visible() && !a.browsing && a.focus != "input" {
			if a.handleTaskBoardKeys(m) {
				return a, nil
			}
		}
		// Structured ask_user questionnaire captures keys while active.
		if a.questionnaire != nil && a.questionnaire.Visible() {
			a.questionnaire.Update(msg)
			return a, nil
		}
		// When diff is visible (fullscreen), route events to diff first
		if a.diffPane.Visible() && !a.confirm.Visible() && !a.coAuthorConfirm.Visible() {
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
		if !a.confirm.Visible() && !a.coAuthorConfirm.Visible() {
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
	case agentEventMsg:
		cmd = a.handleAgentEvent(m.ev)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	case coordinatorEventMsg:
		a.header.SetPhase(m.phase)
		if m.running {
			a.statusBar.SetStatus(fmt.Sprintf("Coordinator: %s phase", m.phase))
		}
		cmds = append(cmds, a.waitForCoordinatorEvent())
	case spinner.TickMsg:
		sp, cmd := a.spin.Update(msg)
		a.spin = sp
		cmds = append(cmds, cmd)
	case streamTickMsg:
		a.streamTickPending = false
		a.conversation.RenderIfDirty()
		if a.thinking && !a.runStart.IsZero() {
			if elapsed := time.Since(a.runStart).Seconds(); elapsed > 0.5 && a.runTokens > 0 {
				rate := int(float64(a.runTokens) / elapsed)
				// Smooth so the number doesn't flicker.
				a.tokRate = (a.tokRate + rate) / 2
				a.spin.SetLabel(fmt.Sprintf("thinking · %d tok/s", a.tokRate))
			}
		}
		if a.conversation.NeedsRender() {
			a.streamTickPending = true
			cmds = append(cmds, scheduleStreamTick())
		}
	case components.FileTreeLoadedMsg:
		a.fileTree.SetItems(m.Items)
	case components.SessionSelectedMsg:
		if m.Session != nil {
			if err := a.restoreSession(m.Session); err != nil {
				a.statusBar.SetStatus("Session loaded (provider switch failed: " + err.Error() + ")")
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
		// Only clear if still showing the Ctrl+C message
		if a.statusBar.View() != "" && !a.thinking {
			a.statusBar.SetStatus("Ready")
		}
	case hideDiffPaneMsg:
		// Safely hide diff pane from main event loop (not from goroutine)
		if a.diffPane.Visible() {
			a.diffPane.Toggle()
			a.layout()
		}
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
			// Check if we need to hide diff pane after confirmation
			if a.pendingDiffHide && a.diffPane.Visible() {
				a.diffPane.Toggle()
				a.pendingDiffHide = false
			}
			a.layout()
		}
	}
	if a.coAuthorConfirm.Visible() {
		c, cmd := a.coAuthorConfirm.Update(msg)
		a.coAuthorConfirm = c
		cmds = append(cmds, cmd)
		if !a.coAuthorConfirm.Visible() {
			a.layout()
		}
	}
	return a, tea.Batch(cmds...)
}
