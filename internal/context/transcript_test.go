package context

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/iSundram/Automergent/internal/ai"
)

func TestTranscriptAppendAndReconstruct(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "t.jsonl")
	tr := NewTranscript(path)

	user := ai.NewTextMessage(ai.RoleUser, "fix the bug")
	tr.Append(user, "turn1")

	assistant := ai.Message{
		Role: ai.RoleAssistant,
		Content: []ai.ContentPart{
			{Type: ai.ContentTypeToolCall, ToolCall: &ai.ToolCall{ID: "c1", Name: "grep", Args: map[string]any{"pattern": "bug"}}},
		},
	}
	tr.Append(assistant, "turn2")

	toolRes := ai.Message{
		Role: ai.RoleTool,
		Content: []ai.ContentPart{{
			Type: ai.ContentTypeToolResult,
			ToolResult: &ai.ToolResult{ToolCallID: "c1", Content: "found"},
		}},
	}
	tr.Append(toolRes, "turn2")

	if tr.Len() != 3 {
		t.Fatalf("expected 3 items, got %d", tr.Len())
	}

	msgs := tr.ToMessages()
	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(msgs))
	}
	if msgs[0].TextContent() != "fix the bug" {
		t.Fatalf("unexpected first message: %q", msgs[0].TextContent())
	}

	// Reload from disk.
	tr2 := NewTranscript(path)
	if tr2.Len() != 3 {
		t.Fatalf("expected 3 items after reload, got %d", tr2.Len())
	}
	if len(tr2.PristineItems()) != 3 {
		t.Fatalf("expected 3 pristine items, got %d", len(tr2.PristineItems()))
	}
}

func TestTranscriptRollback(t *testing.T) {
	tr := NewTranscript("")
	tr.Append(ai.NewTextMessage(ai.RoleUser, "a"), "t1")
	tr.Append(ai.NewTextMessage(ai.RoleUser, "b"), "t2")
	tr.Rollback(1)
	if tr.Len() != 1 {
		t.Fatalf("expected 1 item after rollback, got %d", tr.Len())
	}
	if got := tr.Items()[0].Role; got != ai.RoleUser {
		t.Fatalf("unexpected role: %s", got)
	}
}

func TestNormalizeMessagesForAPI(t *testing.T) {
	msgs := []ai.Message{
		ai.NewTextMessage(ai.RoleUser, "hello"),
		{ // assistant with tool calls, no text
			Role: ai.RoleAssistant,
			Content: []ai.ContentPart{
				{Type: ai.ContentTypeToolCall, ToolCall: &ai.ToolCall{ID: "c1", Name: "read"}},
			},
		},
	}
	// The assistant stub has no results; normalization should drop it.
	normalized := NormalizeMessagesForAPI(msgs)
	if len(normalized) != 1 {
		t.Fatalf("expected 1 message after normalization, got %d", len(normalized))
	}

	// With a tool result present, both should survive.
	msgs2 := []ai.Message{
		ai.NewTextMessage(ai.RoleUser, "hello"),
		{
			Role: ai.RoleAssistant,
			Content: []ai.ContentPart{
				{Type: ai.ContentTypeToolCall, ToolCall: &ai.ToolCall{ID: "c1", Name: "read"}},
			},
		},
		{
			Role: ai.RoleTool,
			Content: []ai.ContentPart{{
				Type: ai.ContentTypeToolResult,
				ToolResult: &ai.ToolResult{ToolCallID: "c1", Content: "data"},
			}},
		},
	}
	normalized = NormalizeMessagesForAPI(msgs2)
	if len(normalized) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(normalized))
	}
}

func TestSynthMissingToolResults(t *testing.T) {
	msgs := []ai.Message{
		{
			Role: ai.RoleAssistant,
			Content: []ai.ContentPart{
				{Type: ai.ContentTypeToolCall, ToolCall: &ai.ToolCall{ID: "c1", Name: "edit"}},
			},
		},
	}
	msgs = SynthMissingToolResults(msgs)
	if len(msgs) != 2 {
		t.Fatalf("expected synthetic result appended, got %d messages", len(msgs))
	}
	last := msgs[len(msgs)-1]
	if last.Role != ai.RoleTool {
		t.Fatalf("expected tool role, got %s", last.Role)
	}
}

func TestMergeConsecutiveUser(t *testing.T) {
	msgs := []ai.Message{
		ai.NewTextMessage(ai.RoleUser, "one"),
		ai.NewTextMessage(ai.RoleUser, "two"),
		ai.NewTextMessage(ai.RoleAssistant, "ok"),
	}
	out := mergeConsecutiveUser(msgs)
	if len(out) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(out))
	}
	if out[0].TextContent() != "onetwo" {
		t.Fatalf("expected merged text, got %q", out[0].TextContent())
	}
}

func TestTranscriptPersistenceRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.jsonl")
	tr := NewTranscript(path)
	tr.Append(ai.NewTextMessage(ai.RoleUser, "persist me"), "t1")

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read transcript file: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("expected non-empty transcript file")
	}
}