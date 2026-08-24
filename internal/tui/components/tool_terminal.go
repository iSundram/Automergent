package components

// Terminal tool cards: bash, run_command, and the shell family.
// Output renders as a tail box (last N lines on a different background)
// with an "↑ N more" scroll hint; failures keep the same shape — the red
// status in the header carries severity.

import "strings"

const terminalFamilyKey = "run"

// terminalFamily reports shell-execution tools sharing the tail box.
func terminalFamily(name string) bool {
	switch name {
	case "bash", "run_command", "read_shell", "write_shell", "stop_shell", "wait":
		return true
	}
	return false
}

// renderTerminalCard renders one terminal call: header + command line +
// boxed output tail (done/error) or spinner note while running.
func (c *Conversation) renderTerminalCard(m ConversationMsg, width int) string {
	header := c.toolHeader(m, width, 1)
	collapse := c.expandMode == ExpandCompact && !c.reviewMode

	var body strings.Builder

	if m.ToolContext != "" {
		body.WriteString("\n\n  " + c.styles.Dim.Render(ansiSafeTruncate(m.ToolContext, width-8)))
	}

	switch {
	case m.Status == "running":
		if hasSummary(m) {
			body.WriteString("\n  " + c.styles.Dim.Copy().Italic(true).Render(oneLine(m.ToolSummary)))
		}
	case collapse:
		if hasSummary(m) {
			body.WriteString("\n  " + c.styles.Dim.Copy().Italic(true).Render(oneLine(m.ToolSummary)))
		}
	default:
		if m.Content != "" {
			body.WriteString("\n\n" + indentBlock(c.outputBox(m.Content, width)))
		} else if hasSummary(m) {
			body.WriteString("\n\n  " + c.styles.Dim.Copy().Italic(true).Render(oneLine(m.ToolSummary)))
		}
	}

	_, accent, _ := c.toolBranding("bash")
	return wrapCard(accent, width, header+body.String())
}
