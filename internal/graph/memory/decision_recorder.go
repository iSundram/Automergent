package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/iSundram/Automergent/internal/graph"
)

type DecisionRecorder struct {
	store     *graph.Store
	mu        sync.RWMutex
	embedding EmbeddingProvider
}

func NewDecisionRecorder(store *graph.Store, embedding EmbeddingProvider) *DecisionRecorder {
	return &DecisionRecorder{
		store:     store,
		embedding: embedding,
	}
}

func (dr *DecisionRecorder) RecordDecision(ctx context.Context, decision *Decision) error {
	dr.mu.Lock()
	defer dr.mu.Unlock()

	if decision.ID == uuid.Nil {
		decision.ID = uuid.New()
	}
	now := time.Now()
	decision.CreatedAt = now
	decision.UpdatedAt = now

	if decision.Status == "" {
		decision.Status = "recorded"
	}

	if dr.embedding != nil && len(decision.Rationale) > 0 {
		emb, err := dr.embedding.GetEmbedding(ctx, decision.Rationale)
		if err != nil {
			log.Warn().Err(err).Msg("failed to generate embedding for decision rationale")
		} else {
			if decision.Metadata == nil {
				decision.Metadata = json.RawMessage("{}")
			}
			var meta map[string]interface{}
			if err := json.Unmarshal(decision.Metadata, &meta); err == nil {
				meta["embedding"] = emb
				decision.Metadata, _ = json.Marshal(meta)
			}
		}
	}

	node, err := decision.ToNode()
	if err != nil {
		return fmt.Errorf("convert decision to node: %w", err)
	}

	if err := dr.store.CreateNode(ctx, node); err != nil {
		return fmt.Errorf("create decision node: %w", err)
	}

	if decision.ContextBucket != uuid.Nil {
		edge := &graph.Edge{
			ID:        uuid.New(),
			FromID:    decision.ContextBucket,
			ToID:      decision.ID,
			Type:      graph.EdgeTypeContains,
			CreatedAt: now,
		}
		if err := dr.store.CreateEdge(ctx, edge); err != nil {
			log.Warn().Err(err).Msg("failed to link decision to context bucket")
		}
	}

	if decision.TaskID != uuid.Nil {
		edge := &graph.Edge{
			ID:        uuid.New(),
			FromID:    decision.TaskID,
			ToID:      decision.ID,
			Type:      graph.EdgeTypeRelatesTo,
			CreatedAt: now,
		}
		if err := dr.store.CreateEdge(ctx, edge); err != nil {
			log.Warn().Err(err).Msg("failed to link decision to task")
		}
	}

	for _, filePath := range decision.FilePaths {
		fileID := uuid.NewSHA1(uuid.NameSpaceURL, []byte(filePath))
		fileNode := &graph.File{
			ID:   fileID,
			Path: filePath,
		}
		fileGraphNode, _ := fileNode.ToNode()
		_ = dr.store.CreateNode(ctx, fileGraphNode)

		edge := &graph.Edge{
			ID:        uuid.New(),
			FromID:    decision.ID,
			ToID:      fileID,
			Type:      graph.EdgeTypeReferences,
			CreatedAt: now,
		}
		if err := dr.store.CreateEdge(ctx, edge); err != nil {
			log.Warn().Err(err).Str("file", filePath).Msg("failed to link decision to file")
		}
	}

	for _, tool := range decision.ToolsUsed {
		edgeData := map[string]string{"tool": tool}
		edgeDataJSON, _ := json.Marshal(edgeData)
		edge := &graph.Edge{
			ID:        uuid.New(),
			FromID:    decision.ID,
			ToID:      uuid.NewSHA1(uuid.NameSpaceURL, []byte("tool:"+tool)),
			Type:      graph.EdgeTypeReferences,
			Data:      edgeDataJSON,
			CreatedAt: now,
		}
		if err := dr.store.CreateEdge(ctx, edge); err != nil {
			log.Warn().Err(err).Str("tool", tool).Msg("failed to link decision to tool")
		}
	}

	for _, ev := range decision.Evidence {
		edgeData := map[string]interface{}{
			"evidence_id": ev.ID.String(),
			"type":        ev.Type,
			"relevance":   ev.Relevance,
		}
		edgeDataJSON, _ := json.Marshal(edgeData)
		edge := &graph.Edge{
			ID:        uuid.New(),
			FromID:    decision.ID,
			ToID:      ev.ID,
			Type:      graph.EdgeTypeDerivedFrom,
			Data:      edgeDataJSON,
			CreatedAt: now,
		}
		if err := dr.store.CreateEdge(ctx, edge); err != nil {
			log.Warn().Err(err).Msg("failed to link decision to evidence")
		}
	}

	return nil
}

func (dr *DecisionRecorder) GetDecisionsForContext(ctx context.Context, bucketID uuid.UUID) ([]*Decision, error) {
	dr.mu.RLock()
	defer dr.mu.RUnlock()

	edges, err := dr.store.GetEdgesFrom(ctx, bucketID, graph.EdgeTypeContains)
	if err != nil {
		return nil, fmt.Errorf("get edges from context bucket: %w", err)
	}

	var decisions []*Decision
	for _, edge := range edges {
		node, err := dr.store.GetNode(ctx, edge.ToID)
		if err != nil {
			continue
		}
		if node.Type == graph.NodeTypeDecision {
			var d Decision
			if err := node.UnmarshalData(&d); err == nil {
				decisions = append(decisions, &d)
			}
		}
	}

	return decisions, nil
}

func (dr *DecisionRecorder) GetDecisionsForFile(ctx context.Context, filePath string) ([]*Decision, error) {
	dr.mu.RLock()
	defer dr.mu.RUnlock()

	fileID := uuid.NewSHA1(uuid.NameSpaceURL, []byte(filePath))
	edges, err := dr.store.GetEdgesTo(ctx, fileID, graph.EdgeTypeReferences)
	if err != nil {
		return nil, fmt.Errorf("get edges to file: %w", err)
	}

	var decisions []*Decision
	seen := make(map[uuid.UUID]bool)
	for _, edge := range edges {
		if seen[edge.FromID] {
			continue
		}
		node, err := dr.store.GetNode(ctx, edge.FromID)
		if err != nil {
			continue
		}
		if node.Type == graph.NodeTypeDecision {
			var d Decision
			if err := node.UnmarshalData(&d); err == nil {
				decisions = append(decisions, &d)
				seen[edge.FromID] = true
			}
		}
	}

	return decisions, nil
}

func (dr *DecisionRecorder) GetDecisionsForTask(ctx context.Context, taskID uuid.UUID) ([]*Decision, error) {
	dr.mu.RLock()
	defer dr.mu.RUnlock()

	edges, err := dr.store.GetEdgesFrom(ctx, taskID, graph.EdgeTypeRelatesTo)
	if err != nil {
		return nil, fmt.Errorf("get edges from task: %w", err)
	}

	var decisions []*Decision
	for _, edge := range edges {
		node, err := dr.store.GetNode(ctx, edge.ToID)
		if err != nil {
			continue
		}
		if node.Type == graph.NodeTypeDecision {
			var d Decision
			if err := node.UnmarshalData(&d); err == nil {
				decisions = append(decisions, &d)
			}
		}
	}

	return decisions, nil
}

func (dr *DecisionRecorder) GetDecisionReplay(ctx context.Context, filePath string) ([]*DecisionReplay, error) {
	dr.mu.RLock()
	defer dr.mu.RUnlock()

	fileID := uuid.NewSHA1(uuid.NameSpaceURL, []byte(filePath))
	edges, err := dr.store.GetEdgesTo(ctx, fileID, graph.EdgeTypeReferences)
	if err != nil {
		return nil, fmt.Errorf("get edges to file: %w", err)
	}

	var replays []*DecisionReplay
	seen := make(map[uuid.UUID]bool)
	for _, edge := range edges {
		if seen[edge.FromID] {
			continue
		}
		node, err := dr.store.GetNode(ctx, edge.FromID)
		if err != nil {
			continue
		}
		if node.Type == graph.NodeTypeDecision {
			var d Decision
			if err := node.UnmarshalData(&d); err == nil {
				replay := &DecisionReplay{
					Decision:  &d,
					FilePath:  filePath,
					TouchType: "reference",
					Timestamp: edge.CreatedAt,
				}
				replays = append(replays, replay)
				seen[edge.FromID] = true
			}
		}
	}

	return replays, nil
}

func (d *Decision) ToNode() (*graph.Node, error) {
	data, err := json.Marshal(d)
	if err != nil {
		return nil, err
	}
	return &graph.Node{
		ID:        d.ID,
		Type:      graph.NodeTypeDecision,
		Data:      data,
		CreatedAt: d.CreatedAt,
		UpdatedAt: d.UpdatedAt,
	}, nil
}