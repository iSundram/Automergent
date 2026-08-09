package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/iSundram/Automergent/internal/agent"
	"github.com/iSundram/Automergent/internal/ai"
	"github.com/iSundram/Automergent/internal/config"
	"github.com/iSundram/Automergent/internal/session"
	"github.com/iSundram/Automergent/internal/tui/components"
	"github.com/iSundram/Automergent/internal/tui/keys"
	"github.com/iSundram/Automergent/internal/tui/themes"
)

// agentEventMsg wraps an agent.Event for BubbleTea dispatch.
type agentEventMsg struct{ ev agent.Event }

// App is the root Bubble Tea model.
type App struct {
	cfg            *config.Config
	ag             *agent.Agent
	sess           *session.Session
	keys           *keys.Bindings
	styles         *themes.Styles
	theme          *themes.Theme
	conversation   components.Conversation
	diffPane       components.Diff
	input          components.Input
	header         components.Header
	statusBar      components.StatusBar
	spin           components.Spinner
	confirm        components.Confirm
	sessionBrowser components.SessionBrowser
	lspPanel       components.LSPPanel
	stats          components.Stats
	helpOverlay    components.HelpOverlay
	fileTree       components.FileTree
	width          int
	height         int
	thinking       bool
	statusMsg      string
	showFileTree   bool
	showHelp       bool
	ctx            context.Context
	cancel         context.CancelFunc
	initialPrompt  string
	focus          string // "input", "conversation", "diff", "tree"
}

// NewApp creates the root TUI model.
func NewApp(cfg *config.Config, ag *agent.Agent, sess *session.Session, initialPrompt string) *App {
	theme := themes.Get(cfg.Theme)
	styles := themes.NewStyles(theme)
	kb := keys.Get(cfg.Keybindings)

	ctx, cancel := context.WithCancel(context.Background())

	app := &App{
		cfg:            cfg,
		ag:             ag,
		sess:           sess,
		keys:           kb,
		styles:         styles,
		theme:          theme,
		conversation:   components.NewConversation(styles),
		diffPane:       components.NewDiff(styles),
		input:          components.NewInput(styles),
		header:         components.NewHeader(styles),
		statusBar:      components.NewStatusBar(styles),
		spin:           components.NewSpinner(styles),
		confirm:        components.NewConfirm(styles),
		sessionBrowser: components.NewSessionBrowser(styles),
		lspPanel:       components.NewLSPPanel(styles),
		stats:          components.NewStats(styles),
		helpOverlay:    components.NewHelpOverlay(styles),
		fileTree:       components.NewFileTree(styles),
		ctx:            ctx,
		cancel:         cancel,
		initialPrompt:  initialPrompt,
		statusMsg:      "Ready",
		focus:          "input",
	}
	app.header.SetModel(cfg.Model)
	app.header.SetProvider(cfg.Provider)
	app.header.SetMode(cfg.Mode)
	return app
}

// Init is the initial command.
func (a *App) Init() tea.Cmd {
	cmds := []tea.Cmd{
		a.input.Focus(),
		a.spin.Tick(),
		a.fileTree.Load("."),
	}
	if a.initialPrompt != "" {
		cmds = append(cmds, a.startAgent(a.initialPrompt))
	}
	return tea.Batch(cmds...)
}

// startAgent kicks off the agent goroutine and begins polling events.
func (a *App) startAgent(prompt string) tea.Cmd {
	a.thinking = true
	a.spin.Start()
	a.conversation.AddMessage("user", prompt, false)
	a.statusBar.SetStatus("Thinking…")

	go func() {
		_ = a.ag.Run(a.ctx, prompt)
	}()

	return a.waitForAgentEvent()
}

// waitForAgentEvent returns a Cmd that blocks until the next agent event.
func (a *App) waitForAgentEvent() tea.Cmd {
	return func() tea.Msg {
		ev := <-a.ag.Events()
		return agentEventMsg{ev: ev}
	}
}

// Update handles messages.
func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = m.Width
		a.height = m.Height
		a.layout()
		return a, nil

	case tea.KeyMsg:
		cmd = a.handleKey(m)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}

	case agentEventMsg:
		cmd = a.handleAgentEvent(m.ev)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}

	case spinner.TickMsg:
		sp, cmd := a.spin.Update(msg)
		a.spin = sp
		cmds = append(cmds, cmd)

	case components.FileTreeLoadedMsg:
		a.fileTree.SetItems(m.Items)
	}

	// Always update these if they're visible
	if a.sessionBrowser.Visible() {
		sb, cmd := a.sessionBrowser.Update(msg)
		a.sessionBrowser = sb
		cmds = append(cmds, cmd)
	}
	if a.confirm.Visible() {
		c, cmd := a.confirm.Update(msg)
		a.confirm = c
		cmds = append(cmds, cmd)
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

	// Global Hotkeys
	key := m.String()
	switch key {
	case "ctrl+c", "ctrl+q":
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
		a.sessionBrowser.Show()
		return nil
	case "ctrl+t":
		a.showFileTree = !a.showFileTree
		a.layout()
		return nil
	case "?":
		if a.input.Value() == "" {
			a.showHelp = true
			return nil
		}
	case "tab":
		// Cycle focus
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
		if a.focus == "input" {
			return a.input.Focus()
		}
		a.input.Blur()
		return nil
	}

	// Forward keys to focused component
	var cmd tea.Cmd
	switch a.focus {
	case "input":
		if (key == "enter" || key == "ctrl+m") && !a.thinking {
			prompt := strings.TrimSpace(a.input.Value())
			if prompt != "" {
				a.input.Reset()
				if strings.HasPrefix(prompt, "/") {
					return a.handleSlashCommand(prompt)
				}
				return a.startAgent(prompt)
			}
		}
		inp, cmd := a.input.Update(m)
		a.input = inp
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

	return cmd
}

func (a *App) handleAgentEvent(ev agent.Event) tea.Cmd {
	switch ev.Type {
	case agent.EventToken:
		if tok, ok := ev.Payload.(string); ok {
			a.conversation.AppendToken(tok)
		}
		return a.waitForAgentEvent()

	case agent.EventToolCall:
		if tc, ok := ev.Payload.(ai.ToolCall); ok {
			a.conversation.AddToolCall(tc.Name, "")
			a.stats.ToolCallCount++
			a.statusBar.SetStatus(fmt.Sprintf("⚙ %s…", tc.Name))
		}
		return a.waitForAgentEvent()

	case agent.EventToolDone:
		a.statusBar.SetStatus("Thinking…")
		return a.waitForAgentEvent()

	case agent.EventStatus:
		if s, ok := ev.Payload.(string); ok {
			a.statusBar.SetStatus(s)
		}
		return a.waitForAgentEvent()

	case agent.EventDone:
		a.thinking = false
		a.spin.Stop()
		a.statusBar.SetStatus("Ready")
		a.stats.InputTokens = a.sess.TotalInputTokens
		a.stats.OutputTokens = a.sess.TotalOutputTokens
		a.header.SetTokens(a.sess.TotalInputTokens + a.sess.TotalOutputTokens)
		return nil

	case agent.EventError:
		a.thinking = false
		a.spin.Stop()
		if err, ok := ev.Payload.(error); ok {
			a.conversation.AddMessage("assistant", fmt.Sprintf("Error: %v", err), true)
		}
		a.statusBar.SetStatus("Error")
		return nil

	case agent.EventConfirm:
		if payload, ok := ev.Payload.(map[string]any); ok {
			if tc, ok := payload["tool_call"].(ai.ToolCall); ok {
				a.confirm.Show(fmt.Sprintf("Allow %s?", tc.Name))
				if replyCh, ok := payload["reply"].(chan bool); ok {
					a.confirm.SetReply(replyCh)
				}
			}
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

	switch cmd {
	case "/help":
		a.showHelp = true
	case "/clear":
		a.conversation.Clear()
		a.statusBar.SetStatus("Conversation cleared")
	case "/model":
		if len(args) > 0 {
			a.cfg.Model = args[0]
			a.header.SetModel(args[0])
			a.conversation.AddMessage("system", fmt.Sprintf("Model switched to %s", args[0]), false)
		}
	case "/mode":
		if len(args) > 0 && agent.IsValid(args[0]) {
			a.cfg.Mode = args[0]
			a.header.SetMode(args[0])
			a.conversation.AddMessage("system", fmt.Sprintf("Mode switched to %s", args[0]), false)
		}
	case "/sessions":
		a.sessionBrowser.Show()
	case "/diff":
		a.diffPane.Toggle()
		a.layout()
	case "/tree":
		a.showFileTree = !a.showFileTree
		a.layout()
	case "/lsp":
		a.lspPanel.Toggle()
		a.layout()
	case "/stats":
		a.conversation.AddMessage("system", a.stats.View(), false)
	}
	return nil
}

func (a *App) layout() {
	if a.width <= 0 || a.height <= 0 {
		return
	}
	headerH := 4
	statusH := 1
	inputH := a.input.LineCount() + 2
	if inputH < 3 {
		inputH = 3
	}
	if inputH > 10 {
		inputH = 10
	}

	mainH := a.height - headerH - statusH - inputH
	if mainH < 1 {
		mainH = 1
	}

	a.header.SetWidth(a.width)
	a.statusBar.SetWidth(a.width)
	a.input.SetWidth(a.width)

	mainW := a.width
	if a.showFileTree {
		treeW := 25
		if a.width > 80 {
			treeW = a.width / 5
		}
		a.fileTree.SetSize(treeW, mainH)
		mainW = a.width - treeW - 1
	}

	if a.diffPane.Visible() {
		convW := mainW * 4 / 10
		diffW := mainW - convW - 1
		a.conversation.SetSize(convW, mainH)
		a.diffPane.SetSize(diffW, mainH)
	} else {
		a.conversation.SetSize(mainW, mainH)
	}

	a.sessionBrowser.SetSize(a.width*3/4, a.height*3/4)
	a.helpOverlay.SetSize(a.width*3/4, a.height*3/4)
}

func (a *App) View() string {
	if a.width <= 0 || a.height <= 0 {
		return "Initializing..."
	}
	if a.showHelp {
		return a.helpOverlay.View()
	}

	var sb strings.Builder

	headerView := a.header.View()
	sb.WriteString(headerView)
	if headerView != "" {
		sb.WriteString("\n")
	}

	if a.sessionBrowser.Visible() {
		sb.WriteString(a.sessionBrowser.View())
	} else {
		var mainRow string
		convView := a.conversation.View()

		// Style conversation based on focus
		if a.focus == "conversation" {
			convView = a.styles.ActivePane.Width(lipgloss.Width(convView)).Render(convView)
		}

		if a.showFileTree {
			treeView := a.fileTree.View()
			mainRow = lipgloss.JoinHorizontal(lipgloss.Top, treeView, " ", convView)
		} else {
			mainRow = convView
		}

		if a.diffPane.Visible() {
			diffView := a.diffPane.View()
			mainRow = lipgloss.JoinHorizontal(lipgloss.Top, mainRow, " ", diffView)
		}

		if a.lspPanel.Visible() {
			mainRow = lipgloss.JoinVertical(lipgloss.Left, mainRow, a.lspPanel.View())
		}
		sb.WriteString(mainRow)
	}

	sb.WriteByte('\n')

	if a.confirm.Visible() {
		sb.WriteString(lipgloss.PlaceHorizontal(a.width, lipgloss.Center, a.confirm.View()))
	} else if a.thinking {
		sb.WriteString("  " + a.spin.View() + " Thinking...")
	} else {
		sb.WriteString(a.input.View())
	}

	sb.WriteByte('\n')
	sb.WriteString(a.statusBar.View())

	return sb.String()
}

func Run(cfg *config.Config, ag *agent.Agent, sess *session.Session, initialPrompt string) error {
	app := NewApp(cfg, ag, sess, initialPrompt)
	p := tea.NewProgram(app, tea.WithAltScreen(), tea.WithMouseCellMotion())
	_, err := p.Run()
	return err
}
