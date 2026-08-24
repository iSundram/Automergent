package components

// Read-family tool cards: read_file, view, list_directory, glob, grep,
// search. Reads are ALWAYS one line per file — even untruncated — with
// line-range and size context; consecutive reads collapse into a single
// "Read N files" card.

import (
	"encoding/json"
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

// readFamily reports whether a tool renders as a one-line read row.
func readFamily(name string) bool {
	switch name {
	case "read_file", "view", "list_directory", "glob", "grep", "search":
		return true
	}
	return false
}

// groupKeyFor collapses same-FAMILY consecutive calls into grouped cards
// (reads across read_file/view/list_directory merge; everything else groups
// only on identical tool names).
func groupKeyFor(name string) string {
	if readFamily(name) {
		return "read"
	}
	if isFileEditTool(name) || name == "multi_edit" {
		return "edit"
	}
	if name == "bash" || name == "run_command" {
		return "run"
	}
	return name
}

// readRow describes one rendered read entry.
type readRow struct {
	label  string
	ranges string // "L40-80"
	size   string // "41 lines"
	err    bool
}

// readRowsFor extracts display rows from one read message.
func readRowsFor(m ConversationMsg) []readRow {
	row := readRow{label: m.ToolContext}
	if row.label == "" {
		row.label = oneLine(m.ToolSummary)
	}
	var args struct {
		StartLine int `json:"start_line"`
		EndLine   int `json:"end_line"`
		Path      string
	}
	if m.ToolArgs != "" && json.Unmarshal([]byte(m.ToolArgs), &args) == nil && args.Path != "" && row.label == "" {
		row.label = args.Path
	}
	if args.StartLine > 0 || args.EndLine > 0 {
		switch {
		case args.EndLine > 0:
			row.ranges = fmt.Sprintf("L%d–%d", max(1, args.StartLine), args.EndLine)
		case args.StartLine > 0:
			row.ranges = fmt.Sprintf("L%d→", args.StartLine)
		}
	}
	if n := strings.Count(m.Content, "\n") + 1; m.Content != "" && !m.IsError {
		row.size = fmt.Sprintf("%d lines", n)
	} else if m.IsError {
		row.err = true
		row.size = oneLine(m.Content)
	}
	return []readRow{row}
}

// renderReadGroup renders the whole read family: header says
// "Read file" / "Read N files"; body is one line per file.
func (c *Conversation) renderReadGroup(group []ConversationMsg, width int) string {
	first := group[0]
	files := 0
	for _, g := range group {
		if g.ToolName == "read_file" || g.ToolName == "view" {
			files++
		}
	}

	rollup := first
	for _, g := range group {
		rollup.Duration += g.Duration
	}
	header := c.toolHeader(rollup, width, len(group))

	var body strings.Builder
	limit := 6
	if c.expandMode == ExpandFull || c.reviewMode {
		limit = len(group)
	} else if c.expandMode == ExpandAuto && len(group) > limit {
		limit = 4
	}
	shown := 0
	for i, g := range group {
		for _, row := range readRowsFor(g) {
			if shown >= limit {
				break
			}
			mark := ""
			if row.err || g.IsError {
				mark = lipgloss.NewStyle().Foreground(c.styles.T.Red).Render("✗ ")
			}
			parts := []string{ansiSafeTruncate(row.label, width-24)}
			if row.ranges != "" {
				parts = append(parts, c.styles.Dim.Render(row.ranges))
			}
			if row.size != "" {
				parts = append(parts, c.styles.Dim.Render(row.size))
			}
			body.WriteString("\n  " + mark + strings.Join(parts, " · "))
			shown++
		}
		if i < len(group)-1 && strings.Contains(g.ToolContext, ",") {
			continue
		}
	}
	if hidden := len(group) - shown; hidden > 0 {
		body.WriteString(fmt.Sprintf("\n  %s", c.styles.Dim.Render(fmt.Sprintf("… %d more files", hidden))))
	}

	_, accent, _ := c.toolBranding(first.ToolName)
	return wrapCard(accent, width, header+body.String())
}
