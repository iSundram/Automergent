package components

// Discovery families: read (read_file, view), list (list_directory, glob) and
// search (grep, search).
//
// These are the highest-frequency tools in any session, so they are the most
// aggressively compressed: a read is ALWAYS one line, even when it returned
// 500 lines, because what matters in the log is which file was read and how
// much of it — not the contents, which the model already has.

import (
	"fmt"
	"path/filepath"
	"strings"
)

// renderReadCard renders one read_file / view call.
//
//	● Read  internal/tui/components/tool_box.go · L40–160 · 121 lines    12ms
func (c *Conversation) renderReadCard(m ConversationMsg, width int) string {
	return c.callLine(m, width, subjectFor(m), c.readChips(m), durationChip(m.Duration))
}

// readChips builds the "L40–160 · 121 lines" trailer chips for a read.
func (c *Conversation) readChips(m ConversationMsg) []string {
	var chips []string
	if r := lineRange(m); r != "" {
		chips = append(chips, r)
	}
	switch {
	case m.IsError:
		chips = append(chips, oneLine(m.Content))
	case strings.TrimSpace(m.Content) != "":
		chips = append(chips, plural(strings.Count(strings.TrimRight(m.Content, "\n"), "\n")+1, "line"))
	}
	return chips
}

// lineRange formats the requested slice of a file, covering both the
// start_line/end_line pair (read_file) and the view_range array (view).
func lineRange(m ConversationMsg) string {
	args := argsOf(m)
	if args == nil {
		return ""
	}
	start, end := 0, 0
	if v, ok := args["start_line"]; ok {
		start = intOf(v)
	}
	if v, ok := args["end_line"]; ok {
		end = intOf(v)
	}
	if vr, ok := args["view_range"].([]any); ok && len(vr) == 2 {
		start, end = intOf(vr[0]), intOf(vr[1])
	}
	switch {
	case start > 0 && end > 0:
		return fmt.Sprintf("L%d–%d", start, end)
	case start > 0:
		return fmt.Sprintf("L%d%s", start, glyphTo)
	case end > 0:
		return fmt.Sprintf("L1–%d", end)
	}
	return ""
}

func intOf(v any) int {
	switch t := v.(type) {
	case int:
		return t
	case int64:
		return int(t)
	case float64:
		return int(t)
	}
	return 0
}

// renderReadGroup collapses a run of reads into one card: the headline counts
// the files, and each file gets exactly one row.
//
//	● Read  3 files                                                     38ms
//	  ⎿ tool_box.go · 132 lines
//	  ⎿ tool_read.go · L1–60 · 60 lines
func (c *Conversation) renderReadGroup(group []ConversationMsg, width int) string {
	if len(group) == 1 {
		return c.renderReadCard(group[0], width)
	}
	rollup := group[0]
	rollup.Duration = 0
	errs := 0
	for _, g := range group {
		rollup.Duration += g.Duration
		if g.IsError {
			errs++
		}
	}
	if errs > 0 {
		rollup.IsError = true
		rollup.Status = "error"
	}

	chips := []string{plural(len(group), "file")}
	if errs > 0 {
		chips = append(chips, plural(errs, "failed"))
	}
	head := c.callLine(rollup, width, "", chips, durationChip(rollup.Duration))
	if c.compactOnly() {
		return head
	}

	limit := c.bodyLimit(len(group))
	rows := make([]string, 0, limit+1)
	for i, g := range group {
		if i >= limit {
			break
		}
		mark := ""
		if g.IsError {
			mark = c.severityMark("error")
		}
		label := filepath.Base(subjectFor(g))
		if chips := c.readChips(g); len(chips) > 0 {
			label += glyphSep + strings.Join(chips, glyphSep)
		}
		rows = append(rows, c.resultRow(mark, label, width))
	}
	if more := len(group) - limit; more > 0 {
		rows = append(rows, c.moreRow(more, "file"))
	}
	return join(head, strings.Join(rows, "\n"))
}

// renderListCard renders list_directory / glob.
//
//	● Glob  **/*.go  ·  248 files                                       31ms
//	  ⎿ cmd/automergent/main.go
//	    … 246 more entries
func (c *Conversation) renderListCard(m ConversationMsg, width int) string {
	chips := c.listChips(m)
	head := c.callLine(m, width, subjectFor(m), chips, durationChip(m.Duration))
	if c.compactOnly() || m.Status == "running" || m.IsError {
		if m.IsError {
			return join(head, c.resultRow(c.severityMark("error"), oneLine(m.Content), width))
		}
		return head
	}

	entries, hidden := firstLines(m.Content, c.bodyLimit(0))
	rows := make([]string, 0, len(entries)+1)
	for _, e := range entries {
		if strings.HasPrefix(e, glyphMore) || strings.HasPrefix(e, "...") {
			continue
		}
		rows = append(rows, c.resultRow("", e, width))
	}
	if hidden > 0 {
		rows = append(rows, c.moreRow(hidden, "entry"))
	}
	return join(head, strings.Join(rows, "\n"))
}

// listChips prefers the tool's own metadata count over counting lines.
func (c *Conversation) listChips(m ConversationMsg) []string {
	if n, ok := metaInt(m, "count"); ok {
		chips := []string{plural(n, "file")}
		if metaBool(m, "truncated") {
			chips = append(chips, "truncated")
		}
		return chips
	}
	if strings.TrimSpace(m.Content) == "" || m.Status == "running" {
		return nil
	}
	if strings.HasPrefix(strings.TrimSpace(m.Content), "no matches") {
		return []string{"no matches"}
	}
	_, total := firstLines(m.Content, 0)
	return []string{plural(total, "entry")}
}

// renderSearchCard renders grep / search: the headline carries the match and
// file counts from metadata, the body one row per matched file with a hit
// count, so a 400-match grep stays four lines tall.
//
//	● Grep  "renderToolCall"  ·  12 matches in 4 files                  0.3s
//	  ⎿ conversation_tools.go:109   ×6
func (c *Conversation) renderSearchCard(m ConversationMsg, width int) string {
	subject := subjectFor(m)
	if subject != "" {
		subject = `"` + subject + `"`
	}
	head := c.callLine(m, width, subject, c.searchChips(m), durationChip(m.Duration))
	if c.compactOnly() || m.Status == "running" {
		return head
	}
	if m.IsError {
		return join(head, c.resultRow(c.severityMark("error"), oneLine(m.Content), width))
	}

	hits := searchHits(m.Content)
	if len(hits) == 0 {
		return head
	}
	limit := c.bodyLimit(len(hits))
	rows := make([]string, 0, limit+1)
	for i, h := range hits {
		if i >= limit {
			break
		}
		label := h.location
		if h.count > 1 {
			label += "  " + countChip(h.count)
		}
		rows = append(rows, c.resultRow("", label, width))
	}
	if more := len(hits) - limit; more > 0 {
		rows = append(rows, c.moreRow(more, "file"))
	}
	return join(head, strings.Join(rows, "\n"))
}

// searchChips reports match totals, preferring metadata to line counting.
func (c *Conversation) searchChips(m ConversationMsg) []string {
	if m.Status == "running" {
		return nil
	}
	matches, hasMatches := metaInt(m, "total_matches")
	files, hasFiles := metaInt(m, "files_matched")
	if hasMatches && matches == 0 {
		return []string{"no matches"}
	}
	var chips []string
	switch {
	case hasMatches && hasFiles:
		chips = append(chips, fmt.Sprintf("%s in %s",
			plural(matches, "match"), plural(files, "file")))
	case hasMatches:
		chips = append(chips, plural(matches, "match"))
	case strings.HasPrefix(strings.TrimSpace(m.Content), "no matches"):
		chips = append(chips, "no matches")
	}
	if metaBool(m, "truncated") {
		chips = append(chips, "truncated")
	}
	return chips
}

// searchHit is one matched file with its first location and hit count.
type searchHit struct {
	location string
	count    int
}

// searchHits folds "path:line:text" output into one entry per file. Grep
// output modes vary (content / files_with_matches / count), so anything that
// does not parse as a location is passed through as its own row.
func searchHits(content string) []searchHit {
	lines, _ := firstLines(content, 1<<20)
	order := make([]string, 0, len(lines))
	seen := make(map[string]int, len(lines))
	first := make(map[string]string, len(lines))

	for _, line := range lines {
		if strings.HasPrefix(line, glyphMore) || strings.HasPrefix(line, "...") {
			continue
		}
		file, loc := splitLocation(line)
		if _, ok := seen[file]; !ok {
			order = append(order, file)
			first[file] = loc
		}
		seen[file]++
	}

	out := make([]searchHit, 0, len(order))
	for _, f := range order {
		out = append(out, searchHit{location: first[f], count: seen[f]})
	}
	return out
}

// splitLocation extracts (file, "file:line") from a grep-style output row.
// Rows without a parseable location return themselves as both values, so they
// still render as a distinct row.
func splitLocation(line string) (file, location string) {
	parts := strings.SplitN(line, ":", 3)
	if len(parts) < 2 {
		return line, line
	}
	path := parts[0]
	if path == "" || strings.ContainsAny(path, " \t") {
		return line, line
	}
	lineNo := strings.TrimSpace(parts[1])
	if lineNo == "" || strings.IndexFunc(lineNo, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
		return path, path
	}
	return path, filepath.Base(path) + ":" + lineNo
}
