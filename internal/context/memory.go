package context

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// Memory is a key-value store persisted to disk.
type Memory struct {
	path  string
	store map[string]string
	mu    sync.RWMutex
}

// NewMemory loads or creates a memory store at the given path.
func NewMemory(dir string) (*Memory, error) {
	path := filepath.Join(dir, "memory.json")
	m := &Memory{path: path, store: make(map[string]string)}
	data, err := os.ReadFile(path)
	if err == nil {
		_ = json.Unmarshal(data, &m.store)
	}
	return m, nil
}

// Set stores a value under key.
func (m *Memory) Set(key, value string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.store[key] = value
}

// Get retrieves a value by key.
func (m *Memory) Get(key string) (string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	v, ok := m.store[key]
	return v, ok
}

// Delete removes a key.
func (m *Memory) Delete(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.store, key)
}

// Save persists the memory to disk.
func (m *Memory) Save() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(m.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(m.store, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(m.path, data, 0o644)
}

// All returns a copy of all stored key-value pairs.
func (m *Memory) All() map[string]string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make(map[string]string, len(m.store))
	for k, v := range m.store {
		out[k] = v
	}
	return out
}

// MemoryEntry is a key-value pair returned by RelevantTo with score.
type MemoryEntry struct {
	Key   string
	Value string
	Score int
}

// RelevantTo returns memory entries that match the given intent via keyword overlap and scores them.
func (m *Memory) RelevantTo(intent string) []MemoryEntry {
	if m == nil || intent == "" {
		return nil
	}

	intentLower := strings.ToLower(intent)
	intentWords := strings.Fields(intentLower)
	if len(intentWords) == 0 {
		return nil
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	var scoredMatches []struct {
		entry MemoryEntry
		score int
	}

	for key, value := range m.store {
		keyLower := strings.ToLower(key)
		valueLower := strings.ToLower(value)
		combined := keyLower + " " + valueLower

		score := 0
		// Exact phrase bonus
		if strings.Contains(combined, intentLower) {
			score += 5
		}

		for _, word := range intentWords {
			if len(word) < 2 {
				continue
			}
			if strings.Contains(keyLower, word) {
				score += 3 // Key match is more important
			} else if strings.Contains(valueLower, word) {
				score += 1
			}
		}

		if score > 0 {
			scoredMatches = append(scoredMatches, struct {
				entry MemoryEntry
				score int
			}{
				entry: MemoryEntry{Key: key, Value: value, Score: score},
				score: score,
			})
		}
	}

	// Sort by score descending
	sort.Slice(scoredMatches, func(i, j int) bool {
		return scoredMatches[i].score > scoredMatches[j].score
	})

	matches := make([]MemoryEntry, len(scoredMatches))
	for i, sm := range scoredMatches {
		matches[i] = sm.entry
	}

	return matches
}
