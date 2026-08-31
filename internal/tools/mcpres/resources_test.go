package mcpres

import (
	"context"
	"strings"
	"testing"

	"github.com/iSundram/Automergent/internal/mcp"
)

type fakeResources struct {
	listed []mcp.ResourceInfo
	blocks map[string][]mcp.ContentBlock
}

func (f *fakeResources) ListResources() []mcp.ResourceInfo { return f.listed }
func (f *fakeResources) ReadResource(_ context.Context, uri string) ([]mcp.ContentBlock, error) {
	return f.blocks[uri], nil
}

func TestListAndReadResources(t *testing.T) {
	res := &fakeResources{
		listed: []mcp.ResourceInfo{{URI: "db://schema", Server: "db", Description: "the schema"}},
		blocks: map[string][]mcp.ContentBlock{
			"db://schema": {{Type: "text", Text: "CREATE TABLE users..."}},
		},
	}

	lr, err := NewListTool(res).Execute(context.Background(), nil)
	if err != nil || lr.IsError {
		t.Fatalf("list failed: %v %s", err, lr.Content)
	}
	if !strings.Contains(lr.Content, "db://schema") || !strings.Contains(lr.Content, "the schema") {
		t.Fatalf("list output wrong:\n%s", lr.Content)
	}

	rr, err := NewReadTool(res).Execute(context.Background(), map[string]any{"uri": "db://schema"})
	if err != nil || rr.IsError {
		t.Fatalf("read failed: %v %s", err, rr.Content)
	}
	if !strings.Contains(rr.Content, "CREATE TABLE") {
		t.Fatalf("read output wrong:\n%s", rr.Content)
	}
}

func TestEmptyOrMissingOrchestrator(t *testing.T) {
	lr, _ := NewListTool(nil).Execute(context.Background(), nil)
	if strings.Contains(lr.Content, "\t") && !strings.Contains(lr.Content, "no MCP servers") && !strings.Contains(lr.Content, "no resources") {
		t.Fatalf("nil orchestrator must degrade: %s", lr.Content)
	}
	rr, _ := NewReadTool(nil).Execute(context.Background(), map[string]any{"uri": "x://y"})
	if !rr.IsError {
		t.Fatal("read with nil orchestrator must error")
	}
	rr2, _ := NewReadTool(&fakeResources{}).Execute(context.Background(), map[string]any{})
	if !rr2.IsError {
		t.Fatal("read without uri must error")
	}
}
