package learning

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// Suggestion represents a personalized suggestion.
type Suggestion struct {
	ID          string                 `json:"id"`
	Type        string                 `json:"type"`         // "tool", "action", "setting", "shortcut"
	Content     string                 `json:"content"`      // The actual suggestion
	Reason      string                 `json:"reason"`       // Why this is suggested
	Confidence  float64                `json:"confidence"`   // 0.0 to 1.0
	Priority    int                    `json:"priority"`     // Higher = more important
	Context     string                 `json:"context"`      // When to show this
	LearnedFrom string                 `json:"learned_from"` // Pattern ID that generated this
	CreatedAt   time.Time              `json:"created_at"`
	ExpiresAt   time.Time              `json:"expires_at,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// PredictedContext represents predicted user context.
type PredictedContext struct {
	WorkingDirectory string            `json:"working_directory,omitempty"`
	RelevantFiles    []string          `json:"relevant_files,omitempty"`
	LikelyTask       string            `json:"likely_task,omitempty"`
	ProbableTools    []string          `json:"probable_tools,omitempty"`
	ExpectedDuration string            `json:"expected_duration,omitempty"`
	Confidence       float64           `json:"confidence"`
	Predictions      map[string]string `json:"predictions,omitempty"`
}

// SmartDefaults represents personalized default values.
type SmartDefaults struct {
	Provider         string            `json:"provider,omitempty"`
	Model            string            `json:"model,omitempty"`
	Theme            string            `json:"theme,omitempty"`
	ResponseStyle    string            `json:"response_style,omitempty"`
	Verbosity        string            `json:"verbosity,omitempty"`
	AutoConfirmTools []string          `json:"auto_confirm_tools,omitempty"`
	ToolTimeouts     map[string]string `json:"tool_timeouts,omitempty"`
	PreferredBranch  string            `json:"preferred_branch,omitempty"`
	FileFilters      []string          `json:"file_filters,omitempty"`
	ExcludePatterns  []string          `json:"exclude_patterns,omitempty"`
}

// PersonalizationEngine provides personalized experiences.
type PersonalizationEngine struct {
	mu          sync.RWMutex
	profile     *UserProfile
	recognizer  *PatternRecognizer
	feedback    *FeedbackCollector
	suggestions []*Suggestion
	defaults    *SmartDefaults
	context     *PredictedContext
}

// NewPersonalizationEngine creates a new personalization engine.
func NewPersonalizationEngine() *PersonalizationEngine {
	return &PersonalizationEngine{
		suggestions: make([]*Suggestion, 0),
		defaults:    &SmartDefaults{},
		context:     &PredictedContext{},
	}
}

// SetProfile connects the engine to a user profile.
func (pe *PersonalizationEngine) SetProfile(p *UserProfile) {
	pe.mu.Lock()
	defer pe.mu.Unlock()
	pe.profile = p
}

// SetRecognizer connects the engine to a pattern recognizer.
func (pe *PersonalizationEngine) SetRecognizer(pr *PatternRecognizer) {
	pe.mu.Lock()
	defer pe.mu.Unlock()
	pe.recognizer = pr
}

// SetFeedback connects the engine to a feedback collector.
func (pe *PersonalizationEngine) SetFeedback(fc *FeedbackCollector) {
	pe.mu.Lock()
	defer pe.mu.Unlock()
	pe.feedback = fc
}

// GenerateSuggestions creates personalized suggestions based on patterns.
func (pe *PersonalizationEngine) GenerateSuggestions() []*Suggestion {
	pe.mu.Lock()
	defer pe.mu.Unlock()

	pe.suggestions = make([]*Suggestion, 0)

	if pe.recognizer == nil {
		return pe.suggestions
	}

	patterns := pe.recognizer.GetConfidentPatterns()
	now := time.Now()

	for _, p := range patterns {
		switch p.Type {
		case PatternTypeToolUsage:
			if p.Confidence >= 0.7 && p.Frequency >= 5 {
				pe.suggestions = append(pe.suggestions, &Suggestion{
					ID:          fmt.Sprintf("sug_tool_%s", p.ID),
					Type:        "shortcut",
					Content:     fmt.Sprintf("Create alias for frequently used tool: %s", p.Name),
					Reason:      fmt.Sprintf("Used %d times with %.0f%% confidence", p.Frequency, p.Confidence*100),
					Confidence:  p.Confidence,
					Priority:    p.Frequency,
					LearnedFrom: p.ID,
					CreatedAt:   now,
				})
			}

		case PatternTypeCodingStyle:
			if p.Data.IndentStyle != "" {
				pe.suggestions = append(pe.suggestions, &Suggestion{
					ID:          fmt.Sprintf("sug_style_%s", p.ID),
					Type:        "setting",
					Content:     fmt.Sprintf("Set default indentation: %s", formatIndent(p.Data.IndentStyle, p.Data.IndentSize)),
					Reason:      "Detected from your code patterns",
					Confidence:  p.Confidence,
					Priority:    5,
					LearnedFrom: p.ID,
					CreatedAt:   now,
				})
			}

		case PatternTypeWorkflow:
			if len(p.Data.StepSequence) >= 3 {
				pe.suggestions = append(pe.suggestions, &Suggestion{
					ID:          fmt.Sprintf("sug_workflow_%s", p.ID),
					Type:        "action",
					Content:     "Create workflow automation for common task sequence",
					Reason:      fmt.Sprintf("Detected %d-step repeated pattern", len(p.Data.StepSequence)),
					Confidence:  p.Confidence,
					Priority:    3,
					LearnedFrom: p.ID,
					CreatedAt:   now,
					Metadata: map[string]interface{}{
						"steps": p.Data.StepSequence,
					},
				})
			}

		case PatternTypeTime:
			if len(p.Data.PeakHours) > 0 {
				pe.suggestions = append(pe.suggestions, &Suggestion{
					ID:          fmt.Sprintf("sug_time_%s", p.ID),
					Type:        "setting",
					Content:     fmt.Sprintf("Schedule heavy tasks outside peak hours (%s)", formatHours(p.Data.PeakHours)),
					Reason:      "Based on your activity patterns",
					Confidence:  p.Confidence,
					Priority:    2,
					LearnedFrom: p.ID,
					CreatedAt:   now,
				})
			}

		case PatternTypeCommunication:
			if p.Data.Verbosity != "" && pe.profile != nil {
				if pe.profile.Preferences.ResponseVerbosity == "" {
					pe.suggestions = append(pe.suggestions, &Suggestion{
						ID:          fmt.Sprintf("sug_comm_%s", p.ID),
						Type:        "setting",
						Content:     fmt.Sprintf("Set response verbosity to: %s", p.Data.Verbosity),
						Reason:      "Matches your communication style",
						Confidence:  p.Confidence,
						Priority:    4,
						LearnedFrom: p.ID,
						CreatedAt:   now,
					})
				}
			}
		}
	}

	// Sort by priority (descending) then confidence
	sort.Slice(pe.suggestions, func(i, j int) bool {
		if pe.suggestions[i].Priority != pe.suggestions[j].Priority {
			return pe.suggestions[i].Priority > pe.suggestions[j].Priority
		}
		return pe.suggestions[i].Confidence > pe.suggestions[j].Confidence
	})

	return pe.suggestions
}

// GetSuggestions returns current suggestions, optionally filtered.
func (pe *PersonalizationEngine) GetSuggestions(sugType string, limit int) []*Suggestion {
	pe.mu.RLock()
	defer pe.mu.RUnlock()

	if len(pe.suggestions) == 0 {
		pe.mu.RUnlock()
		pe.GenerateSuggestions()
		pe.mu.RLock()
	}

	var result []*Suggestion
	for _, s := range pe.suggestions {
		if sugType != "" && s.Type != sugType {
			continue
		}
		result = append(result, s)
		if limit > 0 && len(result) >= limit {
			break
		}
	}
	return result
}

// PredictContext predicts user's likely context.
func (pe *PersonalizationEngine) PredictContext(currentDir string, recentFiles []string) *PredictedContext {
	pe.mu.Lock()
	defer pe.mu.Unlock()

	pe.context = &PredictedContext{
		WorkingDirectory: currentDir,
		Predictions:      make(map[string]string),
	}

	if pe.recognizer == nil {
		return pe.context
	}

	patterns := pe.recognizer.GetConfidentPatterns()
	var confidence float64

	// Predict likely tools based on file patterns
	toolPatterns := make(map[string]int)
	for _, p := range patterns {
		if p.Type == PatternTypeToolUsage {
			toolPatterns[p.Name] = p.Frequency
		}
	}

	// Sort tools by frequency
	type toolFreq struct {
		name string
		freq int
	}
	var tools []toolFreq
	for name, freq := range toolPatterns {
		tools = append(tools, toolFreq{name, freq})
	}
	sort.Slice(tools, func(i, j int) bool {
		return tools[i].freq > tools[j].freq
	})

	for i, t := range tools {
		if i >= 5 {
			break
		}
		pe.context.ProbableTools = append(pe.context.ProbableTools, t.name)
	}

	// Add relevant files from recent history
	pe.context.RelevantFiles = recentFiles
	if len(pe.context.RelevantFiles) > 10 {
		pe.context.RelevantFiles = pe.context.RelevantFiles[:10]
	}

	// Predict task based on time patterns
	for _, p := range patterns {
		if p.Type == PatternTypeTime && len(p.Data.PeakHours) > 0 {
			hour := time.Now().Hour()
			for _, peakHour := range p.Data.PeakHours {
				if hour == peakHour {
					pe.context.Predictions["peak_time"] = "true"
					confidence += 0.2
					break
				}
			}
		}
	}

	// Check for workflow patterns
	for _, p := range patterns {
		if p.Type == PatternTypeWorkflow && len(p.Data.StepSequence) > 0 {
			pe.context.LikelyTask = "continuation of " + p.Data.StepSequence[0]
			confidence += 0.1
		}
	}

	// Normalize confidence
	if confidence > 1.0 {
		confidence = 1.0
	}
	pe.context.Confidence = confidence

	return pe.context
}

// ComputeSmartDefaults calculates personalized defaults.
func (pe *PersonalizationEngine) ComputeSmartDefaults() *SmartDefaults {
	pe.mu.Lock()
	defer pe.mu.Unlock()

	pe.defaults = &SmartDefaults{}

	// Start with profile preferences
	if pe.profile != nil {
		pe.defaults.Theme = pe.profile.Preferences.Theme
		pe.defaults.Verbosity = pe.profile.Preferences.ResponseVerbosity
		pe.defaults.PreferredBranch = pe.profile.Preferences.DefaultBranch
	}

	// Learn from feedback
	if pe.feedback != nil {
		prefs := pe.feedback.InferPreferences()
		if tools, ok := prefs["favorite_tools"].([]string); ok {
			pe.defaults.AutoConfirmTools = tools
		}
	}

	// Learn from patterns
	if pe.recognizer != nil {
		patterns := pe.recognizer.GetConfidentPatterns()
		for _, p := range patterns {
			switch p.Type {
			case PatternTypeCodingStyle:
				if p.Data.IndentStyle != "" && pe.profile != nil {
					pe.profile.Preferences.PreferredIndentation = formatIndent(p.Data.IndentStyle, p.Data.IndentSize)
				}

			case PatternTypeCommunication:
				if p.Data.Verbosity != "" && pe.defaults.Verbosity == "" {
					pe.defaults.Verbosity = p.Data.Verbosity
				}

			case PatternTypeFile:
				// Add common file patterns to filters
				for _, ext := range p.Data.FileExtensions {
					pe.defaults.FileFilters = addUnique(pe.defaults.FileFilters, "*."+ext)
				}
			}
		}
	}

	return pe.defaults
}

// GetSmartDefaults returns current smart defaults.
func (pe *PersonalizationEngine) GetSmartDefaults() *SmartDefaults {
	pe.mu.RLock()
	defer pe.mu.RUnlock()

	if pe.defaults == nil {
		pe.mu.RUnlock()
		return pe.ComputeSmartDefaults()
	}
	return pe.defaults
}

// ApplySuggestion applies a suggestion and records feedback.
func (pe *PersonalizationEngine) ApplySuggestion(suggestionID string) error {
	pe.mu.Lock()
	defer pe.mu.Unlock()

	var suggestion *Suggestion
	for _, s := range pe.suggestions {
		if s.ID == suggestionID {
			suggestion = s
			break
		}
	}

	if suggestion == nil {
		return fmt.Errorf("suggestion not found: %s", suggestionID)
	}

	// Record positive feedback
	if pe.feedback != nil {
		pe.feedback.RecordAccept(suggestionID, "suggestion", map[string]interface{}{
			"type":    suggestion.Type,
			"content": suggestion.Content,
		})
	}

	// Remove from suggestions
	newSuggestions := make([]*Suggestion, 0, len(pe.suggestions)-1)
	for _, s := range pe.suggestions {
		if s.ID != suggestionID {
			newSuggestions = append(newSuggestions, s)
		}
	}
	pe.suggestions = newSuggestions

	return nil
}

// DismissSuggestion removes a suggestion and records negative feedback.
func (pe *PersonalizationEngine) DismissSuggestion(suggestionID, reason string) error {
	pe.mu.Lock()
	defer pe.mu.Unlock()

	var found bool
	newSuggestions := make([]*Suggestion, 0, len(pe.suggestions))
	for _, s := range pe.suggestions {
		if s.ID == suggestionID {
			found = true
			// Record negative feedback
			if pe.feedback != nil {
				pe.feedback.RecordReject(suggestionID, "suggestion", reason)
			}
		} else {
			newSuggestions = append(newSuggestions, s)
		}
	}

	if !found {
		return fmt.Errorf("suggestion not found: %s", suggestionID)
	}

	pe.suggestions = newSuggestions
	return nil
}

// PersonalizeResponse adapts a response based on user preferences.
func (pe *PersonalizationEngine) PersonalizeResponse(response string) string {
	pe.mu.RLock()
	defer pe.mu.RUnlock()

	if pe.profile == nil {
		return response
	}

	prefs := pe.profile.Preferences

	// Adjust verbosity
	switch prefs.ResponseVerbosity {
	case "brief":
		// Could summarize, but for now just return as-is
		return response
	case "detailed":
		// Could expand, but for now just return as-is
		return response
	}

	return response
}

// GetPersonalizationSummary returns a summary of personalization state.
func (pe *PersonalizationEngine) GetPersonalizationSummary() map[string]interface{} {
	pe.mu.RLock()
	defer pe.mu.RUnlock()

	summary := make(map[string]interface{})

	if pe.profile != nil {
		summary["profile_id"] = pe.profile.ID
		summary["total_sessions"] = pe.profile.Stats.TotalSessions
		summary["patterns_learned"] = len(pe.profile.Patterns)
		summary["privacy_local_only"] = pe.profile.Privacy.LocalOnly
	}

	if pe.recognizer != nil {
		patterns := pe.recognizer.GetConfidentPatterns()
		summary["confident_patterns"] = len(patterns)
	}

	summary["active_suggestions"] = len(pe.suggestions)

	if pe.defaults != nil {
		summary["has_smart_defaults"] = true
	}

	if pe.context != nil && pe.context.Confidence > 0 {
		summary["context_confidence"] = pe.context.Confidence
	}

	return summary
}

// formatIndent formats indentation preference.
func formatIndent(style string, size int) string {
	if style == "tabs" {
		return "tabs"
	}
	if size > 0 {
		return fmt.Sprintf("spaces:%d", size)
	}
	return style
}

// formatHours formats a list of hours for display.
func formatHours(hours []int) string {
	if len(hours) == 0 {
		return "N/A"
	}
	sort.Ints(hours)
	var parts []string
	for _, h := range hours {
		parts = append(parts, fmt.Sprintf("%02d:00", h))
	}
	return strings.Join(parts, ", ")
}
