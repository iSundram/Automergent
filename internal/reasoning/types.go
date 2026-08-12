package reasoning

import (
	"context"
	"time"

	"github.com/iSundram/Automergent/internal/verification"
)

// TaskType categorizes the type of task to execute.
type TaskType string

const (
	TaskTypeBugFix        TaskType = "bug_fix"
	TaskTypeFeature       TaskType = "feature"
	TaskTypeRefactor      TaskType = "refactor"
	TaskTypeDocumentation TaskType = "documentation"
	TaskTypeInvestigation TaskType = "investigation"
	TaskTypeTest          TaskType = "test"
	TaskTypeMultiFile     TaskType = "multi_file"
	TaskTypeProjectWide   TaskType = "project_wide"
	TaskTypeBuild         TaskType = "build"
	TaskTypeDeployment    TaskType = "deployment"
)

// Scope defines the breadth of the task.
type Scope string

const (
	ScopeSingleFile  Scope = "single_file"
	ScopeMultiFile   Scope = "multi_file"
	ScopeProjectWide Scope = "project_wide"
	ScopeExternal    Scope = "external" // requires external APIs, docs, etc.
)

// Complexity estimates task difficulty.
type Complexity string

const (
	ComplexityTrivial  Complexity = "trivial"  // <5 min, single action
	ComplexitySimple   Complexity = "simple"   // 5-15 min, few steps
	ComplexityModerate Complexity = "moderate" // 15-60 min, multiple phases
	ComplexityComplex  Complexity = "complex"  // 1-4 hours, deep analysis
	ComplexityMajor    Complexity = "major"    // 4+ hours, architecture changes
)

// Phase represents the execution stage.
type Phase string

const (
	PhaseAnalysis     Phase = "analysis"
	PhasePlanning     Phase = "planning"
	PhaseExecution    Phase = "execution"
	PhaseVerification Phase = "verification"
	PhaseComplete     Phase = "complete"
	PhaseFailed       Phase = "failed"
)

// TaskAnalysis represents the result of analyzing a user request.
type TaskAnalysis struct {
	Intent        string            // User's primary goal
	TaskType      TaskType          // Classification
	Scope         Scope             // Breadth of work
	Complexity    Complexity        // Difficulty estimate
	EstimatedTime time.Duration     // Rough time estimate
	RequiredFiles []string          // Files needed for context
	Dependencies  []string          // External dependencies
	Risks         []string          // Potential issues
	Assumptions   []string          // Key assumptions made
	Confidence    float64           // Confidence in analysis (0.0-1.0)
	Metadata      map[string]string // Additional context
	AnalyzedAt    time.Time
}

// ReasoningTrace captures the thinking process.
type ReasoningTrace struct {
	Steps     []ReasoningStep
	StartedAt time.Time
	Duration  time.Duration
}

// ReasoningStep represents one logical thought.
type ReasoningStep struct {
	Phase     Phase
	Thought   string
	Action    string
	Result    string
	Timestamp time.Time
}

// Task represents a unit of work in the execution plan.
type Task struct {
	ID                 string            // Unique identifier
	Description        string            // What to do
	Type               TaskType          // Task classification
	Dependencies       []string          // Task IDs that must complete first
	Parallel           bool              // Can run in parallel with siblings
	Priority           int               // Higher = more important
	Estimated          time.Duration     // Time estimate
	Tools              []string          // Required tools
	Context            map[string]string // Task-specific context
	Verification       []Checkpoint      // How to verify completion
	Status             TaskStatus
	Result             *TaskResult
	CreatedAt          time.Time
	VerificationResult *verification.Result `json:"verification_result,omitempty"`
}

// TaskStatus tracks execution state.
type TaskStatus string

const (
	TaskStatusPending    TaskStatus = "pending"
	TaskStatusInProgress TaskStatus = "in_progress"
	TaskStatusBlocked    TaskStatus = "blocked"
	TaskStatusComplete   TaskStatus = "complete"
	TaskStatusFailed     TaskStatus = "failed"
	TaskStatusSkipped    TaskStatus = "skipped"
)

// TaskResult stores execution outcome.
type TaskResult struct {
	Success      bool
	Output       string
	Error        error
	Attempts     int
	Duration     time.Duration
	ToolsUsed    []string
	FilesChanged []string
	CompletedAt  time.Time
}

// ExecutionPlan is a structured breakdown of work.
type ExecutionPlan struct {
	ID             string
	Analysis       *TaskAnalysis
	Confidence     float64
	Tasks          []*Task
	ExecutionOrder [][]string // Groups of parallel tasks in sequence
	Checkpoints    []Checkpoint
	Metadata       map[string]string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// Checkpoint defines a verification point.
type Checkpoint struct {
	ID          string
	Description string
	Type        CheckpointType
	Validator   string // Tool or method to use for validation
	Required    bool   // Must pass to continue
	Passed      *bool  // nil = not run, true/false = result
}

// VerificationContext captures the execution context used for multi-layer verification.
type VerificationContext struct {
	WorkingDir   string
	Files        []string
	ChangedFiles []string
	Operation    string
	ToolName     string
	Expected     string
	Metadata     map[string]interface{}
}

// CheckpointType categorizes verification methods.
type CheckpointType string

const (
	CheckpointSyntax      CheckpointType = "syntax"
	CheckpointSemantic    CheckpointType = "semantic"
	CheckpointTest        CheckpointType = "test"
	CheckpointIntegration CheckpointType = "integration"
	CheckpointPerformance CheckpointType = "performance"
	CheckpointSecurity    CheckpointType = "security"
)

// Finding represents a research result.
type Finding struct {
	Query     string
	Source    string // file path or tool name
	Relevance float64
	Content   string
	FoundAt   time.Time
}

// ExecutionState tracks progress through a plan.
type ExecutionState struct {
	PlanID         string
	CurrentPhase   Phase
	CompletedTasks []string
	ActiveTasks    []string
	FailedTasks    []string
	Attempts       map[string]int // task ID -> attempt count
	Findings       []Finding
	UpdatedAt      time.Time
}

// Strategy defines how to approach task execution.
type Strategy interface {
	// Name returns strategy identifier.
	Name() string

	// CanHandle determines if strategy applies to this task type.
	CanHandle(taskType TaskType) bool

	// Decompose breaks down a task into subtasks.
	Decompose(ctx context.Context, analysis *TaskAnalysis) ([]*Task, error)

	// EstimateEffort provides time/complexity estimate.
	EstimateEffort(analysis *TaskAnalysis) time.Duration

	// Confidence returns confidence level for this strategy (0.0-1.0).
	Confidence() float64
}

// ErrorPattern represents a recognized error pattern.
type ErrorPattern struct {
	Pattern    string  `json:"pattern"`
	Severity   string  `json:"severity"`
	Category   string  `json:"category"`
	Location   string  `json:"location,omitempty"`
	Message    string  `json:"message"`
	Confidence float64 `json:"confidence"`
}

// FailedTask represents a task that failed during execution.
type FailedTask struct {
	TaskID       string   `json:"task_id"`
	Description  string   `json:"description"`
	Error        string   `json:"error"`
	Fixable      bool     `json:"fixable"`
	Retryable    bool     `json:"retryable"`
	FailureCount int      `json:"failure_count"`
	Diagnostics  []string `json:"diagnostics,omitempty"`
}

// Fix represents a suggested fix for a failure.
type Fix struct {
	Description string   `json:"description"`
	Action      string   `json:"action"`
	Priority    int      `json:"priority"`
	Confidence  float64  `json:"confidence"`
	Automated   bool     `json:"automated"`
	Steps       []string `json:"steps,omitempty"`
}

// FailureAnalysis represents comprehensive failure analysis results.
type FailureAnalysis struct {
	RootCause       string         `json:"root_cause"`
	Confidence      float64        `json:"confidence"`
	ErrorPatterns   []ErrorPattern `json:"error_patterns"`
	FailedTasks     []FailedTask   `json:"failed_tasks"`
	SuggestedFixes  []Fix          `json:"suggested_fixes"`
	Retryable       bool           `json:"retryable"`
	RequiresManual  bool           `json:"requires_manual"`
	HistoricalMatch bool           `json:"historical_match"`
	AnalyzedAt      time.Time      `json:"analyzed_at"`
}

// ConcreteStrategy is a concrete implementation with learning metrics.
type ConcreteStrategy struct {
	StrategyName     string
	ConfidenceScore  float64
	TimesUsed        int
	TotalSuccesses   int
	AvgExecutionTime time.Duration
	NeedsReview      bool
	Handler          Strategy
}
