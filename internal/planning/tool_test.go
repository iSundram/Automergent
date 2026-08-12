package planning

import (
	"context"
	"testing"
)

func TestToolExecute(t *testing.T) {
	tool := NewTool(".")
	res, err := tool.Execute(context.Background(), map[string]any{"request": "plan internal/planning"})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	if res.Metadata == nil {
		t.Fatal("expected metadata")
	}
}
