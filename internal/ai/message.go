package ai

import "strings"

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
