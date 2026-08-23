package components

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/iSundram/Automergent/internal/tui/themes"
)

// DockEntryKind discriminates bottom-dock rows.
type DockEntryKind string

const (
	DockShell DockEntryKind = "shell"
	DockAgent DockEntryKind = "agent"
)

// DockEntry is one background resource row (shell session or subagent).
type DockEntry struct {
	Kind    DockEntryKind
	ID      string
	Label   string
	Status  string
	Detail  string // elapsed / exit code / turns
	Created time.Time
}

// BottomDock is the tmux-style tray under the input listing background
// terminals and subagents. Focus flows here with ↓ from the input; the
// prompt marker moves out of the input while the dock owns the keyboard.
type BottomDock struct {
	styles  *themes.Styles
	entries []DockEntry
	cursor  int
	focused bool
	width   int
}

// NewBottomDock creates an unfocused, empty dock.
func NewBottomDock(styles *themes.Styles) *BottomDock {
	return &BottomDock{styles: styles}
}

// SetWidth updates the available width.
func (d *BottomDock) SetWidth(w int) { d.width = w }

// SetEntries replaces the rows and clamps the cursor.
func (d *BottomDock) SetEntries(entries []DockEntry) {
	d.entries = entries
	if d.cursor >= len(d.entries) {
		d.cursor = len(d.entries) - 1
	}
	if d.cursor < 0 {
		d.cursor = 0
	}
}

// Entries returns the current rows.
func (d BottomDock) Entries() []DockEntry { return d.entries }

// HasContent reports whether any row exists.
func (d BottomDock) HasContent() bool { return len(d.entries) > 0 }

// Focused reports whether the dock owns the keyboard.
func (d BottomDock) Focused() bool { return d.focused }

// SetFocused moves keyboard ownership between input and dock.
func (d *BottomDock) SetFocused(v bool) {
	d.focused = v
	if !v && d.cursor >= len(d.entries) {
		d.cursor = len(d.entries) - 1
	}
}

// MoveCursor moves the highlight within bounds.
func (d *BottomDock) MoveCursor(delta int) {
	if len(d.entries) == 0 {
		return
	}
	d.cursor += delta
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

const dockMaxRows = 5

// Height returns rendered height in lines (0 when nothing to show).
func (d BottomDock) Height() int {
	if len(d.entries) == 0 {
		return 0
	}
	rows := len(d.entries)
	if rows > dockMaxRows {
		rows = dockMaxRows
	}
	return rows + 2 // section header + top border allowance
}

// View renders the dock; empty string hides it entirely.
func (d BottomDock) View() string {
	if len(d.entries) == 0 || d.width <= 0 {
		return ""
	}

	title := lipgloss.NewStyle().Foreground(d.styles.T.Accent).Bold(true)
	muted := lipgloss.NewStyle().Foreground(d.styles.T.Muted)
	runStyle := lipgloss.NewStyle().Foreground(d.styles.T.Yellow)
	okStyle := lipgloss.NewStyle().Foreground(d.styles.T.Green)
	errStyle := lipgloss.NewStyle().Foreground(d.styles.T.Red)

	var b strings.Builder
	b.WriteString(title.Render("󰆍 BACKGROUND") + muted.Render("  ↑ back · enter inspect") + "\n")

	start := 0
	if overflow := len(d.entries) - dockMaxRows; overflow > 0 {
		start = overflow
		if d.cursor < start {
			start = d.cursor
		}
	}
	for i := start; i < len(d.entries); i++ {
		e := d.entries[i]
		cursor := "  "
		style := lipgloss.NewStyle()
		if d.focused && i == d.cursor {
			cursor = "▸ "
			style = lipgloss.NewStyle().Bold(true)
		}
		status := e.Status
		switch e.Status {
		case "running":
			status = runStyle.Render(status)
		case "completed", "done":
			status = okStyle.Render(status)
		case "failed", "cancelled", "killed":
			status = errStyle.Render(status)
		default:
			status = muted.Render(status)
		}
		line := fmt.Sprintf("%s%s %-9s %s",
			cursor,
			truncate(e.Label, max(10, d.width/2-8)),
			status,
			muted.Render(e.Detail),
		)
		b.WriteString(style.Render(line) + "\n")
	}

	return lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), true, false, false, false).
		BorderForeground(d.styles.T.BorderNormal).
		Padding(0, 2).
		Width(d.width).
		Render(strings.TrimRight(b.String(), "\n"))
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// SelectedKind helpers keep call sites readable.
func (d BottomDock) HasShells() bool {
	for _, e := range d.entries {
		if e.Kind == DockShell {
			return true
		}
	}
	return false
}
