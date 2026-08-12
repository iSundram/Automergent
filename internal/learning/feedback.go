package learning

import (
	"sync"
	"time"
)

// FeedbackType categorizes feedback signals.
type FeedbackType string

const (
	FeedbackTypeAccept   FeedbackType = "accept"   // User accepted suggestion
	FeedbackTypeReject   FeedbackType = "reject"   // User rejected suggestion
	FeedbackTypeModify   FeedbackType = "modify"   // User modified output
	FeedbackTypeUndo     FeedbackType = "undo"     // User undid action
	FeedbackTypeRating   FeedbackType = "rating"   // Explicit rating
	FeedbackTypeCorrect  FeedbackType = "correct"  // User corrected output
	FeedbackTypePrefer   FeedbackType = "prefer"   // User stated preference
	FeedbackTypeSkip     FeedbackType = "skip"     // User skipped suggestion
	FeedbackTypeRetry    FeedbackType = "retry"    // User requested retry
	FeedbackTypeImplicit FeedbackType = "implicit" // Inferred from behavior
)

// Feedback represents a single feedback event.
type Feedback struct {
	ID         string                 `json:"id"`
	Type       FeedbackType           `json:"type"`
	Sentiment  SentimentType          `json:"sentiment,omitempty"`
	Rating     int                    `json:"rating,omitempty"`
	Timestamp  time.Time              `json:"timestamp"`
	SessionID  string                 `json:"session_id,omitempty"`
	Context    string                 `json:"context,omitempty"`     // What was happening
	Target     string                 `json:"target,omitempty"`      // What received feedback
	TargetType string                 `json:"target_type,omitempty"` // "suggestion", "tool", "response"
	Value      interface{}            `json:"value,omitempty"`       // Rating value, correction, etc.
	Signal     float64                `json:"signal"`                // -1.0 to 1.0
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

// SentimentType represents user satisfaction.
type SentimentType string

const (
	SentimentPositive SentimentType = "positive"
	SentimentNeutral  SentimentType = "neutral"
	SentimentNegative SentimentType = "negative"
)

// FeedbackAggregation holds aggregated feedback for a target.
type FeedbackAggregation struct {
	Target        string    `json:"target"`
	TargetType    string    `json:"target_type"`
	TotalCount    int       `json:"total_count"`
	AcceptCount   int       `json:"accept_count"`
	RejectCount   int       `json:"reject_count"`
	ModifyCount   int       `json:"modify_count"`
	AverageRating float64   `json:"average_rating"`
	LastFeedback  time.Time `json:"last_feedback"`
	NetSignal     float64   `json:"net_signal"` // cumulative signal
}

// FeedbackCollector collects and processes user feedback.
type FeedbackCollector struct {
	mu         sync.RWMutex
	feedback   []*Feedback
	aggregates map[string]*FeedbackAggregation
	maxHistory int
	learner    *Learner
	sessionID  string
}

// NewFeedbackCollector creates a new feedback collector.
func NewFeedbackCollector(maxHistory int) *FeedbackCollector {
	return &FeedbackCollector{
		feedback:   make([]*Feedback, 0),
		aggregates: make(map[string]*FeedbackAggregation),
		maxHistory: maxHistory,
	}
}

// SetLearner connects the collector to a learner.
func (fc *FeedbackCollector) SetLearner(l *Learner) {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	fc.learner = l
}

// SetSession sets the current session ID.
func (fc *FeedbackCollector) SetSession(sessionID string) {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	fc.sessionID = sessionID
}

// RecordAccept records when user accepts a suggestion.
func (fc *FeedbackCollector) RecordAccept(target, targetType string, metadata map[string]interface{}) {
	fc.record(&Feedback{
		Type:       FeedbackTypeAccept,
		Target:     target,
		TargetType: targetType,
		Signal:     1.0,
		Metadata:   metadata,
	})
}

// RecordReject records when user rejects a suggestion.
func (fc *FeedbackCollector) RecordReject(target, targetType, reason string) {
	fc.record(&Feedback{
		Type:       FeedbackTypeReject,
		Target:     target,
		TargetType: targetType,
		Signal:     -1.0,
		Value:      reason,
	})
}

// RecordModify records when user modifies suggested output.
func (fc *FeedbackCollector) RecordModify(target, targetType string, changeRatio float64) {
	// Signal based on how much was changed (more change = more negative)
	signal := 1.0 - (changeRatio * 1.5)
	if signal < -1.0 {
		signal = -1.0
	}

	fc.record(&Feedback{
		Type:       FeedbackTypeModify,
		Target:     target,
		TargetType: targetType,
		Signal:     signal,
		Value:      changeRatio,
	})
}

// RecordUndo records when user undoes an action.
func (fc *FeedbackCollector) RecordUndo(target, targetType string) {
	fc.record(&Feedback{
		Type:       FeedbackTypeUndo,
		Target:     target,
		TargetType: targetType,
		Signal:     -0.8, // Strong negative signal
	})
}

// RecordRating records an explicit rating.
func (fc *FeedbackCollector) RecordRating(target, targetType string, rating float64) {
	// Normalize rating to signal (-1 to 1)
	signal := (rating - 3.0) / 2.0 // Assuming 1-5 scale
	if signal < -1.0 {
		signal = -1.0
	}
	if signal > 1.0 {
		signal = 1.0
	}

	fc.record(&Feedback{
		Type:       FeedbackTypeRating,
		Target:     target,
		TargetType: targetType,
		Signal:     signal,
		Value:      rating,
	})
}

// RecordCorrection records when user provides a correction.
func (fc *FeedbackCollector) RecordCorrection(target, targetType string, original, corrected interface{}) {
	fc.record(&Feedback{
		Type:       FeedbackTypeCorrect,
		Target:     target,
		TargetType: targetType,
		Signal:     -0.5, // Mild negative - needed correction but user engaged
		Value:      corrected,
		Metadata: map[string]interface{}{
			"original": original,
		},
	})
}

// RecordPreference records an explicit user preference.
func (fc *FeedbackCollector) RecordPreference(preference string, value interface{}) {
	fc.record(&Feedback{
		Type:       FeedbackTypePrefer,
		Target:     preference,
		TargetType: "preference",
		Signal:     1.0,
		Value:      value,
	})
}

// RecordSkip records when user skips a suggestion.
func (fc *FeedbackCollector) RecordSkip(target, targetType string) {
	fc.record(&Feedback{
		Type:       FeedbackTypeSkip,
		Target:     target,
		TargetType: targetType,
		Signal:     -0.2, // Mild negative
	})
}

// RecordRetry records when user requests a retry.
func (fc *FeedbackCollector) RecordRetry(target, targetType string) {
	fc.record(&Feedback{
		Type:       FeedbackTypeRetry,
		Target:     target,
		TargetType: targetType,
		Signal:     -0.6, // Moderate negative - output wasn't satisfactory
	})
}

// RecordImplicit records implicit feedback from behavior.
func (fc *FeedbackCollector) RecordImplicit(target, targetType string, behavior string, signal float64) {
	fc.record(&Feedback{
		Type:       FeedbackTypeImplicit,
		Target:     target,
		TargetType: targetType,
		Signal:     signal,
		Value:      behavior,
	})
}

// record adds a feedback event and updates aggregations.
func (fc *FeedbackCollector) record(fb *Feedback) {
	fc.mu.Lock()
	defer fc.mu.Unlock()

	fb.Timestamp = time.Now()
	fb.SessionID = fc.sessionID

	// Add to history
	fc.feedback = append(fc.feedback, fb)
	if len(fc.feedback) > fc.maxHistory {
		fc.feedback = fc.feedback[1:]
	}

	// Update aggregation
	key := fb.TargetType + ":" + fb.Target
	agg, exists := fc.aggregates[key]
	if !exists {
		agg = &FeedbackAggregation{
			Target:     fb.Target,
			TargetType: fb.TargetType,
		}
		fc.aggregates[key] = agg
	}

	agg.TotalCount++
	agg.LastFeedback = fb.Timestamp
	agg.NetSignal += fb.Signal

	switch fb.Type {
	case FeedbackTypeAccept:
		agg.AcceptCount++
	case FeedbackTypeReject:
		agg.RejectCount++
	case FeedbackTypeModify:
		agg.ModifyCount++
	case FeedbackTypeRating:
		if rating, ok := fb.Value.(float64); ok {
			// Update rolling average
			agg.AverageRating = (agg.AverageRating*float64(agg.TotalCount-1) + rating) / float64(agg.TotalCount)
		}
	}

	// Notify learner
	if fc.learner != nil {
		go fc.learner.ProcessFeedback(fb)
	}
}

// GetAggregation retrieves aggregated feedback for a target.
func (fc *FeedbackCollector) GetAggregation(target, targetType string) *FeedbackAggregation {
	fc.mu.RLock()
	defer fc.mu.RUnlock()

	key := targetType + ":" + target
	return fc.aggregates[key]
}

// GetAllAggregations returns all feedback aggregations.
func (fc *FeedbackCollector) GetAllAggregations() map[string]*FeedbackAggregation {
	fc.mu.RLock()
	defer fc.mu.RUnlock()

	result := make(map[string]*FeedbackAggregation, len(fc.aggregates))
	for k, v := range fc.aggregates {
		result[k] = v
	}
	return result
}

// GetRecentFeedback returns feedback from the last N events.
func (fc *FeedbackCollector) GetRecentFeedback(n int) []*Feedback {
	fc.mu.RLock()
	defer fc.mu.RUnlock()

	if n >= len(fc.feedback) {
		result := make([]*Feedback, len(fc.feedback))
		copy(result, fc.feedback)
		return result
	}

	start := len(fc.feedback) - n
	result := make([]*Feedback, n)
	copy(result, fc.feedback[start:])
	return result
}

// GetFeedbackByType returns feedback of a specific type.
func (fc *FeedbackCollector) GetFeedbackByType(ftype FeedbackType) []*Feedback {
	fc.mu.RLock()
	defer fc.mu.RUnlock()

	var result []*Feedback
	for _, fb := range fc.feedback {
		if fb.Type == ftype {
			result = append(result, fb)
		}
	}
	return result
}

// ComputeAcceptRate calculates acceptance rate for a target.
func (fc *FeedbackCollector) ComputeAcceptRate(target, targetType string) float64 {
	fc.mu.RLock()
	defer fc.mu.RUnlock()

	key := targetType + ":" + target
	agg, exists := fc.aggregates[key]
	if !exists || agg.TotalCount == 0 {
		return 0.5 // Unknown, return neutral
	}

	return float64(agg.AcceptCount) / float64(agg.TotalCount)
}

// ComputeOverallSentiment calculates overall sentiment for a target.
func (fc *FeedbackCollector) ComputeOverallSentiment(target, targetType string) float64 {
	fc.mu.RLock()
	defer fc.mu.RUnlock()

	key := targetType + ":" + target
	agg, exists := fc.aggregates[key]
	if !exists || agg.TotalCount == 0 {
		return 0 // Neutral
	}

	return agg.NetSignal / float64(agg.TotalCount)
}

// Reset clears all feedback data.
func (fc *FeedbackCollector) Reset() {
	fc.mu.Lock()
	defer fc.mu.Unlock()

	fc.feedback = make([]*Feedback, 0)
	fc.aggregates = make(map[string]*FeedbackAggregation)
}

// Export returns all feedback for export.
func (fc *FeedbackCollector) Export() ([]*Feedback, map[string]*FeedbackAggregation) {
	fc.mu.RLock()
	defer fc.mu.RUnlock()

	feedback := make([]*Feedback, len(fc.feedback))
	copy(feedback, fc.feedback)

	aggregates := make(map[string]*FeedbackAggregation, len(fc.aggregates))
	for k, v := range fc.aggregates {
		aggregates[k] = v
	}

	return feedback, aggregates
}
