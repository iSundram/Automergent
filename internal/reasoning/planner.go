package reasoning

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Planner creates structured execution plans from task analysis.
type Planner struct {
	decomposers map[TaskType]Decomposer
}

// Decomposer breaks down tasks into subtasks.
type Decomposer interface {
	Decompose(ctx context.Context, analysis *TaskAnalysis) ([]*Task, error)
}

// NewPlanner creates a new task planner.
func NewPlanner() *Planner {
	p := &Planner{
		decomposers: make(map[TaskType]Decomposer),
	}

	// Register default decomposers
	p.registerDefaultDecomposers()

	return p
}

// CreatePlan generates a complete execution plan.
func (p *Planner) CreatePlan(ctx context.Context, analysis *TaskAnalysis, strategies map[TaskType]Strategy) (*ExecutionPlan, error) {
	plan := &ExecutionPlan{
		ID:        generateID(),
		Analysis:  analysis,
		Tasks:     []*Task{},
		Metadata:  make(map[string]string),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// Decompose into tasks
	tasks, err := p.decompose(ctx, analysis, strategies)
	if err != nil {
		return nil, fmt.Errorf("decomposition failed: %w", err)
	}
	plan.Tasks = tasks

	// Determine execution order (topological sort with parallelization)
	plan.ExecutionOrder = p.determineExecutionOrder(tasks)

	// Create verification checkpoints
	plan.Checkpoints = p.createCheckpoints(analysis, tasks)

	// Add metadata
	plan.Metadata["task_count"] = fmt.Sprintf("%d", len(tasks))
	plan.Metadata["estimated_duration"] = analysis.EstimatedTime.String()
	plan.Metadata["complexity"] = string(analysis.Complexity)

	return plan, nil
}

// decompose breaks down the analysis into executable tasks.
func (p *Planner) decompose(ctx context.Context, analysis *TaskAnalysis, strategies map[TaskType]Strategy) ([]*Task, error) {
	// Use strategy if available
	if strategy, ok := strategies[analysis.TaskType]; ok {
		return strategy.Decompose(ctx, analysis)
	}

	// Use decomposer if available
	if decomposer, ok := p.decomposers[analysis.TaskType]; ok {
		return decomposer.Decompose(ctx, analysis)
	}

	// Fallback to generic decomposition
	return p.genericDecompose(analysis)
}

// genericDecompose provides a basic task breakdown for any task type.
func (p *Planner) genericDecompose(analysis *TaskAnalysis) ([]*Task, error) {
	tasks := []*Task{}

	// Phase 1: Context gathering (always needed)
	if len(analysis.RequiredFiles) > 0 || analysis.Scope != ScopeSingleFile {
		tasks = append(tasks, &Task{
			ID:           generateID(),
			Description:  "Gather required context and files",
			Type:         TaskTypeInvestigation,
			Dependencies: []string{},
			Parallel:     false,
			Priority:     100,
			Estimated:    5 * time.Minute,
			Tools:        []string{"grep", "glob", "read"},
			Context:      map[string]string{"phase": "context_gathering"},
			Verification: []Checkpoint{
				{
					ID:          generateID(),
					Description: "Required files identified",
					Type:        CheckpointSemantic,
					Validator:   "file_check",
					Required:    true,
				},
			},
			Status:    TaskStatusPending,
			CreatedAt: time.Now(),
		})
	}

	// Phase 2: Main work task
	mainTask := &Task{
		ID:           generateID(),
		Description:  analysis.Intent,
		Type:         analysis.TaskType,
		Dependencies: []string{},
		Parallel:     false,
		Priority:     90,
		Estimated:    analysis.EstimatedTime - (10 * time.Minute), // Reserve time for verification
		Tools:        p.selectToolsForTask(analysis.TaskType),
		Context:      map[string]string{"phase": "execution"},
		Verification: []Checkpoint{
			{
				ID:          generateID(),
				Description: "Task objective achieved",
				Type:        CheckpointSemantic,
				Validator:   "objective_check",
				Required:    true,
			},
		},
		Status:    TaskStatusPending,
		CreatedAt: time.Now(),
	}

	// Add dependency on context gathering if it exists
	if len(tasks) > 0 {
		mainTask.Dependencies = []string{tasks[0].ID}
	}

	tasks = append(tasks, mainTask)

	// Phase 3: Verification (for code changes)
	if p.requiresVerification(analysis.TaskType) {
		verifyTask := &Task{
			ID:           generateID(),
			Description:  "Verify changes and run tests",
			Type:         TaskTypeTest,
			Dependencies: []string{mainTask.ID},
			Parallel:     false,
			Priority:     80,
			Estimated:    5 * time.Minute,
			Tools:        []string{"bash"},
			Context:      map[string]string{"phase": "verification"},
			Verification: []Checkpoint{
				{
					ID:          generateID(),
					Description: "All tests pass",
					Type:        CheckpointTest,
					Validator:   "test_runner",
					Required:    true,
				},
			},
			Status:    TaskStatusPending,
			CreatedAt: time.Now(),
		}
		tasks = append(tasks, verifyTask)
	}

	return tasks, nil
}

// determineExecutionOrder creates groups of tasks that can run in parallel.
func (p *Planner) determineExecutionOrder(tasks []*Task) [][]string {
	order := [][]string{}

	// Build dependency graph
	taskMap := make(map[string]*Task)
	for _, task := range tasks {
		taskMap[task.ID] = task
	}

	// Topological sort with parallelization
	completed := make(map[string]bool)

	for len(completed) < len(tasks) {
		// Find tasks ready to execute (no pending dependencies)
		ready := []string{}

		for _, task := range tasks {
			if completed[task.ID] {
				continue
			}

			// Check if all dependencies are complete
			canExecute := true
			for _, depID := range task.Dependencies {
				if !completed[depID] {
					canExecute = false
					break
				}
			}

			if canExecute {
				ready = append(ready, task.ID)
			}
		}

		if len(ready) == 0 {
			// Circular dependency or error - break with remaining tasks in sequence
			for _, task := range tasks {
				if !completed[task.ID] {
					order = append(order, []string{task.ID})
					completed[task.ID] = true
				}
			}
			break
		}

		// Group parallel tasks
		parallelGroup := []string{}
		for _, taskID := range ready {
			task := taskMap[taskID]
			if task.Parallel && len(parallelGroup) > 0 {
				parallelGroup = append(parallelGroup, taskID)
			} else {
				if len(parallelGroup) > 0 {
					order = append(order, parallelGroup)
					parallelGroup = []string{}
				}
				parallelGroup = append(parallelGroup, taskID)
			}
			completed[taskID] = true
		}

		if len(parallelGroup) > 0 {
			order = append(order, parallelGroup)
		}
	}

	return order
}

// createCheckpoints generates verification checkpoints for the plan.
func (p *Planner) createCheckpoints(analysis *TaskAnalysis, tasks []*Task) []Checkpoint {
	checkpoints := []Checkpoint{}

	// Always check syntax for code changes
	if p.isCodeChange(analysis.TaskType) {
		checkpoints = append(checkpoints, Checkpoint{
			ID:          generateID(),
			Description: "Code syntax is valid",
			Type:        CheckpointSyntax,
			Validator:   "syntax_check",
			Required:    true,
		})
	}

	// Semantic verification for complex tasks
	if analysis.Complexity == ComplexityComplex || analysis.Complexity == ComplexityMajor {
		checkpoints = append(checkpoints, Checkpoint{
			ID:          generateID(),
			Description: "Logic is semantically correct",
			Type:        CheckpointSemantic,
			Validator:   "semantic_check",
			Required:    true,
		})
	}

	// Test execution for critical changes
	if p.requiresVerification(analysis.TaskType) {
		checkpoints = append(checkpoints, Checkpoint{
			ID:          generateID(),
			Description: "All tests pass",
			Type:        CheckpointTest,
			Validator:   "test_runner",
			Required:    true,
		})
	}

	// Integration check for multi-file changes
	if analysis.Scope == ScopeMultiFile || analysis.Scope == ScopeProjectWide {
		checkpoints = append(checkpoints, Checkpoint{
			ID:          generateID(),
			Description: "Changes integrate correctly",
			Type:        CheckpointIntegration,
			Validator:   "integration_check",
			Required:    false,
		})
	}

	return checkpoints
}

// selectToolsForTask recommends tools based on task type.
func (p *Planner) selectToolsForTask(taskType TaskType) []string {
	switch taskType {
	case TaskTypeBugFix:
		return []string{"grep", "read", "edit", "bash"}
	case TaskTypeFeature:
		return []string{"create", "edit", "bash"}
	case TaskTypeRefactor:
		return []string{"read", "edit", "bash"}
	case TaskTypeDocumentation:
		return []string{"read", "create", "edit"}
	case TaskTypeInvestigation:
		return []string{"grep", "glob", "read", "bash"}
	case TaskTypeTest:
		return []string{"create", "edit", "bash"}
	default:
		return []string{"read", "edit"}
	}
}

// requiresVerification determines if task type needs test verification.
func (p *Planner) requiresVerification(taskType TaskType) bool {
	return taskType == TaskTypeBugFix ||
		taskType == TaskTypeFeature ||
		taskType == TaskTypeRefactor
}

// isCodeChange determines if task modifies code.
func (p *Planner) isCodeChange(taskType TaskType) bool {
	return taskType != TaskTypeInvestigation &&
		taskType != TaskTypeDocumentation
}

// registerDefaultDecomposers sets up task-specific decomposers.
func (p *Planner) registerDefaultDecomposers() {
	// Bug fix decomposer
	p.decomposers[TaskTypeBugFix] = &BugFixDecomposer{}

	// Feature decomposer
	p.decomposers[TaskTypeFeature] = &FeatureDecomposer{}

	// Refactor decomposer
	p.decomposers[TaskTypeRefactor] = &RefactorDecomposer{}
}

// BugFixDecomposer breaks down bug fix tasks.
type BugFixDecomposer struct{}

func (d *BugFixDecomposer) Decompose(ctx context.Context, analysis *TaskAnalysis) ([]*Task, error) {
	tasks := []*Task{
		{
			ID:          generateID(),
			Description: "Reproduce and diagnose the bug",
			Type:        TaskTypeInvestigation,
			Priority:    100,
			Estimated:   10 * time.Minute,
			Tools:       []string{"grep", "read", "bash"},
			Status:      TaskStatusPending,
			CreatedAt:   time.Now(),
		},
		{
			ID:          generateID(),
			Description: "Implement the fix",
			Type:        TaskTypeBugFix,
			Priority:    90,
			Estimated:   15 * time.Minute,
			Tools:       []string{"edit"},
			Status:      TaskStatusPending,
			CreatedAt:   time.Now(),
		},
		{
			ID:          generateID(),
			Description: "Verify fix and run tests",
			Type:        TaskTypeTest,
			Priority:    80,
			Estimated:   10 * time.Minute,
			Tools:       []string{"bash"},
			Status:      TaskStatusPending,
			CreatedAt:   time.Now(),
		},
	}

	// Set dependencies
	tasks[1].Dependencies = []string{tasks[0].ID}
	tasks[2].Dependencies = []string{tasks[1].ID}

	return tasks, nil
}

// FeatureDecomposer breaks down feature tasks.
type FeatureDecomposer struct{}

func (d *FeatureDecomposer) Decompose(ctx context.Context, analysis *TaskAnalysis) ([]*Task, error) {
	tasks := []*Task{
		{
			ID:          generateID(),
			Description: "Research existing patterns and dependencies",
			Type:        TaskTypeInvestigation,
			Priority:    100,
			Estimated:   10 * time.Minute,
			Tools:       []string{"grep", "glob", "read"},
			Status:      TaskStatusPending,
			CreatedAt:   time.Now(),
		},
		{
			ID:          generateID(),
			Description: "Implement core feature logic",
			Type:        TaskTypeFeature,
			Priority:    90,
			Estimated:   20 * time.Minute,
			Tools:       []string{"create", "edit"},
			Status:      TaskStatusPending,
			CreatedAt:   time.Now(),
		},
		{
			ID:          generateID(),
			Description: "Add tests for new feature",
			Type:        TaskTypeTest,
			Priority:    80,
			Estimated:   15 * time.Minute,
			Tools:       []string{"create", "bash"},
			Status:      TaskStatusPending,
			CreatedAt:   time.Now(),
		},
	}

	tasks[1].Dependencies = []string{tasks[0].ID}
	tasks[2].Dependencies = []string{tasks[1].ID}

	return tasks, nil
}

// RefactorDecomposer breaks down refactoring tasks.
type RefactorDecomposer struct{}

func (d *RefactorDecomposer) Decompose(ctx context.Context, analysis *TaskAnalysis) ([]*Task, error) {
	tasks := []*Task{
		{
			ID:          generateID(),
			Description: "Analyze current implementation",
			Type:        TaskTypeInvestigation,
			Priority:    100,
			Estimated:   15 * time.Minute,
			Tools:       []string{"grep", "read"},
			Status:      TaskStatusPending,
			CreatedAt:   time.Now(),
		},
		{
			ID:          generateID(),
			Description: "Identify test coverage",
			Type:        TaskTypeTest,
			Priority:    95,
			Estimated:   10 * time.Minute,
			Tools:       []string{"bash"},
			Status:      TaskStatusPending,
			CreatedAt:   time.Now(),
		},
		{
			ID:          generateID(),
			Description: "Perform refactoring",
			Type:        TaskTypeRefactor,
			Priority:    90,
			Estimated:   25 * time.Minute,
			Tools:       []string{"edit"},
			Status:      TaskStatusPending,
			CreatedAt:   time.Now(),
		},
		{
			ID:          generateID(),
			Description: "Verify behavior preservation",
			Type:        TaskTypeTest,
			Priority:    80,
			Estimated:   10 * time.Minute,
			Tools:       []string{"bash"},
			Status:      TaskStatusPending,
			CreatedAt:   time.Now(),
		},
	}

	tasks[1].Dependencies = []string{tasks[0].ID}
	tasks[2].Dependencies = []string{tasks[0].ID, tasks[1].ID}
	tasks[3].Dependencies = []string{tasks[2].ID}

	return tasks, nil
}

// generateID creates a unique identifier.
func generateID() string {
	return uuid.New().String()
}
