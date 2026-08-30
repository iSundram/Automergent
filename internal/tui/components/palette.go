package components

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/iSundram/Automergent/internal/tui/themes"
	"github.com/sahilm/fuzzy"
)

// CommandTier indicates visual importance in the palette.
type CommandTier int

const (
	TierPrimary   CommandTier = iota // accent-styled, prominent
	TierSecondary                    // normal styling (default)
	TierTertiary                     // dimmed
)

// maxPaletteRows caps how many rows the palette shows regardless of terminal
// height — the picker stays compact instead of swallowing the
// conversation on tall terminals.
const maxPaletteRows = 15

// paletteOverhead is the fixed chrome around the item rows: one blank line
// and the footer hint line. MaxVisibleItems and Height must agree on this
// constant or the layout double-counts space.
const paletteOverhead = 2

// PaletteItem represents an entry in the command palette.
type PaletteItem struct {
	Label       string
	Value       string
	Description string
	Icon        string
	Category    string
	Hint        string
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
//
// Layout is the minimal single-line style: no icons, no header, no box —
// just `❯` marking the selection, the label and dim args hint in a left
// column, and the dim description after it. A single footer line carries the
// position count and key hints. Category is still carried on items so
// shift+tab/ctrl+tab section jumps keep working.
type CommandPalette struct {
	styles       *themes.Styles
	width        int
	height       int
	cursor       int
	scrollOffset int
	items        []PaletteItem
	visible      bool
	query        string
	// emptyHint is shown instead of the generic message when no items match.
	emptyHint string
}

func NewCommandPalette(styles *themes.Styles) CommandPalette { return CommandPalette{styles: styles} }

func (p *CommandPalette) SetSize(w, h int) { p.width, p.height = w, h }

// SetEmptyHint overrides the generic "no matching results" line for the
// current trigger (e.g. "no matching command — enter sends it as a message").
func (p *CommandPalette) SetEmptyHint(hint string) { p.emptyHint = hint }

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

func (p CommandPalette) View() string {
	if !p.visible || p.width <= 0 || p.height <= 0 {
		return ""
	}
	w := max(12, p.width)

	maxItems := p.MaxVisibleItems()
	end := min(p.scrollOffset+maxItems, len(p.items))
	visible := p.items[p.scrollOffset:end]

	// The description column starts after the widest left zone among the
	// visible rows, capped so long args-hints can't starve the description.
	leftW := 0
	for _, it := range visible {
		if lw := itemLeftWidth(it); lw > leftW {
			leftW = lw
		}
	}
	leftW = min(leftW, max(16, w*3/5))

	rows := make([]string, 0, maxItems)
	for i, item := range visible {
		rows = append(rows, p.renderItem(item, p.scrollOffset+i == p.cursor, w-2, leftW))
	}
	if len(rows) == 0 {
		hint := p.emptyHint
		if hint == "" {
			hint = "No matching results"
		}
		rows = append(rows, lipgloss.NewStyle().Foreground(p.styles.T.Muted).Italic(true).PaddingLeft(4).Width(w-2).Render(hint))
	}
	rows = p.addScrollbar(rows, maxItems)

	count := fmt.Sprintf("%d/%d", min(p.cursor+1, max(1, len(p.items))), len(p.items))
	footerText := count + "  ↑↓ select · ⇥ section · ⏎ run · esc close"
	footer := lipgloss.NewStyle().Foreground(p.styles.T.Muted).PaddingLeft(2).
		MaxWidth(max(1, w-2)).Render(footerText)
	return lipgloss.JoinVertical(lipgloss.Left, lipgloss.JoinVertical(lipgloss.Left, rows...), "", footer)
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
		// rows[i] is already rendered at (p.width-2) by renderItem;
		// appending the scrollbar glyph + trailing space brings total to p.width.
		rows[i] = row +
			lipgloss.NewStyle().Foreground(p.styles.T.BorderNormal).Render(bar) + " "
	}
	return rows
}

// MaxVisibleItems is the item-row capacity: terminal height minus the palette
// chrome, capped at maxPaletteRows.
func (p CommandPalette) MaxVisibleItems() int {
	if p.height > 0 {
		return max(1, min(maxPaletteRows, p.height-paletteOverhead))
	}
	return 8 // default when height is not yet known
}

// Height reports the rendered height: rows plus the same paletteOverhead
// constant MaxVisibleItems subtracts, so the two can never disagree.
func (p CommandPalette) Height() int {
	if !p.visible {
		return 0
	}
	visible := min(len(p.items), p.MaxVisibleItems())
	if visible < 1 {
		visible = 1
	}
	return visible + paletteOverhead
}

// itemLeftWidth is the rendered width of an item's left zone: label, args
// hint and current marker. Icons and kind badges are not rendered.
func itemLeftWidth(item PaletteItem) int {
	w := lipgloss.Width(item.Label)
	if item.Hint != "" {
		w += 1 + lipgloss.Width(item.Hint)
	}
	if item.Current {
		w += 2 + lipgloss.Width("(current)")
	}
	return w
}

func (p CommandPalette) renderItem(item PaletteItem, selected bool, width int, leftW int) string {
	// Selection marker: `❯` in the accent colour, matching the input prompt.
	marker := "  "
	if selected {
		marker = lipgloss.NewStyle().Foreground(p.styles.T.Accent).Bold(true).Render("❯ ")
	}
	textColor := p.styles.T.Text
	if item.Disabled {
		textColor = p.styles.T.Muted
	}
	labelStyle := lipgloss.NewStyle().Foreground(textColor)
	if selected || (!item.Disabled && item.Tier == TierPrimary) {
		labelStyle = labelStyle.Bold(true)
	}
	if !selected && item.Tier == TierTertiary {
		labelStyle = lipgloss.NewStyle().Foreground(p.styles.T.Muted).Faint(true)
	}

	label := p.renderMatch(item.Label, labelStyle)
	if item.Hint != "" {
		label += lipgloss.NewStyle().Foreground(p.styles.T.Muted).Render(" " + item.Hint)
	}
	left := marker + label
	if item.Current {
		left += lipgloss.NewStyle().Foreground(p.styles.T.Green).Render("  (current)")
	}

	description := item.Description
	if item.Disabled && item.DisabledReason != "" {
		description = item.DisabledReason
	}

	// Two-zone row: left zone padded to leftW, description in the remainder.
	avail := width - leftW - 4
	desc := ""
	if avail >= 10 && description != "" {
		descText := truncateCells(description, avail)
		gap := leftW - itemLeftWidth(item) + max(2, avail-lipgloss.Width(descText))
		desc = strings.Repeat(" ", max(2, gap)) +
			lipgloss.NewStyle().Foreground(p.styles.T.Muted).Render(descText) + " "
	} else {
		// No room for a description column: just pad the row out.
		desc = strings.Repeat(" ", max(0, width-lipgloss.Width(left)-2))
	}
	return lipgloss.NewStyle().Width(width).MaxWidth(width).Render(left + desc)
}

// renderMatch highlights the query inside the label: a contiguous substring
// directly, otherwise the runes picked by the fuzzy matcher (so non-contiguous
// fuzzy results explain themselves).
func (p CommandPalette) renderMatch(label string, base lipgloss.Style) string {
	query := strings.TrimSpace(strings.TrimLeft(p.query, "/@?"))
	if query == "" {
		return base.Render(label)
	}
	matched := matchIndexes(query, label)
	if len(matched) == 0 {
		return base.Render(label)
	}
	// Use rune-based slicing so multi-byte characters (CJK, emoji) never
	// produce a mid-rune boundary panic or garbled output.
	labelRunes := []rune(label)
	matchSet := make(map[int]bool, len(matched))
	for _, idx := range matched {
		matchSet[idx] = true
	}
	var out strings.Builder
	for i, r := range labelRunes {
		if matchSet[i] {
			out.WriteString(base.Foreground(p.styles.T.Accent).Underline(true).Render(string(r)))
		} else {
			out.WriteString(base.Render(string(r)))
		}
	}
	return out.String()
}

// matchIndexes returns label rune indexes matched by the query: the substring
// span when the query appears contiguously, otherwise fuzzy indexes.
func matchIndexes(query, label string) []int {
	lowerLabel, lowerQuery := strings.ToLower(label), strings.ToLower(query)
	if start := strings.Index(lowerLabel, lowerQuery); start >= 0 && lowerQuery != "" {
		runeStart := len([]rune(label[:start]))
		n := len([]rune(query))
		idx := make([]int, n)
		for i := 0; i < n; i++ {
			idx[i] = runeStart + i
		}
		return idx
	}
	matches := fuzzy.Find(query, []string{label})
	if len(matches) == 0 {
		return nil
	}
	return matches[0].MatchedIndexes
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
