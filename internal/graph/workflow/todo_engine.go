package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

type TodoWorkflowEngine struct {
	store         StoreInterface
	bucketManager *ContextBucketManager
	mu            sync.RWMutex
}

func NewTodoWorkflowEngine(store StoreInterface, bucketManager *ContextBucketManager) *TodoWorkflowEngine {
	return &TodoWorkflowEngine{
		store:         store,
		bucketManager: bucketManager,
	}
}

func (e *TodoWorkflowEngine) CreateWorkflow(ctx context.Context, taskID uuid.UUID, category, title, description string) (*TodoWorkflow, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	workflow := &TodoWorkflow{
		TaskID:      taskID,
		Category:    category,
		Title:       title,
		Description: description,
		Status:      "active",
		Metadata:    json.RawMessage("{}"),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	node, err := workflow.toNode()
	if err != nil {
		return nil, fmt.Errorf("create workflow node: %w", err)
	}

	if err := e.store.CreateNode(ctx, node); err != nil {
		return nil, fmt.Errorf("persist workflow: %w", err)
	}

	workflow.ID = node.ID

	bucket, err := e.bucketManager.CreateBucket(ctx, taskID, ContextBucketTypeWorkflow, title, description, "system", SharePolicyFull)
	if err != nil {
		return nil, fmt.Errorf("create workflow bucket: %w", err)
	}

	edge, err := NewEdge(workflow.ID, bucket.ID, EdgeTypeContains, nil)
	if err != nil {
		return nil, fmt.Errorf("create edge: %w", err)
	}
	if err := e.store.CreateEdge(ctx, edge); err != nil {
		return nil, fmt.Errorf("link workflow to bucket: %w", err)
	}
	parentEdge, err := NewEdge(taskID, workflow.ID, EdgeTypeParentOf, nil)
	if err != nil {
		return nil, fmt.Errorf("create task workflow edge: %w", err)
	}
	if err := e.store.CreateEdge(ctx, parentEdge); err != nil {
		return nil, fmt.Errorf("link task to workflow: %w", err)
	}

	return workflow, nil
}

func (e *TodoWorkflowEngine) GetWorkflow(ctx context.Context, workflowID uuid.UUID) (*TodoWorkflow, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	node, err := e.store.GetNode(ctx, workflowID)
	if err != nil {
		return nil, err
	}

	if node.Type != NodeTypeTodo {
		return nil, ErrWorkflowNotFound
	}

	var workflow TodoWorkflow
	if err := json.Unmarshal(node.Data, &workflow); err != nil {
		return nil, fmt.Errorf("unmarshal workflow: %w", err)
	}
	workflow.ID = node.ID
	return &workflow, nil
}

func (e *TodoWorkflowEngine) AddTodo(ctx context.Context, workflowID uuid.UUID, title, description string, dependencies []uuid.UUID, sharePolicy SharePolicy, shareKeys []string) (*TodoItem, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	workflow, err := e.getWorkflowUnsafe(ctx, workflowID)
	if err != nil {
		return nil, err
	}

	for _, depID := range dependencies {
		if err := e.validateDependency(ctx, depID); err != nil {
			return nil, fmt.Errorf("invalid dependency %s: %w", depID, err)
		}
	}

	if err := e.checkDependencyCycle(ctx, workflowID, dependencies); err != nil {
		return nil, err
	}

	todo := &TodoItem{
		WorkflowID:   workflowID,
		Title:        title,
		Description:  description,
		Status:       TodoStatusPending,
		Priority:     len(dependencies),
		Dependencies: dependencies,
		SharePolicy:  sharePolicy,
		ShareKeys:    shareKeys,
		Metadata:     json.RawMessage("{}"),
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	node, err := todo.toNode()
	if err != nil {
		return nil, fmt.Errorf("create todo node: %w", err)
	}

	if err := e.store.CreateNode(ctx, node); err != nil {
		return nil, fmt.Errorf("persist todo: %w", err)
	}

	todo.ID = node.ID

	bucket, err := e.bucketManager.CreateBucketForTodo(ctx, todo.ID, title, description, "system", sharePolicy)
	if err != nil {
		return nil, fmt.Errorf("create todo bucket: %w", err)
	}

	todo.BucketID = bucket.ID
	todoNode, err := todo.toNode()
	if err != nil {
		return nil, fmt.Errorf("update todo node: %w", err)
	}
	if err := e.store.UpdateNode(ctx, todoNode); err != nil {
		return nil, fmt.Errorf("update todo with bucket: %w", err)
	}

	for _, depID := range dependencies {
		edge, err := NewEdge(todo.ID, depID, EdgeTypeDependsOn, nil)
		if err != nil {
			return nil, fmt.Errorf("create dependency edge: %w", err)
		}
		if err := e.store.CreateEdge(ctx, edge); err != nil {
			return nil, fmt.Errorf("link dependency: %w", err)
		}
	}

	if workflow.CurrentTodo == uuid.Nil {
		workflow.CurrentTodo = todo.ID
		workflow.UpdatedAt = time.Now()
		wNode, err := workflow.toNode()
		if err != nil {
			return nil, fmt.Errorf("update workflow node: %w", err)
		}
		if err := e.store.UpdateNode(ctx, wNode); err != nil {
			return nil, fmt.Errorf("update workflow current todo: %w", err)
		}
	}

	return todo, nil
}

func (e *TodoWorkflowEngine) GetNextTodo(ctx context.Context, workflowID uuid.UUID) (*TodoItem, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	workflow, err := e.getWorkflowUnsafe(ctx, workflowID)
	if err != nil {
		return nil, err
	}

	if workflow.CurrentTodo != uuid.Nil {
		current, err := e.getTodoUnsafe(ctx, workflow.CurrentTodo)
		if err == nil && current.Status != TodoStatusDone && current.Status != TodoStatusSkipped {
			return current, nil
		}
	}

	todos, err := e.getTodosForWorkflowUnsafe(ctx, workflowID)
	if err != nil {
		return nil, err
	}

	for _, todo := range todos {
		if todo.Status == TodoStatusPending || todo.Status == TodoStatusBlocked {
			if e.areDependenciesMetUnsafe(ctx, todo) {
				return todo, nil
			}
		}
	}

	return nil, nil
}

func (e *TodoWorkflowEngine) MarkTodoStatus(ctx context.Context, todoID uuid.UUID, status TodoStatus) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	todo, err := e.getTodoUnsafe(ctx, todoID)
	if err != nil {
		return err
	}

	if !isValidTransition(todo.Status, status) {
		return ErrInvalidStatus
	}

	now := time.Now()
	todo.Status = status
	todo.UpdatedAt = now

	switch status {
	case TodoStatusInProgress:
		todo.StartedAt = &now
	case TodoStatusDone, TodoStatusSkipped:
		todo.CompletedAt = &now
	}

	node, err := todo.toNode()
	if err != nil {
		return fmt.Errorf("create todo node: %w", err)
	}

	if err := e.store.UpdateNode(ctx, node); err != nil {
		return fmt.Errorf("update todo: %w", err)
	}

	if status == TodoStatusDone || status == TodoStatusSkipped {
		return e.advanceWorkflowUnsafe(ctx, todo.WorkflowID)
	}

	return nil
}

func (e *TodoWorkflowEngine) AssignTodo(ctx context.Context, todoID uuid.UUID, assignee string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	todo, err := e.getTodoUnsafe(ctx, todoID)
	if err != nil {
		return err
	}
	todo.Assignee = assignee
	todo.UpdatedAt = time.Now()
	node, err := todo.toNode()
	if err != nil {
		return err
	}
	if err := e.store.UpdateNode(ctx, node); err != nil {
		return err
	}
	if todo.BucketID != uuid.Nil {
		return e.bucketManager.SetBucketOwner(ctx, todo.BucketID, assignee)
	}
	return nil
}

func (e *TodoWorkflowEngine) GetSharableContext(ctx context.Context, todoID uuid.UUID) (map[string]interface{}, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	todo, err := e.getTodoUnsafe(ctx, todoID)
	if err != nil {
		return nil, err
	}

	bucket, err := e.bucketManager.GetBucket(ctx, todo.BucketID)
	if err != nil {
		return nil, err
	}

	var data map[string]interface{}
	if err := json.Unmarshal(bucket.Data, &data); err != nil {
		return make(map[string]interface{}), nil
	}

	result := make(map[string]interface{})

	switch todo.SharePolicy {
	case SharePolicyFull:
		for k, v := range data {
			result[k] = v
		}
	case SharePolicySummary:
		result["summary"] = bucket.Description
		result["bucket_id"] = bucket.ID.String()
		result["todo_id"] = todo.ID.String()
		result["title"] = todo.Title
		result["status"] = todo.Status
	case SharePolicyPartial:
		for _, key := range todo.ShareKeys {
			if val, ok := data[key]; ok {
				result[key] = val
			}
		}
	case SharePolicyInjected:
		if injected, ok := data["injected_messages"].([]interface{}); ok {
			result["injected_messages"] = injected
		}
	case SharePolicyNone:
		return make(map[string]interface{}), nil
	}

	return result, nil
}

func (e *TodoWorkflowEngine) AdvanceWorkflow(ctx context.Context, workflowID uuid.UUID) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.advanceWorkflowUnsafe(ctx, workflowID)
}

func (e *TodoWorkflowEngine) advanceWorkflowUnsafe(ctx context.Context, workflowID uuid.UUID) error {
	workflow, err := e.getWorkflowUnsafe(ctx, workflowID)
	if err != nil {
		return err
	}

	nextTodo, err := e.getNextTodoUnsafe(ctx, workflowID)
	if err != nil {
		return err
	}

	if nextTodo == nil {
		workflow.Status = "completed"
		workflow.CurrentTodo = uuid.Nil
		workflow.UpdatedAt = time.Now()
		node, err := workflow.toNode()
		if err != nil {
			return fmt.Errorf("create workflow node: %w", err)
		}
		return e.store.UpdateNode(ctx, node)
	}

	workflow.CurrentTodo = nextTodo.ID
	workflow.UpdatedAt = time.Now()
	node, err := workflow.toNode()
	if err != nil {
		return fmt.Errorf("create workflow node: %w", err)
	}
	return e.store.UpdateNode(ctx, node)
}

func (e *TodoWorkflowEngine) getNextTodoUnsafe(ctx context.Context, workflowID uuid.UUID) (*TodoItem, error) {
	todos, err := e.getTodosForWorkflowUnsafe(ctx, workflowID)
	if err != nil {
		return nil, err
	}

	for _, todo := range todos {
		if todo.Status == TodoStatusPending || todo.Status == TodoStatusBlocked {
			if e.areDependenciesMetUnsafe(ctx, todo) {
				return todo, nil
			}
		}
	}
	return nil, nil
}

func (e *TodoWorkflowEngine) areDependenciesMetUnsafe(ctx context.Context, todo *TodoItem) bool {
	for _, depID := range todo.Dependencies {
		dep, err := e.getTodoUnsafe(ctx, depID)
		if err != nil || dep.Status != TodoStatusDone {
			return false
		}
	}
	return true
}

func (e *TodoWorkflowEngine) validateDependency(ctx context.Context, depID uuid.UUID) error {
	_, err := e.getTodoUnsafe(ctx, depID)
	return err
}

func (e *TodoWorkflowEngine) checkDependencyCycle(ctx context.Context, workflowID uuid.UUID, newDeps []uuid.UUID) error {
	visited := make(map[uuid.UUID]bool)
	var checkCycle func(todoID uuid.UUID) error
	checkCycle = func(todoID uuid.UUID) error {
		if visited[todoID] {
			return ErrDependencyCycle
		}
		visited[todoID] = true

		todo, err := e.getTodoUnsafe(ctx, todoID)
		if err != nil {
			return nil
		}

		for _, depID := range todo.Dependencies {
			for _, newDep := range newDeps {
				if depID == newDep {
					return ErrDependencyCycle
				}
			}
			if err := checkCycle(depID); err != nil {
				return err
			}
		}
		return nil
	}

	for _, depID := range newDeps {
		if err := checkCycle(depID); err != nil {
			return err
		}
	}
	return nil
}

func (e *TodoWorkflowEngine) getWorkflowUnsafe(ctx context.Context, workflowID uuid.UUID) (*TodoWorkflow, error) {
	node, err := e.store.GetNode(ctx, workflowID)
	if err != nil {
		return nil, err
	}
	if node.Type != NodeTypeTodo {
		return nil, ErrWorkflowNotFound
	}
	var workflow TodoWorkflow
	if err := json.Unmarshal(node.Data, &workflow); err != nil {
		return nil, fmt.Errorf("unmarshal workflow: %w", err)
	}
	workflow.ID = node.ID
	return &workflow, nil
}

func (e *TodoWorkflowEngine) getTodoUnsafe(ctx context.Context, todoID uuid.UUID) (*TodoItem, error) {
	node, err := e.store.GetNode(ctx, todoID)
	if err != nil {
		return nil, err
	}
	if node.Type != NodeTypeTodo {
		return nil, ErrTodoNotFound
	}
	var todo TodoItem
	if err := json.Unmarshal(node.Data, &todo); err != nil {
		return nil, fmt.Errorf("unmarshal todo: %w", err)
	}
	todo.ID = node.ID
	return &todo, nil
}

func (e *TodoWorkflowEngine) getTodosForWorkflowUnsafe(ctx context.Context, workflowID uuid.UUID) ([]*TodoItem, error) {
	nodes, err := e.store.ListNodes(ctx, NodeTypeTodo, 1000, 0)
	if err != nil {
		return nil, err
	}

	var todos []*TodoItem
	for _, node := range nodes {
		var item TodoItem
		if err := json.Unmarshal(node.Data, &item); err != nil {
			continue
		}
		if item.WorkflowID == workflowID {
			item.ID = node.ID
			todos = append(todos, &item)
		}
	}
	return todos, nil
}

func (e *TodoWorkflowEngine) GetWorkflowSummary(ctx context.Context, workflowID uuid.UUID) (*WorkflowSummary, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	workflow, err := e.getWorkflowUnsafe(ctx, workflowID)
	if err != nil {
		return nil, err
	}

	todos, err := e.getTodosForWorkflowUnsafe(ctx, workflowID)
	if err != nil {
		return nil, err
	}

	completed := 0
	var current *TodoItem
	for _, todo := range todos {
		if todo.Status == TodoStatusDone {
			completed++
		}
		if todo.ID == workflow.CurrentTodo {
			current = todo
		}
	}

	progress := 0.0
	if len(todos) > 0 {
		progress = float64(completed) / float64(len(todos))
	}

	return &WorkflowSummary{
		WorkflowID:     workflow.ID,
		TaskID:         workflow.TaskID,
		Title:          workflow.Title,
		Category:       workflow.Category,
		Status:         workflow.Status,
		TotalTodos:     len(todos),
		CompletedTodos: completed,
		CurrentTodo:    current,
		Progress:       progress,
		CreatedAt:      workflow.CreatedAt,
		UpdatedAt:      workflow.UpdatedAt,
	}, nil
}

func (e *TodoWorkflowEngine) ListWorkflows(ctx context.Context, taskID uuid.UUID) ([]*TodoWorkflow, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	nodes, err := e.store.ListNodes(ctx, NodeTypeTodo, 1000, 0)
	if err != nil {
		return nil, err
	}

	var workflows []*TodoWorkflow
	for _, node := range nodes {
		var wf TodoWorkflow
		if err := json.Unmarshal(node.Data, &wf); err != nil {
			continue
		}
		if wf.TaskID == taskID {
			wf.ID = node.ID
			workflows = append(workflows, &wf)
		}
	}
	return workflows, nil
}

func isValidTransition(from, to TodoStatus) bool {
	valid := map[TodoStatus][]TodoStatus{
		TodoStatusPending:    {TodoStatusInProgress, TodoStatusSkipped, TodoStatusBlocked},
		TodoStatusInProgress: {TodoStatusDone, TodoStatusBlocked, TodoStatusPending},
		TodoStatusBlocked:    {TodoStatusPending, TodoStatusInProgress, TodoStatusSkipped},
		TodoStatusDone:       {},
		TodoStatusSkipped:    {},
	}
	for _, s := range valid[from] {
		if s == to {
			return true
		}
	}
	return false
}

func (w *TodoWorkflow) toNode() (*Node, error) {
	data, err := json.Marshal(w)
	if err != nil {
		return nil, err
	}
	id := w.ID
	if id == uuid.Nil {
		id = uuid.New()
	}
	return &Node{
		ID:        id,
		Type:      NodeTypeTodo,
		Data:      data,
		CreatedAt: w.CreatedAt,
		UpdatedAt: w.UpdatedAt,
	}, nil
}

func (t *TodoItem) toNode() (*Node, error) {
	data, err := json.Marshal(t)
	if err != nil {
		return nil, err
	}
	id := t.ID
	if id == uuid.Nil {
		id = uuid.New()
	}
	return &Node{
		ID:        id,
		Type:      NodeTypeTodo,
		Data:      data,
		CreatedAt: t.CreatedAt,
		UpdatedAt: t.UpdatedAt,
	}, nil
}
