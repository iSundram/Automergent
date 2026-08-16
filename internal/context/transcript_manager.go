package context

import (
	"sync"

	"github.com/iSundram/Automergent/internal/ai"
)

// TranscriptManager owns the durable conversation transcript and exposes
// normalization/hardening for the model-facing sequence. It is the single
// stateful owner of conversation history across turns.
type TranscriptManager struct {
	mu        sync.RWMutex
	transcript *Transcript
	turnID    string
}

// NewTranscriptManager creates a transcript manager.
func NewTranscriptManager(transcript *Transcript) *TranscriptManager {
	if transcript == nil {
		transcript = NewTranscript("")
	}
	return &TranscriptManager{transcript: transcript}
}

// NewSession begins a new turn grouping.
func (tm *TranscriptManager) NewSession() {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.turnID = newTurnID()
}

// Append records a message into the transcript.
func (tm *TranscriptManager) Append(msg ai.Message) {
	tm.mu.Lock()
	turnID := tm.turnID
	tm.mu.Unlock()
	tm.transcript.Append(msg, turnID)
}

// AppendReplace records a message replacing a prior one (provenance link).
func (tm *TranscriptManager) AppendReplace(msg ai.Message, parentID string, abstracts []string) {
	tm.mu.Lock()
	turnID := tm.turnID
	tm.mu.Unlock()
	tm.transcript.AppendReplace(msg, turnID, parentID, abstracts)
}

// ReplaceWithSummary inserts a summary item and marks the abstracted range.
func (tm *TranscriptManager) ReplaceWithSummary(summary ai.Message, abstractedIDs []string) {
	tm.mu.Lock()
	turnID := tm.turnID
	tm.mu.Unlock()
	tm.transcript.AppendReplace(summary, turnID, "", abstractedIDs)
}

// AppendCompactBoundary inserts a compaction boundary marker.
func (tm *TranscriptManager) AppendCompactBoundary(summary string) {
	tm.mu.Lock()
	turnID := tm.turnID
	tm.mu.Unlock()
	msg := ai.NewTextMessage(ai.RoleSystem, "# Compacted Context Summary\n\n"+summary)
	tm.transcript.AppendReplace(msg, turnID, "", nil)
}

// ToMessages renders the hardened, normalized message sequence for the API.
func (tm *TranscriptManager) ToMessages() []ai.Message {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	msgs := tm.transcript.ToMessages()
	return NormalizeMessagesForAPI(msgs)
}

// RawMessages returns the unmodified transcript messages.
func (tm *TranscriptManager) RawMessages() []ai.Message {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return tm.transcript.ToMessages()
}

// PristineMessages returns only never-replaced items.
func (tm *TranscriptManager) PristineMessages() []ai.Message {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	items := tm.transcript.PristineItems()
	msgs := make([]ai.Message, 0, len(items))
	for _, it := range items {
		msgs = append(msgs, it.ToMessage())
	}
	return msgs
}

// Rollback truncates the transcript to the given length.
func (tm *TranscriptManager) Rollback(length int) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	tm.transcript.Rollback(length)
}

// Transcript exposes the underlying transcript.
func (tm *TranscriptManager) Transcript() *Transcript {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return tm.transcript
}