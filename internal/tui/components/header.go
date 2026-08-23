package components

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/iSundram/Automergent/internal/tui/themes"
)

// Header renders the top bar with a modern HUD look.
type Header struct {
	styles         *themes.Styles
	width          int
	model          string
	provider       string
	mode           string
	phase          string // "research", "plan", "execute"
	activeTokens   int    // tokens in current prompt (active context)
	totalTokens    int    // cumulative session tokens
	maxTokens      int
	adaptiveWeight float64 // learned token estimation weight (1.0 = perfect)
	cost           float64 // session cost in USD
}

// NewHeader creates a new Header component.
func NewHeader(styles *themes.Styles) Header {
	return Header{
		styles:    styles,
		maxTokens: 200000,
	}
}

func (h *Header) SetWidth(w int)              { h.width = w }
func (h *Header) SetModel(m string)           { h.model = m }
func (h *Header) SetProvider(p string)        { h.provider = p }
func (h *Header) SetMode(m string)            { h.mode = m }
func (h *Header) SetPhase(p string)           { h.phase = p }
func (h *Header) SetTokens(n int)             { h.totalTokens = n }
func (h *Header) SetActiveTokens(n int)       { h.activeTokens = n }
func (h *Header) SetMaxTokens(n int)          { h.maxTokens = n }
func (h *Header) SetAdaptiveWeight(w float64) { h.adaptiveWeight = w }
func (h *Header) SetCost(usd float64)         { h.cost = usd }

func (h *Header) getPhaseStyle() lipgloss.Style {
	base := lipgloss.NewStyle().Bold(true).Padding(0, 1).Foreground(h.styles.T.Background)
	switch strings.ToLower(h.phase) {
	case "research":
		return base.Background(h.styles.T.Blue)
	case "plan":
		return base.Background(h.styles.T.Yellow)
	case "execute":
		return base.Background(h.styles.T.Green)
	default:
		return base.Background(h.styles.T.Accent)
	}
}

func (h *Header) getProviderIcon() string {
	p := strings.ToLower(h.provider)
	switch {
	case strings.Contains(p, "google") || strings.Contains(p, "gemini"):
		return "󰊭"
	default:
		return "󰩩"
	}
}

func (h *Header) renderProgressBar(width int) string {
	if h.maxTokens <= 0 || width <= 0 {
		return ""
	}
	// Progress bar shows active context usage
	tokens := h.activeTokens
	if tokens == 0 {
		tokens = h.totalTokens
	}
	ratio := float64(tokens) / float64(h.maxTokens)
	if ratio > 1.0 {
		ratio = 1.0
	}
	blocks := int(float64(width) * ratio)

	barColor := h.styles.T.Accent
	if ratio > 0.8 {
		barColor = h.styles.T.Red
	} else if ratio > 0.5 {
		barColor = h.styles.T.Yellow
	}

	barStyle := lipgloss.NewStyle().Foreground(barColor)
	full := strings.Repeat("█", blocks)
	empty := strings.Repeat("░", width-blocks)

	pct := lipgloss.NewStyle().Foreground(barColor).Bold(true).Render(fmt.Sprintf("%2.0f%%", ratio*100))
	return barStyle.Render(full) + lipgloss.NewStyle().Foreground(h.styles.T.Muted).Render(empty) + " " + pct
}

// View renders the header bar as an adaptive modern HUD.
func (h Header) View() string {
	if h.width <= 0 {
		return ""
	}

	// 1. Left Section: Brand & Phase
	// Keep one trailing cell as part of the brand so adjacent header content
	// never appears stuck to the final letter.
	brandText := "⟡ AUTOMERGENT "
	if h.width < 70 {
		brandText = "⟡"
	}
	brand := h.styles.HeaderBrand.Render(brandText)

	phaseLabel := ""
	if h.width > 50 {
		phaseText := strings.ToUpper(h.phase)
		if phaseText == "" {
			phaseText = "IDLE"
		}
		phaseLabel = " " + h.getPhaseStyle().Render(phaseText)
	}
	left := lipgloss.JoinHorizontal(lipgloss.Center, brand, phaseLabel)

	// 2. Center Section: Provider & Model
	providerIcon := h.getProviderIcon()
	providerName := ""
	if h.width > 110 {
		providerName = h.provider + " "
	}

	modelStr := h.model
	if modelStr == "" {
		modelStr = "detecting..."
	}
	if h.width < 60 && len(modelStr) > 10 {
		modelStr = modelStr[:7] + "..."
	}

	center := lipgloss.JoinHorizontal(lipgloss.Center,
		lipgloss.NewStyle().Foreground(h.styles.T.Accent).Render(providerIcon+" "),
		lipgloss.NewStyle().Foreground(h.styles.T.Muted).Render(providerName),
		lipgloss.NewStyle().Foreground(h.styles.T.Text).Render(modelStr),
	)

	// 3. Right Section: Cost, Tokens, Adaptive Weight & Bar
	tokenStr := fmt.Sprintf("%s/%s", formatTokens(h.activeTokens), formatTokens(h.totalTokens))
	usageInfo := lipgloss.NewStyle().Foreground(h.styles.T.Subtext).Render(tokenStr)
	if h.cost > 0 {
		costStyle := lipgloss.NewStyle().Foreground(h.styles.T.Green)
		usageInfo = costStyle.Render(fmt.Sprintf("$%.4f", h.cost)) + " │ " + usageInfo
	}
	if h.adaptiveWeight > 0 {
		weightStyle := lipgloss.NewStyle().Foreground(h.styles.T.Muted)
		if h.adaptiveWeight < 0.8 || h.adaptiveWeight > 1.2 {
			weightStyle = lipgloss.NewStyle().Foreground(h.styles.T.Yellow)
		}
		usageInfo += " " + weightStyle.Render(fmt.Sprintf("w:%.2f", h.adaptiveWeight))
	}

	barWidth := 0
	if h.width > 130 {
		barWidth = 15
	} else if h.width > 100 {
		barWidth = 10
	}

	contextBar := ""
	if barWidth > 0 {
		contextBar = h.renderProgressBar(barWidth) + "  "
	}
	right := lipgloss.JoinHorizontal(lipgloss.Center, contextBar, usageInfo)

	// 4. Composition using Flex-style gaps
	leftWidth := lipgloss.Width(left)
	centerWidth := lipgloss.Width(center)
	rightWidth := lipgloss.Width(right)

	// Subtract padding (2 on each side = 4)
	availableWidth := h.width - 4

	gap1 := (availableWidth/2 - centerWidth/2) - leftWidth
	gap2 := availableWidth - (leftWidth + gap1 + centerWidth) - rightWidth

	if gap1 < 1 {
		gap1 = 1
	}
	if gap2 < 1 {
		gap2 = 1
	}

	content := lipgloss.JoinHorizontal(lipgloss.Center,
		left,
		strings.Repeat(" ", gap1),
		center,
		strings.Repeat(" ", gap2),
		right,
	)

	// Apply the theme background to the entire bar to prevent black gaps
	return lipgloss.NewStyle().
		Background(h.styles.T.Background).
		Foreground(h.styles.T.Text).
		Padding(0, 2).
		Width(h.width).
		Border(lipgloss.NormalBorder(), false, false, true, false).
		BorderForeground(h.styles.T.BorderNormal).
		Render(content)
}

func formatTokens(n int) string {
	if n >= 1000000 {
		return fmt.Sprintf("%.1fM", float64(n)/1000000)
	}
	if n >= 1000 {
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	}
	return fmt.Sprintf("%d", n)
}
