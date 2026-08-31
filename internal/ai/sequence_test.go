package ai

import (
	"strings"
	"testing"
)

func TestValidateMessageSequenceValid(t *testing.T) {
	messages := []Message{
		{Role: RoleSystem, Content: []ContentPart{{Type: ContentTypeText, Text: "sys"}}},
		NewTextMessage(RoleUser, "find bug"),
		{
			Role: RoleAssistant,
			Content: []ContentPart{
				{Type: ContentTypeToolCall, ToolCall: &ToolCall{ID: "call_1", Name: "read_file", Args: map[string]any{"path": "main.go"}}},
			},
		},
		{
			Role: RoleTool,
			Content: []ContentPart{
				{Type: ContentTypeToolResult, ToolResult: &ToolResult{ToolCallID: "call_1", Content: "ok"}},
			},
		},
		NewTextMessage(RoleAssistant, "done"),
	}

	if err := ValidateMessageSequence(messages); err != nil {
		t.Fatalf("expected valid sequence, got error: %v", err)
	}
}

func TestValidateMessageSequenceInvalidTransitions(t *testing.T) {
	t.Run("consecutive assistant messages", func(t *testing.T) {
		err := ValidateMessageSequence([]Message{
			NewTextMessage(RoleUser, "a"),
			NewTextMessage(RoleAssistant, "b"),
			NewTextMessage(RoleAssistant, "c"),
		})
		if err == nil || !strings.Contains(err.Error(), "consecutive assistant messages") {
			t.Fatalf("expected consecutive assistant error, got %v", err)
		}
	})

	t.Run("tool without pending assistant call", func(t *testing.T) {
		err := ValidateMessageSequence([]Message{
			NewTextMessage(RoleUser, "a"),
			{
				Role: RoleTool,
				Content: []ContentPart{
					{Type: ContentTypeToolResult, ToolResult: &ToolResult{ToolCallID: "call_1", Content: "x"}},
				},
			},
		})
		if err == nil || !strings.Contains(err.Error(), "no pending assistant tool calls") {
			t.Fatalf("expected missing pending tool call error, got %v", err)
		}
	})

	t.Run("unknown tool_result id", func(t *testing.T) {
		err := ValidateMessageSequence([]Message{
			NewTextMessage(RoleUser, "a"),
			{
				Role: RoleAssistant,
				Content: []ContentPart{
					{Type: ContentTypeToolCall, ToolCall: &ToolCall{ID: "call_1", Name: "x", Args: map[string]any{}}},
				},
			},
			{
				Role: RoleTool,
				Content: []ContentPart{
					{Type: ContentTypeToolResult, ToolResult: &ToolResult{ToolCallID: "call_2", Content: "x"}},
				},
			},
		})
		if err == nil || !strings.Contains(err.Error(), "unknown tool_call_id") {
			t.Fatalf("expected unknown tool_call_id error, got %v", err)
		}
	})

	t.Run("assistant before all tool results returned", func(t *testing.T) {
		err := ValidateMessageSequence([]Message{
			NewTextMessage(RoleUser, "a"),
			{
				Role: RoleAssistant,
				Content: []ContentPart{
					{Type: ContentTypeToolCall, ToolCall: &ToolCall{ID: "call_1", Name: "x", Args: map[string]any{}}},
					{Type: ContentTypeToolCall, ToolCall: &ToolCall{ID: "call_2", Name: "y", Args: map[string]any{}}},
				},
			},
			{
				Role: RoleTool,
				Content: []ContentPart{
					{Type: ContentTypeToolResult, ToolResult: &ToolResult{ToolCallID: "call_1", Content: "x"}},
				},
			},
			NewTextMessage(RoleAssistant, "too soon"),
		})
		if err == nil || !strings.Contains(err.Error(), "cannot appear before all pending tool results") {
			t.Fatalf("expected pending tool result error, got %v", err)
		}
	})
}

func TestRepairMissingToolResults(t *testing.T) {
	messages := []Message{
		NewTextMessage(RoleUser, "inspect"),
		{Role: RoleAssistant, Content: []ContentPart{{Type: ContentTypeToolCall, ToolCall: &ToolCall{ID: "call_613309", Name: "read_file"}}}},
		NewTextMessage(RoleUser, "continue"),
	}

	repaired := RepairMissingToolResults(messages)
	if err := ValidateMessageSequence(repaired); err != nil {
		t.Fatalf("repaired sequence is invalid: %v", err)
	}
	if len(repaired) != 4 || repaired[2].Role != RoleTool {
		t.Fatalf("unexpected repaired sequence: %+v", repaired)
	}
	result := repaired[2].Content[0].ToolResult
	if result == nil || result.ToolCallID != "call_613309" || !result.IsError {
		t.Fatalf("unexpected synthetic result: %+v", result)
	}
}

// TestMergeConsecutiveAssistantAcrossSystem pins the compaction-shape repair:
// a system message between two assistant turns (compaction boundary marker,
// compacted summary, JIT load) does not make the pair legal. The validator
// skips system roles, so the merge must look past them or the request fails
// with INVALID_INPUT ("consecutive assistant messages").
func TestMergeConsecutiveAssistantAcrossSystem(t *testing.T) {
	messages := []Message{
		NewTextMessage(RoleUser, "hello"),
		NewTextMessage(RoleAssistant, "first reply"),
		NewTextMessage(RoleSystem, "── compaction boundary ──"),
		NewTextMessage(RoleAssistant, "second reply"),
	}

	merged := MergeConsecutiveAssistantMessages(messages)
	if err := ValidateMessageSequence(merged); err != nil {
		t.Fatalf("merged sequence must validate, got: %v", err)
	}

	// The system message keeps its position between the two assistant
	// messages; the second assistant message's content is merged into the
	// first, so the sequence is [user, assistant(+merged), system].
	if len(merged) != 3 {
		t.Fatalf("expected 3 messages after merge, got %d: %+v", len(merged), merged)
	}
	if merged[1].Role != RoleAssistant {
		t.Fatalf("expected assistant at [1], got %s", merged[1].Role)
	}
	if merged[2].Role != RoleSystem {
		t.Fatalf("expected system at [2], got %s", merged[2].Role)
	}
	if got := merged[1].TextContent(); got != "first replysecond reply" {
		t.Fatalf("assistant content must be merged, got %q", got)
	}
}

// TestMergeConsecutiveAssistantDirect pins the simple adjacent case: two
// assistant messages with nothing between them collapse into one.
func TestMergeConsecutiveAssistantDirect(t *testing.T) {
	messages := []Message{
		NewTextMessage(RoleUser, "hello"),
		NewTextMessage(RoleAssistant, "part one"),
		NewTextMessage(RoleAssistant, "part two"),
	}
	merged := MergeConsecutiveAssistantMessages(messages)
	if len(merged) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(merged))
	}
	if got := merged[1].TextContent(); got != "part onepart two" {
		t.Fatalf("expected merged text, got %q", got)
	}
	if err := ValidateMessageSequence(merged); err != nil {
		t.Fatalf("merged sequence must validate: %v", err)
	}
}

// TestMergePreservesUserSeparatedAssistants pins that a user message between
// two assistant messages still keeps them separate — the merge only bridges
// system messages, never user or tool turns.
func TestMergePreservesUserSeparatedAssistants(t *testing.T) {
	messages := []Message{
		NewTextMessage(RoleUser, "hello"),
		NewTextMessage(RoleAssistant, "reply one"),
		NewTextMessage(RoleUser, "follow-up"),
		NewTextMessage(RoleAssistant, "reply two"),
	}
	merged := MergeConsecutiveAssistantMessages(messages)
	if len(merged) != 4 {
		t.Fatalf("user-separated assistants must stay separate, got %d messages", len(merged))
	}
}

// TestMergeAcrossMultipleSystemMessages pins the worst compaction shape:
// several system markers between two assistant turns still merge.
func TestMergeAcrossMultipleSystemMessages(t *testing.T) {
	messages := []Message{
		NewTextMessage(RoleUser, "hello"),
		NewTextMessage(RoleAssistant, "a"),
		NewTextMessage(RoleSystem, "marker one"),
		NewTextMessage(RoleSystem, "marker two"),
		NewTextMessage(RoleAssistant, "b"),
	}
	merged := MergeConsecutiveAssistantMessages(messages)
	if err := ValidateMessageSequence(merged); err != nil {
		t.Fatalf("merged sequence must validate: %v", err)
	}
	// [user, assistant(a+b), system, system]
	if len(merged) != 4 {
		t.Fatalf("expected 4 messages, got %d", len(merged))
	}
	if got := merged[1].TextContent(); got != "ab" {
		t.Fatalf("expected merged text, got %q", got)
	}
}
