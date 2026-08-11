package components

import (
	"fmt"

	"charm.land/lipgloss/v2"

	"github.com/iSundram/Automergent/internal/tui/themes"
)

// Header renders the top bar.
type Header struct {
	styles   *themes.Styles
	width    int
	model    string
	provider string
	mode     string
	tokens   int
	cost     float64
}

// NewHeader creates a new Header component.
func NewHeader(styles *themes.Styles) Header {
	return Header{styles: styles}
}

// SetWidth updates the header width.
func (h *Header) SetWidth(w int) { h.width = w }

// SetModel updates the model name displayed.
func (h *Header) SetModel(m string) { h.model = m }

// SetProvider updates the provider name displayed.
func (h *Header) SetProvider(p string) { h.provider = p }

// SetMode updates the mode displayed.
func (h *Header) SetMode(m string) { h.mode = m }

// SetTokens updates the token count displayed.
func (h *Header) SetTokens(n int) { h.tokens = n }

// SetCost updates the cost displayed.
func (h *Header) SetCost(c float64) { h.cost = c }

// modeColor returns a color indicator for the mode.
func (h *Header) modeIcon() string {
	switch h.mode {
	case "edit":
		return "✏️ "
	case "plan":
		return "📋"
	default:
		return "💬"
	}
}

// View renders the header bar as a floating pill.
func (h Header) View() string {
	if h.width <= 0 {
		return ""
	}
	modelStr := h.model
	if modelStr == "" {
		modelStr = "gemini-3.6-flash"
	}
	modeStr := h.mode
	if modeStr == "" {
		modeStr = "edit"
	}

	brand := h.styles.HeaderBrand.Render(" ◈ Automergent ")
	modelInfo := h.styles.HeaderCenter.Render(fmt.Sprintf("[ %s ]", modelStr))
	modeInfo := h.styles.Header.Render(fmt.Sprintf("%s %s ", h.modeIcon(), modeStr))

	tokenStyle := lipgloss.NewStyle().Foreground(h.styles.T.Muted)
	if h.tokens > 100000 {
		tokenStyle = lipgloss.NewStyle().Foreground(h.styles.T.Red)
	} else if h.tokens > 50000 {
		tokenStyle = lipgloss.NewStyle().Foreground(h.styles.T.Yellow)
	}

	tokenStr := tokenStyle.Render(fmt.Sprintf("%s tokens", formatTokens(h.tokens)))
	costStr := lipgloss.NewStyle().Foreground(h.styles.T.Subtext).Render(fmt.Sprintf("$%.3f", h.cost))

	rightInfo := lipgloss.JoinHorizontal(lipgloss.Center, " │ ", costStr, "   ", tokenStr, " ")
	content := lipgloss.JoinHorizontal(lipgloss.Center, brand, " │ ", modelInfo, " │ ", modeInfo, " │ ", rightInfo)

	pill := h.styles.HeaderPill.Render(content)

	// Ensure pill is not wider than width
	pillWidth := lipgloss.Width(pill)
	if pillWidth > h.width {
		// If too wide, just render content without pill style or with reduced padding
		return content
	}

	return lipgloss.PlaceHorizontal(h.width, lipgloss.Center, pill)
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
