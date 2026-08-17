package session

import (
	"encoding/json"
	"strings"

	"github.com/iSundram/Automergent/internal/ai"
)

// defaultMaxSessionBytes is the ceiling for a persisted session file.
// Sessions larger than this have their oldest tool outputs truncated to
// summaries so history stays loadable and disk usage stays bounded.
const defaultMaxSessionBytes = 10 << 20

// maxToolResultBytes is the per-message tool-result size kept in full.
const maxToolResultBytes = 8 << 10

// compactSummarySuffix marks a truncated tool result.
const compactSummarySuffix = "\n… (truncated to keep session file bounded)"

// CompactForSize mutates a *snapshot* session (never the live session) so its
// marshaled size stays under maxBytes. Oldest tool/tool_result messages are
// truncated first; user and assistant text is preserved. Returns true if any
// content was truncated.
func CompactForSize(sess *Session, maxBytes int64) bool {
	if maxBytes <= 0 {
		maxBytes = defaultMaxSessionBytes
	}
	if sizeOf(sess) <= maxBytes {
		return false
	}

	truncated := false
	for i := range sess.Messages {
		m := &sess.Messages[i]
		if m.Role == ai.RoleUser || m.Role == ai.RoleAssistant {
			continue
		}
		for j := range m.Content {
			part := &m.Content[j]
			if part.ToolResult != nil && len(part.ToolResult.Content) > maxToolResultBytes {
				part.ToolResult.Content = summarize(part.ToolResult.Content)
				truncated = true
			}
			if part.Type == ai.ContentTypeText && len(part.Text) > maxToolResultBytes && m.Role != ai.RoleUser {
				part.Text = summarize(part.Text)
				truncated = true
			}
		}
		if sizeOf(sess) <= maxBytes {
			return truncated
		}
	}

	// Still over budget: drop old tool messages entirely, keeping a marker.
	for i := range sess.Messages {
		m := &sess.Messages[i]
		if m.Role == ai.RoleUser || m.Role == ai.RoleAssistant {
			continue
		}
		if len(m.Content) == 0 {
			continue
		}
		kept := m.Content[:0]
		for j := range m.Content {
			part := m.Content[j]
			if part.ToolCall != nil {
				kept = append(kept, part)
			} else if part.ToolResult != nil {
				part.ToolResult.Content = "… (tool result removed by session compaction)"
				kept = append(kept, part)
			} else if part.Type == ai.ContentTypeText {
				part.Text = "… (tool output removed by session compaction)"
				kept = append(kept, part)
			}
		}
		m.Content = kept
		truncated = true
		if sizeOf(sess) <= maxBytes {
			return truncated
		}
	}
	return truncated
}

func sizeOf(sess *Session) int64 {
	// Measure the pretty-printed form so the persisted file size honors the
	// budget exactly (MarshalIndent is what Storage.Save writes).
	data, err := json.MarshalIndent(sess, "", "  ")
	if err != nil {
		return 1 << 62
	}
	return int64(len(data))
}

func summarize(text string) string {
	const keep = 800
	text = strings.TrimSpace(text)
	if len(text) <= keep {
		return text
	}
	return text[:keep] + compactSummarySuffix
}
