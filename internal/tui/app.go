package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sahilm/fuzzy"
	"gopkg.in/yaml.v3"

	"github.com/iSundram/Automergent/internal/agent"
	"github.com/iSundram/Automergent/internal/ai"
	googleProvider "github.com/iSundram/Automergent/internal/ai/google"
	"github.com/iSundram/Automergent/internal/config"
	gitutil "github.com/iSundram/Automergent/internal/git"
	"github.com/iSundram/Automergent/internal/session"
	"github.com/iSundram/Automergent/internal/tools"
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

	availableModels    []ai.Model
	fetchingModels     bool
	availableProviders []string
	streamedReply      bool
}

func NewApp(cfg *config.Config, ag *agent.Agent, sess *session.Session, initialPrompt string) *App {
	theme := themes.Get(cfg.Theme)
	styles := themes.NewStyles(theme)
	kb := keys.Get(cfg.Keybindings)
	ctx, cancel := context.WithCancel(context.Background())

	app := &App{
		cfg:                cfg,
		ag:                 ag,
		sess:               sess,
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
		sessionBrowser:     components.NewSessionBrowser(styles),
		lspPanel:           components.NewLSPPanel(styles),
		stats:              components.NewStats(styles),
		helpOverlay:        components.NewHelpOverlay(styles),
		fileTree:           components.NewFileTree(styles),
		palette:            components.NewCommandPalette(styles),
		ctx:                ctx,
		cancel:             cancel,
		initialPrompt:      initialPrompt,
		statusMsg:          "Ready",
		focus:              "input",
		availableProviders: []string{"google"},
	}
	sort.Strings(app.availableProviders)
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
	a.streamedReply = false
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
	case components.SessionSelectedMsg:
		if m.Session != nil {
			a.sess = m.Session
			a.conversation.Clear()
			for _, sm := range m.Session.Messages {
				a.conversation.AddMessage(string(sm.Role), sm.TextContent(), false)
			}
			a.stats.InputTokens = m.Session.TotalInputTokens
			a.stats.OutputTokens = m.Session.TotalOutputTokens
			a.header.SetTokens(m.Session.TotalInputTokens + m.Session.TotalOutputTokens)
			a.statusBar.SetStatus("Session loaded")
		}
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
				trigger := a.input.TriggerType()
				if trigger == "command" || trigger == "help" {
					execNoArg := map[string]bool{
						"help": true, "clear": true, "reset": true, "stats": true,
						"tree": true, "diff": true, "lsp": true, "sessions": true,
						"quit": true, "exit": true,
					}
					if execNoArg[sel.Value] {
						a.palette.Hide()
						a.layout()
						return a.handleSlashCommand("/" + sel.Value)
					}
				}
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
		a.sessionBrowser.SetSessions([]*session.Session{a.sess})
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
			{Name: "api-key", Description: "Set API key for active provider", Value: "api-key", Icon: "🔑"},
			{Name: "model-api-key", Description: "Set API key for active model", Value: "model-api-key", Icon: "🧠"},
			{Name: "base-url", Description: "Set base URL for active provider", Value: "base-url", Icon: "🌐"},
			{Name: "provider-api-key", Description: "Set API key for a provider", Value: "provider-api-key", Icon: "🔐"},
			{Name: "provider-model-api-key", Description: "Set API key for provider+model", Value: "provider-model-api-key", Icon: "🧩"},
			{Name: "provider-base-url", Description: "Set base URL for a provider", Value: "provider-base-url", Icon: "🔗"},
			{Name: "clear", Description: "Clear screen", Value: "clear", Icon: "🧹"},
			{Name: "reset", Description: "Reset history", Value: "reset", Icon: "🔄"},
			{Name: "sessions", Description: "Browse sessions", Value: "sessions", Icon: "📁"},
			{Name: "diff", Description: "Toggle diff pane", Value: "diff", Icon: "🔍"},
			{Name: "tree", Description: "Toggle file tree", Value: "tree", Icon: "🌳"},
			{Name: "lsp", Description: "Toggle LSP pane", Value: "lsp", Icon: "📐"},
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
	case "provider":
		var providerItems []components.PaletteItem
		for _, p := range a.availableProviders {
			providerItems = append(providerItems, components.PaletteItem{
				Name: p, Description: "AI provider", Value: p, Icon: "🔌",
			})
		}
		items = a.fuzzyFilter(providerItems, filter)

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
			if strings.TrimSpace(tok) != "" {
				a.streamedReply = true
			}
		}
		return a.waitForAgentEvent()
	case agent.EventToolCall:
		if tc, ok := ev.Payload.(ai.ToolCall); ok {
			argText := ""
			if len(tc.Args) > 0 {
				if b, err := json.Marshal(tc.Args); err == nil {
					argText = string(b)
				}
			}
			a.conversation.AddToolCall(tc.Name, argText)
			a.stats.ToolCallCount++
			a.statusBar.SetStatus(fmt.Sprintf("⚙ %s…", tc.Name))
		}
		return a.waitForAgentEvent()
	case agent.EventToolDone:
		if r, ok := ev.Payload.(tools.Result); ok {
			if r.IsError {
				a.conversation.AddMessage("assistant", "Tool error: "+r.Content, true)
			} else if strings.TrimSpace(r.Content) != "" {
				a.conversation.AddMessage("tool_result", truncateUIContent(r.Content, a.conversation.ReviewMode()), false)
			}
		}
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
		if text, ok := ev.Payload.(string); ok && strings.TrimSpace(text) != "" && !a.streamedReply {
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
		a.statusBar.SetStatus("Provider updated")
		_ = a.persistProjectConfig()
	case "/model":
		if len(args) > 0 {
			if err := a.switchProvider(a.cfg.Provider, args[0]); err != nil {
				a.conversation.AddMessage("assistant", fmt.Sprintf("Error: %v", err), true)
				a.statusBar.SetStatus("Error")
				return nil
			}
			a.conversation.AddMessage("system", fmt.Sprintf("Model switched to %s", args[0]), false)
			a.statusBar.SetStatus("Model updated")
			_ = a.persistProjectConfig()
		}
	case "/mode":
		if len(args) > 0 && agent.IsValid(args[0]) {
			a.cfg.Mode = args[0]
			a.header.SetMode(args[0])
			a.conversation.AddMessage("system", fmt.Sprintf("Mode switched to %s", args[0]), false)
			_ = a.persistProjectConfig()
		} else {
			a.conversation.AddMessage("assistant", "Error: usage /mode <suggest|auto-edit|full-auto|plan>", true)
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
	case "/model-api-key":
		if len(args) == 0 {
			a.conversation.AddMessage("assistant", "Error: usage /model-api-key <value>", true)
			return nil
		}
		a.ensureProviderConfig(a.cfg.Provider)
		pc := a.cfg.Providers[a.cfg.Provider]
		if pc.Models == nil {
			pc.Models = map[string]config.ModelConfig{}
		}
		mc := pc.Models[a.cfg.Model]
		mc.APIKey = strings.Join(args, " ")
		pc.Models[a.cfg.Model] = mc
		a.cfg.Providers[a.cfg.Provider] = pc
		if err := a.switchProvider(a.cfg.Provider, a.cfg.Model); err != nil {
			a.conversation.AddMessage("assistant", fmt.Sprintf("Error: %v", err), true)
			return nil
		}
		a.conversation.AddMessage("system", fmt.Sprintf("API key updated for %s/%s", a.cfg.Provider, a.cfg.Model), false)
		a.statusBar.SetStatus("Model API key updated")
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
	case "/provider-model-api-key":
		if len(args) < 3 {
			a.conversation.AddMessage("assistant", "Error: usage /provider-model-api-key <provider> <model> <value>", true)
			return nil
		}
		provider := args[0]
		model := args[1]
		if !isSupportedProvider(provider) {
			a.conversation.AddMessage("assistant", fmt.Sprintf("Error: unknown provider %q", provider), true)
			return nil
		}
		a.ensureProviderConfig(provider)
		pc := a.cfg.Providers[provider]
		if pc.Models == nil {
			pc.Models = map[string]config.ModelConfig{}
		}
		mc := pc.Models[model]
		mc.APIKey = strings.Join(args[2:], " ")
		pc.Models[model] = mc
		a.cfg.Providers[provider] = pc
		if provider == a.cfg.Provider && model == a.cfg.Model {
			if err := a.switchProvider(a.cfg.Provider, a.cfg.Model); err != nil {
				a.conversation.AddMessage("assistant", fmt.Sprintf("Error: %v", err), true)
				return nil
			}
		}
		a.conversation.AddMessage("system", fmt.Sprintf("API key updated for %s/%s", provider, model), false)
		a.statusBar.SetStatus("Provider/model API key updated")
		_ = a.persistProjectConfig()
	case "/sessions":
		a.sessionBrowser.SetSessions([]*session.Session{a.sess})
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
		a.stats.InputTokens = a.sess.TotalInputTokens
		a.stats.OutputTokens = a.sess.TotalOutputTokens
		a.conversation.AddMessage("system", a.stats.View(), false)
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
		convW := mainW * 45 / 100
		diffW := mainW - convW - 1
		if a.lspPanel.Visible() {
			diffW = diffW * 60 / 100
			lspW := mainW - convW - diffW - 2
			if lspW < 20 {
				lspW = 20
				diffW = mainW - convW - lspW - 2
			}
			a.lspPanel.SetSize(lspW, mainH)
		}
		a.conversation.SetSize(convW, mainH)
		a.diffPane.SetSize(diffW, mainH)
	} else {
		if a.lspPanel.Visible() {
			convW := mainW * 70 / 100
			lspW := mainW - convW - 1
			a.conversation.SetSize(convW, mainH)
			a.lspPanel.SetSize(lspW, mainH)
		} else {
			a.conversation.SetSize(mainW, mainH)
		}
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
		if a.lspPanel.Visible() {
			mainRow = lipgloss.JoinHorizontal(lipgloss.Top, mainRow, " ", a.lspPanel.View())
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

func (a *App) persistProjectConfig() error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	root, err := gitutil.RootDir(a.ctx, cwd)
	if err != nil || root == "" {
		return nil
	}
	automergentDir := filepath.Join(root, ".automergent")
	if err := os.MkdirAll(automergentDir, 0o700); err != nil {
		return err
	}

	doc := map[string]any{
		"provider": a.cfg.Provider,
		"model":    a.cfg.Model,
		"mode":     a.cfg.Mode,
	}
	if len(a.cfg.Providers) > 0 {
		providers := map[string]map[string]any{}
		for name, p := range a.cfg.Providers {
			entry := map[string]any{}
			if p.APIKey != "" {
				entry["apiKey"] = p.APIKey
			}
			if p.BaseURL != "" {
				entry["baseUrl"] = p.BaseURL
			}
			if p.OrgID != "" {
				entry["orgId"] = p.OrgID
			}
			if len(p.Models) > 0 {
				models := map[string]map[string]any{}
				for model, mc := range p.Models {
					modelEntry := map[string]any{}
					if mc.APIKey != "" {
						modelEntry["apiKey"] = mc.APIKey
					}
					if mc.BaseURL != "" {
						modelEntry["baseUrl"] = mc.BaseURL
					}
					if len(modelEntry) > 0 {
						models[model] = modelEntry
					}
				}
				if len(models) > 0 {
					entry["models"] = models
				}
			}
			if len(entry) > 0 {
				providers[name] = entry
			}
		}
		if len(providers) > 0 {
			doc["providers"] = providers
		}
	}

	data, err := yaml.Marshal(doc)
	if err != nil {
		return err
	}
	return atomicWriteFile(filepath.Join(automergentDir, "config.yaml"), data, 0o600)
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

func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".automergent-cfg-tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() {
		tmp.Close()
		_ = os.Remove(tmpPath)
	}()
	if _, err := tmp.Write(data); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpPath, perm); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
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
	p, err := buildProviderFromConfig(a.cfg)
	if err != nil {
		a.cfg.Provider = oldProvider
		a.cfg.Model = oldModel
		return err
	}
	a.ag.SetProvider(p)
	a.header.SetProvider(a.cfg.Provider)
	if a.cfg.Model != "" {
		a.header.SetModel(a.cfg.Model)
	}
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

func buildProviderFromConfig(cfg *config.Config) (ai.Provider, error) {
	pc := cfg.Providers[cfg.Provider]
	if mc, ok := pc.Models[cfg.Model]; ok {
		if mc.APIKey != "" {
			pc.APIKey = mc.APIKey
		}
		if mc.BaseURL != "" {
			pc.BaseURL = mc.BaseURL
		}
	}
	aiCfg := ai.ProviderConfig{
		APIKey:       pc.APIKey,
		BaseURL:      pc.BaseURL,
		DefaultModel: cfg.Model,
		OrgID:        pc.OrgID,
	}

	switch cfg.Provider {
	case "google":
		return googleProvider.New(aiCfg), nil
	default:
		if p, ok := ai.Get(cfg.Provider); ok {
			return p, nil
		}
		return nil, fmt.Errorf("unknown provider %q", cfg.Provider)
	}
}
