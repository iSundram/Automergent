package coordinator

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

// Engine is the main coordinator implementation.
type Engine struct {
	config    *CoordinatorConfig
	executor  AgentExecutor
	workers   map[string]*Worker
	tasks     map[string]*Task
	taskQueue *taskQueue
	events    chan CoordinatorEvent
	metrics   *CoordinatorMetrics

	workerWG sync.WaitGroup
	taskWG   sync.WaitGroup

	ctx     context.Context
	cancel  context.CancelFunc
	running atomic.Bool
	mu      sync.RWMutex
}

// NewEngine creates a new coordinator engine.
func NewEngine(config *CoordinatorConfig, executor AgentExecutor) *Engine {
	if config == nil {
		config = DefaultConfig()
	}

	return &Engine{
		config:    config,
		executor:  executor,
		workers:   make(map[string]*Worker),
		tasks:     make(map[string]*Task),
		taskQueue: newTaskQueue(),
		events:    make(chan CoordinatorEvent, 1024),
		metrics:   &CoordinatorMetrics{},
	}
}

// Start initializes and starts the coordinator.
func (e *Engine) Start(ctx context.Context) error {
	if e.running.Load() {
		return fmt.Errorf("coordinator already running")
	}

	e.ctx, e.cancel = context.WithCancel(ctx)
	e.running.Store(true)

	// Initialize workers for each role
	for role, count := range e.config.WorkersPerRole {
		for i := 0; i < count; i++ {
			if err := e.spawnWorker(role); err != nil {
				return fmt.Errorf("failed to spawn worker: %w", err)
			}
		}
	}

	// Start the task dispatcher
	go e.dispatchLoop()

	// Start metrics collector
	go e.metricsLoop()

	// Start work stealing if enabled
	if e.config.EnableWorkStealing {
		go e.workStealingLoop()
	}

	return nil
}

// Stop gracefully shuts down the coordinator.
func (e *Engine) Stop(ctx context.Context) error {
	if !e.running.Load() {
		return nil
	}

	e.running.Store(false)
	e.cancel()

	// Signal all workers to stop
	e.mu.Lock()
	for _, w := range e.workers {
		w.mu.Lock()
		w.Status = WorkerStatusStopping
		w.mu.Unlock()
	}
	e.mu.Unlock()

	// Wait for workers with timeout
	done := make(chan struct{})
	go func() {
		e.workerWG.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-ctx.Done():
		return ctx.Err()
	}

	close(e.events)
	return nil
}

// Submit adds a task to the queue.
func (e *Engine) Submit(task *Task) error {
	if task.ID == "" {
		task.ID = uuid.New().String()
	}
	if task.CreatedAt.IsZero() {
		task.CreatedAt = time.Now()
	}
	if task.Status == "" {
		task.Status = TaskStatusPending
	}
	if task.Priority == 0 {
		task.Priority = PriorityNormal
	}
	if task.Timeout == 0 {
		task.Timeout = e.config.DefaultTimeout
	}
	if task.MaxRetries == 0 {
		task.MaxRetries = e.config.MaxRetries
	}
	if task.Metadata == nil {
		task.Metadata = make(map[string]any)
	}

	e.mu.Lock()
	e.tasks[task.ID] = task
	e.mu.Unlock()

	// Check dependencies
	if len(task.Dependencies) > 0 {
		if !e.checkDependencies(task) {
			task.Status = TaskStatusPending
			return nil // Will be queued when dependencies complete
		}
	}

	task.Status = TaskStatusQueued
	e.taskQueue.Push(task)
	e.emit(EventTaskQueued, task.ID, "", task)

	return nil
}

// SubmitBatch adds multiple tasks to the queue.
func (e *Engine) SubmitBatch(tasks []*Task) error {
	for _, task := range tasks {
		if err := e.Submit(task); err != nil {
			return err
		}
	}
	return nil
}

// Cancel cancels a task.
func (e *Engine) Cancel(taskID string) error {
	e.mu.RLock()
	task, ok := e.tasks[taskID]
	e.mu.RUnlock()

	if !ok {
		return fmt.Errorf("task not found: %s", taskID)
	}

	task.mu.Lock()
	task.Status = TaskStatusCancelled
	task.CompletedAt = time.Now()
	task.mu.Unlock()

	return nil
}

// GetTask retrieves a task by ID.
func (e *Engine) GetTask(taskID string) (*Task, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	task, ok := e.tasks[taskID]
	if !ok {
		return nil, fmt.Errorf("task not found: %s", taskID)
	}
	return task, nil
}

// ListTasks returns tasks matching the filter.
func (e *Engine) ListTasks(filter TaskFilter) []*Task {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var result []*Task
	for _, task := range e.tasks {
		if matchesFilter(task, filter) {
			result = append(result, task)
		}
	}

	if filter.Limit > 0 && len(result) > filter.Limit {
		result = result[:filter.Limit]
	}

	return result
}

// ScaleWorkers adjusts the number of workers for a role.
func (e *Engine) ScaleWorkers(role AgentRole, count int) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	currentCount := 0
	var roleWorkers []*Worker
	for _, w := range e.workers {
		if w.Role == role {
			currentCount++
			roleWorkers = append(roleWorkers, w)
		}
	}

	if count > currentCount {
		// Scale up
		for i := 0; i < count-currentCount; i++ {
			if err := e.spawnWorkerLocked(role); err != nil {
				return err
			}
		}
	} else if count < currentCount {
		// Scale down
		toRemove := currentCount - count
		for i := 0; i < toRemove && i < len(roleWorkers); i++ {
			w := roleWorkers[i]
			w.mu.Lock()
			w.Status = WorkerStatusStopping
			w.mu.Unlock()
		}
	}

	e.config.WorkersPerRole[role] = count
	return nil
}

// GetWorker retrieves a worker by ID.
func (e *Engine) GetWorker(workerID string) (*Worker, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	worker, ok := e.workers[workerID]
	if !ok {
		return nil, fmt.Errorf("worker not found: %s", workerID)
	}
	return worker, nil
}

// ListWorkers returns all workers.
func (e *Engine) ListWorkers() []*Worker {
	e.mu.RLock()
	defer e.mu.RUnlock()

	result := make([]*Worker, 0, len(e.workers))
	for _, w := range e.workers {
		result = append(result, w)
	}
	return result
}

// Execute runs an execution plan and synthesizes results.
func (e *Engine) Execute(ctx context.Context, plan *ExecutionPlan) (*SynthesisResult, error) {
	var allResults []*TaskResult

	// Execute phases in order
	for _, phase := range plan.Phases {
		var phaseTasks []*Task
		for _, taskID := range phase.TaskIDs {
			if task, err := e.GetTask(taskID); err == nil {
				phaseTasks = append(phaseTasks, task)
			}
		}

		if phase.Parallel {
			results, err := e.ExecuteParallel(ctx, phaseTasks)
			if err != nil {
				return nil, fmt.Errorf("phase %d failed: %w", phase.Index, err)
			}
			allResults = append(allResults, results...)
		} else {
			// Execute sequentially
			for _, task := range phaseTasks {
				if err := e.Submit(task); err != nil {
					return nil, err
				}
				// Wait for completion
				if err := e.waitForTask(ctx, task.ID); err != nil {
					return nil, err
				}
				if task.Result != nil {
					allResults = append(allResults, task.Result)
				}
			}
		}
	}

	return e.Synthesize(ctx, allResults)
}

// ExecuteParallel runs multiple tasks in parallel.
func (e *Engine) ExecuteParallel(ctx context.Context, tasks []*Task) ([]*TaskResult, error) {
	var wg sync.WaitGroup
	results := make([]*TaskResult, len(tasks))
	errors := make([]error, len(tasks))

	for i, task := range tasks {
		wg.Add(1)
		go func(idx int, t *Task) {
			defer wg.Done()

			if err := e.Submit(t); err != nil {
				errors[idx] = err
				return
			}

			if err := e.waitForTask(ctx, t.ID); err != nil {
				errors[idx] = err
				return
			}

			results[idx] = t.Result
		}(i, task)
	}

	wg.Wait()

	// Check for errors
	for _, err := range errors {
		if err != nil {
			return nil, err
		}
	}

	// Filter nil results
	var validResults []*TaskResult
	for _, r := range results {
		if r != nil {
			validResults = append(validResults, r)
		}
	}

	return validResults, nil
}

// Synthesize aggregates results using quality-weighted merging.
func (e *Engine) Synthesize(ctx context.Context, results []*TaskResult) (*SynthesisResult, error) {
	if len(results) == 0 {
		return &SynthesisResult{}, nil
	}

	e.emit(EventSynthesisStart, "", "", len(results))

	synthesis := &SynthesisResult{
		SelectedResults: results,
	}

	// Detect conflicts
	conflicts := e.detectConflicts(results)
	synthesis.ConflictCount = len(conflicts)

	if len(conflicts) > 0 {
		resolutions, err := e.ResolveConflicts(ctx, conflicts)
		if err != nil {
			return nil, err
		}

		// Apply resolutions
		for _, resolution := range resolutions {
			if resolution.ChosenOption != nil {
				// Mark the chosen result
				for _, r := range results {
					if r.TaskID == resolution.ChosenOption.TaskID {
						r.Quality += 0.1 // Boost resolved quality
					}
				}
			}
		}
	}

	// Quality-weighted merge
	synthesis.FinalOutput = e.mergeResults(results)
	synthesis.QualityScore = e.calculateAverageQuality(results)
	synthesis.ConsensusLevel = e.calculateConsensus(results)
	synthesis.Artifacts = e.collectArtifacts(results)
	synthesis.Summary = e.generateSummary(results)

	e.emit(EventSynthesisDone, "", "", synthesis)

	return synthesis, nil
}

// ResolveConflicts handles conflicts between task results.
func (e *Engine) ResolveConflicts(ctx context.Context, conflicts []*Conflict) ([]*ConflictResolution, error) {
	var resolutions []*ConflictResolution

	for _, conflict := range conflicts {
		e.emit(EventConflict, "", "", conflict)

		resolution := &ConflictResolution{
			Strategy:   ResolutionByQuality,
			ResolvedAt: time.Now(),
		}

		// Find best option by quality
		var bestOption *ConflictOption
		for i := range conflict.Options {
			opt := &conflict.Options[i]
			if bestOption == nil || opt.Quality > bestOption.Quality {
				bestOption = opt
			}
		}

		if bestOption != nil {
			resolution.ChosenOption = bestOption
			resolution.Reasoning = fmt.Sprintf("Selected based on quality score %.2f", bestOption.Quality)
		}

		conflict.Resolution = resolution
		resolutions = append(resolutions, resolution)

		e.emit(EventConsensus, "", "", resolution)
		e.metrics.mu.Lock()
		e.metrics.ConsensusCount++
		e.metrics.mu.Unlock()
	}

	return resolutions, nil
}

// Events returns the event channel.
func (e *Engine) Events() <-chan CoordinatorEvent {
	return e.events
}

// Metrics returns current coordinator metrics.
func (e *Engine) Metrics() *CoordinatorMetrics {
	e.metrics.mu.RLock()
	defer e.metrics.mu.RUnlock()

	// Count workers and tasks
	e.mu.RLock()
	activeWorkers := 0
	for _, w := range e.workers {
		w.mu.RLock()
		if w.Status == WorkerStatusBusy {
			activeWorkers++
		}
		w.mu.RUnlock()
	}

	pendingTasks := 0
	runningTasks := 0
	completedTasks := 0
	failedTasks := 0
	for _, t := range e.tasks {
		t.mu.RLock()
		switch t.Status {
		case TaskStatusPending, TaskStatusQueued:
			pendingTasks++
		case TaskStatusRunning:
			runningTasks++
		case TaskStatusCompleted:
			completedTasks++
		case TaskStatusFailed:
			failedTasks++
		}
		t.mu.RUnlock()
	}
	e.mu.RUnlock()

	return &CoordinatorMetrics{
		ActiveWorkers:   activeWorkers,
		PendingTasks:    pendingTasks,
		RunningTasks:    runningTasks,
		CompletedTasks:  completedTasks,
		FailedTasks:     failedTasks,
		TotalTokensUsed: e.metrics.TotalTokensUsed,
		AvgTaskDuration: e.metrics.AvgTaskDuration,
		AvgQualityScore: e.metrics.AvgQualityScore,
		WorkStealCount:  e.metrics.WorkStealCount,
		ConflictCount:   e.metrics.ConflictCount,
		ConsensusCount:  e.metrics.ConsensusCount,
	}
}

// Internal methods

func (e *Engine) spawnWorker(role AgentRole) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.spawnWorkerLocked(role)
}

func (e *Engine) spawnWorkerLocked(role AgentRole) error {
	workerID := fmt.Sprintf("%s-%s", role, uuid.New().String()[:8])

	worker := &Worker{
		ID:        workerID,
		Role:      role,
		Status:    WorkerStatusIdle,
		StartedAt: time.Now(),
		Model:     e.getModelForRole(role),
	}

	e.workers[workerID] = worker
	e.workerWG.Add(1)

	go e.workerLoop(worker)

	e.emit(EventWorkerStarted, "", workerID, worker)

	return nil
}

func (e *Engine) workerLoop(worker *Worker) {
	defer e.workerWG.Done()
	defer func() {
		worker.mu.Lock()
		worker.Status = WorkerStatusStopped
		worker.mu.Unlock()
		e.emit(EventWorkerStopped, "", worker.ID, nil)
	}()

	for {
		select {
		case <-e.ctx.Done():
			return
		default:
		}

		worker.mu.RLock()
		stopping := worker.Status == WorkerStatusStopping
		worker.mu.RUnlock()

		if stopping {
			return
		}

		// Get next task for this role
		task := e.getNextTask(worker.Role)
		if task == nil {
			time.Sleep(100 * time.Millisecond)
			continue
		}

		// Execute the task
		worker.mu.Lock()
		worker.Status = WorkerStatusBusy
		worker.CurrentTask = task
		worker.LastActiveAt = time.Now()
		worker.mu.Unlock()

		task.mu.Lock()
		task.Status = TaskStatusRunning
		task.StartedAt = time.Now()
		task.mu.Unlock()

		e.emit(EventTaskStarted, task.ID, worker.ID, task)

		// Execute with timeout
		ctx, cancel := context.WithTimeout(e.ctx, task.Timeout)
		result := e.executeTask(ctx, worker, task)
		cancel()

		// Handle result
		task.mu.Lock()
		task.Result = result
		task.CompletedAt = time.Now()

		if result.Success {
			task.Status = TaskStatusCompleted
			e.emit(EventTaskCompleted, task.ID, worker.ID, result)
		} else {
			if task.Retries < task.MaxRetries {
				task.Retries++
				task.Status = TaskStatusRetrying
				e.emit(EventTaskRetrying, task.ID, worker.ID, task.Retries)
				// Re-queue the task
				e.taskQueue.Push(task)
			} else {
				task.Status = TaskStatusFailed
				e.emit(EventTaskFailed, task.ID, worker.ID, result.Errors)
			}
		}
		task.mu.Unlock()

		// Update worker metrics
		worker.mu.Lock()
		worker.Status = WorkerStatusIdle
		worker.CurrentTask = nil
		worker.TasksHandled++
		worker.TokensUsed += result.TokensUsed
		worker.Metrics.TasksCompleted++
		if !result.Success {
			worker.Metrics.TasksFailed++
		}
		worker.Metrics.TotalDuration += result.Duration
		if worker.Metrics.TasksCompleted > 0 {
			worker.Metrics.AvgDuration = worker.Metrics.TotalDuration / time.Duration(worker.Metrics.TasksCompleted)
			worker.Metrics.SuccessRate = float64(worker.Metrics.TasksCompleted-worker.Metrics.TasksFailed) / float64(worker.Metrics.TasksCompleted)
		}
		worker.mu.Unlock()

		// Check and process dependent tasks
		e.processDependentTasks(task.ID)
	}
}

func (e *Engine) executeTask(ctx context.Context, worker *Worker, task *Task) *TaskResult {
	startTime := time.Now()

	if e.executor == nil {
		return &TaskResult{
			TaskID:   task.ID,
			WorkerID: worker.ID,
			Success:  false,
			Errors:   []string{"no executor configured"},
			Duration: time.Since(startTime),
		}
	}

	result, err := e.executor.Execute(ctx, worker.Role, task.Prompt, task.Context, worker.Model)
	if err != nil {
		return &TaskResult{
			TaskID:   task.ID,
			WorkerID: worker.ID,
			Success:  false,
			Errors:   []string{err.Error()},
			Duration: time.Since(startTime),
		}
	}

	if result == nil {
		result = &TaskResult{}
	}

	result.TaskID = task.ID
	result.WorkerID = worker.ID
	result.Duration = time.Since(startTime)

	return result
}

func (e *Engine) getNextTask(role AgentRole) *Task {
	for {
		task := e.taskQueue.Pop()
		if task == nil {
			return nil
		}

		task.mu.RLock()
		if task.Status == TaskStatusCancelled {
			task.mu.RUnlock()
			continue
		}

		// Check if task role matches (or task can be handled by any role)
		if task.Role != "" && task.Role != role {
			task.mu.RUnlock()
			// Put back in queue
			e.taskQueue.Push(task)
			return nil
		}
		task.mu.RUnlock()

		return task
	}
}

func (e *Engine) dispatchLoop() {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-e.ctx.Done():
			return
		case <-ticker.C:
			// Check for pending tasks with satisfied dependencies
			e.mu.RLock()
			for _, task := range e.tasks {
				task.mu.RLock()
				if task.Status == TaskStatusPending && len(task.Dependencies) > 0 {
					task.mu.RUnlock()
					if e.checkDependencies(task) {
						task.mu.Lock()
						task.Status = TaskStatusQueued
						task.mu.Unlock()
						e.taskQueue.Push(task)
					}
				} else {
					task.mu.RUnlock()
				}
			}
			e.mu.RUnlock()
		}
	}
}

func (e *Engine) metricsLoop() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-e.ctx.Done():
			return
		case <-ticker.C:
			e.updateMetrics()
		}
	}
}

func (e *Engine) workStealingLoop() {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-e.ctx.Done():
			return
		case <-ticker.C:
			e.tryWorkStealing()
		}
	}
}

func (e *Engine) tryWorkStealing() {
	e.mu.RLock()
	defer e.mu.RUnlock()

	// Find idle workers
	var idleWorkers []*Worker
	for _, w := range e.workers {
		w.mu.RLock()
		if w.Status == WorkerStatusIdle {
			idleWorkers = append(idleWorkers, w)
		}
		w.mu.RUnlock()
	}

	if len(idleWorkers) == 0 {
		return
	}

	// Check if there are queued tasks that could use different roles
	queuedCount := e.taskQueue.Len()
	if queuedCount > len(idleWorkers) {
		// Allow idle workers to steal tasks from other roles
		for _, worker := range idleWorkers {
			task := e.taskQueue.Pop()
			if task == nil {
				break
			}

			task.mu.Lock()
			if task.Role != "" && task.Role != worker.Role {
				// This is a steal - log it
				e.metrics.mu.Lock()
				e.metrics.WorkStealCount++
				e.metrics.mu.Unlock()
				e.emit(EventWorkerStolen, task.ID, worker.ID, worker.Role)
			}
			task.mu.Unlock()

			// Put task back - the worker loop will pick it up
			e.taskQueue.Push(task)
		}
	}
}

func (e *Engine) checkDependencies(task *Task) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.checkDependenciesUnlocked(task)
}

// checkDependenciesUnlocked checks if all dependencies are met.
// Assumes e.mu is already held by caller.
func (e *Engine) checkDependenciesUnlocked(task *Task) bool {
	for _, depID := range task.Dependencies {
		dep, ok := e.tasks[depID]
		if !ok {
			return false
		}
		dep.mu.RLock()
		if dep.Status != TaskStatusCompleted {
			dep.mu.RUnlock()
			return false
		}
		dep.mu.RUnlock()
	}
	return true
}

func (e *Engine) processDependentTasks(completedTaskID string) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	for _, task := range e.tasks {
		task.mu.RLock()
		if task.Status != TaskStatusPending {
			task.mu.RUnlock()
			continue
		}

		hasDep := false
		for _, dep := range task.Dependencies {
			if dep == completedTaskID {
				hasDep = true
				break
			}
		}
		task.mu.RUnlock()

		if hasDep && e.checkDependenciesUnlocked(task) {
			task.mu.Lock()
			task.Status = TaskStatusQueued
			task.mu.Unlock()
			e.taskQueue.Push(task)
		}
	}
}

func (e *Engine) waitForTask(ctx context.Context, taskID string) error {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			task, err := e.GetTask(taskID)
			if err != nil {
				return err
			}
			task.mu.RLock()
			status := task.Status
			task.mu.RUnlock()

			switch status {
			case TaskStatusCompleted:
				return nil
			case TaskStatusFailed:
				return fmt.Errorf("task failed: %s", taskID)
			case TaskStatusCancelled:
				return fmt.Errorf("task cancelled: %s", taskID)
			}
		}
	}
}

func (e *Engine) updateMetrics() {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var totalDuration time.Duration
	var totalQuality float64
	var completedCount int
	var totalTokens int

	for _, task := range e.tasks {
		task.mu.RLock()
		if task.Status == TaskStatusCompleted && task.Result != nil {
			completedCount++
			totalDuration += task.Result.Duration
			totalQuality += task.Result.Quality
			totalTokens += task.Result.TokensUsed
		}
		task.mu.RUnlock()
	}

	e.metrics.mu.Lock()
	e.metrics.TotalTokensUsed = totalTokens
	if completedCount > 0 {
		e.metrics.AvgTaskDuration = totalDuration / time.Duration(completedCount)
		e.metrics.AvgQualityScore = totalQuality / float64(completedCount)
	}
	e.metrics.mu.Unlock()
}

func (e *Engine) getModelForRole(role AgentRole) string {
	// Fast/cheap models for exploration and testing
	// High-quality models for coding and review
	switch role {
	case RoleResearcher:
		return "gemini-3.6-flash"
	case RoleCoder:
		return e.config.Model
	case RoleReviewer:
		return e.config.Model
	case RoleTester:
		return "gemini-3.6-flash"
	case RoleDocumenter:
		return "gemini-3.6-flash"
	default:
		return e.config.Model
	}
}

func (e *Engine) emit(eventType EventType, taskID, workerID string, payload any) {
	select {
	case e.events <- CoordinatorEvent{
		Type:      eventType,
		Timestamp: time.Now(),
		TaskID:    taskID,
		WorkerID:  workerID,
		Payload:   payload,
	}:
	default:
		// Channel full, drop event
	}
}

func (e *Engine) detectConflicts(results []*TaskResult) []*Conflict {
	var conflicts []*Conflict

	// Simple conflict detection: compare outputs for similar tasks
	artifactMap := make(map[string][]*TaskResult)
	for _, r := range results {
		for _, a := range r.Artifacts {
			if a.Path != "" {
				artifactMap[a.Path] = append(artifactMap[a.Path], r)
			}
		}
	}

	for path, rs := range artifactMap {
		if len(rs) > 1 {
			conflict := &Conflict{
				ID:          uuid.New().String(),
				Type:        ConflictTypeImplementation,
				Description: fmt.Sprintf("Multiple implementations for %s", path),
			}

			for _, r := range rs {
				conflict.TaskIDs = append(conflict.TaskIDs, r.TaskID)
				for _, a := range r.Artifacts {
					if a.Path == path {
						conflict.Options = append(conflict.Options, ConflictOption{
							TaskID:   r.TaskID,
							WorkerID: r.WorkerID,
							Content:  a.Content,
							Quality:  r.Quality,
						})
					}
				}
			}

			conflicts = append(conflicts, conflict)
			e.metrics.mu.Lock()
			e.metrics.ConflictCount++
			e.metrics.mu.Unlock()
		}
	}

	return conflicts
}

func (e *Engine) mergeResults(results []*TaskResult) string {
	if len(results) == 0 {
		return ""
	}

	// Sort by quality (descending)
	sorted := make([]*TaskResult, len(results))
	copy(sorted, results)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Quality > sorted[j].Quality
	})

	// Use the highest quality result as base
	var merged string
	for _, r := range sorted {
		if r.Output != "" {
			if merged == "" {
				merged = r.Output
			} else {
				merged += "\n\n---\n\n" + r.Output
			}
		}
	}

	return merged
}

func (e *Engine) calculateAverageQuality(results []*TaskResult) float64 {
	if len(results) == 0 {
		return 0
	}

	var total float64
	for _, r := range results {
		total += r.Quality
	}
	return total / float64(len(results))
}

func (e *Engine) calculateConsensus(results []*TaskResult) float64 {
	if len(results) < 2 {
		return 1.0
	}

	// Simple consensus: high quality, low conflicts
	avgQuality := e.calculateAverageQuality(results)
	conflicts := e.detectConflicts(results)
	conflictPenalty := float64(len(conflicts)) / float64(len(results))

	return avgQuality * (1 - conflictPenalty)
}

func (e *Engine) collectArtifacts(results []*TaskResult) []Artifact {
	var artifacts []Artifact
	seen := make(map[string]bool)

	for _, r := range results {
		for _, a := range r.Artifacts {
			key := fmt.Sprintf("%s:%s", a.Type, a.Path)
			if !seen[key] {
				seen[key] = true
				artifacts = append(artifacts, a)
			}
		}
	}

	return artifacts
}

func (e *Engine) generateSummary(results []*TaskResult) string {
	completed := 0
	failed := 0
	var totalQuality float64

	for _, r := range results {
		if r.Success {
			completed++
		} else {
			failed++
		}
		totalQuality += r.Quality
	}

	avgQuality := 0.0
	if len(results) > 0 {
		avgQuality = totalQuality / float64(len(results))
	}

	return fmt.Sprintf("Completed: %d, Failed: %d, Avg Quality: %.2f", completed, failed, avgQuality)
}

func matchesFilter(task *Task, filter TaskFilter) bool {
	task.mu.RLock()
	defer task.mu.RUnlock()

	if len(filter.Status) > 0 {
		match := false
		for _, s := range filter.Status {
			if task.Status == s {
				match = true
				break
			}
		}
		if !match {
			return false
		}
	}

	if len(filter.Role) > 0 {
		match := false
		for _, r := range filter.Role {
			if task.Role == r {
				match = true
				break
			}
		}
		if !match {
			return false
		}
	}

	if len(filter.Type) > 0 {
		match := false
		for _, t := range filter.Type {
			if task.Type == t {
				match = true
				break
			}
		}
		if !match {
			return false
		}
	}

	if len(filter.Priority) > 0 {
		match := false
		for _, p := range filter.Priority {
			if task.Priority == p {
				match = true
				break
			}
		}
		if !match {
			return false
		}
	}

	return true
}
