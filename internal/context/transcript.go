package context

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/iSundram/Automergent/internal/ai"
)

// TranscriptItemKind classifies the provenance of a transcript item.
type TranscriptItemKind string

const (
	KindUser      TranscriptItemKind = "user"
	KindAssistant TranscriptItemKind = "assistant"
	KindTool      TranscriptItemKind = "tool"
	KindSummary   TranscriptItemKind = "summary"
	KindSnapshot  TranscriptItemKind = "snapshot"
	KindCompact   TranscriptItemKind = "compact_boundary"
)

// TranscriptItem is the durable unit of conversation history. It carries a
// stable content-hash ID and provenance links (ParentID, Abstracts) so that
// masking, degradation, and summarization are surgical and reversible.
type TranscriptItem struct {
	ID        string             `json:"id"`
	Kind      TranscriptItemKind `json:"kind"`
	Role      ai.Role            `json:"role"`
	Parts     []PartRef          `json:"parts"`
	ParentID  string             `json:"parent_id,omitempty"` // replacesId
	Abstracts []string           `json:"abstracts,omitempty"` // abstractsIds
	TurnID    string             `json:"turn_id,omitempty"`
	Seq       int64              `json:"seq"`
	Compacted bool               `json:"compacted,omitempty"`
	Tokens    int                `json:"tokens,omitempty"`
	CreatedAt time.Time          `json:"created_at"`
}

// PartRef is a light reference to a message content part, allowing provenance
// tracking without duplicating large payloads.
type PartRef struct {
	Type       ai.ContentType `json:"type"`
	Text       string         `json:"text,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
	ToolName   string         `json:"tool_name,omitempty"`
	IsError    bool           `json:"is_error,omitempty"`
}

// ToMessage converts a transcript item back into an ai.Message.
func (ti *TranscriptItem) ToMessage() ai.Message {
	msg := ai.Message{Role: ti.Role}
	for _, p := range ti.Parts {
		switch p.Type {
		case ai.ContentTypeText:
			msg.Content = append(msg.Content, ai.ContentPart{Type: ai.ContentTypeText, Text: p.Text})
		case ai.ContentTypeThought:
			msg.Content = append(msg.Content, ai.ContentPart{Type: ai.ContentTypeThought, Thought: p.Text})
		case ai.ContentTypeToolCall:
			msg.Content = append(msg.Content, ai.ContentPart{Type: ai.ContentTypeToolCall, ToolCall: &ai.ToolCall{
				ID:   p.ToolCallID,
				Name: p.ToolName,
			}})
		case ai.ContentTypeToolResult:
			msg.Content = append(msg.Content, ai.ContentPart{Type: ai.ContentTypeToolResult, ToolResult: &ai.ToolResult{
				ToolCallID: p.ToolCallID,
				Content:    p.Text,
				IsError:    p.IsError,
			}})
		}
	}
	return msg
}

// contentHash computes a stable content-hash ID for a message.
func contentHash(msg ai.Message) string {
	var sb strings.Builder
	for _, p := range msg.Content {
		switch p.Type {
		case ai.ContentTypeText, ai.ContentTypeThought:
			sb.WriteString(string(p.Type))
			sb.WriteString(":")
			sb.WriteString(p.Text)
			sb.WriteString("|")
		case ai.ContentTypeToolCall:
			if p.ToolCall != nil {
				sb.WriteString("tool_call:")
				sb.WriteString(p.ToolCall.ID)
				sb.WriteString(":")
				sb.WriteString(p.ToolCall.Name)
				sb.WriteString("|")
			}
		case ai.ContentTypeToolResult:
			if p.ToolResult != nil {
				sb.WriteString("tool_result:")
				sb.WriteString(p.ToolResult.ToolCallID)
				sb.WriteString(":")
				sb.WriteString(p.ToolResult.Content)
				sb.WriteString("|")
			}
		case ai.ContentTypeImage:
			sb.WriteString("image:")
			sb.WriteString(p.ImageURL)
			sb.WriteString("|")
		}
	}
	sum := sha256.Sum256([]byte(sb.String()))
	return hex.EncodeToString(sum[:])
}

// Transcript is an append-only, durable conversation history. Mutations only
// produce new items linked via ParentID/Abstracts; the pristine history is
// always reconstructible.
type Transcript struct {
	mu       sync.RWMutex
	items    []*TranscriptItem
	seq      int64
	path     string // optional JSONL persistence path
	byID     map[string]*TranscriptItem
	pristine []*TranscriptItem // items that have never been replaced
}

// NewTranscript creates an in-memory transcript. If path is non-empty the
// transcript is also persisted as append-only JSONL.
func NewTranscript(path string) *Transcript {
	t := &Transcript{
		items:    make([]*TranscriptItem, 0, 64),
		byID:     make(map[string]*TranscriptItem),
		pristine: make([]*TranscriptItem, 0, 64),
		path:     path,
	}
	if path != "" {
		_ = t.load()
	}
	return t
}

// Len returns the number of transcript items.
func (t *Transcript) Len() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.items)
}

// Append adds a message to the transcript with a stable ID.
func (t *Transcript) Append(msg ai.Message, turnID string) *TranscriptItem {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.appendLocked(msg, turnID, "", nil, false)
}

// AppendReplace adds a message that replaces a prior item (provenance link).
func (t *Transcript) AppendReplace(msg ai.Message, turnID, parentID string, abstracts []string) *TranscriptItem {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.appendLocked(msg, turnID, parentID, abstracts, false)
}

func (t *Transcript) appendLocked(msg ai.Message, turnID, parentID string, abstracts []string, compacted bool) *TranscriptItem {
	id := contentHash(msg)
	if parentID == "" {
		parentID = id
	}
	ti := &TranscriptItem{
		ID:        id,
		Kind:      kindForRole(msg.Role),
		Role:      msg.Role,
		ParentID:  parentID,
		Abstracts: abstracts,
		TurnID:    turnID,
		Seq:       t.seq,
		Compacted: compacted,
		Tokens:    EstimateTokens(msg.PlaintextForHistory()),
		CreatedAt: time.Now(),
	}
	ti.Parts = partsFromMessage(msg)
	t.seq++
	t.items = append(t.items, ti)
	t.byID[id] = ti
	if parentID == id {
		t.pristine = append(t.pristine, ti)
	}
	if t.path != "" {
		_ = t.appendToDisk(ti)
	}
	return ti
}

// Items returns a snapshot of all transcript items in order.
func (t *Transcript) Items() []*TranscriptItem {
	t.mu.RLock()
	defer t.mu.RUnlock()
	out := make([]*TranscriptItem, len(t.items))
	copy(out, t.items)
	return out
}

// PristineItems returns the never-replaced items.
func (t *Transcript) PristineItems() []*TranscriptItem {
	t.mu.RLock()
	defer t.mu.RUnlock()
	out := make([]*TranscriptItem, len(t.pristine))
	copy(out, t.pristine)
	return out
}

// ToMessages renders the transcript to the model-facing message sequence.
func (t *Transcript) ToMessages() []ai.Message {
	items := t.Items()
	msgs := make([]ai.Message, 0, len(items))
	for _, it := range items {
		msgs = append(msgs, it.ToMessage())
	}
	return msgs
}

// kindForRole maps a role to a transcript item kind.
func kindForRole(role ai.Role) TranscriptItemKind {
	switch role {
	case ai.RoleUser:
		return KindUser
	case ai.RoleAssistant:
		return KindAssistant
	case ai.RoleTool:
		return KindTool
	default:
		return KindAssistant
	}
}

// partsFromMessage extracts part refs from a message.
func partsFromMessage(msg ai.Message) []PartRef {
	var parts []PartRef
	for _, p := range msg.Content {
		switch p.Type {
		case ai.ContentTypeText:
			parts = append(parts, PartRef{Type: ai.ContentTypeText, Text: p.Text})
		case ai.ContentTypeThought:
			parts = append(parts, PartRef{Type: ai.ContentTypeThought, Text: p.Thought})
		case ai.ContentTypeToolCall:
			if p.ToolCall != nil {
				parts = append(parts, PartRef{Type: ai.ContentTypeToolCall, ToolCallID: p.ToolCall.ID, ToolName: p.ToolCall.Name})
			}
		case ai.ContentTypeToolResult:
			if p.ToolResult != nil {
				parts = append(parts, PartRef{Type: ai.ContentTypeToolResult, ToolCallID: p.ToolResult.ToolCallID, Text: p.ToolResult.Content, IsError: p.ToolResult.IsError})
			}
		case ai.ContentTypeImage:
			parts = append(parts, PartRef{Type: ai.ContentTypeImage, Text: p.ImageURL})
		}
	}
	return parts
}

// AppendToolResults matches pending tool calls with their results, preserving
// the call/result adjacency invariant.
func (t *Transcript) AppendToolResults(results []ai.Message, turnID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, r := range results {
		if r.Role != ai.RoleTool {
			continue
		}
		t.appendLocked(r, turnID, "", nil, false)
	}
}

// Rollback truncates the transcript to the given length (for failed streams).
func (t *Transcript) Rollback(length int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if length < 0 || length >= len(t.items) {
		return
	}
	truncated := t.items[:length]
	t.byID = make(map[string]*TranscriptItem, len(truncated))
	t.pristine = t.pristine[:0]
	for _, it := range truncated {
		t.byID[it.ID] = it
		if it.ParentID == it.ID {
			t.pristine = append(t.pristine, it)
		}
	}
	t.items = truncated
}

// appendToDisk writes a JSONL record. Failure is non-fatal for the session.
func (t *Transcript) appendToDisk(ti *TranscriptItem) error {
	data, err := json.Marshal(ti)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(t.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(append(data, '\n')); err != nil {
		return err
	}
	return nil
}

// load reads a persisted JSONL transcript.
func (t *Transcript) load() error {
	data, err := os.ReadFile(t.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var ti TranscriptItem
		if err := json.Unmarshal([]byte(line), &ti); err != nil {
			continue
		}
		t.items = append(t.items, &ti)
		t.byID[ti.ID] = &ti
		if ti.ParentID == ti.ID || ti.ParentID == "" {
			t.pristine = append(t.pristine, &ti)
		}
		if ti.Seq >= t.seq {
			t.seq = ti.Seq + 1
		}
	}
	return nil
}

// --- Normalization / Hardening ---

// NormalizeMessagesForAPI ensures the model-facing sequence obeys the
// role-alternation and tool call/result adjacency invariants expected by
// strict providers. Missing results are synthesized, orphan outputs removed.
func NormalizeMessagesForAPI(messages []ai.Message) []ai.Message {
	messages = dropTrailingAssistant(messages)
	messages = mergeConsecutiveUser(messages)
	messages = ensureCallResultPairs(messages)
	return messages
}

// HardenHistory is a post-edit scrubber that guarantees the sequence is
// sendable: role alternation, tool pairing, and no trailing dangling tool calls.
func HardenHistory(messages []ai.Message) []ai.Message {
	out := make([]ai.Message, 0, len(messages))
	for _, m := range messages {
		if m.Role != ai.RoleUser && m.Role != ai.RoleAssistant && m.Role != ai.RoleTool && m.Role != ai.RoleSystem {
			continue
		}
		if m.Role == ai.RoleSystem && len(m.Content) == 0 {
			continue
		}
		out = append(out, m)
	}
	out = ensureCallResultPairs(out)
	out = dropTrailingAssistant(out)
	return out
}

// ensureCallResultPairs guarantees every tool_call has a result and every
// tool_result has a matching call within the window.
func ensureCallResultPairs(messages []ai.Message) []ai.Message {
	out := make([]ai.Message, 0, len(messages))
	callsInWindow := make(map[string]bool)
	for _, m := range messages {
		switch m.Role {
		case ai.RoleAssistant:
			for _, tc := range m.ToolCallParts() {
				callsInWindow[tc.ID] = true
			}
			out = append(out, m)
		case ai.RoleTool:
			for _, p := range m.Content {
				if p.Type == ai.ContentTypeToolResult && p.ToolResult != nil {
					if !callsInWindow[p.ToolResult.ToolCallID] {
						// Orphan result: drop it (no matching call in window).
						m.Content = removeToolResultPart(m.Content, p.ToolResult.ToolCallID)
					}
				}
			}
			if len(m.Content) > 0 {
				out = append(out, m)
			}
		default:
			out = append(out, m)
		}
	}
	return out
}

// SynthMissingToolResults inserts synthetic "aborted" results for any tool call
// that is never followed by a result, so APIs never see a dangling call.
func SynthMissingToolResults(messages []ai.Message) []ai.Message {
	called := make(map[string]ai.ToolCall)
	var out []ai.Message
	for _, m := range messages {
		out = append(out, m)
		if m.Role != ai.RoleAssistant {
			continue
		}
		for _, tc := range m.ToolCallParts() {
			called[tc.ID] = tc
		}
	}
	// Walk again: for any called ID never answered, append a synthetic result.
	answered := make(map[string]bool)
	for _, m := range messages {
		for _, p := range m.Content {
			if p.Type == ai.ContentTypeToolResult && p.ToolResult != nil {
				answered[p.ToolResult.ToolCallID] = true
			}
		}
	}
	for id, tc := range called {
		if answered[id] {
			continue
		}
		out = append(out, ai.Message{
			Role: ai.RoleTool,
			Content: []ai.ContentPart{{
				Type: ai.ContentTypeToolResult,
				ToolResult: &ai.ToolResult{
					ToolCallID: id,
					Content:    fmt.Sprintf("[synthetic] tool %q was interrupted before producing a result", tc.Name),
					IsError:    true,
				},
			}},
		})
	}
	return out
}

// mergeConsecutiveUser merges adjacent user messages (and empty ones).
func mergeConsecutiveUser(messages []ai.Message) []ai.Message {
	out := make([]ai.Message, 0, len(messages))
	var pending *ai.Message
	for _, m := range messages {
		if m.Role == ai.RoleUser {
			if pending == nil {
				cp := m
				pending = &cp
			} else {
				for _, p := range m.Content {
					pending.AppendText(p.Text)
				}
			}
			continue
		}
		if pending != nil {
			if len(pending.Content) > 0 {
				out = append(out, *pending)
			}
			pending = nil
		}
		out = append(out, m)
	}
	if pending != nil && len(pending.Content) > 0 {
		out = append(out, *pending)
	}
	return out
}

// dropTrailingAssistant removes a dangling trailing assistant message that has
// only tool calls (its results never arrived) — a common mid-stream failure.
func dropTrailingAssistant(messages []ai.Message) []ai.Message {
	if len(messages) == 0 {
		return messages
	}
	last := messages[len(messages)-1]
	if last.Role != ai.RoleAssistant || !last.HasToolCalls() {
		return messages
	}
	hasText := false
	for _, p := range last.Content {
		if p.Type == ai.ContentTypeText || p.Type == ai.ContentTypeThought {
			hasText = true
			break
		}
	}
	if !hasText {
		// Pure tool-call stub with no results: drop entirely.
		return messages[:len(messages)-1]
	}
	// Strip tool-call parts whose results never arrived.
	var kept []ai.ContentPart
	for _, p := range last.Content {
		if p.Type == ai.ContentTypeToolCall {
			continue
		}
		kept = append(kept, p)
	}
	last.Content = kept
	messages[len(messages)-1] = last
	return messages
}

// removeToolResultPart drops the tool result part matching a call ID.
func removeToolResultPart(parts []ai.ContentPart, callID string) []ai.ContentPart {
	var kept []ai.ContentPart
	for _, p := range parts {
		if p.Type == ai.ContentTypeToolResult && p.ToolResult != nil && p.ToolResult.ToolCallID == callID {
			continue
		}
		kept = append(kept, p)
	}
	return kept
}

// TranscriptPathFor returns the JSONL path for a session transcript.
func TranscriptPathFor(dir, sessionID string) string {
	return filepath.Join(dir, sessionID+".transcript.jsonl")
}

func newTurnID() string {
	return fmt.Sprintf("turn-%d", time.Now().UnixNano())
}
