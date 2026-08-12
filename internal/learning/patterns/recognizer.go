package patterns

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/iSundram/Automergent/internal/learning"
)

// Recognizer identifies and learns patterns from user interactions.
type Recognizer struct {
	storage       learning.Storage
	minConfidence float64
	patterns      map[string]*learning.Pattern
	sequences     map[string]*sequence
	mu            sync.RWMutex
}

// sequence tracks a sequence of events to detect patterns.
type sequence struct {
	events    []learning.Event
	startTime time.Time
	lastSeen  time.Time
}

// NewRecognizer creates a new pattern recognizer.
func NewRecognizer(storage learning.Storage, minConfidence float64) *Recognizer {
	return &Recognizer{
		storage:       storage,
		minConfidence: minConfidence,
		patterns:      make(map[string]*learning.Pattern),
		sequences:     make(map[string]*sequence),
	}
}

// ProcessEvent processes an event for pattern recognition.
func (r *Recognizer) ProcessEvent(ctx context.Context, event learning.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Detect file sequence patterns
	if event.Type == learning.EventTypeFileAccess {
		r.detectFileSequence(ctx, event)
	}

	// Detect tool chain patterns
	if event.Type == learning.EventTypeToolUse || event.Type == learning.EventTypeToolSuccess {
		r.detectToolChain(ctx, event)
	}

	// Detect error resolution patterns
	if event.Type == learning.EventTypeToolError || event.Type == learning.EventTypeTaskFail {
		r.detectErrorResolution(ctx, event)
	}

	// Detect workflow patterns
	if event.Type == learning.EventTypeTaskComplete {
		r.detectWorkflow(ctx, event)
	}

	// Clean up old sequences
	r.cleanupSequences()
}

// detectFileSequence identifies patterns in file access sequences.
func (r *Recognizer) detectFileSequence(ctx context.Context, event learning.Event) {
	sessionID := event.SessionID
	seq, exists := r.sequences[sessionID]
	if !exists {
		seq = &sequence{
			events:    []learning.Event{},
			startTime: time.Now(),
			lastSeen:  time.Now(),
		}
		r.sequences[sessionID] = seq
	}

	seq.events = append(seq.events, event)
	seq.lastSeen = time.Now()

	// Look for repeating file access patterns
	if len(seq.events) >= 3 {
		files := r.extractFileSequence(seq.events)
		if len(files) >= 3 {
			pattern := r.createFileSequencePattern(files)
			r.updateOrCreatePattern(ctx, pattern)
		}
	}
}

// detectToolChain identifies patterns in tool usage sequences.
func (r *Recognizer) detectToolChain(ctx context.Context, event learning.Event) {
	sessionID := event.SessionID
	seq, exists := r.sequences[sessionID+"_tools"]
	if !exists {
		seq = &sequence{
			events:    []learning.Event{},
			startTime: time.Now(),
			lastSeen:  time.Now(),
		}
		r.sequences[sessionID+"_tools"] = seq
	}

	seq.events = append(seq.events, event)
	seq.lastSeen = time.Now()

	// Look for common tool chains
	if len(seq.events) >= 2 {
		tools := r.extractToolSequence(seq.events)
		if len(tools) >= 2 {
			pattern := r.createToolChainPattern(tools)
			r.updateOrCreatePattern(ctx, pattern)
		}
	}
}

// detectErrorResolution identifies patterns in error handling.
func (r *Recognizer) detectErrorResolution(ctx context.Context, event learning.Event) {
	// Extract error type
	errorType, ok := event.Data["error_type"].(string)
	if !ok {
		return
	}

	// Look for subsequent successful events
	sessionID := event.SessionID
	seq, exists := r.sequences[sessionID+"_errors"]
	if !exists {
		seq = &sequence{
			events:    []learning.Event{event},
			startTime: time.Now(),
			lastSeen:  time.Now(),
		}
		r.sequences[sessionID+"_errors"] = seq
		return
	}

	// If we see a success after an error, record the resolution pattern
	if event.Type == learning.EventTypeTaskComplete || event.Type == learning.EventTypeToolSuccess {
		if len(seq.events) > 0 {
			lastEvent := seq.events[len(seq.events)-1]
			if lastEvent.Type == learning.EventTypeToolError || lastEvent.Type == learning.EventTypeTaskFail {
				pattern := r.createErrorResolutionPattern(lastEvent, event, errorType)
				r.updateOrCreatePattern(ctx, pattern)
			}
		}
	}

	seq.events = append(seq.events, event)
	seq.lastSeen = time.Now()
}

// detectWorkflow identifies high-level workflow patterns.
func (r *Recognizer) detectWorkflow(ctx context.Context, event learning.Event) {
	sessionID := event.SessionID
	seq, exists := r.sequences[sessionID+"_workflow"]
	if !exists {
		seq = &sequence{
			events:    []learning.Event{},
			startTime: time.Now(),
			lastSeen:  time.Now(),
		}
		r.sequences[sessionID+"_workflow"] = seq
	}

	seq.events = append(seq.events, event)
	seq.lastSeen = time.Now()

	// Analyze workflow patterns after a task completes
	if len(seq.events) >= 3 {
		pattern := r.createWorkflowPattern(seq.events)
		r.updateOrCreatePattern(ctx, pattern)
	}
}

// extractFileSequence extracts the file access sequence from events.
func (r *Recognizer) extractFileSequence(events []learning.Event) []string {
	var files []string
	seen := make(map[string]bool)

	for _, event := range events {
		if event.Type == learning.EventTypeFileAccess {
			if file, ok := event.Data["file"].(string); ok {
				if !seen[file] {
					files = append(files, file)
					seen[file] = true
				}
			}
		}
	}

	return files
}

// extractToolSequence extracts the tool usage sequence from events.
func (r *Recognizer) extractToolSequence(events []learning.Event) []string {
	var tools []string

	for _, event := range events {
		if event.Type == learning.EventTypeToolUse || event.Type == learning.EventTypeToolSuccess {
			if tool, ok := event.Data["tool"].(string); ok {
				tools = append(tools, tool)
			}
		}
	}

	return tools
}

// createFileSequencePattern creates a pattern from a file sequence.
func (r *Recognizer) createFileSequencePattern(files []string) *learning.Pattern {
	patternID := r.hashSequence("file", files)

	return &learning.Pattern{
		ID:          patternID,
		Type:        learning.PatternTypeWorkflow,
		Confidence:  0.5, // Initial confidence
		Frequency:   1,
		FirstSeen:   time.Now(),
		LastSeen:    time.Now(),
		Description: fmt.Sprintf("File access sequence: %v", files),
		Data: learning.PatternData{
			Directories: files,
		},
	}
}

// createToolChainPattern creates a pattern from a tool sequence.
func (r *Recognizer) createToolChainPattern(tools []string) *learning.Pattern {
	patternID := r.hashSequence("tool", tools)

	return &learning.Pattern{
		ID:          patternID,
		Type:        learning.PatternTypeToolUsage,
		Confidence:  0.5,
		Frequency:   1,
		FirstSeen:   time.Now(),
		LastSeen:    time.Now(),
		Description: fmt.Sprintf("Tool chain: %v", tools),
		Data: learning.PatternData{
			StepSequence: tools,
		},
	}
}

// createErrorResolutionPattern creates a pattern from error resolution.
func (r *Recognizer) createErrorResolutionPattern(errorEvent, resolveEvent learning.Event, errorType string) *learning.Pattern {
	patternID := uuid.New().String()

	resolution, _ := resolveEvent.Data["action"].(string)

	return &learning.Pattern{
		ID:          patternID,
		Type:        learning.PatternTypeError,
		Confidence:  0.6,
		Frequency:   1,
		FirstSeen:   time.Now(),
		LastSeen:    time.Now(),
		Description: fmt.Sprintf("Resolution for %s", errorType),
		Data: learning.PatternData{
			ErrorTypes:     []string{errorType},
			ResolutionPath: resolution,
		},
	}
}

// createWorkflowPattern creates a pattern from a workflow.
func (r *Recognizer) createWorkflowPattern(events []learning.Event) *learning.Pattern {
	patternID := uuid.New().String()

	steps := []string{}
	for _, event := range events {
		if action, ok := event.Data["action"].(string); ok {
			steps = append(steps, action)
		}
	}

	return &learning.Pattern{
		ID:          patternID,
		Type:        learning.PatternTypeWorkflow,
		Confidence:  0.5,
		Frequency:   1,
		FirstSeen:   time.Now(),
		LastSeen:    time.Now(),
		Description: fmt.Sprintf("Workflow: %d steps", len(steps)),
		Data: learning.PatternData{
			StepSequence: steps,
		},
	}
}

// updateOrCreatePattern updates an existing pattern or creates a new one.
func (r *Recognizer) updateOrCreatePattern(ctx context.Context, pattern *learning.Pattern) {
	existing, exists := r.patterns[pattern.ID]
	if exists {
		// Update existing pattern
		existing.Frequency++
		existing.LastSeen = time.Now()
		existing.Confidence = r.calculateConfidence(existing.Frequency, existing.FirstSeen)
	} else {
		// Create new pattern
		r.patterns[pattern.ID] = pattern
	}

	// Persist if confidence meets threshold
	if pattern.Confidence >= r.minConfidence {
		r.storage.SavePattern(ctx, *pattern)
	}
}

// calculateConfidence calculates pattern confidence based on frequency and age.
func (r *Recognizer) calculateConfidence(frequency int, firstSeen time.Time) float64 {
	// Base confidence on frequency
	baseConfidence := float64(frequency) / (float64(frequency) + 10.0)

	// Age factor (older patterns are more established)
	age := time.Since(firstSeen).Hours() / 24.0
	ageFactor := 1.0 - (1.0 / (1.0 + age/30.0))

	return baseConfidence*0.7 + ageFactor*0.3
}

// hashSequence creates a deterministic hash for a sequence.
func (r *Recognizer) hashSequence(prefix string, items []string) string {
	h := sha256.New()
	h.Write([]byte(prefix))
	for _, item := range items {
		h.Write([]byte(item))
	}
	return prefix + "_" + hex.EncodeToString(h.Sum(nil))[:16]
}

// cleanupSequences removes old sequences.
func (r *Recognizer) cleanupSequences() {
	cutoff := time.Now().Add(-30 * time.Minute)

	for key, seq := range r.sequences {
		if seq.lastSeen.Before(cutoff) {
			delete(r.sequences, key)
		}
	}
}

// GetPatterns retrieves patterns of a specific type.
func (r *Recognizer) GetPatterns(ctx context.Context, patternType learning.PatternType) ([]learning.Pattern, error) {
	patterns, err := r.storage.GetPatterns(ctx, patternType)
	if err != nil {
		return nil, err
	}

	// Filter by confidence
	filtered := []learning.Pattern{}
	for _, p := range patterns {
		if p.Confidence >= r.minConfidence {
			filtered = append(filtered, p)
		}
	}

	// Sort by confidence descending
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].Confidence > filtered[j].Confidence
	})

	return filtered, nil
}

// GenerateInsights generates insights from recognized patterns.
func (r *Recognizer) GenerateInsights(ctx context.Context) ([]learning.Insight, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	insights := []learning.Insight{}

	// Find frequently used tool chains
	toolChains, err := r.storage.GetPatterns(ctx, learning.PatternTypeToolUsage)
	if err == nil {
		for _, pattern := range toolChains {
			if pattern.Frequency >= 5 && pattern.Confidence >= 0.8 {
				insights = append(insights, learning.Insight{
					ID:          uuid.New().String(),
					Type:        learning.InsightTypeWorkflow,
					Title:       "Common Tool Chain Detected",
					Description: pattern.Description,
					Confidence:  pattern.Confidence,
					Impact:      learning.ImpactMedium,
					Actionable:  true,
					Action:      "Consider creating a custom command for this tool chain",
					CreatedAt:   time.Now(),
					Data: map[string]interface{}{
						"pattern_id": pattern.ID,
						"frequency":  pattern.Frequency,
					},
				})
			}
		}
	}

	// Find error resolution patterns
	errorPatterns, err := r.storage.GetPatterns(ctx, learning.PatternTypeError)
	if err == nil {
		errorTypes := make(map[string]int)
		for _, pattern := range errorPatterns {
			for _, et := range pattern.Data.ErrorTypes {
				errorTypes[et]++
			}
		}

		for errorType, count := range errorTypes {
			if count >= 3 {
				insights = append(insights, learning.Insight{
					ID:          uuid.New().String(),
					Type:        learning.InsightTypeQuality,
					Title:       fmt.Sprintf("Recurring Error: %s", errorType),
					Description: fmt.Sprintf("This error has occurred %d times", count),
					Confidence:  0.9,
					Impact:      learning.ImpactHigh,
					Actionable:  true,
					Action:      "Review code to prevent this error",
					CreatedAt:   time.Now(),
					Data: map[string]interface{}{
						"error_type": errorType,
						"count":      count,
					},
				})
			}
		}
	}

	return insights, nil
}
