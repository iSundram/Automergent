package components

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/iSundram/Automergent/internal/config"
	"github.com/iSundram/Automergent/internal/tui/themes"
)

// ProviderStudioTab represents the active tab in the studio.
type ProviderStudioTab int

const (
	TabOverview ProviderStudioTab = iota
	TabFallback
	TabAddProvider
)

// ProviderStudioHost is the interface the ProviderStudio needs from the App.
type ProviderStudioHost interface {
	Providers() []string
	ProviderConfig(name string) config.ProviderConfig
	SetProviderConfig(name string, pc config.ProviderConfig)
	EnsureProviderConfig(name string)
	Provider() string
	Model() string
	ProviderAuthSource(name string) string
	ProviderFallbacks() []config.FallbackProvider
	SetProviderFallbacks([]config.FallbackProvider)
	SwitchProvider(provider, model string) error
	PersistProjectConfig() error
	FetchModels() tea.Cmd
	SetStatus(status string)
	CommandError(message string)
	AddSystemMessage(text string)
}

// ProviderStudio is a full-page provider management studio.
type ProviderStudio struct {
	styles         *themes.Styles
	width          int
	height         int
	visible        bool
	tab            ProviderStudioTab
	overviewScroll int
	fallbackCursor int
	addStep        int
	addName        string
	addApiType     string
	addBaseURL     string
	addModelApiURL string
	addAPIKey      string
	addHeaders     string
	addStepCursor  int
	host           ProviderStudioHost
}

func NewProviderStudio(styles *themes.Styles) *ProviderStudio {
	return &ProviderStudio{
		styles:      styles,
		tab:         TabOverview,
		addApiType:  "openai",
		addModelApiURL: "/v1/models",
	}
}

func (p *ProviderStudio) SetHost(host ProviderStudioHost) {
	p.host = host
}

func (p *ProviderStudio) SetSize(w, h int) { p.width, p.height = w, h }

func (p *ProviderStudio) Show() {
	p.visible = true
	p.overviewScroll = 0
	p.fallbackCursor = 0
	p.addStep = 0
	p.addName = ""
	p.addApiType = "openai"
	p.addModelApiURL = "/v1/models"
	p.addAPIKey = ""
	p.addHeaders = ""
	p.addStepCursor = 0
}

func (p *ProviderStudio) Hide()               { p.visible = false }
func (p *ProviderStudio) Visible() bool       { return p.visible }
func (p *ProviderStudio) Title() string       { return "Provider Studio" }

func (p *ProviderStudio) Update(msg tea.Msg) (*ProviderStudio, tea.Cmd) {
	if !p.visible {
		return p, nil
	}
	if m, ok := msg.(tea.KeyMsg); ok {
		switch m.String() {
		case "esc":
			p.Hide()
			return p, nil
		case "tab", "shift+tab":
			if m.String() == "tab" {
				p.tab = (p.tab + 1) % 3
			} else {
				p.tab = (p.tab + 2) % 3
			}
			p.overviewScroll = 0
			p.fallbackCursor = 0
			p.addStep = 0
			return p, nil
		case "left", "right":
			if p.tab == TabOverview || p.tab == TabFallback {
				return p, nil
			}
			if m.String() == "left" && p.addStep > 0 {
				p.addStep--
			} else if m.String() == "right" && p.addStep < 9 {
				p.addStep++
			}
			return p, nil
		case "up", "down":
			if p.tab == TabOverview {
				step := 1
				if m.String() == "down" {
					p.overviewScroll = min(p.overviewScroll+step, 100)
				} else {
					p.overviewScroll = max(p.overviewScroll-step, 0)
				}
			} else if p.tab == TabFallback {
				fps := p.host.ProviderFallbacks()
				if len(fps) > 0 {
					if m.String() == "down" {
						p.fallbackCursor = min(p.fallbackCursor+1, len(p.host.ProviderFallbacks()))
					} else {
						p.fallbackCursor = max(p.fallbackCursor-1, 0)
					}
				}
			}
			return p, nil
		case "enter":
			if p.tab == TabAddProvider {
				p.handleAddStepEnter()
			}
			return p, nil
		case "backspace":
			if p.tab == TabAddProvider {
				p.handleAddStepBackspace()
			}
			return p, nil
		default:
			if p.tab == TabAddProvider {
				p.handleAddStepInput(m.String())
			}
		}
	}
	return p, nil
}

func (p *ProviderStudio) handleAddStepEnter() {
	switch p.addStep {
	case 0: // Name
		if p.addName != "" {
			p.addStep = 1
		}
	case 1: // ApiType
		p.addStep = 2
	case 2: // BaseURL
		if p.addBaseURL != "" {
			p.addStep = 3
		}
	case 3: // ModelApiURL
		if p.addModelApiURL != "" {
			p.addStep = 4
		}
	case 4: // APIKey
		if p.addAPIKey != "" {
			p.addStep = 5
		}
	case 5: // Headers (optional)
		p.addStep = 6
	case 6: // Test
		p.testProvider()
		p.addStep = 7
	case 7: // Fetch models
		p.fetchModels()
		p.addStep = 8
	case 8: // Fallback toggle
		p.addStep = 9
	case 9: // Save
		p.saveProvider()
	}
}

func (p *ProviderStudio) handleAddStepBackspace() {
	switch p.addStep {
	case 1:
		if len(p.addApiType) > 0 {
			p.addApiType = p.addApiType[:len(p.addApiType)-1]
		} else {
			p.addStep = 0
		}
	case 2:
		if len(p.addBaseURL) > 0 {
			p.addBaseURL = p.addBaseURL[:len(p.addBaseURL)-1]
		} else {
			p.addStep = 1
		}
	case 3:
		if len(p.addModelApiURL) > 0 {
			p.addModelApiURL = p.addModelApiURL[:len(p.addModelApiURL)-1]
		} else {
			p.addStep = 2
		}
	case 4:
		if len(p.addAPIKey) > 0 {
			p.addAPIKey = p.addAPIKey[:len(p.addAPIKey)-1]
		} else {
			p.addStep = 3
		}
	case 5:
		if len(p.addHeaders) > 0 {
			p.addHeaders = p.addHeaders[:len(p.addHeaders)-1]
		} else {
			p.addStep = 4
		}
	case 0:
		if len(p.addName) > 0 {
			p.addName = p.addName[:len(p.addName)-1]
		}
	}
}

func (p *ProviderStudio) handleAddStepInput(key string) {
	switch p.addStep {
	case 0: // Name
		if len(key) == 1 && key >= " " && key <= "~" {
			p.addName += key
		}
	case 2: // BaseURL
		if len(key) == 1 && key >= " " && key <= "~" {
			p.addBaseURL += key
		}
	case 3: // ModelApiURL
		if len(key) == 1 && key >= " " && key <= "~" {
			p.addModelApiURL += key
		}
	case 4: // APIKey
		if len(key) == 1 && key >= " " && key <= "~" {
			p.addAPIKey += key
		}
	case 5: // Headers
		if len(key) == 1 && key >= " " && key <= "~" {
			p.addHeaders += key
		}
	}
}

func (p *ProviderStudio) testProvider() {
	if p.host == nil || p.addName == "" {
		return
	}
	p.host.SetStatus(fmt.Sprintf("Testing %s... (would test %s at %s)", p.addName, p.addApiType, p.addBaseURL))
}

func (p *ProviderStudio) fetchModels() {
	if p.host == nil {
		return
	}
	p.host.SetStatus(fmt.Sprintf("Fetching models for %s...", p.addName))
	p.host.FetchModels()
}

func (p *ProviderStudio) saveProvider() {
	if p.host == nil || p.addName == "" {
		return
	}
	pc := config.ProviderConfig{
		APIKey:      p.addAPIKey,
		BaseURL:     p.addBaseURL,
		ModelApiUrl: p.addModelApiURL,
		ApiType:     p.addApiType,
		DefaultModel: "",
	}
	p.host.EnsureProviderConfig(p.addName)
	pc2 := p.host.ProviderConfig(p.addName)
	pc2.APIKey = pc.APIKey
	pc2.BaseURL = pc.BaseURL
	pc2.ModelApiUrl = pc.ModelApiUrl
	pc2.ApiType = pc.ApiType
	p.host.SetProviderConfig(p.addName, pc2)
	if err := p.host.PersistProjectConfig(); err != nil {
		p.host.CommandError(err.Error())
		return
	}
	p.host.SetStatus(fmt.Sprintf("Provider %s saved", p.addName))
	p.addName = ""
	p.addApiType = "openai"
	p.addBaseURL = ""
	p.addModelApiURL = "/v1/models"
	p.addAPIKey = ""
	p.addHeaders = ""
	p.addStep = 0
}

func (p *ProviderStudio) View() string {
	if !p.visible || p.width <= 0 || p.height <= 0 {
		return ""
	}
	w := max(12, p.width)

	// Tab bar
	tabs := []string{"Overview", "Fallback", "Add Provider"}
	var tabBar string
	for i, t := range tabs {
		style := lipgloss.NewStyle().Foreground(p.styles.T.Muted).Padding(0, 1)
		if i == int(p.tab) {
			style = lipgloss.NewStyle().Foreground(p.styles.T.Accent).Bold(true).Padding(0, 1)
		}
		tabBar += style.Render(t) + " "
	}

	rule := lipgloss.NewStyle().Foreground(p.styles.T.BorderNormal).Render(strings.Repeat("─", w))
	header := lipgloss.JoinHorizontal(lipgloss.Left,
		lipgloss.NewStyle().Foreground(p.styles.T.Accent).Bold(true).Render("  ●  "),
		lipgloss.NewStyle().Foreground(p.styles.T.Text).Bold(true).Render("PROVIDER STUDIO"),
		lipgloss.NewStyle().Foreground(p.styles.T.Muted).PaddingLeft(2).Render(tabBar),
	)

	var content string
	switch p.tab {
	case TabOverview:
		content = p.renderOverview()
	case TabFallback:
		content = p.renderFallback()
	case TabAddProvider:
		content = p.renderAddProvider()
	}

	footer := lipgloss.NewStyle().Foreground(p.styles.T.Muted).PaddingLeft(2).
		MaxWidth(max(1, w-2)).Render("tab/shift+tab: switch · esc: close · ←/→: navigate (add) · enter: confirm")

	return lipgloss.JoinVertical(lipgloss.Left, rule, header, "", content, "", rule, footer)
}

func (p *ProviderStudio) renderOverview() string {
	if p.host == nil {
		return lipgloss.NewStyle().Foreground(p.styles.T.Muted).PaddingLeft(4).Render("No host available")
	}

	providers := p.host.Providers()
	if len(providers) == 0 {
		return lipgloss.NewStyle().Foreground(p.styles.T.Muted).PaddingLeft(4).Render("No providers configured.\nPress Tab → Add Provider to add one.")
	}

	var b strings.Builder
	b.WriteString("Configured Providers:\n\n")

	for _, name := range p.host.Providers() {
		spec, ok := config.ProviderSpecFor(name)
		pc := p.host.ProviderConfig(name)
		source := p.host.ProviderAuthSource(name)
		model := pc.DefaultModel
		if model == "" && ok {
			model = spec.DefaultModel
		}

		icon := config.ProviderIcon(name)
		status := "✓"
		if source == "" {
			status = "✗"
		}

		b.WriteString(fmt.Sprintf("  %s %s %s  %s\n", status, icon, name, spec.DisplayName))
		b.WriteString(fmt.Sprintf("    API Type: %s  |  Model: %s  |  BaseURL: %s\n", pc.ApiType, model, orDash(pc.BaseURL)))
		b.WriteString(fmt.Sprintf("    Auth: %s\n", authSourceLabel(source)))
		b.WriteString(fmt.Sprintf("    Fallbacks: %d configured\n\n", len(p.host.ProviderFallbacks())))
	}

	return b.String()
}

func (p *ProviderStudio) renderFallback() string {
	if p.host == nil {
		return lipgloss.NewStyle().Foreground(p.styles.T.Muted).PaddingLeft(4).Render("No host available")
	}

	fps := p.host.ProviderFallbacks()
	if len(fps) == 0 {
		return lipgloss.NewStyle().Foreground(p.styles.T.Muted).PaddingLeft(4).Render(
			"No fallback chain configured.\nPress Tab → Add Provider to add providers, then configure fallback chain.")
	}

	var b strings.Builder
	b.WriteString("Fallback Chain (primary → fallback):\n\n")

	primaryName := p.host.Provider()
	primaryModel := p.host.Model()
	b.WriteString(fmt.Sprintf("  ▸ 0. %s/%s (current)\n", primaryName, primaryModel))

	for i, fp := range p.host.ProviderFallbacks() {
		cursor := "  "
		if i == p.fallbackCursor {
			cursor = "▸ "
		}
		b.WriteString(fmt.Sprintf("%s%d. %s/%s\n", cursor, i+1, fp.Provider, fp.Model))
	}

	b.WriteString("\n")
	b.WriteString("  ↑/↓: navigate  •  a: add  •  d: remove  •  ←/→: move  •  c: clear")

	return b.String()
}

func (p *ProviderStudio) renderAddProvider() string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Add Provider (step %d/10)\n\n", p.addStep+1))

	stepNames := []string{"Name", "API Type", "Base URL", "Model API URL", "API Key", "Headers (opt)", "Test", "Fetch Models", "Fallback if fails", "Save"}

	for i, s := range stepNames {
		marker := "  "
		if i == p.addStep {
			marker = "▸ "
		}
		b.WriteString(fmt.Sprintf("%s%s: %s\n", marker, s, p.addStepValue(i)))
	}

	b.WriteString("\n")
	b.WriteString("  ←/→: navigate steps  •  Enter: confirm/next  •  Backspace: back/edit")

	return b.String()
}

func (p *ProviderStudio) addStepValue(step int) string {
	switch step {
	case 0:
		return p.addName
	case 1:
		return p.addApiType
	case 2:
		return orDash(p.addBaseURL)
	case 3:
		return orDash(p.addModelApiURL)
	case 4:
		if p.addAPIKey == "" {
			return "(empty)"
		}
		return "********"
	case 5:
		return orDash(p.addHeaders)
	case 6:
		return "Press Enter to test"
	case 7:
		return "Press Enter to fetch"
	case 8:
		return "Toggle fallback on failure"
	case 9:
		return "Press Enter to save"
	}
	return ""
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

func authSourceLabel(source string) string {
	switch source {
	case "config":
		return "config file"
	case "env":
		return "environment"
	case "secret store":
		return "secret store"
	default:
		return "not set"
	}
}