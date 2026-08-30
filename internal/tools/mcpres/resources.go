package mcpres

import (
	"context"
	"fmt"
	"strings"

	"github.com/iSundram/Automergent/internal/mcp"
	"github.com/iSundram/Automergent/internal/tools"
)

// MCP resource tools expose the connected MCP servers' resources to the
// model: list what exists, read one by URI. The orchestrator may be nil or
// empty (no servers configured) — the tools then report that plainly.

// Resources provides the orchestrator surface the tools need. It matches
// *mcp.Orchestrator, kept as an interface for testability.
type Resources interface {
	ListResources() []mcp.ResourceInfo
	ReadResource(ctx context.Context, uri string) ([]mcp.ContentBlock, error)
}

// ListMCPResourcesTool lists resources across connected MCP servers.
type ListMCPResourcesTool struct {
	res Resources
}

// ReadMCPResourceTool reads one resource by URI.
type ReadMCPResourceTool struct {
	res Resources
}

// NewListTool creates the resource list tool.
func NewListTool(res Resources) *ListMCPResourcesTool { return &ListMCPResourcesTool{res: res} }

// NewReadTool creates the resource read tool.
func NewReadTool(res Resources) *ReadMCPResourceTool { return &ReadMCPResourceTool{res: res} }

func (t *ListMCPResourcesTool) Name() string { return "list_mcp_resources" }
func (t *ListMCPResourcesTool) Description() string {
	return `List resources exposed by the connected MCP servers.
- Resources are addressable data (files, schemas, records) servers publish.
- Read one with read_mcp_resource and its URI.`
}
func (t *ListMCPResourcesTool) RequiresConfirmation(mode string) bool { return false }
func (t *ListMCPResourcesTool) IsConcurrencySafe(args map[string]any) bool {
	return true
}
func (t *ListMCPResourcesTool) IsReadOnly(args map[string]any) bool    { return true }
func (t *ListMCPResourcesTool) IsDestructive(args map[string]any) bool { return false }

func (t *ListMCPResourcesTool) Meta() *tools.ToolMeta {
	return &tools.ToolMeta{
		Category:   "general",
		Usage:      "Server names pair with /mcp status; URIs are stable across calls.",
		WhenToUse:  "A task needs data an MCP server publishes as a resource rather than a tool.",
		WhenNotTo:  "Prefer the server's tools for actions; resources are read-only context.",
	}
}

func (t *ListMCPResourcesTool) Schema() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}
}

func (t *ListMCPResourcesTool) Execute(_ context.Context, _ map[string]any) (tools.Result, error) {
	if t.res == nil {
		return tools.Result{Content: "no MCP servers configured"}, nil
	}
	resources := t.res.ListResources()
	if len(resources) == 0 {
		return tools.Result{Content: "no resources exposed by the connected MCP servers"}, nil
	}
	var sb strings.Builder
	for _, r := range resources {
		sb.WriteString(fmt.Sprintf("%s\t%s\t%s\n", r.URI, r.Server, oneLine(r.Description)))
	}
	return tools.Result{
		Content:  fmt.Sprintf("uri\tserver\tdescription\n%s", sb.String()),
		Summary:  fmt.Sprintf("listed %d resources", len(resources)),
		Metadata: map[string]any{"count": len(resources)},
	}, nil
}

func (t *ReadMCPResourceTool) Name() string { return "read_mcp_resource" }
func (t *ReadMCPResourceTool) Description() string {
	return `Read one MCP resource by URI (from list_mcp_resources).
- Returns the resource's text content.`
}
func (t *ReadMCPResourceTool) RequiresConfirmation(mode string) bool { return false }
func (t *ReadMCPResourceTool) IsConcurrencySafe(args map[string]any) bool {
	return true
}
func (t *ReadMCPResourceTool) IsReadOnly(args map[string]any) bool    { return true }
func (t *ReadMCPResourceTool) IsDestructive(args map[string]any) bool { return false }

func (t *ReadMCPResourceTool) Meta() *tools.ToolMeta {
	return &tools.ToolMeta{
		Category:   "general",
		Usage:      "Resources can be large; prefer narrow URIs when the server offers them.",
		WhenToUse:  "You need the contents of a specific resource URI.",
		WhenNotTo:  "Do not call with guessed URIs — list first.",
	}
}

func (t *ReadMCPResourceTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"uri": map[string]any{
				"type":        "string",
				"description": "Resource URI from list_mcp_resources.",
			},
		},
		"required": []string{"uri"},
	}
}

func (t *ReadMCPResourceTool) Execute(ctx context.Context, args map[string]any) (tools.Result, error) {
	uri, ok := tools.StringArg(args, "uri")
	if !ok || uri == "" {
		return tools.Result{IsError: true, Content: "uri is required"}, nil
	}
	if t.res == nil {
		return tools.Result{IsError: true, Content: "no MCP servers configured"}, nil
	}
	blocks, err := t.res.ReadResource(ctx, uri)
	if err != nil {
		return tools.Result{IsError: true, Content: fmt.Sprintf("read resource: %v", err)}, nil
	}
	var parts []string
	for _, b := range blocks {
		switch b.Type {
		case "text":
			parts = append(parts, b.Text)
		case "resource":
			parts = append(parts, fmt.Sprintf("[resource %s (%s)]", b.URI, b.MimeType))
		case "image":
			parts = append(parts, fmt.Sprintf("[image %s, %d bytes]", b.MimeType, len(b.Data)))
		}
	}
	if len(parts) == 0 {
		return tools.Result{Content: "resource returned no content"}, nil
	}
	return tools.Result{
		Content: strings.Join(parts, "\n\n"),
		Summary: fmt.Sprintf("read resource %s", uri),
	}, nil
}

func oneLine(s string) string {
	s = strings.TrimSpace(s)
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		s = s[:idx]
	}
	return s
}
