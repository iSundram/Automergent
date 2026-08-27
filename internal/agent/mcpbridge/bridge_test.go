package mcpbridge

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/iSundram/Automergent/internal/mcp"
	"github.com/iSundram/Automergent/internal/tools"
)

// mockOrchestrator implements OrchestratorAPI for testing.
type mockOrchestrator struct {
	tools []mcp.ToolInfo
}

func (m *mockOrchestrator) CallTool(ctx context.Context, call mcp.ToolCall) (*mcp.ToolResult, error) {
	return &mcp.ToolResult{
		Content: []mcp.ContentBlock{
			{Type: "text", Text: "result from " + call.Name},
		},
		IsError: false,
		Server:  call.Server,
	}, nil
}

func (m *mockOrchestrator) ListTools() []mcp.ToolInfo {
	return m.tools
}

func (m *mockOrchestrator) GetTool(name string) (mcp.ToolInfo, bool) {
	for _, t := range m.tools {
		if t.Name == name || t.QualifiedName == name {
			return t, true
		}
	}
	return mcp.ToolInfo{}, false
}

func (m *mockOrchestrator) ListServers() []mcp.ServerInfo {
	return nil
}

func TestBridgeToolName(t *testing.T) {
	bt := &BridgeTool{
		info: mcp.ToolInfo{
			Name:          "read_file",
			Server:        "fs-server",
			QualifiedName: "fs-server/read_file",
		},
	}
	expected := "mcp__fs-server/read_file"
	if bt.Name() != expected {
		t.Errorf("expected name %q, got %q", expected, bt.Name())
	}
}

func TestBridgeToolDescription(t *testing.T) {
	bt := &BridgeTool{
		info: mcp.ToolInfo{
			Name:        "read_file",
			Description: "Read a file from disk",
			Server:      "fs-server",
		},
	}
	desc := bt.Description()
	if desc != "[MCP:fs-server] Read a file from disk" {
		t.Errorf("unexpected description: %q", desc)
	}
}

func TestBridgeToolSchema(t *testing.T) {
	schemaJSON := `{"type":"object","properties":{"path":{"type":"string"}}}`
	bt := &BridgeTool{
		info: mcp.ToolInfo{
			Name:        "read_file",
			InputSchema: json.RawMessage(schemaJSON),
		},
	}
	schema := bt.Schema()
	if schema["type"] != "object" {
		t.Errorf("expected type 'object', got %v", schema["type"])
	}
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatal("expected properties to be a map")
	}
	if _, ok := props["path"]; !ok {
		t.Error("expected 'path' in properties")
	}
}

func TestBridgeToolSchemaNil(t *testing.T) {
	bt := &BridgeTool{
		info: mcp.ToolInfo{
			Name:        "no-schema",
			InputSchema: nil,
		},
	}
	schema := bt.Schema()
	if schema["type"] != "object" {
		t.Errorf("expected fallback schema type 'object', got %v", schema["type"])
	}
}

func TestBridgeToolExecute(t *testing.T) {
	orch := &mockOrchestrator{
		tools: []mcp.ToolInfo{
			{Name: "echo", QualifiedName: "echo", Server: "test"},
		},
	}
	bt := &BridgeTool{
		info: mcp.ToolInfo{
			Name:   "echo",
			Server: "test",
		},
		orch: orch,
	}

	result, err := bt.Execute(context.Background(), map[string]any{"msg": "hello"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Error("expected no error")
	}
	if result.Content != "result from echo" {
		t.Errorf("unexpected content: %q", result.Content)
	}
}

func TestBridgeToolAnnotations(t *testing.T) {
	tests := []struct {
		name         string
		annotations  *mcp.ToolAnnotations
		wantReadOnly bool
		wantDestruct bool
		wantConfirm  bool
	}{
		{
			name:         "read-only tool",
			annotations:  &mcp.ToolAnnotations{ReadOnlyHint: true},
			wantReadOnly: true,
			wantDestruct: false,
			wantConfirm:  false,
		},
		{
			name:         "destructive tool",
			annotations:  &mcp.ToolAnnotations{DestructiveHint: true},
			wantReadOnly: false,
			wantDestruct: true,
			wantConfirm:  true,
		},
		{
			name:         "no annotations",
			annotations:  nil,
			wantReadOnly: false,
			wantDestruct: false,
			wantConfirm:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bt := &BridgeTool{
				info: mcp.ToolInfo{
					Name:        "test",
					Annotations: tt.annotations,
				},
			}
			if bt.IsReadOnly(nil) != tt.wantReadOnly {
				t.Errorf("IsReadOnly: got %v, want %v", bt.IsReadOnly(nil), tt.wantReadOnly)
			}
			if bt.IsDestructive(nil) != tt.wantDestruct {
				t.Errorf("IsDestructive: got %v, want %v", bt.IsDestructive(nil), tt.wantDestruct)
			}
			if bt.RequiresConfirmation("") != tt.wantConfirm {
				t.Errorf("RequiresConfirmation: got %v, want %v", bt.RequiresConfirmation(""), tt.wantConfirm)
			}
		})
	}
}

func TestSync(t *testing.T) {
	orch := &mockOrchestrator{
		tools: []mcp.ToolInfo{
			{Name: "tool1", QualifiedName: "srv/tool1", Server: "srv"},
			{Name: "tool2", QualifiedName: "srv/tool2", Server: "srv"},
		},
	}

	bridge := New(orch)
	reg := tools.NewRegistry()
	bridge.Sync(reg)

	// Should have registered 2 tools
	allTools := reg.All()
	if len(allTools) != 2 {
		t.Errorf("expected 2 tools, got %d", len(allTools))
	}

	// Check tool names
	for _, tool := range allTools {
		if tool.Name() != "mcp__srv/tool1" && tool.Name() != "mcp__srv/tool2" {
			t.Errorf("unexpected tool name: %q", tool.Name())
		}
	}
}

func TestSyncEmpty(t *testing.T) {
	orch := &mockOrchestrator{tools: nil}
	bridge := New(orch)
	reg := tools.NewRegistry()
	bridge.Sync(reg)

	allTools := reg.All()
	if len(allTools) != 0 {
		t.Errorf("expected 0 tools, got %d", len(allTools))
	}
}
