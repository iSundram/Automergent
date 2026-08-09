package components

import (
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/iSundram/Automergent/internal/tui/themes"
)

// Input is a multi-line text input component.
type Input struct {
	ta     textarea.Model
	styles *themes.Styles
}

// NewInput creates a new Input component.
func NewInput(styles *themes.Styles) Input {
	ta := textarea.New()
	ta.Placeholder = "Ask Automergent anything... (Enter to send, Ctrl+C to quit)"
	ta.ShowLineNumbers = false
	ta.SetHeight(3)
	ta.CharLimit = 0
	ta.Focus()

	return Input{ta: ta, styles: styles}
}

// SetWidth updates the input width.
func (i *Input) SetWidth(w int) {
	i.ta.SetWidth(w)
}

// Value returns the current input text.
func (i Input) Value() string { return i.ta.Value() }

// Reset clears the input.
func (i *Input) Reset() { i.ta.Reset() }

// Focus gives the input focus.
func (i *Input) Focus() tea.Cmd { return i.ta.Focus() }

// Blur removes focus from the input.
func (i *Input) Blur() { i.ta.Blur() }

// Update handles key events.
func (i Input) Update(msg tea.Msg) (Input, tea.Cmd) {
	ta, cmd := i.ta.Update(msg)
	i.ta = ta
	return i, cmd
}

// View renders the input.
func (i Input) View() string {
	return i.ta.View()
}
