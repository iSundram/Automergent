package render

import (
	"fmt"
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
)

// Diff renders a unified diff with theme-derived colors, line numbers and
// word-level «del»/‹ins› highlighting.
func Diff(content string) string {
	return DiffWithWidth(content, 0)
}

// DiffWithWidth renders like Diff but pads add/delete rows with their
// background tint out to the given width so full rows highlight edge-to-edge.
func DiffWithWidth(content string, padTo int) string {
	p := Palette()

	addStyle := lipgloss.NewStyle().Foreground(p.AddFg).Background(p.AddBg)
	delStyle := lipgloss.NewStyle().Foreground(p.DelFg).Background(p.DelBg)
	hunkStyle := lipgloss.NewStyle().Foreground(p.Hunk).Bold(true)
	fileStyle := lipgloss.NewStyle().Foreground(p.File).Bold(true)
	wordDelStyle := lipgloss.NewStyle().Foreground(p.DelFg).Background(p.WordDelBg).Strikethrough(true)
	wordAddStyle := lipgloss.NewStyle().Foreground(p.AddFg).Background(p.WordAddBg).Underline(true).Bold(true)
	lineNumStyle := lipgloss.NewStyle().Foreground(p.LineNum).Width(4).Align(lipgloss.Right)
	contextStyle := lipgloss.NewStyle().Foreground(p.Context)

	var sb strings.Builder
	lineNum := 0

	// writeRow emits "NNNN body", optionally padding body's background to
	// padTo columns.
	writeRow := func(num string, body string, style lipgloss.Style, bg color.Color) {
		sb.WriteString(num + " ")
		sb.WriteString(style.Render(body))
		if padTo > 0 && bg != nil {
			if w := lipgloss.Width(body); w < padTo-5 {
				pad := lipgloss.NewStyle().Background(lipgloss.Color(colorHex(bg)))
				sb.WriteString(pad.Render(strings.Repeat(" ", padTo-5-w)))
			}
		}
		sb.WriteByte('\n')
	}

	for _, line := range strings.Split(content, "\n") {
		if line == "" {
			sb.WriteByte('\n')
			continue
		}

		renderedLine := line
		if strings.ContainsAny(line, "«»‹›") {
			renderedLine = WordMarkers(line, wordDelStyle, wordAddStyle)
		}

		switch {
		case strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---"):
			sb.WriteString("     " + fileStyle.Render(line) + "\n")
		case strings.HasPrefix(line, "@@"):
			sb.WriteString("\n     " + hunkStyle.Render(line) + "\n")
			lineNum = 0
		case strings.HasPrefix(line, "+"):
			lineNum++
			writeRow(lineNumStyle.Render(fmt.Sprintf("%d", lineNum)), renderedLine[1:], addStyle, p.AddBg)
		case strings.HasPrefix(line, "-"):
			writeRow(lineNumStyle.Render("·"), renderedLine[1:], delStyle, p.DelBg)
		default:
			lineNum++
			writeRow(lineNumStyle.Render(fmt.Sprintf("%d", lineNum)), renderedLine[1:], contextStyle, nil)
		}
	}
	return strings.TrimSuffix(sb.String(), "\n")
}
