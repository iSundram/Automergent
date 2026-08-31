package components

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/iSundram/Automergent/internal/session"
	"github.com/iSundram/Automergent/internal/tui/themes"
)

// keyText returns the printable text a key press carries, if any.
func keyText(m tea.KeyMsg) string {
	if kp, ok := m.(tea.KeyPressMsg); ok {
		return kp.Text
	}
	return ""
}

// SessionSelectedMsg is sent when the user selects a session.
type SessionSelectedMsg struct {
	Session *session.Session
}

// SessionDeletedMsg is sent when the user confirms deletion of a session.
type SessionDeletedMsg struct {
	Session *session.Session
}

// SessionRenamedMsg is sent when the user commits an inline rename.
type SessionRenamedMsg struct {
	Session *session.Session
	Title   string
}

// sessionItem implements the rendered row for a session.
type sessionItem struct {
	sess *session.Session
}

func (s sessionItem) Title() string {
	if s.sess.Title != "" {
		return s.sess.Title
	}
	if fallback := s.firstUserLine(); fallback != "" {
		return fallback
	}
	return "Untitled session"
}

// firstUserLine is the deterministic fallback title: the first line of the
// first user message. It keeps unnamed sessions identifiable even before an
// auto-title lands.
func (s sessionItem) firstUserLine() string {
	if s.sess == nil {
		return ""
	}
	for _, m := range s.sess.Messages {
		if m.Role != "user" {
			continue
		}
		line := strings.TrimSpace(m.PlaintextForHistory())
		if idx := strings.IndexAny(line, "\n"); idx >= 0 {
			line = line[:idx]
		}
		if len(line) > 80 {
			line = line[:77] + "..."
		}
		return line
	}
	return ""
}

// messageCount reports the session's message count: the loaded slice when
// present, otherwise the lite-listing count carried in metadata (the picker
// lists transcripts without parsing messages).
func (s sessionItem) messageCount() int {
	if len(s.sess.Messages) > 0 || s.sess.Metadata == nil {
		return len(s.sess.Messages)
	}
	if n, err := strconv.Atoi(s.sess.Metadata["lite_message_count"]); err == nil {
		return n
	}
	return len(s.sess.Messages)
}

func (s sessionItem) Description() string {
	parts := []string{}
	if !s.sess.UpdatedAt.IsZero() {
		parts = append(parts, formatRelativeTime(s.sess.UpdatedAt))
	}
	parts = append(parts, fmt.Sprintf("%d msgs", s.messageCount()))
	if s.sess.SizeBytes > 0 {
		parts = append(parts, formatBytes(s.sess.SizeBytes))
	}
	if s.sess.Provider != "" {
		if s.sess.Model != "" {
			parts = append(parts, s.sess.Provider+"/"+s.sess.Model)
		} else {
			parts = append(parts, s.sess.Provider)
		}
	}
	if s.sess.TotalInputTokens > 0 || s.sess.TotalOutputTokens > 0 {
		parts = append(parts,
			fmt.Sprintf("%s in/%s out", formatTokens(s.sess.TotalInputTokens), formatTokens(s.sess.TotalOutputTokens)))
	}
	return strings.Join(parts, " · ")
}

// formatBytes returns a human-friendly byte size.
func formatBytes(b int64) string {
	switch {
	case b >= 1024*1024:
		return fmt.Sprintf("%.1fMB", float64(b)/(1024*1024))
	case b >= 1024:
		return fmt.Sprintf("%.1fKB", float64(b)/1024)
	default:
		return fmt.Sprintf("%dB", b)
	}
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

// SessionBrowser shows a searchable, renamable list of sessions in the same
// line-based style as the command palette: the title sits between rule
// segments, typing filters, an accent ▍ indicator marks the selected row, and
// a scrollbar appears when the list overflows.
type SessionBrowser struct {
	styles *themes.Styles

	width        int
	height       int
	cursor       int
	scrollOffset int
	items        []sessionItem
	visible      bool
	currentID    string

	// query is the typed search filter (matched against title and first
	// user message, case-insensitively).
	query string

	// projectLabel is the workspace name shown above the rows.
	projectLabel string

	// renaming holds the session being renamed inline (ctrl+r); keystrokes
	// edit draft instead of the query while set.
	renaming *session.Session
	draft    string

	// pendingDeleteID is the session awaiting a second ctrl+d. Deleting is
	// destructive, so the first press only arms it; any other key or cursor
	// move disarms.
	pendingDeleteID string
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
	sb.clampCursor()
	sb.updateScroll()
}

// SetCurrent marks the active session in the browser.
func (sb *SessionBrowser) SetCurrent(id string) { sb.currentID = id }

// SetProjectLabel sets the workspace name shown above the rows.
func (sb *SessionBrowser) SetProjectLabel(label string) { sb.projectLabel = label }

// SelectCurrent moves the cursor onto the current session (or the top of the
// list when it is absent) so reopening the browser doesn't land on a stale
// position.
func (sb *SessionBrowser) SelectCurrent() {
	sb.pendingDeleteID = ""
	for i, item := range sb.items {
		if item.sess != nil && item.sess.ID == sb.currentID {
			sb.cursor = i
			sb.updateScroll()
			return
		}
	}
	sb.cursor = 0
	sb.updateScroll()
}

// RemoveByID drops a session from the list (after deletion) and keeps the
// cursor on a valid row.
func (sb *SessionBrowser) RemoveByID(id string) {
	kept := sb.items[:0]
	for _, item := range sb.items {
		if item.sess != nil && item.sess.ID == id {
			continue
		}
		kept = append(kept, item)
	}
	sb.items = kept
	if sb.pendingDeleteID == id {
		sb.pendingDeleteID = ""
	}
	if sb.renaming != nil && sb.renaming.ID == id {
		sb.renaming = nil
	}
	sb.clampCursor()
	sb.updateScroll()
}

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
func (sb *SessionBrowser) Hide() {
	sb.visible = false
	sb.resetTransientState()
}

// Visible reports visibility.
func (sb SessionBrowser) Visible() bool { return sb.visible }

// Query reports the active search filter.
func (sb *SessionBrowser) Query() string { return sb.query }

func (sb *SessionBrowser) resetTransientState() {
	sb.query = ""
	sb.renaming = nil
	sb.draft = ""
	sb.pendingDeleteID = ""
}

func (sb *SessionBrowser) clampCursor() {
	if sb.cursor >= len(sb.items) {
		sb.cursor = max(0, len(sb.items)-1)
	}
}

// filteredItems returns the rows matching the query plus their indices into
// sb.items.
func (sb SessionBrowser) filteredItems() []int {
	if sb.query == "" {
		out := make([]int, len(sb.items))
		for i := range sb.items {
			out[i] = i
		}
		return out
	}
	q := strings.ToLower(sb.query)
	var out []int
	for i, item := range sb.items {
		if item.sess == nil {
			continue
		}
		hay := strings.ToLower(item.Title() + " " + item.firstUserLine() + " " + item.sess.Title)
		if strings.Contains(hay, q) {
			out = append(out, i)
		}
	}
	return out
}

func (sb *SessionBrowser) updateScroll() {
	maxItems := sb.MaxVisibleItems()
	if sb.cursor < sb.scrollOffset {
		sb.scrollOffset = sb.cursor
	}
	if sb.cursor >= sb.scrollOffset+maxItems {
		sb.scrollOffset = sb.cursor - maxItems + 1
	}
}

// Update handles list events. While visible the browser owns the keyboard:
// typing filters, ctrl+r renames inline, ctrl+d arms a delete.
func (sb SessionBrowser) Update(msg tea.Msg) (SessionBrowser, tea.Cmd) {
	if !sb.visible {
		return sb, nil
	}
	if m, ok := msg.(tea.KeyMsg); ok {
		key := m.String()
		// Inline rename mode swallows printable keys into the draft.
		if sb.renaming != nil {
			switch key {
			case "esc":
				sb.renaming = nil
				sb.draft = ""
				return sb, nil
			case "enter":
				title := strings.TrimSpace(sb.draft)
				target := sb.renaming
				sb.renaming = nil
				sb.draft = ""
				if title == "" || target == nil {
					return sb, nil
				}
				return sb, func() tea.Msg { return SessionRenamedMsg{Session: target, Title: title} }
			case "backspace":
				if sb.draft != "" {
					r := []rune(sb.draft)
					sb.draft = string(r[:len(r)-1])
				}
				return sb, nil
			case "ctrl+u":
				sb.draft = ""
				return sb, nil
			}
			if text := keyText(m); text != "" {
				sb.draft += text
				return sb, nil
			}
			return sb, nil
		}

		switch key {
		case "esc":
			if sb.query != "" {
				sb.query = ""
				sb.clampCursor()
				sb.updateScroll()
				return sb, nil
			}
			sb.visible = false
			sb.resetTransientState()
			return sb, nil
		case "enter":
			sb.pendingDeleteID = ""
			if item := sb.selectedSession(); item != nil {
				sb.visible = false
				sb.resetTransientState()
				return sb, func() tea.Msg { return SessionSelectedMsg{Session: item} }
			}
		case "ctrl+d", "delete":
			item := sb.selectedSession()
			if item == nil || item.ID == sb.currentID {
				// Deleting the live session would corrupt the app state.
				sb.pendingDeleteID = ""
				return sb, nil
			}
			if sb.pendingDeleteID == item.ID {
				sb.pendingDeleteID = ""
				return sb, func() tea.Msg { return SessionDeletedMsg{Session: item} }
			}
			sb.pendingDeleteID = item.ID
			return sb, nil
		case "ctrl+r":
			if item := sb.selectedSession(); item != nil {
				sb.renaming = item
				sb.draft = item.Title
				sb.pendingDeleteID = ""
			}
			return sb, nil
		case "ctrl+u":
			sb.query = ""
			sb.clampCursor()
			sb.updateScroll()
			return sb, nil
		case "backspace":
			if sb.query != "" {
				r := []rune(sb.query)
				sb.query = string(r[:len(r)-1])
				sb.clampCursor()
				sb.updateScroll()
			}
			return sb, nil
		case "up", "ctrl+p":
			if idxs := sb.filteredItems(); len(idxs) > 0 {
				sb.pendingDeleteID = ""
				for n, idx := range idxs {
					if idx == sb.cursor {
						if n == 0 {
							sb.cursor = idxs[len(idxs)-1]
						} else {
							sb.cursor = idxs[n-1]
						}
						break
					}
				}
			}
		case "down", "ctrl+n", "tab":
			if idxs := sb.filteredItems(); len(idxs) > 0 {
				sb.pendingDeleteID = ""
				found := false
				for n, idx := range idxs {
					if idx == sb.cursor {
						found = true
						if n == len(idxs)-1 {
							sb.cursor = idxs[0]
						} else {
							sb.cursor = idxs[n+1]
						}
						break
					}
				}
				if !found {
					sb.cursor = idxs[0]
				}
			}
		case "pgup":
			sb.page(-1)
		case "pgdown":
			sb.page(1)
		default:
			// Printable text goes into the search filter; any other key
			// disarms a pending delete.
			if text := keyText(m); text != "" {
				sb.query += text
				sb.pendingDeleteID = ""
				// Snap the cursor onto the first match.
				if idxs := sb.filteredItems(); len(idxs) > 0 {
					sb.cursor = idxs[0]
				} else {
					sb.clampCursor()
				}
				sb.updateScroll()
				return sb, nil
			}
			sb.pendingDeleteID = ""
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
		sb.pendingDeleteID = ""
		sb.updateScroll()
	}
	return sb, nil
}

// page moves the cursor by a viewport's worth of rows among filtered items.
func (sb *SessionBrowser) page(dir int) {
	idxs := sb.filteredItems()
	if len(idxs) == 0 {
		return
	}
	sb.pendingDeleteID = ""
	step := sb.MaxVisibleItems()
	if dir < 0 {
		step = -step
	}
	// Find current position among filtered rows, step, wrap.
	for n, idx := range idxs {
		if idx == sb.cursor {
			pos := n + step
			if pos < 0 {
				pos = 0
			}
			if pos >= len(idxs) {
				pos = len(idxs) - 1
			}
			sb.cursor = idxs[pos]
			return
		}
	}
	sb.cursor = idxs[0]
}

// selectedSession returns the session under the cursor, honoring the filter:
// when a query is active the cursor tracks filtered rows, so jump through the
// filtered indices to the row the user actually sees highlighted. Falls back
// to the first match when the cursor sits on a filtered-out row.
func (sb SessionBrowser) selectedSession() *session.Session {
	idxs := sb.filteredItems()
	for _, idx := range idxs {
		if idx == sb.cursor {
			return sb.items[idx].sess
		}
	}
	if len(idxs) > 0 {
		return sb.items[idxs[0]].sess
	}
	return nil
}

// View renders the session browser.
func (sb SessionBrowser) View() string {
	if !sb.visible || sb.width <= 0 || sb.height <= 0 {
		return ""
	}
	w := max(12, sb.width)
	ruleStyle := lipgloss.NewStyle().Foreground(sb.styles.T.BorderNormal)

	// Header: the title sits between rule segments — same line-based grammar
	// as every other surface in the app.
	titleText := "  Resume session  "
	title := lipgloss.NewStyle().Foreground(sb.styles.T.Accent).Bold(true).Render(titleText)
	ruleWidth := max(0, (w-len(titleText))/2)
	header := ruleStyle.Render(strings.Repeat("─", ruleWidth)) + title +
		ruleStyle.Render(strings.Repeat("─", max(0, w-ruleWidth-len(titleText))))

	// Search line with position count: cursor position among matches over
	// match total ("6/12"), so a query narrows it ("3/5") meaningfully.
	filtered := sb.filteredItems()
	pos := 0
	for n, idx := range filtered {
		if idx == sb.cursor {
			pos = n + 1
			break
		}
	}
	count := fmt.Sprintf("%d/%d", pos, len(filtered))
	if len(filtered) == 0 {
		count = fmt.Sprintf("0/%d", len(sb.items))
	}
	searchText := " Search…"
	if sb.query != "" {
		searchText = " " + sb.query
	}
	searchLeft := lipgloss.NewStyle().Foreground(sb.styles.T.Muted).Render("⌕") + searchText
	search := joinEnds(searchLeft, lipgloss.NewStyle().Foreground(sb.styles.T.Muted).Render(count+"  "), w)

	lines := make([]string, 0, 64)
	lines = append(lines, header, search)
	if sb.projectLabel != "" {
		lines = append(lines, lipgloss.NewStyle().Foreground(sb.styles.T.Text).Bold(true).PaddingLeft(2).Render(sb.projectLabel))
	}
	lines = append(lines, "")

	maxItems := sb.MaxVisibleItems()
	drawn := 0
	for _, idx := range filtered {
		if drawn >= maxItems {
			break
		}
		if idx < sb.scrollOffset || idx >= sb.scrollOffset+maxItems {
			continue
		}
		lines = append(lines, sb.renderTitle(idx, idx == sb.cursor, w-2))
		lines = append(lines, sb.renderDescription(idx, w-2))
		drawn++
	}
	if len(filtered) == 0 {
		if sb.query != "" {
			lines = append(lines, lipgloss.NewStyle().Foreground(sb.styles.T.Muted).Italic(true).PaddingLeft(4).Width(w-2).
				Render("No sessions match "+sb.query))
		} else {
			lines = append(lines, lipgloss.NewStyle().Foreground(sb.styles.T.Muted).Italic(true).PaddingLeft(4).Width(w-2).
				Render("No sessions in this workspace"))
		}
	}
	lines = sb.addScrollbar(lines, maxItems, len(filtered))

	lines = append(lines, "", ruleStyle.Render(strings.Repeat("─", w)))
	lines = append(lines, sb.footerLines(w)...)
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

// footerLines renders the two hint lines, adapting to the active mode
// (searching, renaming, pending delete).
func (sb SessionBrowser) footerLines(w int) []string {
	var l1, l2 string
	switch {
	case sb.renaming != nil:
		l1 = "type new title · enter save · esc cancel"
		l2 = "↑↓ navigate · enter resume · ctrl+d delete · ctrl+r rename"
	case sb.pendingDeleteID != "":
		l1 = "press ctrl+d again to delete · any other key cancels"
		l2 = "↑↓ navigate · enter resume · ctrl+d delete · ctrl+r rename"
	default:
		l1 = "↑↓ navigate · enter resume · ctrl+d delete · ctrl+r rename"
		l2 = "type to search · pgup/pgdn page · esc close"
	}
	style := lipgloss.NewStyle().Foreground(sb.styles.T.Muted).PaddingLeft(2).MaxWidth(max(1, w-2))
	return []string{style.Render(l1), style.Render(l2)}
}

func (sb SessionBrowser) renderTitle(i int, selected bool, width int) string {
	indicator := "  "
	if selected {
		indicator = lipgloss.NewStyle().Foreground(sb.styles.T.Accent).Render("▸ ")
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
	if sb.renaming != nil && sb.items[i].sess != nil && sb.items[i].sess.ID == sb.renaming.ID {
		title = sb.draft
		titleStyle = titleStyle.Underline(true)
	}
	if sb.items[i].sess != nil && sb.items[i].sess.ID == sb.currentID {
		title += "  " + lipgloss.NewStyle().Foreground(sb.styles.T.Green).Bold(true).Render("✓ Current")
	}
	if sb.items[i].sess != nil && sb.items[i].sess.ID == sb.pendingDeleteID {
		title += "  " + lipgloss.NewStyle().Foreground(sb.styles.T.Red).Bold(true).Render("✗ delete?")
	}
	left := "  " + indicator + titleStyle.Render(title)
	return lipgloss.NewStyle().Width(width).MaxWidth(width).Render(left)
}

func (sb SessionBrowser) renderDescription(i int, width int) string {
	desc := truncateCells(sb.items[i].Description(), max(1, width-6))
	left := "    " + lipgloss.NewStyle().Foreground(sb.styles.T.Muted).Render(desc)
	return lipgloss.NewStyle().Width(width).MaxWidth(width).Render(left)
}

func (sb SessionBrowser) addScrollbar(lines []string, viewportItems, rowCount int) []string {
	if rowCount <= viewportItems {
		return lines
	}
	// The scrollable region starts after the header/search/project block.
	blockHeight := len(lines)
	trackHeight := blockHeight
	thumbSize := max(1, trackHeight*viewportItems/rowCount)
	thumbTop := 0
	if rowCount > viewportItems {
		thumbTop = sb.scrollOffset * (trackHeight - thumbSize) / (rowCount - viewportItems)
	}
	for i, line := range lines {
		bar := "│"
		if i >= thumbTop && i < thumbTop+thumbSize {
			bar = "█"
		}
		// MaxWidth truncates (never wraps): full-width rules and headers just
		// lose their last two cells to the scrollbar instead of breaking lines.
		lines[i] = lipgloss.NewStyle().MaxWidth(sb.width - 2).Render(line) +
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

// Height reports the rendered height so layout can shrink the conversation
// while the browser is visible.
func (sb SessionBrowser) Height() int {
	if !sb.visible {
		return 0
	}
	visible := min(len(sb.filteredItems()), sb.MaxVisibleItems())
	if visible < 1 {
		visible = 1
	}
	// rule + search + project + blank + rows*2 + blank + rule + 2 footer lines
	return visible*2 + 8
}

