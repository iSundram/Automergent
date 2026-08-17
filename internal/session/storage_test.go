package session

import (
	"os"
	"testing"
	"time"

	"github.com/iSundram/Automergent/internal/ai"
)

func newTestMessage(text string) ai.Message {
	return ai.NewTextMessage(ai.RoleUser, text)
}

func TestAtomicWriteFile(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/test.json"
	data := []byte(`{"hello":"world"}`)

	if err := atomicWriteFile(path, data, 0o600); err != nil {
		t.Fatalf("atomicWriteFile: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != string(data) {
		t.Errorf("got %q, want %q", got, data)
	}

	// Verify permissions.
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("perm = %o, want 0600", info.Mode().Perm())
	}
}

func TestStorageDirectoryPermissions(t *testing.T) {
	parent := t.TempDir()
	dir := parent + "/sessions"

	_, err := NewStorage(dir)
	if err != nil {
		t.Fatalf("NewStorage: %v", err)
	}

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Errorf("dir perm = %o, want 0700", info.Mode().Perm())
	}
}

func TestStorageSaveLoad(t *testing.T) {
	dir := t.TempDir()
	storage, err := NewStorage(dir)
	if err != nil {
		t.Fatalf("NewStorage: %v", err)
	}

	sess := New()
	sess.Title = "test session"
	sess.Provider = "google"
	sess.Model = "gemini-3.6-flash"
	sess.AddMessage(newTestMessage("hello"))

	if err := storage.Save(sess); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := storage.Load(sess.ID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Title != sess.Title {
		t.Errorf("title mismatch: got %q, want %q", loaded.Title, sess.Title)
	}
	if loaded.Provider != sess.Provider {
		t.Errorf("provider mismatch: got %q, want %q", loaded.Provider, sess.Provider)
	}
	if loaded.Model != sess.Model {
		t.Errorf("model mismatch: got %q, want %q", loaded.Model, sess.Model)
	}
	if len(loaded.Messages) != len(sess.Messages) {
		t.Errorf("message count: got %d, want %d", len(loaded.Messages), len(sess.Messages))
	}

	// Check file permissions.
	info, err := os.Stat(dir + "/" + sess.ID + ".json")
	if err != nil {
		t.Fatalf("stat session file: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("session file perm = %o, want 0600", info.Mode().Perm())
	}
}

func TestStorageApprovalsSurviveSaveLoad(t *testing.T) {
	dir := t.TempDir()
	storage, err := NewStorage(dir)
	if err != nil {
		t.Fatalf("NewStorage: %v", err)
	}

	sess := New()
	sess.AddApproval(`name="run_shell_command";action=write;risk=high`, "tui")
	sess.AddApproval(`name="run_shell_command";action=write;risk=high`, "tui") // duplicate ignored
	sess.AddApproval(`name="edit_file";action=write;risk=low`, "headless")

	if err := storage.Save(sess); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := storage.Load(sess.ID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(loaded.AllowedTools) != 2 {
		t.Fatalf("expected 2 approvals after round-trip, got %+v", loaded.AllowedTools)
	}
	if !loaded.HasApproval(`name="edit_file";action=write;risk=low`) {
		t.Fatalf("expected edit_file approval to survive round-trip")
	}

	// Revocation round-trips too.
	if !loaded.RemoveApproval(`name="edit_file";action=write;risk=low`) {
		t.Fatalf("expected RemoveApproval to find the scope")
	}
	if err := storage.Save(loaded); err != nil {
		t.Fatalf("Save after revoke: %v", err)
	}
	reloaded, err := storage.Load(sess.ID)
	if err != nil {
		t.Fatalf("Load after revoke: %v", err)
	}
	if len(reloaded.AllowedTools) != 1 {
		t.Fatalf("expected 1 approval after revoke round-trip, got %+v", reloaded.AllowedTools)
	}
}

func TestStoragePruneCleansCheckpointsAndRecovery(t *testing.T) {
	dir := t.TempDir()
	storage, err := NewStorage(dir)
	if err != nil {
		t.Fatalf("NewStorage: %v", err)
	}

	s := New()
	if err := storage.Save(s); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Orphaned checkpoint for a session that no longer exists.
	orphan := dir + "/deadbeef_cp0001.json"
	if err := os.WriteFile(orphan, []byte(`{}`), 0o600); err != nil {
		t.Fatalf("write orphan checkpoint: %v", err)
	}
	// Recent checkpoint for the alive session (kept).
	recent := dir + "/" + s.ID + "_cp0001.json"
	if err := os.WriteFile(recent, []byte(`{}`), 0o600); err != nil {
		t.Fatalf("write recent checkpoint: %v", err)
	}
	// Stale recovery file referencing a dead session.
	if err := os.WriteFile(dir+"/recovery.json", []byte(`{"session":{"id":"deadbeef"}}`), 0o600); err != nil {
		t.Fatalf("write recovery: %v", err)
	}

	if err := storage.Prune(0, 0); err != nil {
		t.Fatalf("Prune: %v", err)
	}

	if _, err := os.Stat(orphan); !os.IsNotExist(err) {
		t.Errorf("expected orphan checkpoint to be removed")
	}
	if _, err := os.Stat(dir + "/recovery.json"); !os.IsNotExist(err) {
		t.Errorf("expected stale recovery.json to be removed")
	}
	if _, err := os.Stat(recent); err != nil {
		t.Errorf("expected recent checkpoint for alive session to be kept: %v", err)
	}
}

func TestStoragePrune(t *testing.T) {
	dir := t.TempDir()
	storage, err := NewStorage(dir)
	if err != nil {
		t.Fatalf("NewStorage: %v", err)
	}

	// Create 5 sessions.
	for i := 0; i < 5; i++ {
		s := New()
		s.Title = "session"
		if err := storage.Save(s); err != nil {
			t.Fatalf("Save: %v", err)
		}
	}

	// Prune to max 3.
	if err := storage.Prune(3, 0); err != nil {
		t.Fatalf("Prune: %v", err)
	}

	sessions, err := storage.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(sessions) != 3 {
		t.Errorf("after prune: got %d sessions, want 3", len(sessions))
	}
}

func TestStoragePruneByAge(t *testing.T) {
	dir := t.TempDir()
	storage, err := NewStorage(dir)
	if err != nil {
		t.Fatalf("NewStorage: %v", err)
	}

	s := New()
	s.Title = "old session"
	// Force UpdatedAt to be in the past.
	s.UpdatedAt = time.Now().Add(-48 * time.Hour)
	if err := storage.Save(s); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Prune sessions older than 24h.
	if err := storage.Prune(0, 24*time.Hour); err != nil {
		t.Fatalf("Prune: %v", err)
	}

	sessions, err := storage.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions after age prune, got %d", len(sessions))
	}
}
