package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Storage persists sessions to disk as append-only JSONL transcripts
// (see transcript.go). Legacy single-file JSON sessions are still read and
// migrate to the transcript format on their next Save.
type Storage struct {
	dir string
	// maxSessionBytes caps the persisted session size; 0 uses the default.
	maxSessionBytes int64

	mu         sync.Mutex
	saveStates map[string]*saveState
	stats      *statsTracker
	files      *FileHistory
}

// NewStorage creates a Storage that uses the given directory.
// The directory is created with mode 0700 (owner-only) for security.
func NewStorage(dir string) (*Storage, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("session storage: mkdir: %w", err)
	}
	return &Storage{
		dir:        dir,
		saveStates: make(map[string]*saveState),
		stats:      newStatsTracker(dir),
		files:      NewFileHistory(dir),
	}, nil
}

// FileHistory exposes the per-session file checkpoint store.
func (s *Storage) FileHistory() *FileHistory { return s.files }

// SetMaxSessionBytes overrides the default session file size ceiling.
func (s *Storage) SetMaxSessionBytes(max int64) { s.maxSessionBytes = max }

// Save appends the session's changes to its transcript (mode 0600), writing
// only new messages and changed metadata — never rewriting history. The
// session is redacted before anything reaches disk, and snapshotted under
// its own lock so a save racing with agent mutations stays consistent.
func (s *Storage) Save(sess *Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	redacted := RedactSession(sess)
	snap := redacted.Snapshot()

	// A transcript written by another process (or a previous run of this
	// one) may already exist: rebuild the delta bookkeeping from it instead
	// of appending a duplicate snapshot epoch.
	if _, ok := s.saveStates[snap.ID]; !ok {
		if existing := s.findTranscript(snap.ID); existing != "" {
			if prev, err := s.loadTranscript(existing); err == nil {
				st := &saveState{project: sanitizeProjectDir(prev.WorkDir)}
				st.count = len(prev.Messages)
				if st.count > 0 {
					st.lastDigest = digestMessage(prev.Messages[st.count-1])
				}
				st.title = prev.Title
				st.scalarKey = digestScalars(scalarsFrom(prev))
				s.saveStates[snap.ID] = st
			}
		}
	}
	err := s.saveTranscript(snap)
	s.recordStats(snap)
	return err
}

// recordStats folds the saved session into the usage aggregate. Best-effort:
// stats are a display nicety and must never fail a save.
func (s *Storage) recordStats(sess *Session) {
	if s.stats == nil {
		return
	}
	s.stats.recordSession(sess, len(sess.Messages))
}

// Load reads a session by ID, preferring the transcript format and falling
// back to the legacy whole-file JSON.
func (s *Storage) Load(id string) (*Session, error) {
	if path := s.findTranscript(id); path != "" {
		return s.loadTranscript(path)
	}
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

// List returns all sessions sorted by updated time descending. Transcripts
// are listed lite (head+tail windows only, no message parsing); legacy JSON
// files are fully parsed but skipped once a transcript for the same ID
// exists.
func (s *Storage) List() ([]*Session, error) {
	var sessions []*Session
	seen := make(map[string]bool)

	root := filepath.Join(s.dir, projectsSubdir)
	if projects, err := os.ReadDir(root); err == nil {
		for _, p := range projects {
			if !p.IsDir() {
				continue
			}
			files, err := os.ReadDir(filepath.Join(root, p.Name()))
			if err != nil {
				continue
			}
			for _, f := range files {
				if f.IsDir() || !strings.HasSuffix(f.Name(), ".jsonl") {
					continue
				}
				if sess := readLiteSession(filepath.Join(root, p.Name(), f.Name())); sess != nil {
					sessions = append(sessions, sess)
					seen[sess.ID] = true
				}
			}
		}
	}

	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		// skip checkpoints and storage-owned aux files
		if strings.Contains(e.Name(), "_cp") || auxJSONFiles[e.Name()] {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".json")
		if seen[id] {
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
		sess.SizeBytes = int64(len(data))
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

// Delete removes a session's transcript, its tool-results directory, any
// legacy JSON file, and any checkpoints.
func (s *Storage) Delete(id string) error {
	s.mu.Lock()
	delete(s.saveStates, id)
	s.mu.Unlock()

	var firstErr error
	if path := s.findTranscript(id); path != "" {
		if err := os.RemoveAll(filepath.Dir(path) + string(filepath.Separator) + id); err != nil {
			firstErr = err
		}
		if err := os.Remove(path); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	legacy := filepath.Join(s.dir, id+".json")
	if err := os.Remove(legacy); err != nil && !os.IsNotExist(err) && firstErr == nil {
		firstErr = err
	}
	// Checkpoints and file-history artifacts for this session.
	entries, _ := os.ReadDir(s.dir)
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), id+"_cp") {
			_ = os.Remove(filepath.Join(s.dir, e.Name()))
		}
	}
	if fh := filepath.Join(s.dir, "file-history", id); true {
		_ = os.RemoveAll(fh)
	}
	return firstErr
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

// auxJSONFiles are storage-owned JSON files that must never be treated as
// sessions by List, Audit or Prune.
var auxJSONFiles = map[string]bool{
	statsFileName:    true,
	"search_index.json": true,
}

// Prune removes oldest sessions beyond maxSessions, and sessions older than maxAge
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
	// Transcript-format sessions live under projects/<dir>/<id>.jsonl.
	if projects, err := os.ReadDir(filepath.Join(s.dir, projectsSubdir)); err == nil {
		for _, p := range projects {
			if !p.IsDir() {
				continue
			}
			files, err := os.ReadDir(filepath.Join(s.dir, projectsSubdir, p.Name()))
			if err != nil {
				continue
			}
			for _, f := range files {
				if !f.IsDir() && strings.HasSuffix(f.Name(), ".jsonl") {
					alive[strings.TrimSuffix(f.Name(), ".jsonl")] = true
				}
			}
		}
	}
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
		if strings.HasPrefix(name, ".automergent-tmp-") {
			// Leaked temp file from a crashed atomic write (the rename
			// never happened; the deferred cleanup never ran).
			_ = os.Remove(filepath.Join(s.dir, name))
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
