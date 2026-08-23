package components

import (
	"strings"

	"github.com/iSundram/Automergent/internal/tui/themes"
)

// HelpOverlay shows keyboard shortcuts and slash commands.
type HelpOverlay struct {
	styles *themes.Styles
	width  int
	height int
	// slashCommands is injected by the app layer (derived from the command
	// registry) so this view never hardcodes a drifting copy of the list.
	slashCommands [][2]string
}

// NewHelpOverlay creates a new HelpOverlay component.
func NewHelpOverlay(styles *themes.Styles) HelpOverlay {
	return HelpOverlay{styles: styles}
}

// SetSlashCommands supplies the slash-command rows rendered by View.
func (h *HelpOverlay) SetSlashCommands(items [][2]string) { h.slashCommands = items }

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
		{
			"Slash Commands",
			h.slashCommands,
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

	sb.WriteString("\n" + h.styles.Dim.Render("  Press ? or Esc to close"))

	content := sb.String()
	w := h.width
	if w <= 0 {
		w = 76
	}
	return h.styles.HelpBox.Width(w).Render(content)
}
