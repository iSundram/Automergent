package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrBucketNotFound         = errors.New("bucket not found")
	ErrWorkflowNotFound       = errors.New("workflow not found")
	ErrTodoNotFound           = errors.New("todo not found")
	ErrInvalidSharePolicy     = errors.New("invalid share policy")
	ErrDependencyNotMet       = errors.New("dependency not met")
	ErrBucketExists           = errors.New("bucket already exists")
	ErrDependencyCycle        = errors.New("dependency cycle detected")
	ErrInvalidStatus          = errors.New("invalid status transition")
	ErrMemoryNotFound         = errors.New("memory not found")
	ErrInsufficientConfidence = errors.New("insufficient confidence for promotion")
)

type ContextBucketType string

const (
	ContextBucketTypeProject   ContextBucketType = "project"
	ContextBucketTypeSession   ContextBucketType = "session"
	ContextBucketTypeTask      ContextBucketType = "task"
	ContextBucketTypeAgent     ContextBucketType = "agent"
	ContextBucketTypeGlobal    ContextBucketType = "global"
	ContextBucketTypeTemporary ContextBucketType = "temporary"
	ContextBucketTypeTodo      ContextBucketType = "todo"
	ContextBucketTypeWorkflow  ContextBucketType = "workflow"
)

type SharePolicy string

const (
	SharePolicyNone     SharePolicy = "none"
	SharePolicySummary  SharePolicy = "summary"
	SharePolicyFull     SharePolicy = "full"
	SharePolicyPartial  SharePolicy = "partial"
	SharePolicyInjected SharePolicy = "injected"
)

type TodoStatus string

const (
	TodoStatusPending    TodoStatus = "pending"
	TodoStatusInProgress TodoStatus = "in_progress"
	TodoStatusDone       TodoStatus = "done"
	TodoStatusBlocked    TodoStatus = "blocked"
	TodoStatusSkipped    TodoStatus = "skipped"
)

type MemoryType string

const (
	MemoryTypeFact       MemoryType = "fact"
	MemoryTypePattern    MemoryType = "pattern"
	MemoryTypePreference MemoryType = "preference"
	MemoryTypeSkill      MemoryType = "skill"
	MemoryTypeContext    MemoryType = "context"
	MemoryTypeError      MemoryType = "error"
	MemoryTypeSolution   MemoryType = "solution"
)

type MemoryScope string

const (
	MemoryScopeGlobal  MemoryScope = "global"
	MemoryScopeProject MemoryScope = "project"
	MemoryScopeSession MemoryScope = "session"
	MemoryScopeAgent   MemoryScope = "agent"
	MemoryScopeTask    MemoryScope = "task"
)

type TodoWorkflow struct {
	ID          uuid.UUID       `json:"id"`
	TaskID      uuid.UUID       `json:"task_id"`
	Category    string          `json:"category"`
	Title       string          `json:"title"`
	Description string          `json:"description"`
	Status      string          `json:"status"`
	CurrentTodo uuid.UUID       `json:"current_todo,omitempty"`
	Metadata    json.RawMessage `json:"metadata,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

type TodoItem struct {
	ID           uuid.UUID       `json:"id"`
	WorkflowID   uuid.UUID       `json:"workflow_id"`
	Title        string          `json:"title"`
	Description  string          `json:"description"`
	Status       TodoStatus      `json:"status"`
	Priority     int             `json:"priority"`
	Dependencies []uuid.UUID     `json:"dependencies,omitempty"`
	SharePolicy  SharePolicy     `json:"share_policy"`
	ShareKeys    []string        `json:"share_keys,omitempty"`
	BucketID     uuid.UUID       `json:"bucket_id,omitempty"`
	Assignee     string          `json:"assignee,omitempty"`
	Tags         []string        `json:"tags,omitempty"`
	Metadata     json.RawMessage `json:"metadata,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
	StartedAt    *time.Time      `json:"started_at,omitempty"`
	CompletedAt  *time.Time      `json:"completed_at,omitempty"`
}

type ContextBucket struct {
	ID          uuid.UUID         `json:"id"`
	WorkflowID  uuid.UUID         `json:"workflow_id,omitempty"`
	TodoID      uuid.UUID         `json:"todo_id,omitempty"`
	Name        string            `json:"name"`
	Type        ContextBucketType `json:"type"`
	Description string            `json:"description"`
	SharePolicy SharePolicy       `json:"share_policy"`
	ShareKeys   []string          `json:"share_keys,omitempty"`
	Owner       string            `json:"owner"`
	Data        json.RawMessage   `json:"data,omitempty"`
	Tags        []string          `json:"tags,omitempty"`
	Metadata    json.RawMessage   `json:"metadata,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

type InjectedMessage struct {
	ID        uuid.UUID       `json:"id"`
	BucketID  uuid.UUID       `json:"bucket_id"`
	TodoID    uuid.UUID       `json:"todo_id"`
	FromAgent string          `json:"from_agent"`
	Message   string          `json:"message"`
	Priority  int             `json:"priority"`
	Tags      []string        `json:"tags,omitempty"`
	Metadata  json.RawMessage `json:"metadata,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
}

type Memory struct {
	ID         uuid.UUID       `json:"id"`
	Content    string          `json:"content"`
	Type       MemoryType      `json:"type"`
	Scope      MemoryScope     `json:"scope"`
	Tags       []string        `json:"tags,omitempty"`
	Source     string          `json:"source,omitempty"`
	Confidence float64         `json:"confidence"`
	Metadata   json.RawMessage `json:"metadata,omitempty"`
}

type BucketSummary struct {
	BucketID     uuid.UUID         `json:"bucket_id"`
	Name         string            `json:"name"`
	Type         ContextBucketType `json:"type"`
	ItemCount    int               `json:"item_count"`
	LastUpdated  time.Time         `json:"last_updated"`
	SharePolicy  SharePolicy       `json:"share_policy"`
	Keys         []string          `json:"keys,omitempty"`
}

type WorkflowSummary struct {
	WorkflowID      uuid.UUID   `json:"workflow_id"`
	TaskID          uuid.UUID   `json:"task_id"`
	Title           string      `json:"title"`
	Category        string      `json:"category"`
	Status          string      `json:"status"`
	TotalTodos      int         `json:"total_todos"`
	CompletedTodos  int         `json:"completed_todos"`
	CurrentTodo     *TodoItem   `json:"current_todo,omitempty"`
	Progress        float64     `json:"progress"`
	CreatedAt       time.Time   `json:"created_at"`
	UpdatedAt       time.Time   `json:"updated_at"`
}

type NodeType string

const (
	NodeTypeTask           NodeType = "task"
	NodeTypeContextBucket  NodeType = "context_bucket"
	NodeTypeDecision       NodeType = "decision"
	NodeTypeMemory         NodeType = "memory"
	NodeTypeFile           NodeType = "file"
	NodeTypeTodo           NodeType = "todo"
	NodeTypeAgent          NodeType = "agent"
	NodeTypeEvent          NodeType = "event"
)

type EdgeType string

const (
	EdgeTypeDependsOn     EdgeType = "depends_on"
	EdgeTypeBlocks        EdgeType = "blocks"
	EdgeTypeRelatesTo     EdgeType = "relates_to"
	EdgeTypeContains      EdgeType = "contains"
	EdgeTypeReferences    EdgeType = "references"
	EdgeTypeDerivedFrom   EdgeType = "derived_from"
	EdgeTypeCausedBy      EdgeType = "caused_by"
	EdgeTypeTriggers      EdgeType = "triggers"
	EdgeTypeParentOf      EdgeType = "parent_of"
	EdgeTypeChildOf       EdgeType = "child_of"
	EdgeTypeNext          EdgeType = "next"
	EdgeTypePrevious      EdgeType = "previous"
)

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

type StoreInterface interface {
	CreateNode(ctx context.Context, node *Node) error
	GetNode(ctx context.Context, id uuid.UUID) (*Node, error)
	UpdateNode(ctx context.Context, node *Node) error
	DeleteNode(ctx context.Context, id uuid.UUID) error
	ListNodes(ctx context.Context, nodeType NodeType, limit, offset int) ([]*Node, error)
	CreateEdge(ctx context.Context, edge *Edge) error
	GetEdgesFrom(ctx context.Context, fromID uuid.UUID, edgeType EdgeType) ([]*Edge, error)
	GetEdgesTo(ctx context.Context, toID uuid.UUID, edgeType EdgeType) ([]*Edge, error)
	GetEdgesBetween(ctx context.Context, fromID, toID uuid.UUID) ([]*Edge, error)
	BeginTx(ctx context.Context) (*Tx, error)
}

type Tx struct {
	tx    interface{}
	store StoreInterface
}

func NewEdge(fromID, toID uuid.UUID, edgeType EdgeType, data interface{}) (*Edge, error) {
	dataJSON, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	return &Edge{
		ID:        uuid.New(),
		FromID:    fromID,
		ToID:      toID,
		Type:      edgeType,
		Data:      dataJSON,
		CreatedAt: time.Now(),
	}, nil
}