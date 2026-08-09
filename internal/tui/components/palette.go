package components

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/iSundram/Automergent/internal/tui/themes"
)

// PaletteItem is a single entry in the command palette.
type PaletteItem struct {
	Name        string
	Description string
	Value       string // The value to autocomplete
	Icon        string // Optional icon
}

// CommandPalette is a floating dropdown for commands and files.
type CommandPalette struct {
	styles  *themes.Styles
	items   []PaletteItem
	cursor  int
	visible bool
	width   int
}

// NewCommandPalette creates a new Palette component.
func NewCommandPalette(styles *themes.Styles) CommandPalette {
	return CommandPalette{styles: styles}
}

// SetItems updates the list of items.
func (p *CommandPalette) SetItems(items []PaletteItem) {
	p.items = items
	if p.cursor >= len(items) {
		p.cursor = 0
	}
}

// Show makes the palette visible.
func (p *CommandPalette) Show() { p.visible = true }

// Hide hides the palette.
func (p *CommandPalette) Hide() { p.visible = false; p.cursor = 0 }

// Visible reports whether the palette is shown.
func (p CommandPalette) Visible() bool { return p.visible }

// Selected returns the currently highlighted item.
func (p CommandPalette) Selected() *PaletteItem {
	if len(p.items) == 0 || p.cursor < 0 || p.cursor >= len(p.items) {
		return nil
	}
	return &p.items[p.cursor]
}

// SetWidth updates the component width.
func (p *CommandPalette) SetWidth(w int) { p.width = w }

// Update handles keyboard navigation.
func (p CommandPalette) Update(msg tea.Msg) (CommandPalette, tea.Cmd) {
	if !p.visible || len(p.items) == 0 {
		return p, nil
	}

	switch m := msg.(type) {
	case tea.KeyMsg:
		switch m.String() {
		case "up", "ctrl+p":
			if p.cursor > 0 {
				p.cursor--
			} else {
				p.cursor = len(p.items) - 1
			}
		case "down", "tab", "ctrl+n":
			if p.cursor < len(p.items)-1 {
				p.cursor++
			} else {
				p.cursor = 0
			}
		case "esc":
			p.Hide()
		}
	}
	return p, nil
}

// View renders the palette.
func (p CommandPalette) View() string {
	if !p.visible || len(p.items) == 0 {
		return ""
	}

	const maxItems = 7
	itemsToRender := p.items
	truncated := false
	if len(itemsToRender) > maxItems {
		itemsToRender = itemsToRender[:maxItems]
		truncated = true
	}

	var lines []string
	contentW := p.width - 4
	if contentW < 20 {
		contentW = 20
	}

	for i, item := range itemsToRender {
		// Clean alignment: Icon + Name (20 chars) + Description
		icon := item.Icon
		if icon == "" {
			icon = "  "
		}
		name := item.Name
		if len(name) > 18 {
			name = name[:15] + "..."
		}

		line := fmt.Sprintf("%s %-20s %s", icon, name, item.Description)
		styledLine := p.styles.PaletteItem.Width(contentW).Render(line)
		if i == p.cursor {
			styledLine = p.styles.PaletteSelect.Width(contentW).Render(line)
		}
		lines = append(lines, styledLine)
	}

	if truncated {
		more := fmt.Sprintf(" ... (+%d more)", len(p.items)-maxItems)
		lines = append(lines, p.styles.PaletteDim.Width(contentW).Render(more))
	}

	// Join with actual newlines and wrap in border ONCE
	content := strings.Join(lines, "\n")
	return p.styles.Palette.Width(p.width).Render(content)
}
