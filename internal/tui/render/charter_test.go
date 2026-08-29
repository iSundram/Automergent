package render

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// pendingSweep lists files that still contain non-charter glyphs. Every entry
// is a debt, not an exemption: when the file is swept, delete its line here and
// the charter starts enforcing it. Files land here only while someone else is
// actively editing them (a parallel workstream owns them right now).
var pendingSweep = []string{
	"tui/commands/registry.go",
	"tui/commands/custom.go",
	"tui/commands/registry_test.go",
	"tui/components/palette.go",
}

// stringLiterals extracts the contents of every string literal in a line of Go
// source, skipping line comments. Comments may legitimately spell out banned
// codepoint names ("U+2699 GEAR") — it is what the UI *draws* that the charter
// polices.
func stringLiterals(line string) []string {
	if i := strings.Index(line, "//"); i >= 0 {
		line = line[:i]
	}
	var out []string
	for i := 0; i < len(line); i++ {
		switch line[i] {
		case '"':
			j := i + 1
			for j < len(line) && line[j] != '"' {
				if line[j] == '\\' {
					j++
				}
				j++
			}
			if j < len(line) {
				if s, err := strconv.Unquote(line[i : j+1]); err == nil {
					out = append(out, s)
				}
				i = j
			}
		case '`':
			j := strings.Index(line[i+1:], "`")
			if j >= 0 {
				out = append(out, line[i+1:i+1+j])
				i = i + 1 + j
			}
		}
	}
	return out
}

// TestGlyphCharter walks the TUI-adjacent source and fails on any non-ASCII
// rune in a string literal that the charter does not admit.
//
// Test files are exempt: their fixtures deliberately include CJK text, ANSI
// noise and other hostile input — that is what the width tests are for. It is
// what the UI *draws* that the charter polices.
//
// This is the enforcement arm of the glyph charter (see glyphs.go). Without it
// the charter is a comment; with it, a nerd-font glyph pasted into a component
// fails the build instead of shipping as tofu boxes on every unpatched
// terminal.
func TestGlyphCharter(t *testing.T) {
	charter := Charter()
	roots := []string{"../../tui", "../../installer"}
	var pending []string
	for _, root := range roots {
		err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") ||
				strings.HasSuffix(path, "_test.go") {
				return err
			}
			rel, _ := filepath.Rel("../..", path)
			for _, p := range pendingSweep {
				if strings.HasSuffix(filepath.ToSlash(rel), p) {
					pending = append(pending, rel)
					return nil
				}
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			for lineNo, line := range strings.Split(string(data), "\n") {
				for _, lit := range stringLiterals(line) {
					for _, r := range lit {
						if r < 0x80 {
							continue
						}
						if !charter[r] {
							t.Errorf("%s:%d: rune %q (U+%04X) is outside the glyph charter",
								rel, lineNo+1, r, r)
						}
					}
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", root, err)
		}
	}
	if len(pending) > 0 {
		t.Logf("pending sweep (not enforced): %s", strings.Join(pending, ", "))
	}
}

// TestClipNeverExceedsWidth fuzzes Clip and the Cell builders across widths
// 0..40 with inputs mixing ASCII, charter glyphs, combining text and embedded
// ANSI styling. A renderer that ever emits a line wider than its budget breaks
// column alignment for everything after it, so this is the one invariant worth
// fuzzing.
func TestClipNeverExceedsWidth(t *testing.T) {
	inputs := []string{
		"",
		"plain ascii",
		strings.Repeat("x", 100),
		"run ● idle ◌ ok ✓ fail ✗ warn ▲",
		strings.Repeat("●◌✓✗○▸", 20),
		"mixed ● and ascii with a very long tail of text trailing on",
		"\x1b[31mred\x1b[0m and plain",
		"\x1b[1;32mbold green\x1b[0m\x1b[38;5;196mmore\x1b[0m",
		"tab\tand\nnewline",
		"émoji-free accénts",
		"日本語のテキスト",
	}
	for _, in := range inputs {
		for w := 0; w <= 40; w++ {
			if got := Width(Clip(in, w)); got > w {
				t.Errorf("Clip(%q, %d) width = %d", in, w, got)
			}
			// Pad and PadLeft pad-to-width without truncating: input narrower
			// than the budget lands exactly on it; wider input passes through
			// unchanged (Cell is the primitive that never overflows).
			if w > 0 {
				if got := Width(Pad(in, w)); got < w {
					t.Errorf("Pad(%q, %d) width = %d, under-padded", in, w, got)
				}
				if got := Width(PadLeft(in, w)); got < w {
					t.Errorf("PadLeft(%q, %d) width = %d, under-padded", in, w, got)
				}
				if got := Width(Cell(in, w)); got != w {
					t.Errorf("Cell(%q, %d) width = %d, want exactly %d", in, w, got, w)
				}
				if got := Width(CellRight(in, w)); got != w {
					t.Errorf("CellRight(%q, %d) width = %d, want exactly %d", in, w, got, w)
				}
			}
		}
	}
}

// TestElapsedStableWidth pins the dock's timing column. Elapsed is fixed width
// per magnitude, not globally: 0:12 and 4:08 match, 59:59 and 1:00:00 must not.
// What must never happen is a shift within a magnitude, or a shrink as time
// grows — either would make the right-aligned column breathe once a second.
func TestElapsedStableWidth(t *testing.T) {
	prev := 0
	for sec := 0; sec < 86400*3; sec++ {
		if sec > 200 && sec%977 != 0 { // sample the tail densely but cheaply
			continue
		}
		w := Width(Elapsed(sec))
		if w < prev {
			t.Fatalf("Elapsed(%d) width shrank to %d from %d", sec, w, prev)
		}
		prev = w
	}
	// Within one magnitude the width is constant.
	for _, group := range [][2]int{{0, 9}, {10, 59}, {60, 599}, {600, 3599}, {3600, 35999}} {
		want := -1
		for sec := group[0]; sec <= group[1]; sec++ {
			if w := Width(Elapsed(sec)); want == -1 {
				want = w
			} else if w != want {
				t.Fatalf("Elapsed(%d) width = %d, want %d (group %v)", sec, w, want, group)
			}
		}
	}
}
