package render

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// Diff renders a unified diff with color highlighting.
func Diff(content string) string {
	var sb strings.Builder
	// Catppuccin colors
	green := lipgloss.Color("#a6e3a1")
	red := lipgloss.Color("#f38ba8")
	blue := lipgloss.Color("#89b4fa")
	magenta := lipgloss.Color("#cba6f7")
	surface := lipgloss.Color("#313244")

	addStyle := lipgloss.NewStyle().Foreground(green).Background(surface).PaddingLeft(1)
	delStyle := lipgloss.NewStyle().Foreground(red).Background(surface).PaddingLeft(1)
	hunkStyle := lipgloss.NewStyle().Foreground(blue).Bold(true)
	fileStyle := lipgloss.NewStyle().Foreground(magenta).Bold(true).Underline(true)

	for _, line := range strings.Split(content, "\n") {
		if line == "" {
			continue
		}
		switch {
		case strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---"):
			sb.WriteString(" " + fileStyle.Render(line))
		case strings.HasPrefix(line, "@@"):
			sb.WriteString("\n" + hunkStyle.Render(line))
		case strings.HasPrefix(line, "+"):
			sb.WriteString(addStyle.Render(line))
		case strings.HasPrefix(line, "-"):
			sb.WriteString(delStyle.Render(line))
		default:
			sb.WriteString("  " + line)
		}
		sb.WriteByte('\n')
	}
	return sb.String()
}
