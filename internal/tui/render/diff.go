package render

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Diff renders a unified diff with color highlighting.
func Diff(content string) string {
	var sb strings.Builder
	addStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#a6e3a1"))
	delStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#f38ba8"))
	hunkStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#89b4fa"))
	fileStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#cba6f7")).Bold(true)

	for _, line := range strings.Split(content, "\n") {
		switch {
		case strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---"):
			sb.WriteString(fileStyle.Render(line))
		case strings.HasPrefix(line, "@@"):
			sb.WriteString(hunkStyle.Render(line))
		case strings.HasPrefix(line, "+"):
			sb.WriteString(addStyle.Render(line))
		case strings.HasPrefix(line, "-"):
			sb.WriteString(delStyle.Render(line))
		default:
			sb.WriteString(line)
		}
		sb.WriteByte('\n')
	}
	return fmt.Sprintf("%s", sb.String())
}
