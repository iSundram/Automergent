package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// PersistenceState represents the full state that can be persisted and recovered.
type PersistenceState struct {
	Session     *Session          `json:"session"`
	WorkDir     string            `json:"work_dir"`
	ProjectPath string            `json:"project_path"`
	GitBranch   string            `json:"git_branch,omitempty"`
	LastFile    string            `json:"last_file,omitempty"`
	Cursor      *CursorPosition   `json:"cursor,omitempty"`
	OpenFiles   []string          `json:"open_files,omitempty"`
	Context     map[string]string `json:"context,omitempty"`
	Timestamp   time.Time         `json:"timestamp"`
	Version     int               `json:"version"` // For migration support
}

// CursorPosition tracks the last editing position.
type CursorPosition struct {
	File   string `json:"file"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
}

// PersistenceManager handles session persistence with crash recovery.
type PersistenceManager struct {
	dir          string
	mu           sync.RWMutex
	state        *PersistenceState
	autoSaveStop chan struct{}
	dirty        bool
}

const (
	persistenceVersion     = 1
	stateFileName          = "state.json"
	stateBackupFileName    = "state.backup.json"
	recoveryFileName       = "recovery.json"
	defaultAutoSaveSeconds = 30
)

// NewPersistenceManager creates a new persistence manager.
func NewPersistenceManager(dir string) (*PersistenceManager, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("persistence: mkdir: %w", err)
	}
	pm := &PersistenceManager{
		dir:   dir,
		state: &PersistenceState{Version: persistenceVersion},
	}
	return pm, nil
}

// SetSession associates a session with the persistence manager.
func (pm *PersistenceManager) SetSession(sess *Session) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.state.Session = sess
	pm.dirty = true
}

// SetWorkDir sets the working directory context.
func (pm *PersistenceManager) SetWorkDir(dir string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.state.WorkDir = dir
	pm.dirty = true
}

// SetProjectPath sets the project root path.
func (pm *PersistenceManager) SetProjectPath(path string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.state.ProjectPath = path
	pm.dirty = true
}

// SetGitBranch sets the current git branch.
func (pm *PersistenceManager) SetGitBranch(branch string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.state.GitBranch = branch
	pm.dirty = true
}

// SetLastFile sets the last edited file.
func (pm *PersistenceManager) SetLastFile(file string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.state.LastFile = file
	pm.dirty = true
}

// SetCursor sets the cursor position.
func (pm *PersistenceManager) SetCursor(file string, line, column int) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.state.Cursor = &CursorPosition{File: file, Line: line, Column: column}
	pm.dirty = true
}

// SetOpenFiles sets the list of open files.
func (pm *PersistenceManager) SetOpenFiles(files []string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.state.OpenFiles = make([]string, len(files))
	copy(pm.state.OpenFiles, files)
	pm.dirty = true
}

// SetContext sets a context key-value pair.
func (pm *PersistenceManager) SetContext(key, value string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	if pm.state.Context == nil {
		pm.state.Context = make(map[string]string)
	}
	pm.state.Context[key] = value
	pm.dirty = true
}

// GetContext retrieves a context value.
func (pm *PersistenceManager) GetContext(key string) (string, bool) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	if pm.state.Context == nil {
		return "", false
	}
	v, ok := pm.state.Context[key]
	return v, ok
}

// State returns a copy of the current state.
func (pm *PersistenceManager) State() *PersistenceState {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	if pm.state == nil {
		return nil
	}
	// Deep copy
	data, _ := json.Marshal(pm.state)
	var copy PersistenceState
	_ = json.Unmarshal(data, &copy)
	return &copy
}

// Save persists the current state to disk atomically.
func (pm *PersistenceManager) Save() error {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	return pm.saveLocked()
}

func (pm *PersistenceManager) saveLocked() error {
	pm.state.Timestamp = time.Now()

	// Snapshot the session under its own lock before marshaling to avoid
	// torn JSON when the agent is concurrently mutating the session.
	var data []byte
	var err error
	if pm.state.Session != nil {
		pm.state.Session = pm.state.Session.Snapshot()
	}
	data, err = json.MarshalIndent(pm.state, "", "  ")
	if err != nil {
		return fmt.Errorf("persistence: marshal: %w", err)
	}

	statePath := filepath.Join(pm.dir, stateFileName)
	backupPath := filepath.Join(pm.dir, stateBackupFileName)

	// Backup existing state first
	if _, err := os.Stat(statePath); err == nil {
		_ = os.Rename(statePath, backupPath)
	}

	if err := atomicWriteFile(statePath, data, 0o600); err != nil {
		// Restore backup on failure
		if _, berr := os.Stat(backupPath); berr == nil {
			_ = os.Rename(backupPath, statePath)
		}
		return fmt.Errorf("persistence: write: %w", err)
	}

	pm.dirty = false
	return nil
}

// Load reads the persisted state from disk.
func (pm *PersistenceManager) Load() error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	statePath := filepath.Join(pm.dir, stateFileName)
	data, err := os.ReadFile(statePath)
	if err != nil {
		if os.IsNotExist(err) {
			// Try backup
			backupPath := filepath.Join(pm.dir, stateBackupFileName)
			data, err = os.ReadFile(backupPath)
			if err != nil {
				return nil // No state to load
			}
		} else {
			return fmt.Errorf("persistence: read: %w", err)
		}
	}

	var state PersistenceState
	if err := json.Unmarshal(data, &state); err != nil {
		return fmt.Errorf("persistence: unmarshal: %w", err)
	}

	pm.state = &state
	pm.dirty = false
	return nil
}

// HasRecoveryState checks if there's a recoverable state.
func (pm *PersistenceManager) HasRecoveryState() bool {
	recoveryPath := filepath.Join(pm.dir, recoveryFileName)
	_, err := os.Stat(recoveryPath)
	return err == nil
}

// SaveRecoveryPoint saves a recovery checkpoint.
func (pm *PersistenceManager) SaveRecoveryPoint() error {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	pm.state.Timestamp = time.Now()
	if pm.state.Session != nil {
		pm.state.Session = pm.state.Session.Snapshot()
	}
	data, err := json.MarshalIndent(pm.state, "", "  ")
	if err != nil {
		return fmt.Errorf("persistence: recovery marshal: %w", err)
	}

	recoveryPath := filepath.Join(pm.dir, recoveryFileName)
	return atomicWriteFile(recoveryPath, data, 0o600)
}

// LoadRecoveryPoint loads from a recovery checkpoint.
func (pm *PersistenceManager) LoadRecoveryPoint() (*PersistenceState, error) {
	recoveryPath := filepath.Join(pm.dir, recoveryFileName)
	data, err := os.ReadFile(recoveryPath)
	if err != nil {
		return nil, fmt.Errorf("persistence: read recovery: %w", err)
	}

	var state PersistenceState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("persistence: unmarshal recovery: %w", err)
	}

	return &state, nil
}

// ClearRecoveryPoint removes the recovery checkpoint after successful resume.
func (pm *PersistenceManager) ClearRecoveryPoint() error {
	recoveryPath := filepath.Join(pm.dir, recoveryFileName)
	if err := os.Remove(recoveryPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("persistence: clear recovery: %w", err)
	}
	return nil
}

// StartAutoSave begins periodic auto-saving.
func (pm *PersistenceManager) StartAutoSave(intervalSeconds int) {
	if intervalSeconds <= 0 {
		intervalSeconds = defaultAutoSaveSeconds
	}

	pm.mu.Lock()
	if pm.autoSaveStop != nil {
		pm.mu.Unlock()
		return // Already running
	}
	pm.autoSaveStop = make(chan struct{})
	stopCh := pm.autoSaveStop
	pm.mu.Unlock()

	go func() {
		ticker := time.NewTicker(time.Duration(intervalSeconds) * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-stopCh:
				return
			case <-ticker.C:
				pm.mu.Lock()
				if pm.dirty {
					_ = pm.saveLocked()
				}
				pm.mu.Unlock()
			}
		}
	}()
}

// StopAutoSave stops periodic auto-saving.
func (pm *PersistenceManager) StopAutoSave() {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if pm.autoSaveStop != nil {
		close(pm.autoSaveStop)
		pm.autoSaveStop = nil
	}
}

// IsDirty returns whether there are unsaved changes.
func (pm *PersistenceManager) IsDirty() bool {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.dirty
}

// ResumeSession attempts to resume a previous session.
func (pm *PersistenceManager) ResumeSession(sessionID string, storage *Storage) (*Session, error) {
	// A recovery point is not automatically newer than the clean session file.
	// Autosave can leave a short/stale recovery snapshot behind, so compare all
	// available copies and keep the richest history for an explicit resume.
	var candidates []*Session
	var storageErr error
	if storage != nil {
		if sess, err := storage.Load(sessionID); err == nil {
			candidates = append(candidates, sess)
		} else {
			storageErr = err
		}
	}

	var recovery *PersistenceState
	if pm.HasRecoveryState() {
		if state, err := pm.LoadRecoveryPoint(); err == nil && state.Session != nil && state.Session.ID == sessionID {
			recovery = state
			candidates = append(candidates, state.Session)
		}
	}

	if err := pm.Load(); err != nil && len(candidates) == 0 {
		return nil, err
	}
	pm.mu.RLock()
	stateSession := pm.state.Session
	pm.mu.RUnlock()
	if stateSession != nil && stateSession.ID == sessionID {
		candidates = append(candidates, stateSession)
	}
	if len(candidates) == 0 {
		if storageErr != nil {
			return nil, storageErr
		}
		return nil, os.ErrNotExist
	}

	best := candidates[0]
	for _, candidate := range candidates[1:] {
		if richerSession(candidate, best) {
			best = candidate
		}
	}
	if recovery != nil && best == recovery.Session {
		pm.mu.Lock()
		pm.state = recovery
		pm.mu.Unlock()
	} else {
		pm.SetSession(best)
	}
	// The selected snapshot is now represented by the active state; stale
	// recovery data should not be reconsidered on the next launch.
	_ = pm.ClearRecoveryPoint()
	return best, nil
}

func richerSession(candidate, current *Session) bool {
	if len(candidate.Messages) != len(current.Messages) {
		return len(candidate.Messages) > len(current.Messages)
	}
	return candidate.UpdatedAt.After(current.UpdatedAt)
}
