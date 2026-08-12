package learning

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Storage defines the interface for persisting learning data.
type Storage interface {
	// Events
	SaveEvent(ctx context.Context, event Event) error
	GetEvents(ctx context.Context, filters EventFilters) ([]Event, error)

	// Patterns
	SavePattern(ctx context.Context, pattern Pattern) error
	GetPatterns(ctx context.Context, patternType PatternType) ([]Pattern, error)
	DeletePattern(ctx context.Context, id string) error

	// Strategies
	SaveStrategy(ctx context.Context, strategy Strategy) error
	GetStrategies(ctx context.Context) ([]Strategy, error)
	GetStrategy(ctx context.Context, id string) (*Strategy, error)
	UpdateStrategyMetrics(ctx context.Context, id string, metrics StrategyMetrics) error

	// Feedback
	SaveFeedback(ctx context.Context, feedback Feedback) error
	GetFeedback(ctx context.Context, filters FeedbackFilters) ([]Feedback, error)

	// Knowledge
	SaveKnowledge(ctx context.Context, entry KnowledgeEntry) error
	GetKnowledge(ctx context.Context, scope KnowledgeScope, category KnowledgeCategory) ([]KnowledgeEntry, error)
	DeleteKnowledge(ctx context.Context, id string) error

	// User Profiles
	SaveUserProfile(ctx context.Context, profile UserProfile) error
	GetUserProfile(ctx context.Context, userID string) (*UserProfile, error)

	// Project Context
	SaveProjectContext(ctx context.Context, ctx2 ProjectContext) error
	GetProjectContext(ctx context.Context, path string) (*ProjectContext, error)

	// Task Outcomes
	SaveTaskOutcome(ctx context.Context, outcome TaskOutcome) error
	GetTaskOutcomes(ctx context.Context, filters TaskOutcomeFilters) ([]TaskOutcome, error)

	// Metrics
	SaveMetric(ctx context.Context, metric Metric) error
	GetMetrics(ctx context.Context, name string, start, end time.Time) ([]Metric, error)

	// Cleanup
	Prune(ctx context.Context, maxAge time.Duration) error
}

// EventFilters defines filters for querying events.
type EventFilters struct {
	SessionID string
	Type      EventType
	StartTime time.Time
	EndTime   time.Time
	Limit     int
}

// FeedbackFilters defines filters for querying feedback.
type FeedbackFilters struct {
	SessionID string
	Type      FeedbackType
	Sentiment SentimentType
	MinRating int
	StartTime time.Time
	EndTime   time.Time
	Limit     int
}

// TaskOutcomeFilters defines filters for querying task outcomes.
type TaskOutcomeFilters struct {
	SessionID    string
	Success      *bool
	StrategyUsed string
	StartTime    time.Time
	EndTime      time.Time
	Limit        int
}

// StrategyMetrics contains metrics to update for a strategy.
type StrategyMetrics struct {
	SuccessRate float64
	AvgDuration time.Duration
	UseCount    int
}

// FileStorage implements Storage using local filesystem with JSON files.
type FileStorage struct {
	baseDir string
	mu      sync.RWMutex
}

// NewFileStorage creates a new file-based storage.
func NewFileStorage(baseDir string) (*FileStorage, error) {
	dirs := []string{
		"events",
		"patterns",
		"strategies",
		"feedback",
		"knowledge",
		"profiles",
		"projects",
		"outcomes",
		"metrics",
	}

	for _, dir := range dirs {
		path := filepath.Join(baseDir, dir)
		if err := os.MkdirAll(path, 0o700); err != nil {
			return nil, fmt.Errorf("create dir %s: %w", dir, err)
		}
	}

	return &FileStorage{baseDir: baseDir}, nil
}

// SaveEvent saves an event to disk.
func (s *FileStorage) SaveEvent(ctx context.Context, event Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	date := event.Timestamp.Format("2006-01-02")
	dir := filepath.Join(s.baseDir, "events", date)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}

	path := filepath.Join(dir, event.ID+".json")
	return s.writeJSON(path, event)
}

// GetEvents retrieves events matching the filters.
func (s *FileStorage) GetEvents(ctx context.Context, filters EventFilters) ([]Event, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var events []Event
	eventsDir := filepath.Join(s.baseDir, "events")

	err := filepath.Walk(eventsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || filepath.Ext(path) != ".json" {
			return nil
		}

		var event Event
		if err := s.readJSON(path, &event); err != nil {
			return nil // Skip invalid files
		}

		// Apply filters
		if filters.SessionID != "" && event.SessionID != filters.SessionID {
			return nil
		}
		if filters.Type != "" && event.Type != filters.Type {
			return nil
		}
		if !filters.StartTime.IsZero() && event.Timestamp.Before(filters.StartTime) {
			return nil
		}
		if !filters.EndTime.IsZero() && event.Timestamp.After(filters.EndTime) {
			return nil
		}

		events = append(events, event)

		if filters.Limit > 0 && len(events) >= filters.Limit {
			return filepath.SkipDir
		}

		return nil
	})

	return events, err
}

// SavePattern saves a pattern to disk.
func (s *FileStorage) SavePattern(ctx context.Context, pattern Pattern) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	path := filepath.Join(s.baseDir, "patterns", pattern.ID+".json")
	return s.writeJSON(path, pattern)
}

// GetPatterns retrieves patterns of a specific type.
func (s *FileStorage) GetPatterns(ctx context.Context, patternType PatternType) ([]Pattern, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var patterns []Pattern
	patternsDir := filepath.Join(s.baseDir, "patterns")

	entries, err := os.ReadDir(patternsDir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		var pattern Pattern
		path := filepath.Join(patternsDir, entry.Name())
		if err := s.readJSON(path, &pattern); err != nil {
			continue
		}

		if patternType == "" || pattern.Type == patternType {
			patterns = append(patterns, pattern)
		}
	}

	return patterns, nil
}

// DeletePattern removes a pattern from storage.
func (s *FileStorage) DeletePattern(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	path := filepath.Join(s.baseDir, "patterns", id+".json")
	return os.Remove(path)
}

// SaveStrategy saves a strategy to disk.
func (s *FileStorage) SaveStrategy(ctx context.Context, strategy Strategy) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	path := filepath.Join(s.baseDir, "strategies", strategy.ID+".json")
	return s.writeJSON(path, strategy)
}

// GetStrategies retrieves all strategies.
func (s *FileStorage) GetStrategies(ctx context.Context) ([]Strategy, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var strategies []Strategy
	strategiesDir := filepath.Join(s.baseDir, "strategies")

	entries, err := os.ReadDir(strategiesDir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		var strategy Strategy
		path := filepath.Join(strategiesDir, entry.Name())
		if err := s.readJSON(path, &strategy); err != nil {
			continue
		}

		strategies = append(strategies, strategy)
	}

	return strategies, nil
}

// GetStrategy retrieves a specific strategy.
func (s *FileStorage) GetStrategy(ctx context.Context, id string) (*Strategy, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var strategy Strategy
	path := filepath.Join(s.baseDir, "strategies", id+".json")
	if err := s.readJSON(path, &strategy); err != nil {
		return nil, err
	}

	return &strategy, nil
}

// UpdateStrategyMetrics updates metrics for a strategy.
func (s *FileStorage) UpdateStrategyMetrics(ctx context.Context, id string, metrics StrategyMetrics) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	strategy, err := s.GetStrategy(ctx, id)
	if err != nil {
		return err
	}

	strategy.SuccessRate = metrics.SuccessRate
	strategy.AvgDuration = metrics.AvgDuration
	strategy.UseCount = metrics.UseCount
	strategy.LastUsed = time.Now()

	return s.SaveStrategy(ctx, *strategy)
}

// SaveFeedback saves feedback to disk.
func (s *FileStorage) SaveFeedback(ctx context.Context, feedback Feedback) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	path := filepath.Join(s.baseDir, "feedback", feedback.ID+".json")
	return s.writeJSON(path, feedback)
}

// GetFeedback retrieves feedback matching filters.
func (s *FileStorage) GetFeedback(ctx context.Context, filters FeedbackFilters) ([]Feedback, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var feedbacks []Feedback
	feedbackDir := filepath.Join(s.baseDir, "feedback")

	entries, err := os.ReadDir(feedbackDir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		var feedback Feedback
		path := filepath.Join(feedbackDir, entry.Name())
		if err := s.readJSON(path, &feedback); err != nil {
			continue
		}

		// Apply filters
		if filters.SessionID != "" && feedback.SessionID != filters.SessionID {
			continue
		}
		if filters.Type != "" && feedback.Type != filters.Type {
			continue
		}
		if filters.Sentiment != "" && feedback.Sentiment != filters.Sentiment {
			continue
		}
		if filters.MinRating > 0 && feedback.Rating < filters.MinRating {
			continue
		}

		feedbacks = append(feedbacks, feedback)

		if filters.Limit > 0 && len(feedbacks) >= filters.Limit {
			break
		}
	}

	return feedbacks, nil
}

// SaveKnowledge saves a knowledge entry.
func (s *FileStorage) SaveKnowledge(ctx context.Context, entry KnowledgeEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	path := filepath.Join(s.baseDir, "knowledge", entry.ID+".json")
	return s.writeJSON(path, entry)
}

// GetKnowledge retrieves knowledge entries.
func (s *FileStorage) GetKnowledge(ctx context.Context, scope KnowledgeScope, category KnowledgeCategory) ([]KnowledgeEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var entries []KnowledgeEntry
	knowledgeDir := filepath.Join(s.baseDir, "knowledge")

	dirEntries, err := os.ReadDir(knowledgeDir)
	if err != nil {
		return nil, err
	}

	for _, dirEntry := range dirEntries {
		if dirEntry.IsDir() || filepath.Ext(dirEntry.Name()) != ".json" {
			continue
		}

		var entry KnowledgeEntry
		path := filepath.Join(knowledgeDir, dirEntry.Name())
		if err := s.readJSON(path, &entry); err != nil {
			continue
		}

		if (scope == "" || entry.Scope == scope) &&
			(category == "" || entry.Category == category) {
			entries = append(entries, entry)
		}
	}

	return entries, nil
}

// DeleteKnowledge removes a knowledge entry.
func (s *FileStorage) DeleteKnowledge(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	path := filepath.Join(s.baseDir, "knowledge", id+".json")
	return os.Remove(path)
}

// SaveUserProfile saves a user profile.
func (s *FileStorage) SaveUserProfile(ctx context.Context, profile UserProfile) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	path := filepath.Join(s.baseDir, "profiles", profile.UserID+".json")
	return s.writeJSON(path, profile)
}

// GetUserProfile retrieves a user profile.
func (s *FileStorage) GetUserProfile(ctx context.Context, userID string) (*UserProfile, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var profile UserProfile
	path := filepath.Join(s.baseDir, "profiles", userID+".json")
	if err := s.readJSON(path, &profile); err != nil {
		if os.IsNotExist(err) {
			// Return default profile
			return &UserProfile{
				UserID:      userID,
				ID:          userID,
				Preferences: UserPreferences{TechnicalLevel: "expert"},
				Stats:       ProfileStats{},
			}, nil
		}
		return nil, err
	}

	return &profile, nil
}

// SaveProjectContext saves project context.
func (s *FileStorage) SaveProjectContext(ctx context.Context, projectCtx ProjectContext) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Use sanitized path as filename
	filename := filepath.Base(projectCtx.Path) + ".json"
	path := filepath.Join(s.baseDir, "projects", filename)
	return s.writeJSON(path, projectCtx)
}

// GetProjectContext retrieves project context.
func (s *FileStorage) GetProjectContext(ctx context.Context, projectPath string) (*ProjectContext, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	filename := filepath.Base(projectPath) + ".json"
	path := filepath.Join(s.baseDir, "projects", filename)

	var projectCtx ProjectContext
	if err := s.readJSON(path, &projectCtx); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	return &projectCtx, nil
}

// SaveTaskOutcome saves a task outcome.
func (s *FileStorage) SaveTaskOutcome(ctx context.Context, outcome TaskOutcome) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	date := outcome.StartTime.Format("2006-01-02")
	dir := filepath.Join(s.baseDir, "outcomes", date)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}

	path := filepath.Join(dir, outcome.TaskID+".json")
	return s.writeJSON(path, outcome)
}

// GetTaskOutcomes retrieves task outcomes matching filters.
func (s *FileStorage) GetTaskOutcomes(ctx context.Context, filters TaskOutcomeFilters) ([]TaskOutcome, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var outcomes []TaskOutcome
	outcomesDir := filepath.Join(s.baseDir, "outcomes")

	err := filepath.Walk(outcomesDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || filepath.Ext(path) != ".json" {
			return nil
		}

		var outcome TaskOutcome
		if err := s.readJSON(path, &outcome); err != nil {
			return nil
		}

		// Apply filters
		if filters.SessionID != "" && outcome.SessionID != filters.SessionID {
			return nil
		}
		if filters.Success != nil && outcome.Success != *filters.Success {
			return nil
		}
		if filters.StrategyUsed != "" && outcome.StrategyUsed != filters.StrategyUsed {
			return nil
		}

		outcomes = append(outcomes, outcome)

		if filters.Limit > 0 && len(outcomes) >= filters.Limit {
			return filepath.SkipDir
		}

		return nil
	})

	return outcomes, err
}

// SaveMetric saves a metric.
func (s *FileStorage) SaveMetric(ctx context.Context, metric Metric) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	date := metric.Timestamp.Format("2006-01-02")
	dir := filepath.Join(s.baseDir, "metrics", metric.Name, date)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}

	filename := fmt.Sprintf("%d.json", metric.Timestamp.UnixNano())
	path := filepath.Join(dir, filename)
	return s.writeJSON(path, metric)
}

// GetMetrics retrieves metrics for a time range.
func (s *FileStorage) GetMetrics(ctx context.Context, name string, start, end time.Time) ([]Metric, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var metrics []Metric
	metricsDir := filepath.Join(s.baseDir, "metrics", name)

	err := filepath.Walk(metricsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || filepath.Ext(path) != ".json" {
			return nil
		}

		var metric Metric
		if err := s.readJSON(path, &metric); err != nil {
			return nil
		}

		if (start.IsZero() || metric.Timestamp.After(start)) &&
			(end.IsZero() || metric.Timestamp.Before(end)) {
			metrics = append(metrics, metric)
		}

		return nil
	})

	return metrics, err
}

// Prune removes old data beyond maxAge.
func (s *FileStorage) Prune(ctx context.Context, maxAge time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	cutoff := time.Now().Add(-maxAge)

	// Prune events
	eventsDir := filepath.Join(s.baseDir, "events")
	if err := s.pruneDirectory(eventsDir, cutoff); err != nil {
		return err
	}

	// Prune outcomes
	outcomesDir := filepath.Join(s.baseDir, "outcomes")
	if err := s.pruneDirectory(outcomesDir, cutoff); err != nil {
		return err
	}

	// Prune metrics
	metricsDir := filepath.Join(s.baseDir, "metrics")
	return s.pruneDirectory(metricsDir, cutoff)
}

// pruneDirectory removes files older than cutoff.
func (s *FileStorage) pruneDirectory(dir string, cutoff time.Time) error {
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}

		if info.ModTime().Before(cutoff) {
			os.Remove(path)
		}

		return nil
	})
}

// writeJSON writes data as JSON to a file atomically.
func (s *FileStorage) writeJSON(path string, data interface{}) error {
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}

	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".automergent-learning-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() {
		tmp.Close()
		os.Remove(tmpPath)
	}()

	if _, err := tmp.Write(jsonData); err != nil {
		return err
	}

	if err := tmp.Sync(); err != nil {
		return err
	}

	tmp.Close()

	if err := os.Chmod(tmpPath, 0o600); err != nil {
		return err
	}

	return os.Rename(tmpPath, path)
}

// readJSON reads JSON data from a file.
func (s *FileStorage) readJSON(path string, v interface{}) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, v)
}
