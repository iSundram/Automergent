package shell

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/iSundram/Automergent/internal/diagnostics"
	"github.com/iSundram/Automergent/internal/tools"
)

// compilerRecoverySuffix inspects failed-command output for compiler errors
// and, when found, appends actionable recovery guidance. Empty when the
// output contains no recognized diagnostics.
func compilerRecoverySuffix(output string) string {
	report := diagnostics.RecoverCompilerOutput(output, "")
	if report.UserMessage == "" {
		return ""
	}
	return "\n\n" + report.Render()
}

// AsyncSession represents a running async shell session.
type AsyncSession struct {
	ID        string
	Command   string
	Cmd       *exec.Cmd
	Stdin     io.WriteCloser
	Stdout    *bytes.Buffer
	Stderr    *bytes.Buffer
	Started   time.Time
	Completed bool
	ExitCode  int
	Error     error
	mu        sync.Mutex

	// OutputPath is the session's durable output file (see output.go); the
	// RAM buffers hold only a bounded tail of it. outputFile is the append
	// handle, closed when the session completes.
	OutputPath string
	outputFile *os.File

	// done closes when the process is reaped; the watchdogs exit on it.
	done chan struct{}

	// lastGrowth is the Unix-nano timestamp of the last output arrival,
	// read by the stall watchdog without the lock.
	lastGrowth atomicInt64

	// Protect concurrent writes to Stdin
	stdinMu sync.Mutex

	// Track read positions to avoid returning duplicate output
	stdoutReadPos int
	stderrReadPos int

	// truncated records that the RAM buffers were trimmed; read_shell then
	// points at the output file for the full history.
	truncated bool

	// Cancel function for context-based cancellation
	cancel context.CancelFunc

	// Whether this session was exposed as a background operation.
	background bool

	// Cached view of the output for the dock's once-a-second sampling, updated
	// on write so readers never rescan the buffers. See live.go.
	lastLine  atomicString
	sawStderr atomic.Bool
}

// Lock acquires the session mutex.
func (s *AsyncSession) Lock() { s.mu.Lock() }

// Unlock releases the session mutex.
func (s *AsyncSession) Unlock() { s.mu.Unlock() }

// IsCompleted returns whether the session has finished.
func (s *AsyncSession) IsCompleted() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.Completed
}

// Cancel cancels the session context.
func (s *AsyncSession) Cancel() {
	if s.cancel != nil {
		s.cancel()
	}
}

// SessionManager manages async shell sessions.
type SessionManager struct {
	mu       sync.RWMutex
	sessions map[string]*AsyncSession
	history  map[string]SessionRecord
	hooks    []func(SessionNotification)
	counter  int
}

// SessionStatus is the lifecycle status for a background shell operation.
type SessionStatus string

const (
	SessionStatusRunning   SessionStatus = "running"
	SessionStatusCompleted SessionStatus = "completed"
	SessionStatusFailed    SessionStatus = "failed"
	SessionStatusCancelled SessionStatus = "cancelled"
)

// SessionRecord is a durable history entry for a shell operation.
type SessionRecord struct {
	ID          string
	Command     string
	Status      SessionStatus
	StartedAt   time.Time
	CompletedAt time.Time
	ExitCode    int
	Detached    bool
	ErrMessage  string
	// OutputPath is the session's durable output file; the record outlives
	// the session, so post-completion reads go through it.
	OutputPath string
}

// SessionNotification captures completion/failure updates.
type SessionNotification struct {
	ID         string
	Command    string
	Status     SessionStatus
	ExitCode   int
	ErrMessage string
	Duration   time.Duration
}

var globalManager = &SessionManager{
	sessions: make(map[string]*AsyncSession),
	history:  make(map[string]SessionRecord),
}

// defaultAutoBackgroundSeconds is the threshold for auto-backgrounding sync commands.
const defaultAutoBackgroundSeconds = 8

// commandsExcludedFromBackground are commands that should never be auto-backgrounded.
// These are typically short-lived or interactive commands.
var commandsExcludedFromBackground = []string{
	"sleep",
	"wait",
	"read",
	"echo",
	"printf",
	"test",
	"[",
	"[[",
	"pwd",
	"hostname",
	"date",
	"whoami",
	"id",
	"uname",
	"which",
	"whereis",
	"file",
	"stat",
	"ls",
	"dir",
	"cat",
	"head",
	"tail",
	"less",
	"more",
	"grep",
	"awk",
	"sed",
	"cut",
	"sort",
	"uniq",
	"wc",
	"tr",
	"tee",
	"xargs",
	"env",
	"printenv",
	"export",
	"unset",
	"cd",
	"pushd",
	"popd",
	"dirs",
	"true",
	"false",
	"yes",
	"seq",
	"shuf",
	"rev",
	"base64",
	"md5sum",
	"sha1sum",
	"sha256sum",
}

// shouldExcludeFromBackground checks if a command should not be auto-backgrounded.
func shouldExcludeFromBackground(command string) bool {
	cmdLower := strings.TrimSpace(strings.ToLower(command))
	
	// Get the first word (the command name)
	parts := strings.Fields(cmdLower)
	if len(parts) == 0 {
		return false
	}
	baseCmd := parts[0]
	
	// Check if it's in the exclusion list
	for _, excluded := range commandsExcludedFromBackground {
		if baseCmd == excluded {
			return true
		}
	}
	return false
}

// GetManager returns the global session manager.
func GetManager() *SessionManager {
	return globalManager
}

func (m *SessionManager) Create(id string, session *AsyncSession) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[id] = session
	m.history[id] = SessionRecord{
		ID:        id,
		Command:   session.Command,
		Status:    SessionStatusRunning,
		StartedAt: session.Started,
	}
}

func (m *SessionManager) Get(id string) (*AsyncSession, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.sessions[id]
	return s, ok
}

func (m *SessionManager) Delete(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, id)
}

// MarkBackground marks a session as a user-visible background operation.
func (m *SessionManager) MarkBackground(id string, background bool, detached bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.sessions[id]; ok {
		s.background = background
	}
	rec, ok := m.history[id]
	if !ok {
		return
	}
	rec.Detached = detached
	m.history[id] = rec
}

func isTerminalSessionStatus(status SessionStatus) bool {
	return status == SessionStatusCompleted || status == SessionStatusFailed || status == SessionStatusCancelled
}

// UpdateStatus updates the stored status and emits notification hooks for background terminal transitions.
func (m *SessionManager) UpdateStatus(id string, status SessionStatus, exitCode int, err error) bool {
	m.mu.Lock()
	rec, ok := m.history[id]
	if !ok {
		m.mu.Unlock()
		return false
	}
	previousStatus := rec.Status
	rec.Status = status
	rec.ExitCode = exitCode
	if session := m.sessions[id]; session != nil {
		// OutputPath is immutable once attached; the session lock keeps the
		// race detector satisfied without nesting risk (s.mu never acquires
		// m.mu).
		session.mu.Lock()
		if session.OutputPath != "" {
			rec.OutputPath = session.OutputPath
		}
		session.mu.Unlock()
	}
	if isTerminalSessionStatus(status) {
		rec.CompletedAt = time.Now()
	}
	if err != nil {
		rec.ErrMessage = err.Error()
	}
	m.history[id] = rec

	session := m.sessions[id]
	background := session != nil && session.background
	command := rec.Command
	hooks := append([]func(SessionNotification){}, m.hooks...)
	m.mu.Unlock()

	if !background || !isTerminalSessionStatus(status) {
		return true
	}
	if previousStatus == status && isTerminalSessionStatus(previousStatus) {
		return true
	}

	duration := rec.CompletedAt.Sub(rec.StartedAt)
	n := SessionNotification{
		ID:         id,
		Command:    command,
		Status:     status,
		ExitCode:   exitCode,
		ErrMessage: rec.ErrMessage,
		Duration:   duration,
	}
	for _, hook := range hooks {
		hook(n)
	}
	return true
}

// RegisterStatusHook registers completion/failure notification hooks.
func (m *SessionManager) RegisterStatusHook(hook func(SessionNotification)) {
	if hook == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.hooks = append(m.hooks, hook)
}

// Cleanup removes completed sessions older than maxAge to prevent memory leaks.
func (m *SessionManager) Cleanup(maxAge time.Duration) int {
	cutoff := time.Now().Add(-maxAge)
	// Copy references under a read lock to avoid holding manager lock while locking sessions
	m.mu.RLock()
	sessionsCopy := make(map[string]*AsyncSession, len(m.sessions))
	for id, s := range m.sessions {
		sessionsCopy[id] = s
	}
	m.mu.RUnlock()

	removed := 0
	for id, s := range sessionsCopy {
		s.mu.Lock()
		completed := s.Completed
		started := s.Started
		s.mu.Unlock()
		if completed && started.Before(cutoff) {
			m.mu.Lock()
			// Double-check it hasn't been replaced
			if cur, ok := m.sessions[id]; ok && cur == s {
				delete(m.sessions, id)
				removed++
			}
			m.mu.Unlock()
		}
	}
	m.mu.Lock()
	for id, rec := range m.history {
		if isTerminalSessionStatus(rec.Status) && !rec.CompletedAt.IsZero() && rec.CompletedAt.Before(cutoff) {
			delete(m.history, id)
		}
	}
	m.mu.Unlock()
	return removed
}

func (m *SessionManager) List() []*AsyncSession {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]*AsyncSession, 0, len(m.sessions))
	for _, s := range m.sessions {
		result = append(result, s)
	}
	return result
}

// GetRecord returns a history entry for a session.
func (m *SessionManager) GetRecord(id string) (SessionRecord, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	rec, ok := m.history[id]
	return rec, ok
}

// ListRecords returns session history, optionally including completed entries.
func (m *SessionManager) ListRecords(includeCompleted bool) []SessionRecord {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]SessionRecord, 0, len(m.history))
	for _, rec := range m.history {
		if !includeCompleted && isTerminalSessionStatus(rec.Status) {
			continue
		}
		out = append(out, rec)
	}
	return out
}

func (m *SessionManager) NextID() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.counter++
	return fmt.Sprintf("shell-%d", m.counter)
}

// AsyncRunnerTool executes shell commands with async support.
type AsyncRunnerTool struct {
	tools.BaseTool
	timeout          time.Duration
	stripEnvPatterns []string
}

func NewAsyncRunnerTool(timeout time.Duration) *AsyncRunnerTool {
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	return &AsyncRunnerTool{
		timeout:          timeout,
		stripEnvPatterns: defaultSensitivePatterns,
	}
}

func (t *AsyncRunnerTool) Name() string { return "bash" }
func (t *AsyncRunnerTool) Description() string {
	return `Execute shell commands in sync or async mode.
- mode="sync" (default): Run and wait for completion, auto-backgrounds after 8s for long commands
- mode="async": Run in background, returns shell_id for read_shell/write_shell
- detach=true: Process survives session shutdown (for servers)
- Use initial_wait in sync mode to get early output before backgrounding
- Commands like sleep, wait, echo are never auto-backgrounded`
}

// Meta documents bash in the system prompt.
func (t *AsyncRunnerTool) Meta() *tools.ToolMeta {
	return &tools.ToolMeta{
		Category:    "shell",
		DisplayName: "Run command",
		InjectOrder: 10,
		WhenToUse:   "Terminal operations: git, build tools, package managers, test runners, docker. Also the verification step after edits — run the narrowest relevant build/test command and fix what fails.",
		WhenNotTo:   "Never for file reads/writes/edits/searches — the dedicated tools are better (read_file over cat; rg-style grep over hand-rolled find|grep pipelines).",
		Usage: "Prefer sync mode with a tight timeout; escalate to mode=\"async\" only for long-running processes.\n" +
			"Large outputs are preserved to a file rather than truncated — do not pre-truncate with head/tail.\n" +
			"Compound commands run sandboxed per policy; split chains so only steps that truly need escalation get it.",
		UsageByFamily: map[string]string{
			"gemini3": "Gemini 3: keep one logical action per call — emit separate calls for configure/build/test instead of a single chained line, so failures localize.",
		},
		Examples: [][2]string{
			{"bash {\"command\": \"go test ./internal/prompt/\"} to verify a prompt-package change", "bash {\"command\": \"cat internal/prompt/types.go\"} — use read_file instead"},
		},
	}
}
func (t *AsyncRunnerTool) RequiresConfirmation(mode string) bool {
	return mode == "plan" || mode == "edit"
}

func (t *AsyncRunnerTool) EstimatedCost() tools.ToolCost {
	return tools.ToolCost{
		TokensApprox: 100,
		LatencyMs:    300,
		RiskLevel:    "medium",
	}
}

func (t *AsyncRunnerTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command": map[string]any{
				"type":        "string",
				"description": "Shell command to execute.",
			},
			"mode": map[string]any{
				"type":        "string",
				"enum":        []string{"sync", "async"},
				"description": "Execution mode: 'sync' waits for completion, 'async' runs in background.",
			},
			"cwd": map[string]any{
				"type":        "string",
				"description": "Working directory for the command.",
			},
			"timeout": map[string]any{
				"type":        "string",
				"description": "Timeout for sync mode (e.g., '30s', '5m').",
			},
			"initial_wait": map[string]any{
				"type":        "integer",
				"description": "Seconds to wait for initial output in sync mode before backgrounding.",
			},
			"env": map[string]any{
				"type":        "object",
				"description": "Additional environment variables to set.",
			},
			"stdin": map[string]any{
				"type":        "string",
				"description": "Input to send to stdin.",
			},
			"detach": map[string]any{
				"type":        "boolean",
				"description": "If true, process survives session shutdown (for servers/daemons).",
			},
			"shell_id": map[string]any{
				"type":        "string",
				"description": "Custom shell ID (auto-generated if not provided).",
			},
		},
		"required": []string{"command"},
	}
}

func (t *AsyncRunnerTool) Execute(ctx context.Context, args map[string]any) (tools.Result, error) {
	command, ok := tools.StringArg(args, "command")
	if !ok || command == "" {
		return tools.Result{IsError: true, Content: "command is required"}, nil
	}

	mode := "sync"
	if m, ok := tools.StringArg(args, "mode"); ok {
		mode = m
	}

	cwd, _ := tools.StringArg(args, "cwd")
	timeoutStr, _ := tools.StringArg(args, "timeout")
	stdinInput, _ := tools.StringArg(args, "stdin")
	shellID, _ := tools.StringArg(args, "shell_id")

	initialWait := 0
	if n, ok := tools.ArgInt(args, "initial_wait"); ok {
		initialWait = n
	}

	detach := false
	if v, ok := tools.ArgBool(args, "detach"); ok {
		detach = v
	}

	// Build environment
	env := filterEnv(os.Environ(), t.stripEnvPatterns)
	if extraEnv, ok := args["env"].(map[string]any); ok {
		for k, v := range extraEnv {
			if vs, ok := v.(string); ok {
				env = append(env, fmt.Sprintf("%s=%s", k, vs))
			}
		}
	}

	if mode == "async" {
		return t.executeAsync(command, cwd, env, shellID, stdinInput, detach)
	}

	return t.executeSync(ctx, command, cwd, env, timeoutStr, stdinInput, initialWait)
}

func (t *AsyncRunnerTool) executeSync(ctx context.Context, command, cwd string, env []string, timeoutStr, stdinInput string, initialWait int) (tools.Result, error) {
	timeout := t.timeout
	if timeoutStr != "" {
		if d, err := time.ParseDuration(timeoutStr); err == nil {
			timeout = d
		}
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Auto-background after 8 seconds for eligible commands
	if initialWait <= 0 && !shouldExcludeFromBackground(command) {
		initialWait = defaultAutoBackgroundSeconds
	}

	// If no initial_wait requested, keep original blocking behavior.
	if initialWait <= 0 {
		wrapped, cwdFile := wrapWithCwdCapture(command)
		cmd := exec.CommandContext(ctx, shellBin(), "-c", wrapped)
		if dir := resolveCwd(cwd); dir != "" {
			cmd.Dir = dir
		}
		cmd.Env = env
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		if stdinInput != "" {
			cmd.Stdin = bytes.NewBufferString(stdinInput)
		}

		err := cmd.Run()
		if captured := readCapturedCwd(cwdFile); captured != "" {
			updateCwd(captured)
		}

		output := stdout.String()
		if stderr.Len() > 0 {
			output += "\n[stderr]\n" + stderr.String()
		}

		if err != nil {
			if ctx.Err() == context.DeadlineExceeded {
				return tools.Result{
					IsError: true,
					Content: fmt.Sprintf("command timed out after %s\n%s", timeout, output),
				}, nil
			}
			return tools.Result{
				IsError: true,
				Content: fmt.Sprintf("command failed: %v\n%s", err, output),
			}, nil
		}

		return tools.Result{Content: output}, nil
	}

	// initialWait > 0: start the process, collect initial output, then background if still running
	wrapped, cwdFile := wrapWithCwdCapture(command)
	cmd := exec.CommandContext(ctx, shellBin(), "-c", wrapped)
	if dir := resolveCwd(cwd); dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = env
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		return tools.Result{IsError: true, Content: fmt.Sprintf("failed to create stdin pipe: %v", err)}, nil
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		stdinPipe.Close()
		return tools.Result{IsError: true, Content: fmt.Sprintf("failed to create stdout pipe: %v", err)}, nil
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		stdinPipe.Close()
		return tools.Result{IsError: true, Content: fmt.Sprintf("failed to create stderr pipe: %v", err)}, nil
	}

	if err := cmd.Start(); err != nil {
		stdinPipe.Close()
		return tools.Result{IsError: true, Content: fmt.Sprintf("failed to start command: %v", err)}, nil
	}

	// create session and register
	shellID := GetManager().NextID()
	var stdoutBuf, stderrBuf bytes.Buffer
	session := &AsyncSession{
		ID:      shellID,
		Command: command,
		Cmd:     cmd,
		Stdin:   stdinPipe,
		Stdout:  &stdoutBuf,
		Stderr:  &stderrBuf,
		Started: time.Now(),
		cancel:  cancel,
		done:    make(chan struct{}),
	}
	session.attachOutputFile()
	session.noteGrowth()

	GetManager().Create(shellID, session)
	GetManager().startWatchdogs(session)

	// send initial stdin and close to avoid hangs
	if stdinInput != "" {
		if _, werr := stdinPipe.Write([]byte(stdinInput)); werr != nil {
			stdinPipe.Close()
			GetManager().Delete(shellID)
			if cancel != nil {
				cancel()
			}
			return tools.Result{IsError: true, Content: fmt.Sprintf("failed to write to stdin: %v", werr)}, nil
		}
		// close to indicate EOF
		stdinPipe.Close()
		session.mu.Lock()
		session.Stdin = nil
		session.mu.Unlock()
	}

	// copy stdout/stderr into buffers with mutex protection
	go func() {
		buf := make([]byte, 4096)
		for {
			n, rerr := stdoutPipe.Read(buf)
			if n > 0 {
				session.noteStdout(buf[:n])
			}
			if rerr != nil {
				break
			}
		}
	}()

	go func() {
		buf := make([]byte, 4096)
		for {
			n, rerr := stderrPipe.Read(buf)
			if n > 0 {
				session.noteStderr(buf[:n])
			}
			if rerr != nil {
				break
			}
		}
	}()

	// monitor completion
	done := make(chan struct{})
	go func() {
		err := cmd.Wait()
		session.mu.Lock()
		session.Completed = true
		session.Error = err
		if cmd.ProcessState != nil {
			session.ExitCode = cmd.ProcessState.ExitCode()
		}
		exitCode := session.ExitCode
		if session.outputFile != nil {
			_ = session.outputFile.Close()
			session.outputFile = nil
		}
		session.mu.Unlock()
		if captured := readCapturedCwd(cwdFile); captured != "" {
			updateCwd(captured)
		}
		status := SessionStatusCompleted
		if err != nil {
			status = SessionStatusFailed
		}
		_ = GetManager().UpdateStatus(shellID, status, exitCode, err)
		close(done)
		close(session.done)
	}()

	select {
	case <-done:
		// finished within initial wait
		session.mu.Lock()
		out := session.Stdout.String()
		if session.Stderr.Len() > 0 {
			out += "\n[stderr]\n" + session.Stderr.String()
		}
		exitErr := session.Error
		session.mu.Unlock()

		GetManager().Delete(shellID)

		if exitErr != nil {
			if ctx.Err() == context.DeadlineExceeded {
				return tools.Result{IsError: true, Content: fmt.Sprintf("command timed out after %s\n%s", timeout, out)}, nil
			}
			return tools.Result{IsError: true, Content: fmt.Sprintf("command failed: %v\n%s%s", exitErr, out, compilerRecoverySuffix(out))}, nil
		}
		return tools.Result{Content: out}, nil
	case <-time.After(time.Duration(initialWait) * time.Second):
		// still running - return session id for async reads/writes
		GetManager().MarkBackground(shellID, true, false)
		return tools.Result{
			Content: fmt.Sprintf("started async command (shell_id: %s)\nUse read_shell to get output, write_shell to send input", shellID),
			Metadata: map[string]any{
				"shell_id": shellID,
				"pid":      cmd.Process.Pid,
				"detached": false,
			},
		}, nil
	}
}

func (t *AsyncRunnerTool) executeAsync(command, cwd string, env []string, shellID, stdinInput string, detach bool) (tools.Result, error) {
	if shellID == "" {
		shellID = GetManager().NextID()
	}

	// Create a cancellable context for non-detached processes
	var ctx context.Context
	var cancel context.CancelFunc
	if detach {
		ctx = context.Background()
		cancel = nil
	} else {
		ctx, cancel = context.WithCancel(context.Background())
	}

	// Persistent working directory: wrap the command with a pwd -P capture
	// so the next command resumes where this one left off, and resolve the
	// directory this command runs in (explicit cwd > tracked shell cwd >
	// original dir, with recovery when the tracked dir vanished).
	wrapped, cwdFile := wrapWithCwdCapture(command)
	runDir := resolveCwd(cwd)

	cmd := exec.CommandContext(ctx, shellBin(), "-c", wrapped)
	if runDir != "" {
		cmd.Dir = runDir
	}
	cmd.Env = env
	// Own process group: Kill() signals the negative PID and takes the
	// whole tree (pipelines, `&` children), not just the shell.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		if cancel != nil {
			cancel()
		}
		return tools.Result{IsError: true, Content: fmt.Sprintf("failed to create stdin pipe: %v", err)}, nil
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		stdinPipe.Close()
		return tools.Result{IsError: true, Content: fmt.Sprintf("failed to create stdout pipe: %v", err)}, nil
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		stdinPipe.Close()
		return tools.Result{IsError: true, Content: fmt.Sprintf("failed to create stderr pipe: %v", err)}, nil
	}

	if err := cmd.Start(); err != nil {
		stdinPipe.Close()
		if cancel != nil {
			cancel()
		}
		return tools.Result{IsError: true, Content: fmt.Sprintf("failed to start command: %v", err)}, nil
	}

	// create session and register
	var stdoutBuf, stderrBuf bytes.Buffer
	session := &AsyncSession{
		ID:      shellID,
		Command: command,
		Cmd:     cmd,
		Stdin:   stdinPipe,
		Stdout:  &stdoutBuf,
		Stderr:  &stderrBuf,
		Started: time.Now(),
		cancel:  cancel,
		done:    make(chan struct{}),
	}
	session.attachOutputFile()
	session.noteGrowth()

	GetManager().Create(shellID, session)
	GetManager().MarkBackground(shellID, true, detach)
	GetManager().startWatchdogs(session)

	// Send initial stdin if provided
	if stdinInput != "" {
		if _, werr := stdinPipe.Write([]byte(stdinInput)); werr != nil {
			stdinPipe.Close()
			GetManager().Delete(shellID)
			if cancel != nil {
				cancel()
			}
			return tools.Result{IsError: true, Content: fmt.Sprintf("failed to write to stdin: %v", werr)}, nil
		}
		// close to signal EOF for initial input
		stdinPipe.Close()
		session.mu.Lock()
		session.Stdin = nil
		session.mu.Unlock()
	}

	// copy stdout/stderr into buffers with mutex protection
	go func() {
		buf := make([]byte, 4096)
		for {
			n, rerr := stdoutPipe.Read(buf)
			if n > 0 {
				session.noteStdout(buf[:n])
			}
			if rerr != nil {
				break
			}
		}
	}()

	go func() {
		buf := make([]byte, 4096)
		for {
			n, rerr := stderrPipe.Read(buf)
			if n > 0 {
				session.noteStderr(buf[:n])
			}
			if rerr != nil {
				break
			}
		}
	}()

	// Monitor completion in background
	go func() {
		err := cmd.Wait()
		session.mu.Lock()
		session.Completed = true
		session.Error = err
		if cmd.ProcessState != nil {
			session.ExitCode = cmd.ProcessState.ExitCode()
		}
		exitCode := session.ExitCode
		outputPath := session.OutputPath
		if session.outputFile != nil {
			_ = session.outputFile.Close()
			session.outputFile = nil
		}
		session.mu.Unlock()
		// Adopt the shell's new working directory for subsequent commands.
		if captured := readCapturedCwd(cwdFile); captured != "" {
			updateCwd(captured)
		}
		status := SessionStatusCompleted
		if err != nil {
			status = SessionStatusFailed
		}
		_ = GetManager().UpdateStatus(shellID, status, exitCode, err)
		close(session.done)
		_ = outputPath
	}()

	return tools.Result{
		Content: fmt.Sprintf("started async command (shell_id: %s)\nUse read_shell to get output, write_shell to send input", shellID),
		Metadata: map[string]any{
			"shell_id":    shellID,
			"pid":         cmd.Process.Pid,
			"detached":    detach,
			"output_file": session.OutputPath,
		},
	}, nil
}

// ReadShellTool reads output from an async shell session.
type ReadShellTool struct{}

func (t *ReadShellTool) Name() string { return "read_shell" }
func (t *ReadShellTool) Description() string {
	return `Read output from an async shell session.
- Use shell_id from bash async mode
- Returns stdout and stderr since last read
- Shows completion status and exit code if finished`
}
func (t *ReadShellTool) RequiresConfirmation(mode string) bool { return false }

func (t *ReadShellTool) EstimatedCost() tools.ToolCost {
	return tools.ToolCost{
		TokensApprox: 100,
		LatencyMs:    50,
		RiskLevel:    "low",
	}
}

func (t *ReadShellTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"shell_id": map[string]any{
				"type":        "string",
				"description": "Shell session ID from async bash command.",
			},
			"delay": map[string]any{
				"type":        "integer",
				"description": "Seconds to wait before reading (default: 0).",
			},
		},
		"required": []string{"shell_id"},
	}
}

func (t *ReadShellTool) Execute(_ context.Context, args map[string]any) (tools.Result, error) {
	shellID, ok := tools.StringArg(args, "shell_id")
	if !ok || shellID == "" {
		return tools.Result{IsError: true, Content: "shell_id is required"}, nil
	}

	if delay, ok := tools.ArgInt(args, "delay"); ok && delay > 0 {
		time.Sleep(time.Duration(delay) * time.Second)
	}

	session, ok := GetManager().Get(shellID)
	if !ok {
		if rec, found := GetManager().GetRecord(shellID); found {
			status := string(rec.Status)
			if rec.Status == SessionStatusCompleted && rec.ExitCode == 0 {
				status = "completed successfully"
			}
			// The session is gone but its durable output file may remain —
			// the tail is the useful part after completion.
			content := fmt.Sprintf("(session output unavailable)\n\n[%s]", status)
			if tail, terr := tailOutputFile(rec.OutputPath, 200); terr == nil && tail != "" {
				content = tail + "\n\n[" + status + "]"
			}
			return tools.Result{
				Content: content,
				Metadata: map[string]any{
					"shell_id":    shellID,
					"completed":  isTerminalSessionStatus(rec.Status),
					"exit_code":  rec.ExitCode,
					"output_file": rec.OutputPath,
				},
			}, nil
		}
		return tools.Result{IsError: true, Content: fmt.Sprintf("shell session not found: %s", shellID)}, nil
	}

	session.mu.Lock()
	defer session.mu.Unlock()

	// Get only new output since last read
	stdoutData := session.Stdout.Bytes()
	stderrData := session.Stderr.Bytes()

	var output string
	if session.stdoutReadPos < len(stdoutData) {
		output = string(stdoutData[session.stdoutReadPos:])
		session.stdoutReadPos = len(stdoutData)
	}

	if session.stderrReadPos < len(stderrData) {
		newStderr := string(stderrData[session.stderrReadPos:])
		if newStderr != "" {
			if output != "" {
				output += "\n"
			}
			output += "[stderr]\n" + newStderr
		}
		session.stderrReadPos = len(stderrData)
	}

	if output == "" {
		output = "(no new output)"
	}

	// When the RAM view was trimmed, say so and point at the durable file.
	if session.truncated && session.OutputPath != "" {
		output += fmt.Sprintf("\n\n[older output was truncated in the live view; full output: %s]", session.OutputPath)
	}

	if session.Completed {
		status := "completed successfully"
		if session.ExitCode != 0 {
			status = fmt.Sprintf("failed with exit code %d", session.ExitCode)
		}
		output += fmt.Sprintf("\n\n[%s]", status)
	} else {
		output += "\n\n[still running...]"
	}

	return tools.Result{
		Content: output,
		Metadata: map[string]any{
			"shell_id":    shellID,
			"completed":  session.Completed,
			"exit_code":  session.ExitCode,
			"output_file": session.OutputPath,
		},
	}, nil
}

// WriteShellTool sends input to an async shell session.
type WriteShellTool struct{}

func (t *WriteShellTool) Name() string { return "write_shell" }
func (t *WriteShellTool) Description() string {
	return `Send input to a running async shell session.
- Use shell_id from bash async mode
- Can send text and special keys: {enter}, {up}, {down}, {left}, {right}, {backspace}`
}
func (t *WriteShellTool) RequiresConfirmation(mode string) bool { return false }

func (t *WriteShellTool) EstimatedCost() tools.ToolCost {
	return tools.ToolCost{
		TokensApprox: 80,
		LatencyMs:    100,
		RiskLevel:    "low",
	}
}

func (t *WriteShellTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"shell_id": map[string]any{
				"type":        "string",
				"description": "Shell session ID.",
			},
			"input": map[string]any{
				"type":        "string",
				"description": "Input to send. Use {enter}, {up}, {down}, etc. for special keys.",
			},
			"delay": map[string]any{
				"type":        "integer",
				"description": "Seconds to wait after sending input before reading response.",
			},
		},
		"required": []string{"shell_id", "input"},
	}
}

func (t *WriteShellTool) Execute(_ context.Context, args map[string]any) (tools.Result, error) {
	shellID, ok := tools.StringArg(args, "shell_id")
	if !ok || shellID == "" {
		return tools.Result{IsError: true, Content: "shell_id is required"}, nil
	}

	input, ok := tools.StringArg(args, "input")
	if !ok {
		return tools.Result{IsError: true, Content: "input is required"}, nil
	}

	session, ok := GetManager().Get(shellID)
	if !ok {
		return tools.Result{IsError: true, Content: fmt.Sprintf("shell session not found: %s", shellID)}, nil
	}

	session.mu.Lock()
	if session.Completed {
		session.mu.Unlock()
		return tools.Result{IsError: true, Content: "shell session has already completed"}, nil
	}
	session.mu.Unlock()

	// Process special keys
	input = processSpecialKeys(input)

	// Ensure stdin is available
	if session.Stdin == nil {
		return tools.Result{IsError: true, Content: "stdin is closed for this session"}, nil
	}

	// Protect concurrent writes to Stdin
	session.stdinMu.Lock()
	_, werr := session.Stdin.Write([]byte(input))
	session.stdinMu.Unlock()
	if werr != nil {
		return tools.Result{IsError: true, Content: fmt.Sprintf("failed to write to stdin: %v", werr)}, nil
	}

	// Wait for response if delay specified
	if delay, ok := tools.ArgInt(args, "delay"); ok && delay > 0 {
		time.Sleep(time.Duration(delay) * time.Second)
	}

	session.mu.Lock()
	output := session.Stdout.String()
	session.mu.Unlock()

	return tools.Result{
		Content: fmt.Sprintf("sent input to shell %s\n\n%s", shellID, output),
		Metadata: map[string]any{
			"shell_id": shellID,
		},
	}, nil
}

func processSpecialKeys(input string) string {
	replacements := map[string]string{
		"{enter}":     "\n",
		"{up}":        "\x1b[A",
		"{down}":      "\x1b[B",
		"{left}":      "\x1b[D",
		"{right}":     "\x1b[C",
		"{backspace}": "\x7f",
		"{tab}":       "\t",
		"{escape}":    "\x1b",
	}
	for key, val := range replacements {
		input = replaceAll(input, key, val)
	}
	return input
}

func replaceAll(s, old, new string) string {
	return strings.ReplaceAll(s, old, new)
}

// StopShellTool terminates an async shell session.
type StopShellTool struct{}

func (t *StopShellTool) Name() string { return "stop_shell" }
func (t *StopShellTool) Description() string {
	return "Terminate a running async shell session."
}
func (t *StopShellTool) RequiresConfirmation(mode string) bool { return false }

func (t *StopShellTool) EstimatedCost() tools.ToolCost {
	return tools.ToolCost{
		TokensApprox: 50,
		LatencyMs:    100,
		RiskLevel:    "low",
	}
}

func (t *StopShellTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"shell_id": map[string]any{
				"type":        "string",
				"description": "Shell session ID to stop.",
			},
		},
		"required": []string{"shell_id"},
	}
}

func (t *StopShellTool) Execute(_ context.Context, args map[string]any) (tools.Result, error) {
	shellID, ok := tools.StringArg(args, "shell_id")
	if !ok || shellID == "" {
		return tools.Result{IsError: true, Content: "shell_id is required"}, nil
	}

	session, ok := GetManager().Get(shellID)
	if !ok {
		return tools.Result{IsError: true, Content: fmt.Sprintf("shell session not found: %s", shellID)}, nil
	}

	session.mu.Lock()
	if session.Completed {
		exitCode := session.ExitCode
		exitErr := session.Error
		session.mu.Unlock()
		status := SessionStatusCompleted
		if exitCode != 0 {
			status = SessionStatusFailed
		}
		_ = GetManager().UpdateStatus(shellID, status, exitCode, exitErr)
		GetManager().Delete(shellID)
		return tools.Result{Content: fmt.Sprintf("shell %s was already completed, session cleaned up", shellID)}, nil
	}

	// Cancel the context first (graceful shutdown)
	if session.cancel != nil {
		session.cancel()
	}
	session.mu.Unlock()

	// If process is still running after context cancel, force kill
	if session.Cmd.Process != nil {
		session.Cmd.Process.Kill()
	}
	_ = GetManager().UpdateStatus(shellID, SessionStatusCancelled, -1, nil)

	GetManager().Delete(shellID)

	return tools.Result{Content: fmt.Sprintf("stopped shell session: %s", shellID)}, nil
}

// ListShellsTool lists all active shell sessions.
type ListShellsTool struct{}

func (t *ListShellsTool) Name() string                          { return "list_shells" }
func (t *ListShellsTool) Description() string                   { return "List shell sessions and background history." }
func (t *ListShellsTool) RequiresConfirmation(mode string) bool { return false }

func (t *ListShellsTool) EstimatedCost() tools.ToolCost {
	return tools.ToolCost{
		TokensApprox: 50,
		LatencyMs:    50,
		RiskLevel:    "low",
	}
}

func (t *ListShellsTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"include_completed": map[string]any{
				"type":        "boolean",
				"description": "Include completed/failed/cancelled sessions (default: true).",
			},
		},
	}
}

func (t *ListShellsTool) Execute(_ context.Context, args map[string]any) (tools.Result, error) {
	includeCompleted := true
	if v, ok := tools.ArgBool(args, "include_completed"); ok {
		includeCompleted = v
	}
	records := GetManager().ListRecords(includeCompleted)
	if len(records) == 0 {
		return tools.Result{Content: "no active shell sessions"}, nil
	}

	var lines []string
	for _, rec := range records {
		duration := time.Since(rec.StartedAt).Truncate(time.Second)
		if !rec.CompletedAt.IsZero() {
			duration = rec.CompletedAt.Sub(rec.StartedAt).Truncate(time.Second)
		}
		status := string(rec.Status)
		if rec.Status == SessionStatusCompleted {
			status = fmt.Sprintf("completed (exit %d)", rec.ExitCode)
		}
		if rec.Status == SessionStatusFailed {
			status = fmt.Sprintf("failed (exit %d)", rec.ExitCode)
		}
		lines = append(lines, fmt.Sprintf("- %s: %s [%s, %s]", rec.ID, truncateCommand(rec.Command), status, duration))
	}

	return tools.Result{Content: fmt.Sprintf("%d session(s):\n%s", len(records), joinLines(lines))}, nil
}

func truncateCommand(cmd string) string {
	if len(cmd) > 50 {
		return cmd[:47] + "..."
	}
	return cmd
}

func joinLines(lines []string) string {
	var b strings.Builder
	for _, l := range lines {
		b.WriteString(l)
		b.WriteByte('\n')
	}
	return b.String()
}
