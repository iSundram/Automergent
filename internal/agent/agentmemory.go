package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/iSundram/Automergent/internal/agent/agentdef"
	"github.com/iSundram/Automergent/internal/tools"
)

// Persistent agent memory — the reference agent's per-agent memory scopes.
// A definition with MemoryScope global or project gets a memory file that
// survives across spawns and sessions, loaded into the agent's context, and
// writable through the agent_memory tool. Read-only agents may carry memory
// too (they can read it, and write findings for their next spawn).

const agentMemoryMaxEntry = 2000

// AgentMemory is a persistent per-agent-type memory file.
type AgentMemory struct {
	mu      sync.Mutex
	path    string
	entries []string
}

// memoryPath resolves the file for an agent type and scope.
func memoryPath(workDir string, def *agentdef.AgentDefinition) string {
	name := sanitizeFileComponent(def.Name)
	switch def.MemoryScope {
	case agentdef.MemoryScopeGlobal:
		home, err := os.UserHomeDir()
		if err != nil || home == "" {
			home = "."
		}
		return filepath.Join(home, ".automergent", "agent-memory", name+".md")
	default: // project
		return filepath.Join(workDir, ".automergent", "agent-memory", name+".md")
	}
}

// LoadAgentMemory loads (or initializes) the memory file for a definition.
func LoadAgentMemory(workDir string, def *agentdef.AgentDefinition) *AgentMemory {
	path := memoryPath(workDir, def)
	m := &AgentMemory{path: path}
	data, err := os.ReadFile(path)
	if err != nil {
		return m
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "- ") {
			m.entries = append(m.entries, strings.TrimPrefix(line, "- "))
		}
	}
	return m
}

// Entries returns a copy of the memory entries.
func (m *AgentMemory) Entries() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.entries...)
}

// Prompt renders the memory for context injection.
func (m *AgentMemory) Prompt() string {
	entries := m.Entries()
	if len(entries) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("## Agent Memory (persists across your spawns)\n")
	for _, e := range entries {
		sb.WriteString("- " + e + "\n")
	}
	return sb.String()
}

// Append records an entry and persists it.
func (m *AgentMemory) Append(entry string) error {
	entry = strings.TrimSpace(entry)
	if entry == "" {
		return fmt.Errorf("empty memory entry")
	}
	if len(entry) > agentMemoryMaxEntry {
		entry = entry[:agentMemoryMaxEntry]
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	// Idempotent on exact duplicates.
	for _, e := range m.entries {
		if strings.EqualFold(e, entry) {
			return nil
		}
	}
	m.entries = append(m.entries, entry)

	var sb strings.Builder
	for _, e := range m.entries {
		sb.WriteString("- " + e + "\n")
	}
	if err := os.MkdirAll(filepath.Dir(m.path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(m.path, []byte(sb.String()), 0o644)
}

// Clear wipes the memory file.
func (m *AgentMemory) Clear() error {
	m.mu.Lock()
	m.entries = nil
	m.mu.Unlock()
	return os.WriteFile(m.path, []byte(""), 0o644)
}

// installAgentMemory gives a child agent its persistent memory: loaded into
// its user context and a tool to write entries.
func (c *Agent) installAgentMemory(def *agentdef.AgentDefinition) {
	memory := LoadAgentMemory(c.workDir, def)
	c.agentMemory = memory
	if c.tools != nil {
		c.tools.Register(&agentMemoryTool{memory: memory})
	}
}

// agentMemoryTool exposes read/write access to the spawning agent's memory.
type agentMemoryTool struct {
	memory *AgentMemory
}

func (t *agentMemoryTool) Name() string        { return "agent_memory" }
func (t *agentMemoryTool) Description() string { return agentMemoryDescription }
func (t *agentMemoryTool) RequiresConfirmation(string) bool { return false }
func (t *agentMemoryTool) IsReadOnly(map[string]any) bool   { return true }
func (t *agentMemoryTool) IsDestructive(map[string]any) bool { return false }
func (t *agentMemoryTool) IsConcurrencySafe(map[string]any) bool { return true }
func (t *agentMemoryTool) EstimatedCost() tools.ToolCost {
	return tools.ToolCost{TokensApprox: 100, LatencyMs: 10, RiskLevel: "low"}
}

const agentMemoryDescription = `Read and write your persistent agent memory.

Your memory survives across your spawns and sessions. Record facts, findings, and
conventions you want your future spawns to know; read it to recall them.

Actions:
- read: return all memory entries (default)
- write: append an entry (duplicate-safe)
- clear: wipe all entries

Use write for durable knowledge about this codebase or workflow, not for
task-specific state — that belongs in your reply.`

func (t *agentMemoryTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"enum":        []string{"read", "write", "clear"},
				"description": "What to do with the memory.",
			},
			"entry": map[string]any{
				"type":        "string",
				"description": "Memory entry text (action=write).",
			},
		},
	}
}

func (t *agentMemoryTool) Execute(_ context.Context, args map[string]any) (tools.Result, error) {
	action, _ := tools.StringArg(args, "action")
	if action == "" {
		action = "read"
	}
	switch action {
	case "read":
		entries := t.memory.Entries()
		if len(entries) == 0 {
			return tools.Result{Content: "memory is empty"}, nil
		}
		return tools.Result{Content: strings.Join(entries, "\n")}, nil
	case "write":
		entry, _ := tools.StringArg(args, "entry")
		if err := t.memory.Append(entry); err != nil {
			return tools.Result{IsError: true, Content: err.Error()}, nil
		}
		return tools.Result{Content: "recorded"}, nil
	case "clear":
		if err := t.memory.Clear(); err != nil {
			return tools.Result{IsError: true, Content: err.Error()}, nil
		}
		return tools.Result{Content: "memory cleared"}, nil
	default:
		return tools.Result{IsError: true, Content: "unknown action: " + action}, nil
	}
}
