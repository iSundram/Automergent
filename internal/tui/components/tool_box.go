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

	"github.com/iSundram/Automergent/internal/tui/themes"
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
func (c *Conversation) tailLimit() int {
	if c.expandMode == ExpandFull || c.reviewMode {
		return maxTailLines
	}
	return clampInt(c.height/5, defaultTailLines-3, maxTailLines)
}

// slabBackground derives the terminal slab's fill from the theme instead of a
// hardcoded black, so light themes get a dark-but-not-void slab and dark
// themes still get the near-black terminal look.
func (c *Conversation) slabBackground() color.Color {
	bg := c.styles.T.Background
	if themes.IsDark(bg) {
		// Push toward true black so the slab reads as a distinct surface.
		return themes.Mix(bg, color.RGBA{A: 255}, 0.72)
	}
	// On light themes a full-black slab is jarring; use the theme's darkest
	// structural color instead.
	return themes.Mix(bg, c.styles.T.Text, 0.88)
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

// slabRow paints one plain-text line as a full-width slab row.
func (c *Conversation) slabRow(s string, inner int) string {
	bg := lipgloss.NewStyle().Background(c.slabBackground())
	s = strings.TrimRight(s, " \t")
	s = ansiSafeTruncate(s, inner)
	pad := inner - lipgloss.Width(s)
	if pad < 0 {
		pad = 0
	}
	return bg.Render(" " + s + strings.Repeat(" ", pad) + " ")
}

// slabInner is the usable text width inside a slab at a given card width.
func slabInner(width int) int {
	inner := width - lipgloss.Width(rowIndent) - 4
	if inner < 14 {
		inner = 14
	}
	return inner
}

// diffBox renders a file-change preview: a syntax-colored diff of the LAST N
// lines in a rounded box, followed by a "+added −removed" stat line covering
// the WHOLE diff (not just the visible tail).
func (c *Conversation) diffBox(diffText string, width int) string {
	if strings.TrimSpace(diffText) == "" {
		return ""
	}
	added, removed := diffStats(diffText)

	limit := c.tailLimit()
	shown, hidden := tailLines(diffText, limit)
	body := strings.Join(shown, "\n")

	boxWidth := width - lipgloss.Width(rowIndent) - 2
	if boxWidth < 20 {
		boxWidth = 20
	}
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(c.styles.T.BorderNormal).
		Padding(0, 1).
		Width(boxWidth)

	var b strings.Builder
	b.WriteString(indentLines(box.Render(c.diffColored(body, boxWidth)), len(rowIndent)))

	green := lipgloss.NewStyle().Foreground(c.styles.T.Green)
	red := lipgloss.NewStyle().Foreground(c.styles.T.Red)
	stat := green.Render(fmt.Sprintf("+%d", added))
	if removed > 0 {
		stat += " " + red.Render(fmt.Sprintf("−%d", removed))
	}
	b.WriteString("\n" + rowIndent + stat)
	if hidden > 0 {
		b.WriteString(c.styles.Dim.Render(fmt.Sprintf("  %s %d more — ctrl+e expands", glyphUp, hidden)))
	}
	return b.String()
}

// diffColored applies +/- coloring per line using the ACTIVE THEME's green and
// red rather than fixed ANSI 9/10, so diffs follow the user's palette.
func (c *Conversation) diffColored(diff string, width int) string {
	add := lipgloss.NewStyle().Foreground(c.styles.T.Green)
	del := lipgloss.NewStyle().Foreground(c.styles.T.Red)
	ctx := lipgloss.NewStyle().Foreground(c.styles.T.Subtext)

	var b strings.Builder
	for i, line := range strings.Split(diff, "\n") {
		if i > 0 {
			b.WriteString("\n")
		}
		line = ansiSafeTruncate(line, width-4)
		switch {
		case strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++"):
			b.WriteString(add.Render(line))
		case strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---"):
			b.WriteString(del.Render(line))
		default:
			b.WriteString(ctx.Render(line))
		}
	}
	return b.String()
}
