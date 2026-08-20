package tui

import (
	"context"
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/iSundram/Automergent/internal/ai"
	"github.com/iSundram/Automergent/internal/config"
	"github.com/iSundram/Automergent/internal/tui/keys"
	"github.com/iSundram/Automergent/internal/tui/themes"
)

// commandHost implements command.Host interface for the App.
// This provides the command package access to App functionality without import cycles.
type commandHost struct {
	app *App
}

func (h *commandHost) AddSystemMessage(text string) {
	h.app.conversation.AddMessage("system", text, false)
}

func (h *commandHost) AddAssistantMessage(text string) {
	h.app.conversation.AddMessage("assistant", text, true)
}

func (h *commandHost) SetStatus(status string) {
	h.app.statusBar.SetStatus(status)
}

func (h *commandHost) CommandUsage(usage string) {
	h.app.commandUsage(usage)
}

func (h *commandHost) CommandError(message string) {
	h.app.commandError(message)
}

// commandUsage and commandError are implemented on *App for the Host interface.
func (a *App) commandUsage(usage string) {
	a.statusBar.SetStatus("Command needs more information")
	a.conversation.AddMessage("system", "Usage: "+usage, false)
}

func (a *App) commandError(message string) {
	a.statusBar.SetStatus("Command not applied")
	a.conversation.AddMessage("assistant", "Error: "+message, true)
}

func (h *commandHost) StartAgent(prompt string) tea.Cmd {
	return h.app.startAgent(prompt)
}

func (h *commandHost) CancelActiveRun(status string) {
	h.app.cancelActiveRun(status)
}

func (h *commandHost) Thinking() bool {
	return h.app.thinking
}

func (h *commandHost) CompactContext() tea.Cmd {
	return h.app.compactContext()
}

func (h *commandHost) Provider() string {
	return h.app.cfg.Provider
}

func (h *commandHost) Model() string {
	return h.app.cfg.Model
}

func (h *commandHost) Providers() []string {
	return h.app.availableProviders
}

func (h *commandHost) SwitchProvider(provider, model string) error {
	return h.app.switchProvider(provider, model)
}

func (h *commandHost) FetchModels() tea.Cmd {
	return h.app.fetchModels()
}

func (h *commandHost) ModelsAvailable() []ai.Model {
	return h.app.availableModels
}

func (h *commandHost) InputTokens() int {
	return h.app.sess.TotalInputTokens
}

func (h *commandHost) OutputTokens() int {
	return h.app.sess.TotalOutputTokens
}

func (h *commandHost) TotalCost() float64 {
	if tel := h.app.ag.Telemetry(); tel != nil {
		return tel.GetCostSummary().TotalCostUSD
	}
	return 0
}

func (h *commandHost) ShowContextDetail() {
	h.app.showContextDetail()
}

func (h *commandHost) ActiveTokens() int {
	if h.app.ag == nil {
		return 0
	}
	mgr := h.app.ag.ContextManager()
	if mgr == nil {
		return 0
	}
	calc := mgr.AdaptiveCalculator()
	if calc == nil {
		return 0
	}
	if h.app.sess == nil {
		return 0
	}
	active := calc.EstimateMessages(h.app.sess.Messages)
	if pending := h.app.input.Value(); pending != "" {
		active += calc.Estimate(pending)
	}
	return active
}

func (h *commandHost) EnsureProviderConfig(provider string) {
	h.app.ensureProviderConfig(provider)
}

func (h *commandHost) ProviderConfig(provider string) config.ProviderConfig {
	if h.app.cfg.Providers == nil {
		return config.ProviderConfig{}
	}
	return h.app.cfg.Providers[provider]
}

func (h *commandHost) SetProviderConfig(provider string, pc config.ProviderConfig) {
	if h.app.cfg.Providers == nil {
		h.app.cfg.Providers = map[string]config.ProviderConfig{}
	}
	h.app.cfg.Providers[provider] = pc
}

func (h *commandHost) PersistProjectConfig() error {
	return h.app.persistProjectConfig()
}

func (h *commandHost) Mode() string {
	return h.app.cfg.Mode
}

func (h *commandHost) SetMode(mode string) {
	h.app.cfg.Mode = mode
	h.app.header.SetMode(mode)
}

func (h *commandHost) ToggleFileTree() {
	h.app.showFileTree = !h.app.showFileTree
	h.app.layout()
}

func (h *commandHost) ToggleDiffPane() {
	h.app.diffPane.Toggle()
	h.app.layout()
}

func (h *commandHost) ToggleLSPPanel() {
	h.app.lspPanel.Toggle()
	h.app.layout()
}

func (h *commandHost) ToggleReviewMode() {
	h.app.conversation.SetReviewMode(!h.app.conversation.ReviewMode())
}

func (h *commandHost) IsReviewMode() bool {
	return h.app.conversation.ReviewMode()
}

func (h *commandHost) SearchWorkspace(query string) string {
	return h.app.searchWorkspace(query)
}

func (h *commandHost) ExportConversation(path string) error {
	return h.app.exportConversation(path)
}

func (h *commandHost) HandleApprovalsCommand(args []string) {
	h.app.handleApprovalsCommand(args)
}

func (h *commandHost) ShowStats() {
	h.app.stats.InputTokens = h.app.sess.TotalInputTokens
	h.app.stats.OutputTokens = h.app.sess.TotalOutputTokens
	if tel := h.app.ag.Telemetry(); tel != nil {
		h.app.stats.TotalCost = tel.GetCostSummary().TotalCostUSD
	}
	h.app.conversation.AddMessage("system", h.app.stats.View(), false)
}

func (h *commandHost) ShowHelp() {
	h.app.showHelp = true
}

func (h *commandHost) SetTheme(name string) error {
	theme := themes.Get(name)
	if theme.Name != name && name != "catppuccin" {
		return fmt.Errorf("unknown theme %q", name)
	}
	*h.app.styles = *themes.NewStyles(theme)
	h.app.theme = theme
	h.app.cfg.Theme = name
	return nil
}

func (h *commandHost) AvailableThemes() []string {
	return []string{"catppuccin", "dracula", "nord", "modern"}
}

func (h *commandHost) CurrentTheme() string {
	return h.app.cfg.Theme
}

func (h *commandHost) SetKeybindings(scheme string) error {
	h.app.keys = keys.Get(scheme)
	h.app.cfg.Keybindings = scheme
	return nil
}

func (h *commandHost) AvailableKeybindings() []string {
	return []string{"default", "vim", "emacs"}
}

func (h *commandHost) CurrentKeybindings() string {
	return h.app.cfg.Keybindings
}

func (h *commandHost) Context() context.Context {
	return h.app.ctx
}
