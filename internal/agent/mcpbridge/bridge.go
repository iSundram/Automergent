package mcpbridge

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/iSundram/Automergent/internal/agent"
	"github.com/iSundram/Automergent/internal/mcp"
	"github.com/iSundram/Automergent/internal/tools"
)

// OrchestratorAPI is the subset of mcp.Orchestrator that the bridge needs.
type OrchestratorAPI interface {
	CallTool(ctx context.Context, call mcp.ToolCall) (*mcp.ToolResult, error)
	ListTools() []mcp.ToolInfo
	GetTool(name string) (mcp.ToolInfo, bool)
	ListServers() []mcp.ServerInfo
}

// Bridge adapts MCP tools into the tools.Tool interface.
type Bridge struct {
	orch    OrchestratorAPI
	mu      sync.RWMutex
	tools   map[string]*BridgeTool
	agent   *agent.Agent
}

// New creates a new MCP bridge.
func New(orch OrchestratorAPI) *Bridge {
	return &Bridge{
		orch:  orch,
		tools: make(map[string]*BridgeTool),
	}
}

// SetAgent sets the agent reference for event emission.
func (b *Bridge) SetAgent(ag *agent.Agent) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.agent = ag
}

// Sync registers all MCP tools from the orchestrator into a tool registry.
// It should be called at startup and whenever tools change.
func (b *Bridge) Sync(reg *tools.Registry) {
	if b.orch == nil {
		return
	}

	mcpTools := b.orch.ListTools()

	b.mu.Lock()
	// Build new tool map
	newTools := make(map[string]*BridgeTool, len(mcpTools))
	for _, t := range mcpTools {
		if existing, ok := b.tools[t.QualifiedName]; ok {
			existing.info = t
			newTools[t.QualifiedName] = existing
		} else {
			bt := &BridgeTool{
				info: t,
				orch: b.orch,
			}
			newTools[t.QualifiedName] = bt
		}
	}
	b.tools = newTools
	b.mu.Unlock()

	// Register all tools
	b.mu.RLock()
	for _, bt := range b.tools {
		reg.Register(bt)
	}
	b.mu.RUnlock()
}

// SyncWithEvents registers tools and emits events for changes.
func (b *Bridge) SyncWithEvents(reg *tools.Registry) {
	b.mu.RLock()
	oldCount := len(b.tools)
	b.mu.RUnlock()

	b.Sync(reg)

	b.mu.RLock()
	newCount := len(b.tools)
	ag := b.agent
	b.mu.RUnlock()

	if ag != nil && newCount != oldCount {
		ag.Emit(agent.EventNotify, map[string]any{
			"level":   "info",
			"title":   "MCP tools updated",
			"message": fmt.Sprintf("MCP tools: %d → %d", oldCount, newCount),
		})
	}
}

// BridgeTool adapts a single MCP tool to the tools.Tool interface.
type BridgeTool struct {
	info mcp.ToolInfo
	orch OrchestratorAPI
}

func (t *BridgeTool) Name() string {
	return "mcp__" + t.info.QualifiedName
}

func (t *BridgeTool) Description() string {
	desc := t.info.Description
	if t.info.Server != "" {
		desc = fmt.Sprintf("[MCP:%s] %s", t.info.Server, desc)
	}
	return desc
}

func (t *BridgeTool) Schema() map[string]any {
	if t.info.InputSchema == nil {
		return map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		}
	}
	var schema map[string]any
	if err := json.Unmarshal(t.info.InputSchema, &schema); err != nil {
		return map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		}
	}
	return schema
}

func (t *BridgeTool) Execute(ctx context.Context, args map[string]any) (tools.Result, error) {
	result, err := t.orch.CallTool(ctx, mcp.ToolCall{
		Name:   t.info.Name,
		Args:   args,
		Server: t.info.Server,
	})
	if err != nil {
		return tools.Result{
			IsError: true,
			Content: fmt.Sprintf("MCP tool %s failed: %v", t.info.QualifiedName, err),
		}, nil // Return as tool error, not Go error
	}

	// Convert ContentBlocks to a single string
	var sb strings.Builder
	for _, block := range result.Content {
		if block.Type == "text" && block.Text != "" {
			if sb.Len() > 0 {
				sb.WriteString("\n")
			}
			sb.WriteString(block.Text)
		} else if block.Type == "image" {
			if sb.Len() > 0 {
				sb.WriteString("\n")
			}
			sb.WriteString(fmt.Sprintf("[image: %s]", block.MimeType))
		} else if block.Type == "resource" {
			if sb.Len() > 0 {
				sb.WriteString("\n")
			}
			sb.WriteString(fmt.Sprintf("[resource: %s]", block.URI))
		}
	}

	content := sb.String()
	if content == "" && result.IsError {
		content = "MCP tool returned empty error"
	}

	return tools.Result{
		Content: content,
		IsError: result.IsError,
	}, nil
}

func (t *BridgeTool) RequiresConfirmation(_ string) bool {
	// MCP mutation tools require confirmation unless explicitly read-only
	if t.info.Annotations != nil && t.info.Annotations.ReadOnlyHint {
		return false
	}
	return true
}

func (t *BridgeTool) IsConcurrencySafe(_ map[string]any) bool {
	// MCP tools are generally safe to run in parallel
	return true
}

func (t *BridgeTool) IsReadOnly(_ map[string]any) bool {
	if t.info.Annotations != nil {
		return t.info.Annotations.ReadOnlyHint
	}
	return false
}

func (t *BridgeTool) IsDestructive(_ map[string]any) bool {
	if t.info.Annotations != nil {
		return t.info.Annotations.DestructiveHint
	}
	return false
}

func (t *BridgeTool) EstimatedCost() tools.ToolCost {
	return tools.ToolCost{
		TokensApprox: 200,
		LatencyMs:    200,
		RiskLevel:    "medium",
	}
}
