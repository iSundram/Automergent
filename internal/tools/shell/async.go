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
	"time"

	"github.com/iSundram/Automergent/internal/tools"
)

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

	// Protect concurrent writes to Stdin
	stdinMu sync.Mutex

	// Track read positions to avoid returning duplicate output
	stdoutReadPos int
	stderrReadPos int

	// Cancel function for context-based cancellation
	cancel context.CancelFunc
}

// SessionManager manages async shell sessions.
type SessionManager struct {
	mu       sync.RWMutex
	sessions map[string]*AsyncSession
	counter  int
}

var globalManager = &SessionManager{
	sessions: make(map[string]*AsyncSession),
}

// GetManager returns the global session manager.
func GetManager() *SessionManager {
	return globalManager
}

func (m *SessionManager) Create(id string, session *AsyncSession) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[id] = session
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

func (m *SessionManager) NextID() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.counter++
	return fmt.Sprintf("shell-%d", m.counter)
}

// AsyncRunnerTool executes shell commands with async support.
type AsyncRunnerTool struct {
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
- mode="sync" (default): Run and wait for completion
- mode="async": Run in background, returns shell_id for read_shell/write_shell
- detach=true: Process survives session shutdown (for servers)
- Use initial_wait in sync mode to get early output before backgrounding`
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

	// If no initial_wait requested, keep original blocking behavior.
	if initialWait <= 0 {
		cmd := exec.CommandContext(ctx, "bash", "-c", command)
		if cwd != "" {
			cmd.Dir = cwd
		}
		cmd.Env = env

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		if stdinInput != "" {
			cmd.Stdin = bytes.NewBufferString(stdinInput)
		}

		err := cmd.Run()

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
	cmd := exec.CommandContext(ctx, "bash", "-c", command)
	if cwd != "" {
		cmd.Dir = cwd
	}
	cmd.Env = env

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
	}

	GetManager().Create(shellID, session)

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
				session.mu.Lock()
				session.Stdout.Write(buf[:n])
				session.mu.Unlock()
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
				session.mu.Lock()
				session.Stderr.Write(buf[:n])
				session.mu.Unlock()
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
		session.mu.Unlock()
		close(done)
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
			return tools.Result{IsError: true, Content: fmt.Sprintf("command failed: %v\n%s", exitErr, out)}, nil
		}
		return tools.Result{Content: out}, nil
	case <-time.After(time.Duration(initialWait) * time.Second):
		// still running - return session id for async reads/writes
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

	cmd := exec.CommandContext(ctx, "bash", "-c", command)
	if cwd != "" {
		cmd.Dir = cwd
	}
	cmd.Env = env

	stdin, err := cmd.StdinPipe()
	if err != nil {
		if cancel != nil {
			cancel()
		}
		return tools.Result{IsError: true, Content: fmt.Sprintf("failed to create stdin pipe: %v", err)}, nil
	}

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	session := &AsyncSession{
		ID:      shellID,
		Command: command,
		Cmd:     cmd,
		Stdin:   stdin,
		Stdout:  &stdout,
		Stderr:  &stderr,
		Started: time.Now(),
		cancel:  cancel,
	}

	if err := cmd.Start(); err != nil {
		if cancel != nil {
			cancel()
		}
		return tools.Result{IsError: true, Content: fmt.Sprintf("failed to start command: %v", err)}, nil
	}

	GetManager().Create(shellID, session)

	// Send initial stdin if provided
	if stdinInput != "" {
		if _, werr := stdin.Write([]byte(stdinInput)); werr != nil {
			stdin.Close()
			GetManager().Delete(shellID)
			if cancel != nil {
				cancel()
			}
			return tools.Result{IsError: true, Content: fmt.Sprintf("failed to write to stdin: %v", werr)}, nil
		}
		// close to signal EOF for initial input
		stdin.Close()
		session.mu.Lock()
		session.Stdin = nil
		session.mu.Unlock()
	}

	// copy stdout/stderr into buffers with mutex protection
	stdoutPipe, _ := cmd.StdoutPipe()
	stderrPipe, _ := cmd.StderrPipe()

	go func() {
		buf := make([]byte, 4096)
		for {
			n, rerr := stdoutPipe.Read(buf)
			if n > 0 {
				session.mu.Lock()
				session.Stdout.Write(buf[:n])
				session.mu.Unlock()
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
				session.mu.Lock()
				session.Stderr.Write(buf[:n])
				session.mu.Unlock()
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
		session.mu.Unlock()
	}()

	return tools.Result{
		Content: fmt.Sprintf("started async command (shell_id: %s)\nUse read_shell to get output, write_shell to send input", shellID),
		Metadata: map[string]any{
			"shell_id": shellID,
			"pid":      cmd.Process.Pid,
			"detached": detach,
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
			"shell_id":  shellID,
			"completed": session.Completed,
			"exit_code": session.ExitCode,
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
		session.mu.Unlock()
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

	GetManager().Delete(shellID)

	return tools.Result{Content: fmt.Sprintf("stopped shell session: %s", shellID)}, nil
}

// ListShellsTool lists all active shell sessions.
type ListShellsTool struct{}

func (t *ListShellsTool) Name() string                          { return "list_shells" }
func (t *ListShellsTool) Description() string                   { return "List all active shell sessions." }
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
		"type":       "object",
		"properties": map[string]any{},
	}
}

func (t *ListShellsTool) Execute(_ context.Context, _ map[string]any) (tools.Result, error) {
	sessions := GetManager().List()

	if len(sessions) == 0 {
		return tools.Result{Content: "no active shell sessions"}, nil
	}

	var lines []string
	for _, s := range sessions {
		s.mu.Lock()
		status := "running"
		if s.Completed {
			status = fmt.Sprintf("completed (exit %d)", s.ExitCode)
		}
		duration := time.Since(s.Started).Truncate(time.Second)
		lines = append(lines, fmt.Sprintf("- %s: %s [%s, %s]", s.ID, truncateCommand(s.Command), status, duration))
		s.mu.Unlock()
	}

	return tools.Result{Content: fmt.Sprintf("%d session(s):\n%s", len(sessions), joinLines(lines))}, nil
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
