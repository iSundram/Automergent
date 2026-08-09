package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/iSundram/Automergent/internal/agent"
	"github.com/iSundram/Automergent/internal/config"
	"github.com/iSundram/Automergent/internal/session"
	"github.com/iSundram/Automergent/internal/tui/components"
	"github.com/iSundram/Automergent/internal/tui/keys"
	"github.com/iSundram/Automergent/internal/tui/themes"
)

// agentTokenMsg carries a streamed token from the agent.
type agentTokenMsg struct{ token string }

// agentDoneMsg signals the agent finished.
type agentDoneMsg struct{ err error }

// agentToolCallMsg signals a tool call.
type agentToolCallMsg struct{ name string }

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
	width          int
	height         int
	thinking       bool
	ctx            context.Context
	cancel         context.CancelFunc
	initialPrompt  string
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
		ctx:            ctx,
		cancel:         cancel,
		initialPrompt:  initialPrompt,
	}
	app.header.SetModel(cfg.Model)
	app.header.SetMode(cfg.Mode)
	return app
}

// Init is the initial command.
func (a *App) Init() tea.Cmd {
	cmds := []tea.Cmd{
		a.input.Focus(),
		a.spin.Tick(),
	}
	if a.initialPrompt != "" {
		cmds = append(cmds, a.runAgent(a.initialPrompt))
	}
	return tea.Batch(cmds...)
}

func (a *App) runAgent(prompt string) tea.Cmd {
	a.thinking = true
	a.spin.Start()
	a.conversation.AddMessage("user", prompt, false)
	a.statusBar.SetStatus("Thinking…")
	return func() tea.Msg {
		err := a.ag.Run(a.ctx, prompt)
		return agentDoneMsg{err: err}
	}
}

// Update handles messages.
func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = m.Width
		a.height = m.Height
		a.layout()

	case tea.KeyMsg:
		if a.confirm.Visible() {
			c, cmd := a.confirm.Update(msg)
			a.confirm = c
			return a, cmd
		}
		if a.sessionBrowser.Visible() {
			sb, cmd := a.sessionBrowser.Update(msg)
			a.sessionBrowser = sb
			return a, cmd
		}
		switch {
		case m.String() == "ctrl+c" || m.String() == "ctrl+q":
			a.cancel()
			return a, tea.Quit

		case m.String() == "ctrl+d":
			a.diffPane.Toggle()

		case m.String() == "ctrl+l":
			a.lspPanel.Toggle()

		case m.String() == "ctrl+s":
			a.sessionBrowser.Show()

		case m.String() == "enter" && !a.thinking:
			prompt := strings.TrimSpace(a.input.Value())
			if prompt != "" {
				a.input.Reset()
				cmds = append(cmds, a.runAgent(prompt))
			}

		default:
			inp, cmd := a.input.Update(msg)
			a.input = inp
			cmds = append(cmds, cmd)
		}

	case agentTokenMsg:
		a.conversation.AppendToken(m.token)

	case agentToolCallMsg:
		a.conversation.AddMessage("tool_call", "⚙ "+m.name, false)
		a.stats.ToolCallCount++

	case agentDoneMsg:
		a.thinking = false
		a.spin.Stop()
		if m.err != nil {
			a.conversation.AddMessage("assistant", fmt.Sprintf("Error: %v", m.err), true)
			a.statusBar.SetStatus("Error")
		} else {
			a.statusBar.SetStatus("Ready")
		}

	case components.ConfirmMsg:
		// handled by confirm component

	case components.SessionSelectedMsg:
		a.sess = m.Session

	case spinner.TickMsg:
		sp, cmd := a.spin.Update(msg)
		a.spin = sp
		cmds = append(cmds, cmd)
	}

	// Forward to conversation for scrolling
	conv, cmd := a.conversation.Update(msg)
	a.conversation = conv
	cmds = append(cmds, cmd)

	return a, tea.Batch(cmds...)
}

func (a *App) layout() {
	headerH := 1
	statusH := 1
	inputH := 5
	mainH := a.height - headerH - statusH - inputH

	a.header.SetWidth(a.width)
	a.statusBar.SetWidth(a.width)
	a.input.SetWidth(a.width)

	if a.diffPane.Visible() {
		convW := a.width * 2 / 3
		diffW := a.width - convW
		a.conversation.SetSize(convW, mainH)
		a.diffPane.SetSize(diffW, mainH)
	} else {
		a.conversation.SetSize(a.width, mainH)
	}
}

// View renders the full TUI.
func (a *App) View() string {
	var sb strings.Builder

	sb.WriteString(a.header.View())
	sb.WriteByte('\n')

	if a.sessionBrowser.Visible() {
		sb.WriteString(a.sessionBrowser.View())
	} else {
		mainRow := a.conversation.View()
		if a.diffPane.Visible() {
			mainRow = lipgloss.JoinHorizontal(lipgloss.Top, mainRow, a.diffPane.View())
		}
		if a.lspPanel.Visible() {
			mainRow = lipgloss.JoinVertical(lipgloss.Left, mainRow, a.lspPanel.View())
		}
		sb.WriteString(mainRow)
	}

	sb.WriteByte('\n')

	// Input area
	if a.confirm.Visible() {
		sb.WriteString(a.confirm.View())
	} else if a.thinking {
		sb.WriteString(a.spin.View())
	} else {
		sb.WriteString(a.input.View())
	}

	sb.WriteByte('\n')
	sb.WriteString(a.statusBar.View())

	return sb.String()
}

// Run starts the Bubble Tea program.
func Run(cfg *config.Config, ag *agent.Agent, sess *session.Session, initialPrompt string) error {
	app := NewApp(cfg, ag, sess, initialPrompt)
	p := tea.NewProgram(app, tea.WithAltScreen(), tea.WithMouseCellMotion())
	_, err := p.Run()
	return err
}
