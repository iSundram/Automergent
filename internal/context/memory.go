package context

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Memory is a key-value store persisted to disk.
type Memory struct {
	path  string
	store map[string]string
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
	m.store[key] = value
}

// Get retrieves a value by key.
func (m *Memory) Get(key string) (string, bool) {
	v, ok := m.store[key]
	return v, ok
}

// Delete removes a key.
func (m *Memory) Delete(key string) {
	delete(m.store, key)
}

// Save persists the memory to disk.
func (m *Memory) Save() error {
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
	out := make(map[string]string, len(m.store))
	for k, v := range m.store {
		out[k] = v
	}
	return out
}
