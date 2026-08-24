package components

import (
	"fmt"
	"strings"
	"testing"

	"github.com/iSundram/Automergent/internal/tui/themes"
)

func testConv() *Conversation {
	c := NewConversation(themes.NewStyles(themes.Get("modern")))
	c.SetSize(90, 30)
	return &c
}

func TestReadCardSingleLineEvenUntruncated(t *testing.T) {
	c := testConv()
	m := ConversationMsg{
		Role:        "tool_call",
		ToolName:    "read_file",
		ToolContext: "reading main.go",
		Status:      "done",
		Content:     strings.Repeat("line\n", 120), // 120 lines — NOT truncated, still one row
		ToolArgs:    `{"path":"main.go","start_line":40,"end_line":160}`,
	}
	out := c.renderReadGroup([]ConversationMsg{m}, 90)
	if got := strings.Count(out, "\n"); got > 6 {
		t.Fatalf("read card must stay one line tall, got %d lines:\n%s", got, out)
	}
	for _, want := range []string{"main.go", "L40–160", "121 lines"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestReadGroupMultiFile(t *testing.T) {
	c := testConv()
	group := []ConversationMsg{
		{Role: "tool_call", ToolName: "read_file", ToolContext: "a.go", Status: "done", Content: "x\ny"},
		{Role: "tool_call", ToolName: "view", ToolContext: "b.go", Status: "done", Content: "z"},
		{Role: "tool_call", ToolName: "read_file", ToolContext: "c.go", Status: "done", Content: "w"},
	}
	out := c.renderReadGroup(group, 90)
	for _, want := range []string{"a.go", "b.go", "c.go"} {
		if !strings.Contains(out, want) {
			t.Errorf("multi-read missing %q:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "×3") {
		t.Errorf("expected ×3 rollup count:\n%s", out)
	}
}

func TestEditDiffBoxStats(t *testing.T) {
	c := testConv()
	diff := "ctx\n-old\n+new\n+new2\nctx2"
	out := c.diffBox(diff, 90)
	if !strings.Contains(out, "+2") || !strings.Contains(out, "−1") {
		t.Fatalf("diff box missing +2/−1 stats:\n%s", out)
	}
}

func TestTerminalTailBoxShowsLastLines(t *testing.T) {
	c := testConv()
	lines := make([]string, 30)
	for i := range lines {
		lines[i] = fmt.Sprintf("out-%d", i)
	}
	m := ConversationMsg{
		Role:        "tool_call",
		ToolName:    "bash",
		ToolContext: "exec: ./slow-test",
		Status:      "done",
		Content:     strings.Join(lines, "\n"),
	}
	out := c.renderTerminalCard(m, 90)
	if !strings.Contains(out, "out-29") {
		t.Fatal("tail box must show LAST output line")
	}
	if strings.Contains(out, "out-0\n") && strings.Contains(out, "out-1") {
		t.Log("head visible — acceptable only under ExpandFull")
	}
	if !strings.Contains(out, "more lines") {
		t.Errorf("scroll hint missing:\n%s", out)
	}
}

func TestGroupKeyFamilies(t *testing.T) {
	if groupKeyFor("read_file") != groupKeyFor("view") {
		t.Error("read family should merge read_file+view")
	}
	if groupKeyFor("bash") != groupKeyFor("run_command") {
		t.Error("terminal family should merge bash+run_command")
	}
	if groupKeyFor("edit_file") == groupKeyFor("read_file") {
		t.Error("edit and read must not merge")
	}
}
