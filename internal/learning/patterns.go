// Package learning provides user pattern recognition and personalization.
// All data is stored locally by default with strong privacy controls.
package learning

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
	"sync"
	"time"
)

// PatternType categorizes learned patterns.
type PatternType string

const (
	PatternTypeCodingStyle   PatternType = "coding_style"
	PatternTypeToolUsage     PatternType = "tool_usage"
	PatternTypeWorkflow      PatternType = "workflow"
	PatternTypeCommunication PatternType = "communication"
	PatternTypeFile          PatternType = "file"
	PatternTypeTime          PatternType = "time"
	PatternTypeError         PatternType = "error"
	PatternTypeTask          PatternType = "task"
)

// Pattern represents a learned user behavior pattern.
type Pattern struct {
	ID          string      `json:"id"`
	Type        PatternType `json:"type"`
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	Frequency   int         `json:"frequency"`
	Confidence  float64     `json:"confidence"`
	LastSeen    time.Time   `json:"last_seen"`
	FirstSeen   time.Time   `json:"first_seen"`
	Data        PatternData `json:"data"`
}

// PatternData holds type-specific pattern information.
type PatternData struct {
	// Coding style
	IndentStyle     string   `json:"indent_style,omitempty"`     // "tabs" or "spaces"
	IndentSize      int      `json:"indent_size,omitempty"`      // 2, 4, etc.
	QuoteStyle      string   `json:"quote_style,omitempty"`      // "single" or "double"
	NamingStyle     string   `json:"naming_style,omitempty"`     // "camelCase", "snake_case", etc.
	CommentStyle    string   `json:"comment_style,omitempty"`    // comment preferences
	PreferredLangs  []string `json:"preferred_langs,omitempty"`  // frequently used languages
	PreferredFrames []string `json:"preferred_frames,omitempty"` // frameworks

	// Tool usage
	ToolName      string   `json:"tool_name,omitempty"`
	ToolFrequency int      `json:"tool_frequency,omitempty"`
	ToolArgs      []string `json:"tool_args,omitempty"` // common arguments

	// Workflow patterns
	StepSequence []string `json:"step_sequence,omitempty"`
	StartHour    int      `json:"start_hour,omitempty"` // typical work start
	EndHour      int      `json:"end_hour,omitempty"`   // typical work end
	PeakHours    []int    `json:"peak_hours,omitempty"` // most active hours

	// File patterns
	FileExtensions []string `json:"file_extensions,omitempty"`
	Directories    []string `json:"directories,omitempty"`

	// Communication
	Verbosity     string   `json:"verbosity,omitempty"` // "brief", "detailed"
	TechnicalLvl  string   `json:"technical_level,omitempty"`
	ResponseStyle string   `json:"response_style,omitempty"`
	Keywords      []string `json:"keywords,omitempty"` // common terms

	// Error patterns
	ErrorTypes     []string `json:"error_types,omitempty"`
	ResolutionPath string   `json:"resolution_path,omitempty"`

	// Generic key-value store
	Extra map[string]interface{} `json:"extra,omitempty"`
}

// PatternRecognizer identifies patterns from user interactions.
type PatternRecognizer struct {
	mu       sync.RWMutex
	patterns map[string]*Pattern
	profile  *UserProfile

	// Pattern detection config
	minFrequency  int
	minConfidence float64
	decayFactor   float64
}

// NewPatternRecognizer creates a new pattern recognizer.
func NewPatternRecognizer() *PatternRecognizer {
	return &PatternRecognizer{
		patterns:      make(map[string]*Pattern),
		minFrequency:  3,
		minConfidence: 0.6,
		decayFactor:   0.95,
	}
}

// SetProfile associates a user profile with this recognizer.
func (pr *PatternRecognizer) SetProfile(p *UserProfile) {
	pr.mu.Lock()
	defer pr.mu.Unlock()
	pr.profile = p
	if p != nil {
		for id, pattern := range p.Patterns {
			pr.patterns[id] = pattern
		}
	}
}

// RecordToolUsage tracks tool usage patterns.
func (pr *PatternRecognizer) RecordToolUsage(toolName string, args map[string]interface{}, success bool) {
	pr.mu.Lock()
	defer pr.mu.Unlock()

	id := pr.patternID(PatternTypeToolUsage, toolName)
	pattern, exists := pr.patterns[id]
	if !exists {
		pattern = &Pattern{
			ID:        id,
			Type:      PatternTypeToolUsage,
			Name:      toolName,
			FirstSeen: time.Now(),
			Data: PatternData{
				ToolName: toolName,
			},
		}
		pr.patterns[id] = pattern
	}

	pattern.Frequency++
	pattern.LastSeen = time.Now()
	pattern.Data.ToolFrequency = pattern.Frequency
	pattern.Confidence = pr.calculateConfidence(pattern.Frequency)
}

// RecordFileAccess tracks file usage patterns.
func (pr *PatternRecognizer) RecordFileAccess(path string, operation string) {
	pr.mu.Lock()
	defer pr.mu.Unlock()

	// Track file extensions
	ext := extractExtension(path)
	if ext != "" {
		id := pr.patternID(PatternTypeFile, "ext_"+ext)
		pattern, exists := pr.patterns[id]
		if !exists {
			pattern = &Pattern{
				ID:        id,
				Type:      PatternTypeFile,
				Name:      "File extension: " + ext,
				FirstSeen: time.Now(),
				Data: PatternData{
					FileExtensions: []string{ext},
				},
			}
			pr.patterns[id] = pattern
		}
		pattern.Frequency++
		pattern.LastSeen = time.Now()
		pattern.Confidence = pr.calculateConfidence(pattern.Frequency)
	}

	// Track directories
	dir := extractDirectory(path)
	if dir != "" {
		id := pr.patternID(PatternTypeFile, "dir_"+anonymizePath(dir))

		// Anonymize path if privacy settings require it
		storedDir := dir
		if pr.profile != nil && pr.profile.Privacy.AnonymizePaths {
			storedDir = anonymizePath(dir)
		}

		pattern, exists := pr.patterns[id]
		if !exists {
			pattern = &Pattern{
				ID:        id,
				Type:      PatternTypeFile,
				Name:      "Directory pattern",
				FirstSeen: time.Now(),
				Data: PatternData{
					Directories: []string{storedDir},
				},
			}
			pr.patterns[id] = pattern
		} else {
			// Add directory if not already present
			if !contains(pattern.Data.Directories, storedDir) {
				pattern.Data.Directories = append(pattern.Data.Directories, storedDir)
			}
		}
		pattern.Frequency++
		pattern.LastSeen = time.Now()
	}
}

// RecordTimeActivity tracks when user is active.
func (pr *PatternRecognizer) RecordTimeActivity() {
	pr.mu.Lock()
	defer pr.mu.Unlock()

	now := time.Now()
	hour := now.Hour()

	id := pr.patternID(PatternTypeTime, "activity_hours")
	pattern, exists := pr.patterns[id]
	if !exists {
		pattern = &Pattern{
			ID:        id,
			Type:      PatternTypeTime,
			Name:      "Activity hours",
			FirstSeen: now,
			Data: PatternData{
				Extra: map[string]interface{}{
					"hour_counts": make(map[int]int),
				},
			},
		}
		pr.patterns[id] = pattern
	}

	pattern.Frequency++
	pattern.LastSeen = now

	if pattern.Data.Extra == nil {
		pattern.Data.Extra = make(map[string]interface{})
	}
	hourCounts, _ := pattern.Data.Extra["hour_counts"].(map[int]int)
	if hourCounts == nil {
		hourCounts = make(map[int]int)
	}
	hourCounts[hour]++
	pattern.Data.Extra["hour_counts"] = hourCounts

	// Calculate peak hours
	pattern.Data.PeakHours = pr.calculatePeakHours(hourCounts)
}

func (pr *PatternRecognizer) RecordSession(sessionID string) {}

// RecordCodingStyle analyzes code to learn style preferences.
func (pr *PatternRecognizer) RecordCodingStyle(code string, language string) {
	pr.mu.Lock()
	defer pr.mu.Unlock()

	style := analyzeCodingStyle(code)

	id := pr.patternID(PatternTypeCodingStyle, language)
	pattern, exists := pr.patterns[id]
	if !exists {
		pattern = &Pattern{
			ID:        id,
			Type:      PatternTypeCodingStyle,
			Name:      "Coding style: " + language,
			FirstSeen: time.Now(),
			Data:      PatternData{},
		}
		pr.patterns[id] = pattern
	}

	pattern.Frequency++
	pattern.LastSeen = time.Now()

	// Update style preferences
	if style.IndentStyle != "" {
		pattern.Data.IndentStyle = style.IndentStyle
	}
	if style.IndentSize > 0 {
		pattern.Data.IndentSize = style.IndentSize
	}
	if style.QuoteStyle != "" {
		pattern.Data.QuoteStyle = style.QuoteStyle
	}
	if style.NamingStyle != "" {
		pattern.Data.NamingStyle = style.NamingStyle
	}

	// Track language preference
	pattern.Data.PreferredLangs = addUnique(pattern.Data.PreferredLangs, language)
}

// RecordWorkflow tracks sequences of actions.
func (pr *PatternRecognizer) RecordWorkflow(action string, sessionID string) {
	pr.mu.Lock()
	defer pr.mu.Unlock()

	id := pr.patternID(PatternTypeWorkflow, "session_"+sessionID)
	pattern, exists := pr.patterns[id]
	if !exists {
		pattern = &Pattern{
			ID:        id,
			Type:      PatternTypeWorkflow,
			Name:      "Workflow pattern",
			FirstSeen: time.Now(),
			Data:      PatternData{},
		}
		pr.patterns[id] = pattern
	}

	pattern.Frequency++
	pattern.LastSeen = time.Now()
	pattern.Data.StepSequence = append(pattern.Data.StepSequence, action)

	// Limit sequence length
	if len(pattern.Data.StepSequence) > 50 {
		pattern.Data.StepSequence = pattern.Data.StepSequence[len(pattern.Data.StepSequence)-50:]
	}
}

// RecordError tracks error patterns.
func (pr *PatternRecognizer) RecordError(errorType string, resolved bool, resolution string) {
	pr.mu.Lock()
	defer pr.mu.Unlock()

	id := pr.patternID(PatternTypeError, errorType)
	pattern, exists := pr.patterns[id]
	if !exists {
		pattern = &Pattern{
			ID:        id,
			Type:      PatternTypeError,
			Name:      "Error: " + errorType,
			FirstSeen: time.Now(),
			Data: PatternData{
				ErrorTypes: []string{errorType},
			},
		}
		pr.patterns[id] = pattern
	}

	pattern.Frequency++
	pattern.LastSeen = time.Now()

	if resolved && resolution != "" {
		pattern.Data.ResolutionPath = resolution
	}
}

// RecordCommunication analyzes communication preferences.
func (pr *PatternRecognizer) RecordCommunication(message string, isUserMessage bool) {
	pr.mu.Lock()
	defer pr.mu.Unlock()

	if !isUserMessage {
		return
	}

	id := pr.patternID(PatternTypeCommunication, "style")
	pattern, exists := pr.patterns[id]
	if !exists {
		pattern = &Pattern{
			ID:        id,
			Type:      PatternTypeCommunication,
			Name:      "Communication style",
			FirstSeen: time.Now(),
			Data:      PatternData{},
		}
		pr.patterns[id] = pattern
	}

	pattern.Frequency++
	pattern.LastSeen = time.Now()

	// Analyze message characteristics
	words := strings.Fields(message)
	wordCount := len(words)

	if pattern.Data.Extra == nil {
		pattern.Data.Extra = make(map[string]interface{})
	}

	// Update average message length
	avgLen, _ := pattern.Data.Extra["avg_word_count"].(float64)
	count, _ := pattern.Data.Extra["message_count"].(float64)
	newAvg := (avgLen*count + float64(wordCount)) / (count + 1)
	pattern.Data.Extra["avg_word_count"] = newAvg
	pattern.Data.Extra["message_count"] = count + 1

	// Determine verbosity
	if newAvg < 20 {
		pattern.Data.Verbosity = "brief"
	} else if newAvg < 50 {
		pattern.Data.Verbosity = "moderate"
	} else {
		pattern.Data.Verbosity = "detailed"
	}

	// Track technical keywords
	technicalKeywords := extractTechnicalKeywords(message)
	for _, kw := range technicalKeywords {
		pattern.Data.Keywords = addUnique(pattern.Data.Keywords, kw)
	}
	if len(pattern.Data.Keywords) > 50 {
		pattern.Data.Keywords = pattern.Data.Keywords[:50]
	}
}

// GetPatterns returns all patterns of a specific type.
func (pr *PatternRecognizer) GetPatterns(ptype PatternType) []*Pattern {
	pr.mu.RLock()
	defer pr.mu.RUnlock()

	var result []*Pattern
	for _, p := range pr.patterns {
		if p.Type == ptype {
			result = append(result, p)
		}
	}
	return result
}

// GetAllPatterns returns all learned patterns.
func (pr *PatternRecognizer) GetAllPatterns() map[string]*Pattern {
	pr.mu.RLock()
	defer pr.mu.RUnlock()

	result := make(map[string]*Pattern, len(pr.patterns))
	for k, v := range pr.patterns {
		result[k] = v
	}
	return result
}

// GetConfidentPatterns returns patterns above the confidence threshold.
func (pr *PatternRecognizer) GetConfidentPatterns() []*Pattern {
	pr.mu.RLock()
	defer pr.mu.RUnlock()

	var result []*Pattern
	for _, p := range pr.patterns {
		if p.Confidence >= pr.minConfidence && p.Frequency >= pr.minFrequency {
			result = append(result, p)
		}
	}
	return result
}

// DecayPatterns reduces confidence of old patterns over time.
func (pr *PatternRecognizer) DecayPatterns(threshold time.Duration) {
	pr.mu.Lock()
	defer pr.mu.Unlock()

	now := time.Now()
	for id, p := range pr.patterns {
		if now.Sub(p.LastSeen) > threshold {
			p.Confidence *= pr.decayFactor
			if p.Confidence < 0.1 && p.Frequency < pr.minFrequency {
				delete(pr.patterns, id)
			}
		}
	}
}

// patternID generates a unique pattern ID.
func (pr *PatternRecognizer) patternID(ptype PatternType, name string) string {
	h := sha256.Sum256([]byte(string(ptype) + ":" + name))
	return hex.EncodeToString(h[:8])
}

// calculateConfidence computes confidence based on frequency.
func (pr *PatternRecognizer) calculateConfidence(frequency int) float64 {
	if frequency <= 0 {
		return 0
	}
	// Logarithmic confidence that approaches 1.0
	conf := 1.0 - 1.0/float64(frequency+1)
	if conf > 1.0 {
		return 1.0
	}
	return conf
}

// calculatePeakHours determines the most active hours.
func (pr *PatternRecognizer) calculatePeakHours(hourCounts map[int]int) []int {
	if len(hourCounts) == 0 {
		return nil
	}

	// Find max count
	maxCount := 0
	for _, count := range hourCounts {
		if count > maxCount {
			maxCount = count
		}
	}

	// Hours with >50% of max are considered peak
	threshold := maxCount / 2
	var peaks []int
	for hour, count := range hourCounts {
		if count >= threshold {
			peaks = append(peaks, hour)
		}
	}
	return peaks
}

// StyleAnalysis holds analyzed style information.
type StyleAnalysis struct {
	IndentStyle string
	IndentSize  int
	QuoteStyle  string
	NamingStyle string
}

// analyzeCodingStyle extracts style preferences from code.
func analyzeCodingStyle(code string) StyleAnalysis {
	style := StyleAnalysis{}
	lines := strings.Split(code, "\n")

	tabCount := 0
	spaceCount := 0
	spaceSizes := make(map[int]int)

	for _, line := range lines {
		if len(line) == 0 {
			continue
		}
		// Check indentation
		if strings.HasPrefix(line, "\t") {
			tabCount++
		} else if strings.HasPrefix(line, " ") {
			spaceCount++
			// Count leading spaces
			spaces := 0
			for _, c := range line {
				if c == ' ' {
					spaces++
				} else {
					break
				}
			}
			if spaces > 0 && spaces <= 8 {
				spaceSizes[spaces]++
			}
		}
	}

	if tabCount > spaceCount {
		style.IndentStyle = "tabs"
	} else if spaceCount > 0 {
		style.IndentStyle = "spaces"
		// Find most common indent size
		maxCount := 0
		for size, count := range spaceSizes {
			if count > maxCount {
				maxCount = count
				style.IndentSize = size
			}
		}
	}

	// Check quote style
	singleQuotes := strings.Count(code, "'")
	doubleQuotes := strings.Count(code, "\"")
	if singleQuotes > doubleQuotes*2 {
		style.QuoteStyle = "single"
	} else if doubleQuotes > singleQuotes*2 {
		style.QuoteStyle = "double"
	}

	// Check naming style (basic detection)
	camelCase := regexp.MustCompile(`[a-z][a-zA-Z]*[A-Z][a-zA-Z]*`)
	snakeCase := regexp.MustCompile(`[a-z]+_[a-z]+`)

	camelMatches := len(camelCase.FindAllString(code, -1))
	snakeMatches := len(snakeCase.FindAllString(code, -1))

	if camelMatches > snakeMatches*2 {
		style.NamingStyle = "camelCase"
	} else if snakeMatches > camelMatches*2 {
		style.NamingStyle = "snake_case"
	}

	return style
}

// extractExtension returns the file extension without the dot.
func extractExtension(path string) string {
	idx := strings.LastIndex(path, ".")
	if idx == -1 || idx == len(path)-1 {
		return ""
	}
	return strings.ToLower(path[idx+1:])
}

// extractDirectory returns the directory component of a path.
func extractDirectory(path string) string {
	idx := strings.LastIndex(path, "/")
	if idx == -1 {
		return ""
	}
	return path[:idx]
}

// anonymizePath hashes the path for privacy.
func anonymizePath(path string) string {
	h := sha256.Sum256([]byte(path))
	return hex.EncodeToString(h[:4])
}

// addUnique adds an item to a slice if not already present.
func addUnique(slice []string, item string) []string {
	for _, s := range slice {
		if s == item {
			return slice
		}
	}
	return append(slice, item)
}

// contains checks if a slice contains an item.
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// extractTechnicalKeywords finds technical terms in text.
func extractTechnicalKeywords(text string) []string {
	// Common technical terms to look for
	patterns := []string{
		"api", "database", "server", "client", "test", "debug",
		"deploy", "build", "compile", "error", "bug", "fix",
		"refactor", "optimize", "cache", "async", "sync",
		"docker", "kubernetes", "git", "ci", "cd",
	}

	lower := strings.ToLower(text)
	var found []string
	for _, p := range patterns {
		if strings.Contains(lower, p) {
			found = append(found, p)
		}
	}
	return found
}
