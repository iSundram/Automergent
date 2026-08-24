package components

// Shared truncation + preview box primitives for per-tool renderers.
// Every boxed preview shows the LAST N lines (the tail is what matters),
// sits on a subtly different background, and ends with a scroll hint —
// diffs additionally end with a "+added −removed" stat line.

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
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

// outputBox renders a terminal-style output tail: dim rounded box, last
// maxLines lines, and a "↑ N more" scroll hint when truncated.
func (c *Conversation) outputBox(text string, width int) string {
	if strings.TrimSpace(text) == "" {
		return ""
	}
	limit := c.tailLimit()
	shown, hidden := tailLines(text, limit)
	if len(shown) == 0 {
		return ""
	}
	body := strings.Join(shown, "\n")

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(c.styles.T.BorderNormal).
		Background(c.styles.T.Background).
		Padding(0, 1).
		Width(width - 6)

	var b strings.Builder
	b.WriteString(box.Render(body))
	if hidden > 0 {
		b.WriteString("\n  " + c.styles.Dim.Render(fmt.Sprintf("↑ %d more lines — ctrl+e expands", hidden)))
	}
	return b.String()
}

// diffBox renders a file-change preview: syntax-colored diff of the LAST
// maxLines lines inside the same boxed background, ending with a right-
// aligned "+added −removed" stat line covering the WHOLE diff.
func (c *Conversation) diffBox(diffText string, width int) string {
	if strings.TrimSpace(diffText) == "" {
		return ""
	}
	added, removed := diffStats(diffText)

	limit := c.tailLimit()
	shown, hidden := tailLines(diffText, limit)
	body := strings.Join(shown, "\n")

	colored := renderDiffColored(body, width)
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(c.styles.T.BorderNormal).
		Padding(0, 1).
		Width(width - 6)

	var b strings.Builder
	b.WriteString(box.Render(colored))
	hint := ""
	if hidden > 0 {
		hint = fmt.Sprintf("  ↑ %d more — ctrl+e expands", hidden)
	}
	green := lipgloss.NewStyle().Foreground(c.styles.T.Green)
	red := lipgloss.NewStyle().Foreground(c.styles.T.Red)
	statLine := green.Render(fmt.Sprintf("+%d", added))
	if removed > 0 {
		statLine += " " + red.Render(fmt.Sprintf("−%d", removed))
	}
	b.WriteString("\n  " + statLine + c.styles.Dim.Render(hint))
	return b.String()
}

// renderDiffColored applies +/- coloring per line.
func renderDiffColored(diff string, width int) string {
	var b strings.Builder
	for _, line := range strings.Split(diff, "\n") {
		line = ansiSafeTruncate(line, width-8)
		switch {
		case strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++"):
			b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Render(line))
		case strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---"):
			b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render(line))
		default:
			b.WriteString(line)
		}
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// tailLimit derives how many tail lines to show from the viewport height.
func (c *Conversation) tailLimit() int {
	limit := clampInt(c.height/5, defaultTailLines-3, maxTailLines)
	if limit < 4 {
		limit = 4
	}
	return limit
}
