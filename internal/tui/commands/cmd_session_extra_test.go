package commands

import (
	"strings"
	"testing"
	"time"
)

func TestRenameRequiresTitle(t *testing.T) {
	m := NewMockHost()
	handleRename(m, nil)
	if len(m.usageMessages) == 0 {
		t.Fatal("expected usage message when title missing")
	}
	if len(m.renamedTitles) != 0 {
		t.Fatal("rename must not run without a title")
	}
}

func TestRenameSetsSessionTitle(t *testing.T) {
	m := NewMockHost()
	handleRename(m, []string{"fix", "flaky", "tests"})
	if len(m.renamedTitles) != 1 || m.renamedTitles[0] != "fix flaky tests" {
		t.Fatalf("unexpected renames: %v", m.renamedTitles)
	}
	if !strings.Contains(m.systemMessages[0], `"fix flaky tests"`) {
		t.Fatalf("missing confirmation: %v", m.systemMessages)
	}
}

func TestRecapEmptySession(t *testing.T) {
	m := NewMockHost()
	handleRecap(m, nil)
	if !strings.Contains(m.systemMessages[0], "Nothing to recap") {
		t.Fatalf("expected empty-state message, got %q", m.systemMessages[0])
	}
}

func TestRecapSummarizesActivity(t *testing.T) {
	m := NewMockHost()
	m.recap = RecapInfo{
		UserTurns:       3,
		AssistantTurns:  2,
		ToolCalls:       4,
		ToolsUsed:       []string{"shell", "edit_file"},
		LastUserMessage: "make the parser handle unicode",
		StartedAt:       time.Date(2026, 8, 22, 14, 2, 0, 0, time.UTC),
	}

	handleRecap(m, nil)
	out := m.systemMessages[0]
	for _, want := range []string{"3 user turns", "2 replies", "4 tool calls", "shell, edit_file", `Last request: "make the parser handle unicode"`} {
		if !strings.Contains(out, want) {
			t.Fatalf("recap missing %q:\n%s", want, out)
		}
	}
}

func TestRecapTruncatesLongLastMessage(t *testing.T) {
	m := NewMockHost()
	m.recap = RecapInfo{UserTurns: 1, LastUserMessage: strings.Repeat("x", 300)}

	handleRecap(m, nil)
	out := m.systemMessages[0]
	if !strings.Contains(out, "...\"") || len(out) > 600 {
		t.Fatalf("long last message not truncated:\n%s", out)
	}
}

func TestMemoryListsSurfaces(t *testing.T) {
	dir := t.TempDir()
	m := NewMockHost()
	m.workDir = dir
	m.globalConfigPath = "/nonexistent/global.yaml"
	m.projectConfigPath = "/nonexistent/project.yaml"

	handleMemory(m, nil)
	out := m.systemMessages[0]
	// memoryPage renders missing surfaces as warn-flagged rows ("! label").
	if !strings.Contains(out, "! Global config") || !strings.Contains(out, "! Project memory") {
		t.Fatalf("missing surfaces:\n%s", out)
	}
}
