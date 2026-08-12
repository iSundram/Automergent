package learning

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
)

// StrategyAdapter adapts and suggests strategies.
type StrategyAdapter struct {
	storage    Storage
	strategies map[string]*Strategy
	mu         sync.RWMutex
}

// NewStrategyAdapter creates a new strategy adapter.
func NewStrategyAdapter(storage Storage) *StrategyAdapter {
	a := &StrategyAdapter{
		storage:    storage,
		strategies: make(map[string]*Strategy),
	}

	a.strategies["default-cli-go"] = &Strategy{
		ID:          "default-cli-go",
		Name:        "Go CLI",
		Description: "Default strategy for Go CLI projects",
		ProjectType: "cli",
		Framework:   "go",
		SuccessRate: 0.8,
		AvgDuration: 3 * time.Second,
	}
	a.strategies["default-web"] = &Strategy{
		ID:          "default-web",
		Name:        "Web App",
		Description: "Default strategy for web projects",
		ProjectType: "web",
		Framework:   "nextjs",
		SuccessRate: 0.75,
		AvgDuration: 5 * time.Second,
	}

	return a
}

// ProcessEvent updates strategy state from an event.
func (a *StrategyAdapter) ProcessEvent(ctx context.Context, event Event) {
	a.mu.Lock()
	defer a.mu.Unlock()

	id, _ := event.Data["strategy_used"].(string)
	if id == "" {
		return
	}

	strategy, ok := a.strategies[id]
	if !ok {
		strategy = &Strategy{ID: id, Name: id, ProjectType: "unknown"}
		a.strategies[id] = strategy
	}

	strategy.UseCount++
	strategy.LastUsed = event.Timestamp
}

// SuggestStrategy suggests a strategy for a task.
func (a *StrategyAdapter) SuggestStrategy(ctx context.Context, taskDescription string, projectCtx *ProjectContext) (*Strategy, float64, error) {
	if projectCtx == nil {
		return nil, 0, fmt.Errorf("project context required")
	}

	a.mu.RLock()
	defer a.mu.RUnlock()

	var best *Strategy
	for _, strategy := range a.strategies {
		if strategy.ProjectType == projectCtx.Type && (strategy.Framework == "" || strategy.Framework == projectCtx.Framework) {
			best = strategy
			break
		}
	}

	if best == nil {
		for _, strategy := range a.strategies {
			best = strategy
			break
		}
	}
	if best == nil {
		return nil, 0, fmt.Errorf("no strategy available")
	}

	return best, best.SuccessRate, nil
}

// OptimizeProvider returns the preferred provider.
func (a *StrategyAdapter) OptimizeProvider(ctx context.Context, taskType string, budget float64) (string, error) {
	return "google", nil
}

// GenerateInsights returns basic strategy insights.
func (a *StrategyAdapter) GenerateInsights(ctx context.Context) ([]Insight, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	var insights []Insight
	for _, strategy := range a.strategies {
		if strategy.UseCount >= 10 && strategy.SuccessRate < 0.6 {
			insights = append(insights, Insight{
				ID:          uuid.New().String(),
				Type:        InsightTypeQuality,
				Title:       "Low success rate strategy",
				Description: strategy.Name,
				Confidence:  0.8,
				Impact:      ImpactMedium,
				Actionable:  true,
				CreatedAt:   time.Now(),
			})
		}
	}

	return insights, nil
}

// KnowledgeBase stores and retrieves learned knowledge.
type KnowledgeBase struct {
	storage    Storage
	maxEntries int
}

// NewKnowledgeBase creates a new knowledge base.
func NewKnowledgeBase(storage Storage, maxEntries int) *KnowledgeBase {
	return &KnowledgeBase{storage: storage, maxEntries: maxEntries}
}

// ProcessEvent extracts knowledge from an event.
func (k *KnowledgeBase) ProcessEvent(ctx context.Context, event Event) {
	if k.storage == nil {
		return
	}

	if key, ok := event.Data["knowledge_key"].(string); ok && key != "" {
		entry := KnowledgeEntry{
			ID:         uuid.New().String(),
			Scope:      ScopeProject,
			Category:   CategoryPattern,
			Key:        key,
			Value:      event.Data["knowledge_value"],
			Confidence: 0.5,
			Source:     event.Type.String(),
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
			Metadata:   map[string]interface{}{},
		}
		_ = k.storage.SaveKnowledge(ctx, entry)
	}
}

// Get retrieves knowledge entries.
func (k *KnowledgeBase) Get(ctx context.Context, scope KnowledgeScope, category KnowledgeCategory) ([]KnowledgeEntry, error) {
	if k.storage == nil {
		return nil, nil
	}
	return k.storage.GetKnowledge(ctx, scope, category)
}

// GetProjectContext retrieves a project context.
func (k *KnowledgeBase) GetProjectContext(ctx context.Context, projectPath string) (*ProjectContext, error) {
	if k.storage == nil {
		return nil, nil
	}
	return k.storage.GetProjectContext(ctx, projectPath)
}

// GenerateInsights returns basic knowledge insights.
func (k *KnowledgeBase) GenerateInsights(ctx context.Context) ([]Insight, error) {
	return nil, nil
}

// ProcessEvent processes a feedback event into the collector.
func (fc *FeedbackCollector) ProcessEvent(ctx context.Context, event Event) {
	if event.Type != EventTypeUserFeedback {
		return
	}

	fb := Feedback{
		ID:        event.ID,
		Timestamp: event.Timestamp,
		SessionID: event.SessionID,
	}
	if t, ok := event.Data["feedback_type"].(string); ok {
		fb.Type = FeedbackType(t)
	}
	if target, ok := event.Data["target"].(string); ok {
		fb.Target = target
	}
	if targetType, ok := event.Data["target_type"].(string); ok {
		fb.TargetType = targetType
	}
	if rating, ok := event.Data["rating"].(float64); ok {
		fb.Rating = int(rating)
	}
	if signal, ok := event.Data["signal"].(float64); ok {
		fb.Signal = signal
	}

	fc.Record(ctx, fb)
}

// Record records feedback and updates aggregations.
func (fc *FeedbackCollector) Record(ctx context.Context, fb Feedback) error {
	fc.record(&fb)
	return nil
}

// GenerateInsights returns feedback insights.
func (fc *FeedbackCollector) GenerateInsights(ctx context.Context) ([]Insight, error) {
	fc.mu.RLock()
	defer fc.mu.RUnlock()

	var insights []Insight
	for _, agg := range fc.aggregates {
		if agg.TotalCount >= 5 && agg.NetSignal < 0 {
			insights = append(insights, Insight{
				ID:          uuid.New().String(),
				Type:        InsightTypeUserBehavior,
				Title:       "Negative feedback trend",
				Description: fmt.Sprintf("%s:%s has negative feedback", agg.TargetType, agg.Target),
				Confidence:  0.7,
				Impact:      ImpactMedium,
				Actionable:  true,
				CreatedAt:   time.Now(),
			})
		}
	}
	return insights, nil
}

// InferPreferences infers high-level preferences from feedback.
func (fc *FeedbackCollector) InferPreferences() map[string]interface{} {
	fc.mu.RLock()
	defer fc.mu.RUnlock()

	favorite := make([]string, 0)
	for _, agg := range fc.aggregates {
		if agg.AcceptCount > agg.RejectCount && agg.Target != "" {
			favorite = append(favorite, agg.Target)
		}
	}

	return map[string]interface{}{
		"favorite_tools": favorite,
	}
}

// ProcessEvent routes an event into the recognizer.
func (pr *PatternRecognizer) ProcessEvent(ctx context.Context, event Event) {
	switch event.Type {
	case EventTypeToolUse:
		tool, _ := event.Data["tool"].(string)
		pr.RecordToolUsage(tool, event.Data, true)
	case EventTypeFileAccess:
		path, _ := event.Data["file"].(string)
		pr.RecordFileAccess(path, "access")
	case EventTypeTaskComplete, EventTypeTaskFail:
		pr.RecordWorkflow(event.Type.String(), event.SessionID)
	}
}

// GenerateInsights returns insights from confident patterns.
func (pr *PatternRecognizer) GenerateInsights(ctx context.Context) ([]Insight, error) {
	patterns := pr.GetConfidentPatterns()
	insights := make([]Insight, 0, len(patterns))

	for _, p := range patterns {
		insights = append(insights, Insight{
			ID:          uuid.New().String(),
			Type:        InsightTypeWorkflow,
			Title:       p.Name,
			Description: p.Description,
			Confidence:  p.Confidence,
			Impact:      ImpactLow,
			Actionable:  false,
			CreatedAt:   time.Now(),
			Data: map[string]interface{}{
				"pattern_id": p.ID,
				"frequency":  p.Frequency,
			},
		})
	}

	sort.Slice(insights, func(i, j int) bool { return insights[i].Confidence > insights[j].Confidence })
	return insights, nil
}

// GetProfile returns the current profile if it matches the user ID.
func (pe *PersonalizationEngine) GetProfile(ctx context.Context, userID string) (*UserProfile, error) {
	pe.mu.RLock()
	defer pe.mu.RUnlock()

	if pe.profile == nil {
		return nil, nil
	}
	if userID != "" && pe.profile.ID != userID {
		return nil, nil
	}
	return pe.profile, nil
}

// ProcessEvent updates personalization state from an event.
func (pe *PersonalizationEngine) ProcessEvent(ctx context.Context, event Event) {
	pe.mu.Lock()
	defer pe.mu.Unlock()

	if pe.recognizer != nil {
		pe.recognizer.ProcessEvent(ctx, event)
	}
	if pe.feedback != nil {
		pe.feedback.ProcessEvent(ctx, event)
	}
}

func (t EventType) String() string { return string(t) }
