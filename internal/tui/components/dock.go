package components

// The background dock: a one-row rail that is always on, and a tray that opens
// on demand.
//
// The old dock was one always-visible list that cost four to seven rows of
// transcript and recomputed its own height every second, because a row grew
// taller the moment a preview string became non-empty. The result was a
// conversation that reflowed continuously while any background work ran — the
// text you were reading slid up the screen once a second — in exchange for
// information you were mostly not looking at.
//
// Splitting it fixes both. The rail costs exactly one row, never changes height,
// and answers the only question an idle user has: is anything running, and did
// anything break. The tray is the full list, and because it only appears when
// focused it can afford columns, nesting and a preview without disturbing
// anything. Height still depends only on the number of rows, never on their
// content, so opening the tray reflows the view once and then holds still.

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/iSundram/Automergent/internal/tui/render"
	"github.com/iSundram/Automergent/internal/tui/themes"
)

// DockEntryKind discriminates bottom-dock rows.
type DockEntryKind string

const (
	DockShell DockEntryKind = "shell"
	DockAgent DockEntryKind = "agent"
)

// DockEntry is one background task: a shell session or a subagent.
type DockEntry struct {
	Kind DockEntryKind
	ID   string
	// Label is the command line for a shell, the agent name for an agent.
	Label string
	// Status is canonical, so the mark and colour match every other surface.
	Status render.Status
	// Activity is the live right-hand cell: current tool, tail line, or outcome.
	Activity string
	// AgentKind colours an agent row by role; unused for shells.
	AgentKind render.AgentKind
	// Depth, Last and Children describe this row's place in the spawn tree.
	Depth    int
	Last     bool
	Children int
	// ToolCount is how many tools an agent has run.
	ToolCount int
	// HasStderr flags a shell that has written to stderr without failing.
	HasStderr bool

	Started  time.Time
	Finished time.Time
}

// Elapsed reports how long the task has been running, or how long it took.
func (e DockEntry) Elapsed() string {
	if e.Started.IsZero() {
		return ""
	}
	end := e.Finished
	if end.IsZero() {
		end = time.Now()
	}
	return render.Elapsed(int(end.Sub(e.Started).Seconds()))
}

// BottomDock renders the background rail and tray.
type BottomDock struct {
	styles  *themes.Styles
	entries []DockEntry
	cursor  int
	focused bool
	width   int
	// concurrencyCap is the subagent limit, shown in the tray header so the
	// ceiling the orchestrator is working against is visible rather than being
	// discovered by hitting it.
	concurrencyCap int
}

// trayMaxRows caps the tray at a height that leaves the conversation readable
// even on a short terminal. Overflow is announced, never silently dropped.
const trayMaxRows = 6

// NewBottomDock creates an unfocused, empty dock.
func NewBottomDock(styles *themes.Styles) *BottomDock {
	return &BottomDock{styles: styles, concurrencyCap: 8}
}

// SetWidth updates the available width.
func (d *BottomDock) SetWidth(w int) { d.width = w }

// SetConcurrencyCap records the subagent ceiling for the tray header.
func (d *BottomDock) SetConcurrencyCap(n int) { d.concurrencyCap = n }

// SetEntries replaces the rows and clamps the cursor.
func (d *BottomDock) SetEntries(entries []DockEntry) {
	d.entries = entries
	d.clampCursor()
}

// Entries returns the current rows.
func (d BottomDock) Entries() []DockEntry { return d.entries }

// Len reports the number of rows.
func (d BottomDock) Len() int { return len(d.entries) }

// HasContent reports whether any row exists.
func (d BottomDock) HasContent() bool { return len(d.entries) > 0 }

// Focused reports whether the tray is open and owns the keyboard.
func (d BottomDock) Focused() bool { return d.focused }

// SetFocused opens or closes the tray.
func (d *BottomDock) SetFocused(v bool) {
	d.focused = v
	d.clampCursor()
}

// MoveCursor moves the highlight within bounds.
func (d *BottomDock) MoveCursor(delta int) {
	if len(d.entries) == 0 {
		return
	}
	d.cursor += delta
	d.clampCursor()
}

// CursorTo jumps the highlight to an absolute row.
func (d *BottomDock) CursorTo(i int) {
	d.cursor = i
	d.clampCursor()
}

func (d *BottomDock) clampCursor() {
	if len(d.entries) == 0 {
		d.cursor = 0
		return
	}
	if d.cursor < 0 {
		d.cursor = 0
	}
	if d.cursor >= len(d.entries) {
		d.cursor = len(d.entries) - 1
	}
}

// Selected returns the highlighted entry.
func (d BottomDock) Selected() (DockEntry, bool) {
	if d.cursor < 0 || d.cursor >= len(d.entries) {
		return DockEntry{}, false
	}
	return d.entries[d.cursor], true
}

// Counts summarises the rows for the rail.
func (d BottomDock) Counts() (running, failed, total int) {
	for _, e := range d.entries {
		total++
		switch e.Status {
		case render.StatusRunning, render.StatusIdle, render.StatusQueued:
			running++
		case render.StatusFailed:
			failed++
		}
	}
	return running, failed, total
}

// Height returns the rendered height in rows.
//
// This is a real function of the entry count now, and the charter test asserts
// it equals lipgloss.Height(View()). The previous version was dead code that had
// already drifted from what View actually drew, because layout measured the
// rendered string instead of asking.
func (d BottomDock) Height() int {
	if len(d.entries) == 0 || d.width <= 0 {
		return 0
	}
	if !d.focused {
		return 0 // the rail is rendered by the info line region, not here
	}
	rows := len(d.entries)
	overflow := 0
	if rows > trayMaxRows {
		overflow = 1
		rows = trayMaxRows
	}
	// top border + header + rows + overflow notice + one preview row for the
	// cursored entry. The border is part of the tray's chrome and therefore of
	// its height: forgetting it is how layout drift begins.
	return 1 + 1 + rows + overflow + 1
}

// RailText returns the one-row summary for the always-on rail, or "" when there
// is no background work to report. The dock does not draw this itself: it is a
// sentence for the info line, which owns the row above the prompt.
func (d BottomDock) RailText() string {
	running, failed, total := d.Counts()
	if total == 0 {
		return ""
	}
	var parts []string
	if running > 0 {
		parts = append(parts, fmt.Sprintf("%d running", running))
	}
	if failed > 0 {
		parts = append(parts, fmt.Sprintf("%d failed", failed))
	}
	if len(parts) == 0 {
		parts = append(parts, fmt.Sprintf("%d finished", total))
	}
	return strings.Join(parts, render.GlyphSep)
}

// View renders the tray. It returns "" when the tray is closed — the rail is the
// dock's resting state and costs no rows of its own.
func (d BottomDock) View() string {
	if !d.focused || len(d.entries) == 0 || d.width <= 0 {
		return ""
	}

	// The tray lives inside Padding(0, 2) inside Width(d.width), so its line
	// budget is width-4. Composing rows to anything wider makes lipgloss wrap
	// them inside the border, and every claim the dock makes about its height
	// becomes a lie.
	inner := d.width - 4
	if inner < 1 {
		inner = 1
	}

	var b strings.Builder
	b.WriteString(d.header(inner) + "\n")

	start, end := d.window()
	for i := start; i < end; i++ {
		b.WriteString(d.row(d.entries[i], i == d.cursor, inner) + "\n")
	}
	if hidden := len(d.entries) - (end - start); hidden > 0 {
		b.WriteString(d.styles.Dim.Render(fmt.Sprintf("  %s %d more",
			render.GlyphEllipsis, hidden)) + "\n")
	}
	b.WriteString(d.preview(inner))

	// Clip every line to the budget before the border is applied. The row and
	// header builders already fit their columns to it; this is the backstop
	// that makes the invariant unconditional rather than conventional.
	body := strings.TrimRight(b.String(), "\n")
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		lines[i] = render.Clip(line, inner)
	}

	return lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), true, false, false, false).
		BorderForeground(d.styles.T.BorderFocused).
		Padding(0, 2).
		Width(d.width).
		Render(strings.Join(lines, "\n"))
}

// header is the tray's title row: a label, the running/cap ratio, and a hairline
// out to the right edge.
func (d BottomDock) header(inner int) string {
	running, _, _ := d.Counts()
	title := lipgloss.NewStyle().
		Foreground(d.styles.T.Accent).Bold(true).
		Render("background")

	ratio := ""
	if running > 0 && d.concurrencyCap > 0 {
		ratio = d.styles.Dim.Render(fmt.Sprintf("%d/%d", running, d.concurrencyCap))
	}

	used := render.Width(title) + render.Width(ratio)
	gap := inner - used - 2
	if gap < 1 {
		gap = 1
	}
	line := title + " " + d.styles.Dim.Render(render.Rule(gap))
	if ratio != "" {
		line += " " + ratio
	}
	return line
}

// window returns the visible slice of rows, keeping the cursor inside it.
func (d BottomDock) window() (start, end int) {
	if len(d.entries) <= trayMaxRows {
		return 0, len(d.entries)
	}
	start = d.cursor - trayMaxRows/2
	if start < 0 {
		start = 0
	}
	if start > len(d.entries)-trayMaxRows {
		start = len(d.entries) - trayMaxRows
	}
	return start, start + trayMaxRows
}

// row renders one task as a fixed grid:
//
//	▸ ● bash   npm run dev              compiled ok         0:12
//	  ● agent  coord dock redesign      3 children          1:12
//	    └ ● agent  review verify        grep                0:03
//
// Every cell is measured in rendered cells, so the columns line up regardless of
// styling. The version this replaces reached for fmt's %-9s on an already-styled
// status string, which pads to nine bytes of escape sequence — that is, not at
// all — so no two rows ever aligned.
func (d BottomDock) row(e DockEntry, selected bool, inner int) string {
	t := d.styles.T

	cursor := "  "
	if selected {
		cursor = lipgloss.NewStyle().Foreground(t.Accent).Bold(true).Render(render.GlyphCursor) + " "
	}

	// Indentation carries the spawn tree. The connector is drawn only for
	// children: a root needs no elbow, and drawing one implies a parent above it.
	indent := ""
	if e.Depth > 0 {
		connector := render.GlyphBranch
		if e.Last {
			connector = render.GlyphLast
		}
		indent = strings.Repeat("  ", e.Depth-1) + d.styles.Dim.Render(connector) + " "
	}

	mark := lipgloss.NewStyle().Foreground(e.Status.Color(t)).Render(e.Status.Mark())

	kind := "shell"
	kindStyle := lipgloss.NewStyle().Foreground(t.Subtext)
	if e.Kind == DockAgent {
		kind = string(e.AgentKind)
		kindStyle = lipgloss.NewStyle().Foreground(e.AgentKind.Color(t))
	}

	elapsed := d.styles.Dim.Render(render.CellRight(e.Elapsed(), 5))

	activity := e.Activity
	if e.HasStderr && !e.Status.Terminal() && activity != "" {
		// A command writing to stderr while still succeeding is worth flagging
		// without crying wolf: one mark, no colour change to the row.
		activity = render.GlyphWarn + " " + activity
	}
	activityStyle := d.styles.Dim
	switch e.Status {
	case render.StatusFailed:
		activityStyle = lipgloss.NewStyle().Foreground(t.Red)
	case render.StatusRunning:
		activityStyle = lipgloss.NewStyle().Foreground(t.Subtext)
	}

	// Column budget, right to left: elapsed is fixed, activity gets a third of
	// what remains, and the label absorbs the rest. Sizing the flexible column
	// last is what keeps a long command from pushing the timings off-screen.
	// When the tray is too narrow for a column it is dropped — activity first,
	// then the timing — because the label is the cell the user actually reads,
	// and a row wider than its line budget wraps inside the border.
	line := cursor + indent + mark + " " + kindStyle.Render(render.Cell(kind, 7))
	rest := inner - render.Width(line)

	elapsedCol := ""
	if rest >= 12 { // the 5-cell column and its gap, with a label stub left over
		elapsedCol = " " + elapsed
		rest -= 6
	}
	activityCol := ""
	if rest >= 12 { // two gaps and a cell wide enough to say something
		activityW := rest / 3
		if activityW > 24 {
			activityW = 24
		}
		activityCol = "  " + activityStyle.Render(render.Cell(activity, activityW))
		rest -= 2 + activityW
	}
	labelW := rest - 1
	if labelW < 1 {
		labelW = 1
	}

	labelStyle := lipgloss.NewStyle().Foreground(t.Text)
	if e.Status.Terminal() {
		labelStyle = lipgloss.NewStyle().Foreground(t.Subtext)
	}

	line += " " + labelStyle.Render(render.Cell(e.Label, labelW)) + activityCol + elapsedCol

	if selected {
		return lipgloss.NewStyle().Bold(true).Render(line)
	}
	return line
}

// preview is the one detail row under the list, describing the cursored entry.
// It is always exactly one row — that constancy is what keeps the tray's height
// independent of its content.
func (d BottomDock) preview(inner int) string {
	e, ok := d.Selected()
	if !ok {
		return ""
	}
	var facts []string
	facts = append(facts, e.ID)
	if e.Kind == DockAgent {
		if e.ToolCount > 0 {
			facts = append(facts, fmt.Sprintf("%d tools", e.ToolCount))
		}
		if e.Children > 0 {
			facts = append(facts, fmt.Sprintf("%d children", e.Children))
		}
	}
	if e.HasStderr {
		facts = append(facts, "stderr")
	}
	facts = append(facts, e.Status.Label())

	body := strings.Join(facts, render.GlyphSep)
	return d.styles.Dim.Render("  " + render.GlyphElbow + " " + render.Clip(body, inner-4))
}

// HasShells reports whether any shell row exists.
func (d BottomDock) HasShells() bool {
	for _, e := range d.entries {
		if e.Kind == DockShell {
			return true
		}
	}
	return false
}
