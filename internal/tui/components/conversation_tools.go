package components

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image/color"
	"strings"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/iSundram/Automergent/internal/tui/render"
)

// colorAlias keeps the branding table readable (themes expose color.Color).
type colorAlias = color.Color

// toolStatusView derives the status glyph, text and color for a card header.
func (c *Conversation) toolStatusView(status string) (colorAlias, string) {
	switch status {
	case "done":
		return c.styles.T.Green, "󰄬 Completed"
	case "error":
		return c.styles.T.Red, "󰅙 Failed"
	default:
		return c.styles.T.Yellow, "󱓞 Running"
	}
}

// toolHeader assembles the shared one-line card header.
func (c *Conversation) toolHeader(m ConversationMsg, width int, count int) string {
	statusColor, statusText := c.toolStatusView(m.Status)
	prettyName := c.toolBrandingName(m.ToolName)
	if count > 1 {
		statusText = fmt.Sprintf("%s ×%d", statusText, count)
	}

	statusStyle := lipgloss.NewStyle().Foreground(statusColor).Bold(true).Padding(0, 1)

	nameStyled := c.styles.ToolName.Copy().Foreground(c.styles.T.Text).Render(" " + prettyName)
	headerLeft := lipgloss.JoinHorizontal(lipgloss.Center, nameStyled, statusStyle.Render(statusText))

	durationText := ""
	total := m.Duration
	if total > 0 {
		durationText = " " + c.styles.ToolDuration.Render(total.Round(time.Millisecond).String())
	}

	left := headerLeft + durationText
	pad := width - 6 - lipgloss.Width(left)
	if pad < 1 {
		pad = 1
	}
	return left + strings.Repeat(" ", pad)
}

// wrapCard frames card content with the tool's accent border.
func wrapCard(accent colorAlias, width int, content string) string {
	return lipgloss.NewStyle().
		Border(lipgloss.ThickBorder(), false, false, false, true).
		BorderForeground(accent).
		Padding(0, 1).
		MarginBottom(1).
		Width(width - 2).
		Render(content)
}

// looksLikeJSON reports whether s parses as a JSON object or array.
func looksLikeJSON(s string) bool {
	t := strings.TrimSpace(s)
	if len(t) < 2 {
		return false
	}
	if (t[0] != '{' && t[0] != '[') || (t[len(t)-1] != '}' && t[len(t)-1] != ']') {
		return false
	}
	return json.Valid([]byte(t))
}

// formatOutputValue applies per-tool formatting: JSON gets syntax
// highlighting, everything else passes through untouched.
func (c *Conversation) formatOutputValue(text string) string {
	t := strings.TrimSpace(text)
	if looksLikeJSON(t) {
		var pretty bytes.Buffer
		if json.Indent(&pretty, []byte(t), "", "  ") == nil {
			return render.Code(pretty.String(), "json")
		}
	}
	return text
}

// truncateLines clips rendered plain text to n lines with an expand hint.
func (c *Conversation) truncateLines(text string, n int) string {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	if len(lines) <= n {
		return text
	}
	head := strings.Join(lines[:n], "\n")
	return head + "\n  " + c.styles.Dim.Render(fmt.Sprintf("… (%d more lines — ctrl+e expands, ctrl+r for review mode)", len(lines)-n))
}

// renderToolCall routes to the per-family renderer. Each tool family owns
// a file: tool_read.go, tool_edit.go, tool_terminal.go — this fallback is
// the generic param/result card for everything else.
func (c *Conversation) renderToolCall(m ConversationMsg, width int) string {
	switch {
	case readFamily(m.ToolName):
		return c.renderReadGroup([]ConversationMsg{m}, width)
	case isFileEditTool(m.ToolName):
		return c.renderEditCard(m, width)
	case terminalFamily(m.ToolName):
		return c.renderTerminalCard(m, width)
	}
	header := c.toolHeader(m, width, 1)

	details := c.expandMode == ExpandFull || c.reviewMode
	collapse := c.expandMode == ExpandCompact && !c.reviewMode

	lightweight := m.ToolName == "read_file" || m.ToolName == "view" || m.ToolName == "list_directory"
	showDetails := details || (!collapse && !lightweight)

	var body strings.Builder
	hasBody := false
	appendField := func(label, value string, valueStyle lipgloss.Style) {
		if strings.TrimSpace(value) == "" {
			return
		}
		if !hasBody {
			body.WriteString("\n\n")
		} else {
			body.WriteString("\n")
		}
		hasBody = true
		labelWidth := 10
		labelText := c.styles.Dim.Render(fmt.Sprintf("%-*s", labelWidth, label))
		lines := strings.Split(strings.TrimSpace(value), "\n")
		body.WriteString("  " + labelText + valueStyle.Render(lines[0]))
		continuation := "  " + strings.Repeat(" ", labelWidth)
		for _, line := range lines[1:] {
			body.WriteString("\n" + continuation + valueStyle.Render(line))
		}
	}

	if showDetails && !collapse {
		// Section: key parameters
		if m.ToolArgs != "" && m.ToolArgs != "{}" {
			var args map[string]any
			if err := json.Unmarshal([]byte(m.ToolArgs), &args); err == nil {
				for _, key := range []string{"path", "command", "pattern", "url", "query"} {
					if value, ok := args[key]; ok {
						appendField(strings.ToUpper(key[:1])+key[1:], fmt.Sprint(value), lipgloss.NewStyle().Foreground(c.styles.T.Text))
					}
				}
			}
			if details {
				appendField("Details", render.Code(m.ToolArgs, "json"), lipgloss.NewStyle())
			}
		}

		// Section: output / summary
		if m.Status == "done" || m.Status == "error" {
			if m.ToolSummary != "" {
				appendField("Result", m.ToolSummary, c.styles.Dim.Copy().Italic(true))
			}
			if m.Content != "" {
				resultText := c.formatOutputValue(m.Content)
				if !details {
					resultText = c.truncateLines(resultText, 3)
				}
				appendField("Output", resultText, lipgloss.NewStyle().Foreground(c.styles.T.Subtext).Faint(true))
			}
		} else if m.Status == "running" && isFileEditTool(m.ToolName) && m.Content != "" {
			body.WriteString("\n\n")
			hasBody = true
			body.WriteString("  " + c.styles.Dim.Render("Changes") + "\n")
			limit := clampInt(c.height/5, 5, 15)
			raw := strings.Split(strings.TrimSpace(m.Content), "\n")
			shown := raw
			more := 0
			if len(raw) > limit {
				shown = raw[:limit]
				more = len(raw) - limit
			}
			preview := render.DiffWithWidth(strings.Join(shown, "\n"), width-4)
			for _, line := range strings.Split(strings.TrimSuffix(preview, "\n"), "\n") {
				body.WriteString("\n  " + line)
			}
			if more > 0 {
				body.WriteString(fmt.Sprintf("\n  %s", c.styles.Dim.Render(fmt.Sprintf("… (%d more lines, see Diff pane)", more))))
			}
		}
	} else if collapse && hasSummary(m) {
		body.WriteString("\n\n")
		body.WriteString("  " + c.styles.Dim.Copy().Italic(true).Render(oneLine(m.ToolSummary)))
	}

	_, accent, _ := c.toolBranding(m.ToolName)
	return wrapCard(accent, width, header+body.String())
}

func hasSummary(m ConversationMsg) bool { return strings.TrimSpace(m.ToolSummary) != "" }

func oneLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if utf8.RuneCountInString(s) > 80 {
		s = string([]rune(s)[:79]) + "…"
	}
	return s
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// renderGroupedToolCalls collapses N consecutive calls of the same tool into
// a single card with one summary line each.
func (c *Conversation) renderGroupedToolCalls(group []ConversationMsg, width int) string {
	first := group[0]

	var total time.Duration
	for _, g := range group {
		total += g.Duration
	}
	rollup := first
	rollup.Duration = total
	header := c.toolHeader(rollup, width, len(group))

	_, accent, _ := c.toolBranding(first.ToolName)

	if c.expandMode == ExpandCompact && !c.reviewMode {
		if hasSummary(first) {
			return wrapCard(accent, width, header+"\n\n"+"  "+c.styles.Dim.Copy().Italic(true).Render(oneLine(first.ToolSummary)))
		}
		return wrapCard(accent, width, header)
	}

	var body strings.Builder
	body.WriteString("\n")
	listLimit := 5
	if details := c.expandMode == ExpandFull || c.reviewMode; details {
		listLimit = len(group)
	}
	for i, g := range group {
		if i >= listLimit {
			body.WriteString(fmt.Sprintf("\n  %s", c.styles.Dim.Render(fmt.Sprintf("… %d more", len(group)-i))))
			break
		}
		line := g.ToolContext
		if line == "" {
			line = oneLine(g.ToolSummary)
		} else if s := oneLine(g.ToolSummary); s != "" {
			line += " — " + s
		}
		dur := ""
		if g.Duration > 0 {
			dur = " " + c.styles.ToolDuration.Render(g.Duration.Round(time.Millisecond).String())
		}
		statusMark := lipgloss.NewStyle().Foreground(c.styles.T.Green).Render("✓")
		if g.IsError {
			statusMark = lipgloss.NewStyle().Foreground(c.styles.T.Red).Render("✗")
		}
		text := ansiSafeTruncate(line, width-10)
		body.WriteString(fmt.Sprintf("\n  %s %s%s", statusMark, text, dur))
	}
	return wrapCard(accent, width, header+body.String())
}

// ansiSafeTruncate clips a plain-text line to max runes.
func ansiSafeTruncate(s string, max int) string {
	s = strings.TrimSpace(s)
	if max < 4 {
		max = 4
	}
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	r := []rune(s)
	return string(r[:max-1]) + "…"
}

// Update forwards messages to the viewport so scroll mode (and mouse
// scrolling in browsing mode) keeps working.
func (c Conversation) Update(msg tea.Msg) (Conversation, tea.Cmd) {
	if _, ok := msg.(tea.MouseMsg); ok {
		vp, cmd := c.viewport.Update(msg)
		c.viewport = vp
		return c, cmd
	}
	vp, cmd := c.viewport.Update(msg)
	c.viewport = vp
	return c, cmd
}

// View renders the viewport with a manual scrollbar while browsing.
func (c Conversation) View() string {
	content := c.viewport.View()
	if !c.browsing || c.viewport.TotalLineCount() <= c.viewport.VisibleLineCount() {
		return content
	}
	trackHeight := c.viewport.Height()
	total := c.viewport.TotalLineCount()
	visible := c.viewport.VisibleLineCount()
	thumbHeight := visible * trackHeight / total
	if thumbHeight < 1 {
		thumbHeight = 1
	}
	maxOffset := total - visible
	maxTop := trackHeight - thumbHeight
	thumbTop := 0
	if maxOffset > 0 {
		thumbTop = c.viewport.YOffset() * maxTop / maxOffset
	}
	bar := make([]string, trackHeight)
	for i := range bar {
		bar[i] = "░"
		if i >= thumbTop && i < thumbTop+thumbHeight {
			bar[i] = "█"
		}
	}
	trackStyle := lipgloss.NewStyle().Foreground(c.styles.T.Muted)
	thumbStyle := lipgloss.NewStyle().Foreground(c.styles.T.Accent)
	for i := range bar {
		if bar[i] == "█" {
			bar[i] = thumbStyle.Render(bar[i])
		} else {
			bar[i] = trackStyle.Render(bar[i])
		}
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, content, strings.Join(bar, "\n"))
}

// MessageCount returns the number of conversation entries.
func (c Conversation) MessageCount() int {
	return len(c.messages)
}

// LastMessage returns the most recent conversation entry.
func (c Conversation) LastMessage() (ConversationMsg, bool) {
	if len(c.messages) == 0 {
		return ConversationMsg{}, false
	}
	return c.messages[len(c.messages)-1], true
}

func truncateContent(s string, reviewMode bool) string {
	if reviewMode {
		return s
	}
	const maxRunes = 220
	if utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	runes := []rune(s)
	return string(runes[:maxRunes]) + " … [truncated, press Ctrl+R for full review mode]"
}

// toolBrandingName returns just the pretty display name.
func (c *Conversation) toolBrandingName(name string) string {
	_, _, pretty := c.toolBranding(name)
	return pretty
}

func (c *Conversation) toolBranding(name string) (icon string, accent colorAlias, pretty string) {
	icon = "󰆍"
	accent = c.styles.T.Accent
	pretty = name

	switch name {
	case "read_file", "view":
		pretty, icon, accent = "Readfile", "󰈔", c.styles.T.Blue
	case "write_file", "create_file":
		pretty, icon, accent = "Write", "󱇧", c.styles.T.Green
	case "edit_file":
		pretty, icon, accent = "Edit", "󰛓", c.styles.T.Yellow
	case "delete_file":
		pretty, icon, accent = "Delete", "󰆴", c.styles.T.Red
	case "move_file":
		pretty, icon, accent = "Move", "󰪹", c.styles.T.Blue
	case "copy_file":
		pretty, icon, accent = "Copy", "󰪹", c.styles.T.Blue
	case "list_directory":
		pretty, icon, accent = "List directory", "󰉋", c.styles.T.Blue
	case "search":
		pretty, icon, accent = "Deep Search", "󰍉", c.styles.T.Magenta
	case "glob":
		pretty, icon, accent = "Glob", "󰈞", c.styles.T.Blue
	case "grep", "grep_search":
		pretty, icon, accent = "Search", "󰍉", c.styles.T.Magenta
	case "run_shell_command", "run_command", "bash":
		pretty, icon, accent = "Run", "󰆍", c.styles.T.Yellow
	case "read_shell":
		pretty, icon, accent = "Read shell", "󰇯", c.styles.T.Yellow
	case "write_shell":
		pretty, icon, accent = "Write shell", "󰇰", c.styles.T.Yellow
	case "stop_shell":
		pretty, icon, accent = "Stop shell", "󰅙", c.styles.T.Red
	case "lsp_diagnostics", "lsp_symbols":
		pretty, icon, accent = "LSP", "󰘦", c.styles.T.Cyan
	case "web_fetch", "web_search":
		pretty, icon, accent = "Web", "󰖟", c.styles.T.Magenta
	case "sql":
		pretty, icon, accent = "SQL", "󰆼", c.styles.T.Blue
	case "secrets_scan":
		pretty, icon, accent = "Secrets scan", "󰦝", c.styles.T.Red
	case "dependency_audit":
		pretty, icon, accent = "Audit", "󰒺", c.styles.T.Yellow
	case "task":
		pretty, icon, accent = "Task", "󰒋", c.styles.T.Accent
	case "read_agent":
		pretty, icon, accent = "Read agent", "󰒋", c.styles.T.Accent
	}
	return
}
