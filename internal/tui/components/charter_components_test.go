package components

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/iSundram/Automergent/internal/tui/render"
)

// lineCount counts rendered rows of a view string.
func lineCount(s string) int {
	s = strings.TrimRight(s, "\n")
	if s == "" {
		return 0
	}
	return strings.Count(s, "\n") + 1
}

// TestDockHeightMatchesView is the height-invariance contract: what Height()
// reports must be exactly what View() draws. Layout() subtracts Height() from
// the conversation's budget, so any drift either steals transcript rows or
// overflows the screen — and the old dock drifted, because layout measured the
// rendered string while Height() computed something else entirely.
func TestDockHeightMatchesView(t *testing.T) {
	d := NewBottomDock(testStyles())
	d.SetWidth(80)
	d.SetFocused(true)

	cases := [][]DockEntry{
		{},
		{{Kind: DockShell, ID: "s1", Label: "x", Status: render.StatusRunning}},
		{
			{Kind: DockShell, ID: "s1", Label: "x", Status: render.StatusRunning},
			{Kind: DockAgent, ID: "a1", Label: "y", Status: render.StatusDone, Depth: 1},
		},
	}
	// Enough entries to force the overflow row.
	for i := 0; i < trayMaxRows+4; i++ {
		cases = append(cases, nil)
		cases[len(cases)-1] = append(cases[len(cases)-1], DockEntry{
			Kind: DockShell, ID: "s", Label: strings.Repeat("cmd", i+1),
			Status: render.StatusRunning,
		})
	}

	for i, entries := range cases {
		d.SetEntries(entries)
		if got, want := d.Height(), lineCount(d.View()); got != want {
			t.Errorf("case %d: Height() = %d, View() renders %d rows", i, got, want)
		}
	}
}

// TestDockHeightIgnoresContent is the invariant the rail/tray split exists for:
// the tray's height is a function of the row count only. The old dock's height
// rose when a preview string became non-empty, which reflowed the conversation
// under the user once a second while any background work ran.
func TestDockHeightIgnoresContent(t *testing.T) {
	d := NewBottomDock(testStyles())
	d.SetWidth(80)
	d.SetFocused(true)

	d.SetEntries([]DockEntry{
		{Kind: DockShell, ID: "s1", Label: "a", Status: render.StatusRunning, Activity: ""},
		{Kind: DockShell, ID: "s2", Label: "b", Status: render.StatusDone, Activity: ""},
	})
	h := d.Height()

	d.SetEntries([]DockEntry{
		{Kind: DockShell, ID: "s1", Label: strings.Repeat("very long command ", 20),
			Status: render.StatusFailed, Activity: strings.Repeat("stderr noise ", 30)},
		{Kind: DockShell, ID: "s2", Label: strings.Repeat("another long one ", 20),
			Status: render.StatusRunning, Activity: strings.Repeat("chatty output ", 30)},
	})
	if d.Height() != h {
		t.Errorf("height changed with content: %d -> %d (must be count-only)", h, d.Height())
	}
}

// TestDockRowsFitWidth: no rendered tray row may exceed the dock's width. A
// row that overflows wraps in the terminal and every assertion about height
// becomes a lie.
func TestDockRowsFitWidth(t *testing.T) {
	for _, w := range []int{20, 40, 60, 80, 120, 200} {
		d := NewBottomDock(testStyles())
		d.SetWidth(w)
		d.SetFocused(true)
		d.SetEntries([]DockEntry{
			{Kind: DockShell, ID: "s1", Label: strings.Repeat("npm run build ", 12),
				Status: render.StatusRunning, Activity: strings.Repeat("compiling ", 12)},
			{Kind: DockAgent, ID: "a1", Label: strings.Repeat("coordinator ", 8),
				Status: render.StatusFailed, Activity: strings.Repeat("child failed ", 8), Depth: 2},
		})
		for _, line := range strings.Split(d.View(), "\n") {
			if got := ansi.StringWidth(line); got > w {
				t.Errorf("width %d: row measures %d cells:\n%s", w, got, line)
			}
		}
	}
}

// TestQueueStripHeightMatchesView: same contract as the dock. The strip sits
// between the prompt and the dock, so its height lie would displace both.
func TestQueueStripHeightMatchesView(t *testing.T) {
	q := NewQueueStrip(testStyles())
	q.SetWidth(80)

	q.SetItems(nil)
	if q.Height() != 0 || q.View() != "" {
		t.Errorf("empty queue must render nothing, got height %d", q.Height())
	}

	for n := 1; n <= queueMaxRows+4; n++ {
		items := make([]QueueItem, n)
		for i := range items {
			items[i] = QueueItem{Text: strings.Repeat("message ", i+1), Boundary: i%2 == 0}
		}
		q.SetItems(items)
		if got, want := q.Height(), lineCount(q.View()); got != want {
			t.Errorf("%d items: Height() = %d, View() renders %d rows", n, got, want)
		}
	}
}

// TestQueueStripRowsFitWidth across narrow and wide terminals.
func TestQueueStripRowsFitWidth(t *testing.T) {
	for _, w := range []int{20, 40, 80, 120} {
		q := NewQueueStrip(testStyles())
		q.SetWidth(w)
		q.SetItems([]QueueItem{
			{Text: strings.Repeat("fix the failing tests ", 10), Boundary: true},
			{Text: "/run go test ./...", IsCmd: true},
			{Text: strings.Repeat("then deploy ", 10)},
		})
		for _, line := range strings.Split(q.View(), "\n") {
			if got := ansi.StringWidth(line); got > w {
				t.Errorf("width %d: row measures %d cells:\n%s", w, got, line)
			}
		}
	}
}

// TestTaskBoardHeightMatchesView for the board's todo-only layout.
func TestTaskBoardHeightMatchesView(t *testing.T) {
	b := NewTaskBoard(testStyles())
	b.SetSize(40, 24)
	b.Toggle()
	if b.View() == "" {
		t.Fatal("visible board must render")
	}
	// The board sizes its pane from SetSize rather than a Height() method, so
	// the contract here is narrower: the view must be non-empty and every line
	// within the width budget.
	for _, line := range strings.Split(b.View(), "\n") {
		if got := ansi.StringWidth(line); got > 38 {
			t.Errorf("board row measures %d cells (budget 38):\n%s", got, line)
		}
	}
}
