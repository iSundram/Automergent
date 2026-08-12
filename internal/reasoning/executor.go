package reasoning

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Executor coordinates task execution with parallelization and progress tracking.
type Executor struct {
	maxWorkers int
	mu         sync.RWMutex

	// Hooks for integration with external systems
	onTaskStart func(task *Task)
	onTaskDone  func(task *Task, result *TaskResult)
	onProgress  func(completed, total int)
}

// MonitoringAdapter receives execution lifecycle signals.
type MonitoringAdapter interface {
	RecordTaskQueued(id string, name string, parentID string)
	RecordTaskStarted(id string, estimatedDuration time.Duration) error
	RecordTaskProgress(id string, progress float64) error
	CompleteTask(id string) error
	FailTask(id string, reason string) error
}

// NewExecutor creates a new task executor.
func NewExecutor(maxWorkers int) *Executor {
	if maxWorkers <= 0 {
		maxWorkers = 3
	}

	return &Executor{
		maxWorkers: maxWorkers,
	}
}

// Execute runs the execution plan with parallelization where possible.
func (e *Executor) Execute(ctx context.Context, plan *ExecutionPlan, state *ExecutionState) error {
	if plan == nil {
		return fmt.Errorf("execution plan is nil")
	}

	if len(plan.Tasks) == 0 {
		return fmt.Errorf("no tasks to execute")
	}

	// Build task map for quick lookup
	taskMap := make(map[string]*Task)
	for _, task := range plan.Tasks {
		taskMap[task.ID] = task
	}

	// Execute tasks in order
	for groupIdx, group := range plan.ExecutionOrder {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("execution cancelled: %w", err)
		}

		// Execute group in parallel
		if err := e.executeGroup(ctx, group, taskMap, state); err != nil {
			return fmt.Errorf("group %d execution failed: %w", groupIdx, err)
		}

		// Report progress
		if e.onProgress != nil {
			completed := len(state.CompletedTasks)
			total := len(plan.Tasks)
			e.onProgress(completed, total)
		}
	}

	// Check if all tasks completed successfully
	for _, task := range plan.Tasks {
		if task.Status != TaskStatusComplete {
			return fmt.Errorf("task %s (%s) did not complete: status=%s",
				task.ID, task.Description, task.Status)
		}
	}

	return nil
}

// executeGroup runs a group of tasks in parallel.
func (e *Executor) executeGroup(ctx context.Context, group []string, taskMap map[string]*Task, state *ExecutionState) error {
	if len(group) == 0 {
		return nil
	}

	// Single task - execute directly
	if len(group) == 1 {
		task := taskMap[group[0]]
		return e.executeTask(ctx, task, state)
	}

	// Multiple tasks - execute in parallel with worker pool
	return e.executeParallel(ctx, group, taskMap, state)
}

// executeParallel runs multiple tasks concurrently with a worker pool.
func (e *Executor) executeParallel(ctx context.Context, taskIDs []string, taskMap map[string]*Task, state *ExecutionState) error {
	// Create channels
	tasks := make(chan *Task, len(taskIDs))
	results := make(chan error, len(taskIDs))

	// Start worker pool
	workerCount := min(e.maxWorkers, len(taskIDs))
	var wg sync.WaitGroup
	wg.Add(workerCount)

	for i := 0; i < workerCount; i++ {
		go func() {
			defer wg.Done()
			for task := range tasks {
				err := e.executeTask(ctx, task, state)
				results <- err
			}
		}()
	}

	// Queue tasks
	go func() {
		for _, taskID := range taskIDs {
			task := taskMap[taskID]
			tasks <- task
		}
		close(tasks)
	}()

	// Wait for completion
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results
	var firstError error
	for err := range results {
		if err != nil && firstError == nil {
			firstError = err
		}
	}

	return firstError
}

// executeTask executes a single task with retries and error handling.
func (e *Executor) executeTask(ctx context.Context, task *Task, state *ExecutionState) error {
	e.mu.Lock()

	// Check if already completed
	if task.Status == TaskStatusComplete {
		e.mu.Unlock()
		return nil
	}

	// Mark as in progress
	task.Status = TaskStatusInProgress
	state.ActiveTasks = append(state.ActiveTasks, task.ID)
	state.UpdatedAt = time.Now()

	// Initialize attempt counter
	if state.Attempts[task.ID] == 0 {
		state.Attempts[task.ID] = 0
	}

	e.mu.Unlock()

	// Notify start
	if e.onTaskStart != nil {
		e.onTaskStart(task)
	}

	// Execute with retries
	maxAttempts := 3
	var lastErr error

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		e.mu.Lock()
		state.Attempts[task.ID] = attempt
		e.mu.Unlock()

		result, err := e.performTask(ctx, task)

		if err == nil {
			// Success
			e.mu.Lock()
			task.Result = result
			task.Status = TaskStatusComplete
			state.CompletedTasks = append(state.CompletedTasks, task.ID)
			state.ActiveTasks = removeString(state.ActiveTasks, task.ID)
			state.UpdatedAt = time.Now()
			e.mu.Unlock()

			if e.onTaskDone != nil {
				e.onTaskDone(task, result)
			}

			return nil
		}

		lastErr = err

		// Check if should retry
		if attempt < maxAttempts && e.shouldRetry(err) {
			time.Sleep(time.Duration(attempt) * time.Second) // Exponential backoff
			continue
		}

		break
	}

	// All attempts failed
	e.mu.Lock()
	task.Status = TaskStatusFailed
	task.Result = &TaskResult{
		Success:     false,
		Error:       lastErr,
		Attempts:    state.Attempts[task.ID],
		CompletedAt: time.Now(),
	}
	state.FailedTasks = append(state.FailedTasks, task.ID)
	state.ActiveTasks = removeString(state.ActiveTasks, task.ID)
	state.UpdatedAt = time.Now()
	e.mu.Unlock()

	if e.onTaskDone != nil {
		e.onTaskDone(task, task.Result)
	}

	return fmt.Errorf("task %s failed after %d attempts: %w", task.ID, maxAttempts, lastErr)
}

// performTask executes the actual task logic.
func (e *Executor) performTask(ctx context.Context, task *Task) (*TaskResult, error) {
	startTime := time.Now()

	// Simulate task execution
	// In real implementation, this would:
	// 1. Call appropriate tools based on task.Tools
	// 2. Execute task-specific logic based on task.Type
	// 3. Verify results using task.Verification checkpoints

	result := &TaskResult{
		Success:      true,
		Output:       fmt.Sprintf("Task %s executed successfully", task.Description),
		Error:        nil,
		Attempts:     1,
		Duration:     time.Since(startTime),
		ToolsUsed:    task.Tools,
		FilesChanged: []string{},
		CompletedAt:  time.Now(),
	}

	// Run verification checkpoints
	for _, checkpoint := range task.Verification {
		if checkpoint.Required {
			// Simulate checkpoint validation
			passed := true
			checkpoint.Passed = &passed
		}
	}

	return result, nil
}

// shouldRetry determines if an error is retryable.
func (e *Executor) shouldRetry(err error) bool {
	// In real implementation, check for specific error types
	// e.g., network errors, timeouts, temporary failures
	return true
}

// SetHooks configures callback functions for execution events.
func (e *Executor) SetHooks(
	onTaskStart func(task *Task),
	onTaskDone func(task *Task, result *TaskResult),
	onProgress func(completed, total int),
) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.onTaskStart = onTaskStart
	e.onTaskDone = onTaskDone
	e.onProgress = onProgress
}

// AttachMonitoring wires monitoring hooks into the executor.
func (e *Executor) AttachMonitoring(adapter MonitoringAdapter) {
	if adapter == nil {
		return
	}
	e.SetHooks(
		func(task *Task) {
			adapter.RecordTaskQueued(task.ID, task.Description, "")
			_ = adapter.RecordTaskStarted(task.ID, task.Estimated)
		},
		func(task *Task, result *TaskResult) {
			if result != nil && result.Success {
				_ = adapter.CompleteTask(task.ID)
				return
			}
			reason := ""
			if result != nil && result.Error != nil {
				reason = result.Error.Error()
			}
			_ = adapter.FailTask(task.ID, reason)
		},
		func(completed, total int) {
			if total > 0 {
				_ = adapter.RecordTaskProgress("execution-progress", float64(completed)/float64(total))
			}
		},
	)
}

// AdaptiveExecutor extends Executor with adaptive strategy switching.
type AdaptiveExecutor struct {
	*Executor
	strategies map[string]ExecutionStrategy
}

// ExecutionStrategy defines how to execute a specific task type.
type ExecutionStrategy interface {
	Execute(ctx context.Context, task *Task) (*TaskResult, error)
	CanHandle(task *Task) bool
	Priority() int
}

// NewAdaptiveExecutor creates an executor with adaptive strategy selection.
func NewAdaptiveExecutor(maxWorkers int) *AdaptiveExecutor {
	return &AdaptiveExecutor{
		Executor:   NewExecutor(maxWorkers),
		strategies: make(map[string]ExecutionStrategy),
	}
}

// RegisterStrategy adds a new execution strategy.
func (e *AdaptiveExecutor) RegisterStrategy(name string, strategy ExecutionStrategy) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.strategies[name] = strategy
}

// selectStrategy chooses the best strategy for a task.
func (e *AdaptiveExecutor) selectStrategy(task *Task) ExecutionStrategy {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var bestStrategy ExecutionStrategy
	bestPriority := -1

	for _, strategy := range e.strategies {
		if strategy.CanHandle(task) {
			if priority := strategy.Priority(); priority > bestPriority {
				bestStrategy = strategy
				bestPriority = priority
			}
		}
	}

	return bestStrategy
}

// ErrorRecovery handles execution errors with intelligent recovery.
type ErrorRecovery struct {
	maxRetries    int
	backoffFactor time.Duration
	handlers      map[string]RecoveryHandler
}

// RecoveryHandler attempts to recover from specific error types.
type RecoveryHandler interface {
	CanRecover(err error) bool
	Recover(ctx context.Context, task *Task, err error) error
}

// NewErrorRecovery creates a new error recovery system.
func NewErrorRecovery(maxRetries int) *ErrorRecovery {
	return &ErrorRecovery{
		maxRetries:    maxRetries,
		backoffFactor: time.Second,
		handlers:      make(map[string]RecoveryHandler),
	}
}

// RegisterHandler adds a recovery handler.
func (r *ErrorRecovery) RegisterHandler(name string, handler RecoveryHandler) {
	r.handlers[name] = handler
}

// Recover attempts to recover from an execution error.
func (r *ErrorRecovery) Recover(ctx context.Context, task *Task, err error) error {
	for _, handler := range r.handlers {
		if handler.CanRecover(err) {
			return handler.Recover(ctx, task, err)
		}
	}
	return err
}

// ProgressTracker monitors execution progress.
type ProgressTracker struct {
	total     int
	completed int
	failed    int
	startTime time.Time
	mu        sync.RWMutex
}

// NewProgressTracker creates a progress tracker.
func NewProgressTracker(total int) *ProgressTracker {
	return &ProgressTracker{
		total:     total,
		startTime: time.Now(),
	}
}

// Update increments progress counters.
func (p *ProgressTracker) Update(completed, failed int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.completed = completed
	p.failed = failed
}

// GetProgress returns current progress statistics.
func (p *ProgressTracker) GetProgress() (completed, failed, total int, elapsed time.Duration) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.completed, p.failed, p.total, time.Since(p.startTime)
}

// EstimateRemaining estimates time remaining.
func (p *ProgressTracker) EstimateRemaining() time.Duration {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.completed == 0 {
		return 0
	}

	elapsed := time.Since(p.startTime)
	avgPerTask := elapsed / time.Duration(p.completed)
	remaining := p.total - p.completed - p.failed

	return avgPerTask * time.Duration(remaining)
}

// Helper functions

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func removeString(slice []string, s string) []string {
	result := []string{}
	for _, item := range slice {
		if item != s {
			result = append(result, item)
		}
	}
	return result
}
