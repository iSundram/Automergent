package installer

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAtomicWriteFileReplacesExisting(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "update-config.json")

	if err := os.WriteFile(target, []byte("old"), 0644); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	if err := atomicWriteFile(target, []byte("new"), 0644); err != nil {
		t.Fatalf("atomic write: %v", err)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read result: %v", err)
	}
	if string(got) != "new" {
		t.Fatalf("unexpected content: %q", string(got))
	}
}

func TestAcquireUpdateLockTimesOutWhenHeld(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "update.lock")

	lock, err := acquireUpdateLockWithParams(lockPath, time.Hour, 200*time.Millisecond, 25*time.Millisecond)
	if err != nil {
		t.Fatalf("initial lock acquire: %v", err)
	}
	defer lock.Release()

	if _, err := acquireUpdateLockWithParams(lockPath, time.Hour, 100*time.Millisecond, 25*time.Millisecond); err == nil {
		t.Fatalf("expected lock contention error")
	}
}

func TestAcquireUpdateLockBreaksStaleLock(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "update.lock")

	if err := os.WriteFile(lockPath, []byte("stale"), 0600); err != nil {
		t.Fatalf("write stale lock: %v", err)
	}
	staleTime := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(lockPath, staleTime, staleTime); err != nil {
		t.Fatalf("mark stale lock: %v", err)
	}

	lock, err := acquireUpdateLockWithParams(lockPath, time.Hour, 200*time.Millisecond, 25*time.Millisecond)
	if err != nil {
		t.Fatalf("acquire after stale lock: %v", err)
	}
	defer lock.Release()
}
