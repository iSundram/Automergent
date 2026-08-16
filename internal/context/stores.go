package context

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ToolResultStore persists large tool outputs to disk.
type ToolResultStore struct {
	mu   sync.Mutex
	root string
}

// NewToolResultStore creates a store for tool results.
func NewToolResultStore(rootDir string) *ToolResultStore {
	dir := filepath.Join(rootDir, ".automergent", "tool-results")
	return &ToolResultStore{root: dir}
}

// Store writes a tool result to disk and returns the path.
func (trs *ToolResultStore) Store(content string) (string, error) {
	trs.mu.Lock()
	defer trs.mu.Unlock()

	if err := os.MkdirAll(trs.root, 0o700); err != nil {
		return "", err
	}

	hash := sha256.Sum256([]byte(content))
	name := hex.EncodeToString(hash[:16]) + ".txt"
	path := filepath.Join(trs.root, name)

	// Atomic write
	tmp, err := os.CreateTemp(trs.root, ".automergent-tr-*")
	if err != nil {
		return "", err
	}
	tmpPath := tmp.Name()
	defer func() {
		tmp.Close()
		os.Remove(tmpPath)
	}()

	if _, err := tmp.WriteString(content); err != nil {
		return "", err
	}
	if err := tmp.Sync(); err != nil {
		return "", err
	}
	if err := tmp.Chmod(0o600); err != nil {
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return "", err
	}

	return path, nil
}

// Load retrieves a tool result by hash.
func (trs *ToolResultStore) Load(hash string) (string, error) {
	path := filepath.Join(trs.root, hash+".txt")
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// MasterSnapshot is the structured state snapshot (gemini-style).
type MasterSnapshot struct {
	ActiveTasks          []string `json:"active_tasks"`
	DiscoveredFacts      []string `json:"discovered_facts"`
	ConstraintsAndPrefs  []string `json:"constraints_and_preferences"`
	RecentArc            []string `json:"recent_arc"`
	CreatedAt            time.Time `json:"created_at"`
	TokenCount           int       `json:"token_count,omitempty"`
}

// Summarize produces a human-readable summary of the snapshot.
func (ms *MasterSnapshot) Summarize() string {
	var sb strings.Builder
	if len(ms.ActiveTasks) > 0 {
		sb.WriteString("## Active Tasks\n")
		for _, t := range ms.ActiveTasks {
			sb.WriteString("- " + t + "\n")
		}
		sb.WriteString("\n")
	}
	if len(ms.DiscoveredFacts) > 0 {
		sb.WriteString("## Discovered Facts\n")
		for _, f := range ms.DiscoveredFacts {
			sb.WriteString("- " + f + "\n")
		}
		sb.WriteString("\n")
	}
	if len(ms.ConstraintsAndPrefs) > 0 {
		sb.WriteString("## Constraints & Preferences\n")
		for _, c := range ms.ConstraintsAndPrefs {
			sb.WriteString("- " + c + "\n")
		}
		sb.WriteString("\n")
	}
	if len(ms.RecentArc) > 0 {
		sb.WriteString("## Recent Arc\n")
		for _, a := range ms.RecentArc {
			sb.WriteString("- " + a + "\n")
		}
	}
	return sb.String()
}

// SnapshotStore persists MasterSnapshot JSON files.
type SnapshotStore struct {
	mu   sync.Mutex
	root string
}

// NewSnapshotStore creates a snapshot store.
func NewSnapshotStore(rootDir string) *SnapshotStore {
	dir := filepath.Join(rootDir, ".automergent", "snapshots")
	return &SnapshotStore{root: dir}
}

// Store writes a snapshot to disk.
func (ss *SnapshotStore) Store(snap *MasterSnapshot) error {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	if err := os.MkdirAll(ss.root, 0o700); err != nil {
		return err
	}

	name := fmt.Sprintf("snapshot-%d.json", time.Now().Unix())
	path := filepath.Join(ss.root, name)

	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return err
	}

	// Atomic write
	tmp, err := os.CreateTemp(ss.root, ".automergent-snap-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() {
		tmp.Close()
		os.Remove(tmpPath)
	}()

	if _, err := tmp.Write(data); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

// LoadLatest returns the most recent snapshot.
func (ss *SnapshotStore) LoadLatest() (*MasterSnapshot, error) {
	entries, err := os.ReadDir(ss.root)
	if err != nil {
		return nil, err
	}
	var latest string
	latestTime := time.Time{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		info, _ := e.Info()
		if info.ModTime().After(latestTime) {
			latestTime = info.ModTime()
			latest = e.Name()
		}
	}
	if latest == "" {
		return nil, os.ErrNotExist
	}
	data, err := os.ReadFile(filepath.Join(ss.root, latest))
	if err != nil {
		return nil, err
	}
	var snap MasterSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil, err
	}
	return &snap, nil
}