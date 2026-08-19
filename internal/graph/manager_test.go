package graph

import (
	"context"
	"os"
	"testing"
)

func TestManagerIntegration(t *testing.T) {
	dbPath := "/tmp/test_graph_integration.db"
	os.Remove(dbPath)
	defer os.Remove(dbPath)

	mgr, err := NewManager(ManagerConfig{Path: dbPath})
	if err != nil {
		t.Fatalf("Error creating manager: %v", err)
	}
	defer mgr.Close()

	ctx := context.Background()

	task := &Task{
		Title:       "Test Task",
		Description: "A test task",
		Status:      "pending",
		Priority:    1,
		Tags:        []string{"test"},
	}
	node, err := mgr.CreateTask(ctx, task)
	if err != nil {
		t.Fatalf("Error creating task: %v", err)
	}

	task2 := &Task{
		Title:       "Dependent Task",
		Description: "Depends on first task",
		Status:      "pending",
		Priority:    2,
	}
	node2, err := mgr.CreateTask(ctx, task2)
	if err != nil {
		t.Fatalf("Error creating task2: %v", err)
	}

	edge, err := mgr.Link(ctx, node2.ID, node.ID, EdgeTypeDependsOn, nil)
	if err != nil {
		t.Fatalf("Error linking: %v", err)
	}
	if edge == nil {
		t.Fatal("Expected edge to be created")
	}

	deps, err := mgr.GetDependencies(ctx, node2.ID)
	if err != nil {
		t.Fatalf("Error getting dependencies: %v", err)
	}
	if len(deps) != 1 {
		t.Fatalf("Expected 1 dependency, got %d", len(deps))
	}
	if deps[0].ID != node.ID {
		t.Fatalf("Expected dependency to be first task")
	}

	result, err := mgr.TraverseFrom(ctx, node2.ID, TraversalOptions{
		MaxDepth:  5,
		Direction: TraversalOutbound,
	})
	if err != nil {
		t.Fatalf("Error traversing: %v", err)
	}
	if len(result.Nodes) != 1 {
		t.Fatalf("Expected 1 node in traversal, got %d", len(result.Nodes))
	}

	path, err := mgr.FindPath(ctx, node2.ID, node.ID, []EdgeType{EdgeTypeDependsOn})
	if err != nil {
		t.Fatalf("Error finding path: %v", err)
	}
	if len(path.Path) != 2 {
		t.Fatalf("Expected path of length 2, got %d", len(path.Path))
	}

	stats, err := mgr.GetStats(ctx)
	if err != nil {
		t.Fatalf("Error getting stats: %v", err)
	}
	if stats["task"] != 2 {
		t.Fatalf("Expected 2 tasks, got %d", stats["task"])
	}
	if stats["edges"] != 1 {
		t.Fatalf("Expected 1 edge, got %d", stats["edges"])
	}

	analysis, err := mgr.AnalyzeGraph(ctx)
	if err != nil {
		t.Fatalf("Error analyzing: %v", err)
	}
	if analysis.TaskComponents != 1 {
		t.Fatalf("Expected 1 component, got %d", analysis.TaskComponents)
	}

	bucket := &ContextBucket{
		Name:        "Test Bucket",
		Type:        ContextBucketTypeProject,
		Description: "A test bucket",
		SharePolicy: SharePolicyPrivate,
		Owner:       "test-user",
	}
	bucketNode, err := mgr.CreateContextBucket(ctx, bucket)
	if err != nil {
		t.Fatalf("Error creating bucket: %v", err)
	}

	err = mgr.AddToContextBucket(ctx, bucketNode.ID, node.ID)
	if err != nil {
		t.Fatalf("Error adding to bucket: %v", err)
	}

	children, err := mgr.GetContextBucketContents(ctx, bucketNode.ID)
	if err != nil {
		t.Fatalf("Error getting bucket contents: %v", err)
	}
	if len(children) != 1 {
		t.Fatalf("Expected 1 child in bucket, got %d", len(children))
	}
}

func TestStoreTransactions(t *testing.T) {
	dbPath := "/tmp/test_graph_tx.db"
	os.Remove(dbPath)
	defer os.Remove(dbPath)

	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("Error creating store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	tx, err := store.BeginTx(ctx)
	if err != nil {
		t.Fatalf("Error beginning transaction: %v", err)
	}

	task := &Task{
		Title:       "TX Task",
		Description: "In transaction",
		Status:      "pending",
		Priority:    1,
	}
	node, err := task.ToNode()
	if err != nil {
		t.Fatalf("Error creating node: %v", err)
	}

	if err := tx.CreateNode(ctx, node); err != nil {
		t.Fatalf("Error creating node in tx: %v", err)
	}

	if err := tx.Commit(); err != nil {
		t.Fatalf("Error committing tx: %v", err)
	}

	retrieved, err := store.GetNode(ctx, node.ID)
	if err != nil {
		t.Fatalf("Error retrieving node: %v", err)
	}
	if retrieved.ID != node.ID {
		t.Fatalf("Node ID mismatch")
	}

	tx2, err := store.BeginTx(ctx)
	if err != nil {
		t.Fatalf("Error beginning tx2: %v", err)
	}

	task2 := &Task{
		Title:       "TX Task 2",
		Description: "Will rollback",
		Status:      "pending",
		Priority:    1,
	}
	node2, err := task2.ToNode()
	if err != nil {
		t.Fatalf("Error creating node2: %v", err)
	}

	if err := tx2.CreateNode(ctx, node2); err != nil {
		t.Fatalf("Error creating node2 in tx: %v", err)
	}

	if err := tx2.Rollback(); err != nil {
		t.Fatalf("Error rolling back tx2: %v", err)
	}

	_, err = store.GetNode(ctx, node2.ID)
	if err != ErrNodeNotFound {
		t.Fatalf("Expected node2 to not exist after rollback")
	}
}

func TestQueryEngine(t *testing.T) {
	dbPath := "/tmp/test_graph_query.db"
	os.Remove(dbPath)
	defer os.Remove(dbPath)

	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("Error creating store: %v", err)
	}
	defer store.Close()

	query := NewQueryEngine(store)
	ctx := context.Background()

	task1 := &Task{Title: "Task 1", Status: "pending", Priority: 1}
	node1, _ := task1.ToNode()
	store.CreateNode(ctx, node1)

	task2 := &Task{Title: "Task 2", Status: "pending", Priority: 2}
	node2, _ := task2.ToNode()
	store.CreateNode(ctx, node2)

	task3 := &Task{Title: "Task 3", Status: "pending", Priority: 3}
	node3, _ := task3.ToNode()
	store.CreateNode(ctx, node3)

	edge1, _ := NewEdge(node1.ID, node2.ID, EdgeTypeDependsOn, nil)
	store.CreateEdge(ctx, edge1)
	edge2, _ := NewEdge(node2.ID, node3.ID, EdgeTypeDependsOn, nil)
	store.CreateEdge(ctx, edge2)

	result, err := query.Traverse(ctx, node1.ID, TraversalOptions{
		MaxDepth:    3,
		Direction:   TraversalOutbound,
		IncludeStart: false,
	})
	if err != nil {
		t.Fatalf("Traverse error: %v", err)
	}
	if len(result.Nodes) != 2 {
		t.Fatalf("Expected 2 nodes in traversal, got %d", len(result.Nodes))
	}

	path, err := query.ShortestPath(ctx, node1.ID, node3.ID, []EdgeType{EdgeTypeDependsOn})
	if err != nil {
		t.Fatalf("ShortestPath error: %v", err)
	}
	if len(path.Path) != 3 {
		t.Fatalf("Expected path of 3 nodes, got %d", len(path.Path))
	}

	neighbors, edges, err := query.GetNeighbors(ctx, node2.ID, TraversalBoth, []EdgeType{EdgeTypeDependsOn})
	if err != nil {
		t.Fatalf("GetNeighbors error: %v", err)
	}
	if len(neighbors) != 2 {
		t.Fatalf("Expected 2 neighbors, got %d", len(neighbors))
	}
	if len(edges) != 2 {
		t.Fatalf("Expected 2 edges, got %d", len(edges))
	}
}