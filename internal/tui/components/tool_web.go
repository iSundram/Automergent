package components

// Web family: web_fetch and web_search.
//
// A fetch is a URL plus a transfer receipt; a search is a URL plus a ranked
// list. Both compress to a call line and a handful of rows — the fetched body
// itself is the model's business, not the log's.

import (
	"fmt"
	"net/url"
	"strings"
)

// renderWebCard dispatches within the web family.
func (c *Conversation) renderWebCard(m ConversationMsg, width int) string {
	if m.ToolName == "web_search" {
		return c.renderWebSearchCard(m, width)
	}
	return c.renderWebFetchCard(m, width)
}

// renderWebFetchCard renders web_fetch.
//
//	● Fetch  go.dev/doc/effective_go  ·  84 KB · text/html               1.4s
//	  ⎿ Effective Go — The Go Programming Language
func (c *Conversation) renderWebFetchCard(m ConversationMsg, width int) string {
	raw := subjectFor(m)
	head := c.callLine(m, width, prettyURL(raw), c.fetchChips(m), durationChip(m.Duration))

	if m.IsError {
		return join(head, c.resultRow(c.severityMark("error"), oneLine(m.Content), width))
	}
	if !c.showDetail() || strings.TrimSpace(m.Content) == "" {
		return head
	}
	// A page title is the single most useful thing to echo back.
	if title := htmlTitle(m.Content); title != "" {
		return join(head, c.resultRow("", title, width))
	}
	lines, _ := firstLines(m.Content, 1)
	if len(lines) > 0 {
		return join(head, c.resultRow("", lines[0], width))
	}
	return head
}

// fetchChips reports the transfer size and any HTTP error status.
func (c *Conversation) fetchChips(m ConversationMsg) []string {
	var chips []string
	if m.Status == "running" {
		return nil
	}
	if status := httpStatus(m.Content); status != "" {
		chips = append(chips, status)
	}
	if n := len(m.Content); n > 0 && !m.IsError {
		chips = append(chips, byteSize(n))
	}
	return chips
}

// renderWebSearchCard renders web_search: one row per result, title over URL.
//
//	● Web search  "lipgloss ansi width"  ·  8 results                    0.9s
//	  ⎿ charmbracelet/lipgloss — GitHub
//	  ⎿ Lipgloss docs
func (c *Conversation) renderWebSearchCard(m ConversationMsg, width int) string {
	subject := subjectFor(m)
	if subject != "" {
		subject = `"` + subject + `"`
	}

	results, _ := firstLines(m.Content, 1<<20)
	var chips []string
	if m.Status != "running" && !m.IsError && len(results) > 0 {
		chips = append(chips, plural(len(results), "result"))
	}
	head := c.callLine(m, width, subject, chips, durationChip(m.Duration))

	if m.IsError {
		return join(head, c.resultRow(c.severityMark("error"), oneLine(m.Content), width))
	}
	if !c.showDetail() || len(results) == 0 {
		return head
	}

	limit := c.bodyLimit(len(results))
	rows := make([]string, 0, limit+1)
	for i, r := range results {
		if i >= limit {
			break
		}
		rows = append(rows, c.resultRow("", r, width))
	}
	if more := len(results) - limit; more > 0 {
		rows = append(rows, c.moreRow(more, "result"))
	}
	return join(head, strings.Join(rows, "\n"))
}

// prettyURL drops the scheme and any trailing slash so the call line spends
// its width on the path rather than on "https://".
func prettyURL(raw string) string {
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return raw
	}
	out := u.Host + u.Path
	if u.RawQuery != "" {
		out += "?" + u.RawQuery
	}
	return strings.TrimSuffix(out, "/")
}

// httpStatus recovers the status line from an error-shaped fetch result, which
// the tool formats as "HTTP 404 Not Found\n<body>".
func httpStatus(content string) string {
	first := content
	if i := strings.IndexByte(content, '\n'); i >= 0 {
		first = content[:i]
	}
	if strings.HasPrefix(first, "HTTP ") {
		return strings.TrimSpace(strings.TrimPrefix(first, "HTTP "))
	}
	return ""
}

// htmlTitle extracts <title> from fetched markup, if present.
func htmlTitle(content string) string {
	lower := strings.ToLower(content)
	start := strings.Index(lower, "<title")
	if start < 0 {
		return ""
	}
	open := strings.IndexByte(content[start:], '>')
	if open < 0 {
		return ""
	}
	start += open + 1
	end := strings.Index(lower[start:], "</title>")
	if end < 0 {
		return ""
	}
	return strings.TrimSpace(oneLine(content[start : start+end]))
}

// byteSize formats a transfer size for a chip.
func byteSize(n int) string {
	switch {
	case n < 1024:
		return fmt.Sprintf("%d B", n)
	case n < 1024*1024:
		return fmt.Sprintf("%.1f KB", float64(n)/1024)
	default:
		return fmt.Sprintf("%.1f MB", float64(n)/(1024*1024))
	}
}
