package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

type DynamicToolGenerator struct {
	mu            sync.RWMutex
	dynamicTools  map[uuid.UUID]*DynamicTool
	patterns      map[string]*ObservedPattern
	store         GraphStore
	logger        *zap.Logger
	registry      *ToolRegistry
}

func NewDynamicToolGenerator(store GraphStore, logger *zap.Logger, registry *ToolRegistry) *DynamicToolGenerator {
	return &DynamicToolGenerator{
		dynamicTools: make(map[uuid.UUID]*DynamicTool),
		patterns:     make(map[string]*ObservedPattern),
		store:        store,
		logger:       logger,
		registry:     registry,
	}
}

func (dtg *DynamicToolGenerator) GenerateTool(pattern string, contextBucket ContextBucketType) (*DynamicTool, error) {
	dtg.mu.Lock()
	defer dtg.mu.Unlock()

	obsPattern, ok := dtg.patterns[pattern]
	if !ok {
		return nil, fmt.Errorf("pattern %s not observed", pattern)
	}

	if obsPattern.Confidence < 0.6 {
		return nil, fmt.Errorf("pattern confidence too low: %.2f", obsPattern.Confidence)
	}

	baseToolName := dtg.extractBaseTool(obsPattern.ToolSequence)
	baseTool, _ := dtg.registry.GetTool(baseToolName)

	character := ToolCharacterModifying
	params := json.RawMessage(`{}`)
	if baseTool != nil {
		character = baseTool.Character
		params = baseTool.Parameters
	}

	dynamicTool := &DynamicTool{
		ID:                uuid.New(),
		Name:              fmt.Sprintf("dynamic_%s_%s", sanitizeName(pattern), contextBucket),
		Description:       fmt.Sprintf("Auto-generated tool for pattern: %s", pattern),
		Character:         character,
		Parameters:        params,
		Pattern:           pattern,
		ContextBucket:     contextBucket,
		Confidence:        obsPattern.Confidence,
		UsageCount:        0,
		SuccessCount:      0,
		SourceTool:        baseToolName,
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
		Promoted:          false,
	}

	dtg.dynamicTools[dynamicTool.ID] = dynamicTool

	if dtg.store != nil {
		node, err := dtg.dynamicToolToNode(dynamicTool)
		if err == nil {
			dtg.store.CreateNode(context.Background(), node)
		}
	}

	return dynamicTool, nil
}

func (dtg *DynamicToolGenerator) LearnFromUsage(toolName string, contextBucket ContextBucketType, outcome string, durationMs int64) {
	dtg.mu.Lock()
	defer dtg.mu.Unlock()

	for _, dt := range dtg.dynamicTools {
		if dt.Name == toolName && dt.ContextBucket == contextBucket {
			dt.UsageCount++
			if outcome == "success" {
				dt.SuccessCount++
			}
			dt.Confidence = float64(dt.SuccessCount) / float64(dt.UsageCount)
			dt.UpdatedAt = time.Now()

			if dtg.store != nil {
				dtg.persistDynamicTool(dt)
			}
			break
		}
	}
}

func (dtg *DynamicToolGenerator) RecordPatternUsage(pattern string, contextBucket ContextBucketType, toolSequence []string, success bool, durationMs int64) {
	dtg.mu.Lock()
	defer dtg.mu.Unlock()

	key := fmt.Sprintf("%s|%s", pattern, contextBucket)
	obsPattern, ok := dtg.patterns[key]
	if !ok {
		obsPattern = &ObservedPattern{
			ID:            uuid.New(),
			Pattern:       pattern,
			ContextBucket: contextBucket,
			ToolSequence:  toolSequence,
			Frequency:     0,
			SuccessRate:   0,
			AvgDurationMs: 0,
			CreatedAt:     time.Now(),
		}
		dtg.patterns[key] = obsPattern
	}

	obsPattern.Frequency++
	obsPattern.LastObserved = time.Now()
	obsPattern.ToolSequence = toolSequence

	if success {
		obsPattern.SuccessRate = (obsPattern.SuccessRate*float64(obsPattern.Frequency-1) + 1.0) / float64(obsPattern.Frequency)
	} else {
		obsPattern.SuccessRate = (obsPattern.SuccessRate * float64(obsPattern.Frequency-1)) / float64(obsPattern.Frequency)
	}

	obsPattern.AvgDurationMs = (obsPattern.AvgDurationMs*int64(obsPattern.Frequency-1) + durationMs) / int64(obsPattern.Frequency)
	obsPattern.Confidence = dtg.calculateConfidence(obsPattern)
	obsPattern.UpdatedAt = time.Now()

	if dtg.store != nil {
		dtg.persistPattern(obsPattern)
	}
}

func (dtg *DynamicToolGenerator) calculateConfidence(pattern *ObservedPattern) float64 {
	if pattern.Frequency < 3 {
		return 0.1
	}

	freqScore := math.Min(float64(pattern.Frequency)/20.0, 1.0)
	successScore := pattern.SuccessRate
	recencyScore := dtg.recencyScore(pattern.LastObserved)

	return (freqScore*0.4 + successScore*0.4 + recencyScore*0.2)
}

func (dtg *DynamicToolGenerator) recencyScore(lastObserved time.Time) float64 {
	hoursSince := time.Since(lastObserved).Hours()
	if hoursSince < 1 {
		return 1.0
	} else if hoursSince < 24 {
		return 0.8
	} else if hoursSince < 168 {
		return 0.5
	} else if hoursSince < 720 {
		return 0.2
	}
	return 0.1
}

func (dtg *DynamicToolGenerator) GetDynamicTools(contextBucket ContextBucketType) []*DynamicTool {
	dtg.mu.RLock()
	defer dtg.mu.RUnlock()

	var tools []*DynamicTool
	for _, dt := range dtg.dynamicTools {
		if dt.ContextBucket == contextBucket || contextBucket == ContextBucketTypeGlobal {
			tools = append(tools, dt)
		}
	}

	sort.Slice(tools, func(i, j int) bool {
		return tools[i].Confidence > tools[j].Confidence
	})

	return tools
}

func (dtg *DynamicToolGenerator) PromoteTool(dynamicToolID uuid.UUID) (*ToolDefinition, error) {
	dtg.mu.Lock()
	defer dtg.mu.Unlock()

	dt, ok := dtg.dynamicTools[dynamicToolID]
	if !ok {
		return nil, fmt.Errorf("dynamic tool not found: %s", dynamicToolID)
	}

	if dt.Promoted {
		return nil, fmt.Errorf("tool already promoted")
	}

	if dt.Confidence < 0.8 {
		return nil, fmt.Errorf("confidence too low for promotion: %.2f", dt.Confidence)
	}

	if dt.UsageCount < 10 {
		return nil, fmt.Errorf("insufficient usage for promotion: %d", dt.UsageCount)
	}

	if dt.SuccessCount < dt.UsageCount*8/10 {
		return nil, fmt.Errorf("success rate too low for promotion: %.2f", float64(dt.SuccessCount)/float64(dt.UsageCount))
	}

	toolDef := &ToolDefinition{
		Name:            dt.Name,
		Description:     dt.Description,
		Character:       dt.Character,
		Parameters:      dt.Parameters,
		ContextTriggers: []ContextTrigger{
			{BucketType: dt.ContextBucket, Keywords: dtg.extractKeywords(dt.Pattern), MinRelevance: 0.3},
		},
		UseCases: []UseCase{
			{Description: fmt.Sprintf("Auto-promoted from dynamic tool: %s", dt.Pattern), Context: dt.ContextBucket},
		},
		Version:   "1.0.0",
		Author:    "dynamic_tool_generator",
		Tags:      []string{"auto-generated", "promoted"},
		CreatedAt: dt.CreatedAt,
		UpdatedAt: time.Now(),
	}

	if err := dtg.registry.RegisterTool(toolDef); err != nil {
		return nil, fmt.Errorf("failed to register promoted tool: %w", err)
	}

	dt.Promoted = true
	dt.UpdatedAt = time.Now()

	if dtg.store != nil {
		dtg.persistDynamicTool(dt)
	}

	delete(dtg.dynamicTools, dynamicToolID)

	return toolDef, nil
}

func (dtg *DynamicToolGenerator) extractBaseTool(toolSequence []string) string {
	if len(toolSequence) == 0 {
		return ""
	}
	return toolSequence[0]
}

func (dtg *DynamicToolGenerator) extractKeywords(pattern string) []string {
	parts := strings.Split(pattern, "|")
	var keywords []string
	for _, part := range parts {
		if strings.HasPrefix(part, "tool:") {
			keywords = append(keywords, strings.TrimPrefix(part, "tool:"))
		} else if strings.HasPrefix(part, "context:") {
			keywords = append(keywords, strings.TrimPrefix(part, "context:"))
		} else if strings.HasPrefix(part, "usecase:") {
			keywords = append(keywords, strings.TrimPrefix(part, "usecase:"))
		}
	}
	return keywords
}

func sanitizeName(name string) string {
	name = strings.ReplaceAll(name, "|", "_")
	name = strings.ReplaceAll(name, ":", "_")
	name = strings.ReplaceAll(name, " ", "_")
	return name
}

func (dtg *DynamicToolGenerator) dynamicToolToNode(dt *DynamicTool) (*Node, error) {
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

func (dtg *DynamicToolGenerator) persistDynamicTool(dt *DynamicTool) {
	node, err := dtg.dynamicToolToNode(dt)
	if err != nil {
		return
	}
	dtg.store.CreateNode(context.Background(), node)
}

func (dtg *DynamicToolGenerator) persistPattern(pattern *ObservedPattern) {
	node := &Node{
		ID:        pattern.ID,
		Type:      NodeTypeObservedPattern,
		CreatedAt: pattern.CreatedAt,
		UpdatedAt: pattern.UpdatedAt,
	}
	data, _ := json.Marshal(pattern)
	node.Data = data
	dtg.store.CreateNode(context.Background(), node)
}

func (dtg *DynamicToolGenerator) GetPattern(pattern string, contextBucket ContextBucketType) (*ObservedPattern, bool) {
	dtg.mu.RLock()
	defer dtg.mu.RUnlock()

	key := fmt.Sprintf("%s|%s", pattern, contextBucket)
	p, ok := dtg.patterns[key]
	return p, ok
}

func (dtg *DynamicToolGenerator) ListPatterns(contextBucket ContextBucketType) []*ObservedPattern {
	dtg.mu.RLock()
	defer dtg.mu.RUnlock()

	var patterns []*ObservedPattern
	for _, p := range dtg.patterns {
		if p.ContextBucket == contextBucket || contextBucket == ContextBucketTypeGlobal {
			patterns = append(patterns, p)
		}
	}

	sort.Slice(patterns, func(i, j int) bool {
		return patterns[i].Confidence > patterns[j].Confidence
	})

	return patterns
}

func (dtg *DynamicToolGenerator) GetPromotedTools() []*DynamicTool {
	dtg.mu.RLock()
	defer dtg.mu.RUnlock()

	var promoted []*DynamicTool
	for _, dt := range dtg.dynamicTools {
		if dt.Promoted {
			promoted = append(promoted, dt)
		}
	}
	return promoted
}

func (dtg *DynamicToolGenerator) LoadFromStore(ctx context.Context) error {
	if dtg.store == nil {
		return nil
	}

	nodes, err := dtg.store.ListNodes(ctx, NodeTypeDynamicTool, 0, 0)
	if err != nil {
		return err
	}

	for _, node := range nodes {
		var dt DynamicTool
		if err := json.Unmarshal(node.Data, &dt); err != nil {
			continue
		}
		dtg.dynamicTools[dt.ID] = &dt
	}

	patternNodes, err := dtg.store.ListNodes(ctx, NodeTypeObservedPattern, 0, 0)
	if err != nil {
		return err
	}

	for _, node := range patternNodes {
		var p ObservedPattern
		if err := json.Unmarshal(node.Data, &p); err != nil {
			continue
		}
		key := fmt.Sprintf("%s|%s", p.Pattern, p.ContextBucket)
		dtg.patterns[key] = &p
	}

	return nil
}