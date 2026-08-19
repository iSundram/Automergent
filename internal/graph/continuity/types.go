package continuity

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type TaskRelation string

const (
	TaskRelationFollowUp TaskRelation = "follow_up"
	TaskRelationNewTask  TaskRelation = "new_task"
	TaskRelationRelated  TaskRelation = "related"
)

type ContinuityConfig struct {
	SimilarityThreshold     float64   `json:"similarity_threshold"`
	MaxPreviousTasks        int       `json:"max_previous_tasks"`
	ContextWindowSize       int       `json:"context_window_size"`
	EnableGraphSimilarity   bool      `json:"enable_graph_similarity"`
	EnableSemanticAnalysis  bool      `json:"enable_semantic_analysis"`
	FollowUpConfidenceMin   float64   `json:"follow_up_confidence_min"`
	RelatedConfidenceMin    float64   `json:"related_confidence_min"`
	DefaultResumePolicy     ResumePolicy `json:"default_resume_policy"`
}

type ResumePolicy string

const (
	ResumePolicyFull    ResumePolicy = "full"
	ResumePolicySummary ResumePolicy = "summary"
	ResumePolicyPartial ResumePolicy = "partial"
	ResumePolicyNone    ResumePolicy = "none"
)

type ResumeConfig struct {
	Policy           ResumePolicy     `json:"policy"`
	ShareBuckets     []uuid.UUID      `json:"share_buckets"`
	ShareKeys        []string         `json:"share_keys"`
	ExcludeKeys      []string         `json:"exclude_keys"`
	MaxTokens        int              `json:"max_tokens"`
	IncludeDecisions bool             `json:"include_decisions"`
	IncludeMemories  bool             `json:"include_memories"`
	IncludeFiles     bool             `json:"include_files"`
	IncludeTodos     bool             `json:"include_todos"`
}

type UndoActionType string

const (
	UndoActionTypeEdit       UndoActionType = "edit"
	UndoActionTypeDecision   UndoActionType = "decision"
	UndoActionTypeTodo       UndoActionType = "todo"
	UndoActionTypeFile       UndoActionType = "file"
	UndoActionTypeBucket     UndoActionType = "bucket"
	UndoActionTypeMemory     UndoActionType = "memory"
)

type UndoScope string

const (
	UndoScopeSingle   UndoScope = "single"
	UndoScopeDecision UndoScope = "decision"
	UndoScopeTodo     UndoScope = "todo"
	UndoScopeTask     UndoScope = "task"
	UndoScopeSession  UndoScope = "session"
)

type UndoAction struct {
	ID           uuid.UUID       `json:"id"`
	Type         UndoActionType  `json:"type"`
	Scope        UndoScope       `json:"scope"`
	TargetID     uuid.UUID       `json:"target_id"`
	TaskID       uuid.UUID       `json:"task_id"`
	Description  string          `json:"description"`
	PreviousData json.RawMessage `json:"previous_data"`
	CurrentData  json.RawMessage `json:"current_data"`
	Metadata     json.RawMessage `json:"metadata,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
	RevertedAt   *time.Time      `json:"reverted_at,omitempty"`
	CanRevert    bool            `json:"can_revert"`
	RevertedBy   string          `json:"reverted_by,omitempty"`
}

type ContextResumeResult struct {
	TaskID           uuid.UUID              `json:"task_id"`
	Relation         TaskRelation           `json:"relation"`
	Confidence       float64                `json:"confidence"`
	CoderContext     *ContextBucketSnapshot `json:"coder_context,omitempty"`
	AssistantContext *ContextBucketSnapshot `json:"assistant_context,omitempty"`
	VerificationCtx  *VerificationSnapshot  `json:"verification_context,omitempty"`
	SharedBuckets    []uuid.UUID            `json:"shared_buckets"`
	ExcludedBuckets  []uuid.UUID            `json:"excluded_buckets"`
	Decisions        []DecisionSummary      `json:"decisions"`
	Memories         []MemorySummary        `json:"memories"`
	Files            []FileSummary          `json:"files"`
	Todos            []TodoSummary          `json:"todos"`
	ResumeConfig     ResumeConfig           `json:"resume_config"`
	GeneratedAt      time.Time              `json:"generated_at"`
}

type ContextBucketSnapshot struct {
	BucketID   uuid.UUID         `json:"bucket_id"`
	Name       string            `json:"name"`
	Type       string            `json:"type"`
	Owner      string            `json:"owner"`
	SharePolicy string           `json:"share_policy"`
	Data       json.RawMessage   `json:"data"`
	Keys       []string          `json:"keys"`
	UpdatedAt  time.Time         `json:"updated_at"`
}

type VerificationSnapshot struct {
	Status           string    `json:"status"`
	LastVerifiedAt   time.Time `json:"last_verified_at"`
	PassedChecks     []string  `json:"passed_checks"`
	FailedChecks     []string  `json:"failed_checks"`
	PendingChecks    []string  `json:"pending_checks"`
	Coverage         float64   `json:"coverage"`
	Metadata         json.RawMessage `json:"metadata,omitempty"`
}

type DecisionSummary struct {
	ID          uuid.UUID `json:"id"`
	Title       string    `json:"title"`
	Type        string    `json:"type"`
	Status      string    `json:"status"`
	Outcome     string    `json:"outcome"`
	Confidence  float64   `json:"confidence"`
	CreatedAt   time.Time `json:"created_at"`
}

type MemorySummary struct {
	ID         uuid.UUID `json:"id"`
	Content    string    `json:"content"`
	Type       string    `json:"type"`
	Scope      string    `json:"scope"`
	Confidence float64   `json:"confidence"`
	CreatedAt  time.Time `json:"created_at"`
}

type FileSummary struct {
	ID       uuid.UUID `json:"id"`
	Path     string    `json:"path"`
	Name     string    `json:"name"`
	Language string    `json:"language"`
	Hash     string    `json:"hash"`
	Size     int64     `json:"size"`
}

type TodoSummary struct {
	ID          uuid.UUID `json:"id"`
	Title       string    `json:"title"`
	Status      string    `json:"status"`
	Priority    int       `json:"priority"`
	Completed   bool      `json:"completed"`
	CreatedAt   time.Time `json:"created_at"`
}

type TaskComparison struct {
	TaskAID          uuid.UUID   `json:"task_a_id"`
	TaskBID          uuid.UUID   `json:"task_b_id"`
	Similarity       float64     `json:"similarity"`
	SharedBuckets    []uuid.UUID `json:"shared_buckets"`
	SharedDecisions  []uuid.UUID `json:"shared_decisions"`
	SharedMemories   []uuid.UUID `json:"shared_memories"`
	SharedFiles      []uuid.UUID `json:"shared_files"`
	Differences      TaskDifferences `json:"differences"`
	Relation         TaskRelation    `json:"relation"`
	Confidence       float64         `json:"confidence"`
	ComparedAt       time.Time       `json:"compared_at"`
}

type TaskDifferences struct {
	BucketsOnlyInA  []uuid.UUID `json:"buckets_only_in_a"`
	BucketsOnlyInB  []uuid.UUID `json:"buckets_only_in_b"`
	DecisionsOnlyInA []uuid.UUID `json:"decisions_only_in_a"`
	DecisionsOnlyInB []uuid.UUID `json:"decisions_only_in_b"`
	MemoriesOnlyInA  []uuid.UUID `json:"memories_only_in_a"`
	MemoriesOnlyInB  []uuid.UUID `json:"memories_only_in_b"`
	FilesOnlyInA     []uuid.UUID `json:"files_only_in_a"`
	FilesOnlyInB     []uuid.UUID `json:"files_only_in_b"`
	TodosOnlyInA     []uuid.UUID `json:"todos_only_in_a"`
	TodosOnlyInB     []uuid.UUID `json:"todos_only_in_b"`
}

type TaskBoundary struct {
	Index       int           `json:"index"`
	MessageID   uuid.UUID     `json:"message_id"`
	Confidence  float64       `json:"confidence"`
	Reason      string        `json:"reason"`
	Type        BoundaryType  `json:"type"`
	PreviousTaskID *uuid.UUID `json:"previous_task_id,omitempty"`
	NewTaskID   *uuid.UUID    `json:"new_task_id,omitempty"`
}

type BoundaryType string

const (
	BoundaryTypeTopicShift   BoundaryType = "topic_shift"
	BoundaryTypeNewRequest   BoundaryType = "new_request"
	BoundaryTypeCompletion   BoundaryType = "completion"
	BoundaryTypeInterruption BoundaryType = "interruption"
	BoundaryTypeContextReset BoundaryType = "context_reset"
)

type TaskPriority struct {
	TaskID     uuid.UUID `json:"task_id"`
	Score      float64   `json:"score"`
	Complexity float64   `json:"complexity"`
	Urgency    float64   `json:"urgency"`
	DependencyCount int   `json:"dependency_count"`
	Reason     string    `json:"reason"`
	CalculatedAt time.Time `json:"calculated_at"`
}

type RouteResult struct {
	Handler      string       `json:"handler"`
	TaskID       *uuid.UUID   `json:"task_id,omitempty"`
	Relation     TaskRelation `json:"relation"`
	Confidence   float64      `json:"confidence"`
	Reason       string       `json:"reason"`
	Priority     TaskPriority `json:"priority"`
	Boundaries   []TaskBoundary `json:"boundaries,omitempty"`
}

func DefaultContinuityConfig() ContinuityConfig {
	return ContinuityConfig{
		SimilarityThreshold:    0.7,
		MaxPreviousTasks:       10,
		ContextWindowSize:      50,
		EnableGraphSimilarity:  true,
		EnableSemanticAnalysis: true,
		FollowUpConfidenceMin:  0.75,
		RelatedConfidenceMin:   0.5,
		DefaultResumePolicy:    ResumePolicySummary,
	}
}

func ResumeConfigForRelation(relation TaskRelation) ResumeConfig {
	base := ResumeConfig{
		Policy:           ResumePolicySummary,
		IncludeDecisions: true,
		IncludeMemories:  true,
		IncludeFiles:     true,
		IncludeTodos:     true,
		MaxTokens:        8000,
	}

	switch relation {
	case TaskRelationFollowUp:
		base.Policy = ResumePolicyFull
		base.MaxTokens = 16000
	case TaskRelationRelated:
		base.Policy = ResumePolicyPartial
		base.MaxTokens = 4000
	case TaskRelationNewTask:
		base.Policy = ResumePolicyNone
		base.MaxTokens = 0
	}

	return base
}