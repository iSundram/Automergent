package strategy

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/iSundram/Automergent/internal/learning"
)

// Adapter learns and adapts strategies based on task outcomes.
type Adapter struct {
	storage    learning.Storage
	strategies map[string]*learning.Strategy
	metrics    map[string]*strategyMetrics
	mu         sync.RWMutex
}

// strategyMetrics tracks real-time metrics for a strategy.
type strategyMetrics struct {
	successes     int
	failures      int
	totalDuration time.Duration
	executions    int
	lastUsed      time.Time
}

// NewAdapter creates a new strategy adapter.
func NewAdapter(storage learning.Storage) *Adapter {
	a := &Adapter{
		storage:    storage,
		strategies: make(map[string]*learning.Strategy),
		metrics:    make(map[string]*strategyMetrics),
	}

	// Initialize with default strategies
	a.initializeDefaultStrategies()

	return a
}

// initializeDefaultStrategies creates baseline strategies for common scenarios.
func (a *Adapter) initializeDefaultStrategies() {
	defaultStrategies := []*learning.Strategy{
		{
			ID:          "web-nextjs-default",
			Name:        "Next.js Web Application",
			Description: "Default strategy for Next.js web projects",
			ProjectType: "web",
			Framework:   "nextjs",
			SuccessRate: 0.75,
			AvgDuration: 5 * time.Second,
			UseCount:    0,
			Configuration: map[string]interface{}{
				"prefer_typescript":  true,
				"test_framework":     "jest",
				"preferred_provider": "google",
				"context_files":      []string{"package.json", "tsconfig.json", "next.config.js"},
			},
		},
		{
			ID:          "cli-go-default",
			Name:        "Go CLI Application",
			Description: "Default strategy for Go CLI projects",
			ProjectType: "cli",
			Framework:   "go",
			SuccessRate: 0.80,
			AvgDuration: 3 * time.Second,
			UseCount:    0,
			Configuration: map[string]interface{}{
				"test_framework":     "testing",
				"preferred_provider": "google",
				"context_files":      []string{"go.mod", "go.sum", "Makefile"},
			},
		},
		{
			ID:          "library-python-default",
			Name:        "Python Library",
			Description: "Default strategy for Python library projects",
			ProjectType: "library",
			Framework:   "python",
			SuccessRate: 0.70,
			AvgDuration: 4 * time.Second,
			UseCount:    0,
			Configuration: map[string]interface{}{
				"test_framework":     "pytest",
				"preferred_provider": "google",
				"context_files":      []string{"setup.py", "pyproject.toml", "requirements.txt"},
			},
		},
		{
			ID:          "api-django-default",
			Name:        "Django API",
			Description: "Default strategy for Django API projects",
			ProjectType: "api",
			Framework:   "django",
			SuccessRate: 0.72,
			AvgDuration: 6 * time.Second,
			UseCount:    0,
			Configuration: map[string]interface{}{
				"test_framework":     "pytest-django",
				"preferred_provider": "google",
				"context_files":      []string{"manage.py", "settings.py", "urls.py"},
			},
		},
	}

	for _, s := range defaultStrategies {
		a.strategies[s.ID] = s
	}
}

// ProcessEvent processes an event to adapt strategies.
func (a *Adapter) ProcessEvent(ctx context.Context, event learning.Event) {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Track strategy switches
	if event.Type == learning.EventTypeStrategySwitch {
		a.handleStrategySwitch(ctx, event)
	}

	// Track task outcomes
	if event.Type == learning.EventTypeTaskComplete || event.Type == learning.EventTypeTaskFail {
		a.handleTaskOutcome(ctx, event)
	}
}

// handleStrategySwitch records when a strategy is switched.
func (a *Adapter) handleStrategySwitch(ctx context.Context, event learning.Event) {
	fromStrategy, _ := event.Data["from_strategy"].(string)
	toStrategy, _ := event.Data["to_strategy"].(string)
	reason, _ := event.Data["reason"].(string)

	// If a strategy was manually switched, it might indicate the previous one wasn't optimal
	if fromStrategy != "" {
		if _, exists := a.metrics[fromStrategy]; exists {
			// Slightly decrease confidence in the from strategy
			if strategy, ok := a.strategies[fromStrategy]; ok {
				strategy.SuccessRate *= 0.95
			}
		}
	}

	// Log the switch for analysis
	_ = reason // Could be used for more sophisticated learning
	_ = toStrategy
}

// handleTaskOutcome updates strategy metrics based on task outcome.
func (a *Adapter) handleTaskOutcome(ctx context.Context, event learning.Event) {
	strategyUsed, ok := event.Data["strategy_used"].(string)
	if !ok || strategyUsed == "" {
		return
	}

	success := event.Type == learning.EventTypeTaskComplete
	duration, _ := event.Data["duration"].(time.Duration)

	// Update metrics
	metrics, exists := a.metrics[strategyUsed]
	if !exists {
		metrics = &strategyMetrics{}
		a.metrics[strategyUsed] = metrics
	}

	metrics.executions++
	metrics.lastUsed = time.Now()
	metrics.totalDuration += duration

	if success {
		metrics.successes++
	} else {
		metrics.failures++
	}

	// Update strategy
	if strategy, ok := a.strategies[strategyUsed]; ok {
		strategy.UseCount = metrics.executions
		strategy.LastUsed = metrics.lastUsed
		strategy.SuccessRate = float64(metrics.successes) / float64(metrics.executions)
		strategy.AvgDuration = metrics.totalDuration / time.Duration(metrics.executions)

		// Persist updated strategy
		a.storage.SaveStrategy(ctx, *strategy)
	}
}

// UpdateStrategyMetrics updates metrics for a strategy from a task outcome.
func (a *Adapter) UpdateStrategyMetrics(ctx context.Context, outcome learning.TaskOutcome) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if outcome.StrategyUsed == "" {
		return
	}

	metrics, exists := a.metrics[outcome.StrategyUsed]
	if !exists {
		metrics = &strategyMetrics{}
		a.metrics[outcome.StrategyUsed] = metrics
	}

	metrics.executions++
	metrics.lastUsed = outcome.EndTime
	metrics.totalDuration += outcome.Duration

	if outcome.Success {
		metrics.successes++
	} else {
		metrics.failures++
	}

	// Update strategy
	if strategy, ok := a.strategies[outcome.StrategyUsed]; ok {
		strategy.UseCount = metrics.executions
		strategy.LastUsed = metrics.lastUsed
		strategy.SuccessRate = float64(metrics.successes) / float64(metrics.executions)
		strategy.AvgDuration = metrics.totalDuration / time.Duration(metrics.executions)

		a.storage.SaveStrategy(ctx, *strategy)
	}
}

// GetBestStrategy retrieves the best strategy for a project type and framework.
func (a *Adapter) GetBestStrategy(ctx context.Context, projectType, framework string) (*learning.Strategy, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	// Load strategies from storage
	strategies, err := a.storage.GetStrategies(ctx)
	if err != nil {
		// Fall back to in-memory strategies
		strategies = []learning.Strategy{}
		for _, s := range a.strategies {
			strategies = append(strategies, *s)
		}
	}

	// Filter matching strategies
	candidates := []learning.Strategy{}
	for _, s := range strategies {
		if s.ProjectType == projectType {
			if framework == "" || s.Framework == framework {
				candidates = append(candidates, s)
			}
		}
	}

	if len(candidates) == 0 {
		return nil, fmt.Errorf("no strategy found for project type: %s, framework: %s", projectType, framework)
	}

	// Score strategies using multi-factor ranking
	type scoredStrategy struct {
		strategy learning.Strategy
		score    float64
	}

	scored := []scoredStrategy{}
	for _, s := range candidates {
		score := a.scoreStrategy(s)
		scored = append(scored, scoredStrategy{strategy: s, score: score})
	}

	// Sort by score descending
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	return &scored[0].strategy, nil
}

// scoreStrategy calculates a composite score for a strategy.
func (a *Adapter) scoreStrategy(s learning.Strategy) float64 {
	// Success rate (40% weight)
	successScore := s.SuccessRate * 0.4

	// Recency (20% weight) - prefer recently used strategies
	daysSinceUse := time.Since(s.LastUsed).Hours() / 24.0
	recencyScore := (1.0 / (1.0 + daysSinceUse/30.0)) * 0.2

	// Experience (20% weight) - prefer well-tested strategies
	experienceScore := math.Min(float64(s.UseCount)/100.0, 1.0) * 0.2

	// Speed (20% weight) - prefer faster strategies
	speedScore := 0.0
	if s.AvgDuration > 0 {
		// Normalize: 1.0 for <1s, decreasing logarithmically
		seconds := s.AvgDuration.Seconds()
		speedScore = math.Max(0, 1.0-math.Log10(seconds+1)/2) * 0.2
	}

	return successScore + recencyScore + experienceScore + speedScore
}

// SuggestStrategy suggests the best strategy for a task based on learning.
func (a *Adapter) SuggestStrategy(ctx context.Context, taskDescription string, projectCtx *learning.ProjectContext) (*learning.Strategy, float64, error) {
	if projectCtx == nil {
		return nil, 0, fmt.Errorf("project context required")
	}

	strategy, err := a.GetBestStrategy(ctx, projectCtx.Type, projectCtx.Framework)
	if err != nil {
		return nil, 0, err
	}

	// Calculate confidence in this suggestion
	confidence := a.scoreStrategy(*strategy)

	return strategy, confidence, nil
}

// OptimizeProvider suggests the best provider based on task type and history.
func (a *Adapter) OptimizeProvider(ctx context.Context, taskType string, budget float64) (string, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	// Analyze historical provider performance
	providerScores := make(map[string]float64)

	// Get task outcomes to analyze provider performance
	outcomes, err := a.storage.GetTaskOutcomes(ctx, learning.TaskOutcomeFilters{Limit: 100})
	if err == nil {
		for _, outcome := range outcomes {
			// Extract provider from metadata
			provider, ok := outcome.Metadata["provider"].(string)
			if !ok {
				continue
			}

			// Calculate cost
			cost := float64(outcome.TokensUsed.InputTokens)*0.000003 +
				float64(outcome.TokensUsed.OutputTokens)*0.000015

			// Score based on success, speed, and cost
			score := 0.0
			if outcome.Success {
				score += 50.0
			}

			// Speed bonus (faster is better)
			if outcome.Duration > 0 {
				speedBonus := 20.0 / (outcome.Duration.Seconds() + 1)
				score += speedBonus
			}

			// Cost penalty (cheaper is better)
			if budget > 0 && cost > 0 {
				costRatio := cost / budget
				score -= costRatio * 30.0
			}

			providerScores[provider] += score
		}
	}

	// Find best provider
	bestProvider := ""
	bestScore := -math.MaxFloat64

	for provider, score := range providerScores {
		if score > bestScore {
			bestScore = score
			bestProvider = provider
		}
	}

	if bestProvider == "" {
		// Default to google if no data
		return "google", nil
	}

	return bestProvider, nil
}

// GenerateInsights generates strategic insights.
func (a *Adapter) GenerateInsights(ctx context.Context) ([]learning.Insight, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	insights := []learning.Insight{}

	// Find underperforming strategies
	for _, strategy := range a.strategies {
		if strategy.UseCount >= 10 && strategy.SuccessRate < 0.6 {
			insights = append(insights, learning.Insight{
				ID:          uuid.New().String(),
				Type:        learning.InsightTypeQuality,
				Title:       fmt.Sprintf("Low Success Rate: %s", strategy.Name),
				Description: fmt.Sprintf("Success rate is %.1f%% over %d uses", strategy.SuccessRate*100, strategy.UseCount),
				Confidence:  0.9,
				Impact:      learning.ImpactHigh,
				Actionable:  true,
				Action:      "Consider reviewing or replacing this strategy",
				CreatedAt:   time.Now(),
				Data: map[string]interface{}{
					"strategy_id":  strategy.ID,
					"success_rate": strategy.SuccessRate,
					"use_count":    strategy.UseCount,
				},
			})
		}
	}

	// Analyze provider cost efficiency
	outcomes, err := a.storage.GetTaskOutcomes(ctx, learning.TaskOutcomeFilters{Limit: 100})
	if err == nil {
		providerCosts := make(map[string]float64)
		providerCount := make(map[string]int)

		for _, outcome := range outcomes {
			provider, ok := outcome.Metadata["provider"].(string)
			if !ok {
				continue
			}

			cost := float64(outcome.TokensUsed.InputTokens)*0.000003 +
				float64(outcome.TokensUsed.OutputTokens)*0.000015

			providerCosts[provider] += cost
			providerCount[provider]++
		}

		// Find cost optimization opportunities
		if len(providerCosts) > 1 {
			// Find cheapest and most expensive
			cheapest := ""
			expensive := ""
			minCost := math.MaxFloat64
			maxCost := 0.0

			for provider, totalCost := range providerCosts {
				avgCost := totalCost / float64(providerCount[provider])
				if avgCost < minCost {
					minCost = avgCost
					cheapest = provider
				}
				if avgCost > maxCost {
					maxCost = avgCost
					expensive = provider
				}
			}

			if maxCost > minCost*1.5 {
				savings := (maxCost - minCost) * float64(providerCount[expensive])
				insights = append(insights, learning.Insight{
					ID:          uuid.New().String(),
					Type:        learning.InsightTypeCostOptimization,
					Title:       "Provider Cost Optimization Available",
					Description: fmt.Sprintf("Switching from %s to %s could save $%.2f", expensive, cheapest, savings),
					Confidence:  0.85,
					Impact:      learning.ImpactMedium,
					Actionable:  true,
					Action:      fmt.Sprintf("Consider using %s for cost-sensitive tasks", cheapest),
					CreatedAt:   time.Now(),
					Data: map[string]interface{}{
						"expensive_provider": expensive,
						"cheap_provider":     cheapest,
						"potential_savings":  savings,
					},
				})
			}
		}
	}

	return insights, nil
}

// LearnFromOutcome adapts strategies based on task outcome.
func (a *Adapter) LearnFromOutcome(ctx context.Context, outcome learning.TaskOutcome) {
	a.mu.Lock()
	defer a.mu.Unlock()

	// If task failed, try to create an improved strategy
	if !outcome.Success && outcome.StrategyUsed != "" {
		if strategy, ok := a.strategies[outcome.StrategyUsed]; ok {
			// Create variation of strategy with adjusted parameters
			newStrategy := *strategy
			newStrategy.ID = uuid.New().String()
			newStrategy.Name = strategy.Name + " (Adapted)"
			newStrategy.UseCount = 0

			// Adjust configuration based on failure type
			if outcome.ErrorType != "" {
				if strings.Contains(outcome.ErrorType, "timeout") {
					// Increase timeout for next attempt
					if config, ok := newStrategy.Configuration["timeout"].(int); ok {
						newStrategy.Configuration["timeout"] = config * 2
					}
				} else if strings.Contains(outcome.ErrorType, "context") {
					// Expand context for next attempt
					newStrategy.Configuration["expand_context"] = true
				}
			}

			a.strategies[newStrategy.ID] = &newStrategy
			a.storage.SaveStrategy(ctx, newStrategy)
		}
	}
}
