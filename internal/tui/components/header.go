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
	phase          string // "init", "explore", "plan", "build"
	activeTokens   int    // tokens in the current conversation (context usage)
	totalTokens    int    // cumulative session usage: all requests, real provider-reported counts
	maxTokens      int
	adaptiveWeight float64 // learned token estimation weight (1.0 = perfect)
	cost           float64 // session cost in USD
	effort         string  // thinking effort: low|medium|high|max
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
func (h *Header) SetTokens(n int)             { h.activeTokens = n }
func (h *Header) SetActiveTokens(n int)       { h.activeTokens = n }
func (h *Header) SetTotalTokens(n int)        { h.totalTokens = n }
func (h *Header) SetMaxTokens(n int)          { h.maxTokens = n }
func (h *Header) SetAdaptiveWeight(w float64) { h.adaptiveWeight = w }
func (h *Header) SetCost(usd float64)         { h.cost = usd }
func (h *Header) SetEffort(e string)          { h.effort = e }

func (h *Header) getPhaseStyle() lipgloss.Style {
	base := lipgloss.NewStyle().Bold(true).Padding(0, 1).Foreground(h.styles.T.Background)
	switch strings.ToLower(h.phase) {
	case "init":
		return base.Background(h.styles.T.Muted)
	case "explore":
		return base.Background(h.styles.T.Blue)
	case "plan":
		return base.Background(h.styles.T.Yellow)
	case "build":
		return base.Background(h.styles.T.Green)
	default:
		return base.Background(h.styles.T.Accent)
	}
}

func (h *Header) getProviderIcon() string {
	p := strings.ToLower(h.provider)
	switch {
	case strings.Contains(p, "google") || strings.Contains(p, "gemini"):
		return "●"
	default:
		return "○"
	}
}

func (h *Header) renderProgressBar(width int) string {
	if h.maxTokens <= 0 || width <= 0 {
		return ""
	}
	// The bar shows CONTEXT usage: the current conversation's tokens
	// against the model's window. Cumulative session totals are a different
	// number and made the bar lie.
	ratio := float64(h.activeTokens) / float64(h.maxTokens)
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
	// BrandMark carries its own trailing space, so the wordmark follows it
	// directly. Keep one trailing cell after the name too, so adjacent header
	// content never appears stuck to the final letter.
	brand := h.styles.BrandMark()
	if h.width >= 70 {
		brand += h.styles.HeaderBrand.Render(themes.BrandName + " ")
	}

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

	// 3. Right Section: Cost, Effort & Bar
	// Context usage is the bar alone — the live token count moved to the
	// conversation spinner's parenthetical ("(12s • ↓ 1.2k)"), so the header
	// no longer prints the raw numbers. Cost is always shown — even $0.00 —
	// so the user has immediate context.
	costStyle := lipgloss.NewStyle().Foreground(h.styles.T.Green)
	usageInfo := costStyle.Render(fmt.Sprintf("$%.2f", h.cost))
	// Effort chip: the thinking level the model is running at. Always shown
	// (even the default) so the user can see what they'll get; widened
	// thresholds only gate it on very narrow terminals.
	if e := strings.ToLower(strings.TrimSpace(h.effort)); e != "" && h.width > 60 {
		usageInfo += " " + lipgloss.NewStyle().Foreground(h.styles.T.Muted).Render("effort:" + e)
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
