package components

// File-change tool cards: edit_file, write_file, create_file, multi_edit.
// Previews render through the shared diff box (tail + "+a −r" stats).

import (
	"encoding/json"
	"fmt"
	"strings"
)

// isFileEditTool reports file-mutation tools (multi_edit included).
func isFileEditTool(name string) bool {
	return name == "write_file" || name == "edit_file" ||
		name == "create_file" || name == "multi_edit"
}

// editDiffText extracts diff-ish preview text from a message.
// - running proposals: Content already carries the proposed diff
// - done write/create: Content is the applied-diff summary
func editDiffText(m ConversationMsg) string {
	if strings.TrimSpace(m.Content) == "" {
		return ""
	}
	// Heuristic: real diffs contain +/- lines; plain summaries don't.
	hasDiffLines := false
	for _, l := range strings.Split(m.Content, "\n") {
		t := strings.TrimLeft(l, " ")
		if strings.HasPrefix(t, "+") || strings.HasPrefix(t, "-") {
			hasDiffLines = true
			break
		}
	}
	if !hasDiffLines {
		return ""
	}
	return m.Content
}

// renderEditCard renders one file-change tool call.
func (c *Conversation) renderEditCard(m ConversationMsg, width int) string {
	header := c.toolHeader(m, width, 1)
	collapse := c.expandMode == ExpandCompact && !c.reviewMode
	details := c.expandMode == ExpandFull || c.reviewMode

	var body strings.Builder

	switch {
	case m.Status == "running" && editDiffText(m) != "":
		body.WriteString("\n\n")
		body.WriteString(indentBlock(c.diffBox(editDiffText(m), width)))

	case collapse:
		if hasSummary(m) {
			body.WriteString("\n\n  " + c.styles.Dim.Copy().Italic(true).Render(oneLine(m.ToolSummary)))
		}
		return c.wrapEdit(width, m, header+body.String())

	default:
		// Key params row for edits: show old→new sizes when available.
		var args struct {
			OldStr     string `json:"old_str"`
			NewStr     string `json:"new_str"`
			Edits      []any  `json:"edits"`
			ReplaceAll bool   `json:"replace_all"`
		}
		if json.Unmarshal([]byte(m.ToolArgs), &args) == nil {
			var param string
			switch {
			case len(args.Edits) > 0:
				param = plural(len(args.Edits), "edit")
			case args.OldStr != "":
				param = plural(strings.Count(args.OldStr, "\n")+1, "line replaced")
				if args.ReplaceAll {
					param += ", all occurrences"
				}
			}
			if param != "" {
				body.WriteString("\n\n  " + c.styles.Dim.Render(param))
			}
		}
		if hasSummary(m) {
			body.WriteString("\n  " + c.styles.Dim.Copy().Italic(true).Render(oneLine(m.ToolSummary)))
		}
		if d := editDiffText(m); d != "" && details {
			body.WriteString("\n\n" + indentBlock(c.diffBox(d, width)))
		}
	}

	return c.wrapEdit(width, m, header+body.String())
}

// wrapEdit frames an edit card with the green accent border.
func (c *Conversation) wrapEdit(width int, m ConversationMsg, content string) string {
	_, accent, _ := c.toolBranding("write_file")
	if m.IsError || m.Status == "error" {
		_, accent, _ = c.toolBranding("delete_file")
	}
	return wrapCard(accent, width, content)
}

func indentBlock(s string) string { return "  " + s }

func plural(n int, singular string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, singular)
	}
	return fmt.Sprintf("%d %ss", n, singular)
}
