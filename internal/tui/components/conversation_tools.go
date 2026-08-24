package components

// The tool-card dispatcher. Every tool call enters here and is routed to its
// family renderer by the spec table in tool_spec.go; the shared row grammar
// lives in tool_row.go and the detail blocks in tool_box.go.
//
// This file owns no visual decisions of its own beyond the generic fallback
// for tools that have no declared family.

import (
	"bytes"
	"encoding/json"
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/iSundram/Automergent/internal/tui/render"
)

// renderToolCall routes one tool call to its family renderer.
func (c *Conversation) renderToolCall(m ConversationMsg, width int) string {
	switch specFor(m.ToolName).Family {
	case familyRead:
		return c.renderReadCard(m, width)
	case familyList:
		return c.renderListCard(m, width)
	case familySearch:
		return c.renderSearchCard(m, width)
	case familyEdit:
		return c.renderEditCard(m, width)
	case familyFileOp:
		return c.renderFileOpCard(m, width)
	case familyTerminal:
		return c.renderTerminalCard(m, width)
	case familyShellSession:
		return c.renderShellSessionCard(m, width)
	case familyShellList:
		return c.renderShellListCard(m, width)
	case familyWeb:
		return c.renderWebCard(m, width)
	case familyDiagnostics:
		return c.renderDiagnosticsCard(m, width)
	case familySecurity:
		return c.renderSecurityCard(m, width)
	case familyData:
		return c.renderDataCard(m, width)
	case familyAgent:
		return c.renderAgentCard(m, width)
	case familyPlan:
		return c.renderPlanCard(m, width)
	case familyTodo:
		return c.renderTodoCard(m, width)
	case familyTaskState:
		return c.renderTaskStateCard(m, width)
	case familyContext:
		return c.renderContextCard(m, width)
	case familyInteraction:
		return c.renderInteractionCard(m, width)
	default:
		return c.renderGenericCard(m, width)
	}
}

// renderToolGroup routes a collapsed run of same-family calls. Families
// without a group renderer fall back to rendering the first call plus a count,
// which is still correct — just less pretty.
func (c *Conversation) renderToolGroup(group []ConversationMsg, width int) string {
	if len(group) == 1 {
		return c.renderToolCall(group[0], width)
	}
	switch specFor(group[0].ToolName).Family {
	case familyRead, familyList:
		return c.renderReadGroup(group, width)
	case familyFileOp:
		return c.renderFileOpGroup(group, width)
	}
	return c.renderSimpleGroup(group, width)
}

// renderSimpleGroup is the family-agnostic rollup: one headline with a ×N
// count, then one summary row per call.
func (c *Conversation) renderSimpleGroup(group []ConversationMsg, width int) string {
	rollup := group[0]
	rollup.Duration = 0
	for _, g := range group {
		rollup.Duration += g.Duration
		if g.IsError {
			rollup.IsError = true
			rollup.Status = "error"
		}
	}

	head := c.callLine(rollup, width, "", []string{countChip(len(group))}, durationChip(rollup.Duration))
	if c.compactOnly() {
		return head
	}

	limit := c.bodyLimit(len(group))
	rows := make([]string, 0, limit+1)
	for i, g := range group {
		if i >= limit {
			break
		}
		mark := ""
		if g.IsError {
			mark = c.severityMark("error")
		}
		label := subjectFor(g)
		if label == "" {
			label = oneLine(g.ToolSummary)
		}
		rows = append(rows, c.resultRow(mark, label, width))
	}
	if more := len(group) - limit; more > 0 {
		rows = append(rows, c.moreRow(more, "call"))
	}
	return join(head, strings.Join(rows, "\n"))
}

// renderGenericCard is the fallback for tools with no declared family: the
// shared call line plus the tool's own summary and a short output tail, so an
// unregistered tool still looks like it belongs.
func (c *Conversation) renderGenericCard(m ConversationMsg, width int) string {
	head := c.callLine(m, width, subjectFor(m), nil, durationChip(m.Duration))

	if m.IsError {
		return join(head, c.resultRow(c.severityMark("error"), oneLine(m.Content), width))
	}
	if !c.showDetail() {
		return head
	}

	var rows []string
	if s := oneLine(m.ToolSummary); s != "" {
		rows = append(rows, c.resultRow("", s, width))
	}
	if strings.TrimSpace(m.Content) != "" {
		lines, hidden := firstLines(c.formatOutputValue(m.Content), c.bodyLimit(0))
		for _, l := range lines {
			rows = append(rows, c.resultRow("", l, width))
		}
		if hidden > 0 {
			rows = append(rows, c.moreRow(hidden, "line"))
		}
	}
	return join(head, strings.Join(rows, "\n"))
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

// formatOutputValue pretty-prints and syntax-highlights JSON output; anything
// else passes through untouched.
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

func hasSummary(m ConversationMsg) bool { return strings.TrimSpace(m.ToolSummary) != "" }

// oneLine collapses text to its first line, capped for headline use.
func oneLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if utf8.RuneCountInString(s) > 160 {
		s = string([]rune(s)[:159]) + glyphMore
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
	return string(r[:max-1]) + glyphMore
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
