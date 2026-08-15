package components

import (
	"strings"
	"sync"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"image/color"

	"github.com/iSundram/Automergent/internal/agent"
	"github.com/iSundram/Automergent/internal/tui/themes"
)

// Confirm renders an inline confirmation panel below the input.
type Confirm struct {
	styles     *themes.Styles
	visible    bool
	prompt     string
	replyCh    chan agent.ConfirmationResponse
	feedback   textinput.Model
	mode       confirmMode
	width      int
	height     int
	mu         *sync.Mutex
	responded  bool
	simple     bool
	trust      bool
	permission *PermissionInfo
}

type PermissionField struct {
	Label string
	Value string
}

type PermissionInfo struct {
	Icon   string
	Tool   string
	Action string
	Fields []PermissionField
	Risk   string
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
		mu:       &sync.Mutex{},
	}
}

// Show displays the confirmation prompt.
func (c *Confirm) Show(prompt string) {
	c.prompt = prompt
	c.visible = true
	c.mode = modeSelection
	c.simple = false
	c.trust = false
	c.permission = nil
	c.feedback.Reset()
	c.mu.Lock()
	c.responded = false
	c.mu.Unlock()
}

// ShowSimple displays a confirmation with only allow and reject actions.
func (c *Confirm) ShowSimple(prompt string) {
	c.Show(prompt)
	c.simple = true
}

// ShowTrust displays session-only, remembered, and reject trust choices.
func (c *Confirm) ShowTrust(prompt string) {
	c.Show(prompt)
	c.trust = true
}

// ShowPermission displays structured information about a tool request.
func (c *Confirm) ShowPermission(info PermissionInfo) {
	c.Show(info.Action)
	c.permission = &info
}

// ShowWithDiff is same as Show (diff preview removed for clean UI).
func (c *Confirm) ShowWithDiff(prompt, _ string) {
	c.Show(prompt)
}

// SetReply sets the channel to send the reply to.
func (c *Confirm) SetReply(ch chan agent.ConfirmationResponse) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.replyCh = ch
	c.responded = false
}

// Hide hides the confirmation prompt.
func (c *Confirm) Hide() { c.visible = false }

// Visible reports whether the prompt is visible.
func (c Confirm) Visible() bool { return c.visible }

// SetSize updates dimensions.
func (c *Confirm) SetSize(w, h int) {
	c.width = w
	c.height = h
	fw := w - 8
	if fw < 20 {
		fw = 20
	}
	if fw > 80 {
		fw = 80
	}
	c.feedback.SetWidth(fw)
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
			if !c.simple || c.trust {
				c.sendResponse(agent.ConfirmationResponse{Allow: true, Always: true})
			}
		case "n", "N":
			c.sendResponse(agent.ConfirmationResponse{Allow: false})
		case "f", "F":
			if !c.simple {
				c.mode = modeFeedback
				c.feedback.Focus()
				return c, textinput.Blink
			}
		case "esc":
			c.sendResponse(agent.ConfirmationResponse{Allow: false})
		}
	}
	return c, nil
}

func (c *Confirm) sendResponse(res agent.ConfirmationResponse) {
	c.visible = false
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.responded {
		return
	}
	c.responded = true
	if c.replyCh != nil {
		select {
		case c.replyCh <- res:
		default:
		}
	}
}

// View renders the confirmation as a full-width inline panel (shown below the input).
func (c Confirm) View() string {
	if !c.visible {
		return ""
	}

	var lines []string

	if c.mode == modeFeedback {
		header := lipgloss.NewStyle().
			Foreground(c.styles.T.Yellow).
			Bold(true).
			Render("Reject with feedback")
		lines = append(lines, header)
		lines = append(lines, c.feedback.View())
		lines = append(lines, c.styles.Dim.Render("enter submit · esc back"))
	} else if c.permission != nil {
		lines = append(lines, c.renderPermission())
		lines = append(lines, c.renderOptions())
	} else {
		lines = append(lines, c.formatPrompt())
		lines = append(lines, c.renderOptions())
	}

	content := lipgloss.JoinVertical(lipgloss.Left, lines...)

	w := c.width
	if w <= 0 {
		w = lipgloss.Width(content) + 4
	}
	w-- // account for the left border added outside the width

	// Replace the input row with a compact two-line confirmation footer.
	return lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), true, false, false, false).
		BorderForeground(c.styles.T.BorderFocused).
		Padding(0, 1).
		Width(w - 2).
		Render(content)
}

func (c Confirm) renderPermission() string {
	p := c.permission
	title := lipgloss.NewStyle().Foreground(c.styles.T.Yellow).Bold(true).
		Render(p.Icon + "  Permission required")
	tool := lipgloss.NewStyle().Foreground(c.styles.T.Accent).Bold(true).Render(p.Tool)
	action := lipgloss.NewStyle().Foreground(c.styles.T.Subtext).Render("  " + p.Action)
	lines := []string{title, tool + action}
	for _, field := range p.Fields {
		label := lipgloss.NewStyle().Foreground(c.styles.T.Muted).Render(field.Label + ": ")
		maxValueWidth := c.width - lipgloss.Width(label) - 4
		if maxValueWidth < 8 {
			maxValueWidth = 8
		}
		valueText := ansi.Truncate(field.Value, maxValueWidth, "…")
		value := lipgloss.NewStyle().Foreground(c.styles.T.Text).Render(valueText)
		lines = append(lines, label+value)
	}
	if p.Risk != "" {
		lines = append(lines, lipgloss.NewStyle().Foreground(c.styles.T.Yellow).Render("Risk: "+p.Risk))
	}
	return strings.Join(lines, "\n")
}

func (c Confirm) formatPrompt() string {
	icon := lipgloss.NewStyle().Foreground(c.styles.T.Yellow).Bold(true).Render("● ")
	prompt := c.prompt
	if !strings.HasPrefix(prompt, "Allow ") {
		return icon + lipgloss.NewStyle().Bold(true).Render(prompt)
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

	return icon + lipgloss.NewStyle().Bold(true).Render("Allow ") + highlighted + remainder
}

func (c Confirm) renderOptions() string {
	makeOption := func(key, label string, fg color.Color) string {
		keyPart := lipgloss.NewStyle().Foreground(fg).Bold(true).Render(key)
		labelPart := lipgloss.NewStyle().Foreground(c.styles.T.Subtext).Render(" " + label)
		return keyPart + labelPart
	}

	sep := lipgloss.NewStyle().Foreground(c.styles.T.Muted).Render("  ·  ")

	options := []string{
		makeOption("y", "allow", c.styles.T.Green),
		makeOption("n", "reject", c.styles.T.Red),
	}
	if c.trust {
		options = []string{
			makeOption("y", "trust this session", c.styles.T.Green),
			makeOption("a", "trust and remember", c.styles.T.Accent),
			makeOption("n", "continue read-only", c.styles.T.Red),
		}
	} else if !c.simple {
		options = []string{
			makeOption("y", "allow", c.styles.T.Green),
			makeOption("a", "always", c.styles.T.Accent),
			makeOption("n", "reject", c.styles.T.Red),
			makeOption("f", "feedback", c.styles.T.Yellow),
		}
	}
	return strings.Join(options, sep)
}
