package components

import (
	"image/color"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/iSundram/Automergent/internal/config"
	"github.com/iSundram/Automergent/internal/tui/themes"
)

// CoAuthorResponse represents the user's choice for co-author.
type CoAuthorResponse struct {
	Include bool   // Whether to include co-author for this commit
	Save    string // "always", "never", or "" (don't save)
}

// CoAuthorConfirm renders a confirmation dialog for co-author inclusion.
type CoAuthorConfirm struct {
	styles  *themes.Styles
	cfg     *config.Config
	visible bool
	replyCh chan CoAuthorResponse
	width   int
	height  int
}

// NewCoAuthorConfirm creates a new CoAuthorConfirm component.
func NewCoAuthorConfirm(styles *themes.Styles, cfg *config.Config) CoAuthorConfirm {
	return CoAuthorConfirm{
		styles: styles,
		cfg:    cfg,
	}
}

// Show displays the co-author confirmation prompt.
func (c *CoAuthorConfirm) Show() {
	c.visible = true
}

// SetReply sets the channel to send the reply to.
func (c *CoAuthorConfirm) SetReply(ch chan CoAuthorResponse) {
	c.replyCh = ch
}

// Hide hides the confirmation prompt.
func (c *CoAuthorConfirm) Hide() {
	c.visible = false
}

// Visible reports whether the prompt is visible.
func (c CoAuthorConfirm) Visible() bool {
	return c.visible
}

// SetSize updates dimensions.
func (c *CoAuthorConfirm) SetSize(w, h int) {
	c.width = w
	c.height = h
}

// Update handles key events.
func (c CoAuthorConfirm) Update(msg tea.Msg) (CoAuthorConfirm, tea.Cmd) {
	if !c.visible {
		return c, nil
	}

	if km, ok := msg.(tea.KeyMsg); ok {
		switch km.String() {
		case "y", "Y", "enter":
			// Yes for this commit
			c.sendResponse(CoAuthorResponse{Include: true, Save: ""})
		case "n", "N":
			// No for this commit
			c.sendResponse(CoAuthorResponse{Include: false, Save: ""})
		case "a", "A":
			// Always (save preference)
			c.sendResponse(CoAuthorResponse{Include: true, Save: "always"})
		case "x", "X":
			// Never (save preference)
			c.sendResponse(CoAuthorResponse{Include: false, Save: "never"})
		case "esc":
			// Cancel (no for this commit)
			c.sendResponse(CoAuthorResponse{Include: false, Save: ""})
		}
	}
	return c, nil
}

func (c *CoAuthorConfirm) sendResponse(res CoAuthorResponse) {
	c.visible = false
	if c.replyCh != nil {
		select {
		case c.replyCh <- res:
		default:
		}
	}
}

// View renders the co-author confirmation as a full-width inline panel.
func (c CoAuthorConfirm) View() string {
	if !c.visible {
		return ""
	}

	icon := lipgloss.NewStyle().Foreground(c.styles.T.Accent).Bold(true).Render("● ")
	prompt := icon + lipgloss.NewStyle().Bold(true).Render("Include Automergent as co-author?")

	content := lipgloss.JoinVertical(lipgloss.Left, prompt, c.renderOptions())

	w := c.width
	if w <= 0 {
		w = lipgloss.Width(content) + 4
	}
	w-- // account for the left border added outside the width

	return lipgloss.NewStyle().
		Border(lipgloss.ThickBorder(), false, false, false, true).
		BorderForeground(c.styles.T.Accent).
		Padding(0, 2).
		Width(w).
		Render(content)
}

func (c CoAuthorConfirm) renderOptions() string {
	makeOption := func(key, label string, fg color.Color) string {
		keyPart := lipgloss.NewStyle().Foreground(fg).Bold(true).Render(key)
		labelPart := lipgloss.NewStyle().Foreground(c.styles.T.Subtext).Render(" " + label)
		return keyPart + labelPart
	}

	sep := lipgloss.NewStyle().Foreground(c.styles.T.Muted).Render("  ·  ")

	return strings.Join([]string{
		makeOption("y", "yes", c.styles.T.Green),
		makeOption("n", "no", c.styles.T.Yellow),
		makeOption("a", "always", c.styles.T.Accent),
		makeOption("x", "never", c.styles.T.Red),
	}, sep)
}
