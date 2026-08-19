package healing

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

var (
	ErrMaintenanceInProgress = errors.New("maintenance already in progress")
)

type GraphMaintenance struct {
	mu              sync.RWMutex
	store           GraphStore
	config          StalenessConfig
	inProgress      bool
	lastRun         time.Time
	lastStats       *CleanupStats
	memoryIndex     map[string][]uuid.UUID
	stalenessScores map[uuid.UUID]float64
}

func NewGraphMaintenance(store GraphStore, config StalenessConfig) *GraphMaintenance {
	return &GraphMaintenance{
		store:           store,
		config:          config,
		memoryIndex:     make(map[string][]uuid.UUID),
		stalenessScores: make(map[uuid.UUID]float64),
	}
}

func (gm *GraphMaintenance) PruneStaleNodes(ctx context.Context, ttl time.Duration) (int64, error) {
	gm.mu.Lock()
	if gm.inProgress {
		gm.mu.Unlock()
		return 0, ErrMaintenanceInProgress
	}
	gm.inProgress = true
	gm.mu.Unlock()

	defer func() {
		gm.mu.Lock()
		gm.inProgress = false
		gm.mu.Unlock()
	}()

	if ttl <= 0 {
		ttl = gm.config.NodeTTL
	}

	cutoff := time.Now().Add(-ttl)
	var removed int64

	nodeTypes := []string{"task", "context_bucket", "decision", "memory", "file", "todo", "agent", "event", "injected_prompt"}

	for _, nodeType := range nodeTypes {
		nodes, err := gm.store.ListNodes(ctx, nodeType, 0, 0)
		if err != nil {
			continue
		}

		for _, node := range nodes {
			n, ok := node.(map[string]interface{})
			if !ok {
				continue
			}

			createdAtStr, _ := n["created_at"].(string)
			createdAt, err := time.Parse(time.RFC3339, createdAtStr)
			if err != nil {
				continue
			}

			if createdAt.Before(cutoff) {
				accessCount := 0
				if ac, ok := n["access_count"].(float64); ok {
					accessCount = int(ac)
				}

				if accessCount < gm.config.MinAccessCount {
					idStr, _ := n["id"].(string)
					id, _ := uuid.Parse(idStr)
					if err := gm.store.DeleteNode(ctx, id); err == nil {
						removed++
						delete(gm.stalenessScores, id)
					}
				}
			}
		}
	}

	return removed, nil
}

func (gm *GraphMaintenance) ConsolidateMemories(ctx context.Context) (int64, error) {
	gm.mu.Lock()
	if gm.inProgress {
		gm.mu.Unlock()
		return 0, ErrMaintenanceInProgress
	}
	gm.inProgress = true
	gm.mu.Unlock()

	defer func() {
		gm.mu.Lock()
		gm.inProgress = false
		gm.mu.Unlock()
	}()

	nodes, err := gm.store.ListNodes(ctx, "memory", 0, 0)
	if err != nil {
		return 0, fmt.Errorf("list memories: %w", err)
	}

	gm.mu.Lock()
	gm.memoryIndex = make(map[string][]uuid.UUID)
	gm.mu.Unlock()

for _, node := range nodes {
			n, ok := node.(map[string]interface{})
			if !ok {
				continue
			}

			content, _ := n["content"].(string)
			idStr, _ := n["id"].(string)
			id, _ := uuid.Parse(idStr)

			gm.memoryIndex[hashContent(content)] = append(gm.memoryIndex[hashContent(content)], id)
		}

	var consolidated int64
	for _, ids := range gm.memoryIndex {
		if len(ids) <= 1 {
			continue
		}

		var bestID uuid.UUID
		var bestScore float64
		var bestNode map[string]interface{}

		for _, id := range ids {
			node, err := gm.store.GetNode(ctx, id)
			if err != nil {
				continue
			}
			n, ok := node.(map[string]interface{})
			if !ok {
				continue
			}

			confidence := 0.0
			if c, ok := n["confidence"].(float64); ok {
				confidence = c
			}

			accessCount := 0
			if ac, ok := n["access_count"].(float64); ok {
				accessCount = int(ac)
			}

			score := confidence + float64(accessCount)*0.1
			if score > bestScore {
				bestScore = score
				bestID = id
				bestNode = n
			}
		}

		for _, id := range ids {
			if id == bestID {
				continue
			}
			if err := gm.store.DeleteNode(ctx, id); err == nil {
				consolidated++
				delete(gm.stalenessScores, id)
			}
		}

		if bestNode != nil {
			bestNode["consolidated_from"] = len(ids)
			bestNode["updated_at"] = time.Now()
			_ = gm.store.UpdateNode(ctx, bestNode)
		}
	}

	return consolidated, nil
}

func (gm *GraphMaintenance) UpdateStalenessScores(ctx context.Context) error {
	gm.mu.Lock()
	if gm.inProgress {
		gm.mu.Unlock()
		return ErrMaintenanceInProgress
	}
	gm.inProgress = true
	gm.mu.Unlock()

	defer func() {
		gm.mu.Lock()
		gm.inProgress = false
		gm.mu.Unlock()
	}()

	nodeTypes := []string{"task", "context_bucket", "decision", "memory", "file", "todo", "agent", "event"}

	for _, nodeType := range nodeTypes {
		nodes, err := gm.store.ListNodes(ctx, nodeType, 0, 0)
		if err != nil {
			continue
		}

		for _, node := range nodes {
			n, ok := node.(map[string]interface{})
			if !ok {
				continue
			}

			idStr, _ := n["id"].(string)
			id, _ := uuid.Parse(idStr)

			createdAtStr, _ := n["created_at"].(string)
			createdAt, _ := time.Parse(time.RFC3339, createdAtStr)

			updatedAtStr, _ := n["updated_at"].(string)
			updatedAt, _ := time.Parse(time.RFC3339, updatedAtStr)

			accessCount := 0
			if ac, ok := n["access_count"].(float64); ok {
				accessCount = int(ac)
			}

			age := time.Since(createdAt)
			timeSinceUpdate := time.Since(updatedAt)

			ageScore := 1.0 - float64(age)/(float64(gm.config.NodeTTL)*2)
			if ageScore < 0 {
				ageScore = 0
			}

			updateScore := 1.0 - float64(timeSinceUpdate)/(float64(gm.config.NodeTTL))
			if updateScore < 0 {
				updateScore = 0
			}

			accessScore := 1.0
			if accessCount == 0 {
				accessScore = 0.1
			} else if accessCount < 5 {
				accessScore = 0.5
			}

			staleness := (ageScore + updateScore + accessScore) / 3.0
			if staleness < 0 {
				staleness = 0
			}
			if staleness > 1 {
				staleness = 1
			}

			gm.mu.Lock()
			gm.stalenessScores[id] = staleness
			gm.mu.Unlock()

			n["staleness_score"] = staleness
			n["updated_at"] = time.Now()
			_ = gm.store.UpdateNode(ctx, n)
		}
	}

	return nil
}

func (gm *GraphMaintenance) GetStalenessScore(id uuid.UUID) float64 {
	gm.mu.RLock()
	defer gm.mu.RUnlock()
	return gm.stalenessScores[id]
}

func (gm *GraphMaintenance) RebuildIndexes(ctx context.Context) (int64, error) {
	gm.mu.Lock()
	if gm.inProgress {
		gm.mu.Unlock()
		return 0, ErrMaintenanceInProgress
	}
	gm.inProgress = true
	gm.mu.Unlock()

	defer func() {
		gm.mu.Lock()
		gm.inProgress = false
		gm.mu.Unlock()
	}()

	var rebuilt int64

	indexQueries := []string{
		"CREATE INDEX IF NOT EXISTS idx_nodes_type ON nodes(type)",
		"CREATE INDEX IF NOT EXISTS idx_nodes_created_at ON nodes(created_at)",
		"CREATE INDEX IF NOT EXISTS idx_nodes_updated_at ON nodes(updated_at)",
		"CREATE INDEX IF NOT EXISTS idx_edges_from ON edges(from_id)",
		"CREATE INDEX IF NOT EXISTS idx_edges_to ON edges(to_id)",
		"CREATE INDEX IF NOT EXISTS idx_edges_type ON edges(type)",
		"CREATE INDEX IF NOT EXISTS idx_node_labels_label ON node_labels(label)",
		"CREATE INDEX IF NOT EXISTS idx_memories_content_hash ON memories(content_hash)",
		"CREATE INDEX IF NOT EXISTS idx_memories_scope ON memories(scope)",
	}

	for _, query := range indexQueries {
		_, err := gm.store.ExecuteQuery(ctx, query)
		if err == nil {
			rebuilt++
		}
	}

	return rebuilt, nil
}

func (gm *GraphMaintenance) CompactGraph(ctx context.Context) error {
	gm.mu.Lock()
	if gm.inProgress {
		gm.mu.Unlock()
		return ErrMaintenanceInProgress
	}
	gm.inProgress = true
	gm.mu.Unlock()

	defer func() {
		gm.mu.Lock()
		gm.inProgress = false
		gm.mu.Unlock()
	}()

	vacuumQueries := []string{
		"VACUUM",
		"ANALYZE",
	}

	for _, query := range vacuumQueries {
		_, err := gm.store.ExecuteQuery(ctx, query)
		if err != nil {
			return fmt.Errorf("compact failed: %w", err)
		}
	}

	return nil
}

func (gm *GraphMaintenance) RunFullMaintenance(ctx context.Context) (*CleanupStats, error) {
	gm.mu.Lock()
	if gm.inProgress {
		gm.mu.Unlock()
		return nil, ErrMaintenanceInProgress
	}
	gm.inProgress = true
	gm.mu.Unlock()

	defer func() {
		gm.mu.Lock()
		gm.inProgress = false
		gm.mu.Unlock()
	}()

	startedAt := time.Now()
	stats := &CleanupStats{
		StartedAt: startedAt,
		Errors:    []string{},
	}

	nodesRemoved, err := gm.PruneStaleNodes(ctx, gm.config.NodeTTL)
	if err != nil {
		stats.Errors = append(stats.Errors, fmt.Sprintf("prune nodes: %v", err))
	}
	stats.NodesRemoved = nodesRemoved

	memoriesConsolidated, err := gm.ConsolidateMemories(ctx)
	if err != nil {
		stats.Errors = append(stats.Errors, fmt.Sprintf("consolidate memories: %v", err))
	}
	stats.MemoriesConsolidated = memoriesConsolidated

	if err := gm.UpdateStalenessScores(ctx); err != nil {
		stats.Errors = append(stats.Errors, fmt.Sprintf("update staleness: %v", err))
	}

	indexesRebuilt, err := gm.RebuildIndexes(ctx)
	if err != nil {
		stats.Errors = append(stats.Errors, fmt.Sprintf("rebuild indexes: %v", err))
	}
	stats.IndexesRebuilt = indexesRebuilt

	if err := gm.CompactGraph(ctx); err != nil {
		stats.Errors = append(stats.Errors, fmt.Sprintf("compact graph: %v", err))
	}
	stats.GraphCompacted = true

	stats.CompletedAt = time.Now()
	stats.Duration = stats.CompletedAt.Sub(startedAt)

	gm.mu.Lock()
	gm.lastRun = stats.CompletedAt
	gm.lastStats = stats
	gm.mu.Unlock()

	return stats, nil
}

func (gm *GraphMaintenance) GetLastStats() *CleanupStats {
	gm.mu.RLock()
	defer gm.mu.RUnlock()
	return gm.lastStats
}

func (gm *GraphMaintenance) GetLastRun() time.Time {
	gm.mu.RLock()
	defer gm.mu.RUnlock()
	return gm.lastRun
}

func (gm *GraphMaintenance) IsRunning() bool {
	gm.mu.RLock()
	defer gm.mu.RUnlock()
	return gm.inProgress
}

func (gm *GraphMaintenance) GetStaleNodes(ctx context.Context, threshold float64) ([]uuid.UUID, error) {
	gm.mu.RLock()
	defer gm.mu.RUnlock()

	var stale []uuid.UUID
	for id, score := range gm.stalenessScores {
		if score >= threshold {
			stale = append(stale, id)
		}
	}
	return stale, nil
}