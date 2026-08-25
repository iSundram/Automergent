package render

// Inline markdown styling for the live streaming edge.
//
// glamour needs a complete block to render: it parses to an AST, so a half
// written paragraph either renders as something structurally different from
// what it will become, or has to be held back as raw text. Holding it back is
// what the conversation view used to do, which is why the newest line of a
// streaming answer arrived as literal `**foo**` and only snapped into bold once
// the newline landed.
//
// Inline fills that gap. It styles the markers a single line can carry without
// any block context, and — the point of the whole file — styles the LAST
// unterminated marker optimistically. The moment `**` is emitted the following
// text turns bold, instead of waiting for the closing `**`. When the closer
// does arrive nothing moves, because the opener was already consumed.

import (
	"image/color"
	"strings"
	"sync"

	"charm.land/lipgloss/v2"
)

// maxInlineDepth bounds nesting recursion. Real prose does not nest emphasis
// four deep; a pathological or truncated stream might.
const maxInlineDepth = 4

// listIndent is the column glamour's List block reserves before an item marker
// (themedStyleConfig sets List.Indent to 1). The inline styler emits it too, so
// a bullet does not shift left by one cell when its line is finalized.
const listIndent = " "

// inlineStyles is the lipgloss style set the inline renderer draws from,
// rebuilt whenever the theme changes.
type inlineStyles struct {
	gen uint64
	p   MarkdownPalette

	text     lipgloss.Style
	code     lipgloss.Style
	linkText lipgloss.Style
	linkURL  lipgloss.Style
	heading  [7]lipgloss.Style // index by level; [0] unused
	bullet   lipgloss.Style
	quote    lipgloss.Style
	rule     lipgloss.Style
	cursor   lipgloss.Style

	// Pre-styled tokens shared verbatim with glamour's StyleConfig.
	quoteToken   string
	taskTicked   string
	taskUnticked string
}

var (
	inlineMu    sync.Mutex
	inlineCache *inlineStyles
)

// inlineStyleSet returns the style set for the active theme, rebuilding it only
// after a theme switch.
//
// Every color comes from MarkdownColors, the same palette themedStyleConfig
// reads. That is the whole point: this renderer draws the live partial line and
// glamour redraws it the instant a newline lands, so any color either one
// derives on its own shows up as text changing color under the user's eyes.
func inlineStyleSet() *inlineStyles {
	gen := MarkdownGeneration()

	inlineMu.Lock()
	defer inlineMu.Unlock()
	if inlineCache != nil && inlineCache.gen == gen {
		return inlineCache
	}

	p := MarkdownColors()
	fg := func(c color.Color) lipgloss.Style { return lipgloss.NewStyle().Foreground(c) }
	base := fg(p.Text)
	s := &inlineStyles{
		gen:  gen,
		p:    p,
		text: base,
		code: fg(p.Code).Background(p.CodeBg),
		// LinkText is underlined and Link is not, matching themedStyleConfig.
		linkText: fg(p.LinkText).Underline(true),
		linkURL:  fg(p.LinkURL),
		heading: [7]lipgloss.Style{
			fg(p.Heading[0]).Bold(true),
			fg(p.Heading[1]).Bold(true),
			fg(p.Heading[2]).Bold(true),
			fg(p.Heading[3]).Bold(true),
			fg(p.Heading[4]).Bold(true),
			fg(p.Heading[5]).Bold(true),
			fg(p.Heading[6]).Bold(true),
		},
		bullet: fg(p.Bullet),
		quote:  fg(p.Quote).Italic(true),
		rule:   fg(p.Rule),
		cursor: fg(p.Cursor),

		quoteToken:   QuoteToken(p.QuoteBar),
		taskTicked:   TaskToken(p, true),
		taskUnticked: TaskToken(p, false),
	}
	inlineCache = s
	return s
}

// Cursor returns the streaming caret, themed. Callers append it to the live
// line so there is a visible answer to "is it still writing?".
func Cursor() string {
	return inlineStyleSet().cursor.Render("▌")
}

// Inline styles the inline markdown in a fragment of text: code spans, bold,
// italic, strikethrough and links. Unterminated markers are styled as though
// they were closed at end of input.
//
// It does no wrapping and no block-level interpretation. Pass whole lines to
// InlineBlock instead.
func Inline(s string) string {
	if s == "" {
		return ""
	}
	st := inlineStyleSet()
	var b strings.Builder
	inlineInto(&b, []rune(s), st.text, st, 0)
	return b.String()
}

// InlineBlock styles one whole line, including the block-level marker it opens
// with — heading hashes, list bullet, ordered marker, task checkbox or
// blockquote — and then the inline markers in the remainder.
func InlineBlock(line string) string {
	st := inlineStyleSet()
	return inlineBlockWith(line, st)
}

func inlineBlockWith(line string, st *inlineStyles) string {
	if strings.TrimSpace(line) == "" {
		return line
	}

	indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
	rest := line[len(indent):]

	switch {
	// Heading: replace the hashes with weight and hue, matching what glamour
	// will do to this line one tick from now.
	case strings.HasPrefix(rest, "#"):
		level := 0
		for level < len(rest) && rest[level] == '#' {
			level++
		}
		if level <= 6 && level < len(rest) && (rest[level] == ' ' || rest[level] == '\t') {
			body := strings.TrimSpace(rest[level:])
			return indent + st.heading[level].Render(body)
		}

	// Blockquote: the bar, then the quoted text (recursing so a nested quote
	// gets a second bar).
	case strings.HasPrefix(rest, ">"):
		body := strings.TrimPrefix(rest[1:], " ")
		inner := inlineBlockWith(body, st)
		if strings.TrimSpace(body) == "" {
			inner = ""
		} else if !strings.HasPrefix(strings.TrimSpace(body), ">") {
			var b strings.Builder
			inlineInto(&b, []rune(body), st.quote, st, 0)
			inner = b.String()
		}
		return indent + st.quoteToken + inner

	// Horizontal rule.
	case isRuleLine(rest):
		return indent + st.rule.Render(RuleFormat)
	}

	// Task list item: "- [ ] " / "- [x] ". The checkbox *replaces* the bullet —
	// glamour's TaskElement is rendered instead of ItemElement, not in addition
	// to it — and the glyph is the same pre-styled token glamour emits, so the
	// box does not change color on finalize either.
	if _, body, ok := splitBullet(rest); ok {
		if checked, task, isTask := splitTask(body); isTask {
			glyph := st.taskUnticked
			if checked {
				glyph = st.taskTicked
			}
			var b strings.Builder
			inlineInto(&b, []rune(task), st.text, st, 0)
			return st.text.Render(listIndent+indent) + glyph + b.String()
		}
		var b strings.Builder
		inlineInto(&b, []rune(body), st.text, st, 0)
		return st.text.Render(listIndent+indent) + st.bullet.Render("• ") + b.String()
	}
	if number, body, ok := splitOrdered(rest); ok {
		// glamour renders the number in the list's own color and only the ". "
		// separator in the marker color (ItemElement passes the digits as the
		// BaseElement prefix, which doRender styles with the parent block).
		var b strings.Builder
		inlineInto(&b, []rune(body), st.text, st, 0)
		return st.text.Render(listIndent+indent+number) + st.bullet.Render(". ") + b.String()
	}

	var b strings.Builder
	inlineInto(&b, []rune(rest), st.text, st, 0)
	return indent + b.String()
}

// splitBullet recognises an unordered list marker and returns the marker and
// the item text.
func splitBullet(s string) (string, string, bool) {
	if len(s) < 2 {
		return "", "", false
	}
	switch s[0] {
	case '-', '*', '+':
	default:
		return "", "", false
	}
	if s[1] != ' ' && s[1] != '\t' {
		return "", "", false
	}
	return s[:1], strings.TrimLeft(s[1:], " \t"), true
}

// splitOrdered recognises "1. " / "2) " and returns the digits plus the item
// text. Only the digits: glamour always renders ". " as the separator via
// Enumeration.Prefix, whichever of "." or ")" the source used.
func splitOrdered(s string) (string, string, bool) {
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i == 0 || i > 9 || i+1 >= len(s) {
		return "", "", false
	}
	if s[i] != '.' && s[i] != ')' {
		return "", "", false
	}
	if s[i+1] != ' ' && s[i+1] != '\t' {
		return "", "", false
	}
	return s[:i], strings.TrimLeft(s[i+1:], " \t"), true
}

// splitTask recognises a checkbox at the head of a list item body.
func splitTask(s string) (checked bool, rest string, ok bool) {
	if len(s) < 3 || s[0] != '[' || s[2] != ']' {
		return false, "", false
	}
	switch s[1] {
	case ' ':
		checked = false
	case 'x', 'X':
		checked = true
	default:
		return false, "", false
	}
	return checked, strings.TrimLeft(s[3:], " \t"), true
}

// isRuleLine reports whether a line is a thematic break.
func isRuleLine(s string) bool {
	s = strings.TrimSpace(s)
	if len(s) < 3 {
		return false
	}
	c := s[0]
	if c != '-' && c != '*' && c != '_' {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] != c && s[i] != ' ' {
			return false
		}
	}
	return true
}

// inlineInto is the scanner. base is the style inherited from the enclosing
// span, so emphasis composes: bold inside italic renders bold-italic.
func inlineInto(b *strings.Builder, runes []rune, base lipgloss.Style, st *inlineStyles, depth int) {
	n := len(runes)
	// plain accumulates unmarked runs so one Render call covers a whole span
	// rather than one per rune.
	var plain strings.Builder
	flush := func() {
		if plain.Len() > 0 {
			b.WriteString(base.Render(plain.String()))
			plain.Reset()
		}
	}

	for i := 0; i < n; {
		c := runes[i]

		// A backslash escape is literal, and consumes the marker so it cannot
		// open a span.
		if c == '\\' && i+1 < n && isMarkdownPunct(runes[i+1]) {
			plain.WriteRune(runes[i+1])
			i += 2
			continue
		}

		// Code spans bind tightest: nothing inside them is a marker.
		if c == '`' {
			fence := runLength(runes, i, '`')
			end := findRun(runes, i+fence, '`', fence)
			var body []rune
			if end < 0 {
				body = runes[i+fence:] // unterminated: style to end of input
				i = n
			} else {
				body = runes[i+fence : end]
				i = end + fence
			}
			flush()
			b.WriteString(st.code.Render(" " + string(body) + " "))
			continue
		}

		// Bare URLs. GFM autolinking is on in glamour, which renders a bare
		// http(s):// or www. run in the link color — so leaving it as plain text
		// here made every URL change color the moment its line was finalized.
		if autolink, next, ok := scanAutolink(runes, i); ok {
			flush()
			b.WriteString(st.linkURL.Render(autolink))
			i = next
			continue
		}

		// Links. A truncated "[text](" or even "[text" still styles the text,
		// so the label does not flicker from plain to blue.
		if c == '[' && depth < maxInlineDepth {
			if label, url, next, ok := scanLink(runes, i); ok {
				flush()
				var lb strings.Builder
				inlineInto(&lb, label, st.linkText, st, depth+1)
				b.WriteString(lb.String())
				if len(url) > 0 {
					// The separating space belongs to the sentence, not the
					// address: glamour emits it as part of the enclosing text
					// node, so styling it as link-colored made the gap before
					// every URL shift hue on finalize.
					b.WriteString(base.Render(" "))
					b.WriteString(st.linkURL.Render(string(url)))
				}
				i = next
				continue
			}
		}

		// Emphasis. Longest marker first: ** before *, ~~ before ~.
		if marker, style, ok := emphasisAt(runes, i, base, st); ok && depth < maxInlineDepth {
			mlen := len(marker)
			end := findRun(runes, i+mlen, marker[0], mlen)
			var body []rune
			if end < 0 {
				body = runes[i+mlen:] // optimistic: the closer has not streamed yet
				i = n
			} else {
				body = runes[i+mlen : end]
				i = end + mlen
			}
			if len(body) == 0 {
				// "**" with nothing after it: keep the literal so the user sees
				// their own keystrokes rather than nothing at all.
				plain.WriteString(string(marker))
				continue
			}
			flush()
			inlineInto(b, body, style, st, depth+1)
			continue
		}

		plain.WriteRune(c)
		i++
	}
	flush()
}

// emphasisAt reports the emphasis marker starting at i, if any, along with the
// style its span should carry.
//
// Each span sets its own foreground rather than just adding an attribute to the
// inherited one. That mirrors glamour, whose cascade lets a child style's color
// win over the enclosing block's — so **bold** is body-colored even inside a
// subtext-colored quote, and *italic* is subtext-colored even in body prose.
// Composing attributes onto the inherited color instead left italics and
// strikethrough a different hue live than they were a tick later.
func emphasisAt(runes []rune, i int, base lipgloss.Style, st *inlineStyles) ([]rune, lipgloss.Style, bool) {
	c := runes[i]
	switch c {
	case '*', '_':
		if runLength(runes, i, c) >= 2 {
			return []rune{c, c}, base.Bold(true).Foreground(st.p.Text), true
		}
		// A lone underscore inside a word is an identifier, not emphasis:
		// snake_case must survive.
		if c == '_' && !atWordBoundary(runes, i) {
			return nil, base, false
		}
		return []rune{c}, base.Italic(true).Foreground(st.p.Subtext), true
	case '~':
		if runLength(runes, i, c) >= 2 {
			return []rune{c, c}, base.Strikethrough(true).Foreground(st.p.Muted), true
		}
	}
	return nil, base, false
}

// atWordBoundary reports whether position i is not sandwiched between two
// alphanumerics.
func atWordBoundary(runes []rune, i int) bool {
	before := i == 0 || !isWordRune(runes[i-1])
	after := i+1 >= len(runes) || !isWordRune(runes[i+1])
	return before || after
}

func isWordRune(r rune) bool {
	return r == '_' ||
		(r >= '0' && r <= '9') ||
		(r >= 'a' && r <= 'z') ||
		(r >= 'A' && r <= 'Z')
}

// scanAutolink recognises a GFM-style bare URL at i and returns it along with
// the index to resume at.
//
// It must start at a word boundary so a query string inside an already-consumed
// URL, or "shttp://" in prose, is not mistaken for a link. Trailing sentence
// punctuation is left outside the link, which is what goldmark's linkify does.
func scanAutolink(runes []rune, i int) (string, int, bool) {
	if i > 0 && isWordRune(runes[i-1]) {
		return "", 0, false
	}
	n := len(runes)
	rest := runes[i:]
	if !hasRunePrefix(rest, "http://") && !hasRunePrefix(rest, "https://") && !hasRunePrefix(rest, "www.") {
		return "", 0, false
	}

	end := i
	for end < n && !isURLTerminator(runes[end]) {
		end++
	}
	// Back off trailing punctuation that reads as sentence structure rather than
	// part of the address, and an unbalanced closing paren (as in "(see
	// https://x.com/a)").
	for end > i {
		switch runes[end-1] {
		case '.', ',', ':', ';', '!', '?', '\'', '"', '*', '_', '~':
		case ')':
			if countRune(runes[i:end], '(') >= countRune(runes[i:end], ')') {
				return string(runes[i:end]), end, true
			}
		default:
			return string(runes[i:end]), end, true
		}
		end--
	}
	return "", 0, false
}

func hasRunePrefix(runes []rune, prefix string) bool {
	p := []rune(prefix)
	if len(runes) < len(p) {
		return false
	}
	for i, r := range p {
		// Scheme comparison is case-insensitive; the rest of a URL is not, but
		// only the prefix is compared here.
		if lowerASCII(runes[i]) != r {
			return false
		}
	}
	return true
}

func lowerASCII(r rune) rune {
	if r >= 'A' && r <= 'Z' {
		return r + ('a' - 'A')
	}
	return r
}

func isURLTerminator(r rune) bool {
	switch r {
	case ' ', '\t', '\n', '<', '>', '`', '"', '\'':
		return true
	}
	return false
}

func countRune(runes []rune, c rune) int {
	n := 0
	for _, r := range runes {
		if r == c {
			n++
		}
	}
	return n
}

// scanLink parses "[label](url)" at i, tolerating truncation at any point after
// the opening bracket. next is the index to resume at.
func scanLink(runes []rune, i int) (label, url []rune, next int, ok bool) {
	n := len(runes)
	close := -1
	for j := i + 1; j < n; j++ {
		if runes[j] == '\\' {
			j++
			continue
		}
		if runes[j] == ']' {
			close = j
			break
		}
	}
	if close < 0 {
		// "[still typing" — treat everything after the bracket as the label.
		// A bare "[" with nothing after it is not worth styling.
		if i+1 >= n {
			return nil, nil, 0, false
		}
		return runes[i+1:], nil, n, true
	}
	if close+1 >= n || runes[close+1] != '(' {
		// A reference-style or bare bracket. Style the label, drop nothing.
		return runes[i+1 : close], nil, close + 1, true
	}
	paren := -1
	for j := close + 2; j < n; j++ {
		if runes[j] == ')' {
			paren = j
			break
		}
	}
	if paren < 0 {
		return runes[i+1 : close], runes[close+2:], n, true
	}
	return runes[i+1 : close], runes[close+2 : paren], paren + 1, true
}

// runLength counts consecutive occurrences of c starting at i.
func runLength(runes []rune, i int, c rune) int {
	k := 0
	for i+k < len(runes) && runes[i+k] == c {
		k++
	}
	return k
}

// findRun returns the index of the next run of exactly want copies of c at or
// after start, or -1.
func findRun(runes []rune, start int, c rune, want int) int {
	for j := start; j < len(runes); j++ {
		if runes[j] == '\\' {
			j++
			continue
		}
		if runes[j] != c {
			continue
		}
		if runLength(runes, j, c) == want {
			return j
		}
		// A longer run cannot close this span; skip past it whole.
		j += runLength(runes, j, c) - 1
	}
	return -1
}

func isMarkdownPunct(r rune) bool {
	switch r {
	case '\\', '`', '*', '_', '{', '}', '[', ']', '(', ')',
		'#', '+', '-', '.', '!', '|', '~', '<', '>', '"', '$':
		return true
	}
	return false
}
