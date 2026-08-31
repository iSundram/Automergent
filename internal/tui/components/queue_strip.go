package components

// The message-queue strip: what is waiting to be sent, made visible.
//
// Messages typed while a run is in flight are queued for later delivery, and
// until now the only evidence they existed was a counter in the status bar and a
// sentence in the info line. A queued message is a commitment the app has made
// on the user's behalf — "this will be sent when the run ends" — and a commitment
// you cannot see, inspect, or retract is a trap. This strip lists the queue
// where it acts (above the prompt), marks each item by how it will be delivered,
// and is the surface the per-item drop and pull-back keys act on.
//
// Height is a function of the item count only, capped at queueMaxRows: the strip
// never reflows the conversation because a queued message happened to be long.

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/iSundram/Automergent/internal/tui/render"
	"github.com/iSundram/Automergent/internal/tui/themes"
)

// QueueItem is one message waiting for delivery.
type QueueItem struct {
	// Text is the message body.
	Text string
	// Boundary requests delivery at the next tool boundary rather than after
	// the turn completes.
	Boundary bool
	// IsCmd marks a slash command, which dispatches locally.
	IsCmd bool
}

// queueMaxRows caps the strip. Overflow is announced, never silently dropped.
const queueMaxRows = 4

// QueueStrip renders the pending message queue above the prompt.
type QueueStrip struct {
	styles *themes.Styles
	items  []QueueItem
	cursor int
	width  int
}

// NewQueueStrip creates an empty strip.
func NewQueueStrip(styles *themes.Styles) *QueueStrip {
	return &QueueStrip{styles: styles}
}

// SetWidth updates the available width.
func (q *QueueStrip) SetWidth(w int) { q.width = w }

// SetItems replaces the queue contents and clamps the cursor.
func (q *QueueStrip) SetItems(items []QueueItem) {
	q.items = items
	q.clampCursor()
}

// Items returns the current queue.
func (q QueueStrip) Items() []QueueItem { return q.items }

// Len reports how many messages are queued.
func (q QueueStrip) Len() int { return len(q.items) }

// MoveCursor moves the highlight within bounds.
func (q *QueueStrip) MoveCursor(delta int) {
	q.cursor += delta
	q.clampCursor()
}

func (q *QueueStrip) clampCursor() {
	// The cursor defaults to the newest item (the one just typed), because that
	// is the one most likely to need retracting.
	if q.cursor >= len(q.items) {
		q.cursor = len(q.items) - 1
	}
	if q.cursor < 0 {
		q.cursor = 0
	}
	if len(q.items) == 0 {
		q.cursor = 0
	}
}

// Selected returns the highlighted item.
func (q QueueStrip) Selected() (QueueItem, int, bool) {
	if q.cursor < 0 || q.cursor >= len(q.items) {
		return QueueItem{}, 0, false
	}
	return q.items[q.cursor], q.cursor, true
}

// Height returns the rendered height in rows: a header, the windowed items, and
// an overflow notice when the queue is longer than the window. Zero when empty.
func (q QueueStrip) Height() int {
	if len(q.items) == 0 || q.width <= 0 {
		return 0
	}
	rows := len(q.items)
	if rows > queueMaxRows {
		return 2 + queueMaxRows + 1
	}
	return 2 + rows
}

// View renders the strip. It returns "" when the queue is empty — an empty
// queue costs no rows and draws no chrome.
func (q QueueStrip) View() string {
	if len(q.items) == 0 || q.width <= 0 {
		return ""
	}

	inner := q.width - 4
	if inner < 20 {
		inner = 20
	}

	var b strings.Builder
	title := lipgloss.NewStyle().
		Foreground(q.styles.T.Accent).Bold(true).
		Render(fmt.Sprintf("queued %d", len(q.items)))
	hint := q.styles.Dim.Render("ctrl+x drop · ctrl+o edit · shift+up/down select · esc clear all")
	used := render.Width(title) + render.Width(hint)
	gap := inner - used - 2
	if gap < 1 {
		gap = 1
		hint = "" // too narrow for both: the count outranks the hint
	}
	b.WriteString(title + " " + q.styles.Dim.Render(render.Rule(gap)))
	if hint != "" {
		b.WriteString(" " + hint)
	}
	b.WriteString("\n")

	start, end := q.window()
	for i := start; i < end; i++ {
		b.WriteString(q.row(q.items[i], i, i == q.cursor, i == start && start > 0, inner) + "\n")
	}
	if hidden := len(q.items) - (end - start); hidden > 0 {
		b.WriteString(q.styles.Dim.Render(fmt.Sprintf("  %s %d more",
			render.GlyphEllipsis, hidden)) + "\n")
	}

	return lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), true, false, false, false).
		BorderForeground(q.styles.T.BorderNormal).
		Padding(0, 2).
		Width(q.width).
		Render(strings.TrimRight(b.String(), "\n"))
}

// window returns the visible slice, anchored to the cursor so the highlighted
// item is always on screen.
func (q QueueStrip) window() (start, end int) {
	if len(q.items) <= queueMaxRows {
		return 0, len(q.items)
	}
	start = q.cursor - queueMaxRows/2
	if start < 0 {
		start = 0
	}
	if start > len(q.items)-queueMaxRows {
		start = len(q.items) - queueMaxRows
	}
	return start, start + queueMaxRows
}

// row renders one queued message:
//
//	▸ 1  → fix the tests next
//	  2  / run the linter
//
// The delivery mark says how the message will be sent: the boundary arrow for a
// steered message, nothing for an end-of-turn message. A slash command shows its
// slash, which is already its own kind mark.
func (q QueueStrip) row(it QueueItem, idx int, selected bool, continued bool, inner int) string {
	t := q.styles.T

	cursor := "  "
	if selected {
		cursor = lipgloss.NewStyle().Foreground(t.Accent).Bold(true).Render(render.GlyphCursor) + " "
	}

	pos := q.styles.Dim.Render(render.CellRight(fmt.Sprintf("%d", idx+1), 2))

	mark := "  "
	if it.Boundary {
		// The arrow is the steering affordance: this message crosses the tool
		// boundary rather than waiting for the turn to end.
		mark = lipgloss.NewStyle().Foreground(t.Accent).Render(render.GlyphTo) + " "
	}

	body := it.Text
	if it.IsCmd {
		body = render.GlyphSep + body // commands are dimmed whole, not marked
	}
	style := lipgloss.NewStyle().Foreground(t.Text)
	if it.IsCmd {
		style = q.styles.Dim
	}

	// Fixed budget: cursor(2) + pos(2) + mark(2) + gaps(2).
	avail := inner - 8
	if avail < 8 {
		avail = 8
	}
	line := cursor + pos + " " + mark + style.Render(render.Cell(render.FirstLine(body), avail))

	if selected {
		return lipgloss.NewStyle().Bold(true).Render(line)
	}
	return line
}
