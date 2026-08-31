package components

// The shared row grammar every tool card is built from. Three primitives, in
// strict order:
//
//	● Grep  "renderToolCall"  ·  12 matches in 4 files            0.3s   <- callLine
//	  ⎿ conversation_tools.go:109   ×6                                   <- resultRow
//	    … 2 more files                                                   <- moreRow
//
// followed by at most one detail block (slab / diffBox / table from
// tool_box.go). No family draws its own chrome — that is what keeps 48 tools
// looking like one program.
//
// Glyphs are plain Unicode, never Nerd Font: a patched font is not a
// prerequisite for reading the log.

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// Status and structure glyphs.
const (
	glyphDone    = "●"
	glyphRunning = "◌"
	glyphElbow   = "⎿"
	glyphOK      = "✓"
	glyphFail    = "✗"
	glyphWarn    = "▲"
	glyphPending = "○"
	glyphActive  = "▸"
	glyphMore    = "…"
	glyphUp      = "↑"
	glyphTo      = "→"
	glyphTimes   = "×"
	glyphReturn  = "⏎"
	glyphSep     = " · "
)

// gutter is the left inset shared by every tool row, so cards align with each
// other and with assistant text.
const gutter = "  "

// rowIndent is where result rows and detail blocks hang, one step in from the
// call line's status glyph.
const rowIndent = "    "

// statusGlyph renders the leading bullet: quiet on success, loud on failure.
func (c *Conversation) statusGlyph(m ConversationMsg) string {
	switch {
	case m.Status == "running":
		return lipgloss.NewStyle().Foreground(c.styles.T.Yellow).Render(glyphRunning)
	case m.IsError || m.Status == "error":
		return lipgloss.NewStyle().Foreground(c.styles.T.Red).Bold(true).Render(glyphDone)
	default:
		return lipgloss.NewStyle().Foreground(c.styles.T.Subtext).Render(glyphDone)
	}
}

// callLine assembles the one-line headline: status glyph, accent-colored tool
// name, subject, meta chips, and a right-aligned trailer (duration / count).
// It is always exactly one visual line at any width.
func (c *Conversation) callLine(m ConversationMsg, width int, subject string, chips []string, trailer string) string {
	spec := specFor(m.ToolName)

	name := lipgloss.NewStyle().
		Foreground(spec.Accent(c.styles.T)).
		Bold(true).
		Render(spec.Display)

	left := gutter + c.statusGlyph(m) + " " + name

	// The subject is the only part allowed to shrink, so chips and the trailer
	// never get cut off.
	tail := ""
	if chip := strings.Join(chips, glyphSep); chip != "" {
		tail = "  " + c.styles.Dim.Render(chip)
	}
	trail := ""
	if strings.TrimSpace(trailer) != "" {
		trail = c.styles.Dim.Render(trailer)
	}

	room := width - lipgloss.Width(left) - lipgloss.Width(tail) - lipgloss.Width(trail) - 3
	if room < 8 {
		room = 8
	}
	subj := ""
	if strings.TrimSpace(subject) != "" {
		subj = "  " + lipgloss.NewStyle().
			Foreground(c.styles.T.Text).
			Render(ansiSafeTruncate(oneLine(subject), room))
	}

	line := left + subj + tail
	if trail == "" {
		return line
	}
	pad := width - lipgloss.Width(line) - lipgloss.Width(trail)
	if pad < 1 {
		pad = 1
	}
	return line + strings.Repeat(" ", pad) + trail
}

// resultRow renders one indented fact under the call line. mark is an optional
// severity glyph (already styled); pass "" for the plain elbow row.
func (c *Conversation) resultRow(mark, text string, width int) string {
	elbow := c.styles.Dim.Render(glyphElbow)
	prefix := rowIndent + elbow + " "
	if mark != "" {
		prefix += mark + " "
	}
	room := width - lipgloss.Width(prefix) - 1
	if room < 8 {
		room = 8
	}
	return prefix + c.styles.Dim.Render(ansiSafeTruncate(oneLine(text), room))
}

// plainRow renders an indented row with no elbow — for list bodies (todos,
// plan steps, table rows) where every line is a peer rather than a result.
func (c *Conversation) plainRow(mark, text string, width int) string {
	prefix := rowIndent + "  "
	if mark != "" {
		prefix += mark + " "
	}
	room := width - lipgloss.Width(prefix) - 1
	if room < 8 {
		room = 8
	}
	return prefix + c.styles.Dim.Render(ansiSafeTruncate(oneLine(text), room))
}

// moreRow renders the truncation tail: "… 2 more files".
func (c *Conversation) moreRow(n int, noun string) string {
	if n <= 0 {
		return ""
	}
	return rowIndent + "  " + c.styles.Dim.Render(
		fmt.Sprintf("%s %s", glyphMore, plural(n, "more "+noun)))
}

// hintRow renders the expand affordance below a truncated detail block. The
// hint names the command that changes the state: a COLLAPSED block
// advertises /expand, an EXPANDED one (clipped only to fit) advertises
// /collapse — so the affordance is always actionable.
func (c *Conversation) hintRow(hidden int, noun string) string {
	if hidden <= 0 {
		return ""
	}
	return rowIndent + c.styles.Dim.Render(
		fmt.Sprintf("%s %d more %s — %s", glyphUp, hidden, noun, c.expandHintVerb()))
}

// expandHintVerb is the command a block's current state invites: collapsed
// content says /expand, expanded content that is merely clipped says
// /collapse (to collapse it back to one line).
func (c *Conversation) expandHintVerb() string {
	if c.expandMode == ExpandFull {
		return "/collapse"
	}
	return "/expand"
}

// severityMark styles a severity glyph for diagnostics and security rows.
func (c *Conversation) severityMark(severity string) string {
	switch strings.ToLower(severity) {
	case "error", "err", "high", "critical", "fail":
		return lipgloss.NewStyle().Foreground(c.styles.T.Red).Render(glyphFail)
	case "warn", "warning", "medium":
		return lipgloss.NewStyle().Foreground(c.styles.T.Yellow).Render(glyphWarn)
	case "ok", "clean", "pass":
		return lipgloss.NewStyle().Foreground(c.styles.T.Green).Render(glyphOK)
	default:
		return c.styles.Dim.Render(glyphSep[1:2])
	}
}

// todoMark styles the checkbox glyph for a todo/task/plan status, matching the
// side board in taskboard.go so both views read as the same object.
func (c *Conversation) todoMark(status string) string {
	switch strings.ToLower(status) {
	case "completed", "done", "complete":
		return lipgloss.NewStyle().Foreground(c.styles.T.Green).Render(glyphOK)
	case "in_progress", "in progress", "running", "active":
		return lipgloss.NewStyle().Foreground(c.styles.T.Yellow).Render(glyphActive)
	case "blocked", "failed", "error", "cancelled", "killed":
		return lipgloss.NewStyle().Foreground(c.styles.T.Red).Render(glyphFail)
	default:
		return c.styles.Dim.Render(glyphPending)
	}
}

// table renders aligned columns for tabular tool output (sql, list_shells).
// Headers are dim and uppercase; columns size to content and shrink from the
// right when the terminal is narrow.
func (c *Conversation) table(headers []string, rows [][]string, width int) string {
	if len(headers) == 0 || len(rows) == 0 {
		return ""
	}
	cols := len(headers)
	widths := make([]int, cols)
	for i, h := range headers {
		widths[i] = lipgloss.Width(h)
	}
	for _, row := range rows {
		for i := 0; i < cols && i < len(row); i++ {
			if w := lipgloss.Width(row[i]); w > widths[i] {
				widths[i] = w
			}
		}
	}

	// Shrink the widest column until the table fits.
	avail := width - lipgloss.Width(rowIndent) - 4
	for {
		total := (cols - 1) * 2
		widest, widestIdx := 0, 0
		for i, w := range widths {
			total += w
			if w > widest {
				widest, widestIdx = w, i
			}
		}
		if total <= avail || widest <= 6 {
			break
		}
		widths[widestIdx]--
	}

	pad := func(s string, w int) string {
		s = ansiSafeTruncate(s, w)
		if gap := w - lipgloss.Width(s); gap > 0 {
			s += strings.Repeat(" ", gap)
		}
		return s
	}

	var b strings.Builder
	headCells := make([]string, cols)
	for i, h := range headers {
		headCells[i] = pad(strings.ToUpper(h), widths[i])
	}
	b.WriteString(rowIndent + c.styles.Dim.Copy().Bold(true).
		Render(strings.TrimRight(strings.Join(headCells, "  "), " ")))

	body := lipgloss.NewStyle().Foreground(c.styles.T.Subtext)
	for _, row := range rows {
		cells := make([]string, cols)
		for i := 0; i < cols; i++ {
			cell := ""
			if i < len(row) {
				cell = oneLine(row[i])
			}
			cells[i] = pad(cell, widths[i])
		}
		b.WriteString("\n" + rowIndent + body.Render(
			strings.TrimRight(strings.Join(cells, "  "), " ")))
	}
	return b.String()
}

// subjectFor picks the headline subject: the first populated argument named by
// the tool's spec, falling back to the event context the agent supplied.
func subjectFor(m ConversationMsg) string {
	spec := specFor(m.ToolName)
	args := argsOf(m)
	for _, key := range spec.Subject {
		if v, ok := args[key]; ok {
			if s := scalarString(v); s != "" {
				return s
			}
		}
	}
	// The agent's own context line is a decent fallback, minus its verb
	// decoration ("reading main.go" / "exec: go build").
	ctx := strings.TrimSpace(m.ToolContext)
	for _, prefix := range []string{"exec: ", "reading ", "writing ", "editing ", "deleting ", "listing ", "search: ", "fetch: ", "web: ", "diagnostics: "} {
		ctx = strings.TrimPrefix(ctx, prefix)
	}
	return ctx
}

// argsOf decodes the recorded tool arguments, cached-free and error-tolerant.
func argsOf(m ConversationMsg) map[string]any {
	if strings.TrimSpace(m.ToolArgs) == "" {
		return nil
	}
	var out map[string]any
	if json.Unmarshal([]byte(m.ToolArgs), &out) != nil {
		return nil
	}
	return out
}

// scalarString renders a JSON scalar for display; containers return "".
func scalarString(v any) string {
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case float64:
		if t == float64(int64(t)) {
			return fmt.Sprintf("%d", int64(t))
		}
		return fmt.Sprintf("%g", t)
	case bool:
		return fmt.Sprintf("%t", t)
	default:
		return ""
	}
}

// metaInt reads an integer from Result.Metadata, which arrives as float64
// through JSON but as int when set in-process.
func metaInt(m ConversationMsg, key string) (int, bool) {
	v, ok := m.Metadata[key]
	if !ok {
		return 0, false
	}
	switch t := v.(type) {
	case int:
		return t, true
	case int64:
		return int(t), true
	case float64:
		return int(t), true
	}
	return 0, false
}

// metaString reads a string from Result.Metadata.
func metaString(m ConversationMsg, key string) string {
	if v, ok := m.Metadata[key]; ok {
		return scalarString(v)
	}
	return ""
}

// metaBool reads a boolean from Result.Metadata.
func metaBool(m ConversationMsg, key string) bool {
	v, ok := m.Metadata[key]
	if !ok {
		return false
	}
	b, _ := v.(bool)
	return b
}

// metaKeys returns Metadata keys in sorted order, for deterministic hashing.
func metaKeys(md map[string]any) []string {
	if len(md) == 0 {
		return nil
	}
	keys := make([]string, 0, len(md))
	for k := range md {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// durationChip formats a duration for the right-aligned trailer. Running calls
// show nothing — the spinner in the status bar already conveys liveness.
func durationChip(d time.Duration) string {
	if d <= 0 {
		return ""
	}
	if d < time.Second {
		return d.Round(time.Millisecond).String()
	}
	return d.Round(10 * time.Millisecond).String()
}

// countChip renders the "×3" rollup for grouped cards.
func countChip(n int) string {
	if n <= 1 {
		return ""
	}
	return fmt.Sprintf("%s%d", glyphTimes, n)
}

// bodyLimit caps how many result rows a family shows before the "… N more"
// tail, widening under ExpandFull / review mode.
func (c *Conversation) bodyLimit(total int) int {
	if c.expandMode == ExpandFull || c.reviewMode {
		return total
	}
	return clampInt(c.height/6, 3, 8)
}

// compactOnly reports whether cards should render as a single call line.
func (c *Conversation) compactOnly() bool {
	return c.expandMode == ExpandCompact && !c.reviewMode
}

// showDetail reports whether rich detail blocks (diff, slab, table) render.
// Auto shows them; compact suppresses them.
func (c *Conversation) showDetail() bool { return !c.compactOnly() }

// join assembles a card from non-empty parts, one per line.
func join(parts ...string) string {
	kept := make([]string, 0, len(parts))
	for _, p := range parts {
		if strings.TrimSpace(ansi.Strip(p)) != "" {
			kept = append(kept, p)
		}
	}
	return strings.Join(kept, "\n")
}

// firstLines returns the first n non-blank lines of text.
func firstLines(text string, n int) ([]string, int) {
	var out []string
	total := 0
	for _, line := range strings.Split(strings.TrimSpace(text), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		total++
		if len(out) < n {
			out = append(out, strings.TrimSpace(line))
		}
	}
	return out, total - len(out)
}
