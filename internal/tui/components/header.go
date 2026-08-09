package components

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"

	"github.com/iSundram/Automergent/internal/tui/themes"
	"github.com/iSundram/Automergent/internal/version"
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
	case "full-auto":
		return "🤖"
	case "auto-edit":
		return "✏️ "
	case "plan":
		return "📋"
	default:
		return "💬"
	}
}

// View renders the header bar.
func (h Header) View() string {
	// Left: Automergent brand + version
	left := h.styles.HeaderBrand.Render(fmt.Sprintf("  ◆ Automergent %s", version.Version))

	// Center: provider/model and mode
	providerStr := h.provider
	if providerStr == "" {
		providerStr = "google"
	}
	modelStr := h.model
	if modelStr == "" {
		modelStr = "gemini-3.6-flash"
	}
	modeStr := h.mode
	if modeStr == "" {
		modeStr = "suggest"
	}
	center := h.styles.HeaderCenter.Render(
		fmt.Sprintf("%s  %s/%s  %s %s",
			"│", providerStr, modelStr, h.modeIcon(), modeStr),
	)

	// Right: tokens
	right := h.styles.HeaderRight.Render(fmt.Sprintf("tokens: %s  ", formatTokens(h.tokens)))

	used := lipgloss.Width(left) + lipgloss.Width(center) + lipgloss.Width(right)
	spacer := h.width - used
	if spacer < 0 {
		spacer = 0
	}

	return left +
		lipgloss.NewStyle().Background(h.styles.T.Surface).Width(spacer/2).Render("") +
		center +
		lipgloss.NewStyle().Background(h.styles.T.Surface).Width(spacer-spacer/2).Render("") +
		right
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
