package render

// Incremental streaming markdown.
//
// The conversation view used to re-render the entire stable prefix of an answer
// on every tick: at token n it glamour-parsed all n tokens, so a long answer
// cost O(n²) parses over its lifetime and the newest line was shown as raw
// markdown because it had no trailing newline yet.
//
// Streamer splits the text into three tiers instead:
//
//	finalized blocks   rendered by glamour exactly once, then cached forever
//	the open block     one glamour render per tick, bounded to a single block
//	the partial line   styled by InlineBlock, which needs no block context
//
// Total work becomes O(n), and the live line is styled the moment a marker
// arrives rather than when it closes.

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// streamBlock is a finalized block: its source, its render, and how it was
// separated from the block before it.
type streamBlock struct {
	src string
	out string
	// blankBefore records that a blank line separated this block from the
	// previous one, so joining the renders reproduces the paragraph spacing a
	// whole-document render would have produced.
	blankBefore bool
}

// Streamer accumulates streamed markdown and renders it incrementally.
//
// It is not safe for concurrent use; it is owned by the component that holds
// the stream.
type Streamer struct {
	width int
	gen   uint64

	full strings.Builder

	blocks []streamBlock
	open   []string // raw lines of the block currently being written
	blanks int      // blank lines seen inside the open block, not yet placed
	tail   string   // the partial line: no newline has arrived for it yet

	openBlank bool // a blank line separates the open block from the previous
	fence     string
	fenceLang string

	openOut   string
	openValid bool
}

// NewStreamer returns a Streamer that renders at the given width. A width of 0
// means "do not wrap".
func NewStreamer(width int) *Streamer {
	return &Streamer{width: width, gen: MarkdownGeneration()}
}

// SetWidth changes the render width, invalidating cached output if it actually
// moved.
func (s *Streamer) SetWidth(width int) {
	if width == s.width {
		return
	}
	s.width = width
	s.invalidate()
}

// Width reports the current render width.
func (s *Streamer) Width() int { return s.width }

// Reset clears all accumulated text.
func (s *Streamer) Reset() {
	s.full.Reset()
	s.blocks = nil
	s.open = nil
	s.blanks = 0
	s.tail = ""
	s.openBlank = false
	s.fence = ""
	s.fenceLang = ""
	s.openOut = ""
	s.openValid = false
	s.gen = MarkdownGeneration()
}

// Raw returns everything written so far, unrendered. Callers finalizing a
// message use this as the message body.
func (s *Streamer) Raw() string { return s.full.String() }

// Empty reports whether anything has been written.
func (s *Streamer) Empty() bool { return s.full.Len() == 0 }

// Write appends a delta of streamed text.
func (s *Streamer) Write(delta string) {
	if delta == "" {
		return
	}
	s.full.WriteString(delta)

	buf := s.tail + delta
	for {
		nl := strings.IndexByte(buf, '\n')
		if nl < 0 {
			break
		}
		s.addLine(strings.TrimSuffix(buf[:nl], "\r"))
		buf = buf[nl+1:]
	}
	s.tail = buf
}

// invalidate drops every cached render. The sources are kept, so the next View
// rebuilds at the new width or under the new theme.
func (s *Streamer) invalidate() {
	for i := range s.blocks {
		s.blocks[i].out = ""
	}
	s.openValid = false
}

// revalidate discards caches that a theme switch has made stale.
func (s *Streamer) revalidate() {
	if gen := MarkdownGeneration(); gen != s.gen {
		s.gen = gen
		s.invalidate()
	}
}

// addLine folds one complete line into the block structure.
func (s *Streamer) addLine(line string) {
	// Inside a fenced code block everything is content until the closing fence,
	// including blank lines and things that look like headings.
	if s.fence != "" {
		s.open = append(s.open, line)
		s.openValid = false
		if closesFence(line, s.fence) {
			s.fence = ""
			s.fenceLang = ""
			s.finalizeOpen()
		}
		return
	}

	if strings.TrimSpace(line) == "" {
		// A blank line does not necessarily end a block: a loose list keeps its
		// numbering across one. Hold it and decide when the next content line
		// tells us what it belongs to.
		s.blanks++
		return
	}

	if marker, lang, ok := fenceAt(line); ok {
		s.flushBlanks(line)
		s.open = append(s.open, line)
		s.fence = marker
		s.fenceLang = lang
		s.openValid = false
		return
	}

	// A heading or a rule is always its own block, so it can be finalized the
	// moment the next line lands rather than waiting for a blank.
	if isOwnBlock(line) {
		s.finalizeOpen()
		s.blankBeforeNext()
		s.open = []string{line}
		s.openValid = false
		s.finalizeOpen()
		return
	}

	if len(s.open) > 0 && isOwnBlock(s.open[len(s.open)-1]) {
		s.finalizeOpen()
	}

	s.flushBlanks(line)
	s.open = append(s.open, line)
	s.openValid = false
}

// flushBlanks resolves the held blank lines now that a content line has
// arrived: either they are interior to the open block, or they closed it.
func (s *Streamer) flushBlanks(next string) {
	if s.blanks == 0 {
		return
	}
	if len(s.open) > 0 && continuesBlock(s.open, next) {
		for i := 0; i < s.blanks; i++ {
			s.open = append(s.open, "")
		}
		s.blanks = 0
		s.openValid = false
		return
	}
	s.blanks = 0
	s.finalizeOpen()
	s.blankBeforeNext()
}

// blankBeforeNext marks the block that opens next as blank-separated.
func (s *Streamer) blankBeforeNext() {
	if len(s.blocks) > 0 {
		s.openBlank = true
	}
}

// finalizeOpen renders the open block once and moves it into the finalized
// list. The render is the only glamour parse that block will ever get.
func (s *Streamer) finalizeOpen() {
	if len(s.open) == 0 {
		return
	}
	src := strings.Join(s.open, "\n")
	s.blocks = append(s.blocks, streamBlock{
		src:         src,
		out:         s.renderBlock(src),
		blankBefore: s.openBlank,
	})
	s.open = nil
	s.openOut = ""
	s.openValid = false
	s.openBlank = false
}

// Finish closes out any open block and partial line. Call it when the stream
// ends, so the last paragraph is rendered as a block rather than left on the
// inline path.
func (s *Streamer) Finish() {
	if s.tail != "" {
		s.addLine(s.tail)
		s.tail = ""
	}
	s.blanks = 0
	s.fence = ""
	s.finalizeOpen()
}

// View renders the accumulated text. showCursor appends a themed caret to the
// live line, which is the cheapest honest signal that output is still arriving.
func (s *Streamer) View(showCursor bool) string {
	s.revalidate()

	var b strings.Builder
	wrote := false
	emit := func(chunk string, blank bool) {
		if chunk == "" {
			return
		}
		if wrote {
			b.WriteString("\n")
			if blank {
				b.WriteString("\n")
			}
		}
		b.WriteString(chunk)
		wrote = true
	}

	for i := range s.blocks {
		if s.blocks[i].out == "" {
			s.blocks[i].out = s.renderBlock(s.blocks[i].src)
		}
		emit(s.blocks[i].out, s.blocks[i].blankBefore)
	}
	emit(s.openRender(), s.openBlank)

	// Held blank lines belong between the open block and the live line.
	tailBlank := s.blanks > 0
	emit(s.tailRender(showCursor), tailBlank)

	return b.String()
}

// openRender renders the block currently being written, caching the result so
// a tick that only extended the partial line costs nothing.
func (s *Streamer) openRender() string {
	if len(s.open) == 0 {
		return ""
	}
	if s.openValid {
		return s.openOut
	}
	if s.fence != "" {
		// An unterminated fence is not valid markdown, so glamour would render
		// it as a paragraph. Highlight it directly with the same chroma style
		// and indent that themedStyleConfig gives a closed fence, so nothing
		// shifts when the closing ``` arrives.
		s.openOut = s.renderOpenFence()
	} else {
		s.openOut = s.renderBlock(strings.Join(s.open, "\n"))
	}
	s.openValid = true
	return s.openOut
}

// renderOpenFence highlights the body of a still-open code fence.
func (s *Streamer) renderOpenFence() string {
	if len(s.open) <= 1 {
		return ""
	}
	body := strings.Join(s.open[1:], "\n")
	highlighted := Code(body, s.fenceLang)
	lines := strings.Split(strings.TrimRight(highlighted, "\n"), "\n")
	for i, line := range lines {
		lines[i] = " " + line
	}
	return strings.Join(lines, "\n")
}

// tailRender styles the partial line. Inside a fence it is code; otherwise it
// goes through the inline styler, which handles unterminated markers.
func (s *Streamer) tailRender(showCursor bool) string {
	if s.tail == "" {
		// No partial text: a cursor here would render as a lone ▌ on its
		// own line — noise between finished blocks. The spinner and status
		// line already say "still working", so emit nothing.
		return ""
	}
	var out string
	if s.fence != "" {
		out = " " + strings.TrimRight(Code(s.tail, s.fenceLang), "\n")
	} else {
		// Wrapped to the same width glamour will use. Without this, a partial
		// line longer than the pane is broken by the terminal at whatever column
		// runs out — then finalized a tick later at a word boundary, so the text
		// visibly re-flowed. ansi.Wordwrap is ANSI-aware, which matters because
		// the line is already styled.
		out = InlineBlock(s.tail)
		if s.width > 0 {
			out = ansi.Wordwrap(out, s.width, "")
		}
	}
	if showCursor {
		out += Cursor()
	}
	return out
}

func (s *Streamer) renderBlock(src string) string {
	if strings.TrimSpace(src) == "" {
		return ""
	}
	return MarkdownWithWidth(src, s.width)
}

// MarkdownStream renders a complete markdown document through the Streamer.
//
// This exists so a finished answer is drawn by the same code that drew it while
// it was arriving. The conversation view used to call MarkdownWithWidth once a
// turn ended, and glamour's own inter-block spacing does not match the
// streamer's — so the whole answer silently re-laid-out on the tick the stream
// closed, which is the "it re-renders after it finishes" the footer chrome got
// blamed for. Routing both paths through here makes them identical by
// construction rather than by two implementations agreeing.
func MarkdownStream(content string, width int) string {
	if strings.TrimSpace(content) == "" {
		return ""
	}
	s := NewStreamer(width)
	s.Write(content)
	s.Finish()
	return s.View(false)
}

// isOwnBlock reports whether a line is a construct that never merges with
// neighbouring lines: an ATX heading or a thematic break.
func isOwnBlock(line string) bool {
	trimmed := strings.TrimLeft(line, " \t")
	if strings.HasPrefix(trimmed, "#") {
		level := 0
		for level < len(trimmed) && trimmed[level] == '#' {
			level++
		}
		return level <= 6 && level < len(trimmed) &&
			(trimmed[level] == ' ' || trimmed[level] == '\t')
	}
	return isRuleLine(trimmed)
}

// continuesBlock decides whether next, arriving after one or more blank lines,
// belongs to the same block as the lines already open.
//
// Getting this wrong is visible: split a loose list and the second half
// restarts at 1; keep a new paragraph attached and it gets list-indented.
func continuesBlock(open []string, next string) bool {
	last := ""
	for i := len(open) - 1; i >= 0; i-- {
		if strings.TrimSpace(open[i]) != "" {
			last = open[i]
			break
		}
	}
	if last == "" {
		return false
	}

	nextIndent := len(next) - len(strings.TrimLeft(next, " \t"))
	trimmedNext := strings.TrimLeft(next, " \t")

	openIsList := isListLine(last)
	nextIsList := isListLine(trimmedNext)

	switch {
	// An indented continuation after a list item is that item's second
	// paragraph.
	case openIsList && nextIndent >= 2:
		return true
	// Two list items separated by a blank line are one loose list.
	case openIsList && nextIsList:
		return true
	// A quote continues across blank lines only if the next line is also
	// quoted; markdown would not join them otherwise.
	case strings.HasPrefix(strings.TrimLeft(last, " \t"), ">"):
		return strings.HasPrefix(trimmedNext, ">")
	}
	return false
}

// isListLine reports whether a line opens a list item.
func isListLine(line string) bool {
	trimmed := strings.TrimLeft(line, " \t")
	if _, _, ok := splitBullet(trimmed); ok {
		return true
	}
	_, _, ok := splitOrdered(trimmed)
	return ok
}

// fenceAt reports whether a line opens a code fence, returning the fence
// marker and the info string (language).
func fenceAt(line string) (marker, lang string, ok bool) {
	trimmed := strings.TrimLeft(line, " \t")
	if len(trimmed) < 3 {
		return "", "", false
	}
	c := trimmed[0]
	if c != '`' && c != '~' {
		return "", "", false
	}
	run := 0
	for run < len(trimmed) && trimmed[run] == c {
		run++
	}
	if run < 3 {
		return "", "", false
	}
	info := strings.TrimSpace(trimmed[run:])
	// A backtick fence cannot carry a backtick in its info string.
	if c == '`' && strings.ContainsRune(info, '`') {
		return "", "", false
	}
	if i := strings.IndexAny(info, " \t"); i >= 0 {
		info = info[:i]
	}
	return strings.Repeat(string(c), run), info, true
}

// closesFence reports whether a line closes an open fence: a run of the same
// character, at least as long as the opener, and nothing else.
func closesFence(line, marker string) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || marker == "" {
		return false
	}
	c := marker[0]
	for i := 0; i < len(trimmed); i++ {
		if trimmed[i] != c {
			return false
		}
	}
	return len(trimmed) >= len(marker)
}
