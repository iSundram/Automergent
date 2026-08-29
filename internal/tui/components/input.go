package components

import (
	"fmt"
	"strings"
	"sync"
	"unicode"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/rivo/uniseg"

	"github.com/iSundram/Automergent/internal/tui/themes"
)

// Input is a multi-line text input component with history and undo/redo.
//
// All Update/View calls happen inside Bubble Tea's single-threaded event loop,
// so no mutex is needed around history or the undo stack.
type Input struct {
	showPrompt    bool
	ta            textarea.Model
	styles        *themes.Styles
	history       []string
	histIdx       int
	focused       bool
	width         int
	pastedContent string
	historyFile   string // optional persistent history path

	// undo/redo ring — snapshots of ta.Value() taken before destructive edits
	undoStack []string
	redoStack []string
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

	return Input{ta: ta, styles: styles, histIdx: -1, focused: true, showPrompt: true}
}

// WithHistoryFile returns a copy of the Input configured to persist history to
// path. Existing history from the file is loaded immediately.
func (i Input) WithHistoryFile(path string) Input {
	i.historyFile = path
	loaded := loadHistory(path)
	if len(loaded) > 0 {
		i.history = loaded
	}
	return i
}

// SetWidth updates the input width.
func (i *Input) SetWidth(w int) {
	i.width = w
	// Subtract horizontal padding (2) and the prompt/indent prefix (2 cols
	// for "❯ " or "  ") so that the textarea content never overflows the
	// lipgloss container and triggers an unwanted line-wrap.
	taW := w - 4
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

// SetValue updates the input text and moves the cursor to the end of the
// new value. Use this for programmatic value changes (history navigation,
// InsertValue); normal keystroke edits go through Update() directly.
//
// Order matters: updateHeight must run before MoveToEnd so the textarea's
// viewport has the correct dimensions when it computes the cursor position.
func (i *Input) SetValue(v string) {
	i.ta.SetValue(v)
	i.pastedContent = ""
	i.updateHeight() // establish correct height FIRST
	i.ta.MoveToEnd() // then anchor viewport with correct height
}

// Reset clears the input and saves the current value to history.
func (i *Input) Reset() {
	val := i.Value()
	if val != "" {
		// Deduplicate consecutive identical entries.
		if len(i.history) == 0 || i.history[len(i.history)-1] != val {
			i.history = append(i.history, val)
			if len(i.history) > maxHistoryMem {
				i.history = i.history[len(i.history)-maxHistoryMem:]
			}
			// Persist asynchronously (best-effort; no goroutine needed — the
			// file write is fast enough to stay in the update loop).
			appendHistory(i.historyFile, val)
		}
	}
	i.ta.Reset()
	i.ta.SetHeight(1)
	i.histIdx = -1
	i.pastedContent = ""
	i.undoStack = nil
	i.redoStack = nil
}

// SetPromptVisible toggles the ❯ prompt marker. The dock clears it while it
// owns the keyboard so exactly one surface shows the cursor.
func (i *Input) SetPromptVisible(v bool) {
	i.showPrompt = v
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

// LineCount returns the number of visual lines the input will occupy after
// the textarea's word-wrap. It faithfully mirrors the textarea's internal
// wrap() function so that updateHeight() always produces a box that exactly
// fits the content — no blank cursor-line bleed-through.
//
// Matching properties of the textarea wrap():
//   - Words are space-delimited by unicode.IsSpace, rune-by-rune.
//   - Widths are measured with uniseg.StringWidth (CJK/emoji correct).
//   - A trailing space is always counted on the last word of each visual line
//     (same as wrap()'s trailing-space append), so >= is used on the final
//     flush instead of >, which is the most common off-by-one source.
func (i Input) LineCount() int {
	val := i.Value()
	if val == "" {
		return 1
	}

	width := i.ta.Width()
	if width < 10 {
		width = 10
	}

	paragraphs := strings.Split(val, "\n")
	totalLines := 0
	for _, para := range paragraphs {
		totalLines += wrapLineCount([]rune(para), width)
	}

	if totalLines < 1 {
		return 1
	}
	return totalLines
}

// wrapLineCount is a faithful port of the textarea's internal wrap() function.
// It counts the number of soft-wrapped visual lines that runes would occupy
// at the given width using the same algorithm the textarea uses when rendering.
func wrapLineCount(runes []rune, width int) int {
	if len(runes) == 0 {
		return 1
	}

	var (
		lineW  int    // accumulated width of the current visual line
		wordW  int    // accumulated width of the current word being built
		word   []rune // current word runes
		spaces int    // trailing spaces after last flushed word
		lines  = 1
	)

	for _, r := range runes {
		if unicode.IsSpace(r) {
			spaces++
			if len(word) > 0 {
				// Flush word: check if line + word + spaces exceed width.
				if lineW+wordW+spaces > width {
					lines++
					lineW = wordW + spaces
				} else {
					lineW += wordW + spaces
				}
				word = word[:0]
				wordW = 0
				spaces = 0
			}
		} else {
			rw := uniseg.StringWidth(string(r))
			word = append(word, r)
			wordW += rw

			// Double-width guard: if word alone already overflows, hard-break.
			// Uses wordW + rw (last char counted twice) like the textarea does.
			if wordW+rw > width {
				if lineW > 0 {
					lines++
					lineW = 0
				}
				lineW = wordW
				word = word[:0]
				wordW = 0
				spaces = 0
			}
		}
	}

	// Final flush — mirrors wrap()'s >= condition exactly. When text fills the
	// line to width, wrap() creates a new trailing row for the cursor anchor;
	// we must count that row so SetHeight allocates space for it.
	if lineW+wordW+spaces >= width {
		lines++
	}

	if lines < 1 {
		return 1
	}
	return lines
}

func (i *Input) updateHeight() {
	lineCount := i.LineCount()
	if lineCount > i.ta.MaxHeight {
		lineCount = i.ta.MaxHeight
	}
	if lineCount < 1 {
		lineCount = 1
	}
	oldHeight := i.ta.Height()
	i.ta.SetHeight(lineCount)
	// When the visual height changes, the textarea may have already scrolled
	// its viewport DOWN while processing the keystroke with the stale height:
	// its internal repositionView() only keeps the cursor row inside
	// [YOffset, YOffset+height-1] — it never scrolls back up once content fits.
	// After the height grows, that leaves earlier wrapped rows hidden above the
	// viewport (only the newest character visible). Re-anchor explicitly:
	// MoveToBegin forces a ScrollUp back to offset 0 (or clamps for content
	// taller than MaxHeight), then MoveToEnd restores the cursor position.
	if lineCount != oldHeight {
		i.ta.MoveToBegin()
		i.ta.MoveToEnd()
	}
}

// ----------------------------------------------------------------------------
// Undo / Redo
// ----------------------------------------------------------------------------

const maxUndoStack = 50

// pushUndo saves the current textarea value onto the undo stack and clears
// the redo stack (a new edit invalidates redo history).
func (i *Input) pushUndo() {
	snap := i.ta.Value()
	i.undoStack = append(i.undoStack, snap)
	if len(i.undoStack) > maxUndoStack {
		i.undoStack = i.undoStack[len(i.undoStack)-maxUndoStack:]
	}
	i.redoStack = nil
}

// undo restores the previous snapshot, if any.
func (i *Input) undo() {
	if len(i.undoStack) == 0 {
		return
	}
	// Push current state to redo before overwriting.
	i.redoStack = append(i.redoStack, i.ta.Value())
	snap := i.undoStack[len(i.undoStack)-1]
	i.undoStack = i.undoStack[:len(i.undoStack)-1]
	i.ta.SetValue(snap)
	i.pastedContent = ""
	i.updateHeight() // height before MoveToEnd so viewport is correctly sized
	i.ta.MoveToEnd()
}

// redo reapplies the last undone snapshot.
func (i *Input) redo() {
	if len(i.redoStack) == 0 {
		return
	}
	i.undoStack = append(i.undoStack, i.ta.Value())
	snap := i.redoStack[len(i.redoStack)-1]
	i.redoStack = i.redoStack[:len(i.redoStack)-1]
	i.ta.SetValue(snap)
	i.pastedContent = ""
	i.updateHeight() // height before MoveToEnd so viewport is correctly sized
	i.ta.MoveToEnd()
}

// ----------------------------------------------------------------------------
// SlashSubPalettes & trigger detection
// ----------------------------------------------------------------------------

// SlashSubPalettes lists slash commands whose argument completion is rendered
// as a dedicated palette (model, provider, mode, theme, keybindings and
// effort pickers). This map is the single source of truth shared by
// TriggerType, InsertValue/TriggerValue and the app's palette enter handling;
// keys must be registered non-Immediate command names.
// slashSubMu guards SlashSubPalettes against concurrent reads/writes.
var slashSubMu sync.RWMutex

var SlashSubPalettes = map[string]bool{
	"model":       true,
	"provider":    true,
	"mode":        true,
	"theme":       true,
	"keybindings": true,
	"effort":      true,
	"run":         true,
	"commit":      true,
	"review":      true,
	"mcp":         true,
	"commands":    true,
	"directory":   true,
	"plan":        true,
	"goal":        true,
}

// RegisterSubPalette registers name as a slash sub-palette command so that
// plugins and external commands can self-register without editing input.go.
func RegisterSubPalette(name string) {
	slashSubMu.Lock()
	SlashSubPalettes[name] = true
	slashSubMu.Unlock()
}

// isSubPalette reports whether name is a registered sub-palette command.
// It is safe to call from multiple goroutines.
func isSubPalette(name string) bool {
	slashSubMu.RLock()
	ok := SlashSubPalettes[name]
	slashSubMu.RUnlock()
	return ok
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

// wordUnderCursor returns the whitespace-delimited word that the textarea
// cursor is currently inside (or just after), using the raw textarea value and
// cursor column position.
//
// col is a rune-count column (as returned by textarea.Column()). We convert it
// to a byte offset before slicing so multi-byte characters (CJK, emoji) don't
// cause a mid-rune boundary panic or wrong word extraction.
func wordUnderCursor(val string, runeCol int) string {
	if runeCol < 0 {
		runeCol = 0
	}
	// Convert rune column → byte offset.
	byteCol := len(val) // default: end of string
	n := 0
	for i, r := range val {
		if n >= runeCol {
			byteCol = i
			break
		}
		n++
		_ = r
	}
	// Work on the single logical line up to the cursor.
	start := strings.LastIndexAny(val[:byteCol], " \t\n")
	if start < 0 {
		start = 0
	} else {
		start++ // skip the delimiter itself
	}
	end := strings.IndexAny(val[byteCol:], " \t\n")
	if end < 0 {
		end = len(val)
	} else {
		end += byteCol
	}
	return val[start:end]
}

// TriggerType returns the current palette trigger if any.
// Sub-palettes only open after a trailing space: "/mode" stays as "command"
// (showing the /mode entry), "/mode " opens the mode picker. This lets the
// user see the single /mode command first and preserves free-form input.
func (i Input) TriggerType() string {
	val := i.ta.Value()
	if val == "?" {
		return "help"
	}
	if strings.HasPrefix(val, "/") {
		if token := slashCommandToken(val); isSubPalette(token) {
			rest := strings.TrimPrefix(val, "/"+token)
			if rest != "" && (rest[0] == ' ' || rest[0] == '\t' || rest[0] == '\n') {
				return token
			}
			// Exact "/mode" stays as command so the palette shows the single entry.
			// Only "/mode " or "/mode arg" opens the sub-palette.
		}
		return "command"
	}
	if strings.Contains(val, "@") {
		// Use cursor position for precise detection so mid-line @-mentions
		// work correctly even when the cursor is inside the token.
		col := i.ta.Column()
		// Flatten multi-line value to single line for column lookup.
		flatVal := strings.ReplaceAll(val, "\n", " ")
		w := wordUnderCursor(flatVal, col)
		if strings.HasPrefix(w, "@") {
			return "file"
		}
		// Only trigger when the user has just typed a bare @.
		if strings.HasSuffix(strings.TrimRight(val, " \t"), "@") {
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
		// Return only the command token (e.g. "/context-fil" → "context-fil")
		// so filtering works correctly even when extra text follows.
		return slashCommandToken(val)
	case "file":
		idx := strings.LastIndex(val, "@")
		if idx != -1 {
			return val[idx+1:]
		}
		return ""
	default:
		if isSubPalette(trigger) {
			return strings.TrimSpace(strings.TrimPrefix(val, "/"+trigger))
		}
		return ""
	}
}

// InsertValue completes the current trigger with the selected value.
func (i *Input) InsertValue(v string) {
	i.pushUndo()
	val := i.ta.Value()
	trigger := i.TriggerType()
	switch trigger {
	case "help", "command":
		i.ta.SetValue("/" + v + " ")
	case "file":
		idx := strings.LastIndex(val, "@")
		if idx != -1 {
			i.ta.SetValue(val[:idx] + "@" + v + " ")
		} else {
			// Fallback: append to end.
			i.ta.SetValue(val + "@" + v + " ")
		}
	default:
		if isSubPalette(trigger) {
			i.ta.SetValue("/" + trigger + " " + v + " ")
		}
	}
	i.ta.MoveToEnd()
	i.updateHeight()
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
		// If we are showing a pasted-content placeholder, expand it back to
		// full content on any key except enter (enter sends the pasted content
		// via Value(), so we leave the placeholder visible until Reset clears
		// it; on any edit key we restore immediately so the user can edit).
		if i.pastedContent != "" && km.String() != "enter" && km.String() != "ctrl+m" {
			i.ta.SetValue(i.pastedContent)
			i.pastedContent = ""
			i.updateHeight() // height first, then reanchor cursor
			i.ta.MoveToEnd()
		}

		// Consume Enter/Ctrl+M when input is empty - app handles sending,
		// prevent textarea adding newline.
		if (km.String() == "enter" || km.String() == "ctrl+m") && strings.TrimSpace(i.ta.Value()) == "" {
			return i, nil
		}

		switch km.String() {
		// History navigation
		case "alt+up", "ctrl+p":
			if len(i.history) > 0 {
				if i.histIdx < len(i.history)-1 {
					i.histIdx++
				}
				idx := len(i.history) - 1 - i.histIdx
				i.ta.SetValue(i.history[idx])
				i.updateHeight() // height first, then reanchor cursor
				i.ta.MoveToEnd()
				return i, nil
			}
		case "alt+down", "ctrl+n":
			if i.histIdx > 0 {
				i.histIdx--
				idx := len(i.history) - 1 - i.histIdx
				i.ta.SetValue(i.history[idx])
				i.updateHeight() // height first, then reanchor cursor
				i.ta.MoveToEnd()
			} else if i.histIdx == 0 {
				i.histIdx = -1
				i.ta.SetValue("")
				i.updateHeight()
			}
			return i, nil

		// Undo / Redo
		case "ctrl+z":
			i.undo()
			return i, nil
		case "ctrl+y", "ctrl+shift+z":
			i.redo()
			return i, nil
		}

		// For regular typing, snapshot for undo before passing to textarea.
		// Only snapshot on printable input to avoid polluting the undo stack
		// with cursor-movement operations.
		if isPrintableKey(km) {
			i.pushUndo()
		}
	}

	ta, cmd := i.ta.Update(msg)
	i.ta = ta

	i.updateHeight()

	return i, cmd
}

// isPrintableKey returns true for key messages that produce visible text
// changes (single chars, backspace, delete) so we only snapshot those.
// We match on the string representation which is stable across v2.
func isPrintableKey(km tea.KeyMsg) bool {
	s := km.String()
	switch s {
	case "backspace", "delete", "ctrl+h":
		return true
	}
	// Single printable rune (no modifier prefix in string means no ctrl/alt).
	runes := []rune(s)
	return len(runes) == 1 && runes[0] >= 32
}

// View renders the input.
//
// NOTE: Value receiver intentionally avoided — textarea.Model contains pointer
// fields (viewport, cache) that get shallow-copied and can have their state
// corrupted when View() is called on a copy. Pointer receiver prevents the
// copy entirely.
func (i *Input) View() string {
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
	promptStr := ""
	if i.showPrompt {
		promptStr = promptStyle.Render("▸ ")
	}

	// ❯ prefixes the FIRST line — it is an input marker, not a cursor
	// follower. The textarea's last rendered line is always a cursor-anchor
	// row (trailing space added by wrap()); placing the prompt there would
	// make it appear on a blank line whenever text fills the full width.
	var renderedLines []string
	for idx, line := range lines {
		if idx == 0 {
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
func (i *Input) Focused() bool { return i.focused }
