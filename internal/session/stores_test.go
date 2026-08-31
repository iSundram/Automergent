package session

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/iSundram/Automergent/internal/ai"
)

func TestFileHistory_CaptureAndRestore(t *testing.T) {
	root := t.TempDir()
	fh := NewFileHistory(root)
	target := filepath.Join(root, "file.txt")

	os.WriteFile(target, []byte("v1"), 0o600)
	fh.Capture("sess1", target, "v1")
	os.WriteFile(target, []byte("v2"), 0o600)
	fh.Capture("sess1", target, "v2")

	versions, err := fh.Versions("sess1", target)
	if err != nil || len(versions) != 2 {
		t.Fatalf("versions = %v err=%v, want 2", versions, err)
	}

	// Restore v1; the pre-restore content is new, so it is captured too
	// (restore is reversible).
	os.WriteFile(target, []byte("v3-never-captured"), 0o600)
	if err := fh.Restore("sess1", target, 1); err != nil {
		t.Fatalf("restore: %v", err)
	}
	data, _ := os.ReadFile(target)
	if string(data) != "v1" {
		t.Fatalf("restored content = %q, want %q", data, "v1")
	}
	versions, _ = fh.Versions("sess1", target)
	if len(versions) != 3 {
		t.Fatalf("restore did not capture pre-restore content: %d versions", len(versions))
	}

	// Identical content is deduplicated.
	before, _ := fh.Versions("sess1", target)
	fh.Capture("sess1", target, "v1")
	after, _ := fh.Versions("sess1", target)
	if len(after) != len(before) {
		t.Fatal("identical content created a duplicate version")
	}
}

func TestStats_RecordAndSnapshot(t *testing.T) {
	root := t.TempDir()
	tr := newStatsTracker(root)
	s1 := New()
	s1.Model = "gemini-3"
	s1.TotalInputTokens = 100
	s1.TotalOutputTokens = 50
	tr.recordSession(s1, 10)
	// Re-record with the same totals: must not double-count.
	tr.recordSession(s1, 10)

	// A second tracker (new process) reads the same file.
	snap := newStatsTracker(root).snapshot()
	if snap.TotalSessions != 1 || snap.TotalIn != 100 || snap.TotalOut != 50 {
		t.Fatalf("snapshot = %+v, want 1 session / 100 / 50", snap)
	}
	if snap.TotalMessages != 10 {
		t.Fatalf("messages = %d, want 10", snap.TotalMessages)
	}
	if snap.ModelTokens["gemini-3"] != [2]int{100, 50} {
		t.Fatalf("model tokens = %v", snap.ModelTokens)
	}
}

func TestPruneCleansFileHistoryAndTempDebris(t *testing.T) {
	root := t.TempDir()
	s, _ := NewStorage(root)
	sess := New()
	sess.AddMessage(ai.NewTextMessage(ai.RoleUser, "hi"))
	if err := s.Save(sess); err != nil {
		t.Fatalf("save: %v", err)
	}
	fh := s.FileHistory()
	if _, err := fh.Capture(sess.ID, "/some/file.go", "content"); err != nil {
		t.Fatalf("capture: %v", err)
	}
	// Simulate leaked atomic-write debris.
	os.WriteFile(filepath.Join(root, ".automergent-tmp-123"), []byte("x"), 0o600)

	if err := s.Prune(0, 0); err != nil {
		t.Fatalf("prune: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".automergent-tmp-123")); !os.IsNotExist(err) {
		t.Fatal("temp debris not swept")
	}

	if err := s.Delete(sess.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "file-history", sess.ID)); !os.IsNotExist(err) {
		t.Fatal("file-history dir not removed with session")
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
