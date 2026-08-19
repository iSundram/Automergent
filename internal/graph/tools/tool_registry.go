package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

type ToolRegistry struct {
	mu              sync.RWMutex
	tools           map[string]*ToolDefinition
	effectiveness   map[string]map[ContextBucketType]*ToolEffectiveness
	usageRecords    []ToolUsageRecord
	store           GraphStore
	logger          *zap.Logger
}

func NewToolRegistry(store GraphStore, logger *zap.Logger) *ToolRegistry {
	tr := &ToolRegistry{
		tools:         make(map[string]*ToolDefinition),
		effectiveness: make(map[string]map[ContextBucketType]*ToolEffectiveness),
		usageRecords:  make([]ToolUsageRecord, 0),
		store:         store,
		logger:        logger,
	}
	return tr
}

func (tr *ToolRegistry) RegisterTool(definition *ToolDefinition) error {
	tr.mu.Lock()
	defer tr.mu.Unlock()

	if _, exists := tr.tools[definition.Name]; exists {
		return fmt.Errorf("tool %s already registered", definition.Name)
	}

	now := time.Now()
	definition.CreatedAt = now
	definition.UpdatedAt = now

	tr.tools[definition.Name] = definition

	if tr.store != nil {
		node, err := tr.toolToNode(definition)
		if err != nil {
			tr.logger.Error("failed to create tool node", zap.Error(err))
		} else {
			if err := tr.store.CreateNode(context.Background(), node); err != nil {
				tr.logger.Error("failed to persist tool", zap.Error(err))
			}
		}
	}

	return nil
}

func (tr *ToolRegistry) GetTool(name string) (*ToolDefinition, bool) {
	tr.mu.RLock()
	defer tr.mu.RUnlock()
	tool, ok := tr.tools[name]
	return tool, ok
}

func (tr *ToolRegistry) SuggestTools(contextBucket ContextBucketType) []*ToolDefinition {
	tr.mu.RLock()
	defer tr.mu.RUnlock()

	type scoredTool struct {
		tool    *ToolDefinition
		score   float64
	}

	var scored []scoredTool

	for _, tool := range tr.tools {
		score := tr.calculateRelevance(tool, contextBucket)
		if score > 0 {
			scored = append(scored, scoredTool{tool: tool, score: score})
		}
	}

	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	result := make([]*ToolDefinition, len(scored))
	for i, s := range scored {
		result[i] = s.tool
	}
	return result
}

func (tr *ToolRegistry) calculateRelevance(tool *ToolDefinition, contextBucket ContextBucketType) float64 {
	maxScore := 0.0
	for _, trigger := range tool.ContextTriggers {
		if trigger.BucketType == contextBucket || trigger.BucketType == ContextBucketTypeGlobal {
			keywordScore := tr.keywordMatchScore(trigger.Keywords, contextBucket)
			if keywordScore >= trigger.MinRelevance {
				score := keywordScore * (1.0 + tr.getEffectivenessBoost(tool.Name, contextBucket))
				if score > maxScore {
					maxScore = score
				}
			}
		}
	}
	return maxScore
}

func (tr *ToolRegistry) keywordMatchScore(keywords []string, contextBucket ContextBucketType) float64 {
	if len(keywords) == 0 {
		return 0.5
	}

	bucketStr := string(contextBucket)
	matches := 0
	for _, kw := range keywords {
		if strings.Contains(strings.ToLower(bucketStr), strings.ToLower(kw)) {
			matches++
		}
	}
	return float64(matches) / float64(len(keywords))
}

func (tr *ToolRegistry) getEffectivenessBoost(toolName string, contextBucket ContextBucketType) float64 {
	if effMap, ok := tr.effectiveness[toolName]; ok {
		if eff, ok := effMap[contextBucket]; ok && eff.TotalUsages > 0 {
			successRate := float64(eff.SuccessfulUsages) / float64(eff.TotalUsages)
			return successRate * 0.3
		}
	}
	return 0.0
}

func (tr *ToolRegistry) RecordToolUsage(toolName string, contextBucket ContextBucketType, outcome string, relevance float64, durationMs int64, err error) {
	tr.mu.Lock()
	defer tr.mu.Unlock()

	record := ToolUsageRecord{
		ID:            uuid.New(),
		ToolName:      toolName,
		ContextBucket: contextBucket,
		Outcome:       outcome,
		Relevance:     relevance,
		DurationMs:    durationMs,
		Timestamp:     time.Now(),
	}
	if err != nil {
		record.Error = err.Error()
	}
	tr.usageRecords = append(tr.usageRecords, record)

	if _, ok := tr.effectiveness[toolName]; !ok {
		tr.effectiveness[toolName] = make(map[ContextBucketType]*ToolEffectiveness)
	}
	if _, ok := tr.effectiveness[toolName][contextBucket]; !ok {
		tr.effectiveness[toolName][contextBucket] = &ToolEffectiveness{
			ToolName:      toolName,
			ContextBucket: contextBucket,
		}
	}

	eff := tr.effectiveness[toolName][contextBucket]
	eff.TotalUsages++
	if outcome == "success" {
		eff.SuccessfulUsages++
	} else {
		eff.FailedUsages++
	}
	eff.AvgRelevance = (eff.AvgRelevance*float64(eff.TotalUsages-1) + relevance) / float64(eff.TotalUsages)
	eff.LastUsed = time.Now()
	eff.UpdatedAt = time.Now()

	if tr.store != nil {
		tr.persistEffectiveness(eff)
		tr.persistUsageRecord(&record)
	}
}

func (tr *ToolRegistry) GetToolEffectiveness(toolName string) map[ContextBucketType]*ToolEffectiveness {
	tr.mu.RLock()
	defer tr.mu.RUnlock()

	if effMap, ok := tr.effectiveness[toolName]; ok {
		result := make(map[ContextBucketType]*ToolEffectiveness)
		for k, v := range effMap {
			result[k] = v
		}
		return result
	}
	return nil
}

func (tr *ToolRegistry) LearnToolPattern(toolName string, contextBucket ContextBucketType) (*DynamicTool, error) {
	tr.mu.RLock()
	tool, ok := tr.tools[toolName]
	tr.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("tool %s not found", toolName)
	}

	effMap := tr.GetToolEffectiveness(toolName)
	eff, ok := effMap[contextBucket]
	if !ok || eff.TotalUsages < 5 {
		return nil, fmt.Errorf("insufficient usage data for pattern learning")
	}

	if eff.SuccessfulUsages < eff.TotalUsages/2 {
		return nil, fmt.Errorf("tool not effective enough in this context")
	}

	pattern := tr.extractPattern(tool, contextBucket)
	confidence := float64(eff.SuccessfulUsages) / float64(eff.TotalUsages)

	dynamicTool := &DynamicTool{
		ID:            uuid.New(),
		Name:          fmt.Sprintf("%s_dynamic_%s", toolName, contextBucket),
		Description:   fmt.Sprintf("Dynamic variant of %s for %s context", toolName, contextBucket),
		Character:     tool.Character,
		Parameters:    tool.Parameters,
		Pattern:       pattern,
		ContextBucket: contextBucket,
		Confidence:    confidence,
		SourceTool:    toolName,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	tr.mu.Lock()
	defer tr.mu.Unlock()

	if tr.store != nil {
		node, err := tr.dynamicToolToNode(dynamicTool)
		if err == nil {
			tr.store.CreateNode(context.Background(), node)
		}
	}

	return dynamicTool, nil
}

func (tr *ToolRegistry) extractPattern(tool *ToolDefinition, contextBucket ContextBucketType) string {
	var parts []string
	parts = append(parts, fmt.Sprintf("tool:%s", tool.Name))
	parts = append(parts, fmt.Sprintf("context:%s", contextBucket))
	for _, uc := range tool.UseCases {
		if uc.Context == contextBucket || uc.Context == ContextBucketTypeGlobal {
			parts = append(parts, fmt.Sprintf("usecase:%s", uc.Description))
		}
	}
	return strings.Join(parts, "|")
}

func (tr *ToolRegistry) toolToNode(tool *ToolDefinition) (*Node, error) {
	data, err := json.Marshal(tool)
	if err != nil {
		return nil, err
	}
	return &Node{
		ID:        uuid.New(),
		Type:      NodeTypeToolDefinition,
		Data:      data,
		CreatedAt: tool.CreatedAt,
		UpdatedAt: tool.UpdatedAt,
	}, nil
}

func (tr *ToolRegistry) dynamicToolToNode(dt *DynamicTool) (*Node, error) {
	data, err := json.Marshal(dt)
	if err != nil {
		return nil, err
	}
	return &Node{
		ID:        dt.ID,
		Type:      NodeTypeDynamicTool,
		Data:      data,
		CreatedAt: dt.CreatedAt,
		UpdatedAt: dt.UpdatedAt,
	}, nil
}

func (tr *ToolRegistry) persistEffectiveness(eff *ToolEffectiveness) {
	node := &Node{
		ID:        uuid.New(),
		Type:      NodeTypeToolEffectiveness,
		CreatedAt: eff.UpdatedAt,
		UpdatedAt: eff.UpdatedAt,
	}
	data, _ := json.Marshal(eff)
	node.Data = data
	tr.store.CreateNode(context.Background(), node)
}

func (tr *ToolRegistry) persistUsageRecord(record *ToolUsageRecord) {
	node := &Node{
		ID:        record.ID,
		Type:      NodeTypeToolUsage,
		CreatedAt: record.Timestamp,
		UpdatedAt: record.Timestamp,
	}
	data, _ := json.Marshal(record)
	node.Data = data
	tr.store.CreateNode(context.Background(), node)
}

func (tr *ToolRegistry) ListTools() []*ToolDefinition {
	tr.mu.RLock()
	defer tr.mu.RUnlock()

	tools := make([]*ToolDefinition, 0, len(tr.tools))
	for _, t := range tr.tools {
		tools = append(tools, t)
	}
	return tools
}

func (tr *ToolRegistry) GetUsageRecords(toolName string, contextBucket ContextBucketType, limit int) []ToolUsageRecord {
	tr.mu.RLock()
	defer tr.mu.RUnlock()

	var filtered []ToolUsageRecord
	for i := len(tr.usageRecords) - 1; i >= 0 && len(filtered) < limit; i-- {
		r := tr.usageRecords[i]
		if r.ToolName == toolName && (contextBucket == "" || r.ContextBucket == contextBucket) {
			filtered = append(filtered, r)
		}
	}
	return filtered
}