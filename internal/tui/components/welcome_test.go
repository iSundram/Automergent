package components

import (
	"strings"
	"testing"

	"github.com/iSundram/Automergent/internal/tui/themes"
)

func welcomeForTest(t *testing.T) *themes.Styles {
	t.Helper()
	return themes.NewStyles(themes.Get("catppuccin"))
}

func TestWelcomeViewRendersAllSections(t *testing.T) {
	styles := welcomeForTest(t)
	out := WelcomeView(styles, 100, 0)
	for _, want := range []string{"AUTOMERGENT", "Your AI coding agent", "/new", "@file", "ctrl+c"} {
		if !strings.Contains(out, want) {
			t.Errorf("welcome missing %q", want)
		}
	}
}

func TestWelcomeViewNarrowWidthsDoNotPanic(t *testing.T) {
	styles := welcomeForTest(t)
	for _, w := range []int{1, 5, 12, 20, 28, 40} {
		out := WelcomeView(styles, w, 0)
		if strings.TrimSpace(out) == "" {
			t.Errorf("width %d rendered nothing", w)
		}
	}
}

func TestWelcomeViewBorderTitleOnlyOnPlainBorder(t *testing.T) {
	// A plain dashed top border gets the title embedded (box wide enough
	// for the full header).
	box := "╭────────────────────╮\n│ hi                 │\n╰────────────────────╯"
	titled := withBorderTitle(box, "Get started")
	if !strings.Contains(titled, "Get started") {
		t.Fatalf("title not embedded: %q", titled)
	}
	// Line lengths are preserved — the rewritten border must not be longer
	// or shorter than the original.
	if len([]rune(strings.Split(titled, "\n")[0])) != len([]rune(strings.Split(box, "\n")[0])) {
		t.Fatalf("border length changed:\n%q\n%q", box, titled)
	}

	// A top border that is NOT all dashes (padding, other glyphs) is left
	// untouched rather than corrupted.
	odd := "╭─ hello ───╮\n│ hi         │\n╰────────────╯"
	if got := withBorderTitle(odd, "Get started"); got != odd {
		t.Fatalf("non-dash interior must not be rewritten:\n%q", got)
	}

	// A title wider than the border is truncated, not overflowing.
	narrow := "╭────╮\n│ hi │\n╰────╯"
	got := withBorderTitle(narrow, "An Extremely Long Title")
	if len([]rune(strings.Split(got, "\n")[0])) != len([]rune(strings.Split(narrow, "\n")[0])) {
		t.Fatalf("overflowing title changed border length:\n%q", got)
	}
}

func TestWelcomeViewVerticalCentering(t *testing.T) {
	styles := welcomeForTest(t)
	short := WelcomeView(styles, 80, 0)
	tall := WelcomeView(styles, 80, 40)
	if lines(tall) <= lines(short) {
		t.Fatalf("height=40 must add vertical padding: %d vs %d lines", lines(tall), lines(short))
	}
	// Centering pads equally top and bottom (within a line).
	topPad := leadingBlankLines(tall)
	bottomPad := trailingBlankLines(tall)
	if abs(topPad-bottomPad) > 1 {
		t.Fatalf("padding not centered: top=%d bottom=%d", topPad, bottomPad)
	}
}

func TestWelcomeViewNoVariationSelector(t *testing.T) {
	styles := welcomeForTest(t)
	out := WelcomeView(styles, 80, 0)
	if strings.ContainsRune(out, '︎') {
		t.Error("brand mark must not carry a variation selector (breaks centering on some terminals)")
	}
}

func lines(s string) int { return len(strings.Split(s, "\n")) }

func leadingBlankLines(s string) int {
	n := 0
	for _, line := range strings.Split(s, "\n") {
		if strings.TrimSpace(line) == "" {
			n++
		} else {
			break
		}
	}
	return n
}

func trailingBlankLines(s string) int {
	n := 0
	all := strings.Split(s, "\n")
	for i := len(all) - 1; i >= 0; i-- {
		if strings.TrimSpace(all[i]) == "" {
			n++
		} else {
			break
		}
	}
	return n
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
