package components

// Scroll-to-end affordance.
//
// The conversation follows the bottom while the user is already at the bottom,
// and deliberately does not while they have scrolled up to read something. That
// second case used to be silent: output kept arriving off-screen with no
// indication that the view was stale or how to get back. ScrollEnd is the
// missing half — a small pill that appears only when the view is behind, says
// how far, and names the key that returns.

import (
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/iSundram/Automergent/internal/tui/themes"
)

// ScrollEnd renders the "jump to latest" pill.
type ScrollEnd struct {
	styles *themes.Styles
	width  int

	visible bool
	unseen  int // messages/tool cards appended while scrolled away
	behind  int // lines between the current view and the bottom
}

// NewScrollEnd returns a hidden pill.
func NewScrollEnd(styles *themes.Styles) ScrollEnd {
	return ScrollEnd{styles: styles}
}

// SetWidth sets the width the pill may occupy.
func (s *ScrollEnd) SetWidth(w int) { s.width = w }

// Set updates the pill state. unseen is how many rows were appended while the
// view was scrolled away; behind is how many lines separate the view from the
// bottom.
func (s *ScrollEnd) Set(visible bool, unseen, behind int) {
	s.visible = visible
	s.unseen = unseen
	s.behind = behind
}

// Visible reports whether the pill would render.
func (s ScrollEnd) Visible() bool { return s.visible && s.width > 0 }

// View renders the pill, or "" when it should not show.
func (s ScrollEnd) View() string {
	if !s.Visible() {
		return ""
	}
	label := s.label()
	if label == "" {
		return ""
	}
	// Reverse-ish fill: the pill sits on top of conversation text, so it needs
	// its own background to stay readable.
	pill := lipgloss.NewStyle().
		Foreground(s.styles.T.Background).
		Background(s.styles.T.Accent).
		Bold(true).
		Padding(0, 1).
		Render(label)
	if ansi.StringWidth(pill) > s.width {
		return ansi.Truncate(pill, s.width, "")
	}
	return pill
}

// label picks the most useful thing the pill can say in the space it has.
func (s ScrollEnd) label() string {
	switch {
	case s.unseen == 1:
		return "↓ 1 new · end"
	case s.unseen > 1:
		return "↓ " + strconv.Itoa(s.unseen) + " new · end"
	case s.behind > 0:
		return "↓ " + strconv.Itoa(s.behind) + " lines · end"
	default:
		return "↓ end"
	}
}

// Overlay draws the pill right-aligned on the last line of content, leaving
// every other line untouched.
//
// It overlays rather than occupying a row of its own: taking a row would resize
// the conversation the moment the user scrolls, which shifts the very text they
// scrolled up to read.
func (s ScrollEnd) Overlay(content string) string {
	pill := s.View()
	if pill == "" {
		return content
	}
	lines := strings.Split(content, "\n")
	if len(lines) == 0 {
		return content
	}
	i := len(lines) - 1
	pillW := ansi.StringWidth(pill)
	// One trailing cell of breathing room from the right edge.
	target := s.width - pillW - 1
	if target < 0 {
		target = 0
	}

	base := lines[i]
	kept := ansi.Truncate(base, target, "")
	if pad := target - ansi.StringWidth(kept); pad > 0 {
		kept += strings.Repeat(" ", pad)
	}
	lines[i] = kept + pill
	return strings.Join(lines, "\n")
}
