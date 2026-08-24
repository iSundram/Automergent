package components

// Execution families: terminal (bash, run_command), shellsession (read_shell,
// write_shell, stop_shell, wait) and shelllist (list_shells).
//
// The slab is the signature block here: a dark full-bleed surface carrying
// "$ command" then the raw output tail, so a shell transcript reads like a
// terminal instead of like prose. The command appears exactly once — on the
// slab's first row, never echoed in both the call line and the body.

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

// renderTerminalCard renders bash / run_command: a call line, then the slab.
//
//	● Bash  go test ./...                                               4.2s
//	  ▓ $ go test ./...                                    ▓
//	  ▓ ok  internal/tui/components   0.412s                ▓
//	  ↑ 12 more lines — ctrl+e expands
func (c *Conversation) renderTerminalCard(m ConversationMsg, width int) string {
	cmd := terminalCommand(m)
	head := c.callLine(m, width, cmd, c.terminalChips(m), durationChip(m.Duration))

	if !c.showDetail() {
		return head
	}
	if strings.TrimSpace(m.Content) == "" && m.Status != "running" {
		return head
	}
	return join(head, c.terminalSlab(m, width))
}

// terminalChips reports the async-vs-sync shape and outcome of a command.
func (c *Conversation) terminalChips(m ConversationMsg) []string {
	var chips []string
	if id := metaString(m, "shell_id"); id != "" {
		chips = append(chips, id)
		if metaBool(m, "detached") {
			chips = append(chips, "detached")
		} else {
			chips = append(chips, "async")
		}
	}
	if m.IsError {
		chips = append(chips, "failed")
	}
	return chips
}

// terminalCommand extracts the executed command, falling back to the event
// context stripped of its "exec: " decoration.
func terminalCommand(m ConversationMsg) string {
	if args := argsOf(m); args != nil {
		if cmd, ok := args["command"].(string); ok && strings.TrimSpace(cmd) != "" {
			return strings.TrimSpace(cmd)
		}
	}
	return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(m.ToolContext), "exec: "))
}

// terminalSlab paints the dark command block: "$ command" first, then the last
// N lines of output, with a scroll hint below the slab when lines were hidden.
func (c *Conversation) terminalSlab(m ConversationMsg, width int) string {
	inner := slabInner(width)

	textFg := lipgloss.NewStyle().Foreground(c.styles.T.Text)
	prompt := lipgloss.NewStyle().Foreground(c.styles.T.Green).Bold(true)
	bg := lipgloss.NewStyle().Background(c.slabBackground())

	// Multi-line commands collapse onto one row with a visible ⏎ so the slab
	// keeps a stable height regardless of how the model formatted the command.
	cmdVis := strings.ReplaceAll(terminalCommand(m), "\n", " "+glyphReturn+" ")

	var rows []string
	if m.Status == "running" {
		// The running chip is right-aligned on the command row itself, so an
		// in-flight command needs no extra line.
		chip := lipgloss.NewStyle().Foreground(c.styles.T.Yellow).Render("running…")
		left := prompt.Render("$") + " " + textFg.Render(ansiSafeTruncate(cmdVis, inner-12))
		pad := inner - lipgloss.Width(left) - lipgloss.Width(chip)
		if pad < 1 {
			pad = 1
		}
		rows = append(rows, bg.Render(" "+left+strings.Repeat(" ", pad)+chip+" "))
	} else {
		rows = append(rows, c.slabRow("$ "+cmdVis, inner))
	}

	hidden := 0
	if strings.TrimSpace(m.Content) != "" {
		var shown []string
		shown, hidden = tailLines(m.Content, c.tailLimit())
		for _, l := range shown {
			rows = append(rows, c.slabRow(l, inner))
		}
	}

	out := c.slab(rows, width)
	if m.Status != "running" {
		if hint := c.hintRow(hidden, "lines"); hint != "" {
			out += "\n" + hint
		}
	}
	return out
}

// renderShellSessionCard renders read_shell / write_shell / stop_shell / wait —
// operations against an already-running shell, keyed by shell id.
//
//	● Read shell  sh_3  ·  48 lines                                      6ms
//	  ▓ …output tail… ▓
//	● Write shell  sh_3  →  "y"                                          1ms
func (c *Conversation) renderShellSessionCard(m ConversationMsg, width int) string {
	args := argsOf(m)
	subject := subjectFor(m)
	var chips []string

	switch m.ToolName {
	case "write_shell":
		if in, ok := args["input"].(string); ok && in != "" {
			subject += "  " + glyphTo + `  "` + oneLine(strings.TrimRight(in, "\n")) + `"`
		}
	case "wait":
		if s := scalarString(args["seconds"]); s != "" {
			subject = s + "s"
		}
	}
	if status := metaString(m, "status"); status != "" {
		chips = append(chips, status)
	}
	if code := metaString(m, "exit_code"); code != "" {
		chips = append(chips, "exit "+code)
	}
	if m.ToolName == "read_shell" && strings.TrimSpace(m.Content) != "" {
		_, total := firstLines(m.Content, 0)
		chips = append(chips, plural(total, "line"))
	}

	head := c.callLine(m, width, subject, chips, durationChip(m.Duration))
	if m.IsError {
		return join(head, c.resultRow(c.severityMark("error"), oneLine(m.Content), width))
	}
	// Only read_shell has output worth slabbing; the rest are control calls.
	if m.ToolName != "read_shell" || !c.showDetail() || strings.TrimSpace(m.Content) == "" {
		return head
	}

	inner := slabInner(width)
	shown, hidden := tailLines(m.Content, c.tailLimit())
	rows := make([]string, 0, len(shown))
	for _, l := range shown {
		rows = append(rows, c.slabRow(l, inner))
	}
	out := join(head, c.slab(rows, width))
	if hint := c.hintRow(hidden, "lines"); hint != "" {
		out += "\n" + hint
	}
	return out
}

// renderShellListCard renders list_shells as a table of live sessions.
//
//	● Shells  2 active
//	  ID    STATUS   PID     COMMAND
//	  sh_3  running  48213   npm run dev
func (c *Conversation) renderShellListCard(m ConversationMsg, width int) string {
	sessions := parseShellRows(m.Content)
	chips := []string{}
	if len(sessions) > 0 {
		chips = append(chips, plural(len(sessions), "session"))
	}
	head := c.callLine(m, width, "", chips, durationChip(m.Duration))

	if m.IsError {
		return join(head, c.resultRow(c.severityMark("error"), oneLine(m.Content), width))
	}
	if !c.showDetail() || len(sessions) == 0 {
		if len(sessions) == 0 && strings.TrimSpace(m.Content) != "" && c.showDetail() {
			return join(head, c.resultRow("", oneLine(m.Content), width))
		}
		return head
	}
	return join(head, c.table([]string{"id", "status", "command"}, sessions, width))
}

// parseShellRows pulls id/status/command triples out of list_shells output.
// The tool formats one session per line; anything unparseable is skipped so a
// format change degrades to an empty table rather than a garbled one.
func parseShellRows(content string) [][]string {
	lines, _ := firstLines(content, 64)
	var rows [][]string
	for _, line := range lines {
		if !strings.HasPrefix(line, "sh_") && !strings.Contains(line, "shell_id") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		id, status := fields[0], fields[1]
		cmd := ""
		if len(fields) > 2 {
			cmd = strings.Join(fields[2:], " ")
		}
		rows = append(rows, []string{
			strings.Trim(id, ":"),
			strings.Trim(status, "[](),"),
			cmd,
		})
	}
	return rows
}

// exitChip renders a command's exit status as a colored chip.
func (c *Conversation) exitChip(code int) string {
	if code == 0 {
		return lipgloss.NewStyle().Foreground(c.styles.T.Green).Render(glyphOK + " exit 0")
	}
	return lipgloss.NewStyle().Foreground(c.styles.T.Red).Render(fmt.Sprintf("%s exit %d", glyphFail, code))
}
