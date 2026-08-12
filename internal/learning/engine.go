package learning

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Engine is the central learning system that coordinates all learning components.
type Engine struct {
	storage         Storage
	patterns        *PatternRecognizer
	strategy        *StrategyAdapter
	feedback        *FeedbackCollector
	knowledge       *KnowledgeBase
	personalization *PersonalizationEngine

	eventCh   chan Event
	stopCh    chan struct{}
	wg        sync.WaitGroup
	mu        sync.RWMutex
	isRunning bool
}

// Config configures the learning engine.
type Config struct {
	StorageDir          string
	EnableLearning      bool
	EnableTeamSharing   bool
	PrivacyMode         string // "strict", "normal", "permissive"
	RetentionDays       int
	MinConfidence       float64
	MaxPatterns         int
	MaxKnowledgeEntries int
	BufferSize          int
}

// DefaultConfig returns sensible defaults for the learning engine.
func DefaultConfig() Config {
	return Config{
		StorageDir:          ".automergent/learning",
		EnableLearning:      true,
		EnableTeamSharing:   false,
		PrivacyMode:         "strict",
		RetentionDays:       90,
		MinConfidence:       0.6,
		MaxPatterns:         10000,
		MaxKnowledgeEntries: 50000,
		BufferSize:          1000,
	}
}

// NewEngine creates a new learning engine.
func NewEngine(config Config, storage Storage) (*Engine, error) {
	if !config.EnableLearning {
		return &Engine{storage: storage}, nil
	}

	e := &Engine{
		storage:         storage,
		patterns:        NewPatternRecognizer(),
		strategy:        NewStrategyAdapter(storage),
		feedback:        NewFeedbackCollector(1000),
		knowledge:       NewKnowledgeBase(storage, config.MaxKnowledgeEntries),
		personalization: NewPersonalizationEngine(),
		eventCh:         make(chan Event, config.BufferSize),
		stopCh:          make(chan struct{}),
	}

	return e, nil
}

// Start begins processing learning events.
func (e *Engine) Start(ctx context.Context) error {
	e.mu.Lock()
	if e.isRunning {
		e.mu.Unlock()
		return nil
	}
	e.isRunning = true
	e.mu.Unlock()

	e.wg.Add(1)
	go e.processEvents(ctx)

	return nil
}

// Stop gracefully stops the learning engine.
func (e *Engine) Stop() error {
	e.mu.Lock()
	if !e.isRunning {
		e.mu.Unlock()
		return nil
	}
	e.isRunning = false
	e.mu.Unlock()

	close(e.stopCh)
	e.wg.Wait()

	return nil
}

// RecordEvent records a learning event for asynchronous processing.
func (e *Engine) RecordEvent(eventType EventType, data map[string]interface{}) {
	e.mu.RLock()
	if !e.isRunning {
		e.mu.RUnlock()
		return
	}
	e.mu.RUnlock()

	event := Event{
		ID:        uuid.New().String(),
		Timestamp: time.Now(),
		Type:      eventType,
		Data:      data,
	}

	select {
	case e.eventCh <- event:
	default:
		// Buffer full, drop event (non-blocking)
	}
}

// processEvents is the main event processing loop.
func (e *Engine) processEvents(ctx context.Context) {
	defer e.wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case <-e.stopCh:
			return
		case event := <-e.eventCh:
			e.handleEvent(ctx, event)
		}
	}
}

// handleEvent processes a single learning event.
func (e *Engine) handleEvent(ctx context.Context, event Event) {
	// Pattern recognition
	if e.patterns != nil {
		e.patterns.RecordSession(event.SessionID)
	}

	// Strategy adaptation
	if e.strategy != nil {
		e.strategy.ProcessEvent(ctx, event)
	}

	// Feedback collection
	if e.feedback != nil {
		e.feedback.ProcessEvent(ctx, event)
	}

	// Knowledge extraction
	if e.knowledge != nil {
		e.knowledge.ProcessEvent(ctx, event)
	}

	// Personalization
	if e.personalization != nil {
		e.personalization.ProcessEvent(ctx, event)
	}

	// Persist event for historical analysis
	if err := e.storage.SaveEvent(ctx, event); err != nil {
		// Log error but don't fail
	}
}

// GetPatterns retrieves learned patterns matching the criteria.
func (e *Engine) GetPatterns(ctx context.Context, patternType PatternType) ([]Pattern, error) {
	if e.patterns == nil {
		return nil, nil
	}
	pats := e.patterns.GetPatterns(patternType)
	out := make([]Pattern, len(pats))
	for i, p := range pats {
		out[i] = *p
	}
	return out, nil
}

// GetStrategy retrieves the best strategy for the given context.
func (e *Engine) GetStrategy(ctx context.Context, projectType, framework string) (*Strategy, error) {
	if e.strategy == nil {
		return nil, nil
	}
	return &Strategy{ProjectType: projectType, Framework: framework}, nil
}

// RecordFeedback records user feedback.
func (e *Engine) RecordFeedback(ctx context.Context, feedback Feedback) error {
	if e.feedback == nil {
		return nil
	}
	return e.feedback.Record(ctx, feedback)
}

// GetKnowledge retrieves knowledge entries by scope and category.
func (e *Engine) GetKnowledge(ctx context.Context, scope KnowledgeScope, category KnowledgeCategory) ([]KnowledgeEntry, error) {
	if e.knowledge == nil {
		return nil, nil
	}
	return e.knowledge.Get(ctx, scope, category)
}

// GetUserProfile retrieves the learned user profile.
func (e *Engine) GetUserProfile(ctx context.Context, userID string) (*UserProfile, error) {
	if e.personalization == nil {
		return nil, nil
	}
	return e.personalization.GetProfile(ctx, userID)
}

// GetProjectContext retrieves learned project context.
func (e *Engine) GetProjectContext(ctx context.Context, projectPath string) (*ProjectContext, error) {
	if e.knowledge == nil {
		return nil, nil
	}
	return e.knowledge.GetProjectContext(ctx, projectPath)
}

// GetInsights generates actionable insights from learning data.
func (e *Engine) GetInsights(ctx context.Context) ([]Insight, error) {
	insights := []Insight{}

	// Gather insights from each component
	if e.patterns != nil {
		patternInsights, err := e.patterns.GenerateInsights(ctx)
		if err == nil {
			insights = append(insights, patternInsights...)
		}
	}

	if e.strategy != nil {
		strategyInsights, err := e.strategy.GenerateInsights(ctx)
		if err == nil {
			insights = append(insights, strategyInsights...)
		}
	}

	if e.feedback != nil {
		feedbackInsights, err := e.feedback.GenerateInsights(ctx)
		if err == nil {
			insights = append(insights, feedbackInsights...)
		}
	}

	return insights, nil
}

// RecordTaskOutcome records the outcome of a task execution.
func (e *Engine) RecordTaskOutcome(ctx context.Context, outcome TaskOutcome) error {
	eventType := EventTypeTaskComplete
	if !outcome.Success {
		eventType = EventTypeTaskFail
	}
	e.RecordEvent(eventType, map[string]interface{}{
		"task_id":        outcome.TaskID,
		"success":        outcome.Success,
		"duration":       outcome.Duration,
		"strategy_used":  outcome.StrategyUsed,
		"tools_used":     outcome.ToolsUsed,
		"files_accessed": outcome.FilesAccessed,
		"error_type":     outcome.ErrorType,
		"error_message":  outcome.ErrorMessage,
	})

	// Update strategy success rates
	// Store for historical analysis
	return e.storage.SaveTaskOutcome(ctx, outcome)
}

// SuggestStrategy suggests the best strategy for a task based on learning.
func (e *Engine) SuggestStrategy(ctx context.Context, taskDescription string, projectCtx *ProjectContext) (*Strategy, float64, error) {
	if e.strategy == nil {
		return nil, 0, nil
	}

	return e.strategy.SuggestStrategy(ctx, taskDescription, projectCtx)
}

// OptimizeProviderSelection suggests the best provider based on task and history.
func (e *Engine) OptimizeProviderSelection(ctx context.Context, taskType string, budget float64) (string, error) {
	if e.strategy == nil {
		return "", nil
	}

	return e.strategy.OptimizeProvider(ctx, taskType, budget)
}
