package workflow

// Journal: the deterministic-resume mechanism. Every agent call gets a key
// — sha256 of the prompt plus the canonical (sorted, display-fields-stripped)
// parameters — and its result is appended to a per-run JSONL file. Resuming
// a run replays journaled results for keys that match, so unchanged steps
// cost nothing.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

// JournalEntry is one journaled agent call.
type JournalEntry struct {
	// Key is the determinism key: matching key ⇒ replayable result.
	Key   string `json:"key"`
	Seq   int    `json:"seq"`
	Step  string `json:"step"`
	Error string `json:"error,omitempty"`
	// Output is the agent result ("dead" entries carry the failure text).
	Output      string `json:"output"`
	OutputTokens int  `json:"outputTokens"`
}

// JournalStore persists entries per run. Safe for concurrent use.
type JournalStore interface {
	Read(runID string) ([]JournalEntry, error)
	Append(runID string, entry JournalEntry) error
	Truncate(runID string) error
}

// FileJournalStore keeps one JSONL file per run under runsDir.
type FileJournalStore struct {
	runsDir string
	mu      sync.Mutex
}

// NewFileJournalStore stores run journals under the given directory.
func NewFileJournalStore(runsDir string) *FileJournalStore {
	return &FileJournalStore{runsDir: runsDir}
}

func (s *FileJournalStore) path(runID string) string {
	return filepath.Join(s.runsDir, runID, "journal.jsonl")
}

func (s *FileJournalStore) Read(runID string) ([]JournalEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(s.path(runID))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var entries []JournalEntry
	for _, line := range splitLines(data) {
		var e JournalEntry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			continue // a torn final line after a crash is skipped, not fatal
		}
		entries = append(entries, e)
	}
	// Parallel completion order ≠ call order; re-sort by seq so the key
	// index is stable across resumes.
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].Seq < entries[j].Seq })
	return entries, nil
}

func (s *FileJournalStore) Append(runID string, entry JournalEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(filepath.Join(s.runsDir, runID), 0o755); err != nil {
		return err
	}
	line, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(s.path(runID), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(line, '\n'))
	return err
}

func (s *FileJournalStore) Truncate(runID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return os.RemoveAll(filepath.Join(s.runsDir, runID))
}

func splitLines(data []byte) []string {
	var lines []string
	start := 0
	for i, b := range data {
		if b == '\n' {
			if i > start {
				lines = append(lines, string(data[start:i]))
			}
			start = i + 1
		}
	}
	if start < len(data) {
		lines = append(lines, string(data[start:]))
	}
	return lines
}

// AgentParams are the execution parameters of one agent call — everything
// that determines its result, and therefore part of the determinism key.
// Display-only fields (Step, Label) are excluded.
type AgentParams struct {
	Prompt    string
	AgentType string
	Model     string
}

// agentCallKey is the determinism key for one agent() call.
func agentCallKey(prompt string, params AgentParams) string {
	canonical := map[string]string{
		"prompt":    prompt,
		"agentType": params.AgentType,
		"model":     params.Model,
	}
	keys := make([]string, 0, len(canonical))
	for k := range canonical {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	canonicalStr := ""
	for _, k := range keys {
		canonicalStr += fmt.Sprintf("%s=%s\n", k, canonical[k])
	}
	sum := sha256.Sum256([]byte(prompt + "\x00" + canonicalStr))
	return hex.EncodeToString(sum[:])
}
