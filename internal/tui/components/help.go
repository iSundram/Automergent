package components

import (
	"fmt"
	"strings"

	"github.com/iSundram/Automergent/internal/tui/themes"
)

// HelpSection is one titled group of [key, description] rows.
type HelpSection struct {
	Title string
	Rows  [][2]string
}

// HelpOverlay shows keyboard shortcuts and slash commands.
type HelpOverlay struct {
	styles *themes.Styles
	width  int
	height int
	// slashCommands is injected by the app layer (derived from the command
	// registry) so this view never hardcodes a drifting copy of the list.
	slashCommands [][2]string
	// slashSections is the categorized sibling of slashCommands; when set it
	// replaces the flat Slash Commands section with grouped subsections.
	slashSections []HelpSection
}

// NewHelpOverlay creates a new HelpOverlay component.
func NewHelpOverlay(styles *themes.Styles) HelpOverlay {
	return HelpOverlay{styles: styles}
}

// SetSlashCommands supplies the slash-command rows rendered by View.
func (h *HelpOverlay) SetSlashCommands(items [][2]string) { h.slashCommands = items }

// SetSlashSections supplies categorized slash-command sections rendered by
// View. When set, these take precedence over the flat row list.
func (h *HelpOverlay) SetSlashSections(sections []HelpSection) { h.slashSections = sections }

// SetSize updates dimensions.
func (h *HelpOverlay) SetSize(w, v int) { h.width = w; h.height = v }

// View renders the help overlay.
func (h HelpOverlay) View() string {
	var sb strings.Builder

	title := h.styles.Bold.Render("  Automergent — Keyboard Shortcuts & Commands")
	sb.WriteString(title + "\n\n")

	sections := []struct {
		header string
		items  [][2]string
	}{
		{
			"Navigation",
			[][2]string{
				{"Enter", "Send message"},
				{"Alt+Up / Ctrl+P", "Previous history"},
				{"Alt+Down / Ctrl+N", "Next history"},
				{"↑ / ↓", "Scroll conversation"},
				{"PgUp / PgDown", "Page scroll"},
			},
		},
		{
			"Panels & Views",
			[][2]string{
				{"Ctrl+D", "Toggle diff pane"},
				{"Ctrl+L", "Toggle LSP panel"},
				{"Ctrl+R", "Toggle review mode (full tool output)"},
				{"Ctrl+S", "Open session browser"},
				{"Ctrl+T", "Toggle file tree"},
				{"?", "Show this help"},
			},
		},
		{
			"Session",
			[][2]string{
				{"Esc", "Interrupt active run"},
				{"Ctrl+U", "Clear input"},
				{"Ctrl+C", "Interrupt (double press to quit)"},
				{"Ctrl+Q", "Quit"},
			},
		},
	}

	keyW := 22
	for _, sec := range sections {
		sb.WriteString("\n" + h.styles.Success.Render("  "+sec.header) + "\n")
		for _, item := range sec.items {
			key := h.styles.Bold.Render(item[0])
			padding := keyW - len(item[0])
			if padding < 1 {
				padding = 1
			}
			sb.WriteString("    " + key + strings.Repeat(" ", padding) + h.styles.Dim.Render(item[1]) + "\n")
		}
	}

	// Slash commands: categorized subsections when the registry's
	// HelpSections are supplied, otherwise the legacy flat list.
	if len(h.slashSections) > 0 {
		sb.WriteString("\n" + h.styles.Success.Render("  Slash Commands") + "\n")
		for _, group := range h.slashSections {
			sb.WriteString("\n    " + h.styles.Bold.Render(group.Title) + "\n")
			for _, item := range group.Rows {
				key := h.styles.Bold.Render(item[0])
				padding := keyW - 4 - len(item[0])
				if padding < 1 {
					padding = 1
				}
				sb.WriteString("      " + key + strings.Repeat(" ", padding) + h.styles.Dim.Render(item[1]) + "\n")
			}
		}
	} else if len(h.slashCommands) > 0 {
		sb.WriteString("\n" + h.styles.Success.Render("  Slash Commands") + "\n")
		for _, item := range h.slashCommands {
			key := h.styles.Bold.Render(item[0])
			padding := keyW - len(item[0])
			if padding < 1 {
				padding = 1
			}
			sb.WriteString("    " + key + strings.Repeat(" ", padding) + h.styles.Dim.Render(item[1]) + "\n")
		}
	}

	sb.WriteString("\n" + h.styles.Dim.Render("  Press ? or Esc to close"))

	content := sb.String()
	w := h.width
	if w <= 0 {
		w = 76
	}

	// Clamp to the available height. The HelpBox adds 2 border rows plus 2
	// padding rows (Padding(1, 2)); an overlay taller than the terminal
	// makes the whole altscreen scroll every frame, which ghosts content
	// across frames (the panel appears stacked and progressively truncated).
	if h.height > 0 {
		const chrome = 4 // border (2) + vertical padding (2)
		maxLines := h.height - chrome
		if lines := strings.Split(content, "\n"); len(lines) > maxLines {
			if maxLines < 1 {
				maxLines = 1
			}
			hidden := len(lines) - maxLines
			lines = lines[:maxLines]
			note := fmt.Sprintf("  … %d more rows — widen the terminal to see everything", hidden)
			lines = append(lines, h.styles.Dim.Render(note))
			content = strings.Join(lines, "\n")
		}
	}

	return h.styles.HelpBox.Width(w).Render(content)
}
