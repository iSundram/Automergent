package components

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/iSundram/Automergent/internal/tui/themes"
)

// WelcomeView renders a welcome screen suitable for use as the
// conversation's empty state: a centered brand mark, tagline, a quick-start
// tip box titled "Get started", and a keybinding hint row. Content is
// centered for WIDTH (the conversation's content width, not the window) and
// vertically centered within HEIGHT when positive.
func WelcomeView(styles *themes.Styles, width, height int) string {
	if width <= 0 {
		width = 80
	}

	// Tip box width: comfortable at 40, shrinking for narrow terminals.
	boxW := width - 8
	if boxW > 40 {
		boxW = 40
	}
	if boxW < 20 {
		boxW = 20
	}

	// ── Brand mark ──────────────────────────────────────────────────────────
	// No variation selector on the glyph: some terminals count U+FE0E as its
	// own cell, which throws centering off by one column.
	brandMark := lipgloss.NewStyle().
		Foreground(styles.T.Accent).
		Bold(true).
		Render("✦  AUTOMERGENT")

	tagline := lipgloss.NewStyle().
		Foreground(styles.T.Subtext).
		Render("Your AI coding agent")

	// ── Tip box ─────────────────────────────────────────────────────────────
	// Columns are computed, not hand-spaced, so label edits cannot break
	// the alignment.
	tips := [][2]string{
		{"/new", "Start a fresh task"},
		{"@file", "Reference a file"},
		{"?", "Show all commands"},
	}
	rows := make([]string, 0, len(tips))
	for _, tip := range tips {
		rows = append(rows, fmt.Sprintf("  %-9s %s", tip[0], tip[1]))
	}
	tipContent := strings.Join(rows, "\n")

	tipBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(styles.T.BorderNormal).
		Width(boxW).
		Render(tipContent)

	tipBox = withBorderTitle(tipBox, "Get started")

	// ── Hint row ────────────────────────────────────────────────────────────
	hints := lipgloss.NewStyle().
		Foreground(styles.T.Muted).
		Render("ctrl+c  quit   ?  help   /  commands")

	// ── Center everything ───────────────────────────────────────────────────
	center := lipgloss.NewStyle().Width(width).Align(lipgloss.Center)
	body := strings.Join([]string{
		center.Render(brandMark),
		center.Render(tagline),
		"",
		center.Render(tipBox),
		center.Render(hints),
	}, "\n")

	if height > 0 {
		return lipgloss.PlaceVertical(height, lipgloss.Center, body)
	}
	return body
}

// withBorderTitle rewrites a lipgloss box's top border to embed a title
// (╭─ Title ───╮). It only rewrites when the top border's interior is
// entirely box-drawing dashes — anything else (padding, a future border
// change) means the geometry assumption is wrong, and the box is returned
// undecorated rather than rendered ragged.
func withBorderTitle(box, title string) string {
	lines := strings.Split(box, "\n")
	if len(lines) == 0 {
		return box
	}
	runes := []rune(lines[0])
	if len(runes) < 2 {
		return box
	}
	inner := string(runes[1 : len(runes)-1])
	if inner == "" {
		return box
	}
	for _, r := range inner {
		if r != '─' {
			return box // not a plain top border; decoration would corrupt it
		}
	}
	header := "─ " + title + " "
	remaining := len([]rune(inner)) - len([]rune(header))
	if remaining < 0 {
		// Title wider than the border: truncate the title to the interior
		// rather than emit a top border longer than the body.
		header = string([]rune(header)[:len([]rune(inner))])
		remaining = 0
	}
	lines[0] = string(runes[0:1]) + header + strings.Repeat("─", remaining) + string(runes[len(runes)-1:])
	return strings.Join(lines, "\n")
}
