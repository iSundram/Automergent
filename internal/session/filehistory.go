// filehistory.go provides per-session file checkpointing: content-addressed
// backups captured before each write, restorable for rewind.
//
// Layout:
//
//	<storage-root>/file-history/<sessionID>/<sha256>-v<n>
//
// Backups are immutable: identical content reuses the same address, so
// repeated captures of an unchanged file cost nothing. The per-session
// directory is removed when the session is deleted.
package session

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// FileVersion is one backed-up version of a file.
type FileVersion struct {
	Path     string    `json:"path"`
	Hash     string    `json:"hash"`
	Version  int       `json:"version"`
	Captured time.Time `json:"captured"`
	Bytes    int64     `json:"bytes"`
}

// FileHistory captures and restores file backups for one session.
type FileHistory struct {
	dir string
	mu  sync.Mutex
	// index caches path → versions, loaded lazily from disk.
	index map[string][]FileVersion
}

const (
	maxFileHistoryFiles = 200 // per session; oldest evicted beyond this
)

// NewFileHistory returns a FileHistory rooted at the storage root.
func NewFileHistory(storageRoot string) *FileHistory {
	return &FileHistory{
		dir:   filepath.Join(storageRoot, "file-history"),
		index: make(map[string][]FileVersion),
	}
}

func (fh *FileHistory) sessionDir(sessionID string) string {
	return filepath.Join(fh.dir, sessionID)
}

func (fh *FileHistory) indexPath(sessionID string) string {
	return filepath.Join(fh.sessionDir(sessionID), "index.json")
}

// Capture records the given content as a version of path for the session.
// A version whose content hash already exists for the path is a no-op.
func (fh *FileHistory) Capture(sessionID, path, content string) (FileVersion, error) {
	fh.mu.Lock()
	defer fh.mu.Unlock()

	if err := fh.loadIndex(sessionID); err != nil {
		return FileVersion{}, err
	}
	sum := sha256.Sum256([]byte(content))
	hash := hex.EncodeToString(sum[:])

	versions := fh.index[path]
	for _, v := range versions {
		if v.Hash == hash {
			return v, nil // identical content already captured
		}
	}

	if err := os.MkdirAll(fh.sessionDir(sessionID), 0o700); err != nil {
		return FileVersion{}, fmt.Errorf("file-history: mkdir: %w", err)
	}
	version := len(versions) + 1
	name := fmt.Sprintf("%s-v%d", hash[:16], version)
	if err := os.WriteFile(filepath.Join(fh.sessionDir(sessionID), name), []byte(content), 0o600); err != nil {
		return FileVersion{}, fmt.Errorf("file-history: write backup: %w", err)
	}

	fv := FileVersion{
		Path:     path,
		Hash:     hash,
		Version:  version,
		Captured: time.Now(),
		Bytes:    int64(len(content)),
	}
	fh.index[path] = append(versions, fv)
	if err := fh.saveIndex(sessionID); err != nil {
		return fv, err
	}
	fh.evictLocked(sessionID)
	return fv, nil
}

// Versions returns the captured versions of a path, oldest first.
func (fh *FileHistory) Versions(sessionID, path string) ([]FileVersion, error) {
	fh.mu.Lock()
	defer fh.mu.Unlock()
	if err := fh.loadIndex(sessionID); err != nil {
		return nil, err
	}
	out := make([]FileVersion, len(fh.index[path]))
	copy(out, fh.index[path])
	return out, nil
}

// Restore writes the given version's content back to its original path.
// The file's current content is captured first so a restore can itself be
// undone.
func (fh *FileHistory) Restore(sessionID, path string, version int) error {
	versions, err := fh.Versions(sessionID, path)
	if err != nil {
		return err
	}
	var target *FileVersion
	for i := range versions {
		if versions[i].Version == version {
			target = &versions[i]
			break
		}
	}
	if target == nil {
		return fmt.Errorf("file-history: version %d of %s not found", version, path)
	}
	data, err := os.ReadFile(filepath.Join(fh.sessionDir(sessionID),
		fmt.Sprintf("%s-v%d", target.Hash[:16], version)))
	if err != nil {
		return fmt.Errorf("file-history: read backup: %w", err)
	}
	// Snapshot the current content so the restore is reversible.
	if current, err := os.ReadFile(path); err == nil {
		_, _ = fh.Capture(sessionID, path, string(current))
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("file-history: mkdir: %w", err)
	}
	return os.WriteFile(path, data, 0o600)
}

// loadIndex reads the session's index.json once per process.
func (fh *FileHistory) loadIndex(sessionID string) error {
	if _, ok := fh.index["_loaded:"+sessionID]; ok {
		return nil
	}
	data, err := os.ReadFile(fh.indexPath(sessionID))
	if err != nil {
		if os.IsNotExist(err) {
			fh.index["_loaded:"+sessionID] = nil
			return nil
		}
		return fmt.Errorf("file-history: read index: %w", err)
	}
	var flat []FileVersion
	if err := json.Unmarshal(data, &flat); err != nil {
		// A corrupt index must not brick capture; start fresh (backups on
		// disk are content-addressed and harmless).
		fh.index["_loaded:"+sessionID] = nil
		return nil
	}
	for _, v := range flat {
		fh.index[v.Path] = append(fh.index[v.Path], v)
	}
	fh.index["_loaded:"+sessionID] = nil
	return nil
}

func (fh *FileHistory) saveIndex(sessionID string) error {
	var flat []FileVersion
	for path, versions := range fh.index {
		if strings.HasPrefix(path, "_loaded:") {
			continue
		}
		flat = append(flat, versions...)
	}
	data, err := json.MarshalIndent(flat, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(fh.indexPath(sessionID), data, 0o600)
}

// evictLocked drops the oldest backups once the session exceeds the cap.
// Caller holds fh.mu.
func (fh *FileHistory) evictLocked(sessionID string) {
	var flat []FileVersion
	for path, versions := range fh.index {
		if strings.HasPrefix(path, "_loaded:") {
			continue
		}
		flat = append(flat, versions...)
	}
	if len(flat) <= maxFileHistoryFiles {
		return
	}
	sort.Slice(flat, func(i, j int) bool { return flat[i].Captured.Before(flat[j].Captured) })
	for _, victim := range flat[:len(flat)-maxFileHistoryFiles] {
		list := fh.index[victim.Path]
		for i, v := range list {
			if v.Version == victim.Version {
				fh.index[victim.Path] = append(list[:i], list[i+1:]...)
				break
			}
		}
		_ = os.Remove(filepath.Join(fh.sessionDir(sessionID),
			fmt.Sprintf("%s-v%d", victim.Hash[:16], victim.Version)))
	}
	_ = fh.saveIndex(sessionID)
}
