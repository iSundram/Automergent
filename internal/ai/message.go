package ai

import (
	"encoding/json"
	"fmt"
	"strings"
)

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
	sb.WriteString(m.TextContent())
	for _, p := range m.Content {
		switch p.Type {
		case ContentTypeToolCall:
			if p.ToolCall != nil {
				b, _ := json.Marshal(p.ToolCall)
				sb.WriteString("\n[tool_call] ")
				sb.Write(b)
			}
		case ContentTypeToolResult:
			if p.ToolResult != nil {
				sb.WriteString(fmt.Sprintf("\n[tool_result id=%s]\n%s", p.ToolResult.ToolCallID, p.ToolResult.Content))
			}
		}
	}
	return sb.String()
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
