package components

// Inspection families: diagnostics (lsp_diagnostics), security (secrets_scan,
// dependency_audit) and data (sql).
//
// These tools all answer "what's wrong / what's there", so they share one
// shape: a severity-counted headline and severity-marked rows. A clean result
// is a single green line — an all-clear should cost one row, not ten.

import (
	"fmt"
	"regexp"
	"strings"
)

// renderDiagnosticsCard renders lsp_diagnostics.
//
//	● Diagnostics  internal/tui  ·  2 errors                             0.7s
//	  ⎿ ✗ tool_box.go:44:6  declared and not used: limit
//
// A clean file collapses to one line with a green check.
func (c *Conversation) renderDiagnosticsCard(m ConversationMsg, width int) string {
	subject := subjectFor(m)

	if m.Status != "running" && diagnosticsClean(m.Content) {
		head := c.callLine(m, width, subject,
			[]string{c.severityMark("clean") + " clean"}, durationChip(m.Duration))
		return head
	}

	findings := parseDiagnostics(m.Content)
	errs, warns := 0, 0
	for _, f := range findings {
		if f.severity == "warn" {
			warns++
		} else {
			errs++
		}
	}
	var chips []string
	if errs > 0 {
		chips = append(chips, plural(errs, "error"))
	}
	if warns > 0 {
		chips = append(chips, plural(warns, "warning"))
	}

	head := c.callLine(m, width, subject, chips, durationChip(m.Duration))
	if !c.showDetail() || len(findings) == 0 {
		if len(findings) == 0 && c.showDetail() && strings.TrimSpace(m.Content) != "" {
			return join(head, c.resultRow("", oneLine(m.Content), width))
		}
		return head
	}

	limit := c.bodyLimit(len(findings))
	rows := make([]string, 0, limit+1)
	for i, f := range findings {
		if i >= limit {
			break
		}
		rows = append(rows, c.resultRow(c.severityMark(f.severity),
			f.location+"  "+f.message, width))
	}
	if more := len(findings) - limit; more > 0 {
		rows = append(rows, c.moreRow(more, "diagnostic"))
	}
	return join(head, strings.Join(rows, "\n"))
}

// diagnosticsClean recognises the tool's all-clear responses.
func diagnosticsClean(content string) bool {
	t := strings.ToLower(strings.TrimSpace(content))
	if t == "" {
		return false
	}
	return strings.HasPrefix(t, "no compile errors") ||
		strings.HasPrefix(t, "no diagnostics") ||
		strings.HasPrefix(t, "no errors")
}

// diagnostic is one parsed compiler/LSP finding.
type diagnostic struct {
	severity string
	location string
	message  string
}

// diagLocation matches "path/file.go:12:5:" and "path/file.go:12:" prefixes,
// which is how both `go build` and gopls format findings.
var diagLocation = regexp.MustCompile(`^([^\s:]+):(\d+)(?::(\d+))?:\s*(.*)$`)

// parseDiagnostics folds compiler output into severity-marked findings.
func parseDiagnostics(content string) []diagnostic {
	lines, _ := firstLines(content, 1<<20)
	var out []diagnostic
	for _, line := range lines {
		if strings.HasPrefix(line, glyphMore) || strings.HasPrefix(line, "...") {
			continue
		}
		mm := diagLocation.FindStringSubmatch(line)
		if mm == nil {
			continue
		}
		loc := mm[1] + ":" + mm[2]
		if mm[3] != "" {
			loc += ":" + mm[3]
		}
		message := mm[4]
		severity := "error"
		if lower := strings.ToLower(message); strings.HasPrefix(lower, "warning") ||
			strings.Contains(lower, "unreachable") || strings.Contains(lower, "deprecated") {
			severity = "warn"
		}
		out = append(out, diagnostic{severity: severity, location: loc, message: message})
	}
	return out
}

// renderSecurityCard renders secrets_scan / dependency_audit.
//
//	● Secrets scan  internal/  ·  2 findings                             1.1s
//	  ⎿ ✗ config/loader.go:88  aws_access_key
//	● Audit  go.mod  ·  ✓ 0 advisories                                   2.3s
func (c *Conversation) renderSecurityCard(m ConversationMsg, width int) string {
	subject := subjectFor(m)
	count, hasCount := metaInt(m, "findings_count")

	// The scanner returns a metadata count; a zero count is genuinely good news
	// and deserves the green single-line treatment.
	if m.Status != "running" && !m.IsError && hasCount && count == 0 {
		return c.callLine(m, width, subject,
			[]string{c.severityMark("clean") + " no findings"}, durationChip(m.Duration))
	}
	if m.Status != "running" && !m.IsError && !hasCount && securityClean(m.Content) {
		return c.callLine(m, width, subject,
			[]string{c.severityMark("clean") + " clean"}, durationChip(m.Duration))
	}

	findings := parseSecurityFindings(m.Content)
	var chips []string
	switch {
	case hasCount:
		chips = append(chips, plural(count, "finding"))
	case len(findings) > 0:
		chips = append(chips, plural(len(findings), "finding"))
	}

	head := c.callLine(m, width, subject, chips, durationChip(m.Duration))
	if m.IsError {
		return join(head, c.resultRow(c.severityMark("error"), oneLine(m.Content), width))
	}
	if !c.showDetail() || len(findings) == 0 {
		return head
	}

	limit := c.bodyLimit(len(findings))
	rows := make([]string, 0, limit+1)
	for i, f := range findings {
		if i >= limit {
			break
		}
		rows = append(rows, c.resultRow(c.severityMark("error"), f, width))
	}
	if more := len(findings) - limit; more > 0 {
		rows = append(rows, c.moreRow(more, "finding"))
	}
	return join(head, strings.Join(rows, "\n"))
}

// securityClean recognises the scanners' all-clear strings.
func securityClean(content string) bool {
	t := strings.ToLower(strings.TrimSpace(content))
	return strings.Contains(t, "no secrets detected") ||
		strings.Contains(t, "no vulnerabilities") ||
		strings.Contains(t, "no advisories")
}

// parseSecurityFindings pulls the scanner's "• file:line - type" bullet rows.
// The masked secret sits on the following continuation line, which is dropped:
// the log should say where the finding is, not reprint even a masked secret.
func parseSecurityFindings(content string) []string {
	lines, _ := firstLines(content, 1<<20)
	var out []string
	for _, line := range lines {
		if !strings.HasPrefix(line, "•") {
			continue
		}
		out = append(out, strings.TrimSpace(strings.TrimPrefix(line, "•")))
	}
	return out
}

// renderDataCard renders sql: the statement on the call line, rows in a table.
//
//	● SQL  SELECT id, name FROM users LIMIT 3                            12ms
//	  ID  NAME
//	  1   ada
//	  3 rows
func (c *Conversation) renderDataCard(m ConversationMsg, width int) string {
	head := c.callLine(m, width, collapseSQL(subjectFor(m)), c.sqlChips(m), durationChip(m.Duration))

	if m.IsError {
		return join(head, c.resultRow(c.severityMark("error"), oneLine(m.Content), width))
	}
	if !c.showDetail() || strings.TrimSpace(m.Content) == "" {
		return head
	}
	if headers, rows, ok := parseSQLTable(m.Content); ok {
		body := c.table(headers, rows, width)
		return join(head, body)
	}
	// Non-tabular results ("3 row(s) updated.", "(no results)") are one row.
	return join(head, c.resultRow("", oneLine(m.Content), width))
}

// sqlChips reports the row count for a result set.
func (c *Conversation) sqlChips(m ConversationMsg) []string {
	if m.Status == "running" || m.IsError {
		return nil
	}
	if _, rows, ok := parseSQLTable(m.Content); ok {
		return []string{plural(len(rows), "row")}
	}
	return nil
}

// collapseSQL flattens a multi-line statement onto one line so the call line
// stays one line regardless of how the query was formatted.
func collapseSQL(q string) string {
	return strings.Join(strings.Fields(q), " ")
}

// parseSQLTable reads the tool's pipe-free column output: a header line then
// value lines. Returns ok=false for scalar or empty results.
func parseSQLTable(content string) (headers []string, rows [][]string, ok bool) {
	lines, _ := firstLines(content, 200)
	if len(lines) < 2 {
		return nil, nil, false
	}
	sep := " | "
	if !strings.Contains(lines[0], sep) {
		return nil, nil, false
	}
	headers = strings.Split(lines[0], sep)
	for _, line := range lines[1:] {
		if strings.HasPrefix(line, "---") {
			continue
		}
		rows = append(rows, strings.Split(line, sep))
	}
	if len(rows) == 0 {
		return nil, nil, false
	}
	return headers, rows, true
}

// countSuffix renders "(N)" for headline chips where a bare number would read
// ambiguously next to a path.
func countSuffix(n int) string { return fmt.Sprintf("(%d)", n) }
