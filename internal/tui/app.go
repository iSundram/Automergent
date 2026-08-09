package tui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sahilm/fuzzy"

	"github.com/iSundram/Automergent/internal/agent"
	"github.com/iSundram/Automergent/internal/ai"
	"github.com/iSundram/Automergent/internal/config"
	"github.com/iSundram/Automergent/internal/session"
	"github.com/iSundram/Automergent/internal/tui/components"
	"github.com/iSundram/Automergent/internal/tui/keys"
	"github.com/iSundram/Automergent/internal/tui/themes"
)

type agentEventMsg struct{ ev agent.Event }
type modelsFetchedMsg []ai.Model

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
	palette        components.CommandPalette
	width          int
	height         int
	thinking       bool
	statusMsg      string
	showFileTree   bool
	showHelp       bool
	ctx            context.Context
	cancel         context.CancelFunc
	initialPrompt  string
	focus          string

	availableModels []ai.Model
	fetchingModels  bool
}

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
		palette:        components.NewCommandPalette(styles),
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

func (a *App) startAgent(prompt string) tea.Cmd {
	prompt = a.expandPrompt(prompt)
	if strings.HasPrefix(prompt, "!") {
		return a.runShellPassthrough(prompt[1:])
	}
	a.thinking = true
	a.spin.Start()
	a.conversation.AddMessage("user", prompt, false)
	a.statusBar.SetStatus("Thinking…")
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
	case tea.WindowSizeMsg:
		a.width, a.height = m.Width, m.Height
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
	case modelsFetchedMsg:
		a.availableModels = m
		a.fetchingModels = false
		a.updatePalette()
	}
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
	key := m.String()
	if a.palette.Visible() {
		switch key {
		case "enter":
			if sel := a.palette.Selected(); sel != nil {
				a.input.InsertValue(sel.Value)
				a.palette.Hide()
				a.layout()
				return nil
			}
		case "up", "down", "ctrl+p", "ctrl+n", "tab":
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
	case "ctrl+c", "ctrl+d":
		a.cancel()
		return tea.Quit
	case "ctrl+l":
		a.conversation.Clear()
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
		trigger := a.input.TriggerType()
		if trigger != "" {
			a.palette.Show()
			a.updatePalette()
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

	var items []components.PaletteItem
	switch trigger {
	case "help", "command":
		allCmds := []components.PaletteItem{
			{Name: "model", Description: "Switch AI model", Value: "model", Icon: "🤖"},
			{Name: "provider", Description: "Switch provider", Value: "provider", Icon: "🔌"},
			{Name: "mode", Description: "Change approval mode", Value: "mode", Icon: "⚙️"},
			{Name: "clear", Description: "Clear screen", Value: "clear", Icon: "🧹"},
			{Name: "reset", Description: "Reset history", Value: "reset", Icon: "🔄"},
			{Name: "sessions", Description: "Browse sessions", Value: "sessions", Icon: "📁"},
			{Name: "diff", Description: "Toggle diff pane", Value: "diff", Icon: "🔍"},
			{Name: "tree", Description: "Toggle file tree", Value: "tree", Icon: "🌳"},
			{Name: "stats", Description: "Show statistics", Value: "stats", Icon: "📈"},
			{Name: "quit", Description: "Exit Automergent", Value: "quit", Icon: "🚪"},
		}
		items = a.fuzzyFilter(allCmds, filter)

	case "model":
		var modelItems []components.PaletteItem
		for _, m := range a.availableModels {
			modelItems = append(modelItems, components.PaletteItem{
				Name:        m.ID,
				Description: fmt.Sprintf("Model (Limit: %d)", m.ContextLimit),
				Value:       m.ID,
				Icon:        "🤖",
			})
		}
		if len(modelItems) == 0 && a.fetchingModels {
			items = []components.PaletteItem{{Name: "Loading...", Description: "Fetching models from provider", Value: "", Icon: "⏳"}}
		} else {
			items = a.fuzzyFilter(modelItems, filter)
		}

	case "file":
		var fileItems []components.PaletteItem
		for _, item := range a.fileTree.Items() {
			if !item.IsDir {
				fileItems = append(fileItems, components.PaletteItem{
					Name:        item.Name,
					Description: item.Path,
					Value:       item.Path,
					Icon:        "📄",
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
		targets = append(targets, item.Name)
	}
	matches := fuzzy.Find(filter, targets)
	var filtered []components.PaletteItem
	for _, match := range matches {
		filtered = append(filtered, items[match.Index])
	}
	return filtered
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
		if text, ok := ev.Payload.(string); ok && text != "" {
			a.conversation.AddMessage("assistant", text, false)
		}
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
	case "/reset":
		a.conversation.Clear()
		a.sess.Messages = nil
		a.statusBar.SetStatus("History reset")
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
	case "/quit", "/exit":
		return tea.Quit
	}
	return nil
}

func (a *App) layout() {
	if a.width <= 0 || a.height <= 0 {
		return
	}
	headerH, statusH := 4, 1
	inputH := a.input.LineCount() + 2
	if inputH < 3 {
		inputH = 3
	}

	paletteH := 0
	if a.palette.Visible() {
		paletteH = 9
	}

	mainH := a.height - headerH - statusH - inputH - paletteH
	if mainH < 1 {
		mainH = 1
	}

	a.header.SetWidth(a.width)
	a.statusBar.SetWidth(a.width)
	a.input.SetWidth(a.width)
	a.palette.SetWidth(a.width / 2)

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
		if a.focus == "conversation" {
			convView = a.styles.ActivePane.Width(lipgloss.Width(convView)).Render(convView)
		}
		if a.showFileTree {
			mainRow = lipgloss.JoinHorizontal(lipgloss.Top, a.fileTree.View(), " ", convView)
		} else {
			mainRow = convView
		}
		if a.diffPane.Visible() {
			mainRow = lipgloss.JoinHorizontal(lipgloss.Top, mainRow, " ", a.diffPane.View())
		}
		sb.WriteString(mainRow)
	}
	sb.WriteByte('\n')
	if a.palette.Visible() {
		sb.WriteString(lipgloss.PlaceHorizontal(a.width, lipgloss.Center, a.palette.View()) + "\n")
	}
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
