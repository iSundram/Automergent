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

type UndoManager struct {
	store *graph.Store
	mu    sync.RWMutex
}

func NewUndoManager(store *graph.Store) *UndoManager {
	return &UndoManager{
		store: store,
	}
}

func (m *UndoManager) UndoEdit(ctx context.Context, editID uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	tx, err := m.store.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	action, err := m.getUndoAction(ctx, tx, editID)
	if err != nil {
		return fmt.Errorf("get undo action: %w", err)
	}

	if !action.CanRevert {
		return fmt.Errorf("edit cannot be reverted")
	}

	if err := m.revertEdit(ctx, tx, action); err != nil {
		return fmt.Errorf("revert edit: %w", err)
	}

	now := time.Now()
	action.RevertedAt = &now
	action.CanRevert = false

	if err := m.updateUndoAction(ctx, tx, action); err != nil {
		return fmt.Errorf("update undo action: %w", err)
	}

	return tx.Commit()
}

func (m *UndoManager) UndoToDecision(ctx context.Context, decisionID uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	tx, err := m.store.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	actions, err := m.getUndoActionsAfterDecision(ctx, tx, decisionID)
	if err != nil {
		return fmt.Errorf("get undo actions: %w", err)
	}

	for i := len(actions) - 1; i >= 0; i-- {
		action := actions[i]
		if !action.CanRevert {
			continue
		}
		if err := m.revertEdit(ctx, tx, action); err != nil {
			return fmt.Errorf("revert action %s: %w", action.ID, err)
		}
		now := time.Now()
		action.RevertedAt = &now
		action.CanRevert = false
		if err := m.updateUndoAction(ctx, tx, action); err != nil {
			return fmt.Errorf("update undo action: %w", err)
		}
	}

	return tx.Commit()
}

func (m *UndoManager) UndoTodo(ctx context.Context, todoID uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	tx, err := m.store.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	actions, err := m.getUndoActionsForTodo(ctx, tx, todoID)
	if err != nil {
		return fmt.Errorf("get undo actions for todo: %w", err)
	}

	for i := len(actions) - 1; i >= 0; i-- {
		action := actions[i]
		if !action.CanRevert {
			continue
		}
		if err := m.revertEdit(ctx, tx, action); err != nil {
			return fmt.Errorf("revert action %s: %w", action.ID, err)
		}
		now := time.Now()
		action.RevertedAt = &now
		action.CanRevert = false
		if err := m.updateUndoAction(ctx, tx, action); err != nil {
			return fmt.Errorf("update undo action: %w", err)
		}
	}

	return tx.Commit()
}

func (m *UndoManager) GetUndoHistory(ctx context.Context, taskID uuid.UUID) ([]*UndoAction, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	edges, err := m.store.GetEdgesFrom(ctx, taskID, graph.EdgeTypeContains)
	if err != nil {
		return nil, fmt.Errorf("get task edges: %w", err)
	}

	var actions []*UndoAction
	for _, edge := range edges {
		node, err := m.store.GetNode(ctx, edge.ToID)
		if err != nil || node.Type != graph.NodeTypeEvent {
			continue
		}

		var event graph.Event
		if err := node.UnmarshalData(&event); err != nil {
			continue
		}

		if event.Type == "undo_action" {
			var action UndoAction
			if err := json.Unmarshal(event.Payload, &action); err != nil {
				continue
			}
			action.ID = node.ID
			actions = append(actions, &action)
		}
	}

	return actions, nil
}

func (m *UndoManager) CanUndo(ctx context.Context, editID uuid.UUID) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	action, err := m.getUndoAction(ctx, nil, editID)
	if err != nil {
		return false, err
	}

	return action.CanRevert, nil
}

func (m *UndoManager) RecordEdit(ctx context.Context, taskID uuid.UUID, targetID uuid.UUID, actionType UndoActionType, scope UndoScope, description string, previousData, currentData interface{}) (*UndoAction, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	prevJSON, _ := json.Marshal(previousData)
	currJSON, _ := json.Marshal(currentData)

	action := &UndoAction{
		ID:           uuid.New(),
		Type:         actionType,
		Scope:        scope,
		TargetID:     targetID,
		TaskID:       taskID,
		Description:  description,
		PreviousData: prevJSON,
		CurrentData:  currJSON,
		CreatedAt:    time.Now(),
		CanRevert:    true,
	}

	event := &graph.Event{
		ID:        action.ID,
		Type:      "undo_action",
		Source:    "undo_manager",
		Target:    taskID.String(),
		Payload:   json.RawMessage("{}"),
		Timestamp: time.Now(),
	}

	eventData, _ := json.Marshal(action)
	event.Payload = eventData

	eventNode, err := event.ToNode()
	if err != nil {
		return nil, fmt.Errorf("create event node: %w", err)
	}

	if err := m.store.CreateNode(ctx, eventNode); err != nil {
		return nil, fmt.Errorf("persist event: %w", err)
	}

	edge, err := graph.NewEdge(taskID, action.ID, graph.EdgeTypeContains, nil)
	if err != nil {
		return nil, fmt.Errorf("create edge: %w", err)
	}

	if err := m.store.CreateEdge(ctx, edge); err != nil {
		return nil, fmt.Errorf("link event to task: %w", err)
	}

	return action, nil
}

func (m *UndoManager) RecordDecision(ctx context.Context, taskID uuid.UUID, decision *graph.Decision) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	tx, err := m.store.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	decNode, err := decision.ToNode()
	if err != nil {
		return fmt.Errorf("create decision node: %w", err)
	}

	if err := tx.CreateNode(ctx, decNode); err != nil {
		return fmt.Errorf("persist decision: %w", err)
	}

	edge, err := graph.NewEdge(taskID, decNode.ID, graph.EdgeTypeReferences, nil)
	if err != nil {
		return fmt.Errorf("create edge: %w", err)
	}

	if err := tx.CreateEdge(ctx, edge); err != nil {
		return fmt.Errorf("create edge: %w", err)
	}

	return tx.Commit()
}

func (m *UndoManager) getUndoAction(ctx context.Context, tx *graph.Tx, editID uuid.UUID) (*UndoAction, error) {
	var node *graph.Node
	var err error

	if tx != nil {
		node, err = tx.GetNode(ctx, editID)
	} else {
		node, err = m.store.GetNode(ctx, editID)
	}

	if err != nil {
		return nil, err
	}

	if node.Type != graph.NodeTypeEvent {
		return nil, fmt.Errorf("not an undo action event")
	}

	var event graph.Event
	if err := node.UnmarshalData(&event); err != nil {
		return nil, err
	}

	var action UndoAction
	if err := json.Unmarshal(event.Payload, &action); err != nil {
		return nil, err
	}

	action.ID = node.ID
	return &action, nil
}

func (m *UndoManager) updateUndoAction(ctx context.Context, tx *graph.Tx, action *UndoAction) error {
	event := &graph.Event{
		ID:        action.ID,
		Type:      "undo_action",
		Source:    "undo_manager",
		Target:    action.TaskID.String(),
		Payload:   json.RawMessage("{}"),
		Timestamp: time.Now(),
	}

	eventData, _ := json.Marshal(action)
	event.Payload = eventData

	eventNode, err := event.ToNode()
	if err != nil {
		return fmt.Errorf("create event node: %w", err)
	}

	if tx != nil {
		return tx.UpdateNode(ctx, eventNode)
	}
	return m.store.UpdateNode(ctx, eventNode)
}

func (m *UndoManager) revertEdit(ctx context.Context, tx *graph.Tx, action *UndoAction) error {
	switch action.Type {
	case UndoActionTypeEdit:
		return m.revertFileEdit(ctx, tx, action)
	case UndoActionTypeDecision:
		return m.revertDecision(ctx, tx, action)
	case UndoActionTypeTodo:
		return m.revertTodo(ctx, tx, action)
	case UndoActionTypeFile:
		return m.revertFileOperation(ctx, tx, action)
	case UndoActionTypeBucket:
		return m.revertBucketChange(ctx, tx, action)
	case UndoActionTypeMemory:
		return m.revertMemoryChange(ctx, tx, action)
	default:
		return fmt.Errorf("unknown undo action type: %s", action.Type)
	}
}

func (m *UndoManager) revertFileEdit(ctx context.Context, tx *graph.Tx, action *UndoAction) error {
	var file graph.File
	if err := json.Unmarshal(action.PreviousData, &file); err != nil {
		return fmt.Errorf("unmarshal previous file data: %w", err)
	}

	fileNode, err := file.ToNode()
	if err != nil {
		return fmt.Errorf("create file node: %w", err)
	}
	fileNode.ID = action.TargetID

	if tx != nil {
		return tx.UpdateNode(ctx, fileNode)
	}
	return m.store.UpdateNode(ctx, fileNode)
}

func (m *UndoManager) revertDecision(ctx context.Context, tx *graph.Tx, action *UndoAction) error {
	var decision graph.Decision
	if err := json.Unmarshal(action.PreviousData, &decision); err != nil {
		return fmt.Errorf("unmarshal previous decision data: %w", err)
	}

	decNode, err := decision.ToNode()
	if err != nil {
		return fmt.Errorf("create decision node: %w", err)
	}
	decNode.ID = action.TargetID

	if tx != nil {
		return tx.UpdateNode(ctx, decNode)
	}
	return m.store.UpdateNode(ctx, decNode)
}

func (m *UndoManager) revertTodo(ctx context.Context, tx *graph.Tx, action *UndoAction) error {
	var todo graph.Todo
	if err := json.Unmarshal(action.PreviousData, &todo); err != nil {
		return fmt.Errorf("unmarshal previous todo data: %w", err)
	}

	todoNode, err := todo.ToNode()
	if err != nil {
		return fmt.Errorf("create todo node: %w", err)
	}
	todoNode.ID = action.TargetID

	if tx != nil {
		return tx.UpdateNode(ctx, todoNode)
	}
	return m.store.UpdateNode(ctx, todoNode)
}

func (m *UndoManager) revertFileOperation(ctx context.Context, tx *graph.Tx, action *UndoAction) error {
	var file graph.File
	if err := json.Unmarshal(action.PreviousData, &file); err != nil {
		return fmt.Errorf("unmarshal previous file data: %w", err)
	}

	fileNode, err := file.ToNode()
	if err != nil {
		return fmt.Errorf("create file node: %w", err)
	}
	fileNode.ID = action.TargetID

	if tx != nil {
		return tx.UpdateNode(ctx, fileNode)
	}
	return m.store.UpdateNode(ctx, fileNode)
}

func (m *UndoManager) revertBucketChange(ctx context.Context, tx *graph.Tx, action *UndoAction) error {
	var bucket graph.ContextBucket
	if err := json.Unmarshal(action.PreviousData, &bucket); err != nil {
		return fmt.Errorf("unmarshal previous bucket data: %w", err)
	}

	bucketNode, err := bucket.ToNode()
	if err != nil {
		return fmt.Errorf("create bucket node: %w", err)
	}
	bucketNode.ID = action.TargetID

	if tx != nil {
		return tx.UpdateNode(ctx, bucketNode)
	}
	return m.store.UpdateNode(ctx, bucketNode)
}

func (m *UndoManager) revertMemoryChange(ctx context.Context, tx *graph.Tx, action *UndoAction) error {
	var memory graph.Memory
	if err := json.Unmarshal(action.PreviousData, &memory); err != nil {
		return fmt.Errorf("unmarshal previous memory data: %w", err)
	}

	memNode, err := memory.ToNode()
	if err != nil {
		return fmt.Errorf("create memory node: %w", err)
	}
	memNode.ID = action.TargetID

	if tx != nil {
		return tx.UpdateNode(ctx, memNode)
	}
	return m.store.UpdateNode(ctx, memNode)
}

func (m *UndoManager) getUndoActionsAfterDecision(ctx context.Context, tx *graph.Tx, decisionID uuid.UUID) ([]*UndoAction, error) {
	taskEdges, err := tx.GetEdgesTo(ctx, decisionID, graph.EdgeTypeReferences)
	if err != nil {
		return nil, err
	}

	if len(taskEdges) == 0 {
		return nil, fmt.Errorf("decision not linked to task")
	}

	taskID := taskEdges[0].FromID
	return m.getUndoActionsForTask(ctx, tx, taskID, decisionID)
}

func (m *UndoManager) getUndoActionsForTodo(ctx context.Context, tx *graph.Tx, todoID uuid.UUID) ([]*UndoAction, error) {
	taskEdges, err := tx.GetEdgesTo(ctx, todoID, graph.EdgeTypeContains)
	if err != nil {
		return nil, err
	}

	if len(taskEdges) == 0 {
		return nil, fmt.Errorf("todo not linked to task")
	}

	taskID := taskEdges[0].FromID
	return m.getUndoActionsForTask(ctx, tx, taskID, uuid.Nil)
}

func (m *UndoManager) getUndoActionsForTask(ctx context.Context, tx *graph.Tx, taskID uuid.UUID, afterDecisionID uuid.UUID) ([]*UndoAction, error) {
	edges, err := tx.GetEdgesFrom(ctx, taskID, graph.EdgeTypeContains)
	if err != nil {
		return nil, err
	}

	var actions []*UndoAction
	foundDecision := afterDecisionID == uuid.Nil

	for _, edge := range edges {
		node, err := tx.GetNode(ctx, edge.ToID)
		if err != nil || node.Type != graph.NodeTypeEvent {
			continue
		}

		var event graph.Event
		if err := node.UnmarshalData(&event); err != nil {
			continue
		}

		if event.Type == "undo_action" {
			if !foundDecision && event.ID == afterDecisionID {
				foundDecision = true
				continue
			}
			if foundDecision {
				var action UndoAction
				if err := json.Unmarshal(event.Payload, &action); err != nil {
					continue
				}
				action.ID = node.ID
				actions = append(actions, &action)
			}
		}
	}

	return actions, nil
}