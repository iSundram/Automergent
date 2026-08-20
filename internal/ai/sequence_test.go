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
