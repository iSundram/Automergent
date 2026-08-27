package render

// Cell arithmetic for the TUI's fixed-column layouts.
//
// Every width decision in the UI goes through this file. Before it, five
// helpers with three different notions of "width" coexisted — byte slicing
// (which split multi-byte runes into mojibake and panicked on small limits),
// rune counting (which mismeasures wide glyphs) and cell counting (correct, but
// blind to ANSI escapes). Rows built from a mix of them could not align, and
// `fmt.Sprintf("%-9s", styledString)` padded to nine *bytes* of escape
// sequence, which is to say not at all.
//
// The rule these functions encode: a display string's width is its rendered
// cell count, measured with ansi.StringWidth, and it is never sliced by byte.

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// Width reports the rendered cell width of s, ignoring ANSI escapes.
func Width(s string) int { return ansi.StringWidth(s) }

// Clip shortens s to at most width cells, marking the cut with an ellipsis.
// It is safe for styled input and for any width, including zero and negative:
// there is no width at which this panics or emits a partial rune.
func Clip(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if ansi.StringWidth(s) <= width {
		return s
	}
	if width == 1 {
		return glyphEllipsis
	}
	return ansi.Truncate(s, width, glyphEllipsis)
}

// Pad right-pads s with spaces to exactly width cells. Input wider than width
// is returned unchanged — use Cell when the column must not overflow.
func Pad(s string, width int) string {
	gap := width - ansi.StringWidth(s)
	if gap <= 0 {
		return s
	}
	return s + strings.Repeat(" ", gap)
}

// PadLeft left-pads s to exactly width cells, right-aligning it.
func PadLeft(s string, width int) string {
	gap := width - ansi.StringWidth(s)
	if gap <= 0 {
		return s
	}
	return strings.Repeat(" ", gap) + s
}

// Cell renders s as a fixed-width column: clipped if too wide, padded if too
// narrow, always exactly width cells. This is the primitive that makes a grid
// of rows line up regardless of styling.
func Cell(s string, width int) string {
	if width <= 0 {
		return ""
	}
	return Pad(Clip(s, width), width)
}

// CellRight is Cell, right-aligned.
func CellRight(s string, width int) string {
	if width <= 0 {
		return ""
	}
	return PadLeft(Clip(s, width), width)
}

// Rule returns a horizontal hairline of exactly n cells, or "" when n <= 0.
func Rule(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.Repeat(glyphRule, n)
}

// FirstLine returns the first line of s with surrounding space trimmed. Used
// wherever a multi-line value has to sit on a single row.
func FirstLine(s string) string {
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

// LastNonEmptyLine returns the last line of s that has non-space content. This
// is what a live output tail shows: the newest thing the process actually said,
// not the trailing newline after it.
func LastNonEmptyLine(s string) string {
	s = strings.TrimRight(s, "\r\n")
	for {
		i := strings.LastIndexByte(s, '\n')
		line := strings.TrimSpace(s[i+1:])
		if line != "" {
			return line
		}
		if i < 0 {
			return ""
		}
		s = s[:i]
	}
}

// TailLines returns the last n lines of s, oldest first, along with how many
// lines were dropped from the front.
func TailLines(s string, n int) (lines []string, hidden int) {
	if n <= 0 {
		return nil, 0
	}
	all := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(all) <= n {
		return all, 0
	}
	return all[len(all)-n:], len(all) - n
}

// Elapsed formats a duration for a live column: m:ss under an hour, h:mm:ss
// above it. Fixed width per magnitude so a ticking cell does not shift the
// columns beside it.
func Elapsed(seconds int) string {
	if seconds < 0 {
		seconds = 0
	}
	h := seconds / 3600
	m := (seconds % 3600) / 60
	s := seconds % 60
	if h > 0 {
		return strconv.Itoa(h) + ":" + pad2(m) + ":" + pad2(s)
	}
	return strconv.Itoa(m) + ":" + pad2(s)
}

func pad2(n int) string {
	if n < 10 {
		return "0" + strconv.Itoa(n)
	}
	return strconv.Itoa(n)
}
