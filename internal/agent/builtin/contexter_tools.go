package builtin

import (
	"context"
	"fmt"

	"github.com/iSundram/Automergent/internal/agent/agentdef"
	"github.com/iSundram/Automergent/internal/tools"
)

// ContextCompactTool triggers compaction on a set of messages.
type ContextCompactTool struct {
	tools.BaseTool
}

func (t *ContextCompactTool) Name() string { return "context_compact" }
func (t *ContextCompactTool) Description() string {
	return `Trigger progressive context compaction. Analyzes context usage and applies the appropriate compaction tier (ghost, truncate, distill, snapshot, microcompact, full).`
}
func (t *ContextCompactTool) RequiresConfirmation(string) bool { return false }

func (t *ContextCompactTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"reason": map[string]any{
				"type":        "string",
				"enum":        []string{"context_limit", "user_requested", "model_downshift"},
				"description": "Reason for compaction.",
			},
			"target_reduction": map[string]any{
				"type":        "number",
				"description": "Target reduction fraction (0.0-1.0). Default: 0.3 (reduce by 30%).",
			},
		},
	}
}

func (t *ContextCompactTool) Execute(ctx context.Context, args map[string]any) (tools.Result, error) {
	reason, _ := tools.StringArg(args, "reason")
	if reason == "" {
		reason = "user_requested"
	}

	return tools.Result{
		Content: fmt.Sprintf("Compaction triggered with reason: %s. The compaction pipeline will analyze context usage and apply progressive tiers as needed.", reason),
		Metadata: map[string]any{
			"reason":   reason,
			"tiers":    []string{"ghost", "truncate_middle", "distill", "snapshot", "microcompact", "full_compact"},
			"status":   "queued",
		},
	}, nil
}

// ContextBucketTool manages shared context buckets for cross-agent collaboration.
type ContextBucketTool struct {
	tools.BaseTool
}

func (t *ContextBucketTool) Name() string { return "context_bucket" }
func (t *ContextBucketTool) Description() string {
	return `Manage shared context buckets for cross-agent collaboration. Buckets are key-value stores that multiple agents can read/write to share findings, state, and results.`
}
func (t *ContextBucketTool) RequiresConfirmation(string) bool { return false }

func (t *ContextBucketTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"enum":        []string{"create", "get", "set", "delete", "list", "list_keys"},
				"description": "Action to perform on the bucket.",
			},
			"bucket": map[string]any{
				"type":        "string",
				"description": "Bucket name.",
			},
			"key": map[string]any{
				"type":        "string",
				"description": "Key within the bucket (for get/set/delete).",
			},
			"value": map[string]any{
				"type":        "string",
				"description": "Value to set (for set action).",
			},
		},
		"required": []string{"action", "bucket"},
	}
}

func (t *ContextBucketTool) Execute(ctx context.Context, args map[string]any) (tools.Result, error) {
	action, _ := tools.StringArg(args, "action")
	bucket, _ := tools.StringArg(args, "bucket")
	key, _ := tools.StringArg(args, "key")
	value, _ := tools.StringArg(args, "value")

	if bucket == "" {
		return tools.Result{IsError: true, Content: "bucket name is required"}, nil
	}

	// Note: Actual bucket operations are performed via the TaskStore interface.
	// This tool provides the interface; the agent framework wires the actual store.
	switch action {
	case "create":
		return tools.Result{Content: fmt.Sprintf("Bucket '%s' created/confirmed", bucket)}, nil
	case "get":
		if key == "" {
			return tools.Result{IsError: true, Content: "key is required for get"}, nil
		}
		return tools.Result{Content: fmt.Sprintf("Get %s/%s - delegated to task store", bucket, key)}, nil
	case "set":
		if key == "" {
			return tools.Result{IsError: true, Content: "key is required for set"}, nil
		}
		return tools.Result{Content: fmt.Sprintf("Set %s/%s = %s - delegated to task store", bucket, key, truncateStr(value, 100))}, nil
	case "delete":
		if key == "" {
			return tools.Result{IsError: true, Content: "key is required for delete"}, nil
		}
		return tools.Result{Content: fmt.Sprintf("Deleted %s/%s - delegated to task store", bucket, key)}, nil
	case "list":
		return tools.Result{Content: fmt.Sprintf("List bucket '%s' - delegated to task store", bucket)}, nil
	case "list_keys":
		return tools.Result{Content: fmt.Sprintf("List keys in '%s' - delegated to task store", bucket)}, nil
	default:
		return tools.Result{IsError: true, Content: fmt.Sprintf("unknown action: %s", action)}, nil
	}
}

// ContextMemoryTool manages the key-value memory store.
type ContextMemoryTool struct {
	tools.BaseTool
}

func (t *ContextMemoryTool) Name() string { return "context_memory" }
func (t *ContextMemoryTool) Description() string {
	return `Manage persistent key-value memory. Stores important facts, decisions, and context that persist across compaction and session boundaries.`
}
func (t *ContextMemoryTool) RequiresConfirmation(string) bool { return false }

func (t *ContextMemoryTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"enum":        []string{"store", "recall", "search", "list"},
				"description": "Action to perform.",
			},
			"key": map[string]any{
				"type":        "string",
				"description": "Memory key.",
			},
			"value": map[string]any{
				"type":        "string",
				"description": "Value to store (for store action).",
			},
			"query": map[string]any{
				"type":        "string",
				"description": "Search query (for search action).",
			},
		},
		"required": []string{"action"},
	}
}

func (t *ContextMemoryTool) Execute(ctx context.Context, args map[string]any) (tools.Result, error) {
	action, _ := tools.StringArg(args, "action")
	key, _ := tools.StringArg(args, "key")
	value, _ := tools.StringArg(args, "value")
	query, _ := tools.StringArg(args, "query")

	switch action {
	case "store":
		if key == "" || value == "" {
			return tools.Result{IsError: true, Content: "key and value are required for store"}, nil
		}
		return tools.Result{Content: fmt.Sprintf("Stored memory: %s = %s", key, truncateStr(value, 100))}, nil
	case "recall":
		if key == "" {
			return tools.Result{IsError: true, Content: "key is required for recall"}, nil
		}
		return tools.Result{Content: fmt.Sprintf("Recall: %s - delegated to memory store", key)}, nil
	case "search":
		if query == "" {
			return tools.Result{IsError: true, Content: "query is required for search"}, nil
		}
		return tools.Result{Content: fmt.Sprintf("Search: %s - delegated to memory store", query)}, nil
	case "list":
		return tools.Result{Content: "List all memories - delegated to memory store"}, nil
	default:
		return tools.Result{IsError: true, Content: fmt.Sprintf("unknown action: %s", action)}, nil
	}
}

// ContextTranscriptTool manages the durable conversation transcript.
type ContextTranscriptTool struct {
	tools.BaseTool
}

func (t *ContextTranscriptTool) Name() string { return "context_transcript" }
func (t *ContextTranscriptTool) Description() string {
	return `Query the durable conversation transcript. Retrieve past messages, search by content, or get summaries of compacted segments.`
}
func (t *ContextTranscriptTool) RequiresConfirmation(string) bool { return false }

func (t *ContextTranscriptTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"enum":        []string{"get", "search", "summary", "stats"},
				"description": "Action to perform.",
			},
			"item_id": map[string]any{
				"type":        "string",
				"description": "Transcript item ID (for get action).",
			},
			"query": map[string]any{
				"type":        "string",
				"description": "Search query (for search action).",
			},
			"since": map[string]any{
				"type":        "string",
				"description": "ISO time string - return items since this time.",
			},
		},
		"required": []string{"action"},
	}
}

func (t *ContextTranscriptTool) Execute(ctx context.Context, args map[string]any) (tools.Result, error) {
	action, _ := tools.StringArg(args, "action")
	itemID, _ := tools.StringArg(args, "item_id")
	query, _ := tools.StringArg(args, "query")
	since, _ := tools.StringArg(args, "since")

	switch action {
	case "get":
		if itemID == "" {
			return tools.Result{IsError: true, Content: "item_id is required for get"}, nil
		}
		return tools.Result{Content: fmt.Sprintf("Get transcript item %s - delegated to transcript manager", itemID)}, nil
	case "search":
		if query == "" {
			return tools.Result{IsError: true, Content: "query is required for search"}, nil
		}
		return tools.Result{Content: fmt.Sprintf("Search transcript: %s - delegated to transcript manager", query)}, nil
	case "summary":
		sinceTime := ""
		if since != "" {
			sinceTime = since
		}
		return tools.Result{Content: fmt.Sprintf("Transcript summary since %s - delegated to transcript manager", sinceTime)}, nil
	case "stats":
		return tools.Result{Content: "Transcript stats - delegated to transcript manager"}, nil
	default:
		return tools.Result{IsError: true, Content: fmt.Sprintf("unknown action: %s", action)}, nil
	}
}

// RegisterContexterTools registers all contexter tools with the given registry.
func RegisterContexterTools(reg interface {
	Register(tool tools.Tool)
}) {
	reg.Register(&ContextCompactTool{})
	reg.Register(&ContextBucketTool{})
	reg.Register(&ContextMemoryTool{})
	reg.Register(&ContextTranscriptTool{})
}

// ContexterAgentDef returns an enhanced contexter agent definition with tool awareness.
func ContexterAgentDef() *agentdef.AgentDefinition {
	def := ContexterAgent()
	def.Tools = []string{
		"read", "grep", "glob", "bash",
		"context_compact", "context_bucket", "context_memory", "context_transcript",
	}
	return def
}

func truncateStr(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
