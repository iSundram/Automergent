package render

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

// Diff renders a unified diff with color highlighting and word-level changes.
func Diff(content string) string {
	var sb strings.Builder
	// Catppuccin colors
	green := lipgloss.Color("#a6e3a1")
	red := lipgloss.Color("#f38ba8")
	blue := lipgloss.Color("#89b4fa")
	magenta := lipgloss.Color("#cba6f7")
	yellow := lipgloss.Color("#f9e2af")
	surface := lipgloss.Color("#313244")
	surfaceDark := lipgloss.Color("#1e1e2e")

	addStyle := lipgloss.NewStyle().Foreground(green).Background(surface)
	delStyle := lipgloss.NewStyle().Foreground(red).Background(surface)
	hunkStyle := lipgloss.NewStyle().Foreground(blue).Bold(true)
	fileStyle := lipgloss.NewStyle().Foreground(magenta).Bold(true)

	// Word-level diff markers
	wordDelStyle := lipgloss.NewStyle().Foreground(red).Background(surfaceDark).Strikethrough(true)
	wordAddStyle := lipgloss.NewStyle().Foreground(green).Background(surfaceDark).Underline(true).Bold(true)
	lineNumStyle := lipgloss.NewStyle().Foreground(yellow).Faint(true).Width(4).Align(lipgloss.Right)
	contextStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#6c7086")) // Overlay0

	lineNum := 0

	for _, line := range strings.Split(content, "\n") {
		if line == "" {
			sb.WriteByte('\n')
			continue
		}

		// Render word-level markers if present
		renderedLine := line
		if strings.Contains(line, "«") || strings.Contains(line, "‹") {
			renderedLine = renderWordDiff(line, wordDelStyle, wordAddStyle)
		}

		switch {
		case strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---"):
			// File headers - no line numbers
			sb.WriteString("    " + fileStyle.Render(line) + "\n")
		case strings.HasPrefix(line, "@@"):
			// Hunk headers
			sb.WriteString("\n    " + hunkStyle.Render(line) + "\n")
			// Reset line number from hunk header if possible
			lineNum = 0
		case strings.HasPrefix(line, "+"):
			lineNum++
			num := lineNumStyle.Render(fmt.Sprintf("%d", lineNum))
			sb.WriteString(num + " " + addStyle.Render(renderedLine) + "\n")
		case strings.HasPrefix(line, "-"):
			// Deleted lines don't increment line number
			num := lineNumStyle.Render("  ")
			sb.WriteString(num + " " + delStyle.Render(renderedLine) + "\n")
		default:
			// Context lines
			lineNum++
			num := lineNumStyle.Render(fmt.Sprintf("%d", lineNum))
			sb.WriteString(num + " " + contextStyle.Render(renderedLine) + "\n")
		}
	}
	return strings.TrimSuffix(sb.String(), "\n")
}

// renderWordDiff processes word-level diff markers and applies styles
func renderWordDiff(line string, delStyle, addStyle lipgloss.Style) string {
	var result strings.Builder
	i := 0
	runes := []rune(line)

	for i < len(runes) {
		switch runes[i] {
		case '«': // Start of deleted word
			j := i + 1
			for j < len(runes) && runes[j] != '»' {
				j++
			}
			if j < len(runes) {
				deleted := string(runes[i+1 : j])
				result.WriteString(delStyle.Render(deleted))
				i = j + 1
				continue
			}
		case '‹': // Start of inserted word
			j := i + 1
			for j < len(runes) && runes[j] != '›' {
				j++
			}
			if j < len(runes) {
				inserted := string(runes[i+1 : j])
				result.WriteString(addStyle.Render(inserted))
				i = j + 1
				continue
			}
		}
		result.WriteRune(runes[i])
		i++
	}
	return result.String()
}
