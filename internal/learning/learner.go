package learning

import (
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Learner is the main entry point for the learning system.
// It coordinates pattern recognition, feedback, personalization, and privacy.
type Learner struct {
	mu sync.RWMutex

	// Core components
	Profile         *UserProfile
	Recognizer      *PatternRecognizer
	Feedback        *FeedbackCollector
	Personalization *PersonalizationEngine
	Privacy         *PrivacyController

	// Storage
	ProfileStorage *ProfileStorage
	dataDir        string

	// Configuration
	enabled       bool
	autoSave      bool
	saveInterval  time.Duration
	lastSave      time.Time
	decayInterval time.Duration
	lastDecay     time.Time

	// Session tracking
	sessionID    string
	sessionStart time.Time
}

// LearnerConfig holds configuration for the learner.
type LearnerConfig struct {
	DataDir       string        // Directory for storing learning data
	Enabled       bool          // Whether learning is enabled
	AutoSave      bool          // Auto-save profile periodically
	SaveInterval  time.Duration // How often to auto-save
	DecayInterval time.Duration // How often to decay old patterns
	UserID        string        // User identifier (can be anonymous)
}

// DefaultLearnerConfig returns default configuration.
func DefaultLearnerConfig() LearnerConfig {
	home, _ := os.UserHomeDir()
	return LearnerConfig{
		DataDir:       filepath.Join(home, ".automergent", "learning"),
		Enabled:       true,
		AutoSave:      true,
		SaveInterval:  5 * time.Minute,
		DecayInterval: 24 * time.Hour,
		UserID:        "default",
	}
}

// NewLearner creates a new learner with the given configuration.
func NewLearner(cfg LearnerConfig) (*Learner, error) {
	// Create storage
	storage, err := NewProfileStorage(cfg.DataDir)
	if err != nil {
		return nil, err
	}

	// Create privacy controller
	privacy, err := NewPrivacyController(cfg.DataDir)
	if err != nil {
		return nil, err
	}

	// Load or create profile
	profile, err := storage.Load(cfg.UserID)
	if err != nil {
		return nil, err
	}
	if profile == nil {
		profile = NewUserProfile(cfg.UserID)
	}

	// Create components
	recognizer := NewPatternRecognizer()
	recognizer.SetProfile(profile)

	feedback := NewFeedbackCollector(1000)

	personalization := NewPersonalizationEngine()
	personalization.SetProfile(profile)
	personalization.SetRecognizer(recognizer)
	personalization.SetFeedback(feedback)

	privacy.SetProfile(profile)

	learner := &Learner{
		Profile:         profile,
		Recognizer:      recognizer,
		Feedback:        feedback,
		Personalization: personalization,
		Privacy:         privacy,
		ProfileStorage:  storage,
		dataDir:         cfg.DataDir,
		enabled:         cfg.Enabled,
		autoSave:        cfg.AutoSave,
		saveInterval:    cfg.SaveInterval,
		decayInterval:   cfg.DecayInterval,
	}

	// Connect feedback to learner
	feedback.SetLearner(learner)

	return learner, nil
}

// StartSession begins a new learning session.
func (l *Learner) StartSession(sessionID string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.sessionID = sessionID
	l.sessionStart = time.Now()

	if l.Feedback != nil {
		l.Feedback.SetSession(sessionID)
	}

	// Record time activity
	if l.canCollect(PatternTypeTime) {
		l.Recognizer.RecordTimeActivity()
	}
}

// EndSession ends the current session and records stats.
func (l *Learner) EndSession(messages int, tokens int64, toolCalls int) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.Profile != nil {
		l.Profile.RecordSession(messages, tokens, toolCalls)
	}

	// Auto-save
	if l.autoSave {
		l.save()
	}

	l.sessionID = ""
}

// RecordToolUse records a tool usage event.
func (l *Learner) RecordToolUse(toolName string, args map[string]interface{}, success bool) {
	if !l.canCollect(PatternTypeToolUsage) {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	l.Recognizer.RecordToolUsage(toolName, args, success)

	// Record as workflow step
	if l.canCollect(PatternTypeWorkflow) && l.sessionID != "" {
		l.Recognizer.RecordWorkflow("tool:"+toolName, l.sessionID)
	}
}

// RecordFileAccess records a file access event.
func (l *Learner) RecordFileAccess(path string, operation string) {
	if !l.canCollect(PatternTypeFile) {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	l.Recognizer.RecordFileAccess(path, operation)
}

// RecordCode records code for style analysis.
func (l *Learner) RecordCode(code string, language string) {
	if !l.canCollect(PatternTypeCodingStyle) {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	l.Recognizer.RecordCodingStyle(code, language)
}

// RecordMessage records a conversation message.
func (l *Learner) RecordMessage(message string, isUser bool) {
	if !l.canCollect(PatternTypeCommunication) {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	l.Recognizer.RecordCommunication(message, isUser)
}

// RecordError records an error event.
func (l *Learner) RecordError(errorType string, resolved bool, resolution string) {
	if !l.canCollect(PatternTypeError) {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	l.Recognizer.RecordError(errorType, resolved, resolution)
	if l.Profile != nil {
		l.Profile.RecordError()
	}
}

// RecordSuccess records a successful task completion.
func (l *Learner) RecordSuccess() {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.Profile != nil {
		l.Profile.RecordSuccess()
	}
}

// ProcessFeedback processes feedback from the collector.
func (l *Learner) ProcessFeedback(fb *Feedback) {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Update patterns based on feedback
	switch fb.Type {
	case FeedbackTypeAccept:
		// Boost confidence for accepted items
		if pattern := l.Recognizer.patterns[fb.Target]; pattern != nil {
			pattern.Confidence = min(1.0, pattern.Confidence+0.1)
		}

	case FeedbackTypeReject:
		// Reduce confidence for rejected items
		if pattern := l.Recognizer.patterns[fb.Target]; pattern != nil {
			pattern.Confidence = max(0.0, pattern.Confidence-0.2)
		}

	case FeedbackTypePrefer:
		// Update profile preference
		if l.Profile != nil {
			l.Profile.SetPreference(fb.Target, fb.Value)
		}
	}
}

// GetSuggestions returns personalized suggestions.
func (l *Learner) GetSuggestions(sugType string, limit int) []*Suggestion {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if l.Personalization == nil {
		return nil
	}

	return l.Personalization.GetSuggestions(sugType, limit)
}

// GetSmartDefaults returns computed smart defaults.
func (l *Learner) GetSmartDefaults() *SmartDefaults {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if l.Personalization == nil {
		return nil
	}

	return l.Personalization.GetSmartDefaults()
}

// PredictContext predicts user's likely context.
func (l *Learner) PredictContext(currentDir string, recentFiles []string) *PredictedContext {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if l.Personalization == nil {
		return nil
	}

	return l.Personalization.PredictContext(currentDir, recentFiles)
}

// GetUserPreferences returns the user's learned preferences.
func (l *Learner) GetUserPreferences() *UserPreferences {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if l.Profile == nil {
		return nil
	}

	return &l.Profile.Preferences
}

// SetPreference explicitly sets a user preference.
func (l *Learner) SetPreference(key string, value interface{}) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.Profile == nil {
		return nil
	}

	// Record as feedback
	if l.Feedback != nil {
		l.Feedback.RecordPreference(key, value)
	}

	return l.Profile.SetPreference(key, value)
}

// GetPrivacyLevel returns the current privacy level.
func (l *Learner) GetPrivacyLevel() PrivacyLevel {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if l.Privacy == nil {
		return PrivacyLevelNone
	}

	return l.Privacy.GetLevel()
}

// SetPrivacyLevel sets the privacy level.
func (l *Learner) SetPrivacyLevel(level PrivacyLevel) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.Privacy != nil {
		l.Privacy.SetLevel(level)
	}
}

// GiveConsent records user consent.
func (l *Learner) GiveConsent(version string, categories []string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.Privacy == nil {
		return nil
	}

	return l.Privacy.GiveConsent(version, categories)
}

// RevokeConsent revokes consent and clears data.
func (l *Learner) RevokeConsent() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.Privacy == nil {
		return nil
	}

	return l.Privacy.RevokeConsent()
}

// ExportData exports all user data.
func (l *Learner) ExportData(path string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.Privacy == nil {
		return nil
	}

	// Gather all data
	var patterns []*Pattern
	for _, p := range l.Recognizer.GetAllPatterns() {
		patterns = append(patterns, p)
	}

	feedback, aggregations := l.Feedback.Export()
	suggestions := l.Personalization.GetSuggestions("", 0)

	export, err := l.Privacy.ExportData(l.Profile, patterns, feedback, aggregations, suggestions)
	if err != nil {
		return err
	}

	return l.Privacy.ExportDataToFile(export, path)
}

// DeleteAllData deletes all learned data.
func (l *Learner) DeleteAllData() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Clear in-memory data
	l.Recognizer.patterns = make(map[string]*Pattern)
	l.Feedback.Reset()

	if l.Privacy != nil {
		return l.Privacy.DeleteAllData()
	}

	return nil
}

// Save persists the current state.
func (l *Learner) Save() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	return l.save()
}

// save internal save without locking (caller must hold lock).
func (l *Learner) save() error {
	if l.Profile == nil || l.ProfileStorage == nil {
		return nil
	}

	// Update profile with current patterns
	for id, p := range l.Recognizer.patterns {
		l.Profile.Patterns[id] = p
	}

	l.lastSave = time.Now()
	return l.ProfileStorage.Save(l.Profile)
}

// PeriodicMaintenance runs periodic maintenance tasks.
func (l *Learner) PeriodicMaintenance() {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()

	// Auto-save
	if l.autoSave && now.Sub(l.lastSave) >= l.saveInterval {
		l.save()
	}

	// Decay old patterns
	if now.Sub(l.lastDecay) >= l.decayInterval {
		l.Recognizer.DecayPatterns(7 * 24 * time.Hour) // Decay patterns not seen in 7 days
		l.lastDecay = now
	}

	// Apply retention policy
	if l.Privacy != nil {
		l.Privacy.ApplyRetentionPolicy()
	}
}

// GetStats returns learning statistics.
func (l *Learner) GetStats() map[string]interface{} {
	l.mu.RLock()
	defer l.mu.RUnlock()

	stats := make(map[string]interface{})

	stats["enabled"] = l.enabled
	stats["privacy_level"] = string(l.GetPrivacyLevel())

	if l.Profile != nil {
		stats["profile_id"] = l.Profile.ID
		stats["total_sessions"] = l.Profile.Stats.TotalSessions
		stats["total_messages"] = l.Profile.Stats.TotalMessages
		stats["patterns_count"] = len(l.Profile.Patterns)
	}

	if l.Recognizer != nil {
		confident := l.Recognizer.GetConfidentPatterns()
		stats["confident_patterns"] = len(confident)
	}

	if l.Personalization != nil {
		summary := l.Personalization.GetPersonalizationSummary()
		for k, v := range summary {
			stats["personalization_"+k] = v
		}
	}

	return stats
}

// IsEnabled returns whether learning is enabled.
func (l *Learner) IsEnabled() bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.enabled
}

// SetEnabled enables or disables learning.
func (l *Learner) SetEnabled(enabled bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.enabled = enabled
}

// canCollect checks if data collection is allowed.
func (l *Learner) canCollect(dataType PatternType) bool {
	if !l.enabled {
		return false
	}

	if l.Privacy != nil {
		return l.Privacy.CanCollect(dataType)
	}

	return true
}

// min returns the minimum of two floats.
func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

// max returns the maximum of two floats.
func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
