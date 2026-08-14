package components

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/iSundram/Automergent/internal/tui/themes"
)

// PaletteItem represents an entry in the command palette.
type PaletteItem struct {
	Label       string
	Value       string
	Description string
	Icon        string
	Category    string
}

// CommandPalette renders the "/" and "?" overlay.
type CommandPalette struct {
	styles       *themes.Styles
	width        int
	height       int
	cursor       int
	scrollOffset int
	items        []PaletteItem
	visible      bool
	query        string
}

// NewCommandPalette creates a new CommandPalette.
func NewCommandPalette(styles *themes.Styles) CommandPalette {
	return CommandPalette{styles: styles}
}

// SetSize updates the palette dimensions.
func (p *CommandPalette) SetSize(w, h int) {
	p.width = w
	p.height = h
}

// Show makes the palette visible with a set of items.
func (p *CommandPalette) Show(items []PaletteItem, query string) {
	p.items = items
	p.query = query
	p.cursor = 0
	p.scrollOffset = 0
	p.visible = true
}

// SetItems updates the items in the palette.
func (p *CommandPalette) SetItems(items []PaletteItem) {
	p.items = items
	if p.cursor >= len(p.items) && len(p.items) > 0 {
		p.cursor = len(p.items) - 1
	}
	p.updateScroll()
}

func (p *CommandPalette) updateScroll() {
	maxItems := p.MaxVisibleItems()

	if p.cursor < p.scrollOffset {
		p.scrollOffset = p.cursor
	} else if p.cursor >= p.scrollOffset+maxItems {
		p.scrollOffset = p.cursor - maxItems + 1
	}
}

// SetWidth updates the palette width.
func (p *CommandPalette) SetWidth(w int) {
	p.width = w
}

// Items returns the current items in the palette.
func (p CommandPalette) Items() []PaletteItem { return p.items }

// Hide hides the palette.
func (p *CommandPalette) Hide() {
	p.visible = false
}

// Visible returns whether the palette is visible.
func (p CommandPalette) Visible() bool { return p.visible }

// Selected returns the currently highlighted item.
func (p CommandPalette) Selected() *PaletteItem {
	if len(p.items) == 0 {
		return nil
	}
	return &p.items[p.cursor]
}

// Update handles key events.
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
			p.updateScroll()
		case "down", "ctrl+n", "tab":
			if p.cursor < len(p.items)-1 {
				p.cursor++
			} else {
				p.cursor = 0
			}
			p.updateScroll()
		}
	}

	return p, nil
}

// View renders the palette as a full-width inline list (shown below the input).
func (p CommandPalette) View() string {
	if !p.visible || p.width <= 0 || p.height <= 0 {
		return ""
	}

	innerW := p.width

	// Items list with scrolling
	var rows []string
	maxItems := p.MaxVisibleItems()

	end := p.scrollOffset + maxItems
	if end > len(p.items) {
		end = len(p.items)
	}

	for i := p.scrollOffset; i < end; i++ {
		rows = append(rows, p.renderItem(p.items[i], i == p.cursor, innerW))
	}

	if len(rows) == 0 {
		rows = append(rows, lipgloss.NewStyle().
			Foreground(p.styles.T.Muted).
			Italic(true).
			PaddingLeft(3).
			Render("No matches"))
	}

	list := lipgloss.JoinVertical(lipgloss.Left, rows...)

	// Footer hint line: position + navigation help
	pos := fmt.Sprintf("%d/%d", p.cursor+1, len(p.items))
	hint := lipgloss.NewStyle().
		Foreground(p.styles.T.Muted).
		PaddingLeft(3).
		Render(fmt.Sprintf("%s  ↑↓ navigate · ⏎ select · esc dismiss", pos))

	return lipgloss.JoinVertical(lipgloss.Left, list, hint)
}

// MaxVisibleItems returns how many rows the palette shows at once.
func (p CommandPalette) MaxVisibleItems() int {
	maxItems := 8
	if p.height > 0 && p.height < 15 {
		maxItems = p.height - 6
	}
	if maxItems < 1 {
		maxItems = 1
	}
	return maxItems
}

// Height returns the total rendered height of the palette (list + hint line).
func (p CommandPalette) Height() int {
	if !p.visible {
		return 0
	}
	visible := len(p.items) - p.scrollOffset
	if max := p.MaxVisibleItems(); visible > max {
		visible = max
	}
	if visible < 1 {
		visible = 1 // "No matches" row
	}
	return visible + 1 // + hint line
}

func (p CommandPalette) renderItem(item PaletteItem, selected bool, width int) string {
	// 1. Base style for the entire row
	rowStyle := lipgloss.NewStyle().Width(width).MaxWidth(width)
	if selected {
		rowStyle = rowStyle.Background(p.styles.T.Surface)
	}

	// 2. Indicator (▍ or space), aligned with the input prompt
	indicator := "   "
	if selected {
		indicator = " " + lipgloss.NewStyle().Foreground(p.styles.T.Accent).Render("▍") + " "
	}

	// 3. Label (Left part)
	labelStyle := lipgloss.NewStyle().Foreground(p.styles.T.Text)
	if selected {
		labelStyle = labelStyle.Foreground(p.styles.T.Accent).Bold(true)
	}
	labelPart := labelStyle.Render(item.Label)

	left := lipgloss.JoinHorizontal(lipgloss.Top, indicator, labelPart)
	leftW := lipgloss.Width(left)

	// 4. Description follows the label on the same line
	descStyle := lipgloss.NewStyle().Foreground(p.styles.T.Muted)

	// Calculate available space for description
	maxDescW := width - leftW - 4
	desc := item.Description
	if maxDescW < 5 {
		desc = ""
	} else if lipgloss.Width(desc) > maxDescW {
		desc = lipgloss.NewStyle().MaxWidth(maxDescW).Render(desc + "...")
	}
	descPart := descStyle.Render(desc)

	// Pad label column so descriptions align
	labelColW := 22
	gapW := labelColW - leftW
	if gapW < 2 {
		gapW = 2
	}

	content := lipgloss.JoinHorizontal(lipgloss.Top,
		left,
		strings.Repeat(" ", gapW),
		descPart,
	)

	return rowStyle.Render(content)
}
