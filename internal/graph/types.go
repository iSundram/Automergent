package graph

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

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

type ContextBucketType string

const (
	ContextBucketTypeProject     ContextBucketType = "project"
	ContextBucketTypeSession     ContextBucketType = "session"
	ContextBucketTypeTask        ContextBucketType = "task"
	ContextBucketTypeAgent       ContextBucketType = "agent"
	ContextBucketTypeGlobal      ContextBucketType = "global"
	ContextBucketTypeTemporary   ContextBucketType = "temporary"
)

type SharePolicy string

const (
	SharePolicyPrivate SharePolicy = "private"
	SharePolicyShared  SharePolicy = "shared"
	SharePolicyPublic  SharePolicy = "public"
)

type DecisionType string

const (
	DecisionTypeArchitecture DecisionType = "architecture"
	DecisionTypeDesign       DecisionType = "design"
	DecisionTypeTechnical    DecisionType = "technical"
	DecisionTypeProcess      DecisionType = "process"
	DecisionTypeTooling      DecisionType = "tooling"
	DecisionTypeSecurity     DecisionType = "security"
	DecisionTypePerformance  DecisionType = "performance"
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
	MemoryScopeGlobal    MemoryScope = "global"
	MemoryScopeProject   MemoryScope = "project"
	MemoryScopeSession   MemoryScope = "session"
	MemoryScopeAgent     MemoryScope = "agent"
	MemoryScopeTask      MemoryScope = "task"
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

type Task struct {
	ID          uuid.UUID `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Status      string    `json:"status"`
	Priority    int       `json:"priority"`
	Assignee    string    `json:"assignee,omitempty"`
	Tags        []string  `json:"tags,omitempty"`
	Metadata    json.RawMessage `json:"metadata,omitempty"`
}

type ContextBucket struct {
	ID          uuid.UUID         `json:"id"`
	Name        string            `json:"name"`
	Type        ContextBucketType `json:"type"`
	Description string            `json:"description"`
	SharePolicy SharePolicy       `json:"share_policy"`
	Owner       string            `json:"owner"`
	Tags        []string          `json:"tags,omitempty"`
	Metadata    json.RawMessage   `json:"metadata,omitempty"`
}

type Decision struct {
	ID          uuid.UUID     `json:"id"`
	Title       string        `json:"title"`
	Description string        `json:"description"`
	Type        DecisionType  `json:"type"`
	Status      string        `json:"status"`
	Options     []string      `json:"options,omitempty"`
	Rationale   string        `json:"rationale,omitempty"`
	Outcome     string        `json:"outcome,omitempty"`
	Tags        []string      `json:"tags,omitempty"`
	Metadata    json.RawMessage `json:"metadata,omitempty"`
}

type Memory struct {
	ID        uuid.UUID     `json:"id"`
	Content   string        `json:"content"`
	Type      MemoryType    `json:"type"`
	Scope     MemoryScope   `json:"scope"`
	Tags      []string      `json:"tags,omitempty"`
	Source    string        `json:"source,omitempty"`
	Confidence float64      `json:"confidence"`
	Metadata  json.RawMessage `json:"metadata,omitempty"`
}

type File struct {
	ID        uuid.UUID `json:"id"`
	Path      string    `json:"path"`
	Name      string    `json:"name"`
	Extension string    `json:"extension"`
	Size      int64     `json:"size"`
	Hash      string    `json:"hash,omitempty"`
	Language  string    `json:"language,omitempty"`
	Metadata  json.RawMessage `json:"metadata,omitempty"`
}

type Todo struct {
	ID          uuid.UUID `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Completed   bool      `json:"completed"`
	Priority    int       `json:"priority"`
	DueDate     *time.Time `json:"due_date,omitempty"`
	Tags        []string  `json:"tags,omitempty"`
	Metadata    json.RawMessage `json:"metadata,omitempty"`
}

type Agent struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Type        string    `json:"type"`
	Status      string    `json:"status"`
	Capabilities []string `json:"capabilities,omitempty"`
	Config      json.RawMessage `json:"config,omitempty"`
	Metadata    json.RawMessage `json:"metadata,omitempty"`
}

type Event struct {
	ID        uuid.UUID       `json:"id"`
	Type      string          `json:"type"`
	Source    string          `json:"source"`
	Target    string          `json:"target,omitempty"`
	Payload   json.RawMessage `json:"payload"`
	Timestamp time.Time       `json:"timestamp"`
	Metadata  json.RawMessage `json:"metadata,omitempty"`
}

func NewNode(nodeType NodeType, data interface{}) (*Node, error) {
	dataJSON, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	return &Node{
		ID:        uuid.New(),
		Type:      nodeType,
		Data:      dataJSON,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
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

func (n *Node) UnmarshalData(v interface{}) error {
	return json.Unmarshal(n.Data, v)
}

func (e *Edge) UnmarshalData(v interface{}) error {
	return json.Unmarshal(e.Data, v)
}

func (t *Task) ToNode() (*Node, error) {
	return NewNode(NodeTypeTask, t)
}

func (c *ContextBucket) ToNode() (*Node, error) {
	return NewNode(NodeTypeContextBucket, c)
}

func (d *Decision) ToNode() (*Node, error) {
	return NewNode(NodeTypeDecision, d)
}

func (m *Memory) ToNode() (*Node, error) {
	return NewNode(NodeTypeMemory, m)
}

func (f *File) ToNode() (*Node, error) {
	return NewNode(NodeTypeFile, f)
}

func (t *Todo) ToNode() (*Node, error) {
	return NewNode(NodeTypeTodo, t)
}

func (a *Agent) ToNode() (*Node, error) {
	return NewNode(NodeTypeAgent, a)
}

func (e *Event) ToNode() (*Node, error) {
	return NewNode(NodeTypeEvent, e)
}