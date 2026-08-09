package components

import (
	"strings"

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
	focused bool
	width   int
}

// NewInput creates a new Input component.
func NewInput(styles *themes.Styles) Input {
	ta := textarea.New()
	ta.Placeholder = "Message Automergent... (Enter to send, / for commands, @ for files, ? for help)"
	ta.ShowLineNumbers = false
	ta.SetHeight(1)
	ta.MaxHeight = 8
	ta.CharLimit = 0
	ta.Focus()

	return Input{ta: ta, styles: styles, histIdx: -1, focused: true}
}

// SetWidth updates the input width.
func (i *Input) SetWidth(w int) {
	i.width = w
	taW := w - 8 // account for margins and borders
	if taW < 10 {
		taW = 10
	}
	i.ta.SetWidth(taW)
}

// Value returns the current input text.
func (i Input) Value() string { return i.ta.Value() }

// SetValue updates the input text.
func (i *Input) SetValue(v string) {
	i.ta.SetValue(v)
	i.ta.SetCursor(len(v))
}

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
	i.ta.SetHeight(1)
	i.histIdx = -1
}

// Focus gives the input focus.
func (i *Input) Focus() tea.Cmd {
	i.focused = true
	return i.ta.Focus()
}

// Blur removes focus from the input.
func (i *Input) Blur() {
	i.focused = false
	i.ta.Blur()
}

// LineCount returns the number of lines in the input.
func (i Input) LineCount() int {
	return i.ta.LineCount()
}

// TriggerType returns the current palette trigger if any.
func (i Input) TriggerType() string {
	val := i.ta.Value()
	if val == "?" {
		return "help"
	}
	if strings.HasPrefix(val, "/") {
		if strings.HasPrefix(val, "/model") {
			return "model"
		}
		if strings.HasPrefix(val, "/provider") {
			return "provider"
		}
		return "command"
	}
	if strings.Contains(val, "@") {
		parts := strings.Fields(val)
		if len(parts) > 0 && strings.HasPrefix(parts[len(parts)-1], "@") {
			return "file"
		}
		if strings.HasSuffix(val, "@") {
			return "file"
		}
	}
	return ""
}

// TriggerValue returns the text after the trigger for filtering.
func (i Input) TriggerValue() string {
	val := i.ta.Value()
	trigger := i.TriggerType()
	switch trigger {
	case "help":
		return ""
	case "command":
		return strings.TrimPrefix(val, "/")
	case "model":
		v := strings.TrimPrefix(val, "/model")
		return strings.TrimSpace(v)
	case "provider":
		v := strings.TrimPrefix(val, "/provider")
		return strings.TrimSpace(v)
	case "file":
		idx := strings.LastIndex(val, "@")
		if idx != -1 {
			return val[idx+1:]
		}
	}
	return ""
}

// InsertValue completes the current trigger with the selected value.
func (i *Input) InsertValue(v string) {
	val := i.ta.Value()
	trigger := i.TriggerType()
	switch trigger {
	case "help", "command":
		i.ta.SetValue("/" + v + " ")
	case "model":
		i.ta.SetValue("/model " + v + " ")
	case "provider":
		i.ta.SetValue("/provider " + v + " ")
	case "file":
		idx := strings.LastIndex(val, "@")
		if idx != -1 {
			i.ta.SetValue(val[:idx] + "@" + v + " ")
		}
	}
	i.ta.SetCursor(len(i.ta.Value()))
}

// Update handles key events and auto-resizing.
func (i Input) Update(msg tea.Msg) (Input, tea.Cmd) {
	if km, ok := msg.(tea.KeyMsg); ok {
		switch km.String() {
		case "alt+up", "ctrl+p":
			if len(i.history) > 0 {
				if i.histIdx < len(i.history)-1 {
					i.histIdx++
				}
				idx := len(i.history) - 1 - i.histIdx
				i.ta.SetValue(i.history[idx])
				return i, nil
			}
		case "alt+down", "ctrl+n":
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

	lineCount := ta.LineCount()
	if lineCount > ta.MaxHeight {
		lineCount = ta.MaxHeight
	}
	if lineCount < 1 {
		lineCount = 1
	}
	i.ta.SetHeight(lineCount)

	return i, cmd
}

// View renders the input.
func (i Input) View() string {
	if i.width <= 0 {
		return ""
	}
	if i.focused {
		return i.styles.InputFocused.Render(i.ta.View())
	}
	return i.styles.Input.Render(i.ta.View())
}
