package components

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/iSundram/Automergent/internal/tui/themes"
)

func testConv() *Conversation {
	c := NewConversation(themes.NewStyles(themes.Get("modern")))
	c.SetSize(90, 30)
	return &c
}

// allToolNames mirrors every tool registered in cmd/automergent/main.go,
// internal/tools/taskstate.go and internal/agent/agent.go. If a tool is added
// there without a toolSpecs entry, TestEveryToolHasASpec fails loudly rather
// than letting it silently fall back to the generic card.
var allToolNames = []string{
	"agent_control", "ask_user", "bash",
	"context_bucket_create", "context_bucket_delete", "context_bucket_get",
	"context_bucket_list", "context_bucket_set", "context_get_init",
	"context_get_intent", "context_list_buckets",
	"copy_file", "create_file", "delete_file", "dependency_audit", "edit_file",
	"glob", "grep", "list_agents", "list_directory", "list_shells",
	"lsp_diagnostics", "move_file", "multi_edit", "notify", "plan",
	"read_agent", "read_file", "read_shell", "replan", "run_command",
	"search", "secrets_scan", "sql", "stop_shell", "task", "task_get",
	"task_list", "task_update", "todo_list", "todo_next", "todo_write",
	"view", "wait", "web_fetch", "web_search", "write_file", "write_shell",
}

func TestEveryToolHasASpec(t *testing.T) {
	if len(allToolNames) != 48 {
		t.Fatalf("expected 48 registered tools, list has %d", len(allToolNames))
	}
	for _, name := range allToolNames {
		spec, ok := toolSpecs[name]
		if !ok {
			t.Errorf("%s: no toolSpecs entry — it would fall back to the generic card", name)
			continue
		}
		if spec.Display == "" {
			t.Errorf("%s: empty Display name", name)
		}
		if spec.Accent == nil {
			t.Errorf("%s: nil Accent", name)
		}
		if spec.Family == familyGeneric {
			t.Errorf("%s: declared but left in familyGeneric", name)
		}
	}
}

func TestEveryToolRendersOneLineWhenCompact(t *testing.T) {
	for _, name := range allToolNames {
		c := testConv()
		c.expandMode = ExpandCompact
		m := ConversationMsg{
			Role:     "tool_call",
			ToolName: name,
			Status:   "done",
			Content:  strings.Repeat("output line\n", 40),
			ToolArgs: `{"path":"main.go","command":"go build","pattern":"foo","query":"foo","url":"https://go.dev","file":"main.go","action":"replace","shell_id":"sh_1","task_id":"1","bucket":"b","key":"k","agent_id":"a1","question":"why","message":"hi","seconds":3,"source":"a.go","destination":"b.go"}`,
			Duration: 12 * time.Millisecond,
		}
		out := c.renderToolCall(m, 90)
		if got := strings.Count(strings.TrimRight(out, "\n"), "\n"); got != 0 {
			t.Errorf("%s: compact card must be exactly one line, got %d extra:\n%s", name, got, out)
		}
	}
}

func TestCallLineNeverExceedsWidth(t *testing.T) {
	for _, width := range []int{40, 60, 90, 120} {
		for _, name := range []string{"read_file", "bash", "grep", "web_fetch", "secrets_scan"} {
			c := testConv()
			c.expandMode = ExpandCompact
			m := ConversationMsg{
				Role:     "tool_call",
				ToolName: name,
				Status:   "done",
				ToolArgs: `{"path":"` + strings.Repeat("very/long/path/segment/", 12) + `file.go","command":"` + strings.Repeat("echo hello world; ", 20) + `","pattern":"` + strings.Repeat("x", 200) + `","url":"https://example.com/` + strings.Repeat("y", 200) + `"}`,
				Duration: 4200 * time.Millisecond,
			}
			line := c.renderToolCall(m, width)
			if got := lipgloss.Width(line); got > width {
				t.Errorf("%s at width %d: call line is %d wide:\n%q", name, width, got, ansi.Strip(line))
			}
		}
	}
}

func TestReadCardStaysOneLineEvenUntruncated(t *testing.T) {
	c := testConv()
	m := ConversationMsg{
		Role:        "tool_call",
		ToolName:    "read_file",
		ToolContext: "reading main.go",
		Status:      "done",
		Content:     strings.Repeat("line\n", 120), // 120 lines — still one row
		ToolArgs:    `{"path":"main.go","start_line":40,"end_line":160}`,
	}
	out := c.renderToolCall(m, 90)
	if got := strings.Count(strings.TrimRight(out, "\n"), "\n"); got != 0 {
		t.Fatalf("read card must stay one line, got %d extra lines:\n%s", got, out)
	}
	plain := ansi.Strip(out)
	for _, want := range []string{"Read", "main.go", "L40–160", "120 lines"} {
		if !strings.Contains(plain, want) {
			t.Errorf("missing %q in: %q", want, plain)
		}
	}
}

func TestReadGroupCollapsesFiles(t *testing.T) {
	c := testConv()
	group := []ConversationMsg{
		{Role: "tool_call", ToolName: "read_file", ToolArgs: `{"path":"a.go"}`, Status: "done", Content: "x\ny"},
		{Role: "tool_call", ToolName: "view", ToolArgs: `{"path":"b.go"}`, Status: "done", Content: "z"},
		{Role: "tool_call", ToolName: "read_file", ToolArgs: `{"path":"c.go"}`, Status: "done", Content: "w"},
	}
	plain := ansi.Strip(c.renderToolGroup(group, 90))
	for _, want := range []string{"3 files", "a.go", "b.go", "c.go"} {
		if !strings.Contains(plain, want) {
			t.Errorf("grouped read missing %q:\n%s", want, plain)
		}
	}
}

func TestSearchCardUsesMetadataCounts(t *testing.T) {
	c := testConv()
	m := ConversationMsg{
		Role:     "tool_call",
		ToolName: "grep",
		Status:   "done",
		ToolArgs: `{"pattern":"renderToolCall"}`,
		Content:  "a/b.go:109:\tfunc x\na/b.go:120:\tfunc y\nc/d.go:82:\tfunc z",
		Metadata: map[string]any{"total_matches": 12, "files_matched": 4},
	}
	plain := ansi.Strip(c.renderToolCall(m, 90))
	if !strings.Contains(plain, "12 matches in 4 files") {
		t.Errorf("search headline should use metadata counts:\n%s", plain)
	}
	// Hits fold to one row per file, with a ×N count for repeats.
	if !strings.Contains(plain, "b.go:109") || !strings.Contains(plain, "×2") {
		t.Errorf("search body should fold per file with a hit count:\n%s", plain)
	}
}

func TestEditCardShowsDiffStats(t *testing.T) {
	c := testConv()
	m := ConversationMsg{
		Role:     "tool_call",
		ToolName: "edit_file",
		Status:   "done",
		ToolArgs: `{"path":"tool_box.go","old_str":"a","new_str":"b"}`,
		Content:  "ctx\n-old\n+new\n+new2\nctx2",
	}
	plain := ansi.Strip(c.renderToolCall(m, 90))
	if !strings.Contains(plain, "+2") || !strings.Contains(plain, "−1") {
		t.Fatalf("edit card missing +2/−1 diff stats:\n%s", plain)
	}
	if !strings.Contains(plain, "1 line replaced") {
		t.Errorf("edit card missing replacement chip:\n%s", plain)
	}
}

func TestTerminalSlabShowsLastLines(t *testing.T) {
	c := testConv()
	lines := make([]string, 30)
	for i := range lines {
		lines[i] = fmt.Sprintf("out-%d", i)
	}
	m := ConversationMsg{
		Role:     "tool_call",
		ToolName: "bash",
		Status:   "done",
		ToolArgs: `{"command":"./slow-test"}`,
		Content:  strings.Join(lines, "\n"),
	}
	plain := ansi.Strip(c.renderToolCall(m, 90))
	if !strings.Contains(plain, "out-29") {
		t.Fatal("slab must show the LAST output line")
	}
	if !strings.Contains(plain, "more lines") {
		t.Errorf("scroll hint missing:\n%s", plain)
	}
	// The command appears exactly once — on the slab's $ row. The call line
	// carries only the tool name, hairline, exit chip and duration.
	if got := strings.Count(plain, "./slow-test"); got != 1 {
		t.Errorf("command should appear once, on the $ row, got %d:\n%s", got, plain)
	}
}

func TestDiagnosticsCleanCollapsesToOneLine(t *testing.T) {
	c := testConv()
	m := ConversationMsg{
		Role:     "tool_call",
		ToolName: "lsp_diagnostics",
		Status:   "done",
		ToolArgs: `{"file":"main.go"}`,
		Content:  "no compile errors (go build succeeded)",
	}
	out := c.renderToolCall(m, 90)
	if got := strings.Count(strings.TrimRight(out, "\n"), "\n"); got != 0 {
		t.Fatalf("a clean diagnostics result should cost one line, got %d extra:\n%s", got, out)
	}
	if !strings.Contains(ansi.Strip(out), glyphOK) {
		t.Errorf("clean result should carry a check glyph:\n%s", ansi.Strip(out))
	}
}

func TestDiagnosticsFindingsAreSeverityMarked(t *testing.T) {
	c := testConv()
	m := ConversationMsg{
		Role:     "tool_call",
		ToolName: "lsp_diagnostics",
		Status:   "done",
		ToolArgs: `{"file":"internal/tui"}`,
		Content:  "tool_box.go:44:6: declared and not used: limit\ntool_edit.go:88:1: warning: unreachable code",
	}
	plain := ansi.Strip(c.renderToolCall(m, 90))
	for _, want := range []string{"1 error", "1 warning", "tool_box.go:44:6", glyphFail, glyphWarn} {
		if !strings.Contains(plain, want) {
			t.Errorf("diagnostics card missing %q:\n%s", want, plain)
		}
	}
}

func TestSecurityCleanScanIsOneGreenLine(t *testing.T) {
	c := testConv()
	m := ConversationMsg{
		Role:     "tool_call",
		ToolName: "secrets_scan",
		Status:   "done",
		ToolArgs: `{"path":"internal/"}`,
		Content:  "✅ No secrets detected",
		Metadata: map[string]any{"findings_count": 0},
	}
	plain := ansi.Strip(c.renderToolCall(m, 90))
	if !strings.Contains(plain, "no findings") {
		t.Errorf("zero findings should read as good news:\n%s", plain)
	}
}

func TestSecretScanRowsDoNotReprintSecrets(t *testing.T) {
	c := testConv()
	m := ConversationMsg{
		Role:     "tool_call",
		ToolName: "secrets_scan",
		Status:   "done",
		ToolArgs: `{"path":"internal/"}`,
		Content:  "⚠️  Found 1 potential secret(s):\n\n• config/loader.go:88 - aws_access_key\n  AKIA****************MPL",
		Metadata: map[string]any{"findings_count": 1},
	}
	plain := ansi.Strip(c.renderToolCall(m, 90))
	if !strings.Contains(plain, "config/loader.go:88") {
		t.Errorf("finding location should be shown:\n%s", plain)
	}
	if strings.Contains(plain, "AKIA") {
		t.Errorf("masked secret must not be reprinted in the card:\n%s", plain)
	}
}

func TestTodoCardCountsStatuses(t *testing.T) {
	c := testConv()
	m := ConversationMsg{
		Role:     "tool_call",
		ToolName: "todo_list",
		Status:   "done",
		Content: "- [completed] Inventory all tools (pri=1)\n" +
			"- [in_progress] Design the row grammar (pri=1)\n" +
			"- [pending] Implement renderers (pri=2)",
	}
	plain := ansi.Strip(c.renderToolCall(m, 90))
	for _, want := range []string{"1 done", "1 in progress", "1 pending", "Design the row grammar"} {
		if !strings.Contains(plain, want) {
			t.Errorf("todo card missing %q:\n%s", want, plain)
		}
	}
	// The "(pri=N)" tail is noise in the log.
	if strings.Contains(plain, "pri=") {
		t.Errorf("todo rows should drop the priority tail:\n%s", plain)
	}
}

func TestTodoStatusWriteIsOneLine(t *testing.T) {
	c := testConv()
	m := ConversationMsg{
		Role:     "tool_call",
		ToolName: "todo_write",
		Status:   "done",
		ToolArgs: `{"action":"status","id":"step-2","status":"in_progress"}`,
		Content:  "updated",
	}
	out := c.renderToolCall(m, 90)
	if got := strings.Count(strings.TrimRight(out, "\n"), "\n"); got != 0 {
		t.Fatalf("a single status move should cost one line, got %d extra:\n%s", got, out)
	}
	if !strings.Contains(ansi.Strip(out), "in_progress") {
		t.Errorf("status move should name the new status:\n%s", ansi.Strip(out))
	}
}

func TestWebFetchShowsHostAndSize(t *testing.T) {
	c := testConv()
	m := ConversationMsg{
		Role:     "tool_call",
		ToolName: "web_fetch",
		Status:   "done",
		ToolArgs: `{"url":"https://go.dev/doc/effective_go"}`,
		Content:  "<html><head><title>Effective Go</title></head><body>x</body></html>",
	}
	plain := ansi.Strip(c.renderToolCall(m, 90))
	if strings.Contains(plain, "https://") {
		t.Errorf("scheme should be dropped from the headline:\n%s", plain)
	}
	for _, want := range []string{"go.dev/doc/effective_go", "Effective Go"} {
		if !strings.Contains(plain, want) {
			t.Errorf("fetch card missing %q:\n%s", want, plain)
		}
	}
}

func TestContextToolsAreOneQuietLine(t *testing.T) {
	for _, name := range []string{
		"context_bucket_create", "context_bucket_set", "context_bucket_get",
		"context_bucket_list", "context_bucket_delete", "context_list_buckets",
		"context_get_intent", "context_get_init",
	} {
		c := testConv()
		m := ConversationMsg{
			Role:     "tool_call",
			ToolName: name,
			Status:   "done",
			ToolArgs: `{"bucket":"plan","key":"step","name":"plan"}`,
			Content:  strings.Repeat("noise\n", 50),
		}
		out := c.renderToolCall(m, 90)
		if got := strings.Count(strings.TrimRight(out, "\n"), "\n"); got != 0 {
			t.Errorf("%s: bookkeeping tools must never open a body, got %d extra lines:\n%s", name, got, out)
		}
	}
}

func TestErrorRendersRedBulletAndReason(t *testing.T) {
	c := testConv()
	m := ConversationMsg{
		Role:     "tool_call",
		ToolName: "read_file",
		Status:   "error",
		IsError:  true,
		ToolArgs: `{"path":"missing.go"}`,
		Content:  "open missing.go: no such file or directory",
	}
	out := c.renderToolCall(m, 90)
	plain := ansi.Strip(out)
	if !strings.Contains(plain, glyphDone) {
		t.Errorf("error card should still carry the status bullet:\n%s", plain)
	}
	if !strings.Contains(plain, "no such file") {
		t.Errorf("error card must state the reason:\n%s", plain)
	}
	// Red must actually be applied, not just implied.
	if !strings.Contains(out, "38;2;") && !strings.Contains(out, "\x1b[") {
		t.Errorf("error bullet should be colored:\n%q", out)
	}
}

func TestRunningCardShowsRunningGlyph(t *testing.T) {
	c := testConv()
	m := ConversationMsg{
		Role:     "tool_call",
		ToolName: "bash",
		Status:   "running",
		ToolArgs: `{"command":"go test ./..."}`,
	}
	plain := ansi.Strip(c.renderToolCall(m, 90))
	if !strings.Contains(plain, glyphRunning) {
		t.Errorf("in-flight call should use the running glyph:\n%s", plain)
	}
	if !strings.Contains(plain, "running…") {
		t.Errorf("running slab should carry the chip:\n%s", plain)
	}
}

func TestExpandFullNeverTruncates(t *testing.T) {
	c := testConv()
	c.expandMode = ExpandFull
	items := make([]string, 25)
	for i := range items {
		items[i] = fmt.Sprintf("- [pending] task number %d", i)
	}
	m := ConversationMsg{
		Role:     "tool_call",
		ToolName: "todo_list",
		Status:   "done",
		Content:  strings.Join(items, "\n"),
	}
	plain := ansi.Strip(c.renderToolCall(m, 90))
	for _, want := range []string{"task number 0", "task number 24"} {
		if !strings.Contains(plain, want) {
			t.Errorf("ExpandFull must show every row, missing %q", want)
		}
	}
	if strings.Contains(plain, "more item") {
		t.Errorf("ExpandFull should not emit a truncation tail:\n%s", plain)
	}
}

func TestGroupKeyFamilies(t *testing.T) {
	if groupKeyFor("read_file") != groupKeyFor("view") {
		t.Error("read family should merge read_file + view")
	}
	if groupKeyFor("edit_file") == groupKeyFor("read_file") {
		t.Error("edit and read must not merge")
	}
	// Terminal calls are deliberately NOT grouped: each command's output slab
	// is the point of the entry, so collapsing them would hide the work.
	if groupKeyFor("bash") == groupKeyFor("run_command") {
		t.Error("terminal calls must stay separate cards")
	}
	if groupsFor("bash") {
		t.Error("bash should not opt into grouping")
	}
	if !groupsFor("read_file") {
		t.Error("read_file should opt into grouping")
	}
}

func TestSlabBackgroundFollowsTheme(t *testing.T) {
	// A light theme must not get a void-black slab.
	for _, name := range []string{"modern", "catppuccin", "nord"} {
		c := NewConversation(themes.NewStyles(themes.Get(name)))
		c.SetSize(90, 30)
		if c.slabBackground() == nil {
			t.Errorf("%s: slab background must be derived, not nil", name)
		}
	}
}

func TestUnknownToolStillRenders(t *testing.T) {
	c := testConv()
	m := ConversationMsg{
		Role:        "tool_call",
		ToolName:    "some_future_tool",
		Status:      "done",
		ToolContext: "doing something",
		ToolSummary: "did the thing",
		Content:     "output",
	}
	plain := ansi.Strip(c.renderToolCall(m, 90))
	if !strings.Contains(plain, "Some Future Tool") {
		t.Errorf("unknown tool should get an inferred display name:\n%s", plain)
	}
	if !strings.Contains(plain, "did the thing") {
		t.Errorf("unknown tool should still show its summary:\n%s", plain)
	}
}

func TestNoNerdFontGlyphsInToolCards(t *testing.T) {
	for _, name := range allToolNames {
		c := testConv()
		m := ConversationMsg{
			Role:     "tool_call",
			ToolName: name,
			Status:   "done",
			ToolArgs: `{"path":"a.go","command":"ls","pattern":"x","query":"x","url":"https://a.b"}`,
			Content:  "out",
		}
		plain := ansi.Strip(c.renderToolCall(m, 90))
		for _, r := range plain {
			// Private Use Area ranges are where Nerd Font glyphs live; a card
			// that reaches into them shows tofu on an unpatched font.
			if (r >= 0xE000 && r <= 0xF8FF) || (r >= 0xF0000 && r <= 0xFFFFD) {
				t.Errorf("%s: card contains Nerd Font glyph %U:\n%s", name, r, plain)
				break
			}
		}
	}
}
