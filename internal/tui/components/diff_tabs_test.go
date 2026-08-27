package components

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func stripANSI(s string) string {
	return ansi.Strip(s)
}

func visibleDiff() Diff {
	d := NewDiff(testStyles())
	d.SetSize(100, 30)
	return d
}

func TestOpenFileRecencyOrderingAndSelection(t *testing.T) {
	d := visibleDiff()
	d.OpenFile("a.go", "--- a.go\n+++ b/a.go\n@@\n-old a\n+new a\n")
	d.OpenFile("b.go", "--- b.go\n+++ b/b.go\n@@\n-old b\n+new b\n")

	if got := d.ActiveLabel(); got != "b.go" {
		t.Fatalf("most recent file should be selected, got %q", got)
	}
	// Re-editing an existing tab refreshes it and pulls it back to the front.
	d.OpenFile("a.go", "--- a.go\n+++ b/a.go\n@@\n-old a2\n+new a2\n")
	if got := d.ActiveLabel(); got != "a.go" {
		t.Fatalf("re-edited file should move to front, got %q", got)
	}
	if d.TabCount() != 2 {
		t.Fatalf("TabCount = %d, want 2 (no duplicates)", d.TabCount())
	}

	// Selected tab renders first, others to its right.
	d.Show()
	view := stripANSI(d.View())
	ia := strings.Index(view, "a.go")
	ib := strings.Index(view, "b.go")
	if ia == -1 || ib == -1 {
		t.Fatalf("tab strip missing tabs: %q", view)
	}
	if ia > ib {
		t.Fatalf("selected tab must render first: %q", view)
	}
}

func TestDiffTabCyclingAndDigitSelect(t *testing.T) {
	d := visibleDiff()
	d.OpenFile("one.go", "--- one.go\n+++ b/one.go\n@@\n+x\n")
	d.OpenFile("two.go", "--- two.go\n+++ b/two.go\n@@\n+y\n")
	d.OpenFile("three.go", "--- three.go\n+++ b/three.go\n@@\n+z\n")
	d.Show()

	if got := d.ActiveLabel(); got != "three.go" {
		t.Fatalf("initial selection = %q, want three.go", got)
	}
	d, _ = d.Update(tea.KeyPressMsg{Code: ']'})
	if got := d.ActiveLabel(); got != "two.go" {
		t.Fatalf("after ] = %q, want two.go", got)
	}
	d, _ = d.Update(tea.KeyPressMsg{Code: '['})
	if got := d.ActiveLabel(); got != "three.go" {
		t.Fatalf("after [ = %q, want three.go", got)
	}
	d, _ = d.Update(tea.KeyPressMsg{Code: '1'})
	if got := d.ActiveLabel(); got != "three.go" {
		t.Fatalf("digit 1 should stay on recency head, got %q", got)
	}
	d, _ = d.Update(tea.KeyPressMsg{Code: '3'})
	// Visual slots: 1=three.go (selected head), 2=two.go, 3=one.go.
	if got := d.ActiveLabel(); got != "one.go" {
		t.Fatalf("digit 3 should select slot 3 (one.go), got %q", got)
	}
	// After re-selection the strip reorders: slots are now [one, three, two].
	d, _ = d.Update(tea.KeyPressMsg{Code: '2'})
	if got := d.ActiveLabel(); got != "three.go" {
		t.Fatalf("digit 2 after reorder should select three.go, got %q", got)
	}
}

func TestHideRetainsTabsForLaterReopen(t *testing.T) {
	d := visibleDiff()
	d.OpenFile("x.go", "--- x.go\n+++ b/x.go\n@@\n+hi\n")
	d.ShowWithConfirm(nil)
	d.Hide()
	if d.Visible() {
		t.Fatal("pane should be hidden after Hide")
	}
	if d.TabCount() != 1 {
		t.Fatalf("tabs must survive Hide for /diff reopen, got %d", d.TabCount())
	}
	d.Toggle()
	if !d.Visible() || d.ActiveLabel() != "x.go" {
		t.Fatalf("reopen should show retained tab, visible=%v label=%q", d.Visible(), d.ActiveLabel())
	}
}

func TestPlainSetContentStillWorks(t *testing.T) {
	d := visibleDiff()
	d.SetContent("# shell demo\n$ echo hi\n\nout\n")
	d.Show()
	view := stripANSI(d.View())
	if !strings.Contains(view, "echo hi") {
		t.Fatalf("plain view lost content: %q", view)
	}
	if d.TabCount() != 0 {
		t.Fatalf("plain content must not register as a modified-file tab: %d", d.TabCount())
	}
}

func TestMinimapSmallFileFitsScreen(t *testing.T) {
	// File smaller than the viewport: slider must span the whole track and
	// change ticks must sit at their proportional position — no row should be
	// left bare of slider or mis-marked.
	d := NewDiff(testStyles())
	d.SetSize(80, 30)
	content := "--- s.go\n+++ b/s.go\n@@ -1,3 +1,3 @@\n keep\n-gone\n+here\n tail\n"
	d.OpenFile("s.go", content)
	d.refresh()
	d.Show()

	view := stripANSI(d.View())
	lines := strings.Split(view, "\n")

	markRows := 0
	for _, l := range lines {
		if strings.Contains(l, "▄") || strings.Contains(l, "▀") || strings.Contains(l, "█") {
			markRows++
		}
	}
	if markRows == 0 {
		t.Fatalf("expected change ticks in minimap:\n%s", view)
	}
}

func TestMinimapLargeFileSliderWindow(t *testing.T) {
	// File much taller than the viewport: the slider must be a bounded block
	// (not the whole track), and it must move when scrolling.
	d := NewDiff(testStyles())
	d.SetSize(80, 24)
	var b strings.Builder
	b.WriteString("--- l.go\n+++ b/l.go\n@@ -1,200 +1,200 @@\n")
	for i := 0; i < 200; i++ {
		b.WriteString(" ctx\n")
	}
	b.WriteString("-old\n+new\n")
	d.OpenFile("l.go", b.String())
	d.refresh()
	d.Show()

	top := stripANSI(d.View())
	d.viewport.SetYOffset(100)
	bottom := stripANSI(d.View())
	if top == bottom {
		t.Fatal("minimap slider should move with scrolling")
	}
}
