package app

// Provider/model configuration glue.
// Moved verbatim from internal/tui/app.go.

import (
	tea "charm.land/bubbletea/v2"
	"fmt"
	"image/color"
	"os"
	"time"

	"github.com/iSundram/Automergent/internal/ai"
	"github.com/iSundram/Automergent/internal/config"
	"github.com/iSundram/Automergent/internal/tui/themes"
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

// refreshModels forces a live re-fetch of the active provider's model list,
// bypassing any cached results. Used by /model refresh.
func (a *App) refreshModels() tea.Cmd {
	if a.fetchingModels {
		return nil
	}
	a.fetchingModels = true
	// Try to invalidate the client-side cache when the provider exposes the
	// optional LiveModels-capable interface.
	if lc, ok := a.ag.Provider().(interface {
		InvalidateModelsCache()
	}); ok {
		lc.InvalidateModelsCache()
	}
	return func() tea.Msg {
		models, _ := a.ag.Provider().Models(a.ctx)
		return modelsFetchedMsg(models)
	}
}

// modelsAvailable returns the cached live model list enriched with user-
// registered custom models from the config.
func (a *App) modelsAvailable() []ai.Model {
	models := append([]ai.Model{}, a.availableModels...)
	pc := a.cfg.Providers[a.cfg.Provider]
	if pc.Models == nil {
		return models
	}
	seen := make(map[string]bool, len(models))
	for _, m := range models {
		seen[m.ID] = true
	}
	for id, mc := range pc.Models {
		if seen[id] {
			continue
		}
		m := ai.Model{
			ID:           id,
			Name:         mc.DisplayName,
			ContextLimit: mc.ContextLimit,
			InputPrice:   mc.InputPrice,
			OutputPrice:  mc.OutputPrice,
		}
		if m.Name == "" {
			m.Name = id
		}
		models = append(models, m)
	}
	return models
}

// testProvider builds a provider from its stored config (without switching
// the active provider) and runs a live connectivity check. The outcome is
// delivered asynchronously as a system message via a tea.Cmd.
func (a *App) testProvider(provider string) tea.Cmd {
	spec, _ := config.ProviderSpecFor(provider)
	pc := a.cfg.Providers[provider]
	authSource := config.ProviderAPIKeySource(a.cfg, provider)
	if authSource == "" {
		authSource = "not set"
	}
	display := provider
	if spec.DisplayName != "" {
		display = spec.DisplayName
	}
	return func() tea.Msg {
		start := time.Now()
		model := pc.DefaultModel
		if model == "" {
			model = config.DefaultModelFor(provider)
		}
		p, err := buildProviderForConfig(a.cfg, provider, model)
		if err != nil {
			return providerTestMsg{
				provider: provider,
				display:  display,
				success:  false,
				message:  fmt.Sprintf("✗ %s — could not build provider: %v", display, err),
			}
		}
		if _, err := p.Models(a.ctx); err != nil {
			return providerTestMsg{
				provider: provider,
				display:  display,
				success:  false,
				message:  fmt.Sprintf("✗ %s — model listing failed: %v", display, err),
			}
		}
		elapsed := time.Since(start).Truncate(time.Millisecond)
		bk := config.EffectiveBackend(provider, pc)
		return providerTestMsg{
			provider: provider,
			display:  display,
			success:  true,
			message:  fmt.Sprintf("✓ %s reachable — models listed in %s (backend: %s, key: %s)", display, elapsed, bk, authSource),
		}
	}
}

// providerTestMsg is the async result of /provider test.
type providerTestMsg struct {
	provider string
	display  string
	success  bool
	message  string
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
	a.ensureProviderConfig(provider)
	oldProvider, oldModel := a.cfg.Provider, a.cfg.Model
	a.cfg.Provider = provider
	if model == "" {
		model = a.perProviderDefaultModel(provider)
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

// perProviderDefaultModel resolves the model to use when none is specified:
// remembered per-provider default (set by /model switch), then catalog
// default, then "" to let the provider client pick.
func (a *App) perProviderDefaultModel(provider string) string {
	if pc, ok := a.cfg.Providers[provider]; ok && pc.DefaultModel != "" {
		return pc.DefaultModel
	}
	return config.DefaultModelFor(provider)
}

// currentProposalID tracks which pending edit is shown in the diff pane.
