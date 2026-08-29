package components

// Live subagent rows in the main conversation.
//
// The task tool card records that a subagent was asked for and what it
// returned, but between those two moments the conversation is silent — the
// agent's life happens off-stage in the dock. These rows put a short, live
// line for each subagent directly in the transcript:
//
//	● explore  "map the TUI render layer"  ·  in grep · 3 tools · 42s
//	  ⎿ Found 4 renderers: tool_read, tool_edit, tool_terminal, tool_box
//
// The row is a ConversationMsg with Role "agent_live", keyed by ToolID = the
// agent instance ID, and updated in place by the App's live tick (only when
// the rendered facts changed — see UpsertAgentRow).

import (
	"strings"
	"time"
)

// UpsertAgentRow adds or refreshes the live row for one subagent. activity is
// the "what is it doing right now" cell (current tool, or the settled
// outcome); result carries the first line of the report on completion.
func (c *Conversation) UpsertAgentRow(id, name, agentType, subject, activity, status, result string) {
	if id == "" {
		return
	}
	c.ensureViewport()
	c.FinalizeStreaming()

	msg := ConversationMsg{
		Role:        "agent_live",
		ToolID:      id,
		ToolName:    agentType,
		Content:     subject,
		ToolContext: activity,
		ToolSummary: result,
		Status:      status,
		Timestamp:   time.Now(),
	}

	for i := len(c.messages) - 1; i >= 0; i-- {
		if c.messages[i].Role == "agent_live" && c.messages[i].ToolID == id {
			// Keep the original arrival position and timestamp: a row that
			// jumps to the bottom on every update is unreadable.
			msg.Timestamp = c.messages[i].Timestamp
			prev := c.messages[i]
			if prev.ToolName == msg.ToolName && prev.Content == msg.Content &&
				prev.ToolContext == msg.ToolContext && prev.Status == msg.Status &&
				prev.ToolSummary == msg.ToolSummary {
				return // nothing observable changed; do not dirty the render
			}
			c.messages[i] = msg
			c.refreshAndFollow(false)
			return
		}
	}

	c.messages = append(c.messages, msg)
	c.refreshAndFollow(true)
}

// renderAgentLiveRow renders one live subagent row. It reuses the agent
// family's call-line grammar so the row reads as kin to the task tool card.
func (c *Conversation) renderAgentLiveRow(m ConversationMsg, width int) string {
	subject := m.Content
	if subject != "" {
		subject = `"` + oneLine(subject) + `"`
	}

	chips := []string{m.ToolContext}
	switch m.Status {
	case "done", "completed":
		chips = append(chips, "done")
	case "failed":
		chips = append(chips, "failed")
	case "cancelled":
		chips = append(chips, "cancelled")
	}

	head := c.callLine(m, width, subject, nonEmptyChips(chips...), "")
	if m.IsError {
		return join(head, c.resultRow(c.severityMark("error"), oneLine(m.ToolSummary), width))
	}
	if strings.TrimSpace(m.ToolSummary) != "" && c.showDetail() {
		return join(head, c.resultRow("", oneLine(m.ToolSummary), width))
	}
	return head
}

// nonEmptyChips keeps the row grammar clean when the activity cell is empty
// (an agent that has not done anything observable yet).
func nonEmptyChips(chips ...string) []string {
	out := make([]string, 0, len(chips))
	for _, ch := range chips {
		if strings.TrimSpace(ch) != "" {
			out = append(out, ch)
		}
	}
	return out
}
