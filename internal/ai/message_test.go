package ai

import (
	"math"
	"strings"
	"testing"
)

func TestMessageValidateRejectsMalformedInputs(t *testing.T) {
	t.Run("invalid role", func(t *testing.T) {
		msg := Message{Role: Role("bad"), Content: []ContentPart{{Type: ContentTypeText, Text: "x"}}}
		if err := msg.Validate(); err == nil || !strings.Contains(err.Error(), "invalid role") {
			t.Fatalf("expected invalid role error, got %v", err)
		}
	})

	t.Run("assistant cannot contain tool_result", func(t *testing.T) {
		msg := Message{
			Role: RoleAssistant,
			Content: []ContentPart{{
				Type:       ContentTypeToolResult,
				ToolResult: &ToolResult{ToolCallID: "call_1", Content: "ok"},
			}},
		}
		if err := msg.Validate(); err == nil || !strings.Contains(err.Error(), "tool_result content is only valid for tool or user messages") {
			t.Fatalf("expected role mismatch error, got %v", err)
		}
	})

	t.Run("non-serializable tool args", func(t *testing.T) {
		msg := Message{
			Role: RoleAssistant,
			Content: []ContentPart{{
				Type: ContentTypeToolCall,
				ToolCall: &ToolCall{
					ID:   "call_1",
					Name: "bad",
					Args: map[string]any{"n": math.NaN()},
				},
			}},
		}
		if err := msg.Validate(); err == nil || !strings.Contains(err.Error(), "not JSON serializable") {
			t.Fatalf("expected serialization error, got %v", err)
		}
	})
}

func TestValidateMessageSequenceRejectsMalformedOrder(t *testing.T) {
	err := ValidateMessageSequence([]Message{
		{
			Role: RoleTool,
			Content: []ContentPart{{
				Type:       ContentTypeToolResult,
				ToolResult: &ToolResult{ToolCallID: "call_1", Content: "x"},
			}},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "first non-system message must be role") {
		t.Fatalf("expected first-tool-message error, got %v", err)
	}

	err = ValidateMessageSequence([]Message{
		NewTextMessage(RoleUser, "start"),
		{
			Role: RoleAssistant,
			Content: []ContentPart{{
				Type: ContentTypeToolCall,
				ToolCall: &ToolCall{
					ID:   "call_1",
					Name: "x",
					Args: map[string]any{},
				},
			}},
		},
		{
			Role: RoleTool,
			Content: []ContentPart{{
				Type:       ContentTypeToolResult,
				ToolResult: &ToolResult{ToolCallID: "unknown", Content: "x"},
			}},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "unknown tool_call_id") {
		t.Fatalf("expected unknown tool id error, got %v", err)
	}
}
