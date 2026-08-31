package filesystem

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iSundram/Automergent/internal/config"
	"github.com/iSundram/Automergent/internal/tools"
)

func TestArtifactToolWritesWithMetadata(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".automergent", "artifacts", "plan.md")
	tool := NewArtifactTool(config.Default())

	res, err := tool.Execute(context.Background(), map[string]any{
		"path":             path,
		"content":          "# Migration plan\n\nStep one.",
		"title":            "Migration plan",
		"kind":             "plan",
		"request_feedback": true,
	})
	if err != nil || res.IsError {
		t.Fatalf("execute failed: %v %s", err, res.Content)
	}
	data, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(data), "Migration plan") {
		t.Fatalf("artifact not written: %v", err)
	}
	if res.Metadata["artifact_title"] != "Migration plan" ||
		res.Metadata["artifact_kind"] != "plan" ||
		res.Metadata["request_feedback"] != true {
		t.Fatalf("metadata missing: %+v", res.Metadata)
	}
	if !strings.Contains(res.Content, "Stop calling tools") {
		t.Fatalf("request_feedback must instruct the model to stop:\n%s", res.Content)
	}
}

func TestArtifactToolRequiresPath(t *testing.T) {
	tool := NewArtifactTool(config.Default())
	res, _ := tool.Execute(context.Background(), map[string]any{})
	if !res.IsError {
		t.Fatal("expected error without path")
	}
}

func TestNotebookEditReplaceInsertDelete(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nb.ipynb")
	nb := `{"cells":[{"cell_type":"code","id":"a","source":["print(1)\n"],"outputs":[],"execution_count":1},{"cell_type":"markdown","id":"b","source":["# hi"]}]}`
	if err := os.WriteFile(path, []byte(nb), 0o644); err != nil {
		t.Fatal(err)
	}
	tool := NewNotebookEditTool(config.Default())
	ctx := context.Background()

	// Replace by cell id.
	if res, _ := tool.Execute(ctx, map[string]any{
		"path": path, "cell_id": "a", "new_source": "print(2)",
	}); res.IsError {
		t.Fatalf("replace failed: %s", res.Content)
	}
	// Insert a markdown cell after index 0.
	if res, _ := tool.Execute(ctx, map[string]any{
		"path": path, "cell_index": 1, "edit_mode": "insert",
		"new_source": "between", "cell_type": "markdown",
	}); res.IsError {
		t.Fatalf("insert failed: %s", res.Content)
	}
	// Delete the original markdown cell (now index 2, id "b").
	if res, _ := tool.Execute(ctx, map[string]any{
		"path": path, "cell_id": "b", "edit_mode": "delete",
	}); res.IsError {
		t.Fatalf("delete failed: %s", res.Content)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "print(2)") || !strings.Contains(content, "between") || strings.Contains(content, "# hi") {
		t.Fatalf("notebook state wrong:\n%s", content)
	}
	// Source must be stored in line-array form.
	if !strings.Contains(content, `"print(2)"`) {
		t.Fatalf("source not stored as array:\n%s", content)
	}
}

func TestNotebookEditRejectsNonNotebook(t *testing.T) {
	tool := NewNotebookEditTool(config.Default())
	res, _ := tool.Execute(context.Background(), map[string]any{"path": "x.txt"})
	if !res.IsError {
		t.Fatal("expected error for non-.ipynb path")
	}
	_ = tools.Result{} // keep import when assertions are compiled out
}
