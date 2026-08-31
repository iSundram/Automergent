package ai

import (
	"fmt"
	"slices"
)

// MergeConsecutiveAssistantMessages merges back-to-back assistant messages into
// one. Consecutive assistant turns are illegal for all providers but can appear
// after an interrupted agentic turn (partial assistant message saved, then
// another assistant message appended on retry) or after autocompact rewrites.
// Merging their content parts is safe because providers treat them as a single
// turn regardless.
//
// System messages between two assistant turns do not make the pair legal: the
// validator skips system roles, and mid-history system messages (compaction
// boundary markers, compacted summaries, JIT loads, stall nudges) routinely
// sit between assistant turns. The merge therefore looks past system messages
// when deciding whether two assistant messages are consecutive — but never
// reorders them: the injected system content stays between the merged parts.
func MergeConsecutiveAssistantMessages(messages []Message) []Message {
	if len(messages) == 0 {
		return messages
	}
	out := make([]Message, 0, len(messages))
	// lastAssistant is the index in out of the most recent assistant message
	// with only system messages after it; -1 when none qualifies.
	lastAssistant := -1
	for _, m := range messages {
		switch {
		case m.Role == RoleAssistant && lastAssistant >= 0:
			// Merge content parts into the earlier assistant message. The
			// system messages between them keep their position: appended
			// content lands after them, which is where the second assistant
			// message would have been anyway.
			out[lastAssistant].Content = append(out[lastAssistant].Content, m.Content...)
			// Metadata from the dropped message (e.g. google_parts) is
			// stale for the merged message; the earlier message's metadata
			// stays authoritative.
		case m.Role == RoleAssistant:
			out = append(out, m)
			lastAssistant = len(out) - 1
		case m.Role == RoleSystem:
			// Keeps the merge eligible across system messages.
			out = append(out, m)
		default:
			out = append(out, m)
			lastAssistant = -1
		}
	}
	return out
}

// RepairMissingToolResults inserts explicit error results for dangling tool
// calls. This repairs sessions interrupted before the tool-result message was
// persisted, allowing strict providers such as Google to accept the next turn.
func RepairMissingToolResults(messages []Message) []Message {
	if len(messages) == 0 {
		return messages
	}

	out := make([]Message, 0, len(messages))
	pending := make(map[string]ToolCall)
	flush := func() {
		if len(pending) == 0 {
			return
		}
		ids := make([]string, 0, len(pending))
		for id := range pending {
			ids = append(ids, id)
		}
		slices.Sort(ids)
		parts := make([]ContentPart, 0, len(ids))
		for _, id := range ids {
			call := pending[id]
			parts = append(parts, ContentPart{
				Type: ContentTypeToolResult,
				ToolResult: &ToolResult{
					ToolCallID: id,
					Content:    fmt.Sprintf("[synthetic] tool %q was interrupted before producing a result", call.Name),
					IsError:    true,
				},
			})
		}
		out = append(out, Message{Role: RoleTool, Content: parts})
		pending = make(map[string]ToolCall)
	}

	for _, message := range messages {
		switch message.Role {
		case RoleAssistant:
			flush()
			out = append(out, message)
			for _, call := range message.ToolCallParts() {
				pending[call.ID] = call
			}
		case RoleTool:
			out = append(out, message)
			for _, part := range message.Content {
				if part.Type == ContentTypeToolResult && part.ToolResult != nil {
					delete(pending, part.ToolResult.ToolCallID)
				}
			}
		case RoleUser:
			if len(pending) > 0 {
				for _, part := range message.Content {
					if part.Type == ContentTypeToolResult && part.ToolResult != nil {
						delete(pending, part.ToolResult.ToolCallID)
					}
				}
				flush()
			}
			out = append(out, message)
		default:
			flush()
			out = append(out, message)
		}
	}
	flush()
	return out
}

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
