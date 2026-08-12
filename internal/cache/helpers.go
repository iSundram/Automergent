package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"

	"github.com/iSundram/Automergent/internal/ai"
)

// hashToolSchemas generates a stable hash for tool schemas.
func hashToolSchemas(tools []ai.ToolSchema) string {
	// Sort tools by name for deterministic hashing
	sorted := make([]ai.ToolSchema, len(tools))
	copy(sorted, tools)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Name < sorted[j].Name
	})

	var sb strings.Builder
	for _, t := range sorted {
		sb.WriteString(t.Name)
		sb.WriteString(t.Description)
		if t.Parameters != nil {
			data, _ := json.Marshal(t.Parameters)
			sb.Write(data)
		}
	}

	h := sha256.Sum256([]byte(sb.String()))
	return hex.EncodeToString(h[:])
}

// hashMessages generates a stable hash for message history.
func hashMessages(messages []ai.Message) string {
	var sb strings.Builder
	for _, m := range messages {
		sb.WriteString(string(m.Role))
		for _, p := range m.Content {
			sb.WriteString(string(p.Type))
			sb.WriteString(p.Text)
			sb.WriteString(p.Thought)
			if p.ToolCall != nil {
				sb.WriteString(p.ToolCall.ID)
				sb.WriteString(p.ToolCall.Name)
			}
			if p.ToolResult != nil {
				sb.WriteString(p.ToolResult.ToolCallID)
				sb.WriteString(p.ToolResult.Content)
			}
		}
	}

	h := sha256.Sum256([]byte(sb.String()))
	return hex.EncodeToString(h[:])
}

// estimateToolSchemaSize estimates the memory size of tool schemas.
func estimateToolSchemaSize(tools []ai.ToolSchema) int64 {
	var size int64
	for _, t := range tools {
		size += int64(len(t.Name) + len(t.Description))
		if t.Parameters != nil {
			data, _ := json.Marshal(t.Parameters)
			size += int64(len(data))
		}
	}
	return size
}

// estimateMessagesSize estimates the memory size of messages.
func estimateMessagesSize(messages []ai.Message) int64 {
	var size int64
	for _, m := range messages {
		size += int64(len(m.Role))
		for _, p := range m.Content {
			size += int64(len(p.Type))
			size += int64(len(p.Text))
			size += int64(len(p.Thought))
			size += int64(len(p.ImageURL))
			if p.ToolCall != nil {
				size += int64(len(p.ToolCall.ID) + len(p.ToolCall.Name))
				data, _ := json.Marshal(p.ToolCall.Args)
				size += int64(len(data))
			}
			if p.ToolResult != nil {
				size += int64(len(p.ToolResult.ToolCallID) + len(p.ToolResult.Content))
			}
		}
	}
	return size
}

// CreateCacheKey creates a deterministic cache key from components.
func CreateCacheKey(components ...string) string {
	joined := strings.Join(components, "\x00")
	h := sha256.Sum256([]byte(joined))
	return hex.EncodeToString(h[:16]) // Use first 16 bytes for shorter keys
}

// NormalizeCacheContent normalizes content for consistent caching.
func NormalizeCacheContent(content string) string {
	// Normalize whitespace
	content = strings.TrimSpace(content)

	// Normalize line endings
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")

	// Remove trailing whitespace from lines
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t")
	}

	return strings.Join(lines, "\n")
}

// ExtractCacheablePrefix extracts the cacheable prefix from a message list.
// Returns the prefix messages and the index of the first non-cacheable message.
func ExtractCacheablePrefix(messages []ai.Message) ([]ai.Message, int) {
	detector := NewBoundaryDetector()
	cacheableEnd := 0

	for i, m := range messages {
		// System messages are always cacheable
		if m.Role == ai.RoleSystem {
			cacheableEnd = i + 1
			continue
		}

		// Check if message content is dynamic
		isDynamic := false
		for _, p := range m.Content {
			if p.Type == ai.ContentTypeText {
				classification := detector.ClassifyContent(p.Text)
				if classification == ClassificationDynamic || classification == ClassificationVolatile {
					isDynamic = true
					break
				}
			}
		}

		if isDynamic {
			break
		}
		cacheableEnd = i + 1
	}

	return messages[:cacheableEnd], cacheableEnd
}

// MergeContentBlocks merges adjacent content blocks with the same cache control.
func MergeContentBlocks(blocks []ContentBlock) []ContentBlock {
	if len(blocks) <= 1 {
		return blocks
	}

	var merged []ContentBlock
	current := blocks[0]

	for i := 1; i < len(blocks); i++ {
		b := blocks[i]
		// Merge if same cache control settings
		if cacheControlEqual(current.CacheControl, b.CacheControl) && current.Type == b.Type {
			current.Text += b.Text
		} else {
			merged = append(merged, current)
			current = b
		}
	}
	merged = append(merged, current)

	return merged
}

// cacheControlEqual compares two cache controls for equality.
func cacheControlEqual(a, b *CacheControl) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return a.Type == b.Type && a.TTL == b.TTL && a.Scope == b.Scope
}

// EstimateTokens estimates the number of tokens in content.
// Uses the conservative estimate of 4 bytes per token.
func EstimateTokens(content string) int {
	return len(content) / 4
}

// ShouldCache determines if content is worth caching based on size and classification.
func ShouldCache(content string, classification ContentClassification) bool {
	// Don't cache volatile content
	if classification == ClassificationVolatile {
		return false
	}

	// Don't cache very small content (overhead not worth it)
	if len(content) < 100 {
		return false
	}

	// Don't cache very large content (memory pressure)
	if len(content) > 1024*1024 { // 1MB
		return false
	}

	return true
}

// GetCacheControlForMessage determines cache control for a message.
func GetCacheControlForMessage(msg ai.Message, isLastInBatch bool) *CacheControl {
	// System messages always get long TTL
	if msg.Role == ai.RoleSystem {
		return LongTTLCacheControl()
	}

	// Only cache last message in a sequence (prompt-cache pattern)
	if !isLastInBatch {
		return nil
	}

	// Tool results are semi-static
	for _, p := range msg.Content {
		if p.Type == ai.ContentTypeToolResult {
			return DefaultCacheControl()
		}
	}

	// Regular messages get default TTL
	return DefaultCacheControl()
}
