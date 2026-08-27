package components

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/iSundram/Automergent/internal/tui/themes"
	"github.com/iSundram/Automergent/internal/tui/tips"
)

// InfoLine is the `└─` line under the thinking spinner (or under the prompt
// when idle). It answers "what is happening, and what do I do about it" in one
// dim line:
//
//	⣾ thinking · 42 tok/s
//	└─ esc interrupts · ctrl+c stops · enter queues a message for after this turn
//
// It is always rendered, so the keyboard affordances for a state are
// discoverable without opening help.
type InfoLine struct {
	styles *themes.Styles
	width  int
	state  tips.State
	ctx    tips.Context
}

// NewInfoLine creates the info line.
func NewInfoLine(styles *themes.Styles) InfoLine {
	return InfoLine{styles: styles, state: tips.StateIdle}
}

// SetWidth updates the available width.
func (i *InfoLine) SetWidth(w int) { i.width = w }

// Set records the state and context the line describes.
func (i *InfoLine) Set(state tips.State, ctx tips.Context) {
	i.state = state
	i.ctx = ctx
}

// State returns the state currently being described.
func (i InfoLine) State() tips.State { return i.state }

// Text returns the unstyled info text, for tests and for callers that need the
// message without the tree glyph.
func (i InfoLine) Text() string { return tips.Info(i.state, i.ctx) }

// View renders the info line. Returns "" when there is no room for it.
func (i InfoLine) View() string {
	if i.width <= 0 {
		return ""
	}
	text := strings.TrimSpace(i.Text())
	if text == "" {
		return ""
	}

	prefix := "└─ "
	// 2 for the leading indent that aligns under the spinner glyph.
	avail := i.width - len(prefix) - 2
	if avail < 8 {
		// Too narrow for the glyph plus a readable sentence: drop the prefix
		// rather than the whole instruction. An empty line would hide the
		// state's keyboard affordances exactly when space is tightest; only
		// give up when not even a few characters fit.
		prefix = ""
		avail = i.width - 2
		if avail < 4 {
			return ""
		}
	}
	if ansi.StringWidth(text) > avail {
		text = ansi.Truncate(text, avail, "…")
	}

	glyph := lipgloss.NewStyle().Foreground(i.styles.T.BorderNormal).Render(prefix)
	body := lipgloss.NewStyle().Foreground(i.styles.T.Muted).Render(text)
	return "  " + glyph + body
}
