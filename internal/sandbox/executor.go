package sandbox

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

// Executor runs commands in a sandboxed environment.
type Executor struct {
	sandbox Sandbox
	policy  *Policy
	mu      sync.Mutex
}

// ExecutionResult contains the result of a sandboxed execution.
type ExecutionResult struct {
	// ExitCode is the process exit code.
	ExitCode int

	// Stdout contains standard output.
	Stdout []byte

	// Stderr contains standard error.
	Stderr []byte

	// Duration is how long the execution took.
	Duration time.Duration

	// Killed indicates if the process was killed (e.g., timeout).
	Killed bool

	// KillReason explains why the process was killed.
	KillReason string

	// ResourceUsage contains resource consumption metrics.
	ResourceUsage *ResourceUsage
}

// ResourceUsage tracks resource consumption during execution.
type ResourceUsage struct {
	// CPUTime is the total CPU time consumed.
	CPUTime time.Duration

	// UserTime is the user-mode CPU time.
	UserTime time.Duration

	// SystemTime is the kernel-mode CPU time.
	SystemTime time.Duration

	// MaxMemoryBytes is the peak memory usage.
	MaxMemoryBytes int64

	// DiskReadBytes is the number of bytes read from disk.
	DiskReadBytes int64

	// DiskWriteBytes is the number of bytes written to disk.
	DiskWriteBytes int64
}

// ExecutionOptions configures a single execution.
type ExecutionOptions struct {
	// WorkDir is the working directory for the command.
	WorkDir string

	// Env is additional environment variables.
	Env []string

	// Stdin provides input to the command.
	Stdin io.Reader

	// StdoutWriter receives stdout (in addition to capturing).
	StdoutWriter io.Writer

	// StderrWriter receives stderr (in addition to capturing).
	StderrWriter io.Writer

	// Timeout overrides the policy timeout.
	Timeout time.Duration
}

// NewExecutor creates a new sandbox executor.
func NewExecutor(sandbox Sandbox, policy *Policy) *Executor {
	return &Executor{
		sandbox: sandbox,
		policy:  policy,
	}
}

// Execute runs a command in the sandbox.
func (e *Executor) Execute(ctx context.Context, name string, args []string, opts *ExecutionOptions) (*ExecutionResult, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if opts == nil {
		opts = &ExecutionOptions{}
	}

	// Determine timeout
	timeout := e.policy.Resources.WallTime
	if opts.Timeout > 0 {
		timeout = opts.Timeout
	}

	// Create context with timeout
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	// Wrap command with sandbox
	wrappedName, wrappedArgs := e.sandbox.Wrap(ctx, name, args)

	// Create command
	cmd := exec.CommandContext(ctx, wrappedName, wrappedArgs...)

	// Set working directory
	if opts.WorkDir != "" {
		cmd.Dir = opts.WorkDir
	} else if e.policy.FileSystem.WorkDir != "" {
		cmd.Dir = e.policy.FileSystem.WorkDir
	}

	// Set environment
	cmd.Env = os.Environ()
	if len(opts.Env) > 0 {
		cmd.Env = append(cmd.Env, opts.Env...)
	}

	// Set up input
	if opts.Stdin != nil {
		cmd.Stdin = opts.Stdin
	}

	// Set up output capture
	var stdout, stderr bytes.Buffer
	if opts.StdoutWriter != nil {
		cmd.Stdout = io.MultiWriter(&stdout, opts.StdoutWriter)
	} else {
		cmd.Stdout = &stdout
	}
	if opts.StderrWriter != nil {
		cmd.Stderr = io.MultiWriter(&stderr, opts.StderrWriter)
	} else {
		cmd.Stderr = &stderr
	}

	// Apply process group for clean termination
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	// Execute
	start := time.Now()
	err := cmd.Start()
	if err != nil {
		return &ExecutionResult{
			ExitCode: -1,
			Stderr:   []byte(err.Error()),
		}, fmt.Errorf("starting command: %w", err)
	}

	// Wait for completion
	waitErr := cmd.Wait()
	duration := time.Since(start)

	result := &ExecutionResult{
		ExitCode: 0,
		Stdout:   stdout.Bytes(),
		Stderr:   stderr.Bytes(),
		Duration: duration,
	}

	// Check for context cancellation (timeout or manual cancel)
	if ctx.Err() != nil {
		result.Killed = true
		if ctx.Err() == context.DeadlineExceeded {
			result.KillReason = "timeout"
		} else {
			result.KillReason = "cancelled"
		}
	}

	// Extract exit code
	if waitErr != nil {
		if exitErr, ok := waitErr.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()

			// Extract resource usage on Unix systems
			if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
				_ = status // For potential future use
			}
			if rusage, ok := exitErr.SysUsage().(*syscall.Rusage); ok {
				result.ResourceUsage = &ResourceUsage{
					UserTime:       time.Duration(rusage.Utime.Nano()),
					SystemTime:     time.Duration(rusage.Stime.Nano()),
					CPUTime:        time.Duration(rusage.Utime.Nano() + rusage.Stime.Nano()),
					MaxMemoryBytes: rusage.Maxrss * 1024, // Convert from KB to bytes
				}
			}
		} else if !result.Killed {
			return result, fmt.Errorf("command execution: %w", waitErr)
		}
	}

	return result, nil
}

// ExecuteScript runs a script in the sandbox.
func (e *Executor) ExecuteScript(ctx context.Context, interpreter string, script string, opts *ExecutionOptions) (*ExecutionResult, error) {
	// Create a temporary script file
	tmpFile, err := os.CreateTemp("", "sandbox-script-*")
	if err != nil {
		return nil, fmt.Errorf("creating temp script: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(script); err != nil {
		tmpFile.Close()
		return nil, fmt.Errorf("writing script: %w", err)
	}
	tmpFile.Close()

	// Make executable
	if err := os.Chmod(tmpFile.Name(), 0755); err != nil {
		return nil, fmt.Errorf("chmod script: %w", err)
	}

	return e.Execute(ctx, interpreter, []string{tmpFile.Name()}, opts)
}

// RunInNamespace executes a function in an isolated namespace (Linux only).
func (e *Executor) RunInNamespace(ctx context.Context, fn func() error) error {
	// This is a placeholder for namespace isolation
	// Full implementation would use clone() with CLONE_NEW* flags
	return fn()
}

// Kill terminates any running execution.
func (e *Executor) Kill() error {
	// This would be implemented to track and kill running processes
	return nil
}
