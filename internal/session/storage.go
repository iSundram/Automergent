package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Storage persists sessions to disk.
type Storage struct {
	dir string
}

// NewStorage creates a Storage that uses the given directory.
// The directory is created with mode 0700 (owner-only) for security.
func NewStorage(dir string) (*Storage, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("session storage: mkdir: %w", err)
	}
	return &Storage{dir: dir}, nil
}

// Save writes a session to disk atomically with mode 0600 (owner-only).
func (s *Storage) Save(sess *Session) error {
	data, err := json.MarshalIndent(sess, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(s.dir, sess.ID+".json")
	return atomicWriteFile(path, data, 0o600)
}

// atomicWriteFile writes data to path atomically: write to temp, sync, rename.
// This prevents partial-write corruption on crash or SIGKILL.
func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".automergent-tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() {
		tmp.Close()
		os.Remove(tmpPath) // no-op if rename already succeeded
	}()

	if _, err := tmp.Write(data); err != nil {
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("sync temp file: %w", err)
	}
	tmp.Close()

	if err := os.Chmod(tmpPath, perm); err != nil {
		return fmt.Errorf("chmod temp file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("rename to final path: %w", err)
	}
	return nil
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

// Prune removes oldest sessions beyond maxSessions, and sessions older than maxAge
// (maxAge == 0 means no age-based pruning). Sessions are sorted by last update time.
func (s *Storage) Prune(maxSessions int, maxAge time.Duration) error {
	sessions, err := s.List()
	if err != nil {
		return err
	}

	// sessions is already sorted newest-first from List()
	if maxSessions > 0 && len(sessions) > maxSessions {
		toDelete := sessions[maxSessions:]
		for _, sess := range toDelete {
			_ = s.Delete(sess.ID)
		}
		sessions = sessions[:maxSessions]
	}

	if maxAge > 0 {
		cutoff := time.Now().Add(-maxAge)
		for _, sess := range sessions {
			if sess.UpdatedAt.Before(cutoff) {
				_ = s.Delete(sess.ID)
			}
		}
	}

	return nil
}
