package ctxinfo

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/iSundram/Automergent/internal/tools"
)

// CtxInspectTool reports the state of the model's own context window:
// budget consumption, the live token estimate, and compaction posture. The
// summary itself is produced by the host (agent + context manager) through
// SetInspector so this package stays free of agent internals.

var (
	mu        sync.RWMutex
	inspector func() string
)

// SetInspector installs the summary provider. Pass nil to disable.
func SetInspector(fn func() string) {
	mu.Lock()
	defer mu.Unlock()
	inspector = fn
}

// CtxInspectTool is the read-only context introspection tool.
type CtxInspectTool struct {
	tools.BaseTool
}

func (t *CtxInspectTool) Name() string { return "ctx_inspect" }
func (t *CtxInspectTool) Description() string {
	return `Inspect your own context window state: token budget usage, the live conversation estimate, and what is consuming the window.
- Use before starting large reads or long tool chains to judge how much room is left.
- Read-only; costs almost nothing.`
}
func (t *CtxInspectTool) RequiresConfirmation(mode string) bool { return false }
func (t *CtxInspectTool) IsConcurrencySafe(args map[string]any) bool {
	return true
}
func (t *CtxInspectTool) IsReadOnly(args map[string]any) bool    { return true }
func (t *CtxInspectTool) IsDestructive(args map[string]any) bool { return false }

func (t *CtxInspectTool) Meta() *tools.ToolMeta {
	return &tools.ToolMeta{
		Category:   "verify",
		Usage:      "When usage is high, prefer narrow reads (line ranges) and let the context engine compact instead of re-reading everything.",
		WhenToUse:  "Before work that could overflow the window, or when compaction seems imminent.",
		WhenNotTo:  "Do not poll it every turn — check at phase boundaries.",
	}
}

func (t *CtxInspectTool) Schema() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}
}

func (t *CtxInspectTool) Execute(_ context.Context, _ map[string]any) (tools.Result, error) {
	mu.RLock()
	fn := inspector
	mu.RUnlock()
	if fn == nil {
		return tools.Result{IsError: true, Content: "context inspection is not available in this runtime"}, nil
	}
	summary := fn()
	if summary == "" {
		return tools.Result{IsError: true, Content: "no context information available"}, nil
	}
	return tools.Result{
		Content: summary,
		Summary: "context window state",
	}, nil
}

// FormatSummary renders the standard summary block from raw numbers; shared
// by the host so the tool's output stays consistent.
func FormatSummary(limit, used, conversation, systemPrompt, toolDefs, contextFiles int, usagePct float64) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "context limit: %s\n", humanTokens(limit))
	fmt.Fprintf(&sb, "estimated in use: %s (%.1f%%)\n", humanTokens(used), usagePct)
	fmt.Fprintf(&sb, "conversation: %s\n", humanTokens(conversation))
	fmt.Fprintf(&sb, "system prompt: %s\n", humanTokens(systemPrompt))
	fmt.Fprintf(&sb, "tool definitions: %s\n", humanTokens(toolDefs))
	fmt.Fprintf(&sb, "context files: %s\n", humanTokens(contextFiles))
	remaining := limit - used
	if remaining < 0 {
		remaining = 0
	}
	fmt.Fprintf(&sb, "remaining: %s", humanTokens(remaining))
	return sb.String()
}

func humanTokens(n int) string {
	switch {
	case n >= 1000000:
		return fmt.Sprintf("%.1fM", float64(n)/1000000)
	case n >= 1000:
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	default:
		return fmt.Sprintf("%d", n)
	}
}
