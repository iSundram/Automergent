package render

// The tests that matter here are the drift tests.
//
// Two renderers draw assistant prose: the inline styler draws the partial line
// still being streamed, and glamour redraws that same line the instant its
// newline arrives. Anything they disagree about — a color, a glyph, a column of
// indent — is visible as the text changing under the user's eyes on a tick
// boundary. So the assertions below compare the two outputs directly rather
// than checking either one in isolation.

import (
	"image/color"
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/iSundram/Automergent/internal/tui/themes"
)

// colorMap renders a string as one line per visible character paired with the
// foreground color active at that character.
//
// A set-of-colors comparison is not enough: two renderers can use the same
// palette and still put the wrong color on the wrong word. This walks the SGR
// stream so "which color is this character" is answered exactly, and it drops
// trailing whitespace because glamour pads every line out to the wrap width with
// styled spaces that carry no meaning.
func colorMap(s string) string {
	var out strings.Builder
	for _, line := range strings.Split(s, "\n") {
		fg := "-"
		var chars strings.Builder
		var colors []string
		runes := []rune(line)
		for i := 0; i < len(runes); {
			if runes[i] == 0x1b {
				i = skipEscape(runes, i, &fg)
				continue
			}
			chars.WriteRune(runes[i])
			colors = append(colors, fg)
			i++
		}
		// Drop trailing whitespace and the colors that went with it.
		trimmed := []rune(strings.TrimRight(chars.String(), " "))
		for i, r := range trimmed {
			out.WriteString(string(r) + "=" + colors[i] + " ")
		}
		out.WriteString("\n")
	}
	return strings.TrimRight(out.String(), "\n ")
}

// skipEscape consumes one escape sequence starting at i and returns the index
// after it, updating fg when the sequence is an SGR that sets a foreground.
//
// It has to handle OSC as well as CSI: glamour wraps links in OSC 8 hyperlinks,
// and treating those as CSI leaves the URL in the visible text.
func skipEscape(runes []rune, i int, fg *string) int {
	if i+1 >= len(runes) {
		return i + 1
	}
	switch runes[i+1] {
	case '[': // CSI
		j := i + 2
		for j < len(runes) && !isSGRFinal(runes[j]) {
			j++
		}
		if j < len(runes) && runes[j] == 'm' {
			*fg = foregroundOf(string(runes[i+2:j]), *fg)
		}
		return j + 1
	case ']': // OSC, terminated by BEL or ST
		j := i + 2
		for j < len(runes) {
			if runes[j] == 0x07 {
				return j + 1
			}
			if runes[j] == 0x1b && j+1 < len(runes) && runes[j+1] == '\\' {
				return j + 2
			}
			j++
		}
		return j
	}
	return i + 2
}

func isSGRFinal(r rune) bool {
	return r >= 0x40 && r <= 0x7e
}

// foregroundOf extracts the foreground color from an SGR parameter list,
// returning prev when the sequence does not set one.
func foregroundOf(params, prev string) string {
	fields := strings.Split(params, ";")
	for i := 0; i < len(fields); i++ {
		switch fields[i] {
		case "0", "":
			prev = "-"
		case "39":
			prev = "-"
		case "38":
			// 38;2;r;g;b or 38;5;n
			if i+1 < len(fields) && fields[i+1] == "2" && i+4 < len(fields) {
				prev = strings.Join(fields[i+1:i+5], ";")
				i += 4
			} else if i+2 < len(fields) {
				prev = strings.Join(fields[i+1:i+3], ";")
				i += 2
			}
		default:
			if len(fields[i]) == 2 && (fields[i][0] == '3' || fields[i][0] == '9') {
				prev = fields[i]
			}
		}
	}
	return prev
}

// inlineLines is the set of single-line constructs both renderers must agree on.
// Each is a whole line, because that is the unit that graduates from the inline
// tier to the block tier.
var inlineLines = []string{
	"plain prose with nothing special",
	"a **bold run** in prose",
	"an *italic run* in prose",
	"a `code span` in prose",
	"a ~~struck run~~ in prose",
	"# Heading one",
	"### Heading three",
	"- a bullet item",
	"1. an ordered item",
	"- [ ] an unticked task",
	"- [x] a ticked task",
	"> a quoted line",
	"see https://example.com/docs for more",
	"a [labelled link](https://example.com) in prose",
}

// TestInlineAndGlamourAgreeOnText is the structural half: the live line and the
// finalized line must contain the same characters, so nothing appears, vanishes,
// or shifts column when a newline lands.
func TestInlineAndGlamourAgreeOnText(t *testing.T) {
	SetTheme(themes.Get("catppuccin"))
	const width = 60

	for _, line := range inlineLines {
		live := trimLines(ansi.Strip(InlineBlock(line)))
		final := trimLines(ansi.Strip(MarkdownWithWidth(line, width)))
		if live != final {
			t.Errorf("live and finalized text differ for %q:\n live  %q\n final %q",
				line, live, final)
		}
	}
}

// trimLines drops the trailing padding glamour writes to fill each line out to
// the wrap width.
func trimLines(s string) string {
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " ")
	}
	return strings.Join(lines, "\n")
}

// TestInlineAndGlamourAgreeOnColor is the palette half — the defect this file
// exists for. Both renderers now read MarkdownColors; before that each derived
// its own hues from the theme, so a code span was one green live and another
// after finalize.
func TestInlineAndGlamourAgreeOnColor(t *testing.T) {
	SetTheme(themes.Get("catppuccin"))
	const width = 60

	for _, line := range inlineLines {
		live := colorMap(InlineBlock(line))
		final := colorMap(MarkdownWithWidth(line, width))
		if live != final {
			t.Errorf("live and finalized colors differ for %q:\n live  %s\n final %s",
				line, live, final)
		}
	}
}

// TestCodeSpanIsNotGreen: green is taken. The diff renderer uses it for
// additions and chroma uses it for string literals inside the fenced block that
// often sits directly below a code span, so a green span read as "added".
func TestCodeSpanIsNotGreen(t *testing.T) {
	for _, name := range []string{"catppuccin", "dracula", "modern", "nord"} {
		theme := themes.Get(name)
		SetTheme(theme)
		p := MarkdownColors()
		if hexOf(p.Code) == hexOf(theme.Green) {
			t.Errorf("%s: inline code is the theme's green (%s)", name, hexOf(p.Code))
		}
		if hexOf(p.Code) != hexOf(theme.Cyan) {
			t.Errorf("%s: inline code is %s, want the blue-side cyan %s",
				name, hexOf(p.Code), hexOf(theme.Cyan))
		}
	}
}

// TestQuoteTokenIsOneCell guards the glamour constraint that forced the glyph
// choice: IndentWriter emits the token once per indent unit but reserves one
// column per unit, so a two-cell token overflows every wrapped quote line.
func TestQuoteTokenIsOneCell(t *testing.T) {
	SetTheme(themes.Get("catppuccin"))
	if got := ansi.StringWidth(QuoteToken(MarkdownColors().QuoteBar)); got != 1 {
		t.Errorf("quote token is %d cells wide, glamour budgets 1", got)
	}
}

// TestBlockConstructsNeverExceedWidth extends the paragraph-only width check to
// the constructs that carry their own indent, which is where the off-by-one
// hides.
func TestBlockConstructsNeverExceedWidth(t *testing.T) {
	SetTheme(themes.Get("catppuccin"))
	long := "a reasonably long run of words that has to wrap at least twice at a narrow width"
	srcs := []string{
		"> " + long,
		"- " + long,
		"1. " + long,
		"- [ ] " + long,
		"> - " + long,
	}
	for _, src := range srcs {
		for _, w := range []int{20, 30, 48, 72} {
			for _, line := range strings.Split(MarkdownWithWidth(src, w), "\n") {
				if got := ansi.StringWidth(line); got > w {
					t.Errorf("width %d, %q: line is %d cells: %q",
						w, src, got, ansi.Strip(line))
				}
			}
		}
	}
}

// TestInlineStylesUnterminatedMarkers is the reason the inline tier exists: the
// closing marker has not streamed yet, and the text must already be styled.
func TestInlineStylesUnterminatedMarkers(t *testing.T) {
	SetTheme(themes.Get("catppuccin"))
	for _, src := range []string{"a **bold run", "a *slanted", "a `spanned", "a ~~struck"} {
		got := Inline(src)
		if got == src {
			t.Errorf("%q was not styled at all", src)
		}
		if plain := ansi.Strip(got); strings.ContainsAny(plain, "*~") {
			t.Errorf("%q leaked its markers: %q", src, plain)
		}
	}
}

// TestInlineKeepsSnakeCase: a lone underscore mid-identifier is not emphasis,
// and a stream of Go code in prose is full of them.
func TestInlineKeepsSnakeCase(t *testing.T) {
	SetTheme(themes.Get("catppuccin"))
	if got := ansi.Strip(Inline("call read_file_at now")); got != "call read_file_at now" {
		t.Errorf("snake_case was eaten as emphasis: %q", got)
	}
}

// TestAutolinkStyled: glamour linkifies bare URLs (GFM is on), so the inline
// tier has to as well or every URL changes color on finalize.
func TestAutolinkStyled(t *testing.T) {
	SetTheme(themes.Get("catppuccin"))
	styled := Inline("see https://example.com/a_b now")
	if ansi.Strip(styled) != "see https://example.com/a_b now" {
		t.Fatalf("autolink text was altered: %q", ansi.Strip(styled))
	}
	if !strings.Contains(colorMap(styled), hexToSGR(MarkdownColors().LinkURL)) {
		t.Errorf("autolink was not painted in the link color: %s", colorMap(styled))
	}
	// Trailing punctuation belongs to the sentence, not the address.
	if got := ansi.Strip(Inline("go to https://example.com.")); got != "go to https://example.com." {
		t.Errorf("trailing period was mangled: %q", got)
	}
}

// hexToSGR renders a palette color the way colorMap reports it.
func hexToSGR(c color.Color) string {
	r, g, b, _ := c.RGBA()
	return "2;" + itoa(int(r>>8)) + ";" + itoa(int(g>>8)) + ";" + itoa(int(b>>8))
}

func itoa(n int) string { return strconv.Itoa(n) }

// streamDoc exercises every block kind the streamer has a branch for.
const streamDoc = "# Title\n\nA paragraph with `code` and **bold**.\n\n" +
	"- first item\n- second item\n\n" +
	"```go\nvar x = 1\n```\n\n" +
	"> quoted\n\nDone.\n"

// TestStreamerChunkingIsIrrelevant: the delta boundaries a provider happens to
// pick must not change a single byte of output. One rune at a time is the worst
// case and the one that exercises every partial-line path.
func TestStreamerChunkingIsIrrelevant(t *testing.T) {
	SetTheme(themes.Get("catppuccin"))
	const width = 58

	drain := func(chunks []string) string {
		s := NewStreamer(width)
		for _, c := range chunks {
			s.Write(c)
		}
		s.Finish()
		return s.View(false)
	}

	byRune := []string{}
	for _, r := range streamDoc {
		byRune = append(byRune, string(r))
	}
	byLine := []string{}
	for _, l := range strings.SplitAfter(streamDoc, "\n") {
		if l != "" {
			byLine = append(byLine, l)
		}
	}

	whole := drain([]string{streamDoc})
	if got := drain(byRune); got != whole {
		t.Errorf("rune-at-a-time differs from one write:\n got:\n%s\nwant:\n%s",
			ansi.Strip(got), ansi.Strip(whole))
	}
	if got := drain(byLine); got != whole {
		t.Errorf("line-at-a-time differs from one write:\n got:\n%s\nwant:\n%s",
			ansi.Strip(got), ansi.Strip(whole))
	}
}

// TestMarkdownStreamMatchesStreamedView is the defect this pair of paths caused:
// the conversation view streamed an answer through Streamer and then re-rendered
// the finished message with glamour directly, whose inter-block spacing differs
// — so the answer visibly re-laid-out on the tick the stream closed.
func TestMarkdownStreamMatchesStreamedView(t *testing.T) {
	SetTheme(themes.Get("catppuccin"))
	const width = 58

	s := NewStreamer(width)
	for _, r := range streamDoc {
		s.Write(string(r))
	}
	s.Finish()

	if got, want := MarkdownStream(streamDoc, width), s.View(false); got != want {
		t.Errorf("finalized render differs from the streamed one:\n got:\n%s\nwant:\n%s",
			ansi.Strip(got), ansi.Strip(want))
	}
}

// TestStreamerKeepsAllContent: the three-tier split is an optimization, so no
// text may be lost, duplicated or reordered relative to a whole-document parse.
// Blank lines are excluded on purpose — the streamer owns its own block spacing
// (glamour glues a heading to the paragraph below it; the streamer separates
// them) and MarkdownStream makes that choice consistent everywhere.
func TestStreamerKeepsAllContent(t *testing.T) {
	SetTheme(themes.Get("catppuccin"))
	const width = 58

	content := func(s string) []string {
		var out []string
		for _, line := range strings.Split(ansi.Strip(s), "\n") {
			if line = strings.TrimRight(line, " "); line != "" {
				out = append(out, line)
			}
		}
		return out
	}

	got := content(MarkdownStream(streamDoc, width))
	want := content(MarkdownWithWidth(streamDoc, width))
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("streamed content differs from whole-document content:\n got:\n%s\nwant:\n%s",
			strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// TestStreamerTailWrapsToWidth: an unwrapped tail is broken by the terminal at
// whatever column runs out, then re-broken at a word boundary a tick later.
func TestStreamerTailWrapsToWidth(t *testing.T) {
	SetTheme(themes.Get("catppuccin"))
	const width = 30
	s := NewStreamer(width)
	s.Write("this is a single long partial line with no newline at the end of it yet")
	for _, line := range strings.Split(s.View(false), "\n") {
		if got := ansi.StringWidth(line); got > width {
			t.Errorf("tail line is %d cells, width is %d: %q", got, width, ansi.Strip(line))
		}
	}
}

// TestStreamerFinalizeDoesNotMoveText: the same content, viewed mid-stream and
// after Finish, must occupy the same lines.
func TestStreamerFinalizeDoesNotMoveText(t *testing.T) {
	SetTheme(themes.Get("catppuccin"))
	const width = 44
	for _, line := range inlineLines {
		s := NewStreamer(width)
		s.Write(line)
		live := ansi.Strip(s.View(false))
		s.Write("\n")
		s.Finish()
		final := ansi.Strip(s.View(false))
		if strings.TrimRight(live, " \n") != strings.TrimRight(final, " \n") {
			t.Errorf("%q moved on finalize:\n live  %q\n final %q", line, live, final)
		}
	}
}

// TestStreamerInvalidatesOnThemeSwitch: a /theme mid-answer must repaint the
// blocks already finalized, not just the ones still to come.
func TestStreamerInvalidatesOnThemeSwitch(t *testing.T) {
	SetTheme(themes.Get("catppuccin"))
	s := NewStreamer(40)
	s.Write("# Heading\n\nbody text\n")
	first := s.View(false)

	SetTheme(themes.Get("dracula"))
	second := s.View(false)

	if first == second {
		t.Errorf("finalized blocks kept the old theme's colors: %q", first)
	}
	if ansi.Strip(first) != ansi.Strip(second) {
		t.Errorf("theme switch changed the text, not just the color:\n%q\n%q",
			ansi.Strip(first), ansi.Strip(second))
	}
}
