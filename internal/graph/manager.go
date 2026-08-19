package graph

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

type GraphManager struct {
	store  *Store
	query  *QueryEngine
	db     *sql.DB
	path   string
	mu     sync.RWMutex
	closed bool
}

type ManagerConfig struct {
	Path string
}

func NewManager(config ManagerConfig) (*GraphManager, error) {
	db, err := OpenDB(config.Path)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	store := &Store{db: db, path: config.Path}
	query := NewQueryEngine(store)

	return &GraphManager{
		store: store,
		query: query,
		db:    db,
		path:  config.Path,
	}, nil
}

func (m *GraphManager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil
	}
	m.closed = true
	return m.db.Close()
}

func (m *GraphManager) Store() *Store {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.store
}

func (m *GraphManager) Query() *QueryEngine {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.query
}

func (m *GraphManager) CreateTask(ctx context.Context, task *Task) (*Node, error) {
	node, err := task.ToNode()
	if err != nil {
		return nil, err
	}
	return node, m.store.CreateNode(ctx, node)
}

func (m *GraphManager) CreateContextBucket(ctx context.Context, bucket *ContextBucket) (*Node, error) {
	node, err := bucket.ToNode()
	if err != nil {
		return nil, err
	}
	return node, m.store.CreateNode(ctx, node)
}

func (m *GraphManager) CreateDecision(ctx context.Context, decision *Decision) (*Node, error) {
	node, err := decision.ToNode()
	if err != nil {
		return nil, err
	}
	return node, m.store.CreateNode(ctx, node)
}

func (m *GraphManager) CreateMemory(ctx context.Context, memory *Memory) (*Node, error) {
	node, err := memory.ToNode()
	if err != nil {
		return nil, err
	}
	return node, m.store.CreateNode(ctx, node)
}

func (m *GraphManager) CreateFile(ctx context.Context, file *File) (*Node, error) {
	node, err := file.ToNode()
	if err != nil {
		return nil, err
	}
	return node, m.store.CreateNode(ctx, node)
}

func (m *GraphManager) CreateTodo(ctx context.Context, todo *Todo) (*Node, error) {
	node, err := todo.ToNode()
	if err != nil {
		return nil, err
	}
	return node, m.store.CreateNode(ctx, node)
}

func (m *GraphManager) CreateAgent(ctx context.Context, agent *Agent) (*Node, error) {
	node, err := agent.ToNode()
	if err != nil {
		return nil, err
	}
	return node, m.store.CreateNode(ctx, node)
}

func (m *GraphManager) CreateEvent(ctx context.Context, event *Event) (*Node, error) {
	node, err := event.ToNode()
	if err != nil {
		return nil, err
	}
	return node, m.store.CreateNode(ctx, node)
}

func (m *GraphManager) GetTask(ctx context.Context, id uuid.UUID) (*Task, error) {
	node, err := m.store.GetNode(ctx, id)
	if err != nil {
		return nil, err
	}
	if node.Type != NodeTypeTask {
		return nil, fmt.Errorf("node is not a task")
	}
	var task Task
	if err := node.UnmarshalData(&task); err != nil {
		return nil, err
	}
	task.ID = node.ID
	return &task, nil
}

func (m *GraphManager) GetContextBucket(ctx context.Context, id uuid.UUID) (*ContextBucket, error) {
	node, err := m.store.GetNode(ctx, id)
	if err != nil {
		return nil, err
	}
	if node.Type != NodeTypeContextBucket {
		return nil, fmt.Errorf("node is not a context bucket")
	}
	var bucket ContextBucket
	if err := node.UnmarshalData(&bucket); err != nil {
		return nil, err
	}
	bucket.ID = node.ID
	return &bucket, nil
}

func (m *GraphManager) GetDecision(ctx context.Context, id uuid.UUID) (*Decision, error) {
	node, err := m.store.GetNode(ctx, id)
	if err != nil {
		return nil, err
	}
	if node.Type != NodeTypeDecision {
		return nil, fmt.Errorf("node is not a decision")
	}
	var decision Decision
	if err := node.UnmarshalData(&decision); err != nil {
		return nil, err
	}
	decision.ID = node.ID
	return &decision, nil
}

func (m *GraphManager) GetMemory(ctx context.Context, id uuid.UUID) (*Memory, error) {
	node, err := m.store.GetNode(ctx, id)
	if err != nil {
		return nil, err
	}
	if node.Type != NodeTypeMemory {
		return nil, fmt.Errorf("node is not a memory")
	}
	var memory Memory
	if err := node.UnmarshalData(&memory); err != nil {
		return nil, err
	}
	memory.ID = node.ID
	return &memory, nil
}

func (m *GraphManager) GetFile(ctx context.Context, id uuid.UUID) (*File, error) {
	node, err := m.store.GetNode(ctx, id)
	if err != nil {
		return nil, err
	}
	if node.Type != NodeTypeFile {
		return nil, fmt.Errorf("node is not a file")
	}
	var file File
	if err := node.UnmarshalData(&file); err != nil {
		return nil, err
	}
	file.ID = node.ID
	return &file, nil
}

func (m *GraphManager) GetTodo(ctx context.Context, id uuid.UUID) (*Todo, error) {
	node, err := m.store.GetNode(ctx, id)
	if err != nil {
		return nil, err
	}
	if node.Type != NodeTypeTodo {
		return nil, fmt.Errorf("node is not a todo")
	}
	var todo Todo
	if err := node.UnmarshalData(&todo); err != nil {
		return nil, err
	}
	todo.ID = node.ID
	return &todo, nil
}

func (m *GraphManager) GetAgent(ctx context.Context, id uuid.UUID) (*Agent, error) {
	node, err := m.store.GetNode(ctx, id)
	if err != nil {
		return nil, err
	}
	if node.Type != NodeTypeAgent {
		return nil, fmt.Errorf("node is not an agent")
	}
	var agent Agent
	if err := node.UnmarshalData(&agent); err != nil {
		return nil, err
	}
	agent.ID = node.ID
	return &agent, nil
}

func (m *GraphManager) GetEvent(ctx context.Context, id uuid.UUID) (*Event, error) {
	node, err := m.store.GetNode(ctx, id)
	if err != nil {
		return nil, err
	}
	if node.Type != NodeTypeEvent {
		return nil, fmt.Errorf("node is not an event")
	}
	var event Event
	if err := node.UnmarshalData(&event); err != nil {
		return nil, err
	}
	event.ID = node.ID
	return &event, nil
}

func (m *GraphManager) Link(ctx context.Context, fromID, toID uuid.UUID, edgeType EdgeType, data interface{}) (*Edge, error) {
	edge, err := NewEdge(fromID, toID, edgeType, data)
	if err != nil {
		return nil, err
	}
	return edge, m.store.CreateEdge(ctx, edge)
}

func (m *GraphManager) Unlink(ctx context.Context, edgeID uuid.UUID) error {
	return m.store.DeleteEdge(ctx, edgeID)
}

func (m *GraphManager) GetDependencies(ctx context.Context, taskID uuid.UUID) ([]*Task, error) {
	edges, err := m.store.GetEdgesFrom(ctx, taskID, EdgeTypeDependsOn)
	if err != nil {
		return nil, err
	}

	var tasks []*Task
	for _, edge := range edges {
		task, err := m.GetTask(ctx, edge.ToID)
		if err == nil {
			tasks = append(tasks, task)
		}
	}
	return tasks, nil
}

func (m *GraphManager) GetDependents(ctx context.Context, taskID uuid.UUID) ([]*Task, error) {
	edges, err := m.store.GetEdgesTo(ctx, taskID, EdgeTypeDependsOn)
	if err != nil {
		return nil, err
	}

	var tasks []*Task
	for _, edge := range edges {
		task, err := m.GetTask(ctx, edge.FromID)
		if err == nil {
			tasks = append(tasks, task)
		}
	}
	return tasks, nil
}

func (m *GraphManager) GetBlockers(ctx context.Context, taskID uuid.UUID) ([]*Task, error) {
	edges, err := m.store.GetEdgesFrom(ctx, taskID, EdgeTypeBlocks)
	if err != nil {
		return nil, err
	}

	var tasks []*Task
	for _, edge := range edges {
		task, err := m.GetTask(ctx, edge.ToID)
		if err == nil {
			tasks = append(tasks, task)
		}
	}
	return tasks, nil
}

func (m *GraphManager) GetRelated(ctx context.Context, nodeID uuid.UUID, nodeType NodeType) ([]*Node, error) {
	edges, err := m.store.GetEdgesFrom(ctx, nodeID, EdgeTypeRelatesTo)
	if err != nil {
		return nil, err
	}

	var nodes []*Node
	for _, edge := range edges {
		node, err := m.store.GetNode(ctx, edge.ToID)
		if err == nil && (nodeType == "" || node.Type == nodeType) {
			nodes = append(nodes, node)
		}
	}
	return nodes, nil
}

func (m *GraphManager) GetChildren(ctx context.Context, parentID uuid.UUID, childType NodeType) ([]*Node, error) {
	edges, err := m.store.GetEdgesFrom(ctx, parentID, EdgeTypeParentOf)
	if err != nil {
		return nil, err
	}

	var nodes []*Node
	for _, edge := range edges {
		node, err := m.store.GetNode(ctx, edge.ToID)
		if err == nil && (childType == "" || node.Type == childType) {
			nodes = append(nodes, node)
		}
	}
	return nodes, nil
}

func (m *GraphManager) GetParents(ctx context.Context, childID uuid.UUID, parentType NodeType) ([]*Node, error) {
	edges, err := m.store.GetEdgesTo(ctx, childID, EdgeTypeParentOf)
	if err != nil {
		return nil, err
	}

	var nodes []*Node
	for _, edge := range edges {
		node, err := m.store.GetNode(ctx, edge.FromID)
		if err == nil && (parentType == "" || node.Type == parentType) {
			nodes = append(nodes, node)
		}
	}
	return nodes, nil
}

func (m *GraphManager) GetContextBucketContents(ctx context.Context, bucketID uuid.UUID) ([]*Node, error) {
	edges, err := m.store.GetEdgesFrom(ctx, bucketID, EdgeTypeContains)
	if err != nil {
		return nil, err
	}

	var nodes []*Node
	for _, edge := range edges {
		node, err := m.store.GetNode(ctx, edge.ToID)
		if err == nil {
			nodes = append(nodes, node)
		}
	}
	return nodes, nil
}

func (m *GraphManager) AddToContextBucket(ctx context.Context, bucketID, nodeID uuid.UUID) error {
	_, err := m.Link(ctx, bucketID, nodeID, EdgeTypeContains, nil)
	return err
}

func (m *GraphManager) RemoveFromContextBucket(ctx context.Context, bucketID, nodeID uuid.UUID) error {
	edges, err := m.store.GetEdgesBetween(ctx, bucketID, nodeID)
	if err != nil {
		return err
	}
	for _, edge := range edges {
		if edge.Type == EdgeTypeContains {
			if err := m.Unlink(ctx, edge.ID); err != nil {
				return err
			}
		}
	}
	return nil
}

func (m *GraphManager) SearchNodes(ctx context.Context, nodeType NodeType, filter func(*Node) bool, limit int) ([]*Node, error) {
	nodes, err := m.store.ListNodes(ctx, nodeType, limit*10, 0)
	if err != nil {
		return nil, err
	}

	var results []*Node
	for _, node := range nodes {
		if filter(node) {
			results = append(results, node)
			if len(results) >= limit {
				break
			}
		}
	}
	return results, nil
}

func (m *GraphManager) TraverseFrom(ctx context.Context, startID uuid.UUID, opts TraversalOptions) (*TraversalResult, error) {
	return m.query.Traverse(ctx, startID, opts)
}

func (m *GraphManager) FindPath(ctx context.Context, from, to uuid.UUID, edgeTypes []EdgeType) (*ShortestPathResult, error) {
	return m.query.ShortestPath(ctx, from, to, edgeTypes)
}

func (m *GraphManager) GetSubgraph(ctx context.Context, nodeIDs []uuid.UUID) (*Subgraph, error) {
	return m.query.ExtractSubgraph(ctx, nodeIDs, true)
}

func (m *GraphManager) GetStats(ctx context.Context) (map[string]int, error) {
	stats := make(map[string]int)

	nodeTypes := []NodeType{
		NodeTypeTask, NodeTypeContextBucket, NodeTypeDecision,
		NodeTypeMemory, NodeTypeFile, NodeTypeTodo, NodeTypeAgent, NodeTypeEvent,
	}

	for _, nt := range nodeTypes {
		count, err := m.store.CountNodes(ctx, nt)
		if err != nil {
			return nil, err
		}
		stats[string(nt)] = count
	}

	var edgeCount int
	err := m.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM edges").Scan(&edgeCount)
	if err != nil {
		return nil, err
	}
	stats["edges"] = edgeCount

	return stats, nil
}

func (m *GraphManager) Backup(ctx context.Context, destPath string) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed {
		return fmt.Errorf("manager closed")
	}

	_, err := m.db.ExecContext(ctx, fmt.Sprintf("VACUUM INTO '%s'", destPath))
	return err
}

func (m *GraphManager) Vacuum(ctx context.Context) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed {
		return fmt.Errorf("manager closed")
	}

	_, err := m.db.ExecContext(ctx, "VACUUM")
	return err
}

func (m *GraphManager) AnalyzeGraph(ctx context.Context) (*GraphAnalysis, error) {
	stats, err := m.GetStats(ctx)
	if err != nil {
		return nil, err
	}

	components, err := m.query.GetConnectedComponents(ctx, NodeTypeTask)
	if err != nil {
		return nil, err
	}

	var orphanedTasks int
	for _, comp := range components {
		if len(comp) == 1 {
			orphanedTasks++
		}
	}

	return &GraphAnalysis{
		NodeCounts:      stats,
		TaskComponents:  len(components),
		OrphanedTasks:   orphanedTasks,
		GeneratedAt:     time.Now(),
	}, nil
}

type GraphAnalysis struct {
	NodeCounts     map[string]int
	TaskComponents int
	OrphanedTasks  int
	GeneratedAt    time.Time
}

func (m *GraphManager) WithTx(ctx context.Context, fn func(*Tx) error) error {
	tx, err := m.store.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := fn(tx); err != nil {
		return err
	}

	return tx.Commit()
}

type BatchOperation struct {
	Nodes []*Node
	Edges []*Edge
}

func (m *GraphManager) BatchCreate(ctx context.Context, batch BatchOperation) error {
	return m.WithTx(ctx, func(tx *Tx) error {
		for _, node := range batch.Nodes {
			if err := tx.CreateNode(ctx, node); err != nil {
				return err
			}
		}
		for _, edge := range batch.Edges {
			if err := tx.CreateEdge(ctx, edge); err != nil {
				return err
			}
		}
		return nil
	})
}