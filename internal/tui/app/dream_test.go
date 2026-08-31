package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Memory consolidation gates: interval, message volume, and the
// cross-process lock (dream.go).

func newDreamTestApp(t *testing.T) *App {
	t.Helper()
	app := newTestApp(t)
	app.workDir = t.TempDir()
	return app
}

func TestConsolidateMemoryManualFires(t *testing.T) {
	app := newDreamTestApp(t)
	app.dreamLastAt = time.Now() // manual skips the interval gate
	app.ConsolidateMemory()
	if !app.dreamRunning {
		t.Fatal("manual /dream must start a consolidation pass")
	}
}

func TestAutoConsolidateGatedByInterval(t *testing.T) {
	app := newDreamTestApp(t)
	app.dreamLastAt = time.Now().Add(-time.Minute) // too recent
	app.maybeConsolidateMemory()
	if app.dreamRunning {
		t.Fatal("consolidation must not fire within the minimum interval")
	}
}

func TestAutoConsolidateGatedByVolume(t *testing.T) {
	app := newDreamTestApp(t)
	app.dreamLastAt = time.Now().Add(-time.Hour) // interval passed
	if len(app.sess.Messages) >= dreamMinNewMessages {
		t.Skip("unexpected session size")
	}
	app.maybeConsolidateMemory()
	if app.dreamRunning {
		t.Fatal("consolidation must not fire without enough new messages")
	}
}

func TestDreamLockExcludesSecondHolder(t *testing.T) {
	app := newDreamTestApp(t)
	if !app.acquireDreamLock() {
		t.Fatal("first acquisition must succeed")
	}
	defer app.releaseDreamLock()

	app2 := newDreamTestApp(t)
	app2.workDir = app.workDir
	if app2.acquireDreamLock() {
		t.Fatal("second acquisition while held must fail")
	}
}

func TestDreamStaleLockIsBroken(t *testing.T) {
	dir := t.TempDir()
	lock := filepath.Join(dir, ".automergent", "dream.lock")
	if err := os.MkdirAll(filepath.Dir(lock), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lock, []byte("12345\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A lock older than the max age is a dead holder — breakable.
	old := time.Now().Add(-2 * dreamLockMaxAge)
	if err := os.Chtimes(lock, old, old); err != nil {
		t.Fatal(err)
	}

	app := newDreamTestApp(t)
	app.workDir = dir
	if !app.acquireDreamLock() {
		t.Fatal("stale lock must be breakable")
	}
	app.releaseDreamLock()
	if _, err := os.Stat(lock); !os.IsNotExist(err) {
		t.Fatal("release must remove the lock")
	}
}

func TestDreamPromptStatesLineCap(t *testing.T) {
	prompt := dreamPrompt("/project", 42)
	if !contains(prompt, "200 lines") {
		t.Fatalf("prompt must state the hard line cap: %q", prompt)
	}
	if !contains(prompt, "/project") {
		t.Fatal("prompt must name the project directory")
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestDreamPromptForbidsDeletingMemoryFile(t *testing.T) {
	prompt := dreamPrompt("/repo", 42)
	for _, want := range []string{
		"NEVER delete AUTOMERGENT.md",
		"not a delete-and-recreate",
		"NEVER touch any file other than AUTOMERGENT.md",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("consolidation prompt missing safety rule %q", want)
		}
	}
}
