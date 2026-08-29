package components

import (
	"strings"
	"testing"
)

func TestUpsertAgentRowUpdatesInPlace(t *testing.T) {
	c := NewConversation(testStyles())
	c.AddMessage("user", "go", false)

	c.UpsertAgentRow("agent-1", "mapper", "explore", "map the tui layer", "in grep", "running", "")
	c.UpsertAgentRow("agent-2", "reviewer", "review", "check the dock", "in read_file", "running", "")

	// Rows are appended after the existing messages.
	if len(c.messages) != 3 {
		t.Fatalf("want 3 messages, got %d", len(c.messages))
	}

	// An update must refresh the same entry, not add another.
	c.UpsertAgentRow("agent-1", "mapper", "explore", "map the tui layer", "in bash", "running", "")
	if len(c.messages) != 3 {
		t.Fatalf("update must not append, got %d messages", len(c.messages))
	}
	if c.messages[1].ToolContext != "in bash" {
		t.Fatalf("update must rewrite the row in place, context=%q", c.messages[1].ToolContext)
	}

	// Settling keeps position and records the outcome.
	c.UpsertAgentRow("agent-1", "mapper", "explore", "map the tui layer", "done", "completed", "Found 4 renderers")
	row := c.messages[1]
	if row.Status != "completed" || row.ToolSummary != "Found 4 renderers" {
		t.Fatalf("settle must record outcome, got status=%q summary=%q", row.Status, row.ToolSummary)
	}
	if c.messages[1].Timestamp.IsZero() || !c.messages[1].Timestamp.Equal(row.Timestamp) {
		t.Fatal("row timestamps must survive updates")
	}
}

func TestRenderAgentLiveRowShowsTypeSubjectActivity(t *testing.T) {
	c := NewConversation(testStyles())

	running := renderAgentRowForTest("mapper", "explore", "map the tui layer", "in grep · 3 tools · 42s", "running", "")
	out := strings.ToLower(c.renderAgentLiveRow(running, 80))
	for _, want := range []string{"explore", "map the tui layer", "in grep"} {
		if !strings.Contains(out, want) {
			t.Errorf("running row missing %q in:\n%s", want, out)
		}
	}

	settled := renderAgentRowForTest("mapper", "explore", "map the tui layer", "done · 5 tools · 1m2s", "completed", "Found 4 renderers")
	out = c.renderAgentLiveRow(settled, 80)
	if !strings.Contains(out, "Found 4 renderers") {
		t.Errorf("settled row must show the report line:\n%s", out)
	}
}

func renderAgentRowForTest(name, typ, subject, activity, status, result string) ConversationMsg {
	return ConversationMsg{
		Role:        "agent_live",
		ToolID:      "agent-1",
		ToolName:    typ,
		Content:     subject,
		ToolContext: activity,
		ToolSummary: result,
		Status:      status,
	}
}
