package components

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/iSundram/Automergent/internal/tui/themes"
)

// ConfirmMsg is sent when the user responds to a confirmation prompt.
type ConfirmMsg struct {
	Confirmed bool
}

// Confirm renders a yes/no confirmation prompt.
type Confirm struct {
	styles  *themes.Styles
	visible bool
	prompt  string
	replyCh chan bool
}

// NewConfirm creates a new Confirm component.
func NewConfirm(styles *themes.Styles) Confirm {
	return Confirm{styles: styles}
}

// Show displays the confirmation prompt.
func (c *Confirm) Show(prompt string) { c.prompt = prompt; c.visible = true }

// SetReply sets the channel to send the reply to.
func (c *Confirm) SetReply(ch chan bool) { c.replyCh = ch }

// Hide hides the confirmation prompt.
func (c *Confirm) Hide() { c.visible = false }

// Visible reports whether the prompt is visible.
func (c Confirm) Visible() bool { return c.visible }

// Update handles yes/no key input.
func (c Confirm) Update(msg tea.Msg) (Confirm, tea.Cmd) {
	if !c.visible {
		return c, nil
	}
	if km, ok := msg.(tea.KeyMsg); ok {
		switch km.String() {
		case "y", "Y":
			c.visible = false
			if c.replyCh != nil {
				select {
				case c.replyCh <- true:
				default:
				}
			}
			return c, func() tea.Msg { return ConfirmMsg{Confirmed: true} }
		case "n", "N", "esc":
			c.visible = false
			if c.replyCh != nil {
				select {
				case c.replyCh <- false:
				default:
				}
			}
			return c, func() tea.Msg { return ConfirmMsg{Confirmed: false} }
		}
	}
	return c, nil
}

// View renders the confirmation prompt.
func (c Confirm) View() string {
	if !c.visible {
		return ""
	}
	box := c.styles.ConfirmBox.Render(
		fmt.Sprintf("\n  %s\n\n  [y] Yes  [n] No\n", c.prompt),
	)
	return box
}
