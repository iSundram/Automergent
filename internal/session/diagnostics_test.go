package session

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/iSundram/Automergent/internal/ai"
)

func TestAuditStorage(t *testing.T) {
	dir := t.TempDir()
	storage, err := NewStorage(dir)
	if err != nil {
		t.Fatalf("NewStorage: %v", err)
	}

	sess := New()
	sess.Title = "Audit Test"
	sess.AddUsage(aiUsageForTest(1000, 500))
	if err := storage.Save(sess); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Create an orphaned checkpoint
	_ = os.WriteFile(filepath.Join(dir, "nonexistent_cp0001.json"), []byte("{}"), 0o600)

	// Create a corrupt session file
	_ = os.WriteFile(filepath.Join(dir, "corrupt.json"), []byte("{invalid json"), 0o600)

	diag, err := AuditStorage(dir)
	if err != nil {
		t.Fatalf("AuditStorage: %v", err)
	}

	if diag.TotalSessions != 1 {
		t.Errorf("TotalSessions = %d, want 1", diag.TotalSessions)
	}
	if len(diag.CorruptSessions) != 1 {
		t.Errorf("CorruptSessions count = %d, want 1", len(diag.CorruptSessions))
	}
	if len(diag.OrphanedCheckpoints) != 1 {
		t.Errorf("OrphanedCheckpoints count = %d, want 1", len(diag.OrphanedCheckpoints))
	}
	if diag.TotalInputTokens != 1000 {
		t.Errorf("TotalInputTokens = %d, want 1000", diag.TotalInputTokens)
	}
}

func aiUsageForTest(in, out int) ai.Usage {
	return ai.Usage{InputTokens: in, OutputTokens: out}
}
