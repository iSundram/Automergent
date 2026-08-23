package components

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/iSundram/Automergent/internal/tui/themes"
)

// Input is a multi-line text input component with history.
type Input struct {
	ta            textarea.Model
	styles        *themes.Styles
	history       []string
	histIdx       int
	focused       bool
	width         int
	pastedContent string
}

// NewInput creates a new Input component.
func NewInput(styles *themes.Styles) Input {
	ta := textarea.New()
	ta.Placeholder = "Message Automergent... (Enter to send, / for commands, @ for files, ? for help)"
	ta.ShowLineNumbers = false
	ta.SetHeight(1)
	ta.MaxHeight = 16
	ta.CharLimit = 0
	ta.Prompt = ""

	// Inline look: accent prompt, muted placeholder, no boxes or highlights.
	st := ta.Styles()
	st.Focused.Base = lipgloss.NewStyle().Foreground(styles.T.Text)
	st.Blurred.Base = lipgloss.NewStyle().Foreground(styles.T.Text)
	st.Focused.Prompt = lipgloss.NewStyle().Foreground(styles.T.Text)
	st.Blurred.Prompt = lipgloss.NewStyle().Foreground(styles.T.Text)
	st.Focused.Placeholder = lipgloss.NewStyle().Foreground(styles.T.Muted)
	st.Blurred.Placeholder = lipgloss.NewStyle().Foreground(styles.T.Subtext)
	st.Focused.Text = lipgloss.NewStyle().Foreground(styles.T.Text)
	st.Blurred.Text = lipgloss.NewStyle().Foreground(styles.T.Text)
	st.Focused.CursorLine = lipgloss.NewStyle().Foreground(styles.T.Text)
	st.Blurred.CursorLine = lipgloss.NewStyle().Foreground(styles.T.Text)
	ta.SetStyles(st)

	ta.Focus()

	return Input{ta: ta, styles: styles, histIdx: -1, focused: true}
}

// SetWidth updates the input width.
func (i *Input) SetWidth(w int) {
	i.width = w
	taW := w - 2 // account for horizontal padding
	if taW < 10 {
		taW = 10
	}
	i.ta.SetWidth(taW)
	i.updateHeight()
}

// Value returns the current input text.
func (i Input) Value() string {
	if i.pastedContent != "" {
		return i.pastedContent
	}
	return i.ta.Value()
}

// SetValue updates the input text.
func (i *Input) SetValue(v string) {
	i.ta.SetValue(v)
	i.pastedContent = ""
	i.updateHeight()
	// i.ta.SetCursor(len(v)) // TODO: Fix for Bubble Tea v2
}

// Reset clears the input.
func (i *Input) Reset() {
	val := i.Value()
	if val != "" {
		i.history = append(i.history, val)
		if len(i.history) > 100 {
			i.history = i.history[len(i.history)-100:]
		}
	}
	i.ta.Reset()
	i.ta.SetHeight(1)
	i.histIdx = -1
	i.pastedContent = ""
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

// LineCount returns the number of lines in the input (including soft-wrapped lines).
func (i Input) LineCount() int {
	val := i.Value()
	if val == "" {
		return 1
	}

	width := i.ta.Width()
	if width <= 0 {
		width = 80
	}
	contentWidth := width
	if contentWidth < 10 {
		contentWidth = 10
	}

	paragraphs := strings.Split(val, "\n")
	totalLines := 0
	for _, para := range paragraphs {
		if para == "" {
			totalLines++
			continue
		}

		words := strings.Fields(para)
		if len(words) == 0 {
			totalLines++
			continue
		}

		paraLines := 1
		currentLen := 0
		for _, word := range words {
			wLen := lipgloss.Width(word)
			if wLen > contentWidth {
				if currentLen > 0 {
					paraLines++
					currentLen = 0
				}
				paraLines += (wLen + contentWidth - 1) / contentWidth
				continue
			}

			if currentLen == 0 {
				currentLen = wLen
			} else if currentLen+1+wLen <= contentWidth {
				currentLen += 1 + wLen
			} else {
				paraLines++
				currentLen = wLen
			}
		}
		totalLines += paraLines
	}

	if totalLines < 1 {
		return 1
	}
	return totalLines
}

func (i *Input) updateHeight() {
	lineCount := i.LineCount()
	if lineCount > i.ta.MaxHeight {
		lineCount = i.ta.MaxHeight
	}
	if lineCount < 1 {
		lineCount = 1
	}
	i.ta.SetHeight(lineCount)
}

// SlashSubPalettes lists slash commands whose argument completion is rendered
// as a dedicated palette (model, provider, mode, theme, keybindings and
// effort pickers). This map is the single source of truth shared by
// TriggerType, InsertValue/TriggerValue and the app's palette enter handling;
// keys must be registered non-Immediate command names.
var SlashSubPalettes = map[string]bool{
	"model":       true,
	"provider":    true,
	"mode":        true,
	"theme":       true,
	"keybindings": true,
	"effort":      true,
}

// slashCommandToken extracts the command token from a "/..." input using
// strict token boundaries: "/provider x" -> "provider",
// "/provider-api-key" -> "provider-api-key", "/model:dump" -> "model:dump".
func slashCommandToken(val string) string {
	rest := strings.TrimPrefix(val, "/")
	if idx := strings.IndexAny(rest, " \t\n"); idx >= 0 {
		rest = rest[:idx]
	}
	return rest
}

// TriggerType returns the current palette trigger if any.
func (i Input) TriggerType() string {
	val := i.ta.Value()
	if val == "?" {
		return "help"
	}
	if strings.HasPrefix(val, "/") {
		// Only exact command tokens open a sub-palette: /provider-api-key or
		// namespaced customs like /model:dump stay plain commands.
		if token := slashCommandToken(val); SlashSubPalettes[token] {
			return token
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
	case "command", "file":
		if trigger == "file" {
			idx := strings.LastIndex(val, "@")
			if idx != -1 {
				return val[idx+1:]
			}
			return ""
		}
		return strings.TrimPrefix(val, "/")
	default:
		if SlashSubPalettes[trigger] {
			return strings.TrimSpace(strings.TrimPrefix(val, "/"+trigger))
		}
		return ""
	}
}

// InsertValue completes the current trigger with the selected value.
func (i *Input) InsertValue(v string) {
	val := i.ta.Value()
	trigger := i.TriggerType()
	switch trigger {
	case "help", "command":
		i.ta.SetValue("/" + v + " ")
	case "file":
		idx := strings.LastIndex(val, "@")
		if idx != -1 {
			i.ta.SetValue(val[:idx] + "@" + v + " ")
		}
	default:
		if SlashSubPalettes[trigger] {
			i.ta.SetValue("/" + trigger + " " + v + " ")
		}
	}
	i.updateHeight()
	// i.ta.SetCursor(len(i.ta.Value())) // TODO: Fix for Bubble Tea v2
}

// Update handles key events and auto-resizing.
func (i Input) Update(msg tea.Msg) (Input, tea.Cmd) {
	// Handle paste events specially - allow multi-line content
	if pm, ok := msg.(tea.PasteMsg); ok {
		cleaned := strings.TrimRight(pm.Content, "\n")
		lines := strings.Split(cleaned, "\n")
		if len(lines) > 1 {
			existing := i.ta.Value()
			if existing == "" {
				i.pastedContent = pm.Content
				i.ta.SetValue(fmt.Sprintf("[pasted %d lines]", len(lines)))
			} else {
				i.pastedContent = existing + "\n" + pm.Content
				i.ta.SetValue(fmt.Sprintf("%s [pasted %d lines]", existing, len(lines)))
			}
			i.updateHeight()
			return i, nil
		}

		i.ta.InsertString(pm.Content)
		i.updateHeight()
		return i, nil
	}

	if km, ok := msg.(tea.KeyMsg); ok {
		// If we are showing a placeholder, expand it back to full content on any edit
		if i.pastedContent != "" && km.String() != "enter" {
			i.ta.SetValue(i.pastedContent)
			i.updateHeight()
			i.pastedContent = ""
		}

		// Consume Enter/Ctrl+M when input is empty - app handles sending, prevent textarea adding newline
		if (km.String() == "enter" || km.String() == "ctrl+m") && strings.TrimSpace(i.ta.Value()) == "" {
			return i, nil
		}

		switch km.String() {
		case "alt+up", "ctrl+p":
			if len(i.history) > 0 {
				if i.histIdx < len(i.history)-1 {
					i.histIdx++
				}
				idx := len(i.history) - 1 - i.histIdx
				i.ta.SetValue(i.history[idx])
				i.updateHeight()
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
			i.updateHeight()
			return i, nil
		}
	}
	ta, cmd := i.ta.Update(msg)
	i.ta = ta

	i.updateHeight()

	return i, cmd
}

// View renders the input.
func (i Input) View() string {
	if i.width <= 0 {
		return ""
	}

	rawView := i.ta.View()
	lines := strings.Split(rawView, "\n")

	var promptStyle lipgloss.Style
	if i.focused {
		promptStyle = lipgloss.NewStyle().Foreground(i.styles.T.Accent).Bold(true)
	} else {
		promptStyle = lipgloss.NewStyle().Foreground(i.styles.T.Subtext)
	}
	promptStr := promptStyle.Render("❯ ")

	var renderedLines []string
	for idx, line := range lines {
		if idx == len(lines)-1 {
			renderedLines = append(renderedLines, promptStr+line)
		} else {
			renderedLines = append(renderedLines, "  "+line)
		}
	}
	content := strings.Join(renderedLines, "\n")

	if i.focused {
		return i.styles.InputFocused.Width(i.width).Render(content)
	}
	return i.styles.Input.Width(i.width).Render(content)
}

// Focused reports whether the input has focus.
func (i Input) Focused() bool { return i.focused }
