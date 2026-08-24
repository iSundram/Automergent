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
	// Manual slab construction: lipgloss Width+Padding double-wraps long
	// output lines, so we truncate plainly and pad each row to an exact
	// visual width, painting the black background per full row.
	inner := width - 12
	if inner < 14 {
		inner = 14
	}

	bg := lipgloss.NewStyle().Background(lipgloss.Color("#000000"))
	textFg := lipgloss.NewStyle().Foreground(c.styles.T.Text)
	green := lipgloss.NewStyle().Foreground(c.styles.T.Green).Bold(true)
	yellow := lipgloss.NewStyle().Foreground(c.styles.T.Yellow) // running chip
	red := lipgloss.NewStyle().Foreground(c.styles.T.Red).Bold(true)

	// row paints one plain-text line as a full-width black row.
	row := func(s string) string {
		s = strings.TrimRight(s, " \t")
		s = ansiSafeTruncate(s, inner)
		pad := inner - lipgloss.Width(s)
		if pad < 0 {
			pad = 0
		}
		return bg.Render(" " + s + strings.Repeat(" ", pad) + " ")
	}

	plainCmd := "$ " + ansiSafeTruncate(strings.ReplaceAll(terminalCommand(m), "\n", " ⏎ "), inner-4)

	var rows []string

	if m.Status == "running" {
		chip := "running…"
		cmdVis := strings.ReplaceAll(terminalCommand(m), "\n", " ⏎ ")
		left := green.Render("$") + " " + textFg.Render(ansiSafeTruncate(cmdVis, inner-6))
		right := yellow.Render(chip)
		pad := inner - 2 - lipgloss.Width(left) - lipgloss.Width(right)
		if pad < 1 {
			pad = 1
		}
		rows = append(rows, bg.Render(" "+left+strings.Repeat(" ", pad)+right+" "))
	} else {
		rows = append(rows, row(plainCmd))
	}

	if strings.TrimSpace(m.Content) != "" {
		limit := c.tailLimit()
		shown, hidden := tailLines(m.Content, limit)
		for _, l := range shown {
			rows = append(rows, row(l))
		}
		if m.IsError {
			msg := "✗ failed"
			if strings.TrimSpace(m.ToolSummary) != "" {
				msg += " — " + oneLine(m.ToolSummary)
			}
			rows = append(rows, row(msg))
			_ = red
		}
		if hidden > 0 {
			rows = append(rows, "")
		}
	}

	body := bg.Render("")
	painted := make([]string, 0, len(rows))
	for _, r := range rows {
		if r == "" {
			painted = append(painted, bg.Render(strings.Repeat(" ", inner+2)))
			continue
		}
		painted = append(painted, r)
	}
	body = strings.Join(painted, "\n")

	out := "\n" + indentBlock(body)

	if m.Status != "running" && strings.TrimSpace(m.Content) != "" {
		_, hidden := tailLines(m.Content, c.tailLimit())
		if hidden > 0 {
			out += "\n" + indentBlock(c.styles.Dim.Render(
				fmt.Sprintf("↑ %d more lines — ctrl+e expands", hidden)))
		}
	}
	return out
}

func max0(v int) int {
	if v < 0 {
		return 0
	}
	return v
}
