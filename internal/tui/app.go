package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/sahilm/fuzzy"
	"github.com/sergi/go-diff/diffmatchpatch"

	"github.com/iSundram/Automergent/internal/agent"
	"github.com/iSundram/Automergent/internal/ai"
	googleProvider "github.com/iSundram/Automergent/internal/ai/google"
	"github.com/iSundram/Automergent/internal/cache"
	"github.com/iSundram/Automergent/internal/config"
	"github.com/iSundram/Automergent/internal/session"
	"github.com/iSundram/Automergent/internal/tools"
	"github.com/iSundram/Automergent/internal/tui/components"
	"github.com/iSundram/Automergent/internal/tui/keys"
	"github.com/iSundram/Automergent/internal/tui/themes"
)

type agentEventMsg struct{ ev agent.Event }
type modelsFetchedMsg []ai.Model
type clearCtrlCStatusMsg struct{}
type hideDiffPaneMsg struct{} // Message to safely hide diff pane from main loop
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
	statusBar         components.StatusBar
	spin              components.Spinner
	confirm           components.Confirm
	coAuthorConfirm   components.CoAuthorConfirm
	sessionBrowser    components.SessionBrowser
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
	statusMsg         string

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
}

func NewApp(cfg *config.Config, ag *agent.Agent, sess *session.Session, storage *session.Storage, persist *session.PersistenceManager, initialPrompt string, showSessionPicker bool) *App {
	theme := themes.Get(cfg.Theme)
	styles := themes.NewStyles(theme)
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
		statusBar:          components.NewStatusBar(styles),
		spin:               components.NewSpinner(styles),
		confirm:            components.NewConfirm(styles),
		coAuthorConfirm:    components.NewCoAuthorConfirm(styles, cfg),
		sessionBrowser:     components.NewSessionBrowser(styles),
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
	}
	sort.Strings(app.availableProviders)
	app.header.SetModel(cfg.Model)
	app.header.SetProvider(cfg.Provider)
	app.header.SetMode(cfg.Mode)
	app.header.SetPhase(string(agent.DetectPhase(sess.Messages)))
	if wd, err := os.Getwd(); err == nil {
		app.workDir = wd
	}
	if len(sess.Messages) == 0 && initialPrompt == "" {
		app.conversation.SetWelcomeState()
	}
	// Initialize active token estimate
	app.updateActiveTokens()
	return app
}

func (a *App) requireProjectApproval(projectPath string) {
	a.pendingProjectPath = projectPath
}

func (a *App) Init() tea.Cmd {
	cmds := []tea.Cmd{
		// Clear screen on startup to ensure clean slate (alt screen will be enabled by View)
		func() tea.Msg { return tea.ClearScreen() },
		a.input.Focus(),
		a.spin.Tick(),
		a.fileTree.Load("."),
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
		a.confirm.SetReply(a.projectApprovalCh)
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

func (a *App) startCoordinatorListener() tea.Cmd {
	coord := a.ag.Coordinator()
	if coord == nil {
		return nil
	}

	return a.waitForCoordinatorEvent()
}

func (a *App) waitForCoordinatorEvent() tea.Cmd {
	coord := a.ag.Coordinator()
	if coord == nil {
		return nil
	}
	return func() tea.Msg {
		event := <-coord.Events()
		switch event.Type {
		case "phase_start":
			phaseName := "research"
			if p, ok := event.Payload.(string); ok {
				phaseName = strings.ToLower(p)
			}
			return coordinatorEventMsg{phase: phaseName, running: true}
		case "phase_complete":
			phaseName := "execute"
			if p, ok := event.Payload.(string); ok {
				phaseName = strings.ToLower(p)
			}
			return coordinatorEventMsg{phase: phaseName, running: false}
		}
		return nil
	}
}

func (a *App) startAgent(prompt string) tea.Cmd {
	prompt = a.expandPrompt(prompt)
	if strings.HasPrefix(prompt, "!") {
		return a.runShellPassthrough(prompt[1:])
	}
	a.thinking = true
	a.streamedReply = false
	a.spin.Start()
	a.conversation.AddMessage("user", prompt, false)
	a.updateActiveTokens()
	a.statusBar.SetStatus("Thinking…")
	a.layout() // Adjust for thinking spinner
	go func() { _ = a.ag.Run(a.ctx, prompt) }()
	return a.waitForAgentEvent()
}

func (a *App) expandPrompt(prompt string) string {
	words := strings.Fields(prompt)
	for i, word := range words {
		if strings.HasPrefix(word, "@") {
			path := word[1:]
			content, err := os.ReadFile(path)
			if err == nil {
				words[i] = fmt.Sprintf("\n--- %s ---\n%s\n", path, string(content))
			}
		}
	}
	return strings.Join(words, " ")
}

func (a *App) runShellPassthrough(command string) tea.Cmd {
	a.conversation.AddMessage("user", "!"+command, false)
	return func() tea.Msg {
		cmd := exec.Command("bash", "-c", command)
		output, _ := cmd.CombinedOutput()
		content := string(output)
		if content == "" {
			content = "(no output)"
		}
		return agentEventMsg{ev: agent.Event{Type: agent.EventDone, Payload: content}}
	}
}

func (a *App) waitForAgentEvent() tea.Cmd {
	return func() tea.Msg {
		ev := <-a.ag.Events()
		return agentEventMsg{ev: ev}
	}
}

func (a *App) fetchModels() tea.Cmd {
	if a.fetchingModels {
		return nil
	}
	a.fetchingModels = true
	return func() tea.Msg {
		models, _ := a.ag.Provider().Models(a.ctx)
		return modelsFetchedMsg(models)
	}
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
	case tea.WindowSizeMsg:
		a.width, a.height = m.Width, m.Height
		a.layout()
		return a, nil
	case tea.KeyMsg:
		// When diff is visible (fullscreen), route events to diff first
		if a.diffPane.Visible() && !a.confirm.Visible() && !a.coAuthorConfirm.Visible() {
			diff, cmd := a.diffPane.Update(msg)
			a.diffPane = diff
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
			// If diff closed itself, continue normal flow
			if !a.diffPane.Visible() {
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
	case components.FileTreeLoadedMsg:
		a.fileTree.SetItems(m.Items)
	case components.SessionSelectedMsg:
		if m.Session != nil {
			a.sess = m.Session
			if a.sess.WorkDir == "" {
				a.sess.WorkDir = a.workDir
			}
			a.ag.SetSession(m.Session)
			if a.persist != nil {
				a.persist.SetSession(m.Session)
			}
			a.conversation.Clear()
			for _, sm := range m.Session.Messages {
				a.conversation.AddMessage(string(sm.Role), sm.TextContent(), false)
			}
			a.stats.InputTokens = m.Session.TotalInputTokens
			a.stats.OutputTokens = m.Session.TotalOutputTokens
			a.header.SetTokens(m.Session.TotalInputTokens + m.Session.TotalOutputTokens)
			a.updateActiveTokens()
			if calc := a.ag.AdaptiveCalculator(); calc != nil {
				a.header.SetAdaptiveWeight(calc.Weight())
			}
			// Restore provider/model from session
			if m.Session.Provider != "" {
				model := m.Session.Model
				if err := a.switchProvider(m.Session.Provider, model); err != nil {
					a.statusBar.SetStatus(fmt.Sprintf("Session loaded (provider switch failed: %v)", err))
				} else {
					a.statusBar.SetStatus("Session loaded")
				}
			} else {
				a.statusBar.SetStatus("Session loaded")
			}
			a.layout()
		}
	case modelsFetchedMsg:
		a.availableModels = m
		a.fetchingModels = false
		a.updatePalette()
	case sessionsLoadedMsg:
		a.sessionBrowser.SetSessions(m.sessions)
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
			a.statusBar.SetStatus("Read-only project")
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
	}
	if a.sessionBrowser.Visible() {
		sb, cmd := a.sessionBrowser.Update(msg)
		a.sessionBrowser = sb
		cmds = append(cmds, cmd)
		if !a.sessionBrowser.Visible() {
			a.layout()
		}
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

func (a *App) handleKey(m tea.KeyMsg) tea.Cmd {
	if a.showHelp {
		if m.String() == "?" || m.String() == "esc" || m.String() == "q" {
			a.showHelp = false
		}
		return nil
	}
	key := m.String()
	if key == "esc" && a.thinking {
		a.cancelActiveRun("Interrupted")
		return nil
	}
	if a.palette.Visible() {
		switch key {
		case "enter":
			if sel := a.palette.Selected(); sel != nil {
				if sel.Disabled {
					a.statusBar.SetStatus(sel.DisabledReason)
					return nil
				}
				trigger := a.input.TriggerType()
				if trigger == "command" || trigger == "help" {
					definition, known := lookupSlashCommand(sel.Value)
					if known && definition.Immediate {
						a.palette.Hide()
						a.layout()
						return a.handleSlashCommand("/" + sel.Value)
					}
					// Commands that need a sub-palette (model, provider, etc.)
					needsSubPalette := map[string]bool{
						"model": true, "provider": true, "mode": true,
					}
					if needsSubPalette[sel.Value] {
						a.input.InsertValue(sel.Value)
						a.updatePalette()
						a.palette.Show(a.palette.Items(), a.input.TriggerValue())
						a.layout()
						if sel.Value == "model" && len(a.availableModels) == 0 {
							return a.fetchModels()
						}
						return nil
					}
				}
				a.input.InsertValue(sel.Value)
				a.palette.Hide()
				a.layout()
				return nil
			}
		case "up", "down", "ctrl+p", "ctrl+n", "tab", "shift+tab", "ctrl+tab", "pgup", "pgdown":
			pal, cmd := a.palette.Update(m)
			a.palette = pal
			return cmd
		case "esc":
			a.palette.Hide()
			a.layout()
			return nil
		}
	}
	switch key {
	case "ctrl+c":
		now := time.Now()
		if now.Sub(a.lastCtrlCAt) <= time.Second {
			a.cancel()
			return tea.Quit
		}
		a.lastCtrlCAt = now
		if a.thinking {
			a.cancelActiveRun("Interrupted")
		} else {
			a.statusBar.SetStatus("Press Ctrl+C again to exit")
			return tea.Tick(2*time.Second, func(time.Time) tea.Msg {
				return clearCtrlCStatusMsg{}
			})
		}
		return nil
	case "ctrl+q":
		a.cancel()
		return tea.Quit
	case "ctrl+d":
		a.diffPane.Toggle()
		a.layout()
		return nil
	case "ctrl+l":
		a.lspPanel.Toggle()
		a.layout()
		return nil
	case "ctrl+s":
		if a.storage != nil {
			sessions, err := a.storage.List()
			if err != nil {
				a.statusBar.SetStatus(fmt.Sprintf("Error listing sessions: %v", err))
				return nil
			}
			a.sessionBrowser.SetSessions(a.projectSessions(sessions))
		} else {
			a.sessionBrowser.SetSessions([]*session.Session{a.sess})
		}
		a.sessionBrowser.Show()
		return nil
	case "ctrl+r":
		a.conversation.SetReviewMode(!a.conversation.ReviewMode())
		if a.conversation.ReviewMode() {
			a.statusBar.SetStatus("Review mode enabled: full tool output")
		} else {
			a.statusBar.SetStatus("Review mode disabled: truncated tool output")
		}
		return nil
	case "ctrl+u":
		a.input.SetValue("")
		a.updateActiveTokens()
		return nil
	case "ctrl+t":
		a.showFileTree = !a.showFileTree
		a.layout()
		return nil
	case "f1":
		a.showHelp = true
		return nil
	case "f2":
		a.diffPane.Toggle()
		a.layout()
		return nil
	case "tab":
		if !a.palette.Visible() {
			switch a.focus {
			case "input":
				a.focus = "conversation"
			case "conversation":
				if a.diffPane.Visible() {
					a.focus = "diff"
				} else if a.showFileTree {
					a.focus = "tree"
				} else {
					a.focus = "input"
				}
			case "diff":
				if a.showFileTree {
					a.focus = "tree"
				} else {
					a.focus = "input"
				}
			case "tree":
				a.focus = "input"
			}
			a.browsing = a.focus == "conversation"
			a.conversation.SetBrowsing(a.browsing)
			a.statusBar.SetBrowsing(a.browsing)
			a.layout()
			if a.focus == "input" {
				return a.input.Focus()
			}
			a.input.Blur()
			a.diffPane.Focus(a.focus == "diff")
		}
		return nil
	}
	switch a.focus {
	case "input":
		if (key == "enter" || key == "ctrl+m") && !a.thinking {
			prompt := strings.TrimSpace(a.input.Value())
			if prompt != "" {
				// If we are waiting for an ask_user response, send it
				if a.askUserReplyCh != nil {
					a.askUserReplyCh <- prompt
					a.askUserReplyCh = nil
					a.input.Reset()
					a.statusBar.SetStatus("Thinking…")
					return nil
				}

				a.input.Reset()
				a.palette.Hide()
				a.layout()
				if strings.HasPrefix(prompt, "/") {
					return a.handleSlashCommand(prompt)
				}
				return a.startAgent(prompt)
			}
		}
		inp, cmd := a.input.Update(m)
		a.input = inp
		a.updateActiveTokens()
		trigger := a.input.TriggerType()
		if trigger != "" {
			a.updatePalette()
			a.palette.Show(a.palette.Items(), a.input.TriggerValue())
			a.layout()
			if trigger == "model" && len(a.availableModels) == 0 {
				return a.fetchModels()
			}
		} else if a.palette.Visible() {
			a.palette.Hide()
			a.layout()
		}
		return cmd
	case "conversation":
		conv, cmd := a.conversation.Update(m)
		a.conversation = conv
		return cmd
	case "diff":
		diff, cmd := a.diffPane.Update(m)
		a.diffPane = diff
		return cmd
	case "tree":
		tree, cmd := a.fileTree.Update(m)
		a.fileTree = tree
		return cmd
	}
	return nil
}

func (a *App) updatePalette() {
	trigger := a.input.TriggerType()
	filter := a.input.TriggerValue()
	a.palette.SetQuery(filter)

	var items []components.PaletteItem
	switch trigger {
	case "help", "command":
		allCmds := commandPaletteItems()
		for i := range allCmds {
			switch allCmds[i].Value {
			case "cancel":
				allCmds[i].Disabled = !a.thinking
				if allCmds[i].Disabled {
					allCmds[i].DisabledReason = "No active request"
				}
			case "review":
				allCmds[i].Current = a.conversation.ReviewMode()
			case "tree":
				allCmds[i].Current = a.showFileTree
			case "diff":
				allCmds[i].Current = a.diffPane.Visible()
			case "lsp":
				allCmds[i].Current = a.lspPanel.Visible()
			}
		}
		items = a.fuzzyFilter(allCmds, filter)

	case "model":
		var modelItems []components.PaletteItem
		for _, m := range a.availableModels {
			modelItems = append(modelItems, components.PaletteItem{
				Label:       m.ID,
				Description: fmt.Sprintf("Model (Limit: %d)", m.ContextLimit),
				Value:       m.ID,
				Icon:        "󰊕",
				Category:    "Models",
				Current:     m.ID == a.cfg.Model,
			})
		}
		if len(modelItems) == 0 && a.fetchingModels {
			items = []components.PaletteItem{{Label: "Loading…", Description: "Fetching models from provider", Value: "", Icon: "󰔟", Category: "Models"}}
		} else {
			items = a.fuzzyFilter(modelItems, filter)
		}
	case "provider":
		providerDescriptions := map[string]string{
			"google": "Gemini models by Google",
		}
		providerIcons := map[string]string{
			"google": "󰊭",
		}
		var providerItems []components.PaletteItem
		for _, p := range a.availableProviders {
			desc := providerDescriptions[p]
			if desc == "" {
				desc = "AI provider"
			}
			icon := providerIcons[p]
			if icon == "" {
				icon = "🔌"
			}
			providerItems = append(providerItems, components.PaletteItem{
				Label: p, Description: desc, Value: p, Icon: icon, Category: "Providers", Current: p == a.cfg.Provider,
			})
		}
		items = a.fuzzyFilter(providerItems, filter)

	case "mode":
		modeItems := []components.PaletteItem{
			{Label: "edit", Description: "Allow edits with permission", Value: "edit", Icon: "󰏫", Category: "Modes", Current: a.cfg.Mode == "edit"},
			{Label: "plan", Description: "Plan and inspect without edits", Value: "plan", Icon: "󰈙", Category: "Modes", Current: a.cfg.Mode == "plan"},
		}
		items = a.fuzzyFilter(modeItems, filter)

	case "file":
		var fileItems []components.PaletteItem
		for _, item := range a.fileTree.Items() {
			if !item.IsDir {
				fileItems = append(fileItems, components.PaletteItem{
					Label:       item.Name,
					Description: item.Path,
					Value:       item.Path,
					Icon:        "󰈔",
					Category:    "Files",
				})
			}
		}
		items = a.fuzzyFilter(fileItems, filter)
	}

	a.palette.SetItems(items)
}

func (a *App) fuzzyFilter(items []components.PaletteItem, filter string) []components.PaletteItem {
	if filter == "" {
		return items
	}
	var targets []string
	for _, item := range items {
		targets = append(targets, item.Label+" "+item.SearchTerms)
	}
	matches := fuzzy.Find(filter, targets)
	var filtered []components.PaletteItem
	for _, match := range matches {
		filtered = append(filtered, items[match.Index])
	}
	return filtered
}

func (a *App) handleAgentEvent(ev agent.Event) tea.Cmd {
	// Live-update active token estimate on every agent event.
	a.updateActiveTokens()
	switch ev.Type {
	case agent.EventToken:
		if tok, ok := ev.Payload.(string); ok {
			a.conversation.AppendToken(tok)
			if strings.TrimSpace(tok) != "" {
				a.streamedReply = true
			}
		}
		return a.waitForAgentEvent()
	case agent.EventThought:
		if thought, ok := ev.Payload.(string); ok {
			a.conversation.AppendThought(thought)
		}
		return a.waitForAgentEvent()
	case agent.EventToolCall:
		if te, ok := ev.Payload.(agent.ToolCallEvent); ok {
			argText := ""
			if len(te.Args) > 0 {
				if b, err := json.Marshal(te.Args); err == nil {
					argText = string(b)
				}
			}
			ctx := te.Context
			if ctx == "" {
				ctx = extractToolContext(te.Name, te.Args)
			}
			a.conversation.AddToolLifecycleStart(te.ID, te.Name, argText, ctx)
			a.stats.ToolCallCount++
			a.statusBar.SetStatus(fmt.Sprintf("⚙ %s…", te.Name))
		} else if tc, ok := ev.Payload.(ai.ToolCall); ok {
			argText := ""
			if len(tc.Args) > 0 {
				if b, err := json.Marshal(tc.Args); err == nil {
					argText = string(b)
				}
			}
			ctx := extractToolContext(tc.Name, tc.Args)
			a.conversation.AddToolLifecycleStart(tc.ID, tc.Name, argText, ctx)
			a.stats.ToolCallCount++
			a.statusBar.SetStatus(fmt.Sprintf("⚙ %s…", tc.Name))
		}
		return a.waitForAgentEvent()
	case agent.EventToolDone:
		if td, ok := ev.Payload.(agent.ToolDoneEvent); ok {
			a.conversation.AddToolLifecycleDone(td.ID, td.Name, td.Context, td.Result.Summary, td.Duration, td.Result, a.conversation.ReviewMode())
		} else if r, ok := ev.Payload.(tools.Result); ok {
			if r.IsError {
				a.conversation.AddMessage("assistant", "Tool error: "+r.Content, true)
			} else if strings.TrimSpace(r.Content) != "" {
				a.conversation.AddMessage("tool_result", truncateUIContent(r.Content, a.conversation.ReviewMode()), false)
			}
		}
		a.header.SetPhase(string(agent.DetectPhase(a.sess.Messages)))
		a.statusBar.SetStatus("Thinking…")
		return a.waitForAgentEvent()
	case agent.EventStatus:
		if s, ok := ev.Payload.(string); ok {
			// Ignore stale transient statuses that can arrive after completion.
			if !a.thinking && isTransientStatus(s) {
				return nil
			}
			a.statusBar.SetStatus(s)
		}
		return a.waitForAgentEvent()
	case agent.EventNotify:
		// Payload expected to be map[string]any{"level":..., "title":..., "message":...}
		if payload, ok := ev.Payload.(map[string]any); ok {
			lvl, _ := payload["level"].(string)
			title, _ := payload["title"].(string)
			msg, _ := payload["message"].(string)
			if title != "" {
				a.statusBar.SetStatus(fmt.Sprintf("%s: %s", title, msg))
			} else {
				a.statusBar.SetStatus(msg)
			}
			// Add to conversation for auditability
			if msg != "" {
				a.conversation.AddMessage("system", fmt.Sprintf("[%s] %s", lvl, msg), false)
			}
		}
		return a.waitForAgentEvent()
	case agent.EventThinking:
		if thinkingText, ok := ev.Payload.(string); ok {
			a.statusBar.SetStatus(thinkingText)
		}
		return a.waitForAgentEvent()
	case agent.EventDone:
		a.thinking = false
		a.spin.Stop()
		text, _ := ev.Payload.(string)
		if a.streamedReply {
			a.conversation.FinalizeStreamingWithContent(text)
		} else {
			a.conversation.FinalizeStreaming()
		}
		a.layout() // Reclaim space from spinner
		a.statusBar.SetStatus("Ready")
		a.stats.InputTokens = a.sess.TotalInputTokens
		a.stats.OutputTokens = a.sess.TotalOutputTokens
		if tel := a.ag.Telemetry(); tel != nil {
			a.stats.TotalCost = tel.GetCostSummary().TotalCostUSD
		}
		a.header.SetTokens(a.sess.TotalInputTokens + a.sess.TotalOutputTokens)
		if calc := a.ag.AdaptiveCalculator(); calc != nil {
			a.header.SetAdaptiveWeight(calc.Weight())
		}
		a.header.SetPhase(string(agent.DetectPhase(a.sess.Messages)))
		if strings.TrimSpace(text) != "" && !a.streamedReply {
			a.conversation.AddMessage("assistant", text, false)
		}
		return nil
	case agent.EventCompacted:
		a.statusBar.SetStatus("Context compacted")
		a.stats.InputTokens = a.sess.TotalInputTokens
		a.stats.OutputTokens = a.sess.TotalOutputTokens
		a.header.SetTokens(a.sess.TotalInputTokens + a.sess.TotalOutputTokens)
		if calc := a.ag.AdaptiveCalculator(); calc != nil {
			a.header.SetAdaptiveWeight(calc.Weight())
		}
		a.header.SetPhase(string(agent.DetectPhase(a.sess.Messages)))
		a.conversation.AddMessage("system", "Context compacted successfully", false)
		return nil
	case agent.EventError:
		a.thinking = false
		a.spin.Stop()
		a.conversation.FinalizeStreaming() // Re-render with markdown
		a.layout()                         // Reclaim space from spinner
		if err, ok := ev.Payload.(error); ok {
			errStr := err.Error()
			msg := formatErrorMessage(errStr)
			if isCancellationError(errStr) {
				a.conversation.AddMessage("system", msg, false)
				a.statusBar.SetStatus("Cancelled")
				return nil
			}
			a.conversation.AddMessage("assistant", msg, true)
			if strings.Contains(errStr, "401") || strings.Contains(errStr, "authentication_error") {
				a.conversation.AddMessage("system", "Tip: You can set the API key using: /api-key <key>", false)
			}
		}
		a.statusBar.SetStatus("Error")
		return nil
	case agent.EventConfirm:
		if payload, ok := ev.Payload.(map[string]any); ok {
			if tc, ok := payload["tool_call"].(ai.ToolCall); ok {
				// Special handling for git_commit with "ask" co-author mode
				if tc.Name == "git_commit" && a.cfg.Git.CoAuthorMode() == "ask" {
					// Check if co_author is not already explicitly set
					if _, hasCoAuthor := tc.Args["co_author"]; !hasCoAuthor {
						// Store the pending commit and show co-author dialog
						tcCopy := tc
						a.pendingCommitToolCall = &tcCopy
						if replyCh, ok := payload["reply"].(chan agent.ConfirmationResponse); ok {
							a.pendingCommitReplyCh = replyCh
						}
						coAuthorReplyCh := make(chan components.CoAuthorResponse, 1)
						a.coAuthorConfirm.SetReply(coAuthorReplyCh)
						a.coAuthorConfirm.Show()
						a.layout()

						// Handle the co-author response in a goroutine
						// Handle the co-author response in a goroutine
						go func() {
							select {
							case res, ok := <-coAuthorReplyCh:
								if !ok {
									if a.pendingCommitReplyCh != nil {
										select {
										case a.pendingCommitReplyCh <- agent.ConfirmationResponse{Allow: false}:
										default:
										}
									}
									a.pendingCommitToolCall = nil
									a.pendingCommitReplyCh = nil
									return
								}
								if res.Save != "" {
									a.cfg.Git.CoAuthor = res.Save
									_ = a.cfg.SaveIfLoaded()
								}
								if a.pendingCommitToolCall != nil {
									a.pendingCommitToolCall.Args["co_author"] = res.Include
								}
								if a.pendingCommitReplyCh != nil {
									a.pendingCommitReplyCh <- agent.ConfirmationResponse{Allow: true}
								}
								a.pendingCommitToolCall = nil
								a.pendingCommitReplyCh = nil
							case <-a.ctx.Done():
								if a.pendingCommitReplyCh != nil {
									select {
									case a.pendingCommitReplyCh <- agent.ConfirmationResponse{Allow: false}:
									default:
									}
								}
								a.pendingCommitToolCall = nil
								a.pendingCommitReplyCh = nil
							}
						}()
						return a.waitForAgentEvent()
					}
				}

				// Use pretty name if possible
				name := tc.Name
				switch tc.Name {
				case "read_file", "view":
					name = "Readfile"
				case "write_file", "create_file":
					name = "Write"
				case "edit_file":
					name = "Edit"
				case "delete_file":
					name = "Delete"
				case "move_file":
					name = "Move"
				case "copy_file":
					name = "Copy"
				case "list_directory":
					name = "List directory"
				case "run_shell_command", "run_command", "bash":
					name = "Run"
				case "git_commit":
					name = "Git commit"
				}

				// Special handling for file edits: show diff
				// Special handling for file edits: show diff with inline confirmation
				if tc.Name == "write_file" || tc.Name == "edit_file" || tc.Name == "create_file" {
					path, _ := tc.Args["path"].(string)
					newContent := ""
					if tc.Name == "write_file" || tc.Name == "create_file" {
						newContent, _ = tc.Args["content"].(string)
					} else {
						// Patch: read file and apply patch
						oldStr, _ := tc.Args["old_str"].(string)
						replaceWith, _ := tc.Args["new_str"].(string)
						replaceAll, _ := tc.Args["replace_all"].(bool)
						data, _ := os.ReadFile(path)
						original := string(data)
						if replaceAll {
							newContent = strings.ReplaceAll(original, oldStr, replaceWith)
						} else {
							newContent = strings.Replace(original, oldStr, replaceWith, 1)
						}
					}

					oldData, _ := os.ReadFile(path)
					diff := computeSimpleDiff(path, string(oldData), newContent)
					a.diffPane.SetContent(diff)
					a.conversation.UpdateToolContent(tc.ID, diff)

					// Use diff component for confirmation (not separate confirm component)
					if replyCh, ok := payload["reply"].(chan agent.ConfirmationResponse); ok {
						a.diffPane.ShowWithConfirm(replyCh)
						a.pendingDiffHide = true
					}
					a.layout()
				} else {
					// Non-file tools use confirm component
					permission := permissionInfoForTool(tc, name)
					a.confirm.ShowPermission(permission)
					a.statusBar.SetPermission(permission.Tool)
					a.layout()
					if replyCh, ok := payload["reply"].(chan agent.ConfirmationResponse); ok {
						wrapped := make(chan agent.ConfirmationResponse, 1)
						a.confirm.SetReply(wrapped)
						go func() {
							select {
							case res, ok := <-wrapped:
								if ok {
									replyCh <- res
								}
							case <-a.ctx.Done():
								select {
								case replyCh <- agent.ConfirmationResponse{Allow: false}:
								default:
								}
							}
						}()
					}
				}
			}
		}
		return a.waitForAgentEvent()
	case agent.EventAskUser:
		if payload, ok := ev.Payload.(map[string]any); ok {
			question, _ := payload["question"].(string)
			replyCh, _ := payload["reply"].(chan string)
			a.askUserReplyCh = replyCh
			a.statusBar.SetStatus("PROMPT: " + question)
			a.input.Focus()
		}
		return a.waitForAgentEvent()
	}
	return a.waitForAgentEvent()
}

func (a *App) handleSlashCommand(input string) tea.Cmd {
	parts := strings.Fields(input)
	if len(parts) == 0 {
		return nil
	}
	cmd := parts[0]
	args := parts[1:]
	if definition, ok := lookupSlashCommand(strings.TrimPrefix(cmd, "/")); ok {
		cmd = "/" + definition.Name
	}
	if handled, command := a.handleWorkflowConfigCommand(cmd, args); handled {
		return command
	}
	switch cmd {
	case "/help":
		a.showHelp = true
	case "/clear":
		a.conversation.Clear()
		a.statusBar.SetStatus("Conversation cleared")
	case "/reset":
		a.conversation.Clear()
		a.sess.SetMessages(nil)
		a.statusBar.SetStatus("History reset")
	case "/compact":
		a.statusBar.SetStatus("Compacting context...")
		return a.compactContext()
	case "/new":
		if a.storage != nil && a.sess != nil && len(a.sess.Messages) > 0 {
			_ = a.storage.Save(a.sess)
		}
		a.conversation.Clear()
		a.sess = session.New()
		a.sess.Provider, a.sess.Model = a.cfg.Provider, a.cfg.Model
		a.sess.WorkDir = a.workDir
		a.ag.SetSession(a.sess)
		if a.persist != nil {
			a.persist.SetSession(a.sess)
		}
		a.updateActiveTokens()
		a.stats.TotalCost = 0
		a.statusBar.SetStatus("New session started")
	case "/context":
		if len(args) > 0 && args[0] == "detail" {
			a.showContextDetail()
		} else {
			a.stats.InputTokens = a.sess.TotalInputTokens
			a.stats.OutputTokens = a.sess.TotalOutputTokens
			if tel := a.ag.Telemetry(); tel != nil {
				a.stats.TotalCost = tel.GetCostSummary().TotalCostUSD
			}
			a.conversation.AddMessage("system", fmt.Sprintf("Provider: %s\nModel: %s\nInput tokens: %d\nOutput tokens: %d\nTotal cost: $%.4f\n\nUse '/context detail' for telemetry breakdown.", a.cfg.Provider, a.cfg.Model, a.sess.TotalInputTokens, a.sess.TotalOutputTokens, a.stats.TotalCost), false)
		}
	case "/provider":
		if len(args) == 0 {
			a.conversation.AddMessage("system", fmt.Sprintf("Current provider: %s (model: %s)", a.cfg.Provider, a.cfg.Model), false)
			return nil
		}
		model := ""
		if len(args) > 1 {
			model = args[1]
		}
		if err := a.switchProvider(args[0], model); err != nil {
			a.conversation.AddMessage("assistant", fmt.Sprintf("Error: %v", err), true)
			a.statusBar.SetStatus("Error")
			return nil
		}
		a.conversation.AddMessage("system", fmt.Sprintf("Provider switched to %s", args[0]), false)
		// Warn if API key is missing for the new provider
		if pc, ok := a.cfg.Providers[args[0]]; !ok || pc.APIKey == "" {
			a.conversation.AddMessage("system", fmt.Sprintf("Warning: No API key set for %s. Use /api-key <key> or set the appropriate environment variable.", args[0]), false)
			a.statusBar.SetStatus("Provider updated (no API key)")
		} else {
			a.statusBar.SetStatus("Provider updated")
		}
		_ = a.persistProjectConfig()
	case "/model":
		if len(args) == 0 {
			a.conversation.AddMessage("system", fmt.Sprintf("Current model: %s (provider: %s)\nUsage: /model <model-name>", a.cfg.Model, a.cfg.Provider), false)
			return nil
		}
		if err := a.switchProvider(a.cfg.Provider, args[0]); err != nil {
			a.conversation.AddMessage("assistant", fmt.Sprintf("Error: %v", err), true)
			a.statusBar.SetStatus("Error")
			return nil
		}
		a.conversation.AddMessage("system", fmt.Sprintf("Model switched to %s", args[0]), false)
		a.statusBar.SetStatus("Model updated")
		_ = a.persistProjectConfig()
	case "/mode":
		if len(args) > 0 && agent.IsValid(args[0]) {
			a.cfg.Mode = args[0]
			a.header.SetMode(args[0])
			a.conversation.AddMessage("system", fmt.Sprintf("Mode switched to %s", args[0]), false)
			_ = a.persistProjectConfig()
		} else {
			a.conversation.AddMessage("assistant", "Error: usage /mode <edit|plan>", true)
		}
	case "/api-key":
		if len(args) == 0 {
			a.conversation.AddMessage("assistant", "Error: usage /api-key <value>", true)
			return nil
		}
		a.ensureProviderConfig(a.cfg.Provider)
		pc := a.cfg.Providers[a.cfg.Provider]
		pc.APIKey = strings.Join(args, " ")
		a.cfg.Providers[a.cfg.Provider] = pc
		if err := a.switchProvider(a.cfg.Provider, a.cfg.Model); err != nil {
			a.conversation.AddMessage("assistant", fmt.Sprintf("Error: %v", err), true)
			return nil
		}
		a.conversation.AddMessage("system", fmt.Sprintf("API key updated for %s", a.cfg.Provider), false)
		a.statusBar.SetStatus("API key updated")
		_ = a.persistProjectConfig()
	case "/base-url":
		if len(args) == 0 {
			a.conversation.AddMessage("assistant", "Error: usage /base-url <url>", true)
			return nil
		}
		a.ensureProviderConfig(a.cfg.Provider)
		pc := a.cfg.Providers[a.cfg.Provider]
		pc.BaseURL = args[0]
		a.cfg.Providers[a.cfg.Provider] = pc
		if err := a.switchProvider(a.cfg.Provider, a.cfg.Model); err != nil {
			a.conversation.AddMessage("assistant", fmt.Sprintf("Error: %v", err), true)
			return nil
		}
		a.conversation.AddMessage("system", fmt.Sprintf("Base URL updated for %s", a.cfg.Provider), false)
		a.statusBar.SetStatus("Base URL updated")
		_ = a.persistProjectConfig()
	case "/provider-api-key":
		if len(args) < 2 {
			a.conversation.AddMessage("assistant", "Error: usage /provider-api-key <provider> <value>", true)
			return nil
		}
		provider := args[0]
		a.ensureProviderConfig(provider)
		pc := a.cfg.Providers[provider]
		pc.APIKey = strings.Join(args[1:], " ")
		a.cfg.Providers[provider] = pc
		if provider == a.cfg.Provider {
			if err := a.switchProvider(a.cfg.Provider, a.cfg.Model); err != nil {
				a.conversation.AddMessage("assistant", fmt.Sprintf("Error: %v", err), true)
				return nil
			}
		}
		a.conversation.AddMessage("system", fmt.Sprintf("API key updated for %s", provider), false)
		_ = a.persistProjectConfig()
	case "/provider-base-url":
		if len(args) < 2 {
			a.conversation.AddMessage("assistant", "Error: usage /provider-base-url <provider> <url>", true)
			return nil
		}
		provider := args[0]
		a.ensureProviderConfig(provider)
		pc := a.cfg.Providers[provider]
		pc.BaseURL = args[1]
		a.cfg.Providers[provider] = pc
		if provider == a.cfg.Provider {
			if err := a.switchProvider(a.cfg.Provider, a.cfg.Model); err != nil {
				a.conversation.AddMessage("assistant", fmt.Sprintf("Error: %v", err), true)
				return nil
			}
		}
		a.conversation.AddMessage("system", fmt.Sprintf("Base URL updated for %s", provider), false)
		_ = a.persistProjectConfig()
	case "/sessions":
		a.showSessions()
	case "/resume":
		if len(args) > 0 {
			if err := a.resumeSession(args[0]); err != nil {
				a.statusBar.SetStatus("Unable to resume session: " + err.Error())
			}
		} else {
			a.showSessions()
		}
	case "/diff":
		a.diffPane.Toggle()
		a.layout()
	case "/tree":
		a.showFileTree = !a.showFileTree
		a.layout()
	case "/lsp":
		a.lspPanel.Toggle()
		a.layout()
	case "/review":
		a.conversation.SetReviewMode(!a.conversation.ReviewMode())
		a.statusBar.SetStatus(fmt.Sprintf("Review mode %s", map[bool]string{true: "enabled", false: "disabled"}[a.conversation.ReviewMode()]))
	case "/cancel":
		if a.thinking {
			a.cancelActiveRun("Cancelled by user")
		} else {
			a.statusBar.SetStatus("No active request")
		}
	case "/search":
		if len(args) == 0 {
			a.conversation.AddMessage("assistant", "Usage: /search <query>", true)
			return nil
		}
		a.conversation.AddMessage("system", a.searchWorkspace(strings.Join(args, " ")), false)
	case "/run":
		if len(args) == 0 {
			a.conversation.AddMessage("assistant", "Usage: /run <command>", true)
			return nil
		}
		return a.startAgent("Run this project command: " + strings.Join(args, " "))
	case "/test":
		request := "Run the project tests"
		if len(args) > 0 {
			request += " for " + strings.Join(args, " ")
		}
		return a.startAgent(request)
	case "/build":
		request := "Build the project"
		if len(args) > 0 {
			request += " with target " + strings.Join(args, " ")
		}
		return a.startAgent(request)
	case "/export":
		path := "conversation.md"
		if len(args) > 0 {
			path = args[0]
		}
		if err := a.exportConversation(path); err != nil {
			a.conversation.AddMessage("assistant", fmt.Sprintf("Export failed: %v", err), true)
			return nil
		}
		a.statusBar.SetStatus("Conversation exported to " + path)
	case "/stats":
		a.stats.InputTokens = a.sess.TotalInputTokens
		a.stats.OutputTokens = a.sess.TotalOutputTokens
		a.conversation.AddMessage("system", a.stats.View(), false)
	case "/approvals":
		a.handleApprovalsCommand(args)
	case "/quit", "/exit":
		return tea.Quit
	default:
		a.conversation.AddMessage("assistant", fmt.Sprintf("Unknown command: %s", cmd), true)
	}
	return nil
}

func (a *App) layout() {
	if a.width <= 0 || a.height <= 0 {
		return
	}

	a.header.SetWidth(a.width)
	a.statusBar.SetWidth(a.width)
	a.input.SetWidth(a.width)
	a.palette.SetSize(a.width, a.height)
	a.confirm.SetSize(a.width, a.height)
	a.coAuthorConfirm.SetSize(a.width, a.height)

	headerH := lipgloss.Height(a.header.View())
	statusH := lipgloss.Height(a.statusBar.View())
	footerH := 0
	if !a.browsing {
		footerH = lipgloss.Height(a.input.View())
	}
	if a.confirm.Visible() {
		footerH = lipgloss.Height(a.confirm.View())
	}
	if a.thinking {
		footerH++
	}
	// Palette and secondary confirmations render inline below the input.
	if a.palette.Visible() && !a.confirm.Visible() && !a.browsing {
		footerH += a.palette.Height()
	}
	if a.coAuthorConfirm.Visible() {
		footerH += lipgloss.Height(a.coAuthorConfirm.View())
	}

	mainH := a.height - headerH - statusH - footerH
	if mainH < 1 {
		mainH = 1
	}

	// Diff is now fullscreen overlay - always set to full dimensions
	a.diffPane.SetSize(a.width, a.height)

	mainW := a.width
	if a.showFileTree {
		treeW := 25
		if a.width > 80 {
			treeW = a.width / 5
		}
		a.fileTree.SetSize(treeW, mainH)
		mainW = a.width - treeW - 1
	}

	if a.lspPanel.Visible() {
		convW := mainW * 70 / 100
		lspW := mainW - convW - 1
		a.conversation.SetSize(convW, mainH)
		a.lspPanel.SetSize(lspW, mainH)
	} else {
		a.conversation.SetSize(mainW, mainH)
	}
	a.sessionBrowser.SetSize(a.width*3/4, a.height*3/4)
}

func (a *App) View() tea.View {
	// Helper to ensure all views have consistent settings
	makeView := func(content string) tea.View {
		v := tea.NewView(content)
		v.AltScreen = true
		v.MouseMode = tea.MouseModeCellMotion // Capture mouse to prevent terminal scrollback
		return v
	}

	if a.width <= 0 || a.height <= 0 {
		return makeView("Initializing...")
	}
	if a.showHelp {
		return makeView(a.helpOverlay.View())
	}

	headerView := a.header.View()
	statusView := a.statusBar.View()
	var sections []string
	if headerView != "" {
		sections = append(sections, headerView)
	}

	var mainRow string
	if a.sessionBrowser.Visible() {
		mainRow = a.sessionBrowser.View()
	} else {
		convView := a.conversation.View()
		// Only wrap in ActivePane border if we have multiple panes (FileTree or LSP)
		hasOtherPanes := a.showFileTree || a.lspPanel.Visible()
		if a.focus == "conversation" && hasOtherPanes {
			convView = a.styles.ActivePane.Width(lipgloss.Width(convView)).Render(convView)
		}
		if a.showFileTree {
			mainRow = lipgloss.JoinHorizontal(lipgloss.Top, a.fileTree.View(), " ", convView)
		} else {
			mainRow = convView
		}
		if a.lspPanel.Visible() {
			mainRow = lipgloss.JoinHorizontal(lipgloss.Top, mainRow, " ", a.lspPanel.View())
		}
	}

	sections = append(sections, mainRow)

	// Footer: spinner, input, then inline palette/confirmation panels below the input.
	var footer []string
	if a.thinking {
		footer = append(footer, "  "+a.spin.View())
	}
	if a.confirm.Visible() {
		footer = append(footer, a.confirm.View())
	} else if !a.browsing {
		footer = append(footer, a.input.View())
	}
	if a.palette.Visible() && !a.confirm.Visible() && !a.browsing {
		footer = append(footer, a.palette.View())
	}
	if a.coAuthorConfirm.Visible() {
		footer = append(footer, a.coAuthorConfirm.View())
	}
	sections = append(sections, lipgloss.JoinVertical(lipgloss.Left, footer...))
	sections = append(sections, statusView)

	fullView := lipgloss.JoinVertical(lipgloss.Left, sections...)

	// Diff is now fullscreen overlay - render on top of everything
	if a.diffPane.Visible() {
		fullView = a.diffPane.View()
	}

	return makeView(fullView)
}

func computeSimpleDiff(filename, old, new string) string {
	dmp := diffmatchpatch.New()

	// Use line-mode diff for better performance on large files
	a, b, lineArray := dmp.DiffLinesToChars(old, new)
	diffs := dmp.DiffMain(a, b, false)
	diffs = dmp.DiffCharsToLines(diffs, lineArray)
	diffs = dmp.DiffCleanupSemantic(diffs)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("--- %s (current)\n", filename))
	sb.WriteString(fmt.Sprintf("+++ %s (proposed)\n", filename))

	// Count lines for hunk header
	oldLineCount := 0
	newLineCount := 0
	for _, d := range diffs {
		lines := strings.Split(strings.TrimSuffix(d.Text, "\n"), "\n")
		switch d.Type {
		case diffmatchpatch.DiffEqual:
			oldLineCount += len(lines)
			newLineCount += len(lines)
		case diffmatchpatch.DiffDelete:
			oldLineCount += len(lines)
		case diffmatchpatch.DiffInsert:
			newLineCount += len(lines)
		}
	}

	sb.WriteString(fmt.Sprintf("@@ -1,%d +1,%d @@\n", oldLineCount, newLineCount))

	// Generate unified diff output with word-level highlighting hints
	for i, d := range diffs {
		text := strings.TrimSuffix(d.Text, "\n")
		lines := strings.Split(text, "\n")

		switch d.Type {
		case diffmatchpatch.DiffEqual:
			for _, line := range lines {
				sb.WriteString(" " + line + "\n")
			}
		case diffmatchpatch.DiffDelete:
			// Check if next diff is an insert (replacement scenario)
			if i+1 < len(diffs) && diffs[i+1].Type == diffmatchpatch.DiffInsert {
				// Word-level diff for replacements
				insertText := strings.TrimSuffix(diffs[i+1].Text, "\n")
				insertLines := strings.Split(insertText, "\n")

				// If same number of lines, do inline word diff
				if len(lines) == len(insertLines) {
					for j, oldLine := range lines {
						newLine := insertLines[j]
						wordDiff := computeWordDiff(dmp, oldLine, newLine)
						sb.WriteString(wordDiff)
					}
					// Mark that we handled the insert
					diffs[i+1] = diffmatchpatch.Diff{Type: diffmatchpatch.DiffEqual, Text: ""}
					continue
				}
			}
			for _, line := range lines {
				sb.WriteString("-" + line + "\n")
			}
		case diffmatchpatch.DiffInsert:
			if d.Text == "" {
				continue // Already handled in word-diff
			}
			for _, line := range lines {
				sb.WriteString("+" + line + "\n")
			}
		}
	}
	return sb.String()
}

// computeWordDiff generates a word-level diff for a single line replacement
func computeWordDiff(dmp *diffmatchpatch.DiffMatchPatch, oldLine, newLine string) string {
	wordDiffs := dmp.DiffMain(oldLine, newLine, false)
	wordDiffs = dmp.DiffCleanupSemantic(wordDiffs)

	var oldSb, newSb strings.Builder
	hasChanges := false

	for _, wd := range wordDiffs {
		switch wd.Type {
		case diffmatchpatch.DiffEqual:
			oldSb.WriteString(wd.Text)
			newSb.WriteString(wd.Text)
		case diffmatchpatch.DiffDelete:
			oldSb.WriteString("«" + wd.Text + "»")
			hasChanges = true
		case diffmatchpatch.DiffInsert:
			newSb.WriteString("‹" + wd.Text + "›")
			hasChanges = true
		}
	}

	if hasChanges {
		return "-" + oldSb.String() + "\n+" + newSb.String() + "\n"
	}
	return " " + oldLine + "\n"
}

func formatErrorMessage(errStr string) string {
	// First, sanitize any URLs to hide API keys
	errStr = sanitizeURLs(errStr)

	// Handle user cancellation gracefully
	if isCancellationError(errStr) {
		return "Request cancelled"
	}
	if strings.Contains(errStr, "401") || strings.Contains(errStr, "authentication_error") {
		return "API Key missing or invalid. Use /api-key <value> to set it, or export the appropriate environment variable (e.g., GEMINI_API_KEY)."
	}
	if strings.Contains(errStr, "403") {
		return "API access forbidden. Check your account permissions or billing status."
	}
	if strings.Contains(errStr, "429") {
		return "Rate limit exceeded. Please wait a moment before trying again."
	}
	if strings.Contains(errStr, "connection refused") || strings.Contains(errStr, "no such host") {
		return "Connection failed. Check your network or API endpoint configuration."
	}
	if strings.Contains(errStr, "timeout") || strings.Contains(errStr, "deadline exceeded") {
		return "Request timed out. The server may be busy, please try again."
	}
	return "Error: " + errStr
}

func isCancellationError(errStr string) bool {
	s := strings.ToLower(errStr)
	return strings.Contains(s, "context canceled") || strings.Contains(s, "context cancelled")
}

// sanitizeURLs removes sensitive data (API keys, tokens) from URLs in error messages
func sanitizeURLs(s string) string {
	// Pattern to match common API key patterns in URLs
	patterns := []struct {
		pattern string
		replace string
	}{
		// ?key=xxx or &key=xxx
		{`([?&])key=[^&\s"']+`, `$1key=***`},
		// ?api_key=xxx or &api_key=xxx
		{`([?&])api_key=[^&\s"']+`, `$1api_key=***`},
		// ?apikey=xxx or &apikey=xxx
		{`([?&])apikey=[^&\s"']+`, `$1apikey=***`},
		// ?token=xxx or &token=xxx
		{`([?&])token=[^&\s"']+`, `$1token=***`},
		// ?access_token=xxx
		{`([?&])access_token=[^&\s"']+`, `$1access_token=***`},
		// Bearer tokens in headers shown in errors
		{`Bearer\s+[A-Za-z0-9_\-\.]+`, `Bearer ***`},
		// x-api-key header values
		{`x-api-key:\s*[^\s"']+`, `x-api-key: ***`},
	}

	result := s
	for _, p := range patterns {
		re := regexp.MustCompile(p.pattern)
		result = re.ReplaceAllString(result, p.replace)
	}
	return result
}

func isTransientStatus(s string) bool {
	n := strings.ToLower(strings.TrimSpace(s))
	if n == "thinking" || n == "thinking…" || n == "thinking..." {
		return true
	}
	return strings.HasPrefix(n, "running ")
}

func Run(cfg *config.Config, ag *agent.Agent, sess *session.Session, storage *session.Storage, persist *session.PersistenceManager, initialPrompt string, showSessionPicker bool, projectApprovalPath string) error {
	// Enter alternate screen immediately before TUI starts to prevent
	// any flash of existing terminal content
	fmt.Print("\x1b[?1049h") // Enter alt screen
	fmt.Print("\x1b[H")      // Move cursor to home position
	fmt.Print("\x1b[2J")     // Clear entire screen

	app := NewApp(cfg, ag, sess, storage, persist, initialPrompt, showSessionPicker)
	app.requireProjectApproval(projectApprovalPath)
	p := tea.NewProgram(app)
	_, err := p.Run()

	// Exit alternate screen after TUI ends
	fmt.Print("\x1b[?1049l")
	return err
}

func (a *App) cancelActiveRun(status string) {
	a.cancel()
	a.ctx, a.cancel = context.WithCancel(context.Background())
	a.thinking = false
	a.spin.Stop()

	// Clean up any pending ask_user channel to prevent agent deadlock
	if a.askUserReplyCh != nil {
		select {
		case a.askUserReplyCh <- "": // Send empty response to unblock
		default:
		}
		a.askUserReplyCh = nil
	}

	// Drain any pending events from the cancelled run
	for {
		select {
		case <-a.ag.Events():
		default:
			goto done
		}
	}
done:
	a.layout() // Reclaim space
	if status != "" {
		a.statusBar.SetStatus(status)
	}
}

func (a *App) persistProjectConfig() error {
	return a.cfg.SaveIfLoaded()
}

func appendUniquePath(paths []string, path string) []string {
	for _, existing := range paths {
		if existing == path {
			return paths
		}
	}
	return append(paths, path)
}

func permissionInfoForTool(tc ai.ToolCall, name string) components.PermissionInfo {
	info := components.PermissionInfo{
		Icon:   "󰌑",
		Tool:   name,
		Action: "Requesting permission to use this tool",
		Risk:   "May change workspace or execute external operations",
	}
	add := func(label, value string) {
		if value == "" {
			return
		}
		info.Fields = append(info.Fields, components.PermissionField{Label: label, Value: value})
	}
	str := func(key string) string {
		value, _ := tc.Args[key].(string)
		return value
	}

	switch tc.Name {
	case "read_file", "view":
		info.Icon, info.Action, info.Risk = "󰈔", "Read file contents", "Reads workspace data"
		add("Path", str("path"))
	case "list_directory":
		info.Icon, info.Action, info.Risk = "󰉋", "Inspect directory structure", "Reads workspace metadata"
		add("Path", str("path"))
	case "run_shell_command", "run_command", "bash":
		info.Icon, info.Action, info.Risk = "󰆍", "Execute shell command", "Runs a local process"
		add("Command", str("command"))
		add("Directory", str("working_directory"))
	case "web_fetch", "web_search":
		info.Icon, info.Action, info.Risk = "󰖟", "Access web resource", "Sends a network request"
		add("URL", str("url"))
		add("Query", str("query"))
	case "git_commit":
		info.Icon, info.Action, info.Risk = "󰊢", "Create git commit", "Changes repository history"
		add("Message", str("message"))
	default:
		if ctx := extractToolContext(tc.Name, tc.Args); ctx != "" {
			add("Context", ctx)
		}
	}
	return info
}

func extractToolContext(name string, args map[string]any) string {
	if args == nil {
		return ""
	}
	switch name {
	case "read_file", "view":
		if path, ok := args["path"].(string); ok {
			return "reading " + filepath.Base(path)
		}
	case "write_file", "create_file":
		if path, ok := args["path"].(string); ok {
			return "writing " + filepath.Base(path)
		}
	case "edit_file":
		if path, ok := args["path"].(string); ok {
			return "editing " + filepath.Base(path)
		}
	case "delete_file":
		if path, ok := args["path"].(string); ok {
			return "deleting " + filepath.Base(path)
		}
	case "move_file":
		src, _ := args["source"].(string)
		dst, _ := args["destination"].(string)
		return "moving " + filepath.Base(src) + " -> " + filepath.Base(dst)
	case "copy_file":
		src, _ := args["source"].(string)
		dst, _ := args["destination"].(string)
		return "copying " + filepath.Base(src) + " -> " + filepath.Base(dst)
	case "list_directory":
		if path, ok := args["path"].(string); ok {
			return "listing " + filepath.Base(path)
		}
	case "run_shell_command", "run_command":
		if cmd, ok := args["command"].(string); ok {
			if len(cmd) > 40 {
				return "exec: " + cmd[:37] + "..."
			}
			return "exec: " + cmd
		}
	case "grep_search", "search":
		if pattern, ok := args["pattern"].(string); ok {
			return "search: " + pattern
		}
	case "web_fetch":
		if u, ok := args["url"].(string); ok {
			return "fetch: " + u
		}
	case "web_search":
		if q, ok := args["query"].(string); ok {
			return "web: " + q
		}
	case "lsp_diagnostics":
		if path, ok := args["path"].(string); ok {
			return "diagnostics: " + filepath.Base(path)
		}
	}
	return ""
}

func truncateUIContent(s string, reviewMode bool) string {
	if reviewMode {
		return s
	}
	const maxRunes = 500
	if utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	runes := []rune(s)
	return string(runes[:maxRunes]) + " … [truncated, press Ctrl+R for full review mode]"
}

func (a *App) ensureProviderConfig(provider string) {
	if a.cfg.Providers == nil {
		a.cfg.Providers = map[string]config.ProviderConfig{}
	}
	if _, ok := a.cfg.Providers[provider]; !ok {
		a.cfg.Providers[provider] = config.ProviderConfig{}
	}
}

func (a *App) switchProvider(provider, model string) error {
	if provider == "" {
		return fmt.Errorf("provider cannot be empty")
	}
	if !isSupportedProvider(provider) {
		return fmt.Errorf("unknown provider %q", provider)
	}
	a.ensureProviderConfig(provider)
	oldProvider, oldModel := a.cfg.Provider, a.cfg.Model
	a.cfg.Provider = provider
	if model == "" {
		model = defaultModelForProvider(provider)
	}
	a.cfg.Model = model
	a.sess.Provider = a.cfg.Provider
	a.sess.Model = a.cfg.Model
	p, err := buildProviderFromConfig(a.cfg)
	if err != nil {
		a.cfg.Provider = oldProvider
		a.cfg.Model = oldModel
		a.sess.Provider = oldProvider
		a.sess.Model = oldModel
		return err
	}
	a.ag.SetProvider(p)
	a.header.SetProvider(a.cfg.Provider)
	a.header.SetModel(a.cfg.Model)
	a.availableModels = nil
	return nil
}

func isSupportedProvider(name string) bool {
	switch name {
	case "google":
		return true
	default:
		return false
	}
}

func defaultModelForProvider(provider string) string {
	switch provider {
	case "google":
		return "gemini-3.6-flash"
	default:
		return ""
	}
}

func (a *App) updateActiveTokens() {
	if a.ag == nil {
		return
	}
	mgr := a.ag.ContextManager()
	if mgr == nil {
		return
	}
	calc := mgr.AdaptiveCalculator()
	if calc == nil {
		return
	}
	if a.sess == nil {
		return
	}
	active := calc.EstimateMessages(a.sess.Messages)

	// Add currently-being-typed prompt if it's non-empty
	if pending := a.input.Value(); pending != "" {
		active += calc.Estimate(pending)
	}

	a.header.SetActiveTokens(active)
}

func (a *App) showContextDetail() {
	var b strings.Builder
	b.WriteString("# Context Telemetry\n\n")

	// Adaptive token weight
	if calc := a.ag.AdaptiveCalculator(); calc != nil {
		b.WriteString(fmt.Sprintf("## Adaptive Token Estimation\n- Model: %s\n- Learned Weight: %.2f\n- Samples: %d\n\n",
			calc.Model(), calc.Weight(), calc.Samples()))
	}

	// Telemetry collector
	if tel := a.ag.Telemetry(); tel != nil {
		breakdowns := tel.GetBreakdowns(1)
		if len(breakdowns) > 0 {
			bd := breakdowns[0]
			b.WriteString(fmt.Sprintf("## Latest Context Breakdown\n- Total: %d tokens\n- System Prompt: %d\n- Tool Definitions: %d\n- Conversation: %d\n- Context Files: %d\n- Tool Calls: %d\n- Thinking: %d\n- Output Reserve: %d\n- Safety Margin: %d\n- Provider Actual: %d\n- Est. Weight: %.2f\n\n",
				bd.TotalTokens, bd.SystemPrompt, bd.ToolDefinitions, bd.Conversation,
				bd.ContextFiles, bd.ToolCalls, bd.Thinking, bd.OutputReserve, bd.SafetyMargin,
				bd.ProviderActual, bd.EstimationWeight))
		}

		compacts := tel.GetCompactionEvents(5)
		if len(compacts) > 0 {
			b.WriteString("## Recent Compaction Events\n")
			for _, c := range compacts {
				b.WriteString(fmt.Sprintf("- %s: %s (tier: %s) %d→%d tokens (%dms) %s\n",
					c.Timestamp.Format("15:04:05"), c.Reason, c.Strategy,
					c.TokensBefore, c.TokensAfter, c.DurationMs,
					map[bool]string{true: "✓", false: "✗"}[c.Success]))
			}
			b.WriteString("\n")
		}

		cost := tel.GetCostSummary()
		if cost.TotalCostUSD > 0 || cost.TotalInputTokens > 0 {
			b.WriteString(fmt.Sprintf("## Cost Summary\n- Total: $%.4f\n- Input Tokens: %d\n- Output Tokens: %d\n",
				cost.TotalCostUSD, cost.TotalInputTokens, cost.TotalOutputTokens))
			for model, mc := range cost.ByModel {
				if mc.TotalCost > 0 || mc.InputTokens > 0 || mc.OutputTokens > 0 {
					b.WriteString(fmt.Sprintf("  - %s: $%.4f (%d in / %d out)\n", model, mc.TotalCost, mc.InputTokens, mc.OutputTokens))
				}
			}
			b.WriteString("\n")
		}

		// Transcript info
		if mgr := a.ag.ContextManager(); mgr != nil {
			if tm := mgr.TranscriptManager(); tm != nil {
				items := tm.RawMessages()
				b.WriteString(fmt.Sprintf("## Transcript\n- Total messages: %d\n", len(items)))
				if pt := tm.PristineMessages(); len(pt) != len(items) {
					b.WriteString(fmt.Sprintf("- Pristine (never compacted): %d\n", len(pt)))
				}
			}
		}
	}

	a.conversation.AddMessage("system", b.String(), false)
}

func (a *App) compactContext() tea.Cmd {
	ctx := a.ctx
	return func() tea.Msg {
		if a.ag == nil {
			return nil
		}
		compacted := a.ag.CompactSessionMessages(ctx, a.sess.Messages)
		a.sess.SetMessages(compacted)
		// Update transcript with compacted messages
		if mgr := a.ag.ContextManager(); mgr != nil {
			if tm := mgr.TranscriptManager(); tm != nil {
				// Rebuild transcript from compacted messages
				tm.Rollback(0)
				for _, m := range compacted {
					tm.Append(m)
				}
			}
		}
		return agentEventMsg{ev: agent.Event{Type: agent.EventCompacted}}
	}
}

func buildProviderFromConfig(cfg *config.Config) (ai.Provider, error) {
	pc := cfg.Providers[cfg.Provider]
	enablePromptCache := shouldEnablePromptCache(cfg, cfg.Provider)
	aiCfg := ai.ProviderConfig{
		APIKey:             pc.APIKey,
		BaseURL:            pc.BaseURL,
		DefaultModel:       cfg.Model,
		OrgID:              pc.OrgID,
		PromptCacheEnabled: &enablePromptCache,
	}

	var provider ai.Provider
	switch cfg.Provider {
	case "google":
		provider = googleProvider.New(aiCfg)
	default:
		if p, ok := ai.Get(cfg.Provider); ok {
			provider = p
			break
		}
		return nil, fmt.Errorf("unknown provider %q", cfg.Provider)
	}

	if shouldWrapPromptCacheProvider(cfg, provider) {
		return cache.NewCachingProvider(provider, cache.NewPromptCache()), nil
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

func shouldWrapPromptCacheProvider(cfg *config.Config, provider ai.Provider) bool {
	return shouldEnablePromptCache(cfg, provider.Name())
}
