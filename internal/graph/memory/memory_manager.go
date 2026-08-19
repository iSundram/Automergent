package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/iSundram/Automergent/internal/graph"
)

type MemoryManager struct {
	store              *graph.Store
	mu                 sync.RWMutex
	embeddingProvider  EmbeddingProvider
	dedupThreshold     float64
	promotionThreshold float64
}

func NewMemoryManager(store *graph.Store, embedding EmbeddingProvider) *MemoryManager {
	return &MemoryManager{
		store:              store,
		embeddingProvider:  embedding,
		dedupThreshold:     DefaultDeduplicationThreshold,
		promotionThreshold: DefaultPromotionThreshold,
	}
}

func (mm *MemoryManager) SetDeduplicationThreshold(threshold float64) {
	mm.mu.Lock()
	defer mm.mu.Unlock()
	mm.dedupThreshold = threshold
}

func (mm *MemoryManager) SetPromotionThreshold(threshold float64) {
	mm.mu.Lock()
	defer mm.mu.Unlock()
	mm.promotionThreshold = threshold
}

func (mm *MemoryManager) CreateMemory(ctx context.Context, memory *Memory) error {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	if memory.ID == uuid.Nil {
		memory.ID = uuid.New()
	}
	now := time.Now()
	memory.CreatedAt = now
	memory.UpdatedAt = now
	memory.AccessCount = 0

	if mm.embeddingProvider != nil && len(memory.Content) > 0 {
		emb, err := mm.embeddingProvider.GetEmbedding(ctx, memory.Content)
		if err != nil {
			log.Warn().Err(err).Msg("failed to generate embedding for memory")
		} else {
			memory.Embedding = emb
		}
	}

	similar, err := mm.findSimilarMemories(ctx, memory)
	if err != nil {
		log.Warn().Err(err).Msg("failed to check for similar memories")
	} else if len(similar) > 0 {
		best := similar[0]
		if best.Similarity >= mm.dedupThreshold {
			return mm.mergeMemories(ctx, memory, best.MemoryID, best.Similarity)
		}
	}

	node, err := memory.ToNode()
	if err != nil {
		return fmt.Errorf("convert memory to node: %w", err)
	}

	if err := mm.store.CreateNode(ctx, node); err != nil {
		return fmt.Errorf("create memory node: %w", err)
	}

	for _, bucketID := range memory.ContextBuckets {
		edge := &graph.Edge{
			ID:        uuid.New(),
			FromID:    bucketID,
			ToID:      memory.ID,
			Type:      graph.EdgeTypeContains,
			CreatedAt: now,
		}
		if err := mm.store.CreateEdge(ctx, edge); err != nil {
			log.Warn().Err(err).Msg("failed to link memory to context bucket")
		}
	}

	for _, taskID := range memory.TaskIDs {
		edge := &graph.Edge{
			ID:        uuid.New(),
			FromID:    taskID,
			ToID:      memory.ID,
			Type:      graph.EdgeTypeRelatesTo,
			CreatedAt: now,
		}
		if err := mm.store.CreateEdge(ctx, edge); err != nil {
			log.Warn().Err(err).Msg("failed to link memory to task")
		}
	}

	for _, filePath := range memory.FilePaths {
		fileID := uuid.NewSHA1(uuid.NameSpaceURL, []byte(filePath))
		fileNode := &graph.File{ID: fileID, Path: filePath}
		fileGraphNode, _ := fileNode.ToNode()
		_ = mm.store.CreateNode(ctx, fileGraphNode)

		edge := &graph.Edge{
			ID:        uuid.New(),
			FromID:    memory.ID,
			ToID:      fileID,
			Type:      graph.EdgeTypeReferences,
			CreatedAt: now,
		}
		if err := mm.store.CreateEdge(ctx, edge); err != nil {
			log.Warn().Err(err).Str("file", filePath).Msg("failed to link memory to file")
		}
	}

	for _, refID := range memory.References {
		edge := &graph.Edge{
			ID:        uuid.New(),
			FromID:    memory.ID,
			ToID:      refID,
			Type:      graph.EdgeTypeDerivedFrom,
			CreatedAt: now,
		}
		if err := mm.store.CreateEdge(ctx, edge); err != nil {
			log.Warn().Err(err).Msg("failed to link memory to reference")
		}
	}

	return nil
}

func (mm *MemoryManager) findSimilarMemories(ctx context.Context, memory *Memory) ([]SimilarityResult, error) {
	nodes, err := mm.store.ListNodes(ctx, graph.NodeTypeMemory, 0, 0)
	if err != nil {
		return nil, err
	}

	var results []SimilarityResult
	for _, node := range nodes {
		var existing Memory
		if err := node.UnmarshalData(&existing); err != nil {
			continue
		}
		if existing.ID == memory.ID {
			continue
		}

		similarity := mm.calculateSimilarity(memory, &existing)
		if similarity >= mm.dedupThreshold {
			results = append(results, SimilarityResult{
				MemoryID:   existing.ID,
				Similarity: similarity,
				MatchType:  "embedding",
			})
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Similarity > results[j].Similarity
	})

	return results, nil
}

func (mm *MemoryManager) calculateSimilarity(a, b *Memory) float64 {
	if len(a.Embedding) > 0 && len(b.Embedding) > 0 {
		return cosineSimilarity(a.Embedding, b.Embedding)
	}
	return textSimilarity(a.Content, b.Content)
}

func cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i] * b[i])
		normA += float64(a[i] * a[i])
		normB += float64(b[i] * b[i])
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

func textSimilarity(a, b string) float64 {
	if a == b {
		return 1.0
	}
	if len(a) == 0 || len(b) == 0 {
		return 0.0
	}
	setA := shingleSet(a)
	setB := shingleSet(b)
	intersection := 0
	for k := range setA {
		if setB[k] {
			intersection++
		}
	}
	union := len(setA) + len(setB) - intersection
	if union == 0 {
		return 0
	}
	return float64(intersection) / float64(union)
}

func shingleSet(s string) map[string]bool {
	set := make(map[string]bool)
	runes := []rune(s)
	for i := 0; i <= len(runes)-3; i++ {
		set[string(runes[i:i+3])] = true
	}
	return set
}

func (mm *MemoryManager) mergeMemories(ctx context.Context, newMem *Memory, existingID uuid.UUID, similarity float64) error {
	node, err := mm.store.GetNode(ctx, existingID)
	if err != nil {
		return err
	}
	var existing Memory
	if err := node.UnmarshalData(&existing); err != nil {
		return err
	}

	existing.Content = mergeContent(existing.Content, newMem.Content)
	existing.Confidence = math.Max(existing.Confidence, newMem.Confidence)
	existing.AccessCount += newMem.AccessCount
	existing.UpdatedAt = time.Now()

	existing.Tags = mergeUniqueStrings(existing.Tags, newMem.Tags)
	existing.FilePaths = mergeUniqueStrings(existing.FilePaths, newMem.FilePaths)
	existing.ContextBuckets = mergeUniqueUUIDs(existing.ContextBuckets, newMem.ContextBuckets)
	existing.TaskIDs = mergeUniqueUUIDs(existing.TaskIDs, newMem.TaskIDs)
	existing.References = mergeUniqueUUIDs(existing.References, newMem.References)

	if newMem.Metadata != nil {
		if existing.Metadata == nil {
			existing.Metadata = newMem.Metadata
		} else {
			var existingMeta, newMeta map[string]interface{}
			json.Unmarshal(existing.Metadata, &existingMeta)
			json.Unmarshal(newMem.Metadata, &newMeta)
			for k, v := range newMeta {
				existingMeta[k] = v
			}
			existing.Metadata, _ = json.Marshal(existingMeta)
		}
	}

	updatedNode, err := existing.ToNode()
	if err != nil {
		return err
	}
	return mm.store.UpdateNode(ctx, updatedNode)
}

func mergeContent(a, b string) string {
	if a == b {
		return a
	}
	return a + "\n---\n" + b
}

func mergeUniqueStrings(a, b []string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, s := range a {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}
	for _, s := range b {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}
	return result
}

func mergeUniqueUUIDs(a, b []uuid.UUID) []uuid.UUID {
	seen := make(map[uuid.UUID]bool)
	var result []uuid.UUID
	for _, id := range a {
		if !seen[id] {
			seen[id] = true
			result = append(result, id)
		}
	}
	for _, id := range b {
		if !seen[id] {
			seen[id] = true
			result = append(result, id)
		}
	}
	return result
}

func (mm *MemoryManager) GetMemoriesForContext(ctx context.Context, bucketIDs []uuid.UUID) ([]*Memory, error) {
	mm.mu.RLock()
	defer mm.mu.RUnlock()

	var allMemories []*Memory
	seen := make(map[uuid.UUID]bool)

	for _, bucketID := range bucketIDs {
		edges, err := mm.store.GetEdgesFrom(ctx, bucketID, graph.EdgeTypeContains)
		if err != nil {
			continue
		}
		for _, edge := range edges {
			if seen[edge.ToID] {
				continue
			}
			node, err := mm.store.GetNode(ctx, edge.ToID)
			if err != nil {
				continue
			}
			if node.Type == graph.NodeTypeMemory {
				var m Memory
				if err := node.UnmarshalData(&m); err == nil {
					m.AccessCount++
					now := time.Now()
					m.LastAccessed = &now
					allMemories = append(allMemories, &m)
					seen[edge.ToID] = true
				}
			}
		}
	}

	sort.Slice(allMemories, func(i, j int) bool {
		return allMemories[i].Confidence > allMemories[j].Confidence
	})

	return allMemories, nil
}

func (mm *MemoryManager) PromoteMemory(ctx context.Context, memoryID uuid.UUID, confidence float64) error {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	if confidence < mm.promotionThreshold {
		return fmt.Errorf("confidence %.2f below promotion threshold %.2f", confidence, mm.promotionThreshold)
	}

	node, err := mm.store.GetNode(ctx, memoryID)
	if err != nil {
		return err
	}
	var memory Memory
	if err := node.UnmarshalData(&memory); err != nil {
		return err
	}

	memory.Scope = MemoryScopeProject
	memory.Confidence = confidence
	memory.UpdatedAt = time.Now()

	updatedNode, err := memory.ToNode()
	if err != nil {
		return err
	}
	return mm.store.UpdateNode(ctx, updatedNode)
}

func (mm *MemoryManager) GetProjectMemories(ctx context.Context, tags []string) ([]*Memory, error) {
	mm.mu.RLock()
	defer mm.mu.RUnlock()

	nodes, err := mm.store.ListNodes(ctx, graph.NodeTypeMemory, 0, 0)
	if err != nil {
		return nil, err
	}

	var memories []*Memory
	for _, node := range nodes {
		var m Memory
		if err := node.UnmarshalData(&m); err != nil {
			continue
		}
		if m.Scope != MemoryScopeProject {
			continue
		}
		if len(tags) > 0 {
			match := false
			for _, tag := range tags {
				for _, mt := range m.Tags {
					if mt == tag {
						match = true
						break
					}
				}
				if match {
					break
				}
			}
			if !match {
				continue
			}
		}
		memories = append(memories, &m)
	}

	sort.Slice(memories, func(i, j int) bool {
		return memories[i].Confidence > memories[j].Confidence
	})

	return memories, nil
}

func (mm *MemoryManager) DeduplicateMemories(ctx context.Context) (int, error) {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	nodes, err := mm.store.ListNodes(ctx, graph.NodeTypeMemory, 0, 0)
	if err != nil {
		return 0, err
	}

	var memories []*Memory
	for _, node := range nodes {
		var m Memory
		if err := node.UnmarshalData(&m); err == nil {
			memories = append(memories, &m)
		}
	}

	merged := 0
	processed := make(map[uuid.UUID]bool)

	for i := range memories {
		if processed[memories[i].ID] {
			continue
		}
		for j := i + 1; j < len(memories); j++ {
			if processed[memories[j].ID] {
				continue
			}
			sim := mm.calculateSimilarity(memories[i], memories[j])
			if sim >= mm.dedupThreshold {
				if err := mm.mergeMemories(ctx, memories[j], memories[i].ID, sim); err == nil {
					processed[memories[j].ID] = true
					merged++
				}
			}
		}
	}

	return merged, nil
}

func (m *Memory) ToNode() (*graph.Node, error) {
	data, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	return &graph.Node{
		ID:        m.ID,
		Type:      graph.NodeTypeMemory,
		Data:      data,
		CreatedAt: m.CreatedAt,
		UpdatedAt: m.UpdatedAt,
	}, nil
}