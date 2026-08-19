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

type TaskRouter struct {
	store           *graph.Store
	query           *graph.QueryEngine
	continuityMgr   *ContinuityManager
	contextResumer  *ContextResumer
	mu              sync.RWMutex
}

type Message struct {
	ID        uuid.UUID       `json:"id"`
	Role      string          `json:"role"`
	Content   string          `json:"content"`
	Timestamp time.Time       `json:"timestamp"`
	Metadata  json.RawMessage `json:"metadata,omitempty"`
}

func NewTaskRouter(store *graph.Store, query *graph.QueryEngine, continuityMgr *ContinuityManager, contextResumer *ContextResumer) *TaskRouter {
	return &TaskRouter{
		store:          store,
		query:          query,
		continuityMgr:  continuityMgr,
		contextResumer: contextResumer,
	}
}

func (r *TaskRouter) RouteTask(ctx context.Context, userMessage string, contextData map[string]interface{}) (*RouteResult, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	messages, err := r.getRecentMessages(ctx, contextData)
	if err != nil {
		return nil, fmt.Errorf("get recent messages: %w", err)
	}

	boundaries := r.DetectTaskBoundary(messages)

	var taskID *uuid.UUID
	var relation TaskRelation
	var confidence float64
	var reason string

	if len(boundaries) > 0 {
		lastBoundary := boundaries[len(boundaries)-1]
		if lastBoundary.NewTaskID != nil {
			taskID = lastBoundary.NewTaskID
			relation = TaskRelationNewTask
			confidence = lastBoundary.Confidence
			reason = fmt.Sprintf("New task detected: %s", lastBoundary.Reason)
		} else if lastBoundary.PreviousTaskID != nil {
			taskID = lastBoundary.PreviousTaskID
			relation = TaskRelationFollowUp
			confidence = lastBoundary.Confidence
			reason = fmt.Sprintf("Continuing task: %s", lastBoundary.Reason)
		}
	}

	if taskID == nil {
		activeTaskID := r.getActiveTaskID(contextData)
		if activeTaskID != nil {
			taskID = activeTaskID
			task, err := r.getTaskByID(ctx, *taskID)
			if err == nil {
				previousTasks, _ := r.continuityMgr.GetPreviousTasks(ctx, *taskID)
				relation, confidence, _ = r.continuityMgr.DetectRelation(ctx, task, previousTasks)
				reason = fmt.Sprintf("Routing to active task with relation: %s", relation)
			}
		}
	}

	if taskID == nil {
		relation = TaskRelationNewTask
		confidence = 1.0
		reason = "No active task found, creating new task"
	}

	priority := r.GetTaskPriority(ctx, taskID, userMessage, relation)

	handler := r.determineHandler(relation, priority)

	return &RouteResult{
		Handler:    handler,
		TaskID:     taskID,
		Relation:   relation,
		Confidence: confidence,
		Reason:     reason,
		Priority:   priority,
		Boundaries: boundaries,
	}, nil
}

func (r *TaskRouter) GetTaskPriority(ctx context.Context, taskID *uuid.UUID, userMessage string, relation TaskRelation) TaskPriority {
	priority := TaskPriority{
		TaskID:        uuid.Nil,
		Score:         0.5,
		Complexity:    0.5,
		Urgency:       0.5,
		Reason:        "Default priority",
		CalculatedAt:  time.Now(),
	}

	if taskID != nil {
		priority.TaskID = *taskID
		task, err := r.getTaskByID(ctx, *taskID)
		if err == nil {
			priority.Complexity = r.calculateComplexity(task)
			priority.Urgency = r.calculateUrgency(task, userMessage)
			priority.DependencyCount = r.countDependencies(ctx, *taskID)
		}
	} else {
		priority.Complexity = r.estimateComplexity(userMessage)
		priority.Urgency = r.estimateUrgency(userMessage)
	}

	switch relation {
	case TaskRelationFollowUp:
		priority.Score = (priority.Complexity*0.3 + priority.Urgency*0.5 + float64(priority.DependencyCount)*0.05) * 1.2
		priority.Reason = "Follow-up task with existing context"
	case TaskRelationRelated:
		priority.Score = (priority.Complexity*0.4 + priority.Urgency*0.4 + float64(priority.DependencyCount)*0.1) * 1.1
		priority.Reason = "Related task sharing partial context"
	case TaskRelationNewTask:
		priority.Score = priority.Complexity*0.5 + priority.Urgency*0.5
		priority.Reason = "New task requiring full setup"
	}

	if priority.Score > 1.0 {
		priority.Score = 1.0
	}

	return priority
}

func (r *TaskRouter) DetectTaskBoundary(messages []Message) []TaskBoundary {
	if len(messages) < 2 {
		return []TaskBoundary{}
	}

	var boundaries []TaskBoundary

	for i := 1; i < len(messages); i++ {
		prev := messages[i-1]
		curr := messages[i]

		boundary := r.analyzeBoundary(prev, curr, i)
		if boundary != nil {
			boundaries = append(boundaries, *boundary)
		}
	}

	return boundaries
}

func (r *TaskRouter) analyzeBoundary(prev, curr Message, index int) *TaskBoundary {
	topicShift := r.detectTopicShift(prev.Content, curr.Content)
	newRequest := r.detectNewRequest(curr.Content)
	completion := r.detectCompletion(prev.Content)
	interruption := r.detectInterruption(prev.Content, curr.Content)
	contextReset := r.detectContextReset(curr.Content)

	var maxConfidence float64
	var boundaryType BoundaryType
	var reason string

	checks := []struct {
		detected bool
		conf     float64
		btype    BoundaryType
		reason   string
	}{
		{topicShift.detected, topicShift.confidence, BoundaryTypeTopicShift, topicShift.reason},
		{newRequest.detected, newRequest.confidence, BoundaryTypeNewRequest, newRequest.reason},
		{completion.detected, completion.confidence, BoundaryTypeCompletion, completion.reason},
		{interruption.detected, interruption.confidence, BoundaryTypeInterruption, interruption.reason},
		{contextReset.detected, contextReset.confidence, BoundaryTypeContextReset, contextReset.reason},
	}

	for _, check := range checks {
		if check.detected && check.conf > maxConfidence {
			maxConfidence = check.conf
			boundaryType = check.btype
			reason = check.reason
		}
	}

	if maxConfidence > 0.6 {
		return &TaskBoundary{
			Index:      index,
			MessageID:  curr.ID,
			Confidence: maxConfidence,
			Reason:     reason,
			Type:       boundaryType,
		}
	}

	return nil
}

type detectionResult struct {
	detected   bool
	confidence float64
	reason     string
}

func (r *TaskRouter) detectTopicShift(prev, curr string) detectionResult {
	prevKeywords := r.extractKeywords(prev)
	currKeywords := r.extractKeywords(curr)

	overlap := r.keywordOverlap(prevKeywords, currKeywords)

	if overlap < 0.3 && len(currKeywords) > 0 {
		return detectionResult{
			detected:   true,
			confidence: 1.0 - overlap,
			reason:     fmt.Sprintf("Topic shift detected (keyword overlap: %.2f)", overlap),
		}
	}

	return detectionResult{}
}

func (r *TaskRouter) detectNewRequest(content string) detectionResult {
	newRequestPatterns := []string{
		"can you", "please", "i need", "i want", "help me", "create", "build", "implement",
		"fix", "debug", "refactor", "add", "remove", "update", "change", "modify",
		"new feature", "new task", "start", "begin",
	}

	contentLower := lowercase(content)
	for _, pattern := range newRequestPatterns {
		if contains(contentLower, pattern) {
			return detectionResult{
				detected:   true,
				confidence: 0.8,
				reason:     fmt.Sprintf("New request pattern detected: %s", pattern),
			}
		}
	}

	return detectionResult{}
}

func (r *TaskRouter) detectCompletion(content string) detectionResult {
	completionPatterns := []string{
		"done", "completed", "finished", "fixed", "resolved", "working",
		"tests pass", "build successful", "deployed", "merged",
	}

	contentLower := lowercase(content)
	for _, pattern := range completionPatterns {
		if contains(contentLower, pattern) {
			return detectionResult{
				detected:   true,
				confidence: 0.7,
				reason:     fmt.Sprintf("Completion pattern detected: %s", pattern),
			}
		}
	}

	return detectionResult{}
}

func (r *TaskRouter) detectInterruption(prev, curr string) detectionResult {
	if len(curr) > len(prev)*3 {
		return detectionResult{
			detected:   true,
			confidence: 0.6,
			reason:     "Significant message length increase suggests interruption",
		}
	}

	interruptionPatterns := []string{
		"wait", "stop", "hold on", "actually", "nevermind", "ignore", "cancel",
	}

	currLower := lowercase(curr)
	for _, pattern := range interruptionPatterns {
		if contains(currLower, pattern) {
			return detectionResult{
				detected:   true,
				confidence: 0.75,
				reason:     fmt.Sprintf("Interruption pattern detected: %s", pattern),
			}
		}
	}

	return detectionResult{}
}

func (r *TaskRouter) detectContextReset(content string) detectionResult {
	resetPatterns := []string{
		"new session", "fresh start", "reset", "clear context", "forget",
		"start over", "from scratch",
	}

	contentLower := lowercase(content)
	for _, pattern := range resetPatterns {
		if contains(contentLower, pattern) {
			return detectionResult{
				detected:   true,
				confidence: 0.9,
				reason:     fmt.Sprintf("Context reset pattern detected: %s", pattern),
			}
		}
	}

	return detectionResult{}
}

func (r *TaskRouter) extractKeywords(content string) []string {
	words := splitWords(lowercase(content))
	keywords := make([]string, 0)

	stopWords := map[string]bool{
		"the": true, "a": true, "an": true, "and": true, "or": true, "but": true,
		"in": true, "on": true, "at": true, "to": true, "for": true, "of": true,
		"with": true, "by": true, "from": true, "is": true, "are": true, "was": true,
		"were": true, "be": true, "been": true, "being": true, "have": true, "has": true,
		"had": true, "do": true, "does": true, "did": true, "will": true, "would": true,
		"could": true, "should": true, "may": true, "might": true, "must": true,
		"can": true, "i": true, "you": true, "we": true, "they": true, "it": true,
		"this": true, "that": true, "these": true, "those": true, "my": true, "your": true,
	}

	for _, word := range words {
		if len(word) > 3 && !stopWords[word] {
			keywords = append(keywords, word)
		}
	}

	return keywords
}

func (r *TaskRouter) keywordOverlap(keywordsA, keywordsB []string) float64 {
	if len(keywordsA) == 0 || len(keywordsB) == 0 {
		return 0.0
	}

	setA := make(map[string]bool)
	for _, k := range keywordsA {
		setA[k] = true
	}

	intersection := 0
	for _, k := range keywordsB {
		if setA[k] {
			intersection++
		}
	}

	union := len(keywordsA) + len(keywordsB) - intersection
	if union == 0 {
		return 0.0
	}

	return float64(intersection) / float64(union)
}

func (r *TaskRouter) getRecentMessages(ctx context.Context, contextData map[string]interface{}) ([]Message, error) {
	if messagesData, ok := contextData["messages"].([]interface{}); ok {
		var messages []Message
		for _, msgData := range messagesData {
			if msgMap, ok := msgData.(map[string]interface{}); ok {
				msg := Message{}
				if idStr, ok := msgMap["id"].(string); ok {
					msg.ID, _ = uuid.Parse(idStr)
				}
				if role, ok := msgMap["role"].(string); ok {
					msg.Role = role
				}
				if content, ok := msgMap["content"].(string); ok {
					msg.Content = content
				}
				if tsStr, ok := msgMap["timestamp"].(string); ok {
					msg.Timestamp, _ = time.Parse(time.RFC3339, tsStr)
				}
				messages = append(messages, msg)
			}
		}
		return messages, nil
	}

	return []Message{}, nil
}

func (r *TaskRouter) getActiveTaskID(contextData map[string]interface{}) *uuid.UUID {
	if taskIDStr, ok := contextData["active_task_id"].(string); ok {
		if id, err := uuid.Parse(taskIDStr); err == nil {
			return &id
		}
	}
	return nil
}

func (r *TaskRouter) getTaskByID(ctx context.Context, taskID uuid.UUID) (*graph.Task, error) {
	node, err := r.store.GetNode(ctx, taskID)
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

func (r *TaskRouter) calculateComplexity(task *graph.Task) float64 {
	complexity := 0.5

	if len(task.Description) > 500 {
		complexity += 0.2
	}
	if len(task.Tags) > 5 {
		complexity += 0.1
	}
	if task.Priority > 5 {
		complexity += 0.2
	}

	if complexity > 1.0 {
		complexity = 1.0
	}

	return complexity
}

func (r *TaskRouter) calculateUrgency(task *graph.Task, userMessage string) float64 {
	urgency := 0.5

	urgentKeywords := []string{"urgent", "asap", "critical", "blocking", "broken", "down", "outage", "emergency"}
	msgLower := lowercase(userMessage)
	for _, kw := range urgentKeywords {
		if contains(msgLower, kw) {
			urgency += 0.15
		}
	}

	if task.Status == "in_progress" {
		urgency += 0.1
	}

	if urgency > 1.0 {
		urgency = 1.0
	}

	return urgency
}

func (r *TaskRouter) estimateComplexity(userMessage string) float64 {
	complexity := 0.3

	complexKeywords := []string{"refactor", "architecture", "design", "system", "distributed", "scale", "performance", "security", "migration"}
	msgLower := lowercase(userMessage)
	for _, kw := range complexKeywords {
		if contains(msgLower, kw) {
			complexity += 0.1
		}
	}

	if len(userMessage) > 200 {
		complexity += 0.1
	}

	if complexity > 1.0 {
		complexity = 1.0
	}

	return complexity
}

func (r *TaskRouter) estimateUrgency(userMessage string) float64 {
	return r.calculateUrgency(nil, userMessage)
}

func (r *TaskRouter) countDependencies(ctx context.Context, taskID uuid.UUID) int {
	edges, err := r.store.GetEdgesFrom(ctx, taskID, graph.EdgeTypeDependsOn)
	if err != nil {
		return 0
	}
	return len(edges)
}

func (r *TaskRouter) determineHandler(relation TaskRelation, priority TaskPriority) string {
	switch relation {
	case TaskRelationFollowUp:
		if priority.Score > 0.7 {
			return "continue_high_priority"
		}
		return "continue_task"
	case TaskRelationRelated:
		return "related_task"
	case TaskRelationNewTask:
		if priority.Score > 0.7 {
			return "new_task_high_priority"
		}
		return "new_task"
	default:
		return "new_task"
	}
}

func (r *TaskRouter) GetContinuityManager() *ContinuityManager {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.continuityMgr
}

func (r *TaskRouter) GetContextResumer() *ContextResumer {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.contextResumer
}

func lowercase(s string) string {
	result := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			result[i] = c + 32
		} else {
			result[i] = c
		}
	}
	return string(result)
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || indexOf(s, substr) >= 0))
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

func splitWords(s string) []string {
	var words []string
	var current []byte

	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') {
			current = append(current, c)
		} else {
			if len(current) > 0 {
				words = append(words, string(current))
				current = nil
			}
		}
	}

	if len(current) > 0 {
		words = append(words, string(current))
	}

	return words
}