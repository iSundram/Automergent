package components

// Detail-block primitives shared by the family renderers: the terminal slab,
// the diff box, and the tail-truncation helpers behind both.
//
// Every block shows the LAST N lines — for a command or a patch, the tail is
// what matters — and reports what it hid so ctrl+e is a discoverable escape
// hatch rather than a secret.

import (
	"fmt"
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/iSundram/Automergent/internal/tui/render"
)

const (
	defaultTailLines = 8
	maxTailLines     = 40
)

// diffStats counts added/removed lines in unified-diff-ish text.
func diffStats(text string) (added, removed int) {
	for _, line := range strings.Split(text, "\n") {
		switch {
		case strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++"):
			added++
		case strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---"):
			removed++
		}
	}
	return added, removed
}

// tailLines returns the last n lines plus how many were cut above.
func tailLines(text string, n int) (shown []string, hidden int) {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	if len(lines) <= n {
		return lines, 0
	}
	return lines[len(lines)-n:], len(lines) - n
}

// tailLimit derives how many tail lines to show from the viewport height.
// The three expand modes map directly: Full/review shows everything,
// Compact shows the single-line summary tail, Auto uses the classic
// height-scaled default.
func (c *Conversation) tailLimit() int {
	if c.expandMode == ExpandFull || c.reviewMode {
		return maxTailLines
	}
	if c.expandMode == ExpandCompact {
		// Collapsed: command row plus a one-line outcome tail. The
		// hintRow below reports how much is hidden and names /expand.
		return 1
	}
	return clampInt(c.height/5, defaultTailLines-3, maxTailLines)
}

// slabBackground is the terminal slab's fill: the theme's Surface slot, so the
// block reads as a distinct panel without fighting the user's palette with a
// hardcoded near-black.
func (c *Conversation) slabBackground() color.Color {
	return c.styles.T.Surface
}

// slab paints plain-text rows as a full-width terminal block. Rows are
// truncated and padded to an exact visual width so the background covers every
// row edge-to-edge — lipgloss Width+Padding double-wraps long output lines,
// which is why this is built by hand.
func (c *Conversation) slab(rows []string, width int) string {
	inner := width - lipgloss.Width(rowIndent) - 4
	if inner < 14 {
		inner = 14
	}
	bg := lipgloss.NewStyle().Background(c.slabBackground())

	painted := make([]string, 0, len(rows))
	for _, r := range rows {
		if r == "" {
			painted = append(painted, bg.Render(strings.Repeat(" ", inner+2)))
			continue
		}
		painted = append(painted, r)
	}
	return indentLines(strings.Join(painted, "\n"), len(rowIndent))
}

// detab expands tabs to spaces so painted widths match what the terminal
// shows — tab stops would otherwise desync the edge-to-edge background fill.
func detab(s string) string {
	return strings.ReplaceAll(s, "\t", "    ")
}

// slabRow paints one plain-text line as a full-width slab row.
func (c *Conversation) slabRow(s string, inner int) string {
	bg := lipgloss.NewStyle().Background(c.slabBackground())
	s = strings.TrimRight(detab(s), " \t")
	s = ansiSafeTruncate(s, inner)
	pad := inner - lipgloss.Width(s)
	if pad < 0 {
		pad = 0
	}
	return bg.Render(" " + s + strings.Repeat(" ", pad) + " ")
}

// slabRowTrailer paints one slab row with a right-aligned trailer chip sharing
// it — a live "running…", an exit status. The left side shrinks so the trailer
// always survives intact; an empty trailer yields a plain painted row. Width
// math is ANSI-aware throughout: left may arrive pre-styled (the green $).
func (c *Conversation) slabRowTrailer(left, trailer string, inner int) string {
	bg := lipgloss.NewStyle().Background(c.slabBackground())
	tw := lipgloss.Width(trailer)
	room := inner - tw - 3
	if room < 8 {
		room = 8
	}
	left = strings.TrimRight(detab(left), " \t")
	if lipgloss.Width(left) > room {
		left = ansi.Truncate(left, room, "")
	}
	pad := inner - tw - lipgloss.Width(left)
	if pad < 1 {
		pad = 1
	}
	return bg.Render(" " + left + strings.Repeat(" ", pad) + trailer + " ")
}

// slabInner is the usable text width inside a slab at a given card width.
func slabInner(width int) int {
	inner := width - lipgloss.Width(rowIndent) - 4
	if inner < 14 {
		inner = 14
	}
	return inner
}

// diffBox renders a file-change preview: a tinted diff of the LAST N lines —
// the exact rendering the fullscreen viewer uses, so ctrl+e zooms rather than
// translates — under a "+added −removed" stat rule covering the WHOLE diff,
// plus an explicit count of what the tail hid so the escape hatch stays
// discoverable.
//
//	● Edit  tool_box.go  ·  1 line replaced                             8ms
//	  +1 −1  ──────────────────────────────────────
//	   34 │ limit := c.tailLimit()
//	   35 │ limit := c.tailLimit() + 2
//	  ↑ first 3 hidden · ctrl+e expands
func (c *Conversation) diffBox(diffText string, width int) string {
	if strings.TrimSpace(diffText) == "" {
		return ""
	}
	added, removed := diffStats(diffText)

	limit := c.tailLimit()
	shown, hidden := tailLines(diffText, limit)
	inner := slabInner(width)
	body := unifiedTail(strings.Join(shown, "\n"), inner)

	var b strings.Builder

	// Stat strip: colored counts followed by a hairline out to the card edge.
	// The summary sits above its block instead of orphaned below it.
	stat := c.diffStatChip(added, removed)
	ruleW := inner - lipgloss.Width(stat) - 3
	if ruleW < 3 {
		ruleW = 3
	}
	b.WriteString(rowIndent + stat + "  " +
		c.styles.Dim.Render(strings.Repeat("─", ruleW)))

	// Body: borderless tinted rows edge-to-edge. DiffWithWidth pads every
	// add/delete background to full width, which is what makes the block read
	// as one object without needing border chrome around it.
	if body != "" {
		b.WriteString("\n" + indentLines(render.DiffWithWidth(body, inner), len(rowIndent)))
	}

	if hidden > 0 {
		hint := fmt.Sprintf("%s first %d hidden · %s", glyphUp, hidden, c.expandHintVerb())
		b.WriteString("\n" + rowIndent + c.styles.Dim.Render(hint))
	}
	return b.String()
}

// diffStatChip renders "+a −b" from the active theme's greens and reds. A
// silent half drops out; an all-context tail still gets its quiet "+0".
func (c *Conversation) diffStatChip(added, removed int) string {
	green := lipgloss.NewStyle().Foreground(c.styles.T.Green)
	red := lipgloss.NewStyle().Foreground(c.styles.T.Red)
	var parts []string
	if added > 0 || removed == 0 {
		parts = append(parts, green.Render(fmt.Sprintf("+%d", added)))
	}
	if removed > 0 {
		parts = append(parts, red.Render(fmt.Sprintf("-%d", removed)))
	}
	return strings.Join(parts, " ")
}

// unifiedTail normalizes proposal lines to unified-diff convention — context
// rows carry a leading space, which DiffWithWidth strips for display — and
// clips each row so the numbered, background-padded output fits the slab's
// inner width exactly.
func unifiedTail(text string, inner int) string {
	budget := inner - 6
	if budget < 8 {
		budget = 8
	}
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	for i, l := range lines {
		var prefixed bool
		switch {
		case l == "":
			// blank row stays blank
		case strings.HasPrefix(l, "+++") || strings.HasPrefix(l, "---"):
			// file headers keep their marker
		case strings.HasPrefix(l, "@@"):
			// hunk separator
		case strings.HasPrefix(l, "+ ") || strings.HasPrefix(l, "- "):
			// some producers emit "+ line"; drop the space now so the
			// renderer's one-char strip doesn't leave a stray indent
			l = l[:1] + l[2:]
			prefixed = true
		case strings.HasPrefix(l, "+"), strings.HasPrefix(l, "-"):
			prefixed = true
		default:
			prefixed = true // context row: DiffWithWidth expects the space
			l = ansiSafeTruncate(l, budget)
			if l != "" {
				l = " " + l
			}
			lines[i] = l
			continue
		}
		if prefixed {
			l = ansiSafeTruncate(l, budget)
		}
		lines[i] = l
	}
	return strings.Join(lines, "\n")
}
