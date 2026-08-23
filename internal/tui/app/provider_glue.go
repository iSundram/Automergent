package app

// Provider/model configuration glue.
// Moved verbatim from internal/tui/app.go.

import (
	tea "charm.land/bubbletea/v2"
	"fmt"
	"github.com/iSundram/Automergent/internal/config"
	"github.com/iSundram/Automergent/internal/tui/themes"
	"image/color"
	"os"
)

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

func (a *App) persistProjectConfig() error {
	return a.cfg.SaveIfLoaded()
}

// handleBackgroundColor reacts to the terminal's reported background color:
// with AUTOMERGENT_AUTO_THEME=1 the theme switches to a matching light/dark
// variant; otherwise the user just gets a one-time hint.
func (a *App) handleBackgroundColor(c color.Color) {
	if c == nil || a.theme == nil || a.theme.Background == nil {
		return
	}
	termDark := themes.IsDark(c)
	themeDark := themes.IsDark(a.theme.Background)
	if termDark == themeDark {
		return
	}
	if os.Getenv("AUTOMERGENT_AUTO_THEME") != "1" {
		return // respect the user's explicit choice; no nagging either
	}
	target := "solarized-light"
	if termDark {
		target = "catppuccin"
	}
	if err := a.SetTheme(target); err == nil {
		a.conversation.Invalidate()
		a.conversation.AddMessage("system", fmt.Sprintf("Auto-switched to %s theme to match your terminal", target), false)
	}
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

// currentProposalID tracks which pending edit is shown in the diff pane.
