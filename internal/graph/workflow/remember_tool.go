package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

type RememberTool struct {
	store               StoreInterface
	bucketManager       *ContextBucketManager
	mu                  sync.RWMutex
	confidenceThreshold float64
}

func NewRememberTool(store StoreInterface, bucketManager *ContextBucketManager, confidenceThreshold float64) *RememberTool {
	if confidenceThreshold == 0 {
		confidenceThreshold = 0.7
	}
	return &RememberTool{
		store:               store,
		bucketManager:       bucketManager,
		confidenceThreshold: confidenceThreshold,
	}
}

func (r *RememberTool) InjectMessage(ctx context.Context, toTodoID uuid.UUID, fromAgent, message string) (*InjectedMessage, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	todo, err := r.getTodoUnsafe(ctx, toTodoID)
	if err != nil {
		return nil, err
	}

	bucket, err := r.bucketManager.GetBucket(ctx, todo.BucketID)
	if err != nil {
		return nil, err
	}

	injectedMsg := &InjectedMessage{
		BucketID:  bucket.ID,
		TodoID:    toTodoID,
		FromAgent: fromAgent,
		Message:   message,
		Priority:  1,
		Tags:      []string{},
		Metadata:  json.RawMessage("{}"),
		CreatedAt: time.Now(),
	}

	if err := r.bucketManager.InjectMessage(ctx, bucket.ID, injectedMsg); err != nil {
		return nil, fmt.Errorf("inject message: %w", err)
	}

	return injectedMsg, nil
}

func (r *RememberTool) GetInjectedMessages(ctx context.Context, todoID uuid.UUID) ([]*InjectedMessage, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	todo, err := r.getTodoUnsafe(ctx, todoID)
	if err != nil {
		return nil, err
	}

	bucket, err := r.bucketManager.GetBucket(ctx, todo.BucketID)
	if err != nil {
		return nil, err
	}

	var data map[string]interface{}
	if err := json.Unmarshal(bucket.Data, &data); err != nil {
		return []*InjectedMessage{}, nil
	}

	injectedRaw, ok := data["injected_messages"].([]interface{})
	if !ok {
		return []*InjectedMessage{}, nil
	}

	var messages []*InjectedMessage
	for _, raw := range injectedRaw {
		msgData, err := json.Marshal(raw)
		if err != nil {
			continue
		}
		var msg InjectedMessage
		if err := json.Unmarshal(msgData, &msg); err != nil {
			continue
		}
		messages = append(messages, &msg)
	}

	return messages, nil
}

func (r *RememberTool) PromoteMemory(ctx context.Context, todoID uuid.UUID, confidence float64) (*Memory, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if confidence < r.confidenceThreshold {
		return nil, ErrInsufficientConfidence
	}

	todo, err := r.getTodoUnsafe(ctx, todoID)
	if err != nil {
		return nil, err
	}

	bucket, err := r.bucketManager.GetBucket(ctx, todo.BucketID)
	if err != nil {
		return nil, err
	}

	var bucketData map[string]interface{}
	if err := json.Unmarshal(bucket.Data, &bucketData); err != nil {
		return nil, fmt.Errorf("unmarshal bucket data: %w", err)
	}

	content := fmt.Sprintf("Todo: %s\nDescription: %s\nStatus: %s", todo.Title, todo.Description, todo.Status)

	if injected, ok := bucketData["injected_messages"].([]interface{}); ok && len(injected) > 0 {
		content += "\n\nInjected Messages:"
		for _, msg := range injected {
			if msgMap, ok := msg.(map[string]interface{}); ok {
				if fromAgent, ok := msgMap["from_agent"].(string); ok {
					if message, ok := msgMap["message"].(string); ok {
						content += fmt.Sprintf("\n- [%s] %s", fromAgent, message)
					}
				}
			}
		}
	}

	for key, val := range bucketData {
		if key != "injected_messages" {
			content += fmt.Sprintf("\n%s: %v", key, val)
		}
	}

	memory := &Memory{
		Content:    content,
		Type:       MemoryTypeContext,
		Scope:      MemoryScopeTask,
		Tags:       []string{"promoted", "todo", todo.ID.String()},
		Source:     "remember_tool",
		Confidence: confidence,
		Metadata:   json.RawMessage("{}"),
	}

	node, err := memory.toNode()
	if err != nil {
		return nil, fmt.Errorf("create memory node: %w", err)
	}

	if err := r.store.CreateNode(ctx, node); err != nil {
		return nil, fmt.Errorf("persist memory: %w", err)
	}

	memory.ID = node.ID

	edge, err := NewEdge(node.ID, todo.ID, EdgeTypeDerivedFrom, nil)
	if err != nil {
		return nil, fmt.Errorf("create edge: %w", err)
	}
	if err := r.store.CreateEdge(ctx, edge); err != nil {
		return nil, fmt.Errorf("link memory to todo: %w", err)
	}

	return memory, nil
}

func (r *RememberTool) GetMemoriesForTodo(ctx context.Context, todoID uuid.UUID) ([]*Memory, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	edges, err := r.store.GetEdgesTo(ctx, todoID, EdgeTypeDerivedFrom)
	if err != nil {
		return nil, err
	}

	var memories []*Memory
	for _, edge := range edges {
		node, err := r.store.GetNode(ctx, edge.FromID)
		if err != nil {
			continue
		}
		if node.Type != NodeTypeMemory {
			continue
		}
		var memory Memory
		if err := json.Unmarshal(node.Data, &memory); err != nil {
			continue
		}
		memory.ID = node.ID
		memories = append(memories, &memory)
	}
	return memories, nil
}

func (r *RememberTool) GetProjectMemories(ctx context.Context, taskID uuid.UUID) ([]*Memory, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	nodes, err := r.store.ListNodes(ctx, NodeTypeMemory, 1000, 0)
	if err != nil {
		return nil, err
	}

	var memories []*Memory
	for _, node := range nodes {
		var memory Memory
		if err := json.Unmarshal(node.Data, &memory); err != nil {
			continue
		}
		if memory.Scope == MemoryScopeProject || memory.Scope == MemoryScopeGlobal {
			memory.ID = node.ID
			memories = append(memories, &memory)
		}
	}
	return memories, nil
}

func (r *RememberTool) SetConfidenceThreshold(threshold float64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.confidenceThreshold = threshold
}

func (r *RememberTool) GetConfidenceThreshold() float64 {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.confidenceThreshold
}

func (r *RememberTool) getTodoUnsafe(ctx context.Context, todoID uuid.UUID) (*TodoItem, error) {
	node, err := r.store.GetNode(ctx, todoID)
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

func (m *Memory) toNode() (*Node, error) {
	data, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	return &Node{
		ID:        uuid.New(),
		Type:      NodeTypeMemory,
		Data:      data,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}, nil
}