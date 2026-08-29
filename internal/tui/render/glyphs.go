package render

// The glyph charter: the complete set of non-ASCII characters this TUI is
// allowed to draw, and the reasons the set is this small.
//
// Two classes of character are banned outright.
//
// Nerd Font private-use glyphs (U+E000–U+F8FF, U+F0000+) render as a tofu box
// for anyone who has not patched their terminal font. A pictogram nobody can
// see is worse than the word it replaced, and the UI carried about ninety of
// them.
//
// Emoji and emoji-presentation characters are a correctness problem, not a
// taste one. Terminals disagree about whether U+2699 GEAR and U+26A0 WARNING
// SIGN are one cell or two; lipgloss measures them as one. Where a terminal
// chose two, every column to the right of that glyph was off by one and the
// row's alignment collapsed. The same applies to any character with
// EastAsianWidth Wide or Ambiguous.
//
// What survives is a small vocabulary that is single-width everywhere and
// legible in an unpatched monospace font. Marks carry state, structure glyphs
// carry hierarchy, and nothing carries decoration.
const (
	// State marks. GlyphRun is a filled bullet rather than a rotating arrow:
	// a static ⟳ promises motion the frame does not deliver, and the spinner
	// beside the prompt is where motion belongs.
	GlyphRun     = "●" // in flight
	GlyphIdle    = "◌" // started, not yet producing
	GlyphOK      = "✓" // finished successfully
	GlyphFail    = "✗" // finished unsuccessfully
	GlyphStopped = "○" // cancelled, killed, or never started
	GlyphWarn    = "▲" // attention, without emoji ambiguity
	GlyphCursor  = "▸" // the focused row

	// Structure.
	GlyphElbow  = "⎿" // a result hanging off its call
	GlyphBranch = "├" // a child with siblings below it
	GlyphLast   = "└" // the last child
	GlyphPipe   = "│" // a continued vertical run
	GlyphRule   = "─" // a horizontal hairline

	// Inline punctuation.
	GlyphEllipsis = "…"
	GlyphSep      = " · "
	GlyphTo       = "→"
	GlyphUp       = "↑"
	GlyphDown     = "↓"
	GlyphTimes    = "×"
	GlyphReturn   = "⏎"
)

// glyphRule and glyphEllipsis are the lowercase aliases cells.go builds on.
const (
	glyphRule     = GlyphRule
	glyphEllipsis = GlyphEllipsis
)

// Charter returns every permitted non-ASCII rune. The charter test walks the
// TUI source and fails on any non-ASCII rune absent from this set, so adding a
// glyph to the UI means adding it here first — deliberately, in one place,
// where the width and font questions get asked.
func Charter() map[rune]bool {
	allowed := map[rune]bool{}
	for _, s := range []string{
		GlyphRun, GlyphIdle, GlyphOK, GlyphFail, GlyphStopped, GlyphWarn, GlyphCursor,
		GlyphElbow, GlyphBranch, GlyphLast, GlyphPipe, GlyphRule,
		GlyphEllipsis, GlyphSep, GlyphTo, GlyphUp, GlyphDown, GlyphTimes, GlyphReturn,
		// Box drawing completed for bordered panes, which lipgloss emits itself
		// but which components also assemble by hand.
		"┌┐┘├┤┬┴┼╭╮╰╯",
		// Meters and separators already load-bearing in the status bar and
		// diff gutter. ▎ is the markdown quote bar: glamour's IndentWriter
		// reserves one column per indent unit, so the bar must be exactly one
		// cell (see render/theme.go).
		"▌▐░▒▓█▀▁▂▃▄▅▆▇▎",
		// Arrows used by scroll and diff affordances.
		"←→↔↕⇥⇤",
		// Typography that appears in prose the UI renders. The guillemets are
		// the word-level diff markers: paired punctuation, single-width
		// everywhere, and distinguishable without colour.
		"—–‘’“”•«»‹›",
		// The brand mark and the variation selector that pins it to text
		// presentation. ✦ is East-Asian-Ambiguous; U+FE0E is what makes a
		// terminal that would resolve it through an emoji face draw it
		// single-width instead (see themes/brand.go, which explains why the
		// selector is load-bearing and not decoration).
		"✦︎",
	} {
		for _, r := range s {
			allowed[r] = true
		}
	}
	return allowed
}
