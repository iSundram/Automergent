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
	maxItems := 10
	if p.height > 0 {
		// Dynamically adjust max items based on height if possible,
		// but 10 is a good safe default for the spotlight look.
		if p.height < 15 {
			maxItems = p.height - 5
		}
	}
	if maxItems < 1 {
		maxItems = 1
	}

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

// View renders the "Spotlight" command palette.
func (p CommandPalette) View() string {
	if !p.visible || p.width <= 0 || p.height <= 0 {
		return ""
	}

	// 1. Calculate palette dimensions
	palWidth := int(float64(p.width) * 0.8)
	if palWidth < 40 {
		palWidth = p.width
	}
	if palWidth > 80 {
		palWidth = 80
	}

	innerW := palWidth - 2 // Width inside the border

	// 2. Search Header
	searchIcon := lipgloss.NewStyle().Foreground(p.styles.T.Accent).Render("󰍉 ")
	queryText := p.query
	if queryText == "" {
		queryText = lipgloss.NewStyle().Foreground(p.styles.T.Muted).Render("Search commands...")
	} else {
		queryText = lipgloss.NewStyle().Foreground(p.styles.T.Text).Bold(true).Render(queryText)
	}

	headerStr := searchIcon + queryText
	// Ensure header fills innerW
	header := lipgloss.NewStyle().
		Padding(0, 1).
		Border(lipgloss.NormalBorder(), false, false, true, false).
		BorderForeground(p.styles.T.BorderNormal).
		Width(innerW).
		Render(headerStr)

	// 3. Items List with Scrolling
	var rows []string
	maxItems := 10
	if p.height < 15 {
		maxItems = p.height - 5
	}
	if maxItems < 1 {
		maxItems = 1
	}

	end := p.scrollOffset + maxItems
	if end > len(p.items) {
		end = len(p.items)
	}

	for i := p.scrollOffset; i < end; i++ {
		rows = append(rows, p.renderItem(p.items[i], i == p.cursor, innerW))
	}

	// Fill empty space if fewer items than maxItems
	for len(rows) < maxItems {
		rows = append(rows, lipgloss.NewStyle().Width(innerW).Render(""))
	}

	list := lipgloss.JoinVertical(lipgloss.Left, rows...)

	// 4. Footer
	footerText := fmt.Sprintf("%d/%d", p.cursor+1, len(p.items))
	footer := lipgloss.NewStyle().
		Foreground(p.styles.T.Background).
		Background(p.styles.T.Accent).
		Bold(true).
		Padding(0, 1).
		Render(footerText)

	// Place footer on the right
	footerRow := lipgloss.PlaceHorizontal(innerW, lipgloss.Right, footer)

	// 5. Final Assembly
	content := lipgloss.JoinVertical(lipgloss.Left, header, list, footerRow)

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(p.styles.T.Accent).
		Background(p.styles.T.Background).
		Padding(0).
		Width(palWidth).
		Render(content)
}

func (p CommandPalette) renderItem(item PaletteItem, selected bool, width int) string {
	// 1. Base style for the entire row
	rowStyle := lipgloss.NewStyle().Width(width).MaxWidth(width)
	if selected {
		rowStyle = rowStyle.Background(p.styles.T.Surface)
	}

	// 2. Indicator (▋ or two spaces)
	indicator := "  "
	if selected {
		indicator = lipgloss.NewStyle().Foreground(p.styles.T.Accent).Render("▋ ")
	}

	// 3. Label (Left part)
	labelStyle := lipgloss.NewStyle().Foreground(p.styles.T.Text)
	if selected {
		labelStyle = labelStyle.Foreground(p.styles.T.Accent).Bold(true)
	}
	labelPart := labelStyle.Render(item.Label)

	left := lipgloss.JoinHorizontal(lipgloss.Top, indicator, labelPart)
	leftW := lipgloss.Width(left)

	// 4. Description (Right part)
	descStyle := lipgloss.NewStyle().Foreground(p.styles.T.Muted).Italic(true)
	if selected {
		descStyle = descStyle.Foreground(p.styles.T.Muted)
	}

	// Calculate available space for description
	maxDescW := width - leftW - 2
	desc := item.Description
	if maxDescW < 5 {
		desc = ""
	} else if lipgloss.Width(desc) > maxDescW {
		desc = lipgloss.NewStyle().MaxWidth(maxDescW).Render(desc + "...")
	}
	descPart := descStyle.Render(desc)

	// 5. Join with calculated gap
	gapW := width - leftW - lipgloss.Width(descPart) - 1
	if gapW < 0 {
		gapW = 0
	}

	content := lipgloss.JoinHorizontal(lipgloss.Top,
		left,
		strings.Repeat(" ", gapW),
		descPart,
		" ",
	)

	return rowStyle.Render(content)
}
