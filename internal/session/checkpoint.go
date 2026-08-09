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
	return os.WriteFile(filepath.Join(dir, name), data, 0o644)
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
