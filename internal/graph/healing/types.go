package healing

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type FixOutcome string

const (
	FixOutcomeSuccess     FixOutcome = "success"
	FixOutcomePartial     FixOutcome = "partial"
	FixOutcomeFailure     FixOutcome = "failure"
	FixOutcomeWorsened    FixOutcome = "worsened"
)

type FixAttempt struct {
	ID              uuid.UUID       `json:"id"`
	IssueID         uuid.UUID       `json:"issue_id"`
	FixDescription  string          `json:"fix_description"`
	FixData         json.RawMessage `json:"fix_data"`
	AttemptNumber   int             `json:"attempt_number"`
	Outcome         FixOutcome      `json:"outcome"`
	Confidence      float64         `json:"confidence"`
	TestsPassed     int             `json:"tests_passed"`
	TestsFailed     int             `json:"tests_failed"`
	AcceptanceMet   bool            `json:"acceptance_met"`
	ErrorMessage    string          `json:"error_message,omitempty"`
	CreatedAt       time.Time       `json:"created_at"`
	CompletedAt     *time.Time      `json:"completed_at,omitempty"`
	RevertedAt      *time.Time      `json:"reverted_at,omitempty"`
	Metadata        json.RawMessage `json:"metadata,omitempty"`
}

type CleanupAction string

const (
	CleanupActionRemoveStaleNodes     CleanupAction = "remove_stale_nodes"
	CleanupActionConsolidateMemories  CleanupAction = "consolidate_memories"
	CleanupActionUpdateStaleness      CleanupAction = "update_staleness"
	CleanupActionRebuildIndexes       CleanupAction = "rebuild_indexes"
	CleanupActionCompactGraph         CleanupAction = "compact_graph"
	CleanupActionRemoveInjectedPrompts CleanupAction = "remove_injected_prompts"
	CleanupActionDeduplicateContext   CleanupAction = "deduplicate_context"
)

type CleanupTarget string

const (
	CleanupTargetNodes       CleanupTarget = "nodes"
	CleanupTargetEdges       CleanupTarget = "edges"
	CleanupTargetMemories    CleanupTarget = "memories"
	CleanupTargetContexts    CleanupTarget = "contexts"
	CleanupTargetPrompts     CleanupTarget = "prompts"
	CleanupTargetIndexes     CleanupTarget = "indexes"
)

type StalenessConfig struct {
	NodeTTL              time.Duration `json:"node_ttl"`
	EdgeTTL              time.Duration `json:"edge_ttl"`
	MemoryTTL            time.Duration `json:"memory_ttl"`
	ContextTTL           time.Duration `json:"context_ttl"`
	PromptTTL            time.Duration `json:"prompt_ttl"`
	MinAccessCount       int           `json:"min_access_count"`
	StalenessThreshold   float64       `json:"staleness_threshold"`
	EnableAutoPrune      bool          `json:"enable_auto_prune"`
	PruneBatchSize       int           `json:"prune_batch_size"`
}

type FixValidatorConfig struct {
	MaxRetries            int           `json:"max_retries"`
	RetryDelay            time.Duration `json:"retry_delay"`
	MinConfidence         float64       `json:"min_confidence"`
	RequireTestsPass      bool          `json:"require_tests_pass"`
	RequireAcceptanceMet  bool          `json:"require_acceptance_met"`
	AutoUndoOnFailure     bool          `json:"auto_undo_on_failure"`
	TrackAttempts         bool          `json:"track_attempts"`
	ValidationTimeout     time.Duration `json:"validation_timeout"`
}

type CleanupStats struct {
	NodesRemoved       int64     `json:"nodes_removed"`
	EdgesRemoved       int64     `json:"edges_removed"`
	MemoriesConsolidated int64   `json:"memories_consolidated"`
	PromptsRemoved     int64     `json:"prompts_removed"`
	ContextsDeduplicated int64   `json:"contexts_deduplicated"`
	IndexesRebuilt     int64     `json:"indexes_rebuilt"`
	GraphCompacted     bool      `json:"graph_compacted"`
	Duration           time.Duration `json:"duration"`
	StartedAt          time.Time `json:"started_at"`
	CompletedAt        time.Time `json:"completed_at"`
	Errors             []string  `json:"errors,omitempty"`
}

type CleanupConfig struct {
	Interval            time.Duration `json:"interval"`
	StalenessConfig     StalenessConfig `json:"staleness_config"`
	FixValidatorConfig  FixValidatorConfig `json:"fix_validator_config"`
	EnablePeriodicCleanup bool        `json:"enable_periodic_cleanup"`
	EnableFixValidation   bool        `json:"enable_fix_validation"`
	EnableContextCleanup  bool        `json:"enable_context_cleanup"`
	EnableGraphMaintenance bool       `json:"enable_graph_maintenance"`
}

func DefaultStalenessConfig() StalenessConfig {
	return StalenessConfig{
		NodeTTL:            24 * time.Hour,
		EdgeTTL:            24 * time.Hour,
		MemoryTTL:          7 * 24 * time.Hour,
		ContextTTL:         6 * time.Hour,
		PromptTTL:          1 * time.Hour,
		MinAccessCount:     2,
		StalenessThreshold: 0.7,
		EnableAutoPrune:    true,
		PruneBatchSize:     100,
	}
}

func DefaultFixValidatorConfig() FixValidatorConfig {
	return FixValidatorConfig{
		MaxRetries:           3,
		RetryDelay:           5 * time.Second,
		MinConfidence:        0.6,
		RequireTestsPass:     true,
		RequireAcceptanceMet: true,
		AutoUndoOnFailure:    true,
		TrackAttempts:        true,
		ValidationTimeout:    2 * time.Minute,
	}
}

func DefaultCleanupConfig() CleanupConfig {
	return CleanupConfig{
		Interval:              1 * time.Hour,
		StalenessConfig:       DefaultStalenessConfig(),
		FixValidatorConfig:    DefaultFixValidatorConfig(),
		EnablePeriodicCleanup: true,
		EnableFixValidation:   true,
		EnableContextCleanup:  true,
		EnableGraphMaintenance: true,
	}
}

type FixHistory struct {
	IssueID      uuid.UUID     `json:"issue_id"`
	Attempts     []*FixAttempt `json:"attempts"`
	TotalAttempts int          `json:"total_attempts"`
	LastOutcome  FixOutcome    `json:"last_outcome"`
	BestOutcome  FixOutcome    `json:"best_outcome"`
	CreatedAt    time.Time     `json:"created_at"`
	UpdatedAt    time.Time     `json:"updated_at"`
}

type ValidationResult struct {
	Valid          bool     `json:"valid"`
	TestsPassed    int      `json:"tests_passed"`
	TestsFailed    int      `json:"tests_failed"`
	AcceptanceMet  bool     `json:"acceptance_met"`
	Confidence     float64  `json:"confidence"`
	Errors         []string `json:"errors,omitempty"`
	Warnings       []string `json:"warnings,omitempty"`
	Duration       time.Duration `json:"duration"`
}

func (fo FixOutcome) IsSuccess() bool {
	return fo == FixOutcomeSuccess
}

func (fo FixOutcome) IsTerminal() bool {
	return fo == FixOutcomeSuccess || fo == FixOutcomeFailure || fo == FixOutcomeWorsened
}

func (fo FixOutcome) Score() float64 {
	switch fo {
	case FixOutcomeSuccess:
		return 1.0
	case FixOutcomePartial:
		return 0.5
	case FixOutcomeFailure:
		return 0.0
	case FixOutcomeWorsened:
		return -0.5
	default:
		return 0.0
	}
}

type GraphStore interface {
	GetNode(ctx context.Context, id uuid.UUID) (interface{}, error)
	UpdateNode(ctx context.Context, node interface{}) error
	DeleteNode(ctx context.Context, id uuid.UUID) error
	ListNodes(ctx context.Context, nodeType string, limit, offset int) ([]interface{}, error)
	CreateNode(ctx context.Context, node interface{}) error
	ExecuteQuery(ctx context.Context, query string, args ...interface{}) (interface{}, error)
}