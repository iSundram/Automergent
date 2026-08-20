package contexttools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/iSundram/Automergent/internal/graph/workflow"
	"github.com/iSundram/Automergent/internal/tools"
)

// Register adds the graph-backed context tools to a tool registry. These tools
// mutate only graph context; they do not edit product files.
func Register(reg *tools.Registry, buckets *workflow.ContextBucketManager, remember *workflow.RememberTool) {
	if reg == nil || buckets == nil {
		return
	}
	reg.Register(&bucketCreateTool{buckets: buckets})
	reg.Register(&bucketListTool{buckets: buckets})
	reg.Register(&bucketGetTool{buckets: buckets})
	reg.Register(&bucketUpdateTool{buckets: buckets})
	reg.Register(&bucketShareTool{buckets: buckets})
	if remember != nil {
		reg.Register(&rememberTool{remember: remember})
	}
}

type bucketCreateTool struct {
	buckets *workflow.ContextBucketManager
}

func (*bucketCreateTool) Name() string { return "context_bucket_create" }
func (*bucketCreateTool) Description() string {
	return "Create a context bucket for a task, workflow, agent, phase, or todo."
}
func (*bucketCreateTool) Schema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{
		"task_id": map[string]any{"type": "string"}, "todo_id": map[string]any{"type": "string"},
		"type": map[string]any{"type": "string"}, "name": map[string]any{"type": "string"},
		"description": map[string]any{"type": "string"}, "owner": map[string]any{"type": "string"},
		"share_policy": map[string]any{"type": "string"},
	}, "required": []string{"name"}}
}
func (t *bucketCreateTool) Execute(ctx context.Context, args map[string]any) (tools.Result, error) {
	name, _ := args["name"].(string)
	if name == "" {
		return tools.Result{IsError: true, Content: "name is required"}, nil
	}
	description, _ := args["description"].(string)
	owner, _ := args["owner"].(string)
	if owner == "" {
		owner = "agent"
	}
	policy := workflow.SharePolicyPartial
	if value, ok := args["share_policy"].(string); ok && value != "" {
		policy = workflow.SharePolicy(value)
	}
	if todoID, ok := args["todo_id"].(string); ok && todoID != "" {
		id, err := uuid.Parse(todoID)
		if err != nil {
			return tools.Result{IsError: true, Content: err.Error()}, nil
		}
		bucket, err := t.buckets.CreateBucketForTodo(ctx, id, name, description, owner, policy)
		return marshalResult(bucket, err)
	}
	taskID, ok := args["task_id"].(string)
	if !ok || taskID == "" {
		return tools.Result{IsError: true, Content: "task_id or todo_id is required"}, nil
	}
	id, err := uuid.Parse(taskID)
	if err != nil {
		return tools.Result{IsError: true, Content: err.Error()}, nil
	}
	typeName, _ := args["type"].(string)
	if typeName == "" {
		typeName = string(workflow.ContextBucketTypeTask)
	}
	bucket, err := t.buckets.CreateBucket(ctx, id, workflow.ContextBucketType(typeName), name, description, owner, policy)
	return marshalResult(bucket, err)
}

type bucketListTool struct {
	buckets *workflow.ContextBucketManager
}

func (*bucketListTool) Name() string { return "context_bucket_list" }
func (*bucketListTool) Description() string {
	return "List graph context buckets associated with a task."
}
func (*bucketListTool) Schema() map[string]any {
	return objectSchema(map[string]any{"task_id": map[string]any{"type": "string"}}, []string{"task_id"})
}
func (t *bucketListTool) Execute(ctx context.Context, args map[string]any) (tools.Result, error) {
	id, err := parseArgUUID(args, "task_id")
	if err != nil {
		return tools.Result{IsError: true, Content: err.Error()}, nil
	}
	buckets, err := t.buckets.ListBuckets(ctx, id)
	return marshalResult(buckets, err)
}

type bucketGetTool struct {
	buckets *workflow.ContextBucketManager
}

func (*bucketGetTool) Name() string { return "context_bucket_get" }
func (*bucketGetTool) Description() string {
	return "Inspect a context bucket summary using its UUID or compact cN handle."
}
func (*bucketGetTool) Schema() map[string]any {
	return objectSchema(map[string]any{"bucket_id": map[string]any{"type": "string"}}, []string{"bucket_id"})
}
func (t *bucketGetTool) Execute(ctx context.Context, args map[string]any) (tools.Result, error) {
	id, err := parseBucketID(ctx, t.buckets, args, "bucket_id")
	if err != nil {
		return tools.Result{IsError: true, Content: err.Error()}, nil
	}
	summary, err := t.buckets.GetBucketSummary(ctx, id)
	return marshalResult(summary, err)
}

type bucketUpdateTool struct {
	buckets *workflow.ContextBucketManager
}

func (*bucketUpdateTool) Name() string        { return "context_bucket_update" }
func (*bucketUpdateTool) Description() string { return "Store a structured value in a context bucket." }
func (*bucketUpdateTool) Schema() map[string]any {
	return objectSchema(map[string]any{"bucket_id": map[string]any{"type": "string"}, "key": map[string]any{"type": "string"}, "value": map[string]any{}}, []string{"bucket_id", "key", "value"})
}
func (t *bucketUpdateTool) Execute(ctx context.Context, args map[string]any) (tools.Result, error) {
	id, err := parseBucketID(ctx, t.buckets, args, "bucket_id")
	if err != nil {
		return tools.Result{IsError: true, Content: err.Error()}, nil
	}
	key, _ := args["key"].(string)
	if key == "" {
		return tools.Result{IsError: true, Content: "key is required"}, nil
	}
	err = t.buckets.UpdateBucketData(ctx, id, key, args["value"])
	return marshalResult(map[string]any{"bucket_id": id, "key": key}, err)
}

type bucketShareTool struct {
	buckets *workflow.ContextBucketManager
}

func (*bucketShareTool) Name() string { return "context_share" }
func (*bucketShareTool) Description() string {
	return "Share selected context from one bucket to another using an explicit policy."
}
func (*bucketShareTool) Schema() map[string]any {
	return objectSchema(map[string]any{"from_bucket_id": map[string]any{"type": "string"}, "to_bucket_id": map[string]any{"type": "string"}, "policy": map[string]any{"type": "string"}}, []string{"from_bucket_id", "to_bucket_id", "policy"})
}
func (t *bucketShareTool) Execute(ctx context.Context, args map[string]any) (tools.Result, error) {
	from, err := parseBucketID(ctx, t.buckets, args, "from_bucket_id")
	if err != nil {
		return tools.Result{IsError: true, Content: err.Error()}, nil
	}
	to, err := parseBucketID(ctx, t.buckets, args, "to_bucket_id")
	if err != nil {
		return tools.Result{IsError: true, Content: err.Error()}, nil
	}
	policy, _ := args["policy"].(string)
	err = t.buckets.ShareContext(ctx, from, to, workflow.SharePolicy(policy))
	return marshalResult(map[string]any{"from": from, "to": to, "policy": policy}, err)
}

type rememberTool struct{ remember *workflow.RememberTool }

func (*rememberTool) Name() string { return "remember" }
func (*rememberTool) Description() string {
	return "Inject a concise message into a specific todo context bucket for a later step or agent."
}
func (*rememberTool) Schema() map[string]any {
	return objectSchema(map[string]any{"todo_id": map[string]any{"type": "string"}, "from_agent": map[string]any{"type": "string"}, "message": map[string]any{"type": "string"}}, []string{"todo_id", "message"})
}
func (t *rememberTool) Execute(ctx context.Context, args map[string]any) (tools.Result, error) {
	id, err := parseArgUUID(args, "todo_id")
	if err != nil {
		return tools.Result{IsError: true, Content: err.Error()}, nil
	}
	message, _ := args["message"].(string)
	if message == "" {
		return tools.Result{IsError: true, Content: "message is required"}, nil
	}
	from, _ := args["from_agent"].(string)
	if from == "" {
		from = "assistant"
	}
	item, err := t.remember.InjectMessage(ctx, id, from, message)
	return marshalResult(item, err)
}

func objectSchema(properties map[string]any, required []string) map[string]any {
	return map[string]any{"type": "object", "properties": properties, "required": required}
}
func parseArgUUID(args map[string]any, key string) (uuid.UUID, error) {
	value, ok := args[key].(string)
	if !ok || value == "" {
		return uuid.Nil, fmt.Errorf("%s is required", key)
	}
	return uuid.Parse(value)
}
func parseBucketID(ctx context.Context, buckets *workflow.ContextBucketManager, args map[string]any, key string) (uuid.UUID, error) {
	value, ok := args[key].(string)
	if !ok || value == "" {
		return uuid.Nil, fmt.Errorf("%s is required", key)
	}
	return buckets.ResolveBucketID(ctx, value)
}
func marshalResult(value any, err error) (tools.Result, error) {
	if err != nil {
		return tools.Result{IsError: true, Content: err.Error()}, nil
	}
	data, e := json.Marshal(value)
	if e != nil {
		return tools.Result{IsError: true, Content: e.Error()}, nil
	}
	return tools.Result{Content: string(data)}, nil
}

// Context tools are graph metadata operations and are safe to run concurrently
// only when their target buckets differ; keep the conservative default.
func (t *bucketCreateTool) RequiresConfirmation(string) bool      { return false }
func (t *bucketCreateTool) IsConcurrencySafe(map[string]any) bool { return false }
func (t *bucketCreateTool) IsReadOnly(map[string]any) bool        { return false }
func (t *bucketCreateTool) IsDestructive(map[string]any) bool     { return false }
func (t *bucketCreateTool) EstimatedCost() tools.ToolCost         { return tools.ToolCost{RiskLevel: "low"} }
func (t *bucketListTool) RequiresConfirmation(string) bool        { return false }
func (t *bucketListTool) IsConcurrencySafe(map[string]any) bool   { return true }
func (t *bucketListTool) IsReadOnly(map[string]any) bool          { return true }
func (t *bucketListTool) IsDestructive(map[string]any) bool       { return false }
func (t *bucketListTool) EstimatedCost() tools.ToolCost           { return tools.ToolCost{RiskLevel: "low"} }
func (t *bucketGetTool) RequiresConfirmation(string) bool         { return false }
func (t *bucketGetTool) IsConcurrencySafe(map[string]any) bool    { return true }
func (t *bucketGetTool) IsReadOnly(map[string]any) bool           { return true }
func (t *bucketGetTool) IsDestructive(map[string]any) bool        { return false }
func (t *bucketGetTool) EstimatedCost() tools.ToolCost            { return tools.ToolCost{RiskLevel: "low"} }
func (t *bucketUpdateTool) RequiresConfirmation(string) bool      { return false }
func (t *bucketUpdateTool) IsConcurrencySafe(map[string]any) bool { return false }
func (t *bucketUpdateTool) IsReadOnly(map[string]any) bool        { return false }
func (t *bucketUpdateTool) IsDestructive(map[string]any) bool     { return false }
func (t *bucketUpdateTool) EstimatedCost() tools.ToolCost         { return tools.ToolCost{RiskLevel: "low"} }
func (t *bucketShareTool) RequiresConfirmation(string) bool       { return false }
func (t *bucketShareTool) IsConcurrencySafe(map[string]any) bool  { return false }
func (t *bucketShareTool) IsReadOnly(map[string]any) bool         { return false }
func (t *bucketShareTool) IsDestructive(map[string]any) bool      { return false }
func (t *bucketShareTool) EstimatedCost() tools.ToolCost          { return tools.ToolCost{RiskLevel: "low"} }
func (t *rememberTool) RequiresConfirmation(string) bool          { return false }
func (t *rememberTool) IsConcurrencySafe(map[string]any) bool     { return false }
func (t *rememberTool) IsReadOnly(map[string]any) bool            { return false }
func (t *rememberTool) IsDestructive(map[string]any) bool         { return false }
func (t *rememberTool) EstimatedCost() tools.ToolCost             { return tools.ToolCost{RiskLevel: "low"} }
