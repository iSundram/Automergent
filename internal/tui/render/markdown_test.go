package render

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/iSundram/Automergent/internal/tui/themes"
)

// stripANSI drops styling so assertions are about structure, not color.
func stripANSI(s string) string { return ansi.Strip(s) }

func TestMarkdownWithWidthUsesCleanTerminalFormatting(t *testing.T) {
	rendered := MarkdownWithWidth("### Heading\n\n---\n\nUse `sync` safely.", 40)
	plain := ansi.Strip(rendered)
	if strings.Contains(plain, "###") {
		t.Fatalf("rendered heading exposes markdown markers: %q", plain)
	}
	if strings.Contains(plain, "--------") {
		t.Fatalf("rendered rule exposes ASCII separator: %q", plain)
	}
	if strings.Contains(plain, " ") {
		t.Fatalf("inline code contains non-breaking spaces: %q", plain)
	}
}

// TestMarkdownFollowsTheme is the point of the themed style config: prose used
// to be colored by glamour's own fixed palette, so /theme changed the chrome
// and left every assistant answer looking the same.
func TestMarkdownFollowsTheme(t *testing.T) {
	SetTheme(themes.Get("catppuccin"))
	catppuccin := MarkdownWithWidth("# Heading", 40)

	SetTheme(themes.Get("dracula"))
	dracula := MarkdownWithWidth("# Heading", 40)

	if catppuccin == dracula {
		t.Errorf("heading rendered identically under two themes:\n%q", catppuccin)
	}
	if stripANSI(catppuccin) != stripANSI(dracula) {
		t.Errorf("themes changed the text, not just the color:\n%q\n%q",
			stripANSI(catppuccin), stripANSI(dracula))
	}
}

// TestThemeSwitchInvalidatesCache guards the old "dark|width" cache key: two
// dark themes shared an entry, so /theme did nothing until a resize.
func TestThemeSwitchInvalidatesCache(t *testing.T) {
	SetTheme(themes.Get("dracula"))
	first := MarkdownWithWidth("# Heading", 41)
	SetTheme(themes.Get("nord")) // also dark
	second := MarkdownWithWidth("# Heading", 41)
	if first == second {
		t.Errorf("two dark themes shared a cached renderer: %q", first)
	}
}

func TestMarkdownGenerationAdvances(t *testing.T) {
	before := MarkdownGeneration()
	SetTheme(themes.Get("gruvbox"))
	if after := MarkdownGeneration(); after == before {
		t.Errorf("generation did not advance across SetTheme (%d)", after)
	}
}

func TestHeadingPrefixesAreStripped(t *testing.T) {
	SetTheme(themes.Get("catppuccin"))
	for _, src := range []string{"# One", "## Two", "###### Six"} {
		got := stripANSI(MarkdownWithWidth(src, 40))
		if strings.Contains(got, "#") {
			t.Errorf("%q kept its hashes: %q", src, got)
		}
	}
}

// TestFencedCodeUsesActiveSyntaxStyle: fenced code in prose and code in a tool
// result are the same language in the same session, so they must not be
// highlighted by two different palettes.
func TestFencedCodeUsesActiveSyntaxStyle(t *testing.T) {
	SetTheme(themes.Get("dracula"))
	fenced := MarkdownWithWidth("```go\nvar x = 1\n```", 40)
	direct := strings.TrimRight(Code("var x = 1", "go"), "\n")
	if !strings.Contains(fenced, direct) {
		t.Errorf("fenced code was not highlighted like tool output:\nfenced=%q\ndirect=%q",
			fenced, direct)
	}
}

func TestMarkdownNeverExceedsWidth(t *testing.T) {
	SetTheme(themes.Get("catppuccin"))
	src := "This is a reasonably long paragraph that certainly has to wrap " +
		"more than once at a narrow width, plus a `code span` and a **bold run**."
	for _, w := range []int{20, 30, 40, 60, 80} {
		for _, line := range strings.Split(MarkdownWithWidth(src, w), "\n") {
			if got := ansi.StringWidth(line); got > w {
				t.Errorf("width %d: line is %d cells: %q", w, got, stripANSI(line))
			}
		}
	}
}

func TestTrimTrailingBlankKeepsLeadingIndent(t *testing.T) {
	got := trimTrailingBlank("\n\n    indented\nnext\n\n")
	want := "    indented\nnext"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRendererCacheIsBounded(t *testing.T) {
	SetTheme(themes.Get("catppuccin"))
	for w := 20; w < 20+maxWidthRenderers*3; w++ {
		MarkdownWithWidth("hello", w)
	}
	rendererMu.Lock()
	n := len(widthRenderers)
	rendererMu.Unlock()
	if n > maxWidthRenderers {
		t.Errorf("cache grew to %d entries, cap is %d", n, maxWidthRenderers)
	}
}
