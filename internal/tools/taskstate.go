package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/iSundram/Automergent/internal/shared"
	"github.com/iSundram/Automergent/internal/taskstate"
)

// RegisterTaskStateTools registers tools that operate on the prompt's task state.
func RegisterTaskStateTools(reg *Registry, store taskstate.TaskStore) {
	if reg == nil || store == nil {
		return
	}
	reg.Register(&taskListTool{store: store})
	reg.Register(&taskGetTool{store: store})
	reg.Register(&taskUpdateTool{store: store})
	reg.Register(&contextBucketCreateTool{store: store})
	reg.Register(&contextBucketListTool{store: store})
	reg.Register(&contextBucketGetTool{store: store})
	reg.Register(&contextBucketSetTool{store: store})
	reg.Register(&contextBucketDeleteTool{store: store})
	reg.Register(&contextListBucketsTool{store: store})
	reg.Register(&contextGetIntentTool{store: store})
	reg.Register(&contextGetInitTool{store: store})
	reg.Register(&todoListTool{store: store})
	reg.Register(&todoNextTool{store: store})
}

// --- Task tools ---

type taskListTool struct{ store taskstate.TaskStore }

func (*taskListTool) Name() string { return "task_list" }
func (*taskListTool) Description() string {
	return "List all tasks in the current plan with their statuses and dependencies."
}
func (*taskListTool) Schema() map[string]any {
	return map[string]any{
		"type": "object", "properties": map[string]any{
			"include_completed": map[string]any{"type": "boolean"},
		},
	}
}
func (*taskListTool) RequiresConfirmation(string) bool { return false }
func (*taskListTool) IsConcurrencySafe(map[string]any) bool { return true }
func (*taskListTool) EstimatedCost() ToolCost { return ToolCost{TokensApprox: 50, LatencyMs: 50, RiskLevel: "low"} }
func (*taskListTool) IsDestructive(map[string]any) bool { return false }
func (*taskListTool) IsReadOnly(map[string]any) bool { return true }

func (t *taskListTool) Execute(ctx context.Context, args map[string]any) (Result, error) {
	includeCompleted, _ := args["include_completed"].(bool)
	records := t.store.GetAllRecords()
	if len(records) == 0 {
		return Result{Content: "No tasks in current plan"}, nil
	}
	var sb strings.Builder
	for _, r := range records {
		if !includeCompleted && r.Status == shared.TodoStatusCompleted {
			continue
		}
		deps := ""
		if len(r.TaskSpec.Dependencies) > 0 {
			deps = " (after: " + strings.Join(r.TaskSpec.Dependencies, ", ") + ")"
		}
		sb.WriteString(fmt.Sprintf("- [%s] %s (%s, pri=%d)%s\n",
			r.Status, r.TaskSpec.Description, r.TaskSpec.Type, r.TaskSpec.Priority, deps))
	}
	return Result{Content: sb.String()}, nil
}

type taskGetTool struct{ store taskstate.TaskStore }

func (*taskGetTool) Name() string { return "task_get" }
func (*taskGetTool) Description() string { return "Get detailed info about a specific task by ID." }
func (*taskGetTool) Schema() map[string]any {
	return map[string]any{
		"type": "object", "properties": map[string]any{
			"task_id": map[string]any{"type": "string", "description": "Task ID to retrieve"},
		}, "required": []string{"task_id"},
	}
}
func (*taskGetTool) RequiresConfirmation(string) bool { return false }
func (*taskGetTool) IsConcurrencySafe(map[string]any) bool { return true }
func (*taskGetTool) EstimatedCost() ToolCost { return ToolCost{TokensApprox: 50, LatencyMs: 50, RiskLevel: "low"} }
func (*taskGetTool) IsDestructive(map[string]any) bool { return false }
func (*taskGetTool) IsReadOnly(map[string]any) bool { return true }

func (t *taskGetTool) Execute(ctx context.Context, args map[string]any) (Result, error) {
	taskID, _ := args["task_id"].(string)
	if taskID == "" {
		return Result{Content: "task_id required"}, nil
	}
	spec, ok := t.store.GetTask(taskID)
	if !ok {
		return Result{Content: "Task not found: " + taskID}, nil
	}
	rec, _ := t.store.GetRecord(taskID)
	data, _ := json.MarshalIndent(map[string]any{
		"spec":   spec,
		"record": rec,
	}, "", "  ")
	return Result{Content: string(data)}, nil
}

type taskUpdateTool struct{ store taskstate.TaskStore }

func (*taskUpdateTool) Name() string { return "task_update" }
func (*taskUpdateTool) Description() string { return "Update a task's status, result, or error." }
func (*taskUpdateTool) Schema() map[string]any {
	return map[string]any{
		"type": "object", "properties": map[string]any{
			"task_id": map[string]any{"type": "string"},
			"status":  map[string]any{"type": "string", "enum": []string{"pending", "in_progress", "completed", "blocked"}},
			"result":  map[string]any{"type": "string"},
			"error":   map[string]any{"type": "string"},
		}, "required": []string{"task_id", "status"},
	}
}
func (*taskUpdateTool) RequiresConfirmation(string) bool { return false }
func (*taskUpdateTool) IsConcurrencySafe(map[string]any) bool { return true }
func (*taskUpdateTool) EstimatedCost() ToolCost { return ToolCost{TokensApprox: 50, LatencyMs: 50, RiskLevel: "low"} }
func (*taskUpdateTool) IsDestructive(map[string]any) bool { return false }
func (*taskUpdateTool) IsReadOnly(map[string]any) bool { return true }

func (t *taskUpdateTool) Execute(ctx context.Context, args map[string]any) (Result, error) {
	taskID, _ := args["task_id"].(string)
	statusStr, _ := args["status"].(string)
	result, _ := args["result"].(string)
	errStr, _ := args["error"].(string)

	var status shared.TodoStatus
	switch statusStr {
	case "pending":
		status = shared.TodoStatusPending
	case "in_progress":
		status = shared.TodoStatusInProgress
	case "completed":
		status = shared.TodoStatusCompleted
	case "blocked":
		status = shared.TodoStatusBlocked
	default:
		return Result{Content: "Invalid status: " + statusStr}, nil
	}
	t.store.SetTaskStatus(taskID, status, result, errStr)
	return Result{Content: fmt.Sprintf("Task %s updated to %s", taskID, status)}, nil
}

// --- Context bucket tools ---

type contextBucketCreateTool struct{ store taskstate.TaskStore }

func (*contextBucketCreateTool) Name() string { return "context_bucket_create" }
func (*contextBucketCreateTool) Description() string { return "Create a new context bucket for sharing data between tasks." }
func (*contextBucketCreateTool) Schema() map[string]any {
	return map[string]any{
		"type": "object", "properties": map[string]any{
			"name": map[string]any{"type": "string", "description": "Bucket name"},
		}, "required": []string{"name"},
	}
}
func (*contextBucketCreateTool) RequiresConfirmation(string) bool { return false }
func (*contextBucketCreateTool) IsConcurrencySafe(map[string]any) bool { return true }
func (*contextBucketCreateTool) EstimatedCost() ToolCost { return ToolCost{TokensApprox: 50, LatencyMs: 50, RiskLevel: "low"} }
func (*contextBucketCreateTool) IsDestructive(map[string]any) bool { return false }
func (*contextBucketCreateTool) IsReadOnly(map[string]any) bool { return true }

func (t *contextBucketCreateTool) Execute(ctx context.Context, args map[string]any) (Result, error) {
	name, _ := args["name"].(string)
	if name == "" {
		return Result{Content: "name required"}, nil
	}
	if err := t.store.CreateBucket(name); err != nil {
		return Result{Content: "Error: " + err.Error()}, nil
	}
	return Result{Content: "Bucket created: " + name}, nil
}

type contextBucketListTool struct{ store taskstate.TaskStore }

func (*contextBucketListTool) Name() string { return "context_bucket_list" }
func (*contextBucketListTool) Description() string { return "List all keys in a context bucket." }
func (*contextBucketListTool) Schema() map[string]any {
	return map[string]any{
		"type": "object", "properties": map[string]any{
			"bucket": map[string]any{"type": "string", "description": "Bucket name"},
		}, "required": []string{"bucket"},
	}
}
func (*contextBucketListTool) RequiresConfirmation(string) bool { return false }
func (*contextBucketListTool) IsConcurrencySafe(map[string]any) bool { return true }
func (*contextBucketListTool) EstimatedCost() ToolCost { return ToolCost{TokensApprox: 50, LatencyMs: 50, RiskLevel: "low"} }
func (*contextBucketListTool) IsDestructive(map[string]any) bool { return false }
func (*contextBucketListTool) IsReadOnly(map[string]any) bool { return true }

func (t *contextBucketListTool) Execute(ctx context.Context, args map[string]any) (Result, error) {
	bucket, _ := args["bucket"].(string)
	if bucket == "" {
		return Result{Content: "bucket required"}, nil
	}
	keys := t.store.BucketList(bucket)
	if len(keys) == 0 {
		return Result{Content: "Bucket empty or not found: " + bucket}, nil
	}
	return Result{Content: strings.Join(keys, "\n")}, nil
}

type contextBucketGetTool struct{ store taskstate.TaskStore }

func (*contextBucketGetTool) Name() string { return "context_bucket_get" }
func (*contextBucketGetTool) Description() string { return "Get a value from a context bucket." }
func (*contextBucketGetTool) Schema() map[string]any {
	return map[string]any{
		"type": "object", "properties": map[string]any{
			"bucket": map[string]any{"type": "string"},
			"key":    map[string]any{"type": "string"},
		}, "required": []string{"bucket", "key"},
	}
}
func (*contextBucketGetTool) RequiresConfirmation(string) bool { return false }
func (*contextBucketGetTool) IsConcurrencySafe(map[string]any) bool { return true }
func (*contextBucketGetTool) EstimatedCost() ToolCost { return ToolCost{TokensApprox: 50, LatencyMs: 50, RiskLevel: "low"} }
func (*contextBucketGetTool) IsDestructive(map[string]any) bool { return false }
func (*contextBucketGetTool) IsReadOnly(map[string]any) bool { return true }

func (t *contextBucketGetTool) Execute(ctx context.Context, args map[string]any) (Result, error) {
	bucket, _ := args["bucket"].(string)
	key, _ := args["key"].(string)
	if bucket == "" || key == "" {
		return Result{Content: "bucket and key required"}, nil
	}
	val, ok := t.store.BucketGet(bucket, key)
	if !ok {
		return Result{Content: "Key not found: " + key + " in " + bucket}, nil
	}
	return Result{Content: val}, nil
}

type contextBucketSetTool struct{ store taskstate.TaskStore }

func (*contextBucketSetTool) Name() string { return "context_bucket_set" }
func (*contextBucketSetTool) Description() string { return "Set a key-value pair in a context bucket." }
func (*contextBucketSetTool) Schema() map[string]any {
	return map[string]any{
		"type": "object", "properties": map[string]any{
			"bucket": map[string]any{"type": "string"},
			"key":    map[string]any{"type": "string"},
			"value":  map[string]any{"type": "string"},
		}, "required": []string{"bucket", "key", "value"},
	}
}
func (*contextBucketSetTool) RequiresConfirmation(string) bool { return false }
func (*contextBucketSetTool) IsConcurrencySafe(map[string]any) bool { return true }
func (*contextBucketSetTool) EstimatedCost() ToolCost { return ToolCost{TokensApprox: 50, LatencyMs: 50, RiskLevel: "low"} }
func (*contextBucketSetTool) IsDestructive(map[string]any) bool { return false }
func (*contextBucketSetTool) IsReadOnly(map[string]any) bool { return true }

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
func (*contextBucketDeleteTool) Description() string { return "Delete a key from a context bucket." }
func (*contextBucketDeleteTool) Schema() map[string]any {
	return map[string]any{
		"type": "object", "properties": map[string]any{
			"bucket": map[string]any{"type": "string"},
			"key":    map[string]any{"type": "string"},
		}, "required": []string{"bucket", "key"},
	}
}
func (*contextBucketDeleteTool) RequiresConfirmation(string) bool { return false }
func (*contextBucketDeleteTool) IsConcurrencySafe(map[string]any) bool { return true }
func (*contextBucketDeleteTool) EstimatedCost() ToolCost { return ToolCost{TokensApprox: 50, LatencyMs: 50, RiskLevel: "low"} }
func (*contextBucketDeleteTool) IsDestructive(map[string]any) bool { return false }
func (*contextBucketDeleteTool) IsReadOnly(map[string]any) bool { return true }

func (t *contextBucketDeleteTool) Execute(ctx context.Context, args map[string]any) (Result, error) {
	bucket, _ := args["bucket"].(string)
	key, _ := args["key"].(string)
	if bucket == "" || key == "" {
		return Result{Content: "bucket and key required"}, nil
	}
	t.store.BucketDelete(bucket, key)
	return Result{Content: fmt.Sprintf("Deleted %s from %s", key, bucket)}, nil
}

type contextListBucketsTool struct{ store taskstate.TaskStore }

func (*contextListBucketsTool) Name() string { return "context_list_buckets" }
func (*contextListBucketsTool) Description() string { return "List all context buckets and their keys." }
func (*contextListBucketsTool) Schema() map[string]any {
	return map[string]any{
		"type": "object", "properties": map[string]any{},
	}
}
func (*contextListBucketsTool) RequiresConfirmation(string) bool { return false }
func (*contextListBucketsTool) IsConcurrencySafe(map[string]any) bool { return true }
func (*contextListBucketsTool) EstimatedCost() ToolCost { return ToolCost{TokensApprox: 50, LatencyMs: 50, RiskLevel: "low"} }
func (*contextListBucketsTool) IsDestructive(map[string]any) bool { return false }
func (*contextListBucketsTool) IsReadOnly(map[string]any) bool { return true }

func (t *contextListBucketsTool) Execute(ctx context.Context, args map[string]any) (Result, error) {
	buckets := t.store.GetBuckets()
	if len(buckets) == 0 {
		return Result{Content: "No buckets created yet"}, nil
	}
	var sb strings.Builder
	for name, b := range buckets {
		sb.WriteString(fmt.Sprintf("Bucket: %s (%d keys)\n", name, len(b)))
		for k := range b {
			sb.WriteString(fmt.Sprintf("  %s\n", k))
		}
	}
	return Result{Content: sb.String()}, nil
}

type contextGetIntentTool struct{ store taskstate.TaskStore }

func (*contextGetIntentTool) Name() string { return "context_get_intent" }
func (*contextGetIntentTool) Description() string { return "Get the intent set identified from the user's original message." }
func (*contextGetIntentTool) Schema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{}}
}
func (*contextGetIntentTool) RequiresConfirmation(string) bool { return false }
func (*contextGetIntentTool) IsConcurrencySafe(map[string]any) bool { return true }
func (*contextGetIntentTool) EstimatedCost() ToolCost { return ToolCost{TokensApprox: 50, LatencyMs: 50, RiskLevel: "low"} }
func (*contextGetIntentTool) IsDestructive(map[string]any) bool { return false }
func (*contextGetIntentTool) IsReadOnly(map[string]any) bool { return true }

func (t *contextGetIntentTool) Execute(ctx context.Context, args map[string]any) (Result, error) {
	intent := t.store.GetIntentSet()
	if intent == nil {
		return Result{Content: "No intent set available"}, nil
	}
	data, _ := json.MarshalIndent(intent, "", "  ")
	return Result{Content: string(data)}, nil
}

type contextGetInitTool struct{ store taskstate.TaskStore }

func (*contextGetInitTool) Name() string { return "context_get_init" }
func (*contextGetInitTool) Description() string { return "Get the initialization phase results (exploration findings)." }
func (*contextGetInitTool) Schema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{}}
}
func (*contextGetInitTool) RequiresConfirmation(string) bool { return false }
func (*contextGetInitTool) IsConcurrencySafe(map[string]any) bool { return true }
func (*contextGetInitTool) EstimatedCost() ToolCost { return ToolCost{TokensApprox: 50, LatencyMs: 50, RiskLevel: "low"} }
func (*contextGetInitTool) IsDestructive(map[string]any) bool { return false }
func (*contextGetInitTool) IsReadOnly(map[string]any) bool { return true }

func (t *contextGetInitTool) Execute(ctx context.Context, args map[string]any) (Result, error) {
	init := t.store.GetInitResults()
	if init == nil {
		return Result{Content: "No init results available"}, nil
	}
	data, _ := json.MarshalIndent(init, "", "  ")
	return Result{Content: string(data)}, nil
}

// --- Todo tools ---

type todoListTool struct{ store taskstate.TaskStore }

func (*todoListTool) Name() string { return "todo_list" }
func (*todoListTool) Description() string { return "List all todo items from the current plan." }
func (*todoListTool) Schema() map[string]any {
	return map[string]any{
		"type": "object", "properties": map[string]any{
			"status_filter": map[string]any{"type": "string", "enum": []string{"pending", "in_progress", "completed", "blocked"}},
		},
	}
}
func (*todoListTool) RequiresConfirmation(string) bool { return false }
func (*todoListTool) IsConcurrencySafe(map[string]any) bool { return true }
func (*todoListTool) EstimatedCost() ToolCost { return ToolCost{TokensApprox: 50, LatencyMs: 50, RiskLevel: "low"} }
func (*todoListTool) IsDestructive(map[string]any) bool { return false }
func (*todoListTool) IsReadOnly(map[string]any) bool { return true }

func (t *todoListTool) Execute(ctx context.Context, args map[string]any) (Result, error) {
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

type todoNextTool struct{ store taskstate.TaskStore }

func (*todoNextTool) Name() string { return "todo_next" }
func (*todoNextTool) Description() string { return "Get the next pending todo item ready to execute." }
func (*todoNextTool) Schema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{}}
}
func (*todoNextTool) RequiresConfirmation(string) bool { return false }
func (*todoNextTool) IsConcurrencySafe(map[string]any) bool { return true }
func (*todoNextTool) EstimatedCost() ToolCost { return ToolCost{TokensApprox: 50, LatencyMs: 50, RiskLevel: "low"} }
func (*todoNextTool) IsDestructive(map[string]any) bool { return false }
func (*todoNextTool) IsReadOnly(map[string]any) bool { return true }

func (t *todoNextTool) Execute(ctx context.Context, args map[string]any) (Result, error) {
	next := t.store.GetNextPendingTask()
	if next == nil {
		return Result{Content: "No pending tasks ready to execute"}, nil
	}
	data, _ := json.MarshalIndent(next, "", "  ")
	return Result{Content: string(data)}, nil
}