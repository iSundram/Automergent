package tools

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type GraphStore interface {
	CreateNode(ctx context.Context, node *Node) error
	GetNode(ctx context.Context, id uuid.UUID) (*Node, error)
	UpdateNode(ctx context.Context, node *Node) error
	ListNodes(ctx context.Context, nodeType NodeType, limit, offset int) ([]*Node, error)
	CreateEdge(ctx context.Context, edge *Edge) error
	GetEdgesFrom(ctx context.Context, fromID uuid.UUID, edgeType EdgeType) ([]*Edge, error)
	GetEdgesTo(ctx context.Context, toID uuid.UUID, edgeType EdgeType) ([]*Edge, error)
}

type Node struct {
	ID        uuid.UUID       `json:"id"`
	Type      NodeType        `json:"type"`
	Data      json.RawMessage `json:"data"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

type Edge struct {
	ID        uuid.UUID       `json:"id"`
	FromID    uuid.UUID       `json:"from_id"`
	ToID      uuid.UUID       `json:"to_id"`
	Type      EdgeType        `json:"type"`
	Data      json.RawMessage `json:"data"`
	CreatedAt time.Time       `json:"created_at"`
}

type NodeType string
type EdgeType string

const (
	NodeTypeToolDefinition      NodeType = "tool_definition"
	NodeTypeToolEffectiveness   NodeType = "tool_effectiveness"
	NodeTypeToolUsage           NodeType = "tool_usage"
	NodeTypeDynamicTool         NodeType = "dynamic_tool"
	NodeTypeCommandNode         NodeType = "command_node"
	NodeTypeCommandEdge         NodeType = "command_edge"
	NodeTypeObservedPattern     NodeType = "observed_pattern"

	EdgeTypeToolTriggers        EdgeType = "tool_triggers"
	EdgeTypeToolUsedIn          EdgeType = "tool_used_in"
	EdgeTypeToolDerivedFrom     EdgeType = "tool_derived_from"
	EdgeTypeCommandDependsOn    EdgeType = "command_depends_on"
	EdgeTypeCommandRunsAfter    EdgeType = "command_runs_after"
	EdgeTypeCommandConflicts    EdgeType = "command_conflicts"
	EdgeTypePatternGenerates    EdgeType = "pattern_generates"
)

type ToolCharacter string

const (
	ToolCharacterReadOnly    ToolCharacter = "read_only"
	ToolCharacterModifying   ToolCharacter = "modifying"
	ToolCharacterDestructive ToolCharacter = "destructive"
	ToolCharacterExternal    ToolCharacter = "external"
)

type ContextBucketType string

const (
	ContextBucketTypeProject     ContextBucketType = "project"
	ContextBucketTypeSession     ContextBucketType = "session"
	ContextBucketTypeTask        ContextBucketType = "task"
	ContextBucketTypeAgent       ContextBucketType = "agent"
	ContextBucketTypeGlobal      ContextBucketType = "global"
	ContextBucketTypeTemporary   ContextBucketType = "temporary"
)

type ContextTrigger struct {
	BucketType   ContextBucketType `json:"bucket_type"`
	Keywords     []string          `json:"keywords"`
	MinRelevance float64           `json:"min_relevance"`
}

type UseCase struct {
	Description string            `json:"description"`
	Examples    []string          `json:"examples,omitempty"`
	Context     ContextBucketType `json:"context"`
}

type ToolEffectiveness struct {
	ToolName        string             `json:"tool_name"`
	ContextBucket   ContextBucketType  `json:"context_bucket"`
	TotalUsages     int                `json:"total_usages"`
	SuccessfulUsages int               `json:"successful_usages"`
	FailedUsages    int                `json:"failed_usages"`
	AvgRelevance    float64            `json:"avg_relevance"`
	LastUsed        time.Time          `json:"last_used"`
	UpdatedAt       time.Time          `json:"updated_at"`
}

type ToolDefinition struct {
	Name              string           `json:"name"`
	Description       string           `json:"description"`
	Character         ToolCharacter    `json:"character"`
	Parameters        json.RawMessage  `json:"parameters"`
	ContextTriggers   []ContextTrigger `json:"context_triggers"`
	UseCases          []UseCase        `json:"use_cases"`
	Version           string           `json:"version"`
	Author            string           `json:"author,omitempty"`
	Tags              []string         `json:"tags,omitempty"`
	CreatedAt         time.Time        `json:"created_at"`
	UpdatedAt         time.Time        `json:"updated_at"`
}

type DynamicTool struct {
	ID                uuid.UUID        `json:"id"`
	Name              string           `json:"name"`
	Description       string           `json:"description"`
	Character         ToolCharacter    `json:"character"`
	Parameters        json.RawMessage  `json:"parameters"`
	Pattern           string           `json:"pattern"`
	ContextBucket     ContextBucketType `json:"context_bucket"`
	Confidence        float64          `json:"confidence"`
	UsageCount        int              `json:"usage_count"`
	SuccessCount      int              `json:"success_count"`
	SourceTool        string           `json:"source_tool,omitempty"`
	CreatedAt         time.Time        `json:"created_at"`
	UpdatedAt         time.Time        `json:"updated_at"`
	Promoted          bool             `json:"promoted"`
}

type ToolUsageRecord struct {
	ID              uuid.UUID        `json:"id"`
	ToolName        string           `json:"tool_name"`
	ContextBucket   ContextBucketType `json:"context_bucket"`
	Outcome         string           `json:"outcome"`
	Relevance       float64          `json:"relevance"`
	DurationMs      int64            `json:"duration_ms"`
	Error           string           `json:"error,omitempty"`
	Timestamp       time.Time        `json:"timestamp"`
}

type CommandNode struct {
	ID           uuid.UUID      `json:"id"`
	Name         string         `json:"name"`
	Command      string         `json:"command"`
	Description  string         `json:"description"`
	Category     string         `json:"category"`
	Dependencies []uuid.UUID    `json:"dependencies"`
	ContextTriggers []ContextTrigger `json:"context_triggers"`
	IsStale      bool           `json:"is_stale"`
	LastRun      *time.Time     `json:"last_run,omitempty"`
	RunCount     int            `json:"run_count"`
	SuccessRate  float64        `json:"success_rate"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
}

type CommandEdge struct {
	FromID   uuid.UUID `json:"from_id"`
	ToID     uuid.UUID `json:"to_id"`
	Type     string    `json:"type"`
	Strength float64   `json:"strength"`
}

type CommandGraph struct {
	Nodes []CommandNode `json:"nodes"`
	Edges []CommandEdge `json:"edges"`
}

type ObservedPattern struct {
	ID              uuid.UUID        `json:"id"`
	Pattern         string           `json:"pattern"`
	ContextBucket   ContextBucketType `json:"context_bucket"`
	ToolSequence    []string         `json:"tool_sequence"`
	Frequency       int              `json:"frequency"`
	SuccessRate     float64          `json:"success_rate"`
	AvgDurationMs   int64            `json:"avg_duration_ms"`
	LastObserved    time.Time        `json:"last_observed"`
	Confidence      float64          `json:"confidence"`
	CreatedAt       time.Time        `json:"created_at"`
	UpdatedAt       time.Time        `json:"updated_at"`
}