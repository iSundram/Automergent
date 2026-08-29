package components

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/iSundram/Automergent/internal/tui/themes"
)

// CommandTier indicates visual importance in the palette.
type CommandTier int

const (
	TierPrimary   CommandTier = iota // accent-styled, prominent
	TierSecondary                    // normal styling (default)
	TierTertiary                     // dimmed
)

// PaletteItem represents an entry in the command palette.
type PaletteItem struct {
	Label          string
	Value          string
	Description    string
	Icon           string
	Category       string
	Hint           string
	// Kind is a short execution hint badge: "↵" prompt commands (start an
	// agent run), "⤢" full-page commands, "" plain handlers.
	Kind           string
	Tier           CommandTier
	Current        bool
	Disabled       bool
	DisabledReason string
	SearchTerms    string
}

// CommandPalette renders the inline command, model, provider and file browser.
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

func NewCommandPalette(styles *themes.Styles) CommandPalette { return CommandPalette{styles: styles} }

func (p *CommandPalette) SetSize(w, h int) { p.width, p.height = w, h }

func (p *CommandPalette) Show(items []PaletteItem, query string) {
	p.items, p.query = items, query
	p.cursor, p.scrollOffset, p.visible = 0, 0, true
}

func (p *CommandPalette) SetItems(items []PaletteItem) {
	p.items = items
	if p.cursor >= len(p.items) && len(p.items) > 0 {
		p.cursor = len(p.items) - 1
	}
	p.updateScroll()
}

func (p *CommandPalette) SetQuery(query string) { p.query = query }

func (p *CommandPalette) updateScroll() {
	maxItems := p.MaxVisibleItems()
	if p.cursor < p.scrollOffset {
		p.scrollOffset = p.cursor
	}
	if p.cursor >= p.scrollOffset+maxItems {
		p.scrollOffset = p.cursor - maxItems + 1
	}
}

func (p *CommandPalette) SetWidth(w int)      { p.width = w }
func (p CommandPalette) Items() []PaletteItem { return p.items }
func (p *CommandPalette) Hide()               { p.visible = false }
func (p CommandPalette) Visible() bool        { return p.visible }

func (p CommandPalette) Selected() *PaletteItem {
	if len(p.items) == 0 {
		return nil
	}
	item := p.items[p.cursor]
	return &item
}

func (p CommandPalette) previousCategory() int {
	current := p.items[p.cursor].Category
	for i := p.cursor - 1; i >= 0; i-- {
		if p.items[i].Category != current {
			category := p.items[i].Category
			for i > 0 && p.items[i-1].Category == category {
				i--
			}
			return i
		}
	}
	return p.cursor
}

func (p CommandPalette) nextCategory() int {
	current := p.items[p.cursor].Category
	for i := p.cursor + 1; i < len(p.items); i++ {
		if p.items[i].Category != current {
			return i
		}
	}
	return p.cursor
}

func (p CommandPalette) Update(msg tea.Msg) (CommandPalette, tea.Cmd) {
	if !p.visible || len(p.items) == 0 {
		return p, nil
	}
	if m, ok := msg.(tea.KeyMsg); ok {
		switch m.String() {
		case "up", "ctrl+p":
			if p.cursor > 0 {
				p.cursor--
			} else {
				p.cursor = len(p.items) - 1
			}
		case "down", "ctrl+n", "tab":
			if p.cursor < len(p.items)-1 {
				p.cursor++
			} else {
				p.cursor = 0
			}
		case "pgup":
			p.cursor = max(0, p.cursor-p.MaxVisibleItems())
		case "pgdown":
			p.cursor = min(len(p.items)-1, p.cursor+p.MaxVisibleItems())
		case "shift+tab":
			p.cursor = p.previousCategory()
		case "ctrl+tab":
			p.cursor = p.nextCategory()
		}
		p.updateScroll()
	}
	if m, ok := msg.(tea.MouseMsg); ok {
		switch m.Mouse().Button {
		case tea.MouseWheelUp:
			p.cursor = max(0, p.cursor-1)
		case tea.MouseWheelDown:
			p.cursor = min(len(p.items)-1, p.cursor+1)
		}
		p.updateScroll()
	}
	return p, nil
}

// categoryMeta maps palette category names to their display title and icon symbol.
var categoryMeta = map[string]struct{ title, symbol string }{
	"Commands":          {"Commands", "/"},
	"Models":            {"Models", "󰊕"},
	"Providers":         {"Providers", "󰒋"},
	"Files":             {"Files", "@"},
	"Modes":             {"Modes", "󰒓"},
	"Themes":            {"Themes", "󰏘"},
	"Keybindings":       {"Keybindings", "󰌌"},
	"Effort":            {"Effort", "󰓅"},
	"Project Commands":  {"Run", "󰆍"},
	"Commit Scope":      {"Commit", "󰊢"},
	"Review Target":     {"Review", "󰤒"},
	"mcp":               {"MCP", "󰌠"},
	"MCP Servers":       {"MCP Servers", "󰌠"},
	"commands":          {"Commands", "󰘳"},
	"directory":         {"Directory", "󰉖"},
	"plan":              {"Plan", "󰈙"},
	"goal":              {"Goal", "󰘧"},
	"Knowledge":         {"Knowledge", "󰚩"},
}

func (p CommandPalette) View() string {
	if !p.visible || p.width <= 0 || p.height <= 0 {
		return ""
	}
	w := max(12, p.width)
	rule := lipgloss.NewStyle().Foreground(p.styles.T.BorderNormal).Render(strings.Repeat("─", w))
	title, symbol := "Commands", "/"
	if len(p.items) > 0 {
		cat := p.items[0].Category
		if meta, ok := categoryMeta[cat]; ok {
			title, symbol = meta.title, meta.symbol
		} else if cat != "" {
			// Use the category name directly with a generic icon.
			title = cat
		}
	}
	count := fmt.Sprintf("%d of %d", min(p.cursor+1, len(p.items)), len(p.items))
	headerLeft := lipgloss.NewStyle().Foreground(p.styles.T.Accent).Bold(true).Render("  "+symbol+"  ") +
		lipgloss.NewStyle().Foreground(p.styles.T.Text).Bold(true).Render(strings.ToUpper(title))
	header := joinEnds(headerLeft, lipgloss.NewStyle().Foreground(p.styles.T.Muted).Render(count)+"  ", w)

	maxItems := p.MaxVisibleItems()
	end := min(p.scrollOffset+maxItems, len(p.items))
	rows := make([]string, 0, maxItems)
	for i := p.scrollOffset; i < end; i++ {
		if p.items[i].Category != "" && (i == p.scrollOffset || p.items[i-1].Category != p.items[i].Category) {
			rows = append(rows, p.renderCategory(p.items[i].Category, w-2))
		}
		rows = append(rows, p.renderItem(p.items[i], i == p.cursor, w-2))
	}
	if len(rows) == 0 {
		rows = append(rows, lipgloss.NewStyle().Foreground(p.styles.T.Muted).Italic(true).PaddingLeft(4).Width(w-2).Render("No matching results"))
	}
	rows = p.addScrollbar(rows, maxItems)

	footerText := "↑↓ navigate · enter select · esc close"
	if w >= 70 {
		footerText = "↑↓ navigate · pgup/pgdn page · ctrl+tab section · enter select · esc close"
	}
	footer := lipgloss.NewStyle().Foreground(p.styles.T.Muted).PaddingLeft(2).
		MaxWidth(max(1, w-2)).Render(footerText)
	return lipgloss.JoinVertical(lipgloss.Left, rule, header, "", lipgloss.JoinVertical(lipgloss.Left, rows...), "", rule, footer)
}

func (p CommandPalette) addScrollbar(rows []string, viewport int) []string {
	if len(p.items) <= viewport {
		return rows
	}
	trackHeight := len(rows)
	thumbSize := max(1, trackHeight*viewport/len(p.items))
	thumbTop := 0
	if len(p.items) > viewport {
		thumbTop = p.scrollOffset * (trackHeight - thumbSize) / (len(p.items) - viewport)
	}
	for i, row := range rows {
		bar := "│"
		if i >= thumbTop && i < thumbTop+thumbSize {
			bar = "┃"
		}
		// rows[i] is already rendered at (p.width-2) by renderItem/renderCategory;
		// appending the scrollbar glyph + trailing space brings total to p.width.
		rows[i] = row +
			lipgloss.NewStyle().Foreground(p.styles.T.BorderNormal).Render(bar) + " "
	}
	return rows
}

func (p CommandPalette) MaxVisibleItems() int {
	const overhead = 9 // rule + header + blank + rows + blank + rule + footer
	if p.height > 0 {
		return max(1, p.height-overhead)
	}
	return 8 // default when height is not yet known
}

func (p CommandPalette) Height() int {
	if !p.visible {
		return 0
	}
	visible := min(len(p.items), p.MaxVisibleItems())
	if visible < 1 {
		visible = 1
	}
	headings := 0
	end := min(p.scrollOffset+visible, len(p.items))
	for i := p.scrollOffset; i < end; i++ {
		if p.items[i].Category != "" && (i == p.scrollOffset || p.items[i-1].Category != p.items[i].Category) {
			headings++
		}
	}
	return visible + headings + 6 // rules, header, two breathing rows and footer
}

func (p CommandPalette) renderCategory(category string, width int) string {
	label := lipgloss.NewStyle().Foreground(p.styles.T.Muted).Bold(true).Render(strings.ToUpper(category))
	return lipgloss.NewStyle().Width(width).MaxWidth(width).PaddingLeft(4).Render(label)
}

func (p CommandPalette) renderItem(item PaletteItem, selected bool, width int) string {
	indicator := "  "
	if selected {
		indicator = lipgloss.NewStyle().Foreground(p.styles.T.Accent).Render("▍ ")
	}
	icon := item.Icon
	if icon == "" {
		icon = "·"
	}
	textColor := p.styles.T.Text
	if selected {
		textColor = p.styles.T.Subtext
	}
	if item.Disabled {
		textColor = p.styles.T.Muted
	}
	iconStyle := lipgloss.NewStyle().Foreground(p.styles.T.Muted)
	labelStyle := lipgloss.NewStyle().Foreground(textColor)
	if selected {
		iconStyle = iconStyle.Foreground(p.styles.T.Accent)
		labelStyle = labelStyle.Bold(true)
	}
	// Tier-based styling: primary commands get accent icon and bold label.
	if !selected && item.Tier == TierPrimary {
		iconStyle = lipgloss.NewStyle().Foreground(p.styles.T.Accent)
		labelStyle = labelStyle.Bold(true)
	}
	if !selected && item.Tier == TierTertiary {
		iconStyle = lipgloss.NewStyle().Foreground(p.styles.T.Muted).Faint(true)
		labelStyle = lipgloss.NewStyle().Foreground(p.styles.T.Muted).Faint(true)
	}
	prefix := "  " + indicator + iconStyle.Render(icon) + "  "
	label := p.renderMatch(item.Label, labelStyle)
	if item.Kind != "" {
		label += lipgloss.NewStyle().Foreground(p.styles.T.Muted).Render(" " + item.Kind)
	}
	if item.Hint != "" {
		label += lipgloss.NewStyle().Foreground(p.styles.T.Muted).Render(" " + item.Hint)
	}
	left := prefix + label
	if item.Current {
		left += lipgloss.NewStyle().Foreground(p.styles.T.Green).Render("  󰄬 Current")
	}
	if item.Disabled && item.DisabledReason != "" {
		item.Description = item.DisabledReason
	}

	available := width - lipgloss.Width(left) - 3
	desc := ""
	if available >= 18 && item.Description != "" {
		descText := truncateCells(item.Description, available)
		desc = strings.Repeat(" ", max(2, available-lipgloss.Width(descText))) +
			lipgloss.NewStyle().Foreground(p.styles.T.Muted).Render(descText) + " "
	}
	return lipgloss.NewStyle().Width(width).MaxWidth(width).Render(left + desc)
}

func (p CommandPalette) renderMatch(label string, base lipgloss.Style) string {
	query := strings.TrimSpace(strings.TrimLeft(p.query, "/@?"))
	if query == "" {
		return base.Render(label)
	}
	start := strings.Index(strings.ToLower(label), strings.ToLower(query))
	if start < 0 {
		return base.Render(label)
	}
	// Use rune-based slicing so multi-byte characters (CJK, emoji) never
	// produce a mid-rune boundary panic or garbled output.
	labelRunes := []rune(label)
	queryRunes := []rune(query)
	runeStart := len([]rune(label[:start]))
	runeEnd := runeStart + len(queryRunes)
	if runeEnd > len(labelRunes) {
		runeEnd = len(labelRunes)
	}
	return base.Render(string(labelRunes[:runeStart])) +
		base.Foreground(p.styles.T.Accent).Underline(true).Render(string(labelRunes[runeStart:runeEnd])) +
		base.Render(string(labelRunes[runeEnd:]))
}

func joinEnds(left, right string, width int) string {
	gap := max(1, width-lipgloss.Width(left)-lipgloss.Width(right))
	return lipgloss.NewStyle().Width(width).MaxWidth(width).Render(left + strings.Repeat(" ", gap) + right)
}

func truncateCells(s string, width int) string {
	if lipgloss.Width(s) <= width {
		return s
	}
	if width <= 1 {
		return ""
	}
	r := []rune(s)
	for len(r) > 0 && lipgloss.Width(string(r))+1 > width {
		r = r[:len(r)-1]
	}
	return string(r) + "…"
}
