package ai

import (
	"fmt"
	"slices"
)

// ValidateMessageSequence validates message ordering and tool-call linkage.
func ValidateMessageSequence(messages []Message) error {
	pendingToolCalls := map[string]struct{}{}
	var lastNonSystemRole Role

	for i, m := range messages {
		if err := m.Validate(); err != nil {
			return fmt.Errorf("message[%d] validation failed: %w", i, err)
		}

		if m.Role == RoleSystem {
			continue
		}

		if lastNonSystemRole == "" && m.Role != RoleUser {
			return fmt.Errorf("message[%d]: first non-system message must be role %q, got %q", i, RoleUser, m.Role)
		}

		switch m.Role {
		case RoleAssistant:
			if lastNonSystemRole == RoleAssistant {
				return fmt.Errorf("message[%d]: consecutive assistant messages are not allowed", i)
			}
			if len(pendingToolCalls) > 0 {
				return fmt.Errorf("message[%d]: assistant message cannot appear before all pending tool results are returned", i)
			}
			seenIDs := map[string]struct{}{}
			for j, p := range m.Content {
				if p.Type != ContentTypeToolCall || p.ToolCall == nil {
					continue
				}
				id := p.ToolCall.ID
				if _, exists := seenIDs[id]; exists {
					return fmt.Errorf("message[%d].content[%d]: duplicate tool_call id %q in assistant message", i, j, id)
				}
				seenIDs[id] = struct{}{}
				pendingToolCalls[id] = struct{}{}
			}

		case RoleTool:
			if len(pendingToolCalls) == 0 {
				return fmt.Errorf("message[%d]: tool message has no pending assistant tool calls", i)
			}
			toolResultCount := 0
			seenIDs := map[string]struct{}{}
			for j, p := range m.Content {
				if p.Type != ContentTypeToolResult || p.ToolResult == nil {
					continue
				}
				toolResultCount++
				id := p.ToolResult.ToolCallID
				if _, exists := seenIDs[id]; exists {
					return fmt.Errorf("message[%d].content[%d]: duplicate tool_result for tool_call_id %q", i, j, id)
				}
				if _, ok := pendingToolCalls[id]; !ok {
					return fmt.Errorf("message[%d].content[%d]: tool_result references unknown tool_call_id %q", i, j, id)
				}
				seenIDs[id] = struct{}{}
				delete(pendingToolCalls, id)
			}
			if toolResultCount == 0 {
				return fmt.Errorf("message[%d]: tool message must contain at least one tool_result content part", i)
			}

		case RoleUser:
			toolResultCount := 0
			seenIDs := map[string]struct{}{}
			for j, p := range m.Content {
				if p.Type != ContentTypeToolResult || p.ToolResult == nil {
					continue
				}
				toolResultCount++
				id := p.ToolResult.ToolCallID
				if _, exists := seenIDs[id]; exists {
					return fmt.Errorf("message[%d].content[%d]: duplicate tool_result for tool_call_id %q", i, j, id)
				}
				if _, ok := pendingToolCalls[id]; !ok {
					return fmt.Errorf("message[%d].content[%d]: tool_result references unknown tool_call_id %q", i, j, id)
				}
				seenIDs[id] = struct{}{}
				delete(pendingToolCalls, id)
			}
			if len(pendingToolCalls) > 0 && toolResultCount == 0 {
				return fmt.Errorf("message[%d]: user message cannot appear before all pending tool results are returned", i)
			}
		}

		lastNonSystemRole = m.Role
	}

	if len(pendingToolCalls) > 0 {
		missing := make([]string, 0, len(pendingToolCalls))
		for id := range pendingToolCalls {
			missing = append(missing, id)
		}
		slices.Sort(missing)
		return fmt.Errorf("conversation ended with missing tool results for tool_call ids: %v", missing)
	}

	return nil
}
