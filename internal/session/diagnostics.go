package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// SessionDiagnostics summarizes health and metrics for a session storage directory.
type SessionDiagnostics struct {
	TotalSessions       int           `json:"total_sessions"`
	CorruptSessions     []string      `json:"corrupt_sessions,omitempty"`
	OrphanedCheckpoints []string      `json:"orphaned_checkpoints,omitempty"`
	TotalInputTokens    int           `json:"total_input_tokens"`
	TotalOutputTokens   int           `json:"total_output_tokens"`
	EstimatedCostUSD    float64       `json:"estimated_cost_usd"`
	OldestSession       time.Time     `json:"oldest_session,omitempty"`
	NewestSession       time.Time     `json:"newest_session,omitempty"`
	StoreSizeBytes      int64         `json:"store_size_bytes"`
	ScannedAt           time.Time     `json:"scanned_at"`
}

// AuditStorage performs a comprehensive health and metrics audit of the session storage directory.
func AuditStorage(storageDir string) (*SessionDiagnostics, error) {
	diag := &SessionDiagnostics{
		ScannedAt: time.Now(),
	}

	entries, err := os.ReadDir(storageDir)
	if err != nil {
		if os.IsNotExist(err) {
			return diag, nil
		}
		return nil, fmt.Errorf("audit storage: read dir: %w", err)
	}

	aliveSessions := make(map[string]bool)
	var storeSize int64

	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		storeSize += info.Size()

		name := e.Name()
		if e.IsDir() {
			continue
		}

		path := filepath.Join(storageDir, name)

		if filepath.Ext(name) == ".json" {
			if stringsContains(name, "_cp") {
				// Checkpoint file
				owner := extractOwnerFromCheckpoint(name)
				if owner != "" {
					// We will verify owner later
				}
				continue
			}
			if name == "search_index.json" || name == "state.json" || name == "state.backup.json" || name == "recovery.json" {
				continue
			}

			// Regular session file
			data, err := os.ReadFile(path)
			if err != nil {
				diag.CorruptSessions = append(diag.CorruptSessions, name)
				continue
			}
			var sess Session
			if err := json.Unmarshal(data, &sess); err != nil {
				diag.CorruptSessions = append(diag.CorruptSessions, name)
				continue
			}

			aliveSessions[sess.ID] = true
			diag.TotalSessions++
			diag.TotalInputTokens += sess.TotalInputTokens
			diag.TotalOutputTokens += sess.TotalOutputTokens

			if diag.OldestSession.IsZero() || sess.CreatedAt.Before(diag.OldestSession) {
				diag.OldestSession = sess.CreatedAt
			}
			if diag.NewestSession.IsZero() || sess.UpdatedAt.After(diag.NewestSession) {
				diag.NewestSession = sess.UpdatedAt
			}
		}
	}

	// Second pass for orphaned checkpoints
	for _, e := range entries {
		name := e.Name()
		if stringsContains(name, "_cp") {
			owner := extractOwnerFromCheckpoint(name)
			if owner != "" && !aliveSessions[owner] {
				diag.OrphanedCheckpoints = append(diag.OrphanedCheckpoints, name)
			}
		}
	}

	diag.StoreSizeBytes = storeSize
	// Approximate cost calculation (e.g. blended $3 per M input tokens, $15 per M output tokens)
	diag.EstimatedCostUSD = (float64(diag.TotalInputTokens) * 0.000003) + (float64(diag.TotalOutputTokens) * 0.000015)

	return diag, nil
}

func stringsContains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || filepath.Ext(s) != "" && containsStr(s, substr))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func extractOwnerFromCheckpoint(name string) string {
	// Format: <session_id>_cp<index>.json
	for i := 0; i < len(name)-3; i++ {
		if name[i:i+3] == "_cp" {
			return name[:i]
		}
	}
	return ""
}
