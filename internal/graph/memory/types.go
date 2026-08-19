package memory

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type DecisionType string

const (
	DecisionTypeArchitecture  DecisionType = "architecture"
	DecisionTypeImplementation DecisionType = "implementation"
	DecisionTypeToolChoice    DecisionType = "tool_choice"
	DecisionTypeScope         DecisionType = "scope"
	DecisionTypeUndo          DecisionType = "undo"
	DecisionTypeVerification  DecisionType = "verification"
)

type MemoryType string

const (
	MemoryTypeFact        MemoryType = "fact"
	MemoryTypePattern     MemoryType = "pattern"
	MemoryTypeFailure     MemoryType = "failure"
	MemoryTypePreference  MemoryType = "preference"
	MemoryTypeDecision    MemoryType = "decision"
	MemoryTypeEvent       MemoryType = "event"
	MemoryTypeSolution    MemoryType = "solution"
)

type MemoryScope string

const (
	MemoryScopeProject MemoryScope = "project"
	MemoryScopeTask    MemoryScope = "task"
	MemoryScopeUser    MemoryScope = "user"
	MemoryScopeGlobal  MemoryScope = "global"
)

type Option struct {
	ID          string          `json:"id"`
	Title       string          `json:"title"`
	Description string          `json:"description"`
	Pros        []string        `json:"pros,omitempty"`
	Cons        []string        `json:"cons,omitempty"`
	Score       float64         `json:"score,omitempty"`
	Metadata    json.RawMessage `json:"metadata,omitempty"`
}

type EvidenceRef struct {
	ID          uuid.UUID `json:"id"`
	Type        string    `json:"type"`
	Source      string    `json:"source"`
	Description string    `json:"description"`
	Relevance   float64   `json:"relevance"`
	CreatedAt   time.Time `json:"created_at"`
}

type Decision struct {
	ID            uuid.UUID       `json:"id"`
	Title         string          `json:"title"`
	Description   string          `json:"description"`
	Type          DecisionType    `json:"type"`
	Status        string          `json:"status"`
	Options       []Option        `json:"options,omitempty"`
	ChosenOption  *Option         `json:"chosen_option,omitempty"`
	Rationale     string          `json:"rationale,omitempty"`
	Outcome       string          `json:"outcome,omitempty"`
	Confidence    float64         `json:"confidence"`
	Evidence      []EvidenceRef   `json:"evidence,omitempty"`
	ContextBucket uuid.UUID       `json:"context_bucket,omitempty"`
	TaskID        uuid.UUID       `json:"task_id,omitempty"`
	FilePaths     []string        `json:"file_paths,omitempty"`
	ToolsUsed     []string        `json:"tools_used,omitempty"`
	Tags          []string        `json:"tags,omitempty"`
	Metadata      json.RawMessage `json:"metadata,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
	CreatedBy     string          `json:"created_by,omitempty"`
}

type Memory struct {
	ID          uuid.UUID       `json:"id"`
	Content     string          `json:"content"`
	Type        MemoryType      `json:"type"`
	Scope       MemoryScope     `json:"scope"`
	Tags        []string        `json:"tags,omitempty"`
	Source      string          `json:"source,omitempty"`
	Confidence  float64         `json:"confidence"`
	Embedding   []float32       `json:"embedding,omitempty"`
	References  []uuid.UUID     `json:"references,omitempty"`
	ContextBuckets []uuid.UUID  `json:"context_buckets,omitempty"`
	TaskIDs     []uuid.UUID     `json:"task_ids,omitempty"`
	FilePaths   []string        `json:"file_paths,omitempty"`
	Metadata    json.RawMessage `json:"metadata,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
	AccessCount int             `json:"access_count"`
	LastAccessed *time.Time     `json:"last_accessed,omitempty"`
}

type DecisionReplay struct {
	Decision     *Decision    `json:"decision"`
	FilePath     string       `json:"file_path"`
	LineRange    *LineRange   `json:"line_range,omitempty"`
	TouchType    string       `json:"touch_type"`
	Timestamp    time.Time    `json:"timestamp"`
	RelatedFiles []string     `json:"related_files,omitempty"`
}

type LineRange struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

type DecisionTimelineEntry struct {
	Decision   *Decision   `json:"decision"`
	Timestamp  time.Time   `json:"timestamp"`
	Sequence   int         `json:"sequence"`
	RelatedTo  []uuid.UUID `json:"related_to,omitempty"`
}

type FailedAttempt struct {
	Issue          string          `json:"issue"`
	Approach       string          `json:"approach"`
	Reason         string          `json:"reason"`
	Decision       *Decision       `json:"decision,omitempty"`
	ErrorDetails   string          `json:"error_details,omitempty"`
	Timestamp      time.Time       `json:"timestamp"`
	Tags           []string        `json:"tags,omitempty"`
	Metadata       json.RawMessage `json:"metadata,omitempty"`
}

type FixAttempt struct {
	FilePath     string          `json:"file_path"`
	Issue        string          `json:"issue"`
	Fix          string          `json:"fix"`
	Outcome      string          `json:"outcome"`
	Decision     *Decision       `json:"decision,omitempty"`
	TestResult   string          `json:"test_result,omitempty"`
	Timestamp    time.Time       `json:"timestamp"`
	Tags         []string        `json:"tags,omitempty"`
	Metadata     json.RawMessage `json:"metadata,omitempty"`
}

const (
	DefaultDeduplicationThreshold = 0.85
	DefaultPromotionThreshold     = 0.7
	DefaultRelevanceThreshold     = 0.6
)

type SimilarityResult struct {
	MemoryID    uuid.UUID `json:"memory_id"`
	Similarity  float64   `json:"similarity"`
	MatchType   string    `json:"match_type"`
}

type EmbeddingProvider interface {
	GetEmbedding(ctx context.Context, text string) ([]float32, error)
}