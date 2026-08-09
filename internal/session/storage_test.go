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
