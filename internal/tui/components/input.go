package components

import (
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/iSundram/Automergent/internal/tui/themes"
)

// Input is a multi-line text input component with history.
type Input struct {
	ta      textarea.Model
	styles  *themes.Styles
	history []string
	histIdx int
}

// NewInput creates a new Input component.
func NewInput(styles *themes.Styles) Input {
	ta := textarea.New()
	ta.Placeholder = "Message Automergent… (Enter to send, Alt+Enter for newline, / for commands)"
	ta.ShowLineNumbers = false
	ta.SetHeight(3)
	ta.CharLimit = 0
	ta.Focus()

	return Input{ta: ta, styles: styles, histIdx: -1}
}

// SetWidth updates the input width.
func (i *Input) SetWidth(w int) {
	i.ta.SetWidth(w - 2)
}

// Value returns the current input text.
func (i Input) Value() string { return i.ta.Value() }

// Reset clears the input.
func (i *Input) Reset() {
	val := i.ta.Value()
	if val != "" {
		i.history = append(i.history, val)
		if len(i.history) > 100 {
			i.history = i.history[len(i.history)-100:]
		}
	}
	i.ta.Reset()
	i.histIdx = -1
}

// Focus gives the input focus.
func (i *Input) Focus() tea.Cmd { return i.ta.Focus() }

// Blur removes focus from the input.
func (i *Input) Blur() { i.ta.Blur() }

// Update handles key events.
func (i Input) Update(msg tea.Msg) (Input, tea.Cmd) {
	if km, ok := msg.(tea.KeyMsg); ok {
		switch km.String() {
		case "alt+up", "ctrl+p":
			// Navigate history backwards
			if len(i.history) > 0 {
				if i.histIdx < len(i.history)-1 {
					i.histIdx++
				}
				idx := len(i.history) - 1 - i.histIdx
				i.ta.SetValue(i.history[idx])
				return i, nil
			}
		case "alt+down", "ctrl+n":
			// Navigate history forwards
			if i.histIdx > 0 {
				i.histIdx--
				idx := len(i.history) - 1 - i.histIdx
				i.ta.SetValue(i.history[idx])
			} else if i.histIdx == 0 {
				i.histIdx = -1
				i.ta.SetValue("")
			}
			return i, nil
		}
	}
	ta, cmd := i.ta.Update(msg)
	i.ta = ta
	return i, cmd
}

// View renders the input.
func (i Input) View() string {
	return i.ta.View()
}
