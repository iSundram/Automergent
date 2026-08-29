package components

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/iSundram/Automergent/internal/tui/themes"
)

// SelectorItem is one row of a SelectorOverlay.
type SelectorItem struct {
	Label          string
	Detail         string
	Current        bool
	Disabled       bool
	DisabledReason string
}

// SelectorSelectedMsg is emitted when the user confirms an enabled item.
type SelectorSelectedMsg struct {
	Index int
}

// SelectorOverlay is a generic full-width list overlay used by commands that
// need interactive selection (rewind checkpoints, permission rules, config
// shortcuts). It mirrors SessionBrowser's look and key handling.
type SelectorOverlay struct {
	styles *themes.Styles

	title        string
	hint         string
	items        []SelectorItem
	cursor       int
	scrollOffset int
	width        int
	height       int
	visible      bool
}

// NewSelectorOverlay creates a new SelectorOverlay.
func NewSelectorOverlay(styles *themes.Styles) SelectorOverlay {
	return SelectorOverlay{styles: styles}
}

// SetTitle sets the overlay header text.
func (s *SelectorOverlay) SetTitle(title string) { s.title = title }

// SetHint sets the footer hint line.
func (s *SelectorOverlay) SetHint(hint string) { s.hint = hint }

// SetItems replaces the rows and clamps the cursor.
func (s *SelectorOverlay) SetItems(items []SelectorItem) {
	s.items = items
	if s.cursor >= len(s.items) {
		s.cursor = max(0, len(s.items)-1)
	}
	if s.cursor < s.scrollOffset {
		s.scrollOffset = s.cursor
	}
}

// SetSize updates dimensions.
func (s *SelectorOverlay) SetSize(w, h int) { s.width, s.height = w, h }

// Show displays the overlay.
func (s *SelectorOverlay) Show() { s.visible = true }

// Hide hides the overlay.
func (s *SelectorOverlay) Hide() { s.visible = false }

// Visible reports whether the overlay is shown.
func (s *SelectorOverlay) Visible() bool { return s.visible }

// ItemCount reports the number of rows.
func (s *SelectorOverlay) ItemCount() int { return len(s.items) }

func (s *SelectorOverlay) maxVisibleItems() int {
	h := max(6, s.height-8)
	return h
}

// Update handles navigation keys; Enter confirms the highlighted enabled item.
func (s SelectorOverlay) Update(msg tea.Msg) (SelectorOverlay, tea.Cmd) {
	if !s.visible {
		return s, nil
	}
	if m, ok := msg.(tea.KeyMsg); ok {
		switch m.String() {
		case "esc", "q":
			s.visible = false
			return s, nil
		case "enter":
			if len(s.items) == 0 || s.cursor < 0 || s.cursor >= len(s.items) {
				return s, nil
			}
			if s.items[s.cursor].Disabled {
				return s, nil // selecting a disabled row is a no-op
			}
			s.visible = false
			idx := s.cursor
			return s, func() tea.Msg { return SelectorSelectedMsg{Index: idx} }
		case "up", "ctrl+p":
			if len(s.items) > 0 {
				if s.cursor > 0 {
					s.cursor--
				} else {
					s.cursor = len(s.items) - 1
				}
			}
		case "down", "ctrl+n", "tab":
			if len(s.items) > 0 {
				if s.cursor < len(s.items)-1 {
					s.cursor++
				} else {
					s.cursor = 0
				}
			}
		case "pgup":
			if len(s.items) > 0 {
				s.cursor = max(0, s.cursor-s.maxVisibleItems())
			}
		case "pgdown":
			if len(s.items) > 0 {
				s.cursor = min(len(s.items)-1, s.cursor+s.maxVisibleItems())
			}
		}
		s.clampScroll()
	}
	if m, ok := msg.(tea.MouseMsg); ok {
		switch m.Mouse().Button {
		case tea.MouseWheelUp:
			s.cursor = max(0, s.cursor-1)
		case tea.MouseWheelDown:
			s.cursor = min(len(s.items)-1, s.cursor+1)
		}
		s.clampScroll()
	}
	return s, nil
}

func (s *SelectorOverlay) clampScroll() {
	maxItems := s.maxVisibleItems()
	if s.cursor < s.scrollOffset {
		s.scrollOffset = s.cursor
	}
	if s.cursor >= s.scrollOffset+maxItems {
		s.scrollOffset = s.cursor - maxItems + 1
	}
	if s.scrollOffset < 0 {
		s.scrollOffset = 0
	}
}

// SelectedDisabledReason returns why the highlighted row cannot be chosen.
func (s SelectorOverlay) SelectedDisabledReason() string {
	if s.cursor < 0 || s.cursor >= len(s.items) {
		return ""
	}
	return s.items[s.cursor].DisabledReason
}

// View renders the overlay, or "" when hidden.
func (s SelectorOverlay) View() string {
	if !s.visible || s.width <= 0 || s.height <= 0 {
		return ""
	}
	w := max(12, s.width)
	rule := lipgloss.NewStyle().Foreground(s.styles.T.BorderNormal).Render(strings.Repeat("─", w))
	headerLeft := lipgloss.NewStyle().Foreground(s.styles.T.Accent).Bold(true).Render("  ▸  ") +
		lipgloss.NewStyle().Foreground(s.styles.T.Text).Bold(true).Render(strings.ToUpper(s.title))
	count := fmt.Sprintf("%d of %d", min(s.cursor+1, len(s.items)), len(s.items))
	if len(s.items) == 0 {
		count = "0 of 0"
	}
	header := joinEnds(headerLeft, lipgloss.NewStyle().Foreground(s.styles.T.Muted).Render(count)+"  ", w)

	maxItems := s.maxVisibleItems()
	end := min(s.scrollOffset+maxItems, len(s.items))
	lines := make([]string, 0, end-s.scrollOffset)
	for i := s.scrollOffset; i < end; i++ {
		lines = append(lines, s.renderItem(i, i == s.cursor, w-2))
	}
	if len(lines) == 0 {
		lines = append(lines, lipgloss.NewStyle().Foreground(s.styles.T.Muted).Italic(true).PaddingLeft(4).Width(w-2).Render("Nothing to choose from"))
	}

	hint := s.hint
	if hint == "" {
		hint = "↑↓ navigate · enter select · esc close"
	}
	footer := lipgloss.NewStyle().Foreground(s.styles.T.Muted).PaddingLeft(2).
		MaxWidth(max(1, w-2)).Render(hint)
	return lipgloss.JoinVertical(lipgloss.Left, rule, header, "", lipgloss.JoinVertical(lipgloss.Left, lines...), "", rule, footer)
}

func (s SelectorOverlay) renderItem(i int, selected bool, width int) string {
	item := s.items[i]
	indicator := "  "
	if selected {
		indicator = lipgloss.NewStyle().Foreground(s.styles.T.Accent).Render("▸ ")
	}
	marker := " "
	switch {
	case item.Current:
		marker = lipgloss.NewStyle().Foreground(s.styles.T.Green).Bold(true).Render("●")
	case item.Disabled:
		marker = lipgloss.NewStyle().Foreground(s.styles.T.Muted).Render("○")
	default:
		marker = lipgloss.NewStyle().Foreground(s.styles.T.Muted).Render("·")
	}
	labelColor := s.styles.T.Text
	if selected && !item.Disabled {
		labelColor = s.styles.T.Subtext
	}
	if item.Disabled {
		labelColor = s.styles.T.Muted
	}
	label := lipgloss.NewStyle().Foreground(labelColor).Render(truncateToWidth(item.Label, width/2))
	detail := item.Detail
	if item.Disabled && item.DisabledReason != "" {
		detail = strings.TrimSpace(detail + " — " + item.DisabledReason)
	}
	detailStyled := lipgloss.NewStyle().Foreground(s.styles.T.Muted).Render(truncateToWidth(detail, max(0, width-len(item.Label)-8)))
	return indicator + marker + " " + label + "  " + detailStyled
}

func truncateToWidth(s string, width int) string {
	if width <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= width {
		return s
	}
	if width <= 1 {
		return string(runes[:width])
	}
	return string(runes[:width-1]) + "…"
}

// Cursor reports the currently highlighted row index.
func (s SelectorOverlay) Cursor() int { return s.cursor }
