package components

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"

	"github.com/iSundram/Automergent/internal/tui/themes"
	"github.com/iSundram/Automergent/internal/version"
)

// Header renders the top bar.
type Header struct {
	styles *themes.Styles
	width  int
	model  string
	mode   string
	tokens int
	cost   float64
}

// NewHeader creates a new Header component.
func NewHeader(styles *themes.Styles) Header {
	return Header{styles: styles}
}

// SetWidth updates the header width.
func (h *Header) SetWidth(w int) { h.width = w }

// SetModel updates the model name displayed.
func (h *Header) SetModel(m string) { h.model = m }

// SetMode updates the mode displayed.
func (h *Header) SetMode(m string) { h.mode = m }

// SetTokens updates the token count displayed.
func (h *Header) SetTokens(n int) { h.tokens = n }

// SetCost updates the cost displayed.
func (h *Header) SetCost(c float64) { h.cost = c }

// View renders the header bar.
func (h Header) View() string {
	left := h.styles.Header.Render(fmt.Sprintf("  Automergent %s", version.Version))
	mid := h.styles.Header.Render(fmt.Sprintf("%s • %s", h.model, h.mode))
	right := h.styles.Header.Render(fmt.Sprintf("tokens: %d  cost: $%.4f  ", h.tokens, h.cost))

	spacer := h.width - lipgloss.Width(left) - lipgloss.Width(mid) - lipgloss.Width(right)
	if spacer < 0 {
		spacer = 0
	}
	return left + mid + lipgloss.NewStyle().Width(spacer).Render("") + right
}
