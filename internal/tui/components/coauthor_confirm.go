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

// View renders the co-author confirmation dialog.
func (c CoAuthorConfirm) View() string {
	if !c.visible {
		return ""
	}

	var content strings.Builder

	// Header
	header := lipgloss.NewStyle().
		Foreground(c.styles.T.Accent).
		Bold(true).
		Render("󰊢 CO-AUTHOR")
	content.WriteString(header + "\n\n")

	// Message
	content.WriteString("Include Automergent as co-author?\n\n")

	// Buttons
	content.WriteString(c.renderButtons())

	// Fixed width box with solid background
	boxWidth := 50

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(c.styles.T.Accent).
		Background(c.styles.T.Background).
		Foreground(c.styles.T.Text).
		Padding(1, 2).
		Width(boxWidth).
		Render(content.String())
}

func (c CoAuthorConfirm) renderButtons() string {
	makeButton := func(key, label string, bg color.Color) string {
		keyStyle := lipgloss.NewStyle().Underline(true).Bold(true)
		return lipgloss.NewStyle().
			Background(bg).
			Foreground(c.styles.T.Background).
			Bold(true).
			Padding(0, 1).
			MarginRight(1).
			Render(keyStyle.Render(key) + " " + label)
	}

	return lipgloss.JoinHorizontal(lipgloss.Center,
		makeButton("y", "Yes", c.styles.T.Green),
		makeButton("n", "No", c.styles.T.Yellow),
		makeButton("a", "Always", c.styles.T.Accent),
		makeButton("x", "Never", c.styles.T.Red),
	)
}
