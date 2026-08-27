package components

// Execution families: terminal (bash, run_command), shellsession (read_shell,
// write_shell, stop_shell, wait) and shelllist (list_shells).
//
// The slab is the signature block here: a theme-Surface panel carrying the
// "$ command" row and the merged output tail. The command appears exactly
// once — on the $ row; the call line above it carries only the tool name, a
// hairline, the exit chip and the duration.

import (
	"fmt"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
)

// renderTerminalCard renders bash / run_command.
//
//	● Bash ────────────────────────────────────── ✗ exit 2    7ms
//	  $ make build
//	  make: *** No rule to make target 'build'.  Stop.
//	  ↑ 6 more lines — ctrl+e expands
//
// The command lives ONCE, on the slab's $ row — the call line carries only the
// tool name, a hairline out to the outcome chip (exit status while done,
// "◌ running…" in flight, silence on quiet success) and the duration. Output
// is one merged stream: the tools' "[stderr]" section markers and their
// "command failed:" preambles are stripped, the chip already says it.
func (c *Conversation) renderTerminalCard(m ConversationMsg, width int) string {
	slab := c.showDetail() &&
		(m.Status == "running" || strings.TrimSpace(m.Content) != "")
	if !slab {
		// No body to paint (quiet success, compact mode): carry the command on
		// the call line itself so it is stated exactly once, wherever it is.
		return c.callLine(m, width, terminalCommand(m), c.terminalChips(m), durationChip(m.Duration))
	}
	return join(c.terminalHead(m, width), c.terminalSlab(m, width))
}

// terminalHead is the terminal family's call line: glyph, name, meta chips,
// then a hairline stretching to a right-aligned trailer of exit chip + time.
func (c *Conversation) terminalHead(m ConversationMsg, width int) string {
	spec := specFor(m.ToolName)
	name := lipgloss.NewStyle().
		Foreground(spec.Accent(c.styles.T)).
		Bold(true).
		Render(spec.Display)

	left := gutter + c.statusGlyph(m) + " " + name

	chips := c.terminalChips(m)
	if chip := strings.Join(chips, glyphSep); chip != "" {
		left += "  " + c.styles.Dim.Render(chip)
	}

	trailer := c.commandOutcome(m)
	dur := durationChip(m.Duration)
	if dur != "" && trailer != "" {
		trailer += "  "
	}
	trailer += c.styles.Dim.Render(dur)

	line := left
	if tw := lipgloss.Width(trailer); tw > 0 {
		rule := width - lipgloss.Width(left) - tw - 4
		if rule >= 3 {
			line += "  " + c.styles.Dim.Render(strings.Repeat("─", rule)) + "  "
		} else {
			line += strings.Repeat(" ", max(3, width-lipgloss.Width(line)-tw))
		}
		line += trailer
	}
	return line
}

// terminalChips reports the async-vs-sync shape of a command. Failure is NOT
// chipped here — the head's exit trailer owns that.
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

// terminalSlab paints the command block on the theme's surface: "$ command"
// first, then the last N lines of merged output, with a scroll hint below the
// slab when lines were hidden.
func (c *Conversation) terminalSlab(m ConversationMsg, width int) string {
	inner := slabInner(width)

	textFg := lipgloss.NewStyle().Foreground(c.styles.T.Text)
	prompt := lipgloss.NewStyle().Foreground(c.styles.T.Accent).Bold(true)

	// Multi-line commands collapse onto one row with a visible ⏎ so the slab
	// keeps a stable height regardless of how the model formatted the command.
	cmdVis := strings.ReplaceAll(terminalCommand(m), "\n", " "+glyphReturn+" ")

	rows := []string{c.slabRowTrailer(
		prompt.Render("$")+" "+textFg.Render(cmdVis), "", inner)}

	hidden := 0
	if strings.TrimSpace(m.Content) != "" {
		shown, hid := tailLines(mergedStream(m.Content), c.tailLimit())
		hidden = hid
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

// mergedStream strips the shell tools' stream bookkeeping — "[stderr]" /
// "[stdout]" section markers and "command failed: …" preambles — leaving one
// plain output stream. Failure is already announced by the head's exit chip;
// printing it again inside the body was pure duplication.
func mergedStream(content string) string {
	lines := strings.Split(strings.TrimRight(content, "\n"), "\n")
	kept := make([]string, 0, len(lines))
	for _, l := range lines {
		t := strings.TrimSpace(l)
		switch {
		case t == "", t == "[stderr]", t == "[stdout]":
			continue
		case strings.HasPrefix(t, "command failed:"), strings.HasPrefix(t, "command error:"):
			continue
		}
		kept = append(kept, l)
	}
	return strings.Join(kept, "\n")
}

// commandOutcome is the right-aligned chip on a command row: a pulsing
// "running…" while in flight, a red exit status on failure, silence on quiet
// success.
func (c *Conversation) commandOutcome(m ConversationMsg) string {
	if m.Status == "running" {
		return lipgloss.NewStyle().Foreground(c.styles.T.Yellow).Render("running…")
	}
	if code, ok := exitCodeOf(m); ok && code != 0 {
		return c.exitChip(code)
	}
	if m.IsError {
		return lipgloss.NewStyle().Foreground(c.styles.T.Red).Render(fmt.Sprintf("%s failed", glyphFail))
	}
	return ""
}

// exitCodeOf reads a command's exit status from Result.Metadata, which arrives
// numeric or string depending on which path recorded it.
func exitCodeOf(m ConversationMsg) (int, bool) {
	if v, ok := metaInt(m, "exit_code"); ok {
		return v, true
	}
	if s := metaString(m, "exit_code"); s != "" {
		if n, err := strconv.Atoi(s); err == nil {
			return n, true
		}
	}
	return 0, false
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
