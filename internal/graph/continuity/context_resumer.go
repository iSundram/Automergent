package continuity

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/iSundram/Automergent/internal/graph"
)

type ContextResumer struct {
	store  *graph.Store
	query  *graph.QueryEngine
	mu     sync.RWMutex
}

func NewContextResumer(store *graph.Store, query *graph.QueryEngine) *ContextResumer {
	return &ContextResumer{
		store: store,
		query: query,
	}
}

func (r *ContextResumer) ResumeCoderContext(ctx context.Context, taskID uuid.UUID) (*ContextBucketSnapshot, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	buckets, err := r.getTaskBuckets(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("get task buckets: %w", err)
	}

	for _, bucket := range buckets {
		if bucket.Type == "agent" && bucket.Owner == "coder" {
			return bucket, nil
		}
	}

	return r.createDefaultCoderBucket(ctx, taskID)
}

func (r *ContextResumer) ResumeAssistantContext(ctx context.Context, taskID uuid.UUID) (*ContextBucketSnapshot, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	buckets, err := r.getTaskBuckets(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("get task buckets: %w", err)
	}

	for _, bucket := range buckets {
		if bucket.Type == "agent" && bucket.Owner == "assistant" {
			return bucket, nil
		}
	}

	return r.createDefaultAssistantBucket(ctx, taskID)
}

func (r *ContextResumer) ResumeVerificationContext(ctx context.Context, taskID uuid.UUID) (*VerificationSnapshot, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.getVerificationSnapshot(ctx, taskID), nil
}

func (r *ContextResumer) GetResumeConfig(relation TaskRelation) ResumeConfig {
	return ResumeConfigForRelation(relation)
}

func (r *ContextResumer) ResumeFullContext(ctx context.Context, taskID uuid.UUID, relation TaskRelation) (*ContextResumeResult, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	taskNode, err := r.store.GetNode(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("get task: %w", err)
	}

	var task graph.Task
	if err := taskNode.UnmarshalData(&task); err != nil {
		return nil, err
	}
	task.ID = taskNode.ID

	coderCtx, _ := r.ResumeCoderContext(ctx, taskID)
	assistantCtx, _ := r.ResumeAssistantContext(ctx, taskID)
	verificationCtx, _ := r.ResumeVerificationContext(ctx, taskID)

	buckets, err := r.getTaskBuckets(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("get task buckets: %w", err)
	}

	decisions, _ := r.getTaskDecisions(ctx, taskID)
	memories, _ := r.getTaskMemories(ctx, taskID)
	files, _ := r.getTaskFiles(ctx, taskID)
	todos, _ := r.getTaskTodos(ctx, taskID)

	var sharedBuckets, excludedBuckets []uuid.UUID
	config := ResumeConfigForRelation(relation)

	for _, bucket := range buckets {
		if r.shouldShareBucket(bucket, relation, config) {
			sharedBuckets = append(sharedBuckets, bucket.BucketID)
		} else {
			excludedBuckets = append(excludedBuckets, bucket.BucketID)
		}
	}

	return &ContextResumeResult{
		TaskID:           taskID,
		Relation:         relation,
		Confidence:       1.0,
		CoderContext:     coderCtx,
		AssistantContext: assistantCtx,
		VerificationCtx:  verificationCtx,
		SharedBuckets:    sharedBuckets,
		ExcludedBuckets:  excludedBuckets,
		Decisions:        decisions,
		Memories:         memories,
		Files:            files,
		Todos:            todos,
		ResumeConfig:     config,
		GeneratedAt:      time.Now(),
	}, nil
}

func (r *ContextResumer) ResumePartialContext(ctx context.Context, taskID uuid.UUID, bucketIDs []uuid.UUID, keys []string) (*ContextResumeResult, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	buckets := make(map[uuid.UUID]*ContextBucketSnapshot)
	allBuckets, err := r.getTaskBuckets(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("get task buckets: %w", err)
	}

	for _, bucket := range allBuckets {
		for _, id := range bucketIDs {
			if bucket.BucketID == id {
				if len(keys) > 0 {
					bucket = r.filterBucketKeys(bucket, keys)
				}
				buckets[bucket.BucketID] = bucket
				break
			}
		}
	}

	decisions, _ := r.getTaskDecisions(ctx, taskID)
	memories, _ := r.getTaskMemories(ctx, taskID)
	files, _ := r.getTaskFiles(ctx, taskID)
	todos, _ := r.getTaskTodos(ctx, taskID)

	var sharedBuckets []uuid.UUID
	for id := range buckets {
		sharedBuckets = append(sharedBuckets, id)
	}

	coderCtx := buckets[uuid.Nil]
	assistantCtx := buckets[uuid.Nil]

	for _, bucket := range buckets {
		if bucket.Type == "agent" && bucket.Owner == "coder" {
			coderCtx = bucket
		}
		if bucket.Type == "agent" && bucket.Owner == "assistant" {
			assistantCtx = bucket
		}
	}

	return &ContextResumeResult{
		TaskID:           taskID,
		Relation:         TaskRelationRelated,
		Confidence:       0.5,
		CoderContext:     coderCtx,
		AssistantContext: assistantCtx,
		VerificationCtx:  r.getVerificationSnapshot(ctx, taskID),
		SharedBuckets:    sharedBuckets,
		Decisions:        decisions,
		Memories:         memories,
		Files:            files,
		Todos:            todos,
		ResumeConfig:     ResumeConfig{Policy: ResumePolicyPartial, ShareKeys: keys},
		GeneratedAt:      time.Now(),
	}, nil
}

func (r *ContextResumer) createDefaultCoderBucket(ctx context.Context, taskID uuid.UUID) (*ContextBucketSnapshot, error) {
	bucket := &graph.ContextBucket{
		Name:        "coder",
		Type:        graph.ContextBucketTypeAgent,
		Description: "Coder agent context bucket",
		SharePolicy: graph.SharePolicyShared,
		Owner:       "coder",
		Tags:        []string{"coder", "agent"},
		Metadata:    json.RawMessage("{}"),
	}

	node, err := bucket.ToNode()
	if err != nil {
		return nil, fmt.Errorf("create bucket node: %w", err)
	}

	if err := r.store.CreateNode(ctx, node); err != nil {
		return nil, fmt.Errorf("persist bucket: %w", err)
	}

	bucket.ID = node.ID

	edge, err := graph.NewEdge(taskID, bucket.ID, graph.EdgeTypeContains, nil)
	if err != nil {
		return nil, fmt.Errorf("create edge: %w", err)
	}

	if err := r.store.CreateEdge(ctx, edge); err != nil {
		return nil, fmt.Errorf("link bucket to task: %w", err)
	}

	var data map[string]interface{}
	json.Unmarshal(bucket.Metadata, &data)
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}

	dataJSON, _ := json.Marshal(data)

	return &ContextBucketSnapshot{
		BucketID:     bucket.ID,
		Name:         bucket.Name,
		Type:         string(bucket.Type),
		Owner:        bucket.Owner,
		SharePolicy:  string(bucket.SharePolicy),
		Data:         dataJSON,
		Keys:         keys,
		UpdatedAt:    time.Now(),
	}, nil
}

func (r *ContextResumer) createDefaultAssistantBucket(ctx context.Context, taskID uuid.UUID) (*ContextBucketSnapshot, error) {
	bucket := &graph.ContextBucket{
		Name:        "assistant",
		Type:        graph.ContextBucketTypeAgent,
		Description: "Assistant agent context bucket",
		SharePolicy: graph.SharePolicyShared,
		Owner:       "assistant",
		Tags:        []string{"assistant", "agent"},
		Metadata:    json.RawMessage("{}"),
	}

	node, err := bucket.ToNode()
	if err != nil {
		return nil, fmt.Errorf("create bucket node: %w", err)
	}

	if err := r.store.CreateNode(ctx, node); err != nil {
		return nil, fmt.Errorf("persist bucket: %w", err)
	}

	bucket.ID = node.ID

	edge, err := graph.NewEdge(taskID, bucket.ID, graph.EdgeTypeContains, nil)
	if err != nil {
		return nil, fmt.Errorf("create edge: %w", err)
	}

	if err := r.store.CreateEdge(ctx, edge); err != nil {
		return nil, fmt.Errorf("link bucket to task: %w", err)
	}

	var data map[string]interface{}
	json.Unmarshal(bucket.Metadata, &data)
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}

	dataJSON, _ := json.Marshal(data)

	return &ContextBucketSnapshot{
		BucketID:     bucket.ID,
		Name:         bucket.Name,
		Type:         string(bucket.Type),
		Owner:        bucket.Owner,
		SharePolicy:  string(bucket.SharePolicy),
		Data:         dataJSON,
		Keys:         keys,
		UpdatedAt:    time.Now(),
	}, nil
}

func (r *ContextResumer) filterBucketKeys(bucket *ContextBucketSnapshot, keys []string) *ContextBucketSnapshot {
	var data map[string]interface{}
	if err := json.Unmarshal(bucket.Data, &data); err != nil {
		return bucket
	}

	filtered := make(map[string]interface{})
	for _, key := range keys {
		if val, ok := data[key]; ok {
			filtered[key] = val
		}
	}

	filteredJSON, _ := json.Marshal(filtered)

	return &ContextBucketSnapshot{
		BucketID:     bucket.BucketID,
		Name:         bucket.Name,
		Type:         bucket.Type,
		Owner:        bucket.Owner,
		SharePolicy:  bucket.SharePolicy,
		Data:         filteredJSON,
		Keys:         keys,
		UpdatedAt:    bucket.UpdatedAt,
	}
}

func (r *ContextResumer) shouldShareBucket(bucket *ContextBucketSnapshot, relation TaskRelation, config ResumeConfig) bool {
	switch relation {
	case TaskRelationFollowUp:
		return true
	case TaskRelationRelated:
		if bucket.Type == "project" || bucket.Type == "session" {
			return true
		}
		if bucket.SharePolicy == "shared" || bucket.SharePolicy == "public" {
			return true
		}
		for _, id := range config.ShareBuckets {
			if bucket.BucketID == id {
				return true
			}
		}
		return false
	case TaskRelationNewTask:
		return bucket.Type == "project" || bucket.Type == "global"
	default:
		return false
	}
}

func (r *ContextResumer) getTaskBuckets(ctx context.Context, taskID uuid.UUID) ([]*ContextBucketSnapshot, error) {
	edges, err := r.store.GetEdgesFrom(ctx, taskID, graph.EdgeTypeContains)
	if err != nil {
		return nil, err
	}

	var buckets []*ContextBucketSnapshot
	for _, edge := range edges {
		bucketNode, err := r.store.GetNode(ctx, edge.ToID)
		if err != nil {
			continue
		}
		if bucketNode.Type != graph.NodeTypeContextBucket {
			continue
		}
		var bucket graph.ContextBucket
		if err := bucketNode.UnmarshalData(&bucket); err != nil {
			continue
		}
		bucket.ID = bucketNode.ID
		if snap := r.bucketToSnapshot(&bucket, bucketNode.UpdatedAt); snap != nil {
			buckets = append(buckets, snap)
		}
	}
	return buckets, nil
}

func (r *ContextResumer) getTaskDecisions(ctx context.Context, taskID uuid.UUID) ([]DecisionSummary, error) {
	edges, err := r.store.GetEdgesFrom(ctx, taskID, graph.EdgeTypeReferences)
	if err != nil {
		return nil, err
	}

	var decisions []DecisionSummary
	for _, edge := range edges {
		decNode, err := r.store.GetNode(ctx, edge.ToID)
		if err != nil || decNode.Type != graph.NodeTypeDecision {
			continue
		}
		var dec graph.Decision
		if err := decNode.UnmarshalData(&dec); err != nil {
			continue
		}
		dec.ID = decNode.ID
		decisions = append(decisions, DecisionSummary{
			ID:         dec.ID,
			Title:      dec.Title,
			Type:       string(dec.Type),
			Status:     dec.Status,
			Outcome:    dec.Outcome,
			Confidence: 0.8,
			CreatedAt:  decNode.CreatedAt,
		})
	}
	return decisions, nil
}

func (r *ContextResumer) getTaskMemories(ctx context.Context, taskID uuid.UUID) ([]MemorySummary, error) {
	edges, err := r.store.GetEdgesFrom(ctx, taskID, graph.EdgeTypeReferences)
	if err != nil {
		return nil, err
	}

	var memories []MemorySummary
	for _, edge := range edges {
		memNode, err := r.store.GetNode(ctx, edge.ToID)
		if err != nil || memNode.Type != graph.NodeTypeMemory {
			continue
		}
		var mem graph.Memory
		if err := memNode.UnmarshalData(&mem); err != nil {
			continue
		}
		mem.ID = memNode.ID
		memories = append(memories, MemorySummary{
			ID:         mem.ID,
			Content:    mem.Content,
			Type:       string(mem.Type),
			Scope:      string(mem.Scope),
			Confidence: mem.Confidence,
			CreatedAt:  memNode.CreatedAt,
		})
	}
	return memories, nil
}

func (r *ContextResumer) getTaskFiles(ctx context.Context, taskID uuid.UUID) ([]FileSummary, error) {
	edges, err := r.store.GetEdgesFrom(ctx, taskID, graph.EdgeTypeReferences)
	if err != nil {
		return nil, err
	}

	var files []FileSummary
	for _, edge := range edges {
		fileNode, err := r.store.GetNode(ctx, edge.ToID)
		if err != nil || fileNode.Type != graph.NodeTypeFile {
			continue
		}
		var file graph.File
		if err := fileNode.UnmarshalData(&file); err != nil {
			continue
		}
		file.ID = fileNode.ID
		files = append(files, FileSummary{
			ID:       file.ID,
			Path:     file.Path,
			Name:     file.Name,
			Language: file.Language,
			Hash:     file.Hash,
			Size:     file.Size,
		})
	}
	return files, nil
}

func (r *ContextResumer) getTaskTodos(ctx context.Context, taskID uuid.UUID) ([]TodoSummary, error) {
	edges, err := r.store.GetEdgesFrom(ctx, taskID, graph.EdgeTypeContains)
	if err != nil {
		return nil, err
	}

	var todos []TodoSummary
	for _, edge := range edges {
		todoNode, err := r.store.GetNode(ctx, edge.ToID)
		if err != nil || todoNode.Type != graph.NodeTypeTodo {
			continue
		}
		var todo graph.Todo
		if err := todoNode.UnmarshalData(&todo); err != nil {
			continue
		}
		todo.ID = todoNode.ID
		status := "pending"
		if todo.Completed {
			status = "done"
		}
		todos = append(todos, TodoSummary{
			ID:        todo.ID,
			Title:     todo.Title,
			Status:    status,
			Priority:  todo.Priority,
			Completed: todo.Completed,
			CreatedAt: todoNode.CreatedAt,
		})
	}
	return todos, nil
}

func (r *ContextResumer) getVerificationSnapshot(ctx context.Context, taskID uuid.UUID) *VerificationSnapshot {
	return &VerificationSnapshot{
		Status:         "pending",
		LastVerifiedAt: time.Time{},
		Coverage:       0.0,
	}
}

func (r *ContextResumer) bucketToSnapshot(bucket *graph.ContextBucket, updatedAt time.Time) *ContextBucketSnapshot {
	var data map[string]interface{}
	if err := json.Unmarshal(bucket.Metadata, &data); err != nil {
		data = make(map[string]interface{})
	}

	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}

	dataJSON, _ := json.Marshal(data)

	return &ContextBucketSnapshot{
		BucketID:     bucket.ID,
		Name:         bucket.Name,
		Type:         string(bucket.Type),
		Owner:        bucket.Owner,
		SharePolicy:  string(bucket.SharePolicy),
		Data:         dataJSON,
		Keys:         keys,
		UpdatedAt:    updatedAt,
	}
}

func (r *ContextResumer) InjectContext(ctx context.Context, taskID uuid.UUID, owner string, key string, value interface{}) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	buckets, err := r.getTaskBuckets(ctx, taskID)
	if err != nil {
		return err
	}

	var targetBucket *ContextBucketSnapshot
	for _, bucket := range buckets {
		if bucket.Type == "agent" && bucket.Owner == owner {
			targetBucket = bucket
			break
		}
	}

	if targetBucket == nil {
		var newBucket *ContextBucketSnapshot
		if owner == "coder" {
			newBucket, err = r.createDefaultCoderBucket(ctx, taskID)
		} else {
			newBucket, err = r.createDefaultAssistantBucket(ctx, taskID)
		}
		if err != nil {
			return err
		}
		targetBucket = newBucket
	}

	var data map[string]interface{}
	if err := json.Unmarshal(targetBucket.Data, &data); err != nil {
		data = make(map[string]interface{})
	}

	data[key] = value

	updatedData, _ := json.Marshal(data)

	bucketNode, err := r.store.GetNode(ctx, targetBucket.BucketID)
	if err != nil {
		return err
	}

	var bucket graph.ContextBucket
	if err := bucketNode.UnmarshalData(&bucket); err != nil {
		return err
	}

	bucket.Metadata = updatedData

	updatedNode, err := bucket.ToNode()
	if err != nil {
		return err
	}
	updatedNode.ID = bucketNode.ID

	return r.store.UpdateNode(ctx, updatedNode)
}

func (r *ContextResumer) GetContextKeys(ctx context.Context, taskID uuid.UUID, owner string) ([]string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	buckets, err := r.getTaskBuckets(ctx, taskID)
	if err != nil {
		return nil, err
	}

	for _, bucket := range buckets {
		if bucket.Type == "agent" && bucket.Owner == owner {
			return bucket.Keys, nil
		}
	}

	return []string{}, nil
}