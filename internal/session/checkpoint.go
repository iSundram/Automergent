package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Checkpoint is a saved snapshot of a session at a point in time.
type Checkpoint struct {
	SessionID string    `json:"session_id"`
	CreatedAt time.Time `json:"created_at"`
	Index     int       `json:"index"`
	Session   *Session  `json:"session"`
}

// SaveCheckpoint writes a checkpoint to disk.
func SaveCheckpoint(dir string, s *Session, index int) error {
	cp := &Checkpoint{
		SessionID: s.ID,
		CreatedAt: time.Now(),
		Index:     index,
		Session:   s,
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("checkpoint mkdir: %w", err)
	}
	data, err := json.MarshalIndent(cp, "", "  ")
	if err != nil {
		return fmt.Errorf("checkpoint marshal: %w", err)
	}
	name := fmt.Sprintf("%s_cp%04d.json", s.ID, index)
	path := filepath.Join(dir, name)

	// Write atomically with restricted permissions
	tmp, err := os.CreateTemp(dir, ".automergent-cp-tmp-*")
	if err != nil {
		return fmt.Errorf("checkpoint temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() {
		tmp.Close()
		os.Remove(tmpPath)
	}()
	if _, err := tmp.Write(data); err != nil {
		return fmt.Errorf("write checkpoint: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("sync checkpoint: %w", err)
	}
	tmp.Close()
	if err := os.Chmod(tmpPath, 0o600); err != nil {
		return fmt.Errorf("chmod checkpoint: %w", err)
	}
	return os.Rename(tmpPath, path)
}

// LoadCheckpoint reads a checkpoint file.
func LoadCheckpoint(path string) (*Checkpoint, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cp Checkpoint
	if err := json.Unmarshal(data, &cp); err != nil {
		return nil, err
	}
	return &cp, nil
}
