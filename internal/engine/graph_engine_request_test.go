package engine

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/iSundram/Automergent/internal/graph"
	"github.com/iSundram/Automergent/internal/graph/analysis"
)

func TestProcessUserRequestPersistsWorkflowWithoutApplyingWiring(t *testing.T) {
	cfg := DefaultGraphConfig()
	cfg.DatabasePath = filepath.Join(t.TempDir(), "graph.db")
	cfg.ProjectRoot = t.TempDir()
	engine, err := NewGraphEngine(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = engine.Close() })

	result, err := engine.ProcessUserRequest(context.Background(), "Add a config option and expose it in the TUI")
	if err != nil {
		t.Fatal(err)
	}
	if result.Analysis == nil || result.Analysis.Category != analysis.RequestCategoryUnknown {
		t.Fatalf("analysis = %+v", result.Analysis)
	}
	if result.IntegrationResult != nil {
		t.Fatal("analysis unexpectedly applied wiring")
	}
	if result.Appearance != nil {
		t.Fatal("graph preparation guessed and persisted a feature appearance")
	}
	taskID, err := uuid.Parse(result.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	node, err := engine.Manager.Store().GetNode(context.Background(), taskID)
	if err != nil {
		t.Fatal(err)
	}
	var task graph.Task
	if err := node.UnmarshalData(&task); err != nil {
		t.Fatal(err)
	}
	var metadata map[string]any
	if err := json.Unmarshal(task.Metadata, &metadata); err != nil {
		t.Fatal(err)
	}
	if _, exists := metadata["category"]; exists {
		t.Fatalf("tool category persisted in graph metadata: %v", metadata)
	}
	view, err := engine.RenderTaskGraph(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"relation: new_task", "entry points: tui, configuration", "todo [pending] Understand the request"} {
		if !strings.Contains(view, want) {
			t.Fatalf("graph view missing %q:\n%s", want, view)
		}
	}
}

func TestProcessUserRequestResumesFollowUpTask(t *testing.T) {
	cfg := DefaultGraphConfig()
	cfg.DatabasePath = filepath.Join(t.TempDir(), "graph.db")
	engine, err := NewGraphEngine(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = engine.Close() })

	first, err := engine.ProcessUserRequest(context.Background(), "Add thinking effort support")
	if err != nil {
		t.Fatal(err)
	}
	second, err := engine.ProcessUserRequest(context.Background(), "Continue, you missed the TUI wiring")
	if err != nil {
		t.Fatal(err)
	}
	if first.TaskID != second.TaskID {
		t.Fatalf("follow-up task id = %s, want %s", second.TaskID, first.TaskID)
	}
	if first.WorkflowID != second.WorkflowID {
		t.Fatalf("follow-up workflow id = %s, want resumed %s", second.WorkflowID, first.WorkflowID)
	}
	if second.Analysis.Context[0].Mode != analysis.ContextShareFull {
		t.Fatalf("follow-up sharing = %s, want full", second.Analysis.Context[0].Mode)
	}
	view, err := engine.RenderTaskGraph(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(view, "relation: follow_up  context: full") {
		t.Fatalf("follow-up graph not updated:\n%s", view)
	}
}

func TestProcessQuestionSkipsFeatureAppearancePipeline(t *testing.T) {
	cfg := DefaultGraphConfig()
	cfg.DatabasePath = filepath.Join(t.TempDir(), "nested", "graph.db")
	engine, err := NewGraphEngine(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = engine.Close() })

	result, err := engine.ProcessUserRequest(context.Background(), "Tell me which files implement the search tool")
	if err != nil {
		t.Fatal(err)
	}
	if result.Appearance != nil || result.WiringPattern != nil || len(result.EntryPoints) != 0 {
		t.Fatalf("question unexpectedly ran feature appearance pipeline: %+v", result)
	}
	if result.TaskID == "" || result.WorkflowID == "" {
		t.Fatalf("question did not persist graph task/workflow: %+v", result)
	}
}
