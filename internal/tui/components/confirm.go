package components

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"image/color"

	"github.com/iSundram/Automergent/internal/agent"
	"github.com/iSundram/Automergent/internal/tui/themes"
)

// Confirm renders a clean confirmation dialog as centered overlay.
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

// ShowWithDiff is same as Show (diff preview removed for clean UI).
func (c *Confirm) ShowWithDiff(prompt, _ string) {
	c.Show(prompt)
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
	c.feedback.SetWidth(40)
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

// View renders the confirmation dialog.
func (c Confirm) View() string {
	if !c.visible {
		return ""
	}

	var content strings.Builder

	if c.mode == modeFeedback {
		// Feedback mode
		header := lipgloss.NewStyle().
			Foreground(c.styles.T.Yellow).
			Bold(true).
			Render("󰙺 REJECT WITH FEEDBACK")
		content.WriteString(header + "\n\n")
		content.WriteString(c.feedback.View() + "\n\n")
		content.WriteString(c.styles.Dim.Render("[Enter] Submit  [Esc] Cancel"))
	} else {
		// Selection mode - clean and simple
		header := lipgloss.NewStyle().
			Foreground(c.styles.T.Accent).
			Bold(true).
			Render("󱈸 CONFIRM ACTION")
		content.WriteString(header + "\n\n")

		// Format the prompt
		prompt := c.formatPrompt()
		content.WriteString(prompt + "\n\n")

		// Buttons
		content.WriteString(c.renderButtons())
	}

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

func (c Confirm) formatPrompt() string {
	prompt := c.prompt
	if !strings.HasPrefix(prompt, "Allow ") {
		return prompt
	}

	rest := strings.TrimPrefix(prompt, "Allow ")
	actionEnd := strings.IndexAny(rest, " :?")
	if actionEnd == -1 {
		actionEnd = len(rest)
	}

	actionName := rest[:actionEnd]
	remainder := rest[actionEnd:]

	highlighted := lipgloss.NewStyle().
		Foreground(c.styles.T.AccentAlt).
		Bold(true).
		Render(actionName)

	return "Allow " + highlighted + remainder
}

func (c Confirm) renderButtons() string {
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
		makeButton("y", "Allow", c.styles.T.Green),
		makeButton("a", "Always", c.styles.T.Accent),
		makeButton("n", "Reject", c.styles.T.Red),
		makeButton("f", "Feedback", c.styles.T.Yellow),
	)
}
