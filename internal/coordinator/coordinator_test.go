package coordinator

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// mockExecutor records calls and returns configurable results.
type mockExecutor struct {
	mu       sync.Mutex
	calls    int
	results  map[string]*TaskResult
	delay    time.Duration
	failRole AgentRole // if set, tasks for this role fail
}

func newMockExecutor() *mockExecutor {
	return &mockExecutor{
		results: make(map[string]*TaskResult),
	}
}

func (m *mockExecutor) Execute(ctx context.Context, role AgentRole, prompt string, taskCtx TaskContext, model string) (*TaskResult, error) {
	m.mu.Lock()
	m.calls++
	m.mu.Unlock()

	if m.delay > 0 {
		select {
		case <-time.After(m.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	if m.failRole != "" && role == m.failRole {
		return nil, fmt.Errorf("mock failure for role %s", role)
	}

	if r, ok := m.results[string(role)]; ok {
		return r, nil
	}

	return &TaskResult{
		Success:  true,
		Output:   fmt.Sprintf("result from %s", role),
		Quality:  0.85,
		TokensUsed: 100,
	}, nil
}

func (m *mockExecutor) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}

func testConfig() *CoordinatorConfig {
	return &CoordinatorConfig{
		MaxWorkers:     5,
		WorkersPerRole: map[AgentRole]int{RoleCoder: 2, RoleResearcher: 1},
		DefaultTimeout: 5 * time.Second,
		MaxRetries:     1,
		ResourceLimits: ResourceLimits{
			MaxTokensPerTask: 10000,
			RateLimitPerMinute: 60,
		},
	}
}

func TestEngine_StartStop(t *testing.T) {
	exec := newMockExecutor()
	engine := NewEngine(testConfig(), exec)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := engine.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if !engine.running.Load() {
		t.Error("engine should be running after Start")
	}

	workers := engine.ListWorkers()
	if len(workers) != 3 { // 2 coders + 1 researcher
		t.Errorf("expected 3 workers, got %d", len(workers))
	}

	if err := engine.Stop(context.Background()); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	if engine.running.Load() {
		t.Error("engine should not be running after Stop")
	}
}

func TestEngine_DoubleStart(t *testing.T) {
	exec := newMockExecutor()
	engine := NewEngine(testConfig(), exec)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := engine.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer engine.Stop(context.Background())

	if err := engine.Start(ctx); err == nil {
		t.Error("expected error on double Start")
	}
}

func TestEngine_SubmitAndGetTask(t *testing.T) {
	exec := newMockExecutor()
	engine := NewEngine(testConfig(), exec)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	engine.Start(ctx)
	defer engine.Stop(context.Background())

	task := &Task{
		ID:     "test-1",
		Role:   RoleCoder,
		Prompt: "write a function",
	}

	if err := engine.Submit(task); err != nil {
		t.Fatalf("Submit failed: %v", err)
	}

	got, err := engine.GetTask("test-1")
	if err != nil {
		t.Fatalf("GetTask failed: %v", err)
	}

	if got.ID != "test-1" {
		t.Errorf("expected task ID test-1, got %s", got.ID)
	}
}

func TestEngine_SubmitBatch(t *testing.T) {
	exec := newMockExecutor()
	cfg := testConfig()
	cfg.WorkersPerRole = map[AgentRole]int{} // No workers — tasks just queue.
	engine := NewEngine(cfg, exec)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	engine.Start(ctx)
	defer engine.Stop(context.Background())

	tasks := []*Task{
		{ID: "batch-1", Role: RoleCoder, Prompt: "task 1"},
		{ID: "batch-2", Role: RoleCoder, Prompt: "task 2"},
		{ID: "batch-3", Role: RoleResearcher, Prompt: "task 3"},
	}

	if err := engine.SubmitBatch(tasks); err != nil {
		t.Fatalf("SubmitBatch failed: %v", err)
	}

	if engine.queues.Len() != 3 {
		t.Errorf("expected 3 queued tasks, got %d", engine.queues.Len())
	}
}

func TestEngine_CancelTask(t *testing.T) {
	exec := newMockExecutor()
	engine := NewEngine(testConfig(), exec)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	engine.Start(ctx)
	defer engine.Stop(context.Background())

	task := &Task{
		ID:      "cancel-1",
		Role:    RoleCoder,
		Prompt:  "long task",
		Timeout: 10 * time.Second,
	}
	engine.Submit(task)

	if err := engine.Cancel("cancel-1"); err != nil {
		t.Fatalf("Cancel failed: %v", err)
	}

	got, _ := engine.GetTask("cancel-1")
	got.mu.RLock()
	defer got.mu.RUnlock()
	if got.Status != TaskStatusCancelled {
		t.Errorf("expected cancelled status, got %s", got.Status)
	}
}

func TestEngine_DependencyResolution(t *testing.T) {
	exec := newMockExecutor()
	engine := NewEngine(testConfig(), exec)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	engine.Start(ctx)
	defer engine.Stop(context.Background())

	// Task B depends on Task A.
	taskA := &Task{ID: "dep-a", Role: RoleCoder, Prompt: "A"}
	taskB := &Task{ID: "dep-b", Role: RoleCoder, Prompt: "B", Dependencies: []string{"dep-a"}}

	engine.Submit(taskA)
	engine.Submit(taskB)

	// Task B should be pending (waiting for A).
	got, _ := engine.GetTask("dep-b")
	got.mu.RLock()
	if got.Status != TaskStatusPending {
		t.Errorf("expected task B pending, got %s", got.Status)
	}
	got.mu.RUnlock()
}

func TestEngine_Metrics(t *testing.T) {
	exec := newMockExecutor()
	engine := NewEngine(testConfig(), exec)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	engine.Start(ctx)
	defer engine.Stop(context.Background())

	metrics := engine.Metrics()
	// Workers start idle, so active count is 0.
	if metrics.ActiveWorkers != 0 {
		t.Errorf("expected 0 active workers at startup, got %d", metrics.ActiveWorkers)
	}
}

func TestEngine_ListWorkers(t *testing.T) {
	exec := newMockExecutor()
	engine := NewEngine(testConfig(), exec)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	engine.Start(ctx)
	defer engine.Stop(context.Background())

	workers := engine.ListWorkers()
	if len(workers) != 3 {
		t.Errorf("expected 3 workers, got %d", len(workers))
	}

	// Check each worker has a role.
	roleCount := make(map[AgentRole]int)
	for _, w := range workers {
		roleCount[w.Role]++
	}
	if roleCount[RoleCoder] != 2 {
		t.Errorf("expected 2 coder workers, got %d", roleCount[RoleCoder])
	}
	if roleCount[RoleResearcher] != 1 {
		t.Errorf("expected 1 researcher worker, got %d", roleCount[RoleResearcher])
	}
}

func TestEngine_GetModelForRole_Override(t *testing.T) {
	cfg := testConfig()
	cfg.ModelOverrides = map[AgentRole]string{
		RoleCoder:      "custom-coder-model",
		RoleResearcher: "custom-researcher-model",
	}
	exec := newMockExecutor()
	engine := NewEngine(cfg, exec)

	if got := engine.getModelForRole(RoleCoder); got != "custom-coder-model" {
		t.Errorf("expected custom-coder-model, got %s", got)
	}
	if got := engine.getModelForRole(RoleResearcher); got != "custom-researcher-model" {
		t.Errorf("expected custom-researcher-model, got %s", got)
	}
	// Non-overridden role should use global model.
	if got := engine.getModelForRole(RoleReviewer); got != cfg.Model {
		t.Errorf("expected global model %s, got %s", cfg.Model, got)
	}
}

func TestEngine_GetModelForRole_Fallback(t *testing.T) {
	cfg := testConfig()
	cfg.Model = "global-model"
	cfg.FallbackModel = "fallback-model"
	exec := newMockExecutor()
	engine := NewEngine(cfg, exec)

	// Coder should use global model.
	if got := engine.getModelForRole(RoleCoder); got != "global-model" {
		t.Errorf("expected global-model for coder, got %s", got)
	}
	// Researcher should use fallback model.
	if got := engine.getModelForRole(RoleResearcher); got != "fallback-model" {
		t.Errorf("expected fallback-model for researcher, got %s", got)
	}
}

func TestEngine_RateLimiter(t *testing.T) {
	rl := newRateLimiter(60) // 60 per minute = 1 per second
	if rl == nil {
		t.Fatal("expected non-nil rate limiter")
	}

	ctx := context.Background()

	// Should be able to consume tokens immediately (bucket is full).
	for i := 0; i < 60; i++ {
		if err := rl.wait(ctx); err != nil {
			t.Fatalf("unexpected error on token %d: %v", i, err)
		}
	}

	// After consuming all tokens, should block (use a short timeout to test).
	shortCtx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	if err := rl.wait(shortCtx); err == nil {
		t.Error("expected timeout error when rate limited")
	}

	rl.stop_()
}

func TestEngine_RateLimiterNil(t *testing.T) {
	var rl *rateLimiter
	if err := rl.wait(context.Background()); err != nil {
		t.Errorf("nil rate limiter should not error, got %v", err)
	}
}

func TestEngine_CancelNonexistent(t *testing.T) {
	exec := newMockExecutor()
	engine := NewEngine(testConfig(), exec)

	if err := engine.Cancel("nonexistent"); err == nil {
		t.Error("expected error for nonexistent task")
	}
}

func TestEngine_GetTaskNonexistent(t *testing.T) {
	exec := newMockExecutor()
	engine := NewEngine(testConfig(), exec)

	if _, err := engine.GetTask("nonexistent"); err == nil {
		t.Error("expected error for nonexistent task")
	}
}

func TestEngine_ListTasks(t *testing.T) {
	exec := newMockExecutor()
	engine := NewEngine(testConfig(), exec)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	engine.Start(ctx)
	defer engine.Stop(context.Background())

	engine.Submit(&Task{ID: "a", Role: RoleCoder, Prompt: "a"})
	engine.Submit(&Task{ID: "b", Role: RoleResearcher, Prompt: "b"})
	engine.Submit(&Task{ID: "c", Role: RoleCoder, Prompt: "c"})

	tasks := engine.ListTasks(TaskFilter{
		Role: []AgentRole{RoleCoder},
	})
	if len(tasks) != 2 {
		t.Errorf("expected 2 coder tasks, got %d", len(tasks))
	}

	tasks = engine.ListTasks(TaskFilter{
		Limit: 1,
	})
	if len(tasks) != 1 {
		t.Errorf("expected 1 task with limit, got %d", len(tasks))
	}
}

func TestEngine_StopIdempotent(t *testing.T) {
	exec := newMockExecutor()
	engine := NewEngine(testConfig(), exec)

	// Stop without start should be a no-op.
	if err := engine.Stop(context.Background()); err != nil {
		t.Fatalf("Stop on non-running engine failed: %v", err)
	}
}

func TestEngine_TaskCompletionCh(t *testing.T) {
	var completed atomic.Int32
	exec := newMockExecutor()
	engine := NewEngine(testConfig(), exec)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	engine.Start(ctx)
	defer engine.Stop(context.Background())

	task := &Task{
		ID:      "ch-test",
		Role:    RoleCoder,
		Prompt:  "test completion channel",
		Timeout: 5 * time.Second,
	}
	engine.Submit(task)

	// Wait for task to complete via channel.
	select {
	case <-task.completionCh:
		completed.Add(1)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for task completion")
	}

	if completed.Load() != 1 {
		t.Error("expected task to complete")
	}
}

func TestEngine_ConcurrentSubmit(t *testing.T) {
	exec := newMockExecutor()
	cfg := testConfig()
	cfg.WorkersPerRole = map[AgentRole]int{} // No workers — tasks just queue.
	engine := NewEngine(cfg, exec)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	engine.Start(ctx)
	defer engine.Stop(context.Background())

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			task := &Task{
				ID:     fmt.Sprintf("concurrent-%d", id),
				Role:   RoleCoder,
				Prompt: fmt.Sprintf("task %d", id),
			}
			engine.Submit(task)
		}(i)
	}
	wg.Wait()

	if engine.queues.Len() != 50 {
		t.Errorf("expected 50 queued tasks, got %d", engine.queues.Len())
	}
}
