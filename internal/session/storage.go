package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Storage persists sessions to disk.
type Storage struct {
	dir string
}

// NewStorage creates a Storage that uses the given directory.
func NewStorage(dir string) (*Storage, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("session storage: mkdir: %w", err)
	}
	return &Storage{dir: dir}, nil
}

// Save writes a session to disk.
func (s *Storage) Save(sess *Session) error {
	data, err := json.MarshalIndent(sess, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(s.dir, sess.ID+".json")
	return os.WriteFile(path, data, 0o644)
}

// Load reads a session by ID.
func (s *Storage) Load(id string) (*Session, error) {
	data, err := os.ReadFile(filepath.Join(s.dir, id+".json"))
	if err != nil {
		return nil, err
	}
	var sess Session
	if err := json.Unmarshal(data, &sess); err != nil {
		return nil, err
	}
	return &sess, nil
}

// List returns all sessions sorted by updated time descending.
func (s *Storage) List() ([]*Session, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}
	var sessions []*Session
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		// skip checkpoints
		if strings.Contains(e.Name(), "_cp") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(s.dir, e.Name()))
		if err != nil {
			continue
		}
		var sess Session
		if err := json.Unmarshal(data, &sess); err != nil {
			continue
		}
		sessions = append(sessions, &sess)
	}
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].UpdatedAt.After(sessions[j].UpdatedAt)
	})
	return sessions, nil
}

// Delete removes a session file.
func (s *Storage) Delete(id string) error {
	return os.Remove(filepath.Join(s.dir, id+".json"))
}
