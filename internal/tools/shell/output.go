package shell

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
)

// File-backed output, watchdogs, process-group control, persistent cwd, and
// shell discovery — the shell parity layer, ported from the reference agent's
// design:
//
//   - Output streams to a per-session file (the durable record) while a
//     BOUNDED slice of it stays in RAM for the dock and read deltas. Before
//     this, output accumulated in unbounded bytes.Buffers: a chatty build
//     was a memory leak with a process attached.
//   - The size watchdog kills a session whose output file exceeds the cap.
//   - The stall watchdog notices a session whose output stopped growing and
//     whose tail looks like an interactive prompt, and tells the model with
//     actionable advice instead of letting it wait out the timeout.
//   - Commands run in their own process group so Kill() takes the whole
//     tree (pipelines, backgrounded children) instead of just bash.
//   - The manager tracks the shell's working directory across commands
//     (pwd -P capture appended per command), with recovery when the
//     directory is deleted underneath us.

const (
	// maxSessionBufferBytes bounds the in-RAM copy of a session's output.
	// The full output always lives in the session file.
	maxSessionBufferBytes = 256 * 1024

	// MaxOutputFileBytes caps the on-disk output file per session; the size
	// watchdog kills the process when it is exceeded.
	MaxOutputFileBytes = 5 * 1024 * 1024 * 1024 // 5GB, matching the reference agent

	// stallCheckInterval and stallThreshold mirror the reference agent's
	// interactive-prompt watchdog cadence.
	stallCheckInterval = 5 * time.Second
	stallThreshold     = 45 * time.Second

	// outputDirName is where session output files live, under the
	// automergent home.
	outputDirName = "shells"
)

// atomicInt64 aliases the atomic int64 used for the growth timestamp.
type atomicInt64 = atomic.Int64

// shellsBaseDir resolves ~/.automergent/shells, creating it when possible.
func shellsBaseDir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	dir := filepath.Join(home, ".automergent", outputDirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return ""
	}
	return dir
}

// outputPathFor returns the output file path for a session ID.
func outputPathFor(id string) string {
	base := shellsBaseDir()
	if base == "" {
		return ""
	}
	return filepath.Join(base, sanitizeShellID(id)+".log")
}

// sanitizeShellID makes a session ID safe as a filename.
func sanitizeShellID(id string) string {
	var b strings.Builder
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	return b.String()
}

// attachOutputFile opens (appending) the session's output file.
func (s *AsyncSession) attachOutputFile() {
	path := outputPathFor(s.ID)
	if path == "" {
		return
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	s.mu.Lock()
	s.OutputPath = path
	s.outputFile = f
	s.mu.Unlock()
}

// OutputPath field accessors are not needed; the field is read under the
// session lock by read_shell and the watchdogs.

// noteFileOutput appends p to the session's output file (best-effort; the
// RAM buffer remains the live view). Caller must NOT hold the session lock.
func (s *AsyncSession) noteFileOutput(p []byte) {
	s.mu.Lock()
	f := s.outputFile
	s.mu.Unlock()
	if f == nil {
		return
	}
	_, _ = f.Write(p)
}

// noteGrowth records that output arrived, for the stall watchdog. Lock-free.
func (s *AsyncSession) noteGrowth() {
	s.lastGrowth.Store(time.Now().UnixNano())
}

// OutputSize returns the current output-file size (0 when file-backed
// output is unavailable).
func (s *AsyncSession) OutputSize() int64 {
	s.mu.Lock()
	f := s.outputFile
	s.mu.Unlock()
	if f == nil {
		return 0
	}
	if pos, err := f.Seek(0, 1); err == nil {
		return pos
	}
	return 0
}

// Kill terminates the session's whole process group. Commands are spawned
// with Setpgid so the group ID equals the process ID; signalling the
// negative PID reaches every descendant (pipelines, `&` children) instead
// of leaving orphans behind when only the shell itself dies.
func (s *AsyncSession) Kill() error {
	s.mu.Lock()
	proc := s.Cmd.Process
	s.mu.Unlock()
	if proc == nil {
		return nil
	}
	if err := syscall.Kill(-proc.Pid, syscall.SIGKILL); err != nil {
		// Group kill can fail (process already reaped, or not a group
		// leader on this platform) — fall back to the direct kill.
		return proc.Kill()
	}
	return nil
}

// promptPatterns match the interactive prompts that stall long-running
// commands: confirmation questions, password asks, "press any key" pauses.
var promptPatterns = []string{
	"y/n", "[y/n]", "y/n]", "(y/n",
	"yes/no", "[yes/no]",
	"press any key", "press enter", "press return",
	"do you want", "would you like", "continue?",
	"enter password", "password:", "passphrase",
	"username:", "login:",
	"confirm", "proceed?",
	"are you sure",
}

// looksLikePrompt reports whether the tail of output resembles an
// interactive prompt the command is blocked on.
func looksLikePrompt(tail string) bool {
	lower := strings.ToLower(tail)
	for _, pattern := range promptPatterns {
		if strings.Contains(lower, pattern) {
			return true
		}
	}
	return false
}

// startWatchdogs runs the stall and size watchdogs for a running session.
// Both stop when the session completes; each fires at most once.
func (m *SessionManager) startWatchdogs(s *AsyncSession) {
	if s.OutputPath == "" {
		// Without file-backed output the stall watchdog can still run off
		// the RAM buffer, but there is no size to watch. Keep it simple:
		// watchdogs require the output file.
		if s.Stdout == nil {
			return
		}
	}
	go func() {
		ticker := time.NewTicker(stallCheckInterval)
		defer ticker.Stop()
		var lastSize int64
		stallNotified := false
		sizeKilled := false
		for {
			select {
			case <-s.done:
				return
			case <-ticker.C:
				if s.IsCompleted() {
					return
				}
				size := s.OutputSize()
				if size > lastSize {
					lastSize = size
					s.noteGrowth()
				}
				if !sizeKilled && size > MaxOutputFileBytes {
					sizeKilled = true
					m.notifyModel(
						"<task-notification>\n<task-id>%s</task-id>\n<summary>Output of %q exceeded the %d-byte cap; the command was killed.</summary>\n</task-notification>",
						s.ID, truncateCommand(s.Command), MaxOutputFileBytes)
					_ = s.Kill()
					return
				}
				if stallNotified {
					continue
				}
				lastNano := s.lastGrowth.Load()
				if lastNano == 0 {
					continue
				}
				idle := time.Since(time.Unix(0, lastNano))
				if idle < stallThreshold {
					continue
				}
				tail := s.outputTail(512)
				if !looksLikePrompt(tail) {
					// Not a prompt: keep watching, re-armed from now.
					s.noteGrowth()
					continue
				}
				stallNotified = true
				m.notifyModel(
					"<task-notification>\n<task-id>%s</task-id>\n<output-file>%s</output-file>\n<summary>Command %q appears to be waiting for interactive input</summary>\n</task-notification>\nThe command is likely blocked on an interactive prompt. Kill this shell (stop_shell) and re-run with piped input (e.g., `echo y | command`) or a non-interactive flag if one exists.\n\nLast output:\n%s",
					s.ID, s.OutputPath, truncateCommand(s.Command), strings.TrimSpace(tail))
			}
		}
	}()
}

// outputTail returns the last n bytes of the session's combined output.
func (s *AsyncSession) outputTail(n int) string {
	lines, _ := s.TailLines(20)
	joined := strings.Join(lines, "\n")
	if len(joined) > n {
		joined = joined[len(joined)-n:]
	}
	return joined
}

// --- model-facing notification hook ---

// modelNotifier receives messages the MODEL should see (stall/size
// watchdog warnings). The root agent wires its steering channel here so
// the notification lands in the conversation at the next tool boundary.
var modelNotifier atomic.Pointer[func(string)]

// RegisterModelNotification registers the model-facing notification sink.
func RegisterModelNotification(fn func(message string)) {
	if fn == nil {
		return
	}
	modelNotifier.Store(&fn)
}

func (m *SessionManager) notifyModel(format string, args ...any) {
	if fn := modelNotifier.Load(); fn != nil && *fn != nil {
		(*fn)(fmt.Sprintf(format, args...))
	}
}

// --- persistent working directory ---

// managerCwd is the shell's current working directory, persisted across
// commands via the per-command pwd -P capture. Empty until the first
// command runs.
var managerCwd atomic.Pointer[string]

// originalCwd is the directory the process started in — the fallback when
// the tracked cwd is deleted underneath us.
var originalCwd atomic.Pointer[string]

// SetOriginalCwd records the process's starting directory (call once at
// startup).
func SetOriginalCwd(dir string) {
	if dir == "" {
		return
	}
	originalCwd.Store(&dir)
}

// CurrentCwd returns the shell's tracked working directory ("" before the
// first command).
func CurrentCwd() string {
	if p := managerCwd.Load(); p != nil {
		return *p
	}
	return ""
}

// updateCwd stores a newly captured working directory.
func updateCwd(dir string) {
	if dir == "" {
		return
	}
	managerCwd.Store(&dir)
}

// resolveCwd picks the directory a command runs in: an explicit cwd wins;
// otherwise the tracked shell cwd, falling back to the original directory
// when the tracked one no longer exists (a command deleted its own CWD).
func resolveCwd(explicit string) string {
	if explicit != "" {
		if _, err := os.Stat(explicit); err == nil {
			return explicit
		}
		if fallback := fallbackCwd(); fallback != "" {
			return fallback
		}
		return explicit
	}
	if tracked := CurrentCwd(); tracked != "" {
		if _, err := os.Stat(tracked); err == nil {
			return tracked
		}
		// Recovery: the directory vanished — fall back and clear the
		// tracked value so every future command doesn't repeat the stat.
		managerCwd.Store(nil)
	}
	return fallbackCwd()
}

func fallbackCwd() string {
	if p := originalCwd.Load(); p != nil {
		if _, err := os.Stat(*p); err == nil {
			return *p
		}
	}
	if wd, err := os.Getwd(); err == nil {
		return wd
	}
	return ""
}

// wrapWithCwdCapture appends a working-directory capture to a command:
//
//	<command>; __ec=$?; pwd -P >| <file>; exit $__ec
//
// The exit code is preserved explicitly (pwd would otherwise mask it), and
// the next command resolves its cwd from the captured file — a persistent
// shell without a persistent process.
func wrapWithCwdCapture(command string) (string, string) {
	f, err := os.CreateTemp("", "automergent-cwd-*")
	if err != nil {
		return command, ""
	}
	path := f.Name()
	f.Close()
	wrapped := fmt.Sprintf("%s; __ec=$?; pwd -P >| %s; exit $__ec", command, path)
	return wrapped, path
}

// readCapturedCwd reads the captured working directory and removes the
// temp file.
func readCapturedCwd(path string) string {
	if path == "" {
		return ""
	}
	defer os.Remove(path)
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// tailOutputFile returns the last maxLines lines of a session output file
// ("" when the file is missing or empty).
func tailOutputFile(path string, maxLines int) (string, error) {
	if path == "" || maxLines <= 0 {
		return "", nil
	}
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, 4*1024*1024))
	if err != nil {
		return "", err
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	return strings.Join(lines, "\n"), nil
}

// --- shell discovery ---

// shellBin resolves the shell to run commands with: an explicit override,
// then the user's login shell (when it is bash or zsh), then PATH lookup,
// then the bare "bash" name.
func shellBin() string {
	if s := os.Getenv("AUTOMERGENT_SHELL"); s != "" && isExecutablePath(s) {
		return s
	}
	if s := os.Getenv("SHELL"); s != "" {
		base := strings.ToLower(filepath.Base(s))
		if (strings.Contains(base, "bash") || strings.Contains(base, "zsh")) && isExecutablePath(s) {
			return s
		}
	}
	for _, candidate := range []string{"bash", "zsh", "sh"} {
		if path, err := exec.LookPath(candidate); err == nil {
			return path
		}
	}
	return "bash"
}

func isExecutablePath(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir() && info.Mode()&0o111 != 0
}
