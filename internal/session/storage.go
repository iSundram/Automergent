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
	// maxSessionBytes caps the persisted session size; 0 uses the default.
	maxSessionBytes int64
}

// NewStorage creates a Storage that uses the given directory.
// The directory is created with mode 0700 (owner-only) for security.
func NewStorage(dir string) (*Storage, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("session storage: mkdir: %w", err)
	}
	return &Storage{dir: dir}, nil
}

// SetMaxSessionBytes overrides the default session file size ceiling.
func (s *Storage) SetMaxSessionBytes(max int64) { s.maxSessionBytes = max }

// Save writes a session to disk atomically with mode 0600 (owner-only).
// The session is snapshotted before marshaling so a save racing with
// concurrent agent mutations can never produce torn JSON, and oversized
// histories are compacted in the snapshot (never the live session).
func (s *Storage) Save(sess *Session) error {
	redacted := RedactSession(sess)
	snap := redacted.Snapshot()
	maxBytes := s.maxSessionBytes
	if maxBytes <= 0 {
		maxBytes = defaultMaxSessionBytes
	}

	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return err
	}
	// Compact in the snapshot (never the live session) until the final,
	// pretty-printed file fits the budget.
	if int64(len(data)) > maxBytes {
		CompactForSize(snap, maxBytes)
		data, err = json.MarshalIndent(snap, "", "  ")
		if err != nil {
			return err
		}
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
	return MigrateSession(&sess)
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
		if migrated, err := MigrateSession(&sess); err == nil {
			sessions = append(sessions, migrated)
		} else {
			sessions = append(sessions, &sess)
		}
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
// It also cleans up orphaned checkpoint files and stale crash-recovery state so
// disk usage stays bounded.
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

	s.cleanupAuxFiles()
	return nil
}

// cleanupAuxFiles removes checkpoint snapshots ("*_cp*.json") and crash-recovery
// files (recovery.json, state.json, state.backup.json) whose owning session no
// longer exists, or which are stale. Checkpoints are never restored automatically,
// so orphaned ones are safe to delete.
func (s *Storage) cleanupAuxFiles() {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return
	}

	alive := make(map[string]bool)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasSuffix(name, ".json") && !strings.Contains(name, "_cp") &&
			!strings.HasPrefix(name, "state") && !strings.HasPrefix(name, "recovery") {
			alive[strings.TrimSuffix(name, ".json")] = true
		}
	}

	cutoff := time.Now().Add(-24 * time.Hour)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.Contains(name, "_cp") {
			// Delete checkpoints for sessions that no longer exist; otherwise
			// keep only recent ones (they may be restored manually).
			owner := strings.SplitN(name, "_cp", 2)[0]
			if !alive[owner] {
				_ = os.Remove(filepath.Join(s.dir, name))
				continue
			}
			if info, err := e.Info(); err == nil && info.ModTime().Before(cutoff) {
				_ = os.Remove(filepath.Join(s.dir, name))
			}
			continue
		}
		if name == recoveryFileName || name == stateFileName || name == stateBackupFileName {
			// Only remove if it references a session that is gone (stale) or
			// older than a day (crash leftovers).
			data, err := os.ReadFile(filepath.Join(s.dir, name))
			if err != nil {
				continue
			}
			var state PersistenceState
			if err := json.Unmarshal(data, &state); err != nil {
				_ = os.Remove(filepath.Join(s.dir, name))
				continue
			}
			if state.Session != nil && !alive[state.Session.ID] {
				_ = os.Remove(filepath.Join(s.dir, name))
				continue
			}
			if info, err := e.Info(); err == nil && info.ModTime().Before(cutoff) {
				_ = os.Remove(filepath.Join(s.dir, name))
			}
		}
	}
}
