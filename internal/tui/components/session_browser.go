package components

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/iSundram/Automergent/internal/session"
	"github.com/iSundram/Automergent/internal/tui/themes"
)

// SessionSelectedMsg is sent when the user selects a session.
type SessionSelectedMsg struct {
	Session *session.Session
}

// sessionItem implements the rendered row for a session.
type sessionItem struct {
	sess *session.Session
}

func (s sessionItem) Title() string {
	if s.sess.Title != "" {
		return s.sess.Title
	}
	return "Untitled session"
}

func (s sessionItem) Description() string {
	parts := []string{}
	if !s.sess.CreatedAt.IsZero() {
		parts = append(parts, "created "+s.sess.CreatedAt.Format("Jan 2, 2006"))
	}
	if !s.sess.UpdatedAt.IsZero() {
		parts = append(parts, "modified "+formatRelativeTime(s.sess.UpdatedAt))
	}
	parts = append(parts, fmt.Sprintf("%d msgs", len(s.sess.Messages)))
	if s.sess.Provider != "" {
		if s.sess.Model != "" {
			parts = append(parts, s.sess.Provider+"/"+s.sess.Model)
		} else {
			parts = append(parts, s.sess.Provider)
		}
	} else if s.sess.Model != "" {
		parts = append(parts, s.sess.Model)
	}
	if s.sess.TotalInputTokens > 0 || s.sess.TotalOutputTokens > 0 {
		parts = append(parts,
			fmt.Sprintf("%s in/%s out", formatTokens(s.sess.TotalInputTokens), formatTokens(s.sess.TotalOutputTokens)))
	}
	if s.sess.WorkDir != "" {
		parts = append(parts, s.sess.WorkDir)
	}
	return strings.Join(parts, " · ")
}

// formatRelativeTime returns a human-friendly relative time string.
func formatRelativeTime(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		mins := int(d.Minutes())
		if mins == 1 {
			return "1 min ago"
		}
		return fmt.Sprintf("%d mins ago", mins)
	case d < 24*time.Hour:
		hrs := int(d.Hours())
		if hrs == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", hrs)
	case d < 7*24*time.Hour:
		days := int(d.Hours() / 24)
		if days == 1 {
			return "yesterday"
		}
		return fmt.Sprintf("%d days ago", days)
	default:
		return t.Format("Jan 2, 2006")
	}
}

// SessionBrowser shows a list of sessions in the same line-based style as the
// command palette: rules above and below, an accent ▍ indicator on the
// selected row, and a scrollbar on the right when the list overflows.
type SessionBrowser struct {
	styles *themes.Styles

	width        int
	height       int
	cursor       int
	scrollOffset int
	items        []sessionItem
	visible      bool
	currentID    string
}

// NewSessionBrowser creates a new SessionBrowser.
func NewSessionBrowser(styles *themes.Styles) SessionBrowser {
	return SessionBrowser{styles: styles}
}

// SetSessions populates the list.
func (sb *SessionBrowser) SetSessions(sessions []*session.Session) {
	items := make([]sessionItem, len(sessions))
	for i, s := range sessions {
		items[i] = sessionItem{sess: s}
	}
	sb.items = items
	if sb.cursor >= len(sb.items) && len(sb.items) > 0 {
		sb.cursor = len(sb.items) - 1
	}
	sb.updateScroll()
}

// SetCurrent marks the active session in the browser.
func (sb *SessionBrowser) SetCurrent(id string) { sb.currentID = id }

// ItemCount reports the number of sessions currently in the list.
func (sb *SessionBrowser) ItemCount() int {
	return len(sb.items)
}

// SetSize updates dimensions.
func (sb *SessionBrowser) SetSize(w, h int) {
	sb.width, sb.height = w, h
}

// Show displays the browser.
func (sb *SessionBrowser) Show() { sb.visible = true }

// Hide hides the browser.
func (sb *SessionBrowser) Hide() { sb.visible = false }

// Visible reports visibility.
func (sb SessionBrowser) Visible() bool { return sb.visible }

func (sb *SessionBrowser) updateScroll() {
	maxItems := sb.MaxVisibleItems()
	if sb.cursor < sb.scrollOffset {
		sb.scrollOffset = sb.cursor
	}
	if sb.cursor >= sb.scrollOffset+maxItems {
		sb.scrollOffset = sb.cursor - maxItems + 1
	}
}

// Update handles list events.
func (sb SessionBrowser) Update(msg tea.Msg) (SessionBrowser, tea.Cmd) {
	if !sb.visible {
		return sb, nil
	}
	if m, ok := msg.(tea.KeyMsg); ok {
		switch m.String() {
		case "esc", "q":
			sb.visible = false
			return sb, nil
		case "enter":
			if len(sb.items) > 0 && sb.cursor >= 0 && sb.cursor < len(sb.items) {
				item := sb.items[sb.cursor].sess
				if item == nil {
					return sb, nil
				}
				sb.visible = false
				return sb, func() tea.Msg { return SessionSelectedMsg{Session: item} }
			}
		case "up", "ctrl+p":
			if len(sb.items) == 0 {
				break
			}
			if sb.cursor > 0 {
				sb.cursor--
			} else {
				sb.cursor = len(sb.items) - 1
			}
		case "down", "ctrl+n", "tab":
			if len(sb.items) == 0 {
				break
			}
			if sb.cursor < len(sb.items)-1 {
				sb.cursor++
			} else {
				sb.cursor = 0
			}
		case "pgup":
			if len(sb.items) == 0 {
				break
			}
			sb.cursor = max(0, sb.cursor-sb.MaxVisibleItems())
		case "pgdown":
			if len(sb.items) == 0 {
				break
			}
			sb.cursor = min(len(sb.items)-1, sb.cursor+sb.MaxVisibleItems())
		}
		sb.updateScroll()
	}
	if m, ok := msg.(tea.MouseMsg); ok {
		switch m.Mouse().Button {
		case tea.MouseWheelUp:
			sb.cursor = max(0, sb.cursor-1)
		case tea.MouseWheelDown:
			sb.cursor = min(len(sb.items)-1, sb.cursor+1)
		}
		sb.updateScroll()
	}
	return sb, nil
}

// View renders the session browser.
func (sb SessionBrowser) View() string {
	if !sb.visible || sb.width <= 0 || sb.height <= 0 {
		return ""
	}
	w := max(12, sb.width)
	rule := lipgloss.NewStyle().Foreground(sb.styles.T.BorderNormal).Render(strings.Repeat("─", w))
	headerLeft := lipgloss.NewStyle().Foreground(sb.styles.T.Accent).Bold(true).Render("  󰆓  ") +
		lipgloss.NewStyle().Foreground(sb.styles.T.Text).Bold(true).Render("SESSION HISTORY")
	count := fmt.Sprintf("%d of %d", min(sb.cursor+1, len(sb.items)), len(sb.items))
	if len(sb.items) == 0 {
		count = "0 of 0"
	}
	header := joinEnds(headerLeft, lipgloss.NewStyle().Foreground(sb.styles.T.Muted).Render(count)+"  ", w)

	maxItems := sb.MaxVisibleItems()
	end := min(sb.scrollOffset+maxItems, len(sb.items))
	lines := make([]string, 0, maxItems*2)
	for i := sb.scrollOffset; i < end; i++ {
		lines = append(lines, sb.renderTitle(i, i == sb.cursor, w-2))
		lines = append(lines, sb.renderDescription(i, w-2))
	}
	if len(lines) == 0 {
		lines = append(lines, lipgloss.NewStyle().Foreground(sb.styles.T.Muted).Italic(true).PaddingLeft(4).Width(w-2).Render("No sessions in this workspace"))
	}
	lines = sb.addScrollbar(lines, maxItems)

	footerText := "↑↓ navigate · enter resume · esc close"
	if w >= 70 {
		footerText = "↑↓ navigate · pgup/pgdn page · enter resume · esc close"
	}
	footer := lipgloss.NewStyle().Foreground(sb.styles.T.Muted).PaddingLeft(2).
		MaxWidth(max(1, w-2)).Render(footerText)
	return lipgloss.JoinVertical(lipgloss.Left, rule, header, "", lipgloss.JoinVertical(lipgloss.Left, lines...), "", rule, footer)
}

func (sb SessionBrowser) renderTitle(i int, selected bool, width int) string {
	indicator := "  "
	if selected {
		indicator = lipgloss.NewStyle().Foreground(sb.styles.T.Accent).Render("▍ ")
	}
	titleColor := sb.styles.T.Text
	if selected {
		titleColor = sb.styles.T.Subtext
	}
	titleStyle := lipgloss.NewStyle().Foreground(titleColor)
	if selected {
		titleStyle = titleStyle.Bold(true)
	}
	title := sb.items[i].Title()
	if sb.items[i].sess != nil && sb.items[i].sess.ID == sb.currentID {
		title += "  " + lipgloss.NewStyle().Foreground(sb.styles.T.Green).Bold(true).Render("󰄬 Current")
	}
	left := "  " + indicator + titleStyle.Render(title)
	return lipgloss.NewStyle().Width(width).MaxWidth(width).Render(left)
}

func (sb SessionBrowser) renderDescription(i int, width int) string {
	desc := truncateCells(sb.items[i].Description(), max(1, width-6))
	left := "    " + lipgloss.NewStyle().Foreground(sb.styles.T.Muted).Render(desc)
	return lipgloss.NewStyle().Width(width).MaxWidth(width).Render(left)
}

func (sb SessionBrowser) addScrollbar(lines []string, viewportItems int) []string {
	if len(sb.items) <= viewportItems {
		return lines
	}
	trackHeight := len(lines)
	thumbSize := max(1, trackHeight*viewportItems/len(sb.items))
	thumbTop := 0
	if len(sb.items) > viewportItems {
		thumbTop = sb.scrollOffset * (trackHeight - thumbSize) / (len(sb.items) - viewportItems)
	}
	for i, line := range lines {
		bar := "│"
		if i >= thumbTop && i < thumbTop+thumbSize {
			bar = "┃"
		}
		lines[i] = lipgloss.NewStyle().Width(sb.width-2).MaxWidth(sb.width-2).Render(line) +
			lipgloss.NewStyle().Foreground(sb.styles.T.BorderNormal).Render(bar) + " "
	}
	return lines
}

// MaxVisibleItems reports how many sessions fit vertically (each row takes two
// lines: title + description). The browser renders inline above the input, so
// it stays compact.
func (sb SessionBrowser) MaxVisibleItems() int {
	maxItems := 6
	if sb.height > 0 && sb.height < 18 {
		maxItems = (sb.height - 6) / 2
	}
	return max(1, maxItems)
}

// Height reports the rendered height (rules, header, rows, footer) so layout
// can shrink the conversation while the browser is visible.
func (sb SessionBrowser) Height() int {
	if !sb.visible {
		return 0
	}
	visible := min(len(sb.items), sb.MaxVisibleItems())
	if visible < 1 {
		visible = 1
	}
	return visible*2 + 6
}
