package builtin

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/iSundram/Automergent/internal/agent/agentdef"
)

// WorkflowPhase represents a phase in the coordinator workflow.
type WorkflowPhase string

const (
	PhaseResearch    WorkflowPhase = "research"
	PhaseSynthesis   WorkflowPhase = "synthesis"
	PhaseImplement   WorkflowPhase = "implement"
	PhaseVerify      WorkflowPhase = "verify"
	PhaseComplete    WorkflowPhase = "complete"
)

// TaskSpec describes a task to be spawned by the coordinator.
type TaskSpec struct {
	ID           string
	Phase        WorkflowPhase
	AgentType    agentdef.AgentType
	Prompt       string
	Name         string
	Description  string
	DependsOn    []string
	Parallel     bool
	Model        string
}

// WorkflowState tracks the state of a coordinator workflow.
type WorkflowState struct {
	mu          sync.RWMutex
	ID          string
	OriginalTask string
	Phase       WorkflowPhase
	Tasks       map[string]*TaskSpec
	Results     map[string]string
	Errors      map[string]string
	StartedAt   time.Time
	PhaseAt     time.Time
	Log         []WorkflowLogEntry
}

// WorkflowLogEntry is a log entry in the workflow.
type WorkflowLogEntry struct {
	Timestamp time.Time
	Phase     WorkflowPhase
	Message   string
	Level     string // "info", "warn", "error"
}

// NewWorkflowState creates a new workflow state.
func NewWorkflowState(id, originalTask string) *WorkflowState {
	return &WorkflowState{
		ID:           id,
		OriginalTask: originalTask,
		Phase:        PhaseResearch,
		Tasks:        make(map[string]*TaskSpec),
		Results:      make(map[string]string),
		Errors:       make(map[string]string),
		StartedAt:    time.Now(),
		PhaseAt:      time.Now(),
		Log:          make([]WorkflowLogEntry, 0),
	}
}

// AddTask adds a task to the workflow.
func (ws *WorkflowState) AddTask(spec *TaskSpec) {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	ws.Tasks[spec.ID] = spec
}

// CompleteTask marks a task as completed with its result.
func (ws *WorkflowState) CompleteTask(id, result string) {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	ws.Results[id] = result
}

// FailTask marks a task as failed.
func (ws *WorkflowState) FailTask(id, errMsg string) {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	ws.Errors[id] = errMsg
}

// AdvancePhase moves to the next phase.
func (ws *WorkflowState) AdvancePhase(next WorkflowPhase) {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	ws.Phase = next
	ws.PhaseAt = time.Now()
}

// Logf adds a log entry.
func (ws *WorkflowState) Logf(phase WorkflowPhase, level, format string, args ...any) {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	ws.Log = append(ws.Log, WorkflowLogEntry{
		Timestamp: time.Now(),
		Phase:     phase,
		Level:     level,
		Message:   fmt.Sprintf(format, args...),
	})
}

// GetReadyTasks returns tasks whose dependencies are met.
func (ws *WorkflowState) GetReadyTasks() []*TaskSpec {
	ws.mu.RLock()
	defer ws.mu.RUnlock()

	completed := make(map[string]bool)
	for id := range ws.Results {
		completed[id] = true
	}

	var ready []*TaskSpec
	for _, spec := range ws.Tasks {
		if _, done := ws.Results[spec.ID]; done {
			continue
		}
		if _, failed := ws.Errors[spec.ID]; failed {
			continue
		}
		depsMet := true
		for _, dep := range spec.DependsOn {
			if !completed[dep] {
				depsMet = false
				break
			}
		}
		if depsMet {
			ready = append(ready, spec)
		}
	}
	return ready
}

// IsComplete returns true if all tasks are done.
func (ws *WorkflowState) IsComplete() bool {
	ws.mu.RLock()
	defer ws.mu.RUnlock()
	return len(ws.Results)+len(ws.Errors) >= len(ws.Tasks) && len(ws.Tasks) > 0
}

// CoordinatorWorkflow implements the coordinator's orchestration logic.
type CoordinatorWorkflow struct {
	state  *WorkflowState
	spawn  func(ctx context.Context, spec TaskSpec) (string, error)
}

// NewCoordinatorWorkflow creates a new coordinator workflow.
func NewCoordinatorWorkflow(
	originalTask string,
	spawnFn func(ctx context.Context, spec TaskSpec) (string, error),
) *CoordinatorWorkflow {
	return &CoordinatorWorkflow{
		state: NewWorkflowState(fmt.Sprintf("wf-%d", time.Now().UnixNano()), originalTask),
		spawn: spawnFn,
	}
}

// DecomposeTask breaks the original task into phases with specific subtasks.
func (cw *CoordinatorWorkflow) DecomposeTask() {
	task := cw.state.OriginalTask

	// Research phase
	cw.state.AddTask(&TaskSpec{
		ID:        "research-1",
		Phase:     PhaseResearch,
		AgentType: agentdef.AgentTypeExplore,
		Prompt:    fmt.Sprintf("Explore the codebase to understand: %s\n\nFind relevant files, patterns, and architecture. Report findings with file paths and line numbers.", task),
		Name:      "research",
		Parallel:  false,
	})

	// Implementation phase (depends on research)
	cw.state.AddTask(&TaskSpec{
		ID:        "implement-1",
		Phase:     PhaseImplement,
		AgentType: agentdef.AgentTypeGeneral,
		Prompt:    fmt.Sprintf("Implement the following based on research findings: %s\n\nUse the research results to make targeted changes.", task),
		Name:      "implement",
		DependsOn: []string{"research-1"},
		Parallel:  false,
	})

	// Verification phase (depends on implementation)
	cw.state.AddTask(&TaskSpec{
		ID:        "verify-1",
		Phase:     PhaseVerify,
		AgentType: agentdef.AgentTypeReview,
		Prompt:    fmt.Sprintf("Review the changes made for: %s\n\nCheck for bugs, security issues, and verify the implementation is correct.", task),
		Name:      "verify",
		DependsOn: []string{"implement-1"},
		Parallel:  false,
	})
}

// Run executes the workflow, processing tasks as dependencies are met.
func (cw *CoordinatorWorkflow) Run(ctx context.Context) error {
	cw.DecomposeTask()

	for !cw.state.IsComplete() {
		ready := cw.state.GetReadyTasks()
		if len(ready) == 0 {
			break
		}

		for _, spec := range ready {
			cw.state.Logf(spec.Phase, "info", "Starting task: %s", spec.Name)

			result, err := cw.spawn(ctx, *spec)
			if err != nil {
				cw.state.FailTask(spec.ID, err.Error())
				cw.state.Logf(spec.Phase, "error", "Task %s failed: %v", spec.Name, err)
				continue
			}

			cw.state.CompleteTask(spec.ID, result)
			cw.state.Logf(spec.Phase, "info", "Task %s completed", spec.Name)

			// Advance phase based on completed task
			switch spec.Phase {
			case PhaseResearch:
				cw.state.AdvancePhase(PhaseSynthesis)
			case PhaseSynthesis:
				cw.state.AdvancePhase(PhaseImplement)
			case PhaseImplement:
				cw.state.AdvancePhase(PhaseVerify)
			case PhaseVerify:
				cw.state.AdvancePhase(PhaseComplete)
			}
		}
	}

	return nil
}

// GetState returns the current workflow state.
func (cw *CoordinatorWorkflow) GetState() *WorkflowState {
	return cw.state
}

// SynthesizeResults combines results from all completed tasks.
func (cw *CoordinatorWorkflow) SynthesizeResults() string {
	cw.state.mu.RLock()
	defer cw.state.mu.RUnlock()

	var sb strings.Builder
	sb.WriteString("## Workflow Results\n\n")
	sb.WriteString(fmt.Sprintf("Task: %s\n", cw.state.OriginalTask))
	sb.WriteString(fmt.Sprintf("Phase: %s\n", cw.state.Phase))
	sb.WriteString(fmt.Sprintf("Duration: %s\n\n", time.Since(cw.state.StartedAt).Truncate(time.Second)))

	sb.WriteString("### Results\n")
	for id, result := range cw.state.Results {
		sb.WriteString(fmt.Sprintf("\n**%s**:\n%s\n", id, truncate(result, 500)))
	}

	if len(cw.state.Errors) > 0 {
		sb.WriteString("\n### Errors\n")
		for id, err := range cw.state.Errors {
			sb.WriteString(fmt.Sprintf("- %s: %s\n", id, err))
		}
	}

	return sb.String()
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
