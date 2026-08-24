package components

// Terminal tool cards: bash, run_command, and the shell family.
// Design: one terminal slab — true-black background, "$ command" first,
// raw output beneath, scroll hint below the slab. The command is shown
// exactly once (never echoed in both header and body).

import (
	"encoding/json"
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

// terminalFamily reports shell-execution tools sharing the slab design.
func terminalFamily(name string) bool {
	switch name {
	case "bash", "run_command", "read_shell", "write_shell", "stop_shell", "wait":
		return true
	}
	return false
}

// terminalCommand extracts the executed command from args, falling back to
// the card context (stripped of any "exec: " decoration).
func terminalCommand(m ConversationMsg) string {
	var args struct {
		Command string `json:"command"`
	}
	if json.Unmarshal([]byte(m.ToolArgs), &args) == nil && strings.TrimSpace(args.Command) != "" {
		return strings.TrimSpace(args.Command)
	}
	ctx := strings.TrimSpace(m.ToolContext)
	ctx = strings.TrimPrefix(ctx, "exec: ")
	return strings.TrimSpace(ctx)
}

// renderTerminalCard renders ONLY the black slab — no card chrome, no
// header, no left border. The slab carries command, output and status;
// a scroll hint sits under it.
func (c *Conversation) renderTerminalCard(m ConversationMsg, width int) string {
	collapse := c.expandMode == ExpandCompact && !c.reviewMode

	if collapse && m.Status != "running" {
		if hasSummary(m) {
			return c.styles.Dim.Copy().Italic(true).Render(oneLine(m.ToolSummary))
		}
		if cmd := terminalCommand(m); cmd != "" {
			return c.styles.Dim.Render(ansiSafeTruncate("$ "+cmd, width-8))
		}
		return ""
	}
	return "\n" + indentBlock(c.terminalSlab(m, width))
}

// terminalSlab renders the black terminal block: "$ command" then the
// output tail (last N lines), plus a scroll hint under the slab.
func (c *Conversation) terminalSlab(m ConversationMsg, width int) string {
	inner := width - 10
	if inner < 12 {
		inner = 12
	}

	promptStyle := lipgloss.NewStyle().Foreground(c.styles.T.Green).Bold(true)
	cmdText := ansiSafeTruncate(strings.ReplaceAll(terminalCommand(m), "\n", " ⏎ "), inner-2)
	promptLine := promptStyle.Render("$") + " " +
		lipgloss.NewStyle().Foreground(c.styles.T.Text).Render(cmdText)

	// While running, the ▙ chip rides on the SAME line as the command,
	// pushed to the right edge of the slab.
	if m.Status == "running" {
		chip := lipgloss.NewStyle().Foreground(c.styles.T.Yellow).Render("running…")
		pad := inner - lipgloss.Width(promptLine) - lipgloss.Width(chip)
		if pad >= 2 {
			promptLine += strings.Repeat(" ", pad) + chip
		} else {
			promptLine += "  " + chip
		}
	}

	content := []string{promptLine}

	switch {
	case strings.TrimSpace(m.Content) != "":
		limit := c.tailLimit()
		shown, hidden := tailLines(m.Content, limit)
		for _, l := range shown {
			l = strings.TrimRight(l, " \t")
			if t := strings.TrimSpace(l); t == "" {
				content = append(content, "")
				continue
			}
			content = append(content,
				lipgloss.NewStyle().Foreground(c.styles.T.Text).Render(ansiSafeTruncate(l, inner)))
		}
		if m.IsError {
			msg := "✗ failed"
			if strings.TrimSpace(m.ToolSummary) != "" {
				msg += " — " + oneLine(m.ToolSummary)
			}
			content = append(content,
				lipgloss.NewStyle().Foreground(c.styles.T.Red).Bold(true).Render(msg))
		}
		if hidden > 0 {
			content = append(content, "") // spacer before the outside hint
			defer func() {}()             // keep structure explicit
		}
	}

	slab := lipgloss.NewStyle().
		Background(lipgloss.Color("#000000")).
		Foreground(c.styles.T.Text).
		Padding(0, 1).
		Width(inner)

	var b strings.Builder
	b.WriteString(slab.Render(strings.Join(content, "\n")))

	// Scroll hint OUTSIDE the slab so it never looks like output.
	if m.Status != "running" && strings.TrimSpace(m.Content) != "" {
		_, hidden := tailLines(m.Content, c.tailLimit())
		if hidden > 0 {
			b.WriteString("\n  " + c.styles.Dim.Render(
				fmt.Sprintf("↑ %d more lines — ctrl+e expands", hidden)))
		}
	}
	return b.String()
}
