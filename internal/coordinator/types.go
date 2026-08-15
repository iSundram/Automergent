// Package coordinator provides a multi-agent coordination system for parallel
// task distribution, result synthesis, and intelligent load balancing.
package coordinator

import (
	"context"
	"sync"
	"time"

	"github.com/iSundram/Automergent/internal/ai"
)

// AgentRole defines the specialized role of a worker agent.
// Role "" means any worker may take the task.
type AgentRole string

const (
	RoleResearcher AgentRole = "researcher"
	RoleCoder      AgentRole = "coder"
	RoleReviewer   AgentRole = "reviewer"
	RoleTester     AgentRole = "tester"
	RoleDocumenter AgentRole = "documenter"
)

// AllRoles returns all available agent roles.
func AllRoles() []AgentRole {
	return []AgentRole{RoleResearcher, RoleCoder, RoleReviewer, RoleTester, RoleDocumenter}
}

// TaskPriority defines the urgency of a task.
type TaskPriority int

const (
	PriorityLow      TaskPriority = 1
	PriorityNormal   TaskPriority = 2
	PriorityHigh     TaskPriority = 3
	PriorityCritical TaskPriority = 4
)

// TaskStatus represents the current state of a task.
type TaskStatus string

const (
	TaskStatusPending   TaskStatus = "pending"
	TaskStatusQueued    TaskStatus = "queued"
	TaskStatusRunning   TaskStatus = "running"
	TaskStatusCompleted TaskStatus = "completed"
	TaskStatusFailed    TaskStatus = "failed"
	TaskStatusCancelled TaskStatus = "cancelled"
	TaskStatusRetrying  TaskStatus = "retrying"
)

// WorkerStatus represents the current state of a worker.
type WorkerStatus string

const (
	WorkerStatusIdle     WorkerStatus = "idle"
	WorkerStatusBusy     WorkerStatus = "busy"
	WorkerStatusStopping WorkerStatus = "stopping"
	WorkerStatusStopped  WorkerStatus = "stopped"
)

// Task represents a unit of work for the coordinator.
type Task struct {
	ID           string
	Type         TaskType
	Role         AgentRole
	Priority     TaskPriority
	Prompt       string
	Context      TaskContext
	Dependencies []string
	MaxRetries   int
	Timeout      time.Duration
	Metadata     map[string]any
	CreatedAt    time.Time
	StartedAt    time.Time
	CompletedAt  time.Time
	Status       TaskStatus
	Result       *TaskResult
	Retries      int
	inQueue      bool
	completionCh chan struct{}
	ctx          context.Context    // task-scoped cancellable context
	cancel       context.CancelFunc // cancels ctx on preemption
	mu           sync.RWMutex
}

// AgentRole defines the specialized role of a worker agent.
// Role "" means any worker may take the task.

// TaskType categorizes the kind of work to be done.
type TaskType string

const (
	TaskTypeExplore    TaskType = "explore"
	TaskTypeImplement  TaskType = "implement"
	TaskTypeReview     TaskType = "review"
	TaskTypeTest       TaskType = "test"
	TaskTypeDocument   TaskType = "document"
	TaskTypeRefactor   TaskType = "refactor"
	TaskTypeDebug      TaskType = "debug"
	TaskTypeSynthesize TaskType = "synthesize"
)

// TaskContext provides additional context for task execution.
type TaskContext struct {
	WorkingDir   string
	Files        []string
	CodeSnippets map[string]string
	Messages     []ai.Message
	ParentTaskID string
	ProjectInfo  map[string]any
	Constraints  []string
}

// TaskResult holds the outcome of a completed task.
type TaskResult struct {
	TaskID     string
	WorkerID   string
	Success    bool
	Output     string
	Artifacts  []Artifact
	Errors     []string
	Warnings   []string
	Quality    float64 // 0.0 to 1.0 quality score
	Confidence float64 // 0.0 to 1.0 confidence score
	TokensUsed int
	Duration   time.Duration
	Metadata   map[string]any
}

// Artifact represents a produced item (file, code, etc).
type Artifact struct {
	Type     ArtifactType
	Path     string
	Content  string
	Language string
	Checksum string
	Metadata map[string]any
}

// ArtifactType categorizes produced artifacts.
type ArtifactType string

const (
	ArtifactTypeCode     ArtifactType = "code"
	ArtifactTypeTest     ArtifactType = "test"
	ArtifactTypeDoc      ArtifactType = "doc"
	ArtifactTypeConfig   ArtifactType = "config"
	ArtifactTypeAnalysis ArtifactType = "analysis"
)

// Worker represents a specialized agent worker.
type Worker struct {
	ID           string
	Role         AgentRole
	Status       WorkerStatus
	CurrentTask  *Task
	TasksHandled int
	TokensUsed   int
	StartedAt    time.Time
	LastActiveAt time.Time
	Model        string
	Metrics      WorkerMetrics
	mu           sync.RWMutex
}

// WorkerMetrics tracks worker performance.
type WorkerMetrics struct {
	TasksCompleted int
	TasksFailed    int
	TotalDuration  time.Duration
	AvgDuration    time.Duration
	SuccessRate    float64
	QualityAvg     float64
}

// CoordinatorConfig configures the coordinator behavior.
type CoordinatorConfig struct {
	MaxWorkers         int
	WorkersPerRole     map[AgentRole]int
	ModelOverrides     map[AgentRole]string // role → model override
	DefaultTimeout     time.Duration
	MaxRetries         int
	EnableWorkStealing bool
	QualityThreshold   float64
	ConsensusThreshold int
	ResourceLimits     ResourceLimits
	Model              string
	FallbackModel      string
	EventsBufferSize   int
}

// ResourceLimits defines resource constraints per agent.
type ResourceLimits struct {
	MaxTokensPerTask   int
	MaxConcurrentTasks int
	MaxMemoryMB        int
	RateLimitPerMinute int
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() *CoordinatorConfig {
	return &CoordinatorConfig{
		MaxWorkers: 10,
		WorkersPerRole: map[AgentRole]int{
			RoleResearcher: 3,
			RoleCoder:      2,
			RoleReviewer:   2,
			RoleTester:     2,
			RoleDocumenter: 1,
		},
		DefaultTimeout:     5 * time.Minute,
		MaxRetries:         3,
		EnableWorkStealing: true,
		QualityThreshold:   0.7,
		ConsensusThreshold: 2,
		ResourceLimits: ResourceLimits{
			MaxTokensPerTask:   100000,
			MaxConcurrentTasks: 5,
			MaxMemoryMB:        512,
			RateLimitPerMinute: 60,
		},
		EventsBufferSize: 1024,
	}
}

// CoordinatorEvent represents events emitted by the coordinator.
type CoordinatorEvent struct {
	Type      EventType
	Timestamp time.Time
	TaskID    string
	WorkerID  string
	Payload   any
}

// EventType categorizes coordinator events.
type EventType string

const (
	EventTaskQueued     EventType = "task_queued"
	EventTaskStarted    EventType = "task_started"
	EventTaskCompleted  EventType = "task_completed"
	EventTaskFailed     EventType = "task_failed"
	EventTaskRetrying   EventType = "task_retrying"
	EventWorkerStarted  EventType = "worker_started"
	EventWorkerStopped  EventType = "worker_stopped"
	EventWorkerStolen   EventType = "worker_stolen"
	EventSynthesisStart EventType = "synthesis_start"
	EventSynthesisDone  EventType = "synthesis_done"
	EventConflict       EventType = "conflict_detected"
	EventConsensus      EventType = "consensus_reached"
	EventPhaseStart     EventType = "phase_start"
	EventPhaseComplete  EventType = "phase_complete"
)

// SynthesisResult holds the output of result aggregation.
type SynthesisResult struct {
	FinalOutput     string
	SelectedResults []*TaskResult
	ConflictCount   int
	QualityScore    float64
	ConsensusLevel  float64
	Artifacts       []Artifact
	Summary         string
}

// Conflict represents a disagreement between task results.
type Conflict struct {
	ID          string
	TaskIDs     []string
	Type        ConflictType
	Description string
	Options     []ConflictOption
	Resolution  *ConflictResolution
}

// ConflictType categorizes conflicts.
type ConflictType string

const (
	ConflictTypeCodeStyle      ConflictType = "code_style"
	ConflictTypeImplementation ConflictType = "implementation"
	ConflictTypeArchitecture   ConflictType = "architecture"
	ConflictTypeNaming         ConflictType = "naming"
	ConflictTypeTest           ConflictType = "test"
)

// ConflictOption represents one way to resolve a conflict.
type ConflictOption struct {
	TaskID    string
	WorkerID  string
	Content   string
	Quality   float64
	Rationale string
}

// ConflictResolution records how a conflict was resolved.
type ConflictResolution struct {
	Strategy     ResolutionStrategy
	ChosenOption *ConflictOption
	Reasoning    string
	ResolvedAt   time.Time
}

// ResolutionStrategy defines how conflicts are resolved.
type ResolutionStrategy string

const (
	ResolutionByQuality   ResolutionStrategy = "quality"
	ResolutionByConsensus ResolutionStrategy = "consensus"
	ResolutionByRecency   ResolutionStrategy = "recency"
	ResolutionByPriority  ResolutionStrategy = "priority"
	ResolutionManual      ResolutionStrategy = "manual"
)

// ExecutionPlan describes how tasks will be executed.
type ExecutionPlan struct {
	ID                string
	Tasks             []*Task
	Phases            []ExecutionPhase
	Dependencies      map[string][]string
	EstimatedDuration time.Duration
	CreatedAt         time.Time
}

// ExecutionPhase groups tasks that can run in parallel.
type ExecutionPhase struct {
	Index    int
	Name     string
	TaskIDs  []string
	Parallel bool
}

// Coordinator is the interface for the multi-agent coordinator.
type Coordinator interface {
	// Lifecycle
	Start(ctx context.Context) error
	Stop(ctx context.Context) error

	// Task management
	Submit(task *Task) error
	SubmitBatch(tasks []*Task) error
	Cancel(taskID string) error
	GetTask(taskID string) (*Task, error)
	ListTasks(filter TaskFilter) []*Task

	// Worker management
	ScaleWorkers(role AgentRole, count int) error
	GetWorker(workerID string) (*Worker, error)
	ListWorkers() []*Worker

	// Execution
	Execute(ctx context.Context, plan *ExecutionPlan) (*SynthesisResult, error)
	ExecuteParallel(ctx context.Context, tasks []*Task) ([]*TaskResult, error)

	// Synthesis
	Synthesize(ctx context.Context, results []*TaskResult) (*SynthesisResult, error)
	ResolveConflicts(ctx context.Context, conflicts []*Conflict) ([]*ConflictResolution, error)

	// Events
	Events() <-chan CoordinatorEvent

	// Metrics
	Metrics() *CoordinatorMetrics
}

// TaskFilter for querying tasks.
type TaskFilter struct {
	Status   []TaskStatus
	Role     []AgentRole
	Type     []TaskType
	Priority []TaskPriority
	Limit    int
}

// CoordinatorMetrics tracks overall coordinator performance.
type CoordinatorMetrics struct {
	ActiveWorkers   int
	PendingTasks    int
	RunningTasks    int
	CompletedTasks  int
	FailedTasks     int
	TotalTokensUsed int
	AvgTaskDuration time.Duration
	AvgQualityScore float64
	WorkStealCount  int
	ConflictCount   int
	ConsensusCount  int
	mu              sync.RWMutex
}

// AgentExecutor interface for running agent tasks.
type AgentExecutor interface {
	Execute(ctx context.Context, role AgentRole, prompt string, context TaskContext, model string) (*TaskResult, error)
}
