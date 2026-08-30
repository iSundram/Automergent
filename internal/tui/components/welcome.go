package components

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/iSundram/Automergent/internal/tui/themes"
)

// WelcomeView renders a welcome screen string suitable for use as the
// conversation's empty state. It shows a centered brand mark, tagline,
// a quick-start tip box, and a keybinding hint row.
func WelcomeView(styles *themes.Styles, width, height int) string {
	if width <= 0 {
		width = 80
	}

	// Box width: min(40, width-8)
	boxW := width - 8
	if boxW > 40 {
		boxW = 40
	}
	if boxW < 20 {
		boxW = 20
	}

	// ── Brand mark ──────────────────────────────────────────────────────────
	brandMark := lipgloss.NewStyle().
		Foreground(styles.T.Accent).
		Bold(true).
		Render("✦︎  AUTOMERGENT")

	tagline := lipgloss.NewStyle().
		Foreground(styles.T.Subtext).
		Render("Your AI coding agent")

	// ── Tip box ─────────────────────────────────────────────────────────────
	tipRows := []string{
		"  /new      Start a fresh task  ",
		"  @file     Reference a file    ",
		"  ?         Show all commands   ",
	}
	tipContent := strings.Join(tipRows, "\n")

	tipBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(styles.T.BorderNormal).
		Width(boxW).
		Render(tipContent)

	// Add "Get started" title to first border line by replacing it
	// ╭─ Get started ──────...╮
	tipLines := strings.Split(tipBox, "\n")
	if len(tipLines) > 0 {
		header := "─ Get started "
		top := tipLines[0] // e.g. "╭──────...──────╮"
		// Replace the interior dashes after ╭ with the header text
		if len(top) > 2 {
			// top[0] is '╭', top[last] is '╮'
			runes := []rune(top)
			inner := string(runes[1 : len(runes)-1])
			headerRunes := []rune(header)
			remaining := len([]rune(inner)) - len(headerRunes)
			if remaining < 0 {
				remaining = 0
			}
			newTop := string(runes[0:1]) + header + strings.Repeat("─", remaining) + string(runes[len(runes)-1:])
			tipLines[0] = newTop
			tipBox = strings.Join(tipLines, "\n")
		}
	}

	// ── Hint row ────────────────────────────────────────────────────────────
	hintStyle := lipgloss.NewStyle().Foreground(styles.T.Muted)
	hints := hintStyle.Render("ctrl+c  quit   ?  help   /  commands")

	// ── Center everything ───────────────────────────────────────────────────
	center := func(s string) string {
		return lipgloss.NewStyle().Width(width).Align(lipgloss.Center).Render(s)
	}

	parts := []string{
		center(brandMark),
		center(tagline),
		"", // blank line
		center(tipBox),
		center(hints),
	}

	return strings.Join(parts, "\n")
}
