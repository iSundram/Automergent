package shell

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWrapWithCwdCapturePreservesExitCode(t *testing.T) {
	wrapped, cwdFile := wrapWithCwdCapture("exit 3")
	if cwdFile == "" {
		t.Skip("no temp dir for cwd capture")
	}
	defer os.Remove(cwdFile)
	if !strings.Contains(wrapped, "exit $__ec") {
		t.Fatalf("wrapped command must preserve the exit code: %q", wrapped)
	}
}

func TestCwdCaptureRoundTrip(t *testing.T) {
	dir := t.TempDir()
	f, err := os.CreateTemp("", "cwd-*")
	if err != nil {
		t.Fatal(err)
	}
	path := f.Name()
	if _, err := f.WriteString(dir); err != nil {
		t.Fatal(err)
	}
	f.Close()

	if got := readCapturedCwd(path); got != dir {
		t.Fatalf("captured cwd = %q, want %q", got, dir)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("capture file must be removed after reading")
	}
}

func TestCwdPersistenceAndRecovery(t *testing.T) {
	dir := t.TempDir()
	SetOriginalCwd(dir)
	defer func() { managerCwd.Store(nil); originalCwd.Store(nil) }()

	// No tracked cwd yet: resolves to the original dir.
	if got := resolveCwd(""); got != dir {
		t.Fatalf("initial cwd = %q, want %q", got, dir)
	}

	// Track a directory that then disappears: recovery falls back.
	vanishing := filepath.Join(dir, "gone")
	if err := os.Mkdir(vanishing, 0o755); err != nil {
		t.Fatal(err)
	}
	updateCwd(vanishing)
	if got := resolveCwd(""); got != vanishing {
		t.Fatalf("tracked cwd not used: %q", got)
	}
	if err := os.Remove(vanishing); err != nil {
		t.Fatal(err)
	}
	if got := resolveCwd(""); got != dir {
		t.Fatalf("deleted cwd must recover to the original dir, got %q", got)
	}

	// Explicit cwd wins over the tracked one.
	nested := filepath.Join(dir, "nested")
	if err := os.Mkdir(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	updateCwd(dir)
	if got := resolveCwd(nested); got != nested {
		t.Fatalf("explicit cwd = %q, want %q", got, nested)
	}
}

func TestLooksLikePrompt(t *testing.T) {
	prompts := []string{
		"Do you want to continue? [y/n]",
		"Press any key to continue...",
		"Enter password:",
		"Continue? (yes/no)",
	}
	for _, p := range prompts {
		if !looksLikePrompt(p) {
			t.Errorf("looksLikePrompt(%q) = false", p)
		}
	}
	notPrompts := []string{
		"Compiled successfully in 2.1s",
		"12 tests passed, 0 failed",
		"",
	}
	for _, p := range notPrompts {
		if looksLikePrompt(p) {
			t.Errorf("looksLikePrompt(%q) = true", p)
		}
	}
}

func TestTrimBufferBoundsRAM(t *testing.T) {
	s := &AsyncSession{stdoutReadPos: 0, stderrReadPos: 0}
	s.Stdout = &bytes.Buffer{}
	s.Stderr = &bytes.Buffer{}

	chunk := bytes.Repeat([]byte("x"), 64*1024)
	// 8 chunks of 64KB = 512KB > 256KB cap; buffers must stay bounded.
	for i := 0; i < 8; i++ {
		s.noteStdout(chunk)
	}
	if s.Stdout.Len() > maxSessionBufferBytes {
		t.Fatalf("stdout buffer unbounded: %d bytes", s.Stdout.Len())
	}
	if !s.truncated {
		t.Error("trimming must set the truncated flag")
	}

	// The read position is adjusted (clamped at zero), never negative.
	if s.stdoutReadPos < 0 {
		t.Fatalf("read position went negative: %d", s.stdoutReadPos)
	}
}

func TestStallWatchdogNotifiesModel(t *testing.T) {
	notifications := make(chan string, 1)
	RegisterModelNotification(func(message string) {
		notifications <- message
	})

	s := &AsyncSession{
		ID:      "test-stall",
		Command: "fake-interactive",
		done:    make(chan struct{}),
	}
	s.Stdout = &bytes.Buffer{}
	s.Stderr = &bytes.Buffer{}
	// Simulate a session that printed a prompt and then went quiet.
	s.noteStdout([]byte("Do you want to continue? [y/n] "))
	// Pretend the last growth was a long time ago.
	s.lastGrowth.Store(time.Now().Add(-2 * stallThreshold).UnixNano())

	GetManager().Create(s.ID, s)
	defer GetManager().Delete(s.ID)
	GetManager().startWatchdogs(s)

	select {
	case msg := <-notifications:
		if !strings.Contains(msg, "interactive input") {
			t.Fatalf("notification lacks advice: %q", msg)
		}
		if !strings.Contains(msg, "test-stall") {
			t.Fatalf("notification lacks task id: %q", msg)
		}
	case <-time.After(2*stallCheckInterval + time.Second):
		t.Fatal("stall watchdog did not notify within the check interval")
	}
	close(s.done)
}

func TestOutputPathForSanitizesID(t *testing.T) {
	path := outputPathFor("../../etc/passwd")
	// The sanitizer replaces every separator, so no traversal can survive:
	// the resolved path must be a direct child of the shells directory.
	if filepath.Dir(path) != filepath.Dir(outputPathFor("safe")) {
		t.Fatalf("unsafe ID escaped the shells dir: %q", path)
	}
	if strings.ContainsRune(strings.TrimPrefix(path, filepath.Dir(path)+string(filepath.Separator)), '/') {
		t.Fatalf("path contains separators after sanitization: %q", path)
	}
	if !strings.HasSuffix(path, ".log") {
		t.Fatalf("output path = %q", path)
	}
}

func TestShellDiscovery(t *testing.T) {
	bin := shellBin()
	if bin == "" {
		t.Fatal("shellBin must never be empty")
	}
	// Without overrides it resolves to an executable-looking path.
	t.Logf("resolved shell: %s", bin)
}

func TestTailOutputFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.log")
	lines := make([]string, 300)
	for i := range lines {
		lines[i] = strings.Repeat("a", 10)
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}
	tail, err := tailOutputFile(path, 50)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(strings.Split(tail, "\n")); got != 50 {
		t.Fatalf("tail lines = %d, want 50", got)
	}
	if _, err := tailOutputFile(filepath.Join(dir, "missing.log"), 10); err == nil {
		t.Error("missing file must error")
	}
}
