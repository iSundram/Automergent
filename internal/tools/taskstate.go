package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/iSundram/Automergent/internal/shared"
	"github.com/iSundram/Automergent/internal/taskstate"
)

// RegisterTaskStateTools registers tools that operate on the prompt's task state.
//
// The surface is deliberately small: todo_write/todo_list maintain the
// user-visible plan, context_bucket_get/set/delete share data between tasks,
// and context_get reads the phase inputs (intent/init). Earlier per-action
// tools (task_list/get/update, context_bucket_create/list, context_list_
// buckets, context_get_intent/init, todo_next) were folded into these six.
func RegisterTaskStateTools(reg *Registry, store taskstate.TaskStore) {
	if reg == nil || store == nil {
		return
	}
	reg.Register(&todoWriteTool{store: store})
	reg.Register(&todoListTool{store: store})
	reg.Register(&contextBucketGetTool{store: store})
	reg.Register(&contextBucketSetTool{store: store})
	reg.Register(&contextBucketDeleteTool{store: store})
	reg.Register(&contextGetTool{store: store})
}

// --- Todo tools ---

// todoWriteTool lets the model maintain its own plan in-log (Cursor-style
// TODO_WRITE): replace the whole list or update a single status.
type todoWriteTool struct{ store taskstate.TaskStore }

func (*todoWriteTool) Name() string { return "todo_write" }
func (*todoWriteTool) Description() string {
	return "Write or update your todo list. Call with action=replace to install the full list, or action=status to move one item."
}
func (*todoWriteTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{"type": "string", "enum": []string{"replace", "status"}, "description": "replace = install full todo list; status = update one item"},
			"todos": map[string]any{
				"type":        "array",
				"description": "For action=replace. Items: {description, priority?, dependencies?, id?}",
				"items": map[string]any{
					"type":       "object",
					"properties": map[string]any{"id": map[string]any{"type": "string"}, "description": map[string]any{"type": "string"}, "priority": map[string]any{"type": "integer"}, "dependencies": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}},
					"required":   []string{"description"},
				},
			},
			"id":     map[string]any{"type": "string", "description": "For action=status: todo ID"},
			"status": map[string]any{"type": "string", "enum": []string{"pending", "in_progress", "completed", "blocked"}},
		},
		"required": []string{"action"},
	}
}
func (*todoWriteTool) RequiresConfirmation(string) bool      { return false }
func (*todoWriteTool) IsConcurrencySafe(map[string]any) bool { return false }
func (*todoWriteTool) EstimatedCost() ToolCost {
	return ToolCost{TokensApprox: 50, LatencyMs: 20, RiskLevel: "low"}
}
func (*todoWriteTool) IsDestructive(map[string]any) bool { return false }
func (*todoWriteTool) IsReadOnly(map[string]any) bool    { return false }

// normalizeTodoStatus maps the natural-language status words models reach
// for ("done", "complete", "finished") onto the canonical enum. Models
// occasionally send them despite the schema enum; rejecting the call made
// the todo board wedge (observed in session 89869a5e: two consecutive
// `invalid status` errors, todo never marked completed).
func normalizeTodoStatus(s string) shared.TodoStatus {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "done", "complete", "completed", "finished":
		return shared.TodoStatusCompleted
	case "in_progress", "inprogress", "started", "active", "doing":
		return shared.TodoStatusInProgress
	case "pending", "todo", "not_started":
		return shared.TodoStatusPending
	case "blocked", "blocked_on", "waiting":
		return shared.TodoStatusBlocked
	default:
		return shared.TodoStatus(s)
	}
}
func (*todoWriteTool) Meta() *ToolMeta {
	return &ToolMeta{
		Category:    "memory",
		DisplayName: "Update todos",
		InjectOrder: 10,
		WhenToUse:   "Right after planning (install the full list), and every time you start/finish an item — keep statuses truthful; the UI board renders this live.",
		UsageByFamily: map[string]string{
			"gemini3": "Gemini 3: send the whole todos array in one call when planning; avoid one-call-per-item writes.",
		},
	}
}

func (t *todoWriteTool) Execute(ctx context.Context, args map[string]any) (Result, error) {
	action, _ := args["action"].(string)
	switch action {
	case "replace":
		raw, ok := args["todos"].([]any)
		if !ok {
			return Result{IsError: true, Content: "todo_write replace requires `todos` array"}, nil
		}
		var items []shared.TodoItem
		for _, r := range raw {
			m, ok := r.(map[string]any)
			if !ok {
				continue
			}
			id, _ := StringArg(m, "id")
			desc, _ := StringArg(m, "description")
			item := shared.TodoItem{
				ID:          id,
				Description: desc,
				Status:      shared.TodoStatusPending,
				ContextKeys: []string{},
			}
			if item.Description == "" {
				continue
			}
			if p, ok := m["priority"].(float64); ok {
				item.Priority = int(p)
			}
			if deps, ok := m["dependencies"].([]any); ok {
				for _, d := range deps {
					if ds, ok := d.(string); ok {
						item.Dependencies = append(item.Dependencies, ds)
					}
				}
			}
			items = append(items, item)
		}
		if len(items) == 0 {
			return Result{IsError: true, Content: "todo_write replace: no valid todos supplied"}, nil
		}
		t.store.ReplaceTodos(items)
		return Result{Content: fmt.Sprintf("Installed %d todos", len(items)), Summary: fmt.Sprintf("%d todos", len(items))}, nil

	case "status":
		id, _ := args["id"].(string)
		statusStr, _ := args["status"].(string)
		status := normalizeTodoStatus(statusStr)
		switch status {
		case shared.TodoStatusPending, shared.TodoStatusInProgress,
			shared.TodoStatusCompleted, shared.TodoStatusBlocked:
		default:
			return Result{IsError: true, Content: "todo_write status: invalid `status` (use pending, in_progress, completed, or blocked)"}, nil
		}
		if !t.store.SetTodoStatus(id, status) {
			return Result{IsError: true, Content: fmt.Sprintf("todo %q not found", id)}, nil
		}
		return Result{Content: fmt.Sprintf("Todo %s -> %s", id, status), Summary: fmt.Sprintf("%s → %s", id, status)}, nil

	default:
		return Result{IsError: true, Content: "todo_write: action must be `replace` or `status`"}, nil
	}
}

type todoListTool struct{ store taskstate.TaskStore }

func (*todoListTool) Name() string { return "todo_list" }
func (*todoListTool) Description() string {
	return "List todo items from the current plan, optionally filtered by status. With next=true returns only the next pending item whose dependencies are met."
}
func (*todoListTool) Schema() map[string]any {
	return map[string]any{
		"type": "object", "properties": map[string]any{
			"status_filter": map[string]any{"type": "string", "enum": []string{"pending", "in_progress", "completed", "blocked"}},
			"next":          map[string]any{"type": "boolean", "description": "Return only the next pending todo whose dependencies are met"},
		},
	}
}
func (*todoListTool) RequiresConfirmation(string) bool      { return false }
func (*todoListTool) IsConcurrencySafe(map[string]any) bool { return true }
func (*todoListTool) EstimatedCost() ToolCost {
	return ToolCost{TokensApprox: 50, LatencyMs: 50, RiskLevel: "low"}
}
func (*todoListTool) IsDestructive(map[string]any) bool { return false }
func (*todoListTool) IsReadOnly(map[string]any) bool    { return true }

func (t *todoListTool) Execute(ctx context.Context, args map[string]any) (Result, error) {
	if next, _ := args["next"].(bool); next {
		n := t.store.GetNextPendingTask()
		if n == nil {
			return Result{Content: "No pending tasks ready to execute"}, nil
		}
		data, _ := json.MarshalIndent(n, "", "  ")
		return Result{Content: string(data)}, nil
	}

	filter, _ := args["status_filter"].(string)
	items := t.store.TodoItems()
	if len(items) == 0 {
		return Result{Content: "No todo items in current plan"}, nil
	}
	var sb strings.Builder
	for _, item := range items {
		if filter != "" && string(item.Status) != filter {
			continue
		}
		deps := ""
		if len(item.Dependencies) > 0 {
			deps = " (after: " + strings.Join(item.Dependencies, ", ") + ")"
		}
		sb.WriteString(fmt.Sprintf("- [%s] %s (pri=%d)%s\n", item.Status, item.Description, item.Priority, deps))
	}
	return Result{Content: sb.String()}, nil
}

// --- Context bucket tools ---

// contextBucketGetTool is the single read surface for buckets: with bucket+key
// it returns one value; with only a bucket it lists the bucket's keys; with
// nothing it lists every bucket with its key count.
type contextBucketGetTool struct{ store taskstate.TaskStore }

func (*contextBucketGetTool) Name() string { return "context_bucket_get" }
func (*contextBucketGetTool) Description() string {
	return "Read from context buckets (shared key-value data between tasks). bucket+key → one value; bucket only → the bucket's keys; no args → all buckets."
}
func (*contextBucketGetTool) Schema() map[string]any {
	return map[string]any{
		"type": "object", "properties": map[string]any{
			"bucket": map[string]any{"type": "string", "description": "Bucket name (omit to list all buckets)"},
			"key":    map[string]any{"type": "string", "description": "Key to read (omit to list the bucket's keys)"},
		},
	}
}
func (*contextBucketGetTool) RequiresConfirmation(string) bool      { return false }
func (*contextBucketGetTool) IsConcurrencySafe(map[string]any) bool { return true }
func (*contextBucketGetTool) EstimatedCost() ToolCost {
	return ToolCost{TokensApprox: 50, LatencyMs: 50, RiskLevel: "low"}
}
func (*contextBucketGetTool) IsDestructive(map[string]any) bool { return false }
func (*contextBucketGetTool) IsReadOnly(map[string]any) bool    { return true }

func (t *contextBucketGetTool) Execute(ctx context.Context, args map[string]any) (Result, error) {
	bucket, _ := args["bucket"].(string)
	key, _ := args["key"].(string)

	switch {
	case bucket != "" && key != "":
		val, ok := t.store.BucketGet(bucket, key)
		if !ok {
			return Result{Content: "Key not found: " + key + " in " + bucket}, nil
		}
		return Result{Content: val}, nil

	case bucket != "":
		keys := t.store.BucketList(bucket)
		if len(keys) == 0 {
			return Result{Content: "Bucket empty or not found: " + bucket}, nil
		}
		return Result{Content: strings.Join(keys, "\n")}, nil

	default:
		buckets := t.store.GetBuckets()
		if len(buckets) == 0 {
			return Result{Content: "No buckets created yet"}, nil
		}
		names := make([]string, 0, len(buckets))
		for name := range buckets {
			names = append(names, name)
		}
		sort.Strings(names)
		var sb strings.Builder
		for _, name := range names {
			sb.WriteString(fmt.Sprintf("Bucket: %s (%d keys)\n", name, len(buckets[name])))
			for _, k := range t.store.BucketList(name) {
				sb.WriteString(fmt.Sprintf("  %s\n", k))
			}
		}
		return Result{Content: sb.String()}, nil
	}
}

type contextBucketSetTool struct{ store taskstate.TaskStore }

func (*contextBucketSetTool) Name() string { return "context_bucket_set" }
func (*contextBucketSetTool) Description() string {
	return "Set a key-value pair in a context bucket. The bucket is created automatically if it does not exist."
}
func (*contextBucketSetTool) Schema() map[string]any {
	return map[string]any{
		"type": "object", "properties": map[string]any{
			"bucket": map[string]any{"type": "string"},
			"key":    map[string]any{"type": "string"},
			"value":  map[string]any{"type": "string"},
		}, "required": []string{"bucket", "key", "value"},
	}
}
func (*contextBucketSetTool) RequiresConfirmation(string) bool      { return false }
func (*contextBucketSetTool) IsConcurrencySafe(map[string]any) bool { return true }
func (*contextBucketSetTool) EstimatedCost() ToolCost {
	return ToolCost{TokensApprox: 50, LatencyMs: 50, RiskLevel: "low"}
}
func (*contextBucketSetTool) IsDestructive(map[string]any) bool { return false }
func (*contextBucketSetTool) IsReadOnly(map[string]any) bool     { return false }

func (t *contextBucketSetTool) Execute(ctx context.Context, args map[string]any) (Result, error) {
	bucket, _ := args["bucket"].(string)
	key, _ := args["key"].(string)
	value, _ := args["value"].(string)
	if bucket == "" || key == "" || value == "" {
		return Result{Content: "bucket, key, and value required"}, nil
	}
	t.store.BucketSet(bucket, key, value)
	return Result{Content: fmt.Sprintf("Set %s=%s in %s", key, value, bucket)}, nil
}

type contextBucketDeleteTool struct{ store taskstate.TaskStore }

func (*contextBucketDeleteTool) Name() string { return "context_bucket_delete" }
func (*contextBucketDeleteTool) Description() string {
	return "Delete a key from a context bucket (or the whole bucket when key is omitted)."
}
func (*contextBucketDeleteTool) Schema() map[string]any {
	return map[string]any{
		"type": "object", "properties": map[string]any{
			"bucket": map[string]any{"type": "string"},
			"key":    map[string]any{"type": "string", "description": "Key to delete (omit to delete the whole bucket)"},
		}, "required": []string{"bucket"},
	}
}
func (*contextBucketDeleteTool) RequiresConfirmation(string) bool      { return false }
func (*contextBucketDeleteTool) IsConcurrencySafe(map[string]any) bool { return true }
func (*contextBucketDeleteTool) EstimatedCost() ToolCost {
	return ToolCost{TokensApprox: 50, LatencyMs: 50, RiskLevel: "low"}
}
func (*contextBucketDeleteTool) IsDestructive(map[string]any) bool { return true }
func (*contextBucketDeleteTool) IsReadOnly(map[string]any) bool    { return false }

func (t *contextBucketDeleteTool) Execute(ctx context.Context, args map[string]any) (Result, error) {
	bucket, _ := args["bucket"].(string)
	key, _ := args["key"].(string)
	if bucket == "" {
		return Result{Content: "bucket required"}, nil
	}
	if key == "" {
		t.store.DeleteBucket(bucket)
		return Result{Content: fmt.Sprintf("Deleted bucket %s", bucket)}, nil
	}
	t.store.BucketDelete(bucket, key)
	return Result{Content: fmt.Sprintf("Deleted %s from %s", key, bucket)}, nil
}

// --- Context phase inputs ---

// contextGetTool reads the pre-execution phase inputs: the intent set
// identified from the user's message, or the init-phase exploration results.
type contextGetTool struct{ store taskstate.TaskStore }

func (*contextGetTool) Name() string { return "context_get" }
func (*contextGetTool) Description() string {
	return "Read pre-execution context: what=intent returns the intents identified from the user's original message; what=init returns the initialization-phase exploration findings."
}
func (*contextGetTool) Schema() map[string]any {
	return map[string]any{
		"type": "object", "properties": map[string]any{
			"what": map[string]any{"type": "string", "enum": []string{"intent", "init"}, "description": "Which phase input to read"},
		}, "required": []string{"what"},
	}
}
func (*contextGetTool) RequiresConfirmation(string) bool      { return false }
func (*contextGetTool) IsConcurrencySafe(map[string]any) bool { return true }
func (*contextGetTool) EstimatedCost() ToolCost {
	return ToolCost{TokensApprox: 50, LatencyMs: 50, RiskLevel: "low"}
}
func (*contextGetTool) IsDestructive(map[string]any) bool { return false }
func (*contextGetTool) IsReadOnly(map[string]any) bool    { return true }

func (t *contextGetTool) Execute(ctx context.Context, args map[string]any) (Result, error) {
	what, _ := args["what"].(string)
	switch what {
	case "intent":
		intent := t.store.GetIntentSet()
		if intent == nil {
			return Result{Content: "No intent set available"}, nil
		}
		data, _ := json.MarshalIndent(intent, "", "  ")
		return Result{Content: string(data)}, nil
	case "init":
		init := t.store.GetInitResults()
		if init == nil {
			return Result{Content: "No init results available"}, nil
		}
		data, _ := json.MarshalIndent(init, "", "  ")
		return Result{Content: string(data)}, nil
	default:
		return Result{IsError: true, Content: "what must be `intent` or `init`"}, nil
	}
}
