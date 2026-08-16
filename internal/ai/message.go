package ai

import (
	"encoding/json"
	"fmt"
	"strings"
)

func isValidRole(role Role) bool {
	switch role {
	case RoleSystem, RoleUser, RoleAssistant, RoleTool:
		return true
	default:
		return false
	}
}

func isValidContentType(contentType ContentType) bool {
	switch contentType {
	case ContentTypeText, ContentTypeThought, ContentTypeImage, ContentTypeToolCall, ContentTypeToolResult:
		return true
	default:
		return false
	}
}

// Validate checks a message for role/content consistency and malformed values.
func (m Message) Validate() error {
	if !isValidRole(m.Role) {
		return fmt.Errorf("invalid role %q", m.Role)
	}
	for i, p := range m.Content {
		if !isValidContentType(p.Type) {
			return fmt.Errorf("content[%d]: invalid content type %q", i, p.Type)
		}
		switch p.Type {
		case ContentTypeThought:
			if m.Role != RoleAssistant {
				return fmt.Errorf("content[%d]: thought content is only valid for assistant messages", i)
			}
		case ContentTypeToolCall:
			if m.Role != RoleAssistant {
				return fmt.Errorf("content[%d]: tool_call content is only valid for assistant messages", i)
			}
			if p.ToolCall == nil {
				return fmt.Errorf("content[%d]: tool_call content missing ToolCall payload", i)
			}
			if p.ToolCall.ID == "" {
				return fmt.Errorf("content[%d]: tool_call id is required", i)
			}
			if p.ToolCall.Name == "" {
				return fmt.Errorf("content[%d]: tool_call name is required", i)
			}
			if _, err := json.Marshal(p.ToolCall.Args); err != nil {
				return fmt.Errorf("content[%d]: tool_call args are not JSON serializable: %w", i, err)
			}
		case ContentTypeToolResult:
			if m.Role != RoleTool && m.Role != RoleUser {
				return fmt.Errorf("content[%d]: tool_result content is only valid for tool or user messages", i)
			}
			if p.ToolResult == nil {
				return fmt.Errorf("content[%d]: tool_result content missing ToolResult payload", i)
			}
			if p.ToolResult.ToolCallID == "" {
				return fmt.Errorf("content[%d]: tool_result tool_call_id is required", i)
			}
		case ContentTypeImage:
			if p.ImageURL == "" {
				return fmt.Errorf("content[%d]: image content missing image URL", i)
			}
		}
	}
	return nil
}

// TextContent concatenates all text parts of a message.
func (m Message) TextContent() string {
	var sb strings.Builder
	for _, p := range m.Content {
		if p.Type == ContentTypeText {
			sb.WriteString(p.Text)
		}
	}
	return sb.String()
}

// PlaintextForHistory renders a message as plain text for providers that do not
// support native tool roles (e.g. Gemini text-only mode). It includes text plus
// serialized tool calls and tool results so multi-turn agent history is preserved.
func (m Message) PlaintextForHistory() string {
	var sb strings.Builder
	for _, p := range m.Content {
		switch p.Type {
		case ContentTypeText:
			sb.WriteString(p.Text)
		case ContentTypeThought:
			sb.WriteString("\n[thought]\n")
			sb.WriteString(p.Thought)
			sb.WriteString("\n[/thought]\n")
		case ContentTypeToolCall:
			if p.ToolCall != nil {
				b, err := json.Marshal(p.ToolCall)
				sb.WriteString("\n[tool_call] ")
				if err != nil {
					sb.WriteString(`{"error":"invalid_tool_call_payload"}`)
				} else {
					sb.Write(b)
				}
			}
		case ContentTypeToolResult:
			if p.ToolResult != nil {
				sb.WriteString(fmt.Sprintf("\n[tool_result id=%s]\n%s", p.ToolResult.ToolCallID, p.ToolResult.Content))
			}
		}
	}
	return sb.String()
}

// Plaintext returns plain text representation for token estimation.
func (m Message) Plaintext() string {
	return m.PlaintextForHistory()
}

// ToolCalls returns all tool-call parts in a message.
func (m Message) ToolCallParts() []ToolCall {
	var calls []ToolCall
	for _, p := range m.Content {
		if p.Type == ContentTypeToolCall && p.ToolCall != nil {
			calls = append(calls, *p.ToolCall)
		}
	}
	return calls
}

// HasToolCalls reports whether the message contains any tool calls.
func (m Message) HasToolCalls() bool {
	for _, p := range m.Content {
		if p.Type == ContentTypeToolCall {
			return true
		}
	}
	return false
}

// AppendText appends a text part to the message.
func (m *Message) AppendText(text string) {
	m.Content = append(m.Content, ContentPart{Type: ContentTypeText, Text: text})
}

// ApproximateTokenCount estimates the token count for a slice of messages.
// It uses the rough approximation of 1 token per 4 characters, including
// serialized tool calls/results from PlaintextForHistory.
// For accurate counts, use provider-specific tokenizers such as tiktoken.
func ApproximateTokenCount(messages []Message) int {
	total := 0
	for _, m := range messages {
		total += len(m.PlaintextForHistory()) / 4
	}
	return total
}
