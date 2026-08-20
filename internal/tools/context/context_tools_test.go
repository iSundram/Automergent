package contexttools_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/iSundram/Automergent/internal/engine"
	"github.com/iSundram/Automergent/internal/graph"
	"github.com/iSundram/Automergent/internal/graph/workflow"
	"github.com/iSundram/Automergent/internal/tools"
	contexttools "github.com/iSundram/Automergent/internal/tools/context"
)

func TestContextToolsCreateListAndRemember(t *testing.T) {
	cfg := engine.DefaultGraphConfig()
	cfg.DatabasePath = filepath.Join(t.TempDir(), "graph.db")
	graphEngine, err := engine.NewGraphEngine(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = graphEngine.Close() })

	taskNode, err := graphEngine.Manager.CreateTask(context.Background(), &graph.Task{Title: "test", Description: "test"})
	if err != nil {
		t.Fatal(err)
	}
	wf, err := graphEngine.TodoEngine.CreateWorkflow(context.Background(), taskNode.ID, "feature", "feature", "feature")
	if err != nil {
		t.Fatal(err)
	}
	todo, err := graphEngine.TodoEngine.AddTodo(context.Background(), wf.ID, "implement", "implement", nil, workflow.SharePolicyPartial, []string{"decisions"})
	if err != nil {
		t.Fatal(err)
	}

	reg := tools.NewRegistry()
	contexttools.Register(reg, graphEngine.BucketManager, graphEngine.RememberTool)
	create, ok := reg.Get("context_bucket_create")
	if !ok {
		t.Fatal("create tool not registered")
	}
	result, err := create.Execute(context.Background(), map[string]any{"task_id": taskNode.ID.String(), "name": "phase-1", "type": "temporary", "share_policy": "partial"})
	if err != nil || result.IsError {
		t.Fatalf("create failed: %v %s", err, result.Content)
	}
	var created workflow.ContextBucket
	if err := json.Unmarshal([]byte(result.Content), &created); err != nil {
		t.Fatal(err)
	}

	remember, ok := reg.Get("remember")
	if !ok {
		t.Fatal("remember tool not registered")
	}
	result, err = remember.Execute(context.Background(), map[string]any{"todo_id": todo.ID.String(), "message": "Preserve the TUI wiring decision for verification."})
	if err != nil || result.IsError {
		t.Fatalf("remember failed: %v %s", err, result.Content)
	}
	messages, err := graphEngine.RememberTool.GetInjectedMessages(context.Background(), todo.ID)
	if err != nil || len(messages) != 1 {
		t.Fatalf("injected messages = %+v, err=%v", messages, err)
	}

	list, ok := reg.Get("context_bucket_list")
	if !ok {
		t.Fatal("list tool not registered")
	}
	result, err = list.Execute(context.Background(), map[string]any{"task_id": taskNode.ID.String()})
	if err != nil || result.IsError || created.ID.String() == "" {
		t.Fatalf("list failed: %v %s", err, result.Content)
	}
}
