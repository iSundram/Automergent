package render

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
)

// styled wraps s in a real SGR pair so tests exercise the escape-aware paths
// rather than the plain-string shortcut.
func styled(s string) string { return "\x1b[38;5;42m" + s + "\x1b[0m" }

// TestClipNeverPanicsNeverSplitsRunes is the regression that motivated this
// file: the helper it replaces sliced by byte, so it emitted broken UTF-8 for
// multi-byte input and panicked outright for limits below three.
func TestClipNeverPanicsNeverSplitsRunes(t *testing.T) {
	inputs := []string{
		"",
		"a",
		"ascii text that is quite long",
		"日本語のテキストです",
		"mixed 日本語 and ascii",
		"café naïve résumé",
		styled("styled and long enough to clip"),
		"● agent explore ⎿ done",
	}
	for _, in := range inputs {
		for w := -3; w <= 40; w++ {
			got := Clip(in, w)
			if !utf8.ValidString(got) {
				t.Fatalf("Clip(%q, %d) produced invalid UTF-8: %q", in, w, got)
			}
			if w > 0 && ansi.StringWidth(got) > w {
				t.Fatalf("Clip(%q, %d) width %d exceeds limit: %q",
					in, w, ansi.StringWidth(got), got)
			}
			if w <= 0 && got != "" {
				t.Fatalf("Clip(%q, %d) = %q, want empty", in, w, got)
			}
		}
	}
}

// TestCellIsExactlyWidth is the property that makes a grid of rows align. The
// old code reached for fmt's %-9s, which counts bytes — and a styled string is
// mostly escape bytes, so it padded by nothing at all.
func TestCellIsExactlyWidth(t *testing.T) {
	inputs := []string{"", "ok", "a much longer value than the column", styled("running"), "日本語"}
	for _, in := range inputs {
		for w := 1; w <= 20; w++ {
			if got := ansi.StringWidth(Cell(in, w)); got != w {
				t.Errorf("Cell(%q, %d) width = %d, want %d", in, w, got, w)
			}
			if got := ansi.StringWidth(CellRight(in, w)); got != w {
				t.Errorf("CellRight(%q, %d) width = %d, want %d", in, w, got, w)
			}
		}
	}
}

func TestCellPreservesStyling(t *testing.T) {
	got := Cell(styled("run"), 10)
	if !strings.Contains(got, "\x1b[38;5;42m") {
		t.Errorf("Cell dropped the SGR prefix: %q", got)
	}
	if !strings.HasSuffix(got, strings.Repeat(" ", 7)) {
		t.Errorf("Cell padded inside the escape rather than after it: %q", got)
	}
}

func TestLastNonEmptyLine(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", ""},
		{"only", "only"},
		{"a\nb\nc", "c"},
		{"a\nb\nc\n", "c"},
		{"a\nb\n\n   \n", "b"},
		{"\n\n\n", ""},
		{"first\r\nsecond\r\n", "second"},
	}
	for _, c := range cases {
		if got := LastNonEmptyLine(c.in); got != c.want {
			t.Errorf("LastNonEmptyLine(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestTailLines(t *testing.T) {
	lines, hidden := TailLines("1\n2\n3\n4\n5", 2)
	if hidden != 3 || strings.Join(lines, ",") != "4,5" {
		t.Errorf("TailLines = %v hidden=%d, want [4 5] hidden=3", lines, hidden)
	}
	if lines, hidden := TailLines("1\n2", 5); hidden != 0 || len(lines) != 2 {
		t.Errorf("TailLines short input = %v hidden=%d", lines, hidden)
	}
	if lines, _ := TailLines("x", 0); lines != nil {
		t.Errorf("TailLines with n=0 = %v, want nil", lines)
	}
}

// TestElapsedIsStableWidth matters because the column ticks once a second: a
// format that changed width would shift every column beside it.
func TestElapsedIsStableWidth(t *testing.T) {
	for s := 0; s < 3600; s += 7 {
		if got := len(Elapsed(s)); got < 4 || got > 5 {
			t.Fatalf("Elapsed(%d) = %q, want 4-5 cells", s, Elapsed(s))
		}
	}
	if got := Elapsed(3725); got != "1:02:05" {
		t.Errorf("Elapsed(3725) = %q, want 1:02:05", got)
	}
	if got := Elapsed(-5); got != "0:00" {
		t.Errorf("Elapsed(-5) = %q, want 0:00", got)
	}
}

func TestRule(t *testing.T) {
	if got := Rule(0); got != "" {
		t.Errorf("Rule(0) = %q, want empty", got)
	}
	if got := ansi.StringWidth(Rule(12)); got != 12 {
		t.Errorf("Rule(12) width = %d, want 12", got)
	}
}
