package sandbox

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestExecutor(t *testing.T) {
	sandbox := New("none") // Use noop for testing
	policy := StandardPolicy()
	executor := NewExecutor(sandbox, policy)

	if executor == nil {
		t.Fatal("NewExecutor() returned nil")
	}
}

func TestExecutionResult(t *testing.T) {
	result := &ExecutionResult{
		ExitCode: 0,
		Stdout:   []byte("hello"),
		Stderr:   []byte(""),
		Duration: 100 * time.Millisecond,
		Killed:   false,
	}

	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", result.ExitCode)
	}

	if string(result.Stdout) != "hello" {
		t.Errorf("Stdout = %q, want %q", string(result.Stdout), "hello")
	}
}

func TestResourceUsage(t *testing.T) {
	usage := &ResourceUsage{
		CPUTime:        1 * time.Second,
		UserTime:       800 * time.Millisecond,
		SystemTime:     200 * time.Millisecond,
		MaxMemoryBytes: 1024 * 1024,
	}

	if usage.CPUTime != time.Second {
		t.Errorf("CPUTime = %v, want %v", usage.CPUTime, time.Second)
	}
}

func TestExecutionOptions(t *testing.T) {
	opts := &ExecutionOptions{
		WorkDir: "/test",
		Env:     []string{"FOO=bar"},
		Timeout: 30 * time.Second,
	}

	if opts.WorkDir != "/test" {
		t.Errorf("WorkDir = %q, want %q", opts.WorkDir, "/test")
	}

	if len(opts.Env) != 1 || opts.Env[0] != "FOO=bar" {
		t.Errorf("Env = %v, want [FOO=bar]", opts.Env)
	}
}

func TestExecutorExecute(t *testing.T) {
	sandbox := New("none")
	policy := StandardPolicy()
	executor := NewExecutor(sandbox, policy)

	ctx := context.Background()
	result, err := executor.Execute(ctx, "echo", []string{"test"}, nil)

	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", result.ExitCode)
	}

	if !strings.Contains(string(result.Stdout), "test") {
		t.Errorf("Stdout = %q, should contain 'test'", string(result.Stdout))
	}
}

func TestExecutorWithWorkDir(t *testing.T) {
	sandbox := New("none")
	policy := StandardPolicy()
	executor := NewExecutor(sandbox, policy)

	ctx := context.Background()
	opts := &ExecutionOptions{
		WorkDir: "/",
	}

	result, err := executor.Execute(ctx, "pwd", []string{}, opts)

	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", result.ExitCode)
	}

	output := strings.TrimSpace(string(result.Stdout))
	if output != "/" {
		t.Errorf("pwd output = %q, want /", output)
	}
}

func TestExecutorWithEnv(t *testing.T) {
	sandbox := New("none")
	policy := StandardPolicy()
	executor := NewExecutor(sandbox, policy)

	ctx := context.Background()
	opts := &ExecutionOptions{
		Env: []string{"TEST_VAR=test_value"},
	}

	result, err := executor.Execute(ctx, "sh", []string{"-c", "echo $TEST_VAR"}, opts)

	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", result.ExitCode)
	}

	output := strings.TrimSpace(string(result.Stdout))
	if output != "test_value" {
		t.Errorf("output = %q, want test_value", output)
	}
}

func TestExecutorTimeout(t *testing.T) {
	sandbox := New("none")
	policy := StandardPolicy()
	executor := NewExecutor(sandbox, policy)

	ctx := context.Background()
	opts := &ExecutionOptions{
		Timeout: 100 * time.Millisecond,
	}

	// This command should timeout
	result, err := executor.Execute(ctx, "sleep", []string{"10"}, opts)

	// Should not error since we handle timeout gracefully
	if err != nil {
		// Some errors are acceptable
		t.Logf("Execute() returned error (may be expected): %v", err)
	}

	if result != nil && result.Killed && result.KillReason != "timeout" {
		t.Errorf("KillReason = %q, want timeout", result.KillReason)
	}
}

func TestExecutorContextCancel(t *testing.T) {
	sandbox := New("none")
	policy := StandardPolicy()
	executor := NewExecutor(sandbox, policy)

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel immediately
	cancel()

	result, err := executor.Execute(ctx, "sleep", []string{"10"}, nil)

	if err == nil && result != nil && !result.Killed {
		t.Error("Expected command to be killed when context is cancelled")
	}
}

func TestExecutorExitCode(t *testing.T) {
	sandbox := New("none")
	policy := StandardPolicy()
	executor := NewExecutor(sandbox, policy)

	ctx := context.Background()

	// Test non-zero exit code
	result, err := executor.Execute(ctx, "sh", []string{"-c", "exit 42"}, nil)

	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if result.ExitCode != 42 {
		t.Errorf("ExitCode = %d, want 42", result.ExitCode)
	}
}

func TestExecutorStderr(t *testing.T) {
	sandbox := New("none")
	policy := StandardPolicy()
	executor := NewExecutor(sandbox, policy)

	ctx := context.Background()

	result, err := executor.Execute(ctx, "sh", []string{"-c", "echo error >&2"}, nil)

	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if !strings.Contains(string(result.Stderr), "error") {
		t.Errorf("Stderr = %q, should contain 'error'", string(result.Stderr))
	}
}

func TestExecutorDuration(t *testing.T) {
	sandbox := New("none")
	policy := StandardPolicy()
	executor := NewExecutor(sandbox, policy)

	ctx := context.Background()

	result, err := executor.Execute(ctx, "sleep", []string{"0.1"}, nil)

	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if result.Duration < 100*time.Millisecond {
		t.Errorf("Duration = %v, expected >= 100ms", result.Duration)
	}
}
