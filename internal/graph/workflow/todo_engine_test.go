package workflow

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/iSundram/Automergent/internal/graph"
)

type graphStoreAdapter struct{ store *graph.Store }

func (a graphStoreAdapter) CreateNode(ctx context.Context, node *Node) error {
	return a.store.CreateNode(ctx, toGraphNode(node))
}
func (a graphStoreAdapter) GetNode(ctx context.Context, id uuid.UUID) (*Node, error) {
	node, err := a.store.GetNode(ctx, id)
	if err != nil {
		return nil, err
	}
	return fromGraphNode(node), nil
}
func (a graphStoreAdapter) UpdateNode(ctx context.Context, node *Node) error {
	return a.store.UpdateNode(ctx, toGraphNode(node))
}
func (a graphStoreAdapter) DeleteNode(ctx context.Context, id uuid.UUID) error {
	return a.store.DeleteNode(ctx, id)
}
func (a graphStoreAdapter) ListNodes(ctx context.Context, nodeType NodeType, limit, offset int) ([]*Node, error) {
	nodes, err := a.store.ListNodes(ctx, graph.NodeType(nodeType), limit, offset)
	if err != nil {
		return nil, err
	}
	result := make([]*Node, len(nodes))
	for i, node := range nodes {
		result[i] = fromGraphNode(node)
	}
	return result, nil
}
func (a graphStoreAdapter) CreateEdge(ctx context.Context, edge *Edge) error {
	return a.store.CreateEdge(ctx, toGraphEdge(edge))
}
func (a graphStoreAdapter) GetEdgesFrom(ctx context.Context, id uuid.UUID, edgeType EdgeType) ([]*Edge, error) {
	return convertEdges(a.store.GetEdgesFrom(ctx, id, graph.EdgeType(edgeType)))
}
func (a graphStoreAdapter) GetEdgesTo(ctx context.Context, id uuid.UUID, edgeType EdgeType) ([]*Edge, error) {
	return convertEdges(a.store.GetEdgesTo(ctx, id, graph.EdgeType(edgeType)))
}
func (a graphStoreAdapter) GetEdgesBetween(ctx context.Context, from, to uuid.UUID) ([]*Edge, error) {
	return convertEdges(a.store.GetEdgesBetween(ctx, from, to))
}
func (a graphStoreAdapter) BeginTx(context.Context) (*Tx, error) { return nil, nil }

func toGraphNode(node *Node) *graph.Node {
	return &graph.Node{ID: node.ID, Type: graph.NodeType(node.Type), Data: node.Data, CreatedAt: node.CreatedAt, UpdatedAt: node.UpdatedAt}
}
func fromGraphNode(node *graph.Node) *Node {
	return &Node{ID: node.ID, Type: NodeType(node.Type), Data: node.Data, CreatedAt: node.CreatedAt, UpdatedAt: node.UpdatedAt}
}
func toGraphEdge(edge *Edge) *graph.Edge {
	return &graph.Edge{ID: edge.ID, FromID: edge.FromID, ToID: edge.ToID, Type: graph.EdgeType(edge.Type), Data: edge.Data, CreatedAt: edge.CreatedAt}
}
func convertEdges(edges []*graph.Edge, err error) ([]*Edge, error) {
	if err != nil {
		return nil, err
	}
	result := make([]*Edge, len(edges))
	for i, edge := range edges {
		result[i] = &Edge{ID: edge.ID, FromID: edge.FromID, ToID: edge.ToID, Type: EdgeType(edge.Type), Data: edge.Data, CreatedAt: edge.CreatedAt}
	}
	return result, nil
}

func TestTodoStatusUpdatesPreserveNodeIdentity(t *testing.T) {
	store, err := graph.NewStore(filepath.Join(t.TempDir(), "graph.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	adapter := graphStoreAdapter{store: store}
	buckets := NewContextBucketManager(adapter)
	engine := NewTodoWorkflowEngine(adapter, buckets)

	taskID := uuid.New()
	if err := store.CreateNode(context.Background(), &graph.Node{ID: taskID, Type: graph.NodeTypeTask, Data: []byte(`{}`)}); err != nil {
		t.Fatal(err)
	}
	workflow, err := engine.CreateWorkflow(context.Background(), taskID, "feature", "feature", "feature")
	if err != nil {
		t.Fatal(err)
	}
	todo, err := engine.AddTodo(context.Background(), workflow.ID, "analyze", "analysis", nil, SharePolicyPartial, []string{"request"})
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.MarkTodoStatus(context.Background(), todo.ID, TodoStatusInProgress); err != nil {
		t.Fatal(err)
	}
	if err := engine.MarkTodoStatus(context.Background(), todo.ID, TodoStatusDone); err != nil {
		t.Fatal(err)
	}

	stored, err := store.GetNode(context.Background(), todo.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.ID != todo.ID {
		t.Fatalf("todo identity changed: %s != %s", stored.ID, todo.ID)
	}
	edges, err := store.GetEdgesFrom(context.Background(), taskID, graph.EdgeTypeParentOf)
	if err != nil || len(edges) != 1 || edges[0].ToID != workflow.ID {
		t.Fatalf("task workflow edges = %+v, err=%v", edges, err)
	}
}
