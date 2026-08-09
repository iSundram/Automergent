package components

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/iSundram/Automergent/internal/agent"
	"github.com/iSundram/Automergent/internal/tui/themes"
)

// Confirm renders an advanced tool confirmation dialog.
type Confirm struct {
	styles   *themes.Styles
	visible  bool
	prompt   string
	replyCh  chan agent.ConfirmationResponse
	feedback textinput.Model
	mode     confirmMode
	width    int
	height   int
}

type confirmMode int

const (
	modeSelection confirmMode = iota
	modeFeedback
)

// NewConfirm creates a new Confirm component.
func NewConfirm(styles *themes.Styles) Confirm {
	ti := textinput.New()
	ti.Placeholder = "Reason for rejection..."
	ti.Focus()
	return Confirm{
		styles:   styles,
		feedback: ti,
		mode:     modeSelection,
	}
}

// Show displays the confirmation prompt.
func (c *Confirm) Show(prompt string) {
	c.prompt = prompt
	c.visible = true
	c.mode = modeSelection
	c.feedback.Reset()
}

// SetReply sets the channel to send the reply to.
func (c *Confirm) SetReply(ch chan agent.ConfirmationResponse) { c.replyCh = ch }

// Hide hides the confirmation prompt.
func (c *Confirm) Hide() { c.visible = false }

// Visible reports whether the prompt is visible.
func (c Confirm) Visible() bool { return c.visible }

// SetSize updates dimensions.
func (c *Confirm) SetSize(w, h int) {
	c.width = w
	c.height = h
	c.feedback.Width = w / 2
}

// Update handles confirmation selection and feedback input.
func (c Confirm) Update(msg tea.Msg) (Confirm, tea.Cmd) {
	if !c.visible {
		return c, nil
	}

	if c.mode == modeFeedback {
		if km, ok := msg.(tea.KeyMsg); ok {
			switch km.String() {
			case "enter":
				feedback := strings.TrimSpace(c.feedback.Value())
				c.sendResponse(agent.ConfirmationResponse{Allow: false, Feedback: feedback})
				return c, nil
			case "esc":
				c.mode = modeSelection
				return c, nil
			}
		}
		var cmd tea.Cmd
		c.feedback, cmd = c.feedback.Update(msg)
		return c, cmd
	}

	if km, ok := msg.(tea.KeyMsg); ok {
		switch km.String() {
		case "y", "Y", "enter":
			c.sendResponse(agent.ConfirmationResponse{Allow: true})
		case "a", "A":
			c.sendResponse(agent.ConfirmationResponse{Allow: true, Always: true})
		case "n", "N":
			c.sendResponse(agent.ConfirmationResponse{Allow: false})
		case "f", "F":
			c.mode = modeFeedback
			c.feedback.Focus()
			return c, textinput.Blink
		case "esc":
			c.sendResponse(agent.ConfirmationResponse{Allow: false})
		}
	}
	return c, nil
}

func (c *Confirm) sendResponse(res agent.ConfirmationResponse) {
	c.visible = false
	if c.replyCh != nil {
		select {
		case c.replyCh <- res:
		default:
		}
	}
}

// View renders the confirmation prompt.
func (c Confirm) View() string {
	if !c.visible {
		return ""
	}

	var content string
	if c.mode == modeFeedback {
		content = fmt.Sprintf(
			" %s\n\n %s\n\n %s",
			c.styles.Bold.Render("Reject with feedback:"),
			c.feedback.View(),
			c.styles.Dim.Render("[enter] Submit  [esc] Back"),
		)
	} else {
		content = fmt.Sprintf(
			" %s\n %s\n\n %s  %s  %s  %s",
			c.styles.Bold.Render("Tool Confirmation Required:"),
			c.prompt,
			c.renderKey("y", "Allow Once"),
			c.renderKey("a", "Always for Session"),
			c.renderKey("n", "Reject"),
			c.renderKey("f", "Reject w/ Feedback"),
		)
	}

	return c.styles.ConfirmBox.
		Width(c.width / 2).
		Render(content)
}

func (c Confirm) renderKey(key, desc string) string {
	// Use a color from the theme for the key highlight
	keyStyle := c.styles.Bold.Copy().Foreground(c.styles.T.Accent)
	return fmt.Sprintf("%s %s ", keyStyle.Render("["+key+"]"), desc)
}
