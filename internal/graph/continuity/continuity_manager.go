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

type ContinuityManager struct {
	store       *graph.Store
	query       *graph.QueryEngine
	config      ContinuityConfig
	mu          sync.RWMutex
}

func NewContinuityManager(store *graph.Store, query *graph.QueryEngine, config ContinuityConfig) *ContinuityManager {
	if config.SimilarityThreshold == 0 {
		config = DefaultContinuityConfig()
	}
	return &ContinuityManager{
		store:  store,
		query:  query,
		config: config,
	}
}

func (m *ContinuityManager) DetectRelation(ctx context.Context, currentTask *graph.Task, previousTasks []*graph.Task) (TaskRelation, float64, error) {
	if len(previousTasks) == 0 {
		return TaskRelationNewTask, 1.0, nil
	}

	var bestRelation TaskRelation
	var bestConfidence float64

	for _, prevTask := range previousTasks {
		if prevTask.ID == currentTask.ID {
			continue
		}

		comparison, err := m.CompareTasks(ctx, currentTask, prevTask)
		if err != nil {
			continue
		}

		if comparison.Confidence > bestConfidence {
			bestConfidence = comparison.Confidence
			bestRelation = comparison.Relation
		}
	}

	if bestConfidence >= m.config.FollowUpConfidenceMin && bestRelation == TaskRelationFollowUp {
		return TaskRelationFollowUp, bestConfidence, nil
	}

	if bestConfidence >= m.config.RelatedConfidenceMin && bestRelation == TaskRelationRelated {
		return TaskRelationRelated, bestConfidence, nil
	}

	return TaskRelationNewTask, 1.0 - bestConfidence, nil
}

func (m *ContinuityManager) ResumeContext(ctx context.Context, taskID uuid.UUID, relation TaskRelation) (map[string]*ContextBucketSnapshot, error) {
	buckets := make(map[string]*ContextBucketSnapshot)

	edges, err := m.store.GetEdgesFrom(ctx, taskID, graph.EdgeTypeContains)
	if err != nil {
		return nil, fmt.Errorf("get task edges: %w", err)
	}

	for _, edge := range edges {
		if edge.Type != graph.EdgeTypeContains {
			continue
		}

		bucketNode, err := m.store.GetNode(ctx, edge.ToID)
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

		snapshot := m.bucketToSnapshot(&bucket, bucketNode.UpdatedAt)
		if snapshot != nil {
			key := fmt.Sprintf("%s:%s", bucket.Type, bucket.Owner)
			buckets[key] = snapshot
		}
	}

	return m.filterBucketsByRelation(buckets, relation), nil
}

func (m *ContinuityManager) GetContextResumeResult(ctx context.Context, taskID uuid.UUID) (*ContextResumeResult, error) {
	taskNode, err := m.store.GetNode(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("get task: %w", err)
	}

	var task graph.Task
	if err := taskNode.UnmarshalData(&task); err != nil {
		return nil, err
	}
	task.ID = taskNode.ID

	previousTasks, err := m.GetPreviousTasks(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("get previous tasks: %w", err)
	}

	relation, confidence, err := m.DetectRelation(ctx, &task, previousTasks)
	if err != nil {
		return nil, fmt.Errorf("detect relation: %w", err)
	}

	buckets, err := m.ResumeContext(ctx, taskID, relation)
	if err != nil {
		return nil, fmt.Errorf("resume context: %w", err)
	}

	decisions, _ := m.getTaskDecisions(ctx, taskID)
	memories, _ := m.getTaskMemories(ctx, taskID)
	files, _ := m.getTaskFiles(ctx, taskID)
	todos, _ := m.getTaskTodos(ctx, taskID)

	var sharedBuckets, excludedBuckets []uuid.UUID
	for key, bucket := range buckets {
		if m.shouldShareBucket(bucket, relation) {
			sharedBuckets = append(sharedBuckets, bucket.BucketID)
		} else {
			excludedBuckets = append(excludedBuckets, bucket.BucketID)
		}
		_ = key
	}

	coderCtx := buckets["agent:coder"]
	assistantCtx := buckets["agent:assistant"]
	verificationCtx := m.getVerificationSnapshot(ctx, taskID)

	return &ContextResumeResult{
		TaskID:           taskID,
		Relation:         relation,
		Confidence:       confidence,
		CoderContext:     coderCtx,
		AssistantContext: assistantCtx,
		VerificationCtx:  verificationCtx,
		SharedBuckets:    sharedBuckets,
		ExcludedBuckets:  excludedBuckets,
		Decisions:        decisions,
		Memories:         memories,
		Files:            files,
		Todos:            todos,
		ResumeConfig:     ResumeConfigForRelation(relation),
		GeneratedAt:      time.Now(),
	}, nil
}

func (m *ContinuityManager) CompareTasks(ctx context.Context, taskA, taskB *graph.Task) (*TaskComparison, error) {
	comparison := &TaskComparison{
		TaskAID:    taskA.ID,
		TaskBID:    taskB.ID,
		ComparedAt: time.Now(),
	}

	bucketsA, _ := m.getTaskBuckets(ctx, taskA.ID)
	bucketsB, _ := m.getTaskBuckets(ctx, taskB.ID)

	decisionsA, _ := m.getTaskDecisions(ctx, taskA.ID)
	decisionsB, _ := m.getTaskDecisions(ctx, taskB.ID)

	memoriesA, _ := m.getTaskMemories(ctx, taskA.ID)
	memoriesB, _ := m.getTaskMemories(ctx, taskB.ID)

	filesA, _ := m.getTaskFiles(ctx, taskA.ID)
	filesB, _ := m.getTaskFiles(ctx, taskB.ID)

	todosA, _ := m.getTaskTodos(ctx, taskA.ID)
	todosB, _ := m.getTaskTodos(ctx, taskB.ID)

	bucketMapA := make(map[uuid.UUID]bool)
	bucketMapB := make(map[uuid.UUID]bool)
	for _, b := range bucketsA {
		bucketMapA[b.BucketID] = true
	}
	for _, b := range bucketsB {
		bucketMapB[b.BucketID] = true
	}

	for id := range bucketMapA {
		if bucketMapB[id] {
			comparison.SharedBuckets = append(comparison.SharedBuckets, id)
		} else {
			comparison.Differences.BucketsOnlyInA = append(comparison.Differences.BucketsOnlyInA, id)
		}
	}
	for id := range bucketMapB {
		if !bucketMapA[id] {
			comparison.Differences.BucketsOnlyInB = append(comparison.Differences.BucketsOnlyInB, id)
		}
	}

	decisionMapA := make(map[uuid.UUID]bool)
	decisionMapB := make(map[uuid.UUID]bool)
	for _, d := range decisionsA {
		decisionMapA[d.ID] = true
	}
	for _, d := range decisionsB {
		decisionMapB[d.ID] = true
	}

	for id := range decisionMapA {
		if decisionMapB[id] {
			comparison.SharedDecisions = append(comparison.SharedDecisions, id)
		} else {
			comparison.Differences.DecisionsOnlyInA = append(comparison.Differences.DecisionsOnlyInA, id)
		}
	}
	for id := range decisionMapB {
		if !decisionMapA[id] {
			comparison.Differences.DecisionsOnlyInB = append(comparison.Differences.DecisionsOnlyInB, id)
		}
	}

	memoryMapA := make(map[uuid.UUID]bool)
	memoryMapB := make(map[uuid.UUID]bool)
	for _, mem := range memoriesA {
		memoryMapA[mem.ID] = true
	}
	for _, mem := range memoriesB {
		memoryMapB[mem.ID] = true
	}

	for id := range memoryMapA {
		if memoryMapB[id] {
			comparison.SharedMemories = append(comparison.SharedMemories, id)
		} else {
			comparison.Differences.MemoriesOnlyInA = append(comparison.Differences.MemoriesOnlyInA, id)
		}
	}
	for id := range memoryMapB {
		if !memoryMapA[id] {
			comparison.Differences.MemoriesOnlyInB = append(comparison.Differences.MemoriesOnlyInB, id)
		}
	}

	fileMapA := make(map[uuid.UUID]bool)
	fileMapB := make(map[uuid.UUID]bool)
	for _, f := range filesA {
		fileMapA[f.ID] = true
	}
	for _, f := range filesB {
		fileMapB[f.ID] = true
	}

	for id := range fileMapA {
		if fileMapB[id] {
			comparison.SharedFiles = append(comparison.SharedFiles, id)
		} else {
			comparison.Differences.FilesOnlyInA = append(comparison.Differences.FilesOnlyInA, id)
		}
	}
	for id := range fileMapB {
		if !fileMapA[id] {
			comparison.Differences.FilesOnlyInB = append(comparison.Differences.FilesOnlyInB, id)
		}
	}

	todoMapA := make(map[uuid.UUID]bool)
	todoMapB := make(map[uuid.UUID]bool)
	for _, t := range todosA {
		todoMapA[t.ID] = true
	}
	for _, t := range todosB {
		todoMapB[t.ID] = true
	}

	for id := range todoMapA {
		if todoMapB[id] {
			// shared todos not tracked separately
		} else {
			comparison.Differences.TodosOnlyInA = append(comparison.Differences.TodosOnlyInA, id)
		}
	}
	for id := range todoMapB {
		if !todoMapA[id] {
			comparison.Differences.TodosOnlyInB = append(comparison.Differences.TodosOnlyInB, id)
		}
	}

	comparison.Similarity = m.calculateSimilarity(comparison)
	comparison.Relation, comparison.Confidence = m.determineRelation(comparison)

	return comparison, nil
}

func (m *ContinuityManager) bucketToSnapshot(bucket *graph.ContextBucket, updatedAt time.Time) *ContextBucketSnapshot {
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

func (m *ContinuityManager) filterBucketsByRelation(buckets map[string]*ContextBucketSnapshot, relation TaskRelation) map[string]*ContextBucketSnapshot {
	result := make(map[string]*ContextBucketSnapshot)

	for key, bucket := range buckets {
		if m.shouldShareBucket(bucket, relation) {
			result[key] = bucket
		}
	}

	return result
}

func (m *ContinuityManager) shouldShareBucket(bucket *ContextBucketSnapshot, relation TaskRelation) bool {
	switch relation {
	case TaskRelationFollowUp:
		return true
	case TaskRelationRelated:
		return bucket.Type == "project" || bucket.Type == "session" || bucket.SharePolicy == "shared" || bucket.SharePolicy == "public"
	case TaskRelationNewTask:
		return bucket.Type == "project" || bucket.Type == "global"
	default:
		return false
	}
}

func (m *ContinuityManager) GetPreviousTasks(ctx context.Context, taskID uuid.UUID) ([]*graph.Task, error) {
	edges, err := m.store.GetEdgesTo(ctx, taskID, graph.EdgeTypePrevious)
	if err != nil {
		return nil, err
	}

	var tasks []*graph.Task
	for _, edge := range edges {
		if len(tasks) >= m.config.MaxPreviousTasks {
			break
		}
		task, err := m.getTaskByNodeID(ctx, edge.FromID)
		if err == nil {
			tasks = append(tasks, task)
		}
	}

	return tasks, nil
}

func (m *ContinuityManager) getTaskByNodeID(ctx context.Context, nodeID uuid.UUID) (*graph.Task, error) {
	node, err := m.store.GetNode(ctx, nodeID)
	if err != nil {
		return nil, err
	}
	if node.Type != graph.NodeTypeTask {
		return nil, fmt.Errorf("not a task")
	}
	var task graph.Task
	if err := node.UnmarshalData(&task); err != nil {
		return nil, err
	}
	task.ID = node.ID
	return &task, nil
}

func (m *ContinuityManager) getTaskBuckets(ctx context.Context, taskID uuid.UUID) ([]*ContextBucketSnapshot, error) {
	edges, err := m.store.GetEdgesFrom(ctx, taskID, graph.EdgeTypeContains)
	if err != nil {
		return nil, err
	}

	var buckets []*ContextBucketSnapshot
	for _, edge := range edges {
		bucketNode, err := m.store.GetNode(ctx, edge.ToID)
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
		if snap := m.bucketToSnapshot(&bucket, bucketNode.UpdatedAt); snap != nil {
			buckets = append(buckets, snap)
		}
	}
	return buckets, nil
}

func (m *ContinuityManager) getTaskDecisions(ctx context.Context, taskID uuid.UUID) ([]DecisionSummary, error) {
	edges, err := m.store.GetEdgesFrom(ctx, taskID, graph.EdgeTypeReferences)
	if err != nil {
		return nil, err
	}

	var decisions []DecisionSummary
	for _, edge := range edges {
		decNode, err := m.store.GetNode(ctx, edge.ToID)
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

func (m *ContinuityManager) getTaskMemories(ctx context.Context, taskID uuid.UUID) ([]MemorySummary, error) {
	edges, err := m.store.GetEdgesFrom(ctx, taskID, graph.EdgeTypeReferences)
	if err != nil {
		return nil, err
	}

	var memories []MemorySummary
	for _, edge := range edges {
		memNode, err := m.store.GetNode(ctx, edge.ToID)
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

func (m *ContinuityManager) getTaskFiles(ctx context.Context, taskID uuid.UUID) ([]FileSummary, error) {
	edges, err := m.store.GetEdgesFrom(ctx, taskID, graph.EdgeTypeReferences)
	if err != nil {
		return nil, err
	}

	var files []FileSummary
	for _, edge := range edges {
		fileNode, err := m.store.GetNode(ctx, edge.ToID)
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

func (m *ContinuityManager) getTaskTodos(ctx context.Context, taskID uuid.UUID) ([]TodoSummary, error) {
	edges, err := m.store.GetEdgesFrom(ctx, taskID, graph.EdgeTypeContains)
	if err != nil {
		return nil, err
	}

	var todos []TodoSummary
	for _, edge := range edges {
		todoNode, err := m.store.GetNode(ctx, edge.ToID)
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

func (m *ContinuityManager) getVerificationSnapshot(ctx context.Context, taskID uuid.UUID) *VerificationSnapshot {
	return &VerificationSnapshot{
		Status:         "pending",
		LastVerifiedAt: time.Time{},
		Coverage:       0.0,
	}
}

func (m *ContinuityManager) calculateSimilarity(comp *TaskComparison) float64 {
	totalShared := len(comp.SharedBuckets) + len(comp.SharedDecisions) + len(comp.SharedMemories) + len(comp.SharedFiles)
	totalA := totalShared + len(comp.Differences.BucketsOnlyInA) + len(comp.Differences.DecisionsOnlyInA) + len(comp.Differences.MemoriesOnlyInA) + len(comp.Differences.FilesOnlyInA)
	totalB := totalShared + len(comp.Differences.BucketsOnlyInB) + len(comp.Differences.DecisionsOnlyInB) + len(comp.Differences.MemoriesOnlyInB) + len(comp.Differences.FilesOnlyInB)

	if totalA == 0 && totalB == 0 {
		return 0.0
	}

	return float64(totalShared*2) / float64(totalA+totalB)
}

func (m *ContinuityManager) determineRelation(comp *TaskComparison) (TaskRelation, float64) {
	sim := comp.Similarity

	if sim >= m.config.FollowUpConfidenceMin {
		return TaskRelationFollowUp, sim
	}
	if sim >= m.config.RelatedConfidenceMin {
		return TaskRelationRelated, sim
	}
	return TaskRelationNewTask, 1.0 - sim
}

func (m *ContinuityManager) GetConfig() ContinuityConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.config
}

func (m *ContinuityManager) UpdateConfig(config ContinuityConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.config = config
}