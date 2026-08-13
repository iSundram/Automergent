package config

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// AutoTuner provides automatic configuration tuning based on usage patterns.
type AutoTuner struct {
	mu sync.RWMutex

	// Metrics collected during operation
	metrics *TuningMetrics

	// Current tuning state
	state *TuningState

	// Persistence path
	dataPath string

	// Configuration
	enabled      bool
	learningRate float64
	minSamples   int
	updatePeriod time.Duration

	// Callbacks
	onTune []func(TuningRecommendation)
}

// TuningMetrics holds performance and usage metrics.
type TuningMetrics struct {
	// Session metrics
	SessionCount       int           `json:"sessionCount"`
	AverageSessionTime time.Duration `json:"avgSessionTime"`
	TotalSessionTime   time.Duration `json:"totalSessionTime"`

	// Context usage
	ContextUsageSamples []float64 `json:"contextUsage"`
	ContextWarnings     int       `json:"contextWarnings"`
	CompressionEvents   int       `json:"compressionEvents"`

	// Performance
	ResponseTimes      []time.Duration `json:"responseTimes"`
	TokensPerSecond    []float64       `json:"tokensPerSecond"`
	MemoryUsageSamples []int64         `json:"memoryUsage"`

	// Provider usage
	ProviderUsage   map[string]int             `json:"providerUsage"`
	ProviderLatency map[string][]time.Duration `json:"providerLatency"`
	ProviderErrors  map[string]int             `json:"providerErrors"`

	// Tool usage
	ToolUsage   map[string]int             `json:"toolUsage"`
	ToolLatency map[string][]time.Duration `json:"toolLatency"`
	ToolErrors  map[string]int             `json:"toolErrors"`

	// File patterns
	FileReadSizes   []int64 `json:"fileReadSizes"`
	TreeFileCounts  []int   `json:"treeFileCounts"`
	ExcludedMatches int     `json:"excludedMatches"`

	// User behavior
	ModeUsage    map[string]int `json:"modeUsage"`
	ThemeUsage   map[string]int `json:"themeUsage"`
	FeatureUsage map[string]int `json:"featureUsage"`

	// Timestamps
	FirstSeen time.Time `json:"firstSeen"`
	LastSeen  time.Time `json:"lastSeen"`
}

// TuningState holds the current auto-tuning state.
type TuningState struct {
	// Applied recommendations
	AppliedRecommendations []AppliedRecommendation `json:"appliedRecommendations"`

	// Performance baselines
	BaselineResponseTime time.Duration `json:"baselineResponseTime"`
	BaselineTokensPerSec float64       `json:"baselineTokensPerSec"`

	// Learning state
	Iteration      int       `json:"iteration"`
	LastTuneTime   time.Time `json:"lastTuneTime"`
	TuningScore    float64   `json:"tuningScore"`
	StabilityScore float64   `json:"stabilityScore"`
}

// TuningRecommendation represents a tuning recommendation.
type TuningRecommendation struct {
	Field      string  `json:"field"`
	CurrentVal any     `json:"current"`
	SuggestVal any     `json:"suggested"`
	Reason     string  `json:"reason"`
	Impact     string  `json:"impact"`
	Confidence float64 `json:"confidence"`
	AutoApply  bool    `json:"autoApply"`
}

// AppliedRecommendation records a recommendation that was applied.
type AppliedRecommendation struct {
	Recommendation TuningRecommendation `json:"recommendation"`
	AppliedAt      time.Time            `json:"appliedAt"`
	PreviousValue  any                  `json:"previousValue"`
	Outcome        string               `json:"outcome"`
}

// NewAutoTuner creates a new auto-tuner.
func NewAutoTuner() (*AutoTuner, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("get home dir: %w", err)
	}

	dataPath := filepath.Join(home, ".automergent", "tuning")
	if err := os.MkdirAll(dataPath, 0o755); err != nil {
		return nil, fmt.Errorf("create tuning dir: %w", err)
	}

	at := &AutoTuner{
		dataPath:     dataPath,
		enabled:      true,
		learningRate: 0.1,
		minSamples:   10,
		updatePeriod: 24 * time.Hour,
	}

	// Load existing state
	at.loadState()
	at.loadMetrics()

	return at, nil
}

// loadState loads the tuning state from disk.
func (at *AutoTuner) loadState() {
	path := filepath.Join(at.dataPath, "state.json")
	content, err := os.ReadFile(path)
	if err != nil {
		at.state = &TuningState{}
		return
	}

	var state TuningState
	if err := json.Unmarshal(content, &state); err != nil {
		at.state = &TuningState{}
		return
	}

	at.state = &state
}

// loadMetrics loads metrics from disk.
func (at *AutoTuner) loadMetrics() {
	path := filepath.Join(at.dataPath, "metrics.json")
	content, err := os.ReadFile(path)
	if err != nil {
		at.metrics = at.newMetrics()
		return
	}

	var metrics TuningMetrics
	if err := json.Unmarshal(content, &metrics); err != nil {
		at.metrics = at.newMetrics()
		return
	}

	at.metrics = &metrics
}

// newMetrics creates new empty metrics.
func (at *AutoTuner) newMetrics() *TuningMetrics {
	return &TuningMetrics{
		ProviderUsage:   make(map[string]int),
		ProviderLatency: make(map[string][]time.Duration),
		ProviderErrors:  make(map[string]int),
		ToolUsage:       make(map[string]int),
		ToolLatency:     make(map[string][]time.Duration),
		ToolErrors:      make(map[string]int),
		ModeUsage:       make(map[string]int),
		ThemeUsage:      make(map[string]int),
		FeatureUsage:    make(map[string]int),
		FirstSeen:       time.Now(),
		LastSeen:        time.Now(),
	}
}

// Save persists current state and metrics.
func (at *AutoTuner) Save() error {
	at.mu.RLock()
	defer at.mu.RUnlock()

	// Save state
	stateData, err := json.MarshalIndent(at.state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}
	if err := os.WriteFile(filepath.Join(at.dataPath, "state.json"), stateData, 0o644); err != nil {
		return fmt.Errorf("write state: %w", err)
	}

	// Save metrics
	metricsData, err := json.MarshalIndent(at.metrics, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal metrics: %w", err)
	}
	if err := os.WriteFile(filepath.Join(at.dataPath, "metrics.json"), metricsData, 0o644); err != nil {
		return fmt.Errorf("write metrics: %w", err)
	}

	return nil
}

// RecordSession records session metrics.
func (at *AutoTuner) RecordSession(duration time.Duration) {
	at.mu.Lock()
	defer at.mu.Unlock()

	at.metrics.SessionCount++
	at.metrics.TotalSessionTime += duration
	at.metrics.AverageSessionTime = at.metrics.TotalSessionTime / time.Duration(at.metrics.SessionCount)
	at.metrics.LastSeen = time.Now()
}

// RecordContextUsage records context window usage.
func (at *AutoTuner) RecordContextUsage(fraction float64) {
	at.mu.Lock()
	defer at.mu.Unlock()

	at.metrics.ContextUsageSamples = append(at.metrics.ContextUsageSamples, fraction)
	// Keep last 1000 samples
	if len(at.metrics.ContextUsageSamples) > 1000 {
		at.metrics.ContextUsageSamples = at.metrics.ContextUsageSamples[1:]
	}

	if fraction >= 0.8 {
		at.metrics.ContextWarnings++
	}
}

// RecordCompression records a context compression event.
func (at *AutoTuner) RecordCompression() {
	at.mu.Lock()
	defer at.mu.Unlock()
	at.metrics.CompressionEvents++
}

// RecordResponseTime records AI response latency.
func (at *AutoTuner) RecordResponseTime(d time.Duration) {
	at.mu.Lock()
	defer at.mu.Unlock()

	at.metrics.ResponseTimes = append(at.metrics.ResponseTimes, d)
	if len(at.metrics.ResponseTimes) > 1000 {
		at.metrics.ResponseTimes = at.metrics.ResponseTimes[1:]
	}
}

// RecordTokensPerSecond records token generation rate.
func (at *AutoTuner) RecordTokensPerSecond(rate float64) {
	at.mu.Lock()
	defer at.mu.Unlock()

	at.metrics.TokensPerSecond = append(at.metrics.TokensPerSecond, rate)
	if len(at.metrics.TokensPerSecond) > 1000 {
		at.metrics.TokensPerSecond = at.metrics.TokensPerSecond[1:]
	}
}

// RecordProviderUsage records provider usage.
func (at *AutoTuner) RecordProviderUsage(provider string, latency time.Duration, err error) {
	at.mu.Lock()
	defer at.mu.Unlock()

	at.metrics.ProviderUsage[provider]++

	if at.metrics.ProviderLatency[provider] == nil {
		at.metrics.ProviderLatency[provider] = []time.Duration{}
	}
	at.metrics.ProviderLatency[provider] = append(at.metrics.ProviderLatency[provider], latency)

	if err != nil {
		at.metrics.ProviderErrors[provider]++
	}
}

// RecordToolUsage records tool usage.
func (at *AutoTuner) RecordToolUsage(tool string, latency time.Duration, err error) {
	at.mu.Lock()
	defer at.mu.Unlock()

	at.metrics.ToolUsage[tool]++

	if at.metrics.ToolLatency[tool] == nil {
		at.metrics.ToolLatency[tool] = []time.Duration{}
	}
	at.metrics.ToolLatency[tool] = append(at.metrics.ToolLatency[tool], latency)

	if err != nil {
		at.metrics.ToolErrors[tool]++
	}
}

// RecordFileRead records file read sizes.
func (at *AutoTuner) RecordFileRead(size int64) {
	at.mu.Lock()
	defer at.mu.Unlock()

	at.metrics.FileReadSizes = append(at.metrics.FileReadSizes, size)
	if len(at.metrics.FileReadSizes) > 1000 {
		at.metrics.FileReadSizes = at.metrics.FileReadSizes[1:]
	}
}

// RecordTreeFiles records directory tree file counts.
func (at *AutoTuner) RecordTreeFiles(count int) {
	at.mu.Lock()
	defer at.mu.Unlock()

	at.metrics.TreeFileCounts = append(at.metrics.TreeFileCounts, count)
	if len(at.metrics.TreeFileCounts) > 100 {
		at.metrics.TreeFileCounts = at.metrics.TreeFileCounts[1:]
	}
}

// RecordModeUsage records mode usage.
func (at *AutoTuner) RecordModeUsage(mode string) {
	at.mu.Lock()
	defer at.mu.Unlock()
	at.metrics.ModeUsage[mode]++
}

// RecordFeatureUsage records feature usage.
func (at *AutoTuner) RecordFeatureUsage(feature string) {
	at.mu.Lock()
	defer at.mu.Unlock()
	at.metrics.FeatureUsage[feature]++
}

// Analyze generates tuning recommendations based on collected metrics.
func (at *AutoTuner) Analyze(cfg *Config) []TuningRecommendation {
	at.mu.RLock()
	defer at.mu.RUnlock()

	var recs []TuningRecommendation

	// Context window tuning
	if rec := at.analyzeContextUsage(cfg); rec != nil {
		recs = append(recs, *rec)
	}

	// File size tuning
	if rec := at.analyzeFileSizes(cfg); rec != nil {
		recs = append(recs, *rec)
	}

	// Tree depth tuning
	if rec := at.analyzeTreeFiles(cfg); rec != nil {
		recs = append(recs, *rec)
	}

	// Provider tuning
	if rec := at.analyzeProviders(cfg); rec != nil {
		recs = append(recs, *rec)
	}

	// Compression tuning
	if rec := at.analyzeCompression(cfg); rec != nil {
		recs = append(recs, *rec)
	}

	// Mode preference
	if rec := at.analyzeModePreference(cfg); rec != nil {
		recs = append(recs, *rec)
	}

	return recs
}

// analyzeContextUsage recommends context token adjustments.
func (at *AutoTuner) analyzeContextUsage(cfg *Config) *TuningRecommendation {
	if len(at.metrics.ContextUsageSamples) < at.minSamples {
		return nil
	}

	avg := average(at.metrics.ContextUsageSamples)
	max := maximum(at.metrics.ContextUsageSamples)

	// If consistently using less than 50% of context, suggest smaller window
	if max < 0.5 && avg < 0.3 {
		suggested := int(float64(cfg.MaxContextTokens) * 0.5)
		return &TuningRecommendation{
			Field:      "maxContextTokens",
			CurrentVal: cfg.MaxContextTokens,
			SuggestVal: suggested,
			Reason:     fmt.Sprintf("Average context usage is %.1f%%, max %.1f%%", avg*100, max*100),
			Impact:     "Reduces memory usage and may improve response times",
			Confidence: 0.8,
			AutoApply:  false,
		}
	}

	// If hitting warnings frequently, suggest larger window
	warningRate := float64(at.metrics.ContextWarnings) / float64(len(at.metrics.ContextUsageSamples))
	if warningRate > 0.2 && avg > 0.7 {
		suggested := int(float64(cfg.MaxContextTokens) * 1.5)
		if suggested > 200000 {
			suggested = 200000
		}
		return &TuningRecommendation{
			Field:      "maxContextTokens",
			CurrentVal: cfg.MaxContextTokens,
			SuggestVal: suggested,
			Reason:     fmt.Sprintf("Context warnings in %.1f%% of sessions", warningRate*100),
			Impact:     "Reduces need for context compression",
			Confidence: 0.7,
			AutoApply:  false,
		}
	}

	return nil
}

// analyzeFileSizes recommends file size limit adjustments.
func (at *AutoTuner) analyzeFileSizes(cfg *Config) *TuningRecommendation {
	if len(at.metrics.FileReadSizes) < at.minSamples {
		return nil
	}

	// Find 95th percentile file size
	sorted := make([]int64, len(at.metrics.FileReadSizes))
	copy(sorted, at.metrics.FileReadSizes)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	p95 := sorted[int(float64(len(sorted))*0.95)]

	// If 95th percentile is much smaller than limit, suggest reduction
	if float64(p95) < float64(cfg.MaxAutoReadFileSize)*0.3 {
		suggested := int(float64(p95) * 1.5)
		if suggested < 64*1024 {
			suggested = 64 * 1024 // Min 64KB
		}
		return &TuningRecommendation{
			Field:      "maxAutoReadFileSize",
			CurrentVal: cfg.MaxAutoReadFileSize,
			SuggestVal: suggested,
			Reason:     fmt.Sprintf("95%% of files are smaller than %dKB", p95/1024),
			Impact:     "Faster file processing",
			Confidence: 0.75,
			AutoApply:  true,
		}
	}

	return nil
}

// analyzeTreeFiles recommends tree file limit adjustments.
func (at *AutoTuner) analyzeTreeFiles(cfg *Config) *TuningRecommendation {
	if len(at.metrics.TreeFileCounts) < 5 {
		return nil
	}

	max := 0
	for _, count := range at.metrics.TreeFileCounts {
		if count > max {
			max = count
		}
	}

	// If max is much lower than limit, suggest reduction
	if max < cfg.MaxTreeFiles/2 {
		suggested := max * 2
		if suggested < 100 {
			suggested = 100
		}
		return &TuningRecommendation{
			Field:      "maxTreeFiles",
			CurrentVal: cfg.MaxTreeFiles,
			SuggestVal: suggested,
			Reason:     fmt.Sprintf("Max observed tree size is %d files", max),
			Impact:     "Faster directory scanning",
			Confidence: 0.7,
			AutoApply:  true,
		}
	}

	return nil
}

// analyzeProviders recommends provider changes based on performance.
func (at *AutoTuner) analyzeProviders(cfg *Config) *TuningRecommendation {
	if len(at.metrics.ProviderLatency) < 2 {
		return nil
	}

	// Find provider with best latency/error ratio
	type providerScore struct {
		name       string
		avgLatency time.Duration
		errorRate  float64
		score      float64
	}

	var scores []providerScore
	for provider, latencies := range at.metrics.ProviderLatency {
		if len(latencies) < 5 {
			continue
		}

		avgLatency := averageDuration(latencies)
		errorRate := float64(at.metrics.ProviderErrors[provider]) / float64(at.metrics.ProviderUsage[provider])

		// Score: lower is better (latency in seconds + error penalty)
		score := avgLatency.Seconds() + (errorRate * 5)

		scores = append(scores, providerScore{
			name:       provider,
			avgLatency: avgLatency,
			errorRate:  errorRate,
			score:      score,
		})
	}

	if len(scores) < 2 {
		return nil
	}

	sort.Slice(scores, func(i, j int) bool { return scores[i].score < scores[j].score })
	best := scores[0]
	current := cfg.Provider

	// Only suggest if significantly better (30%+)
	var currentScore *providerScore
	for _, s := range scores {
		if s.name == current {
			currentScore = &s
			break
		}
	}

	if currentScore != nil && best.name != current && best.score < currentScore.score*0.7 {
		return &TuningRecommendation{
			Field:      "provider",
			CurrentVal: current,
			SuggestVal: best.name,
			Reason:     fmt.Sprintf("%s has %.0f%% faster response times", best.name, (1-best.avgLatency.Seconds()/currentScore.avgLatency.Seconds())*100),
			Impact:     "Improved response times",
			Confidence: 0.6,
			AutoApply:  false,
		}
	}

	return nil
}

// analyzeCompression recommends compression threshold adjustments.
func (at *AutoTuner) analyzeCompression(cfg *Config) *TuningRecommendation {
	if at.metrics.CompressionEvents < 5 {
		return nil
	}

	compressionRate := float64(at.metrics.CompressionEvents) / float64(at.metrics.SessionCount)

	// If compressing too often, raise threshold
	if compressionRate > 0.5 && cfg.AutoCompressAt < 0.95 {
		return &TuningRecommendation{
			Field:      "autoCompressAt",
			CurrentVal: cfg.AutoCompressAt,
			SuggestVal: 0.95,
			Reason:     fmt.Sprintf("Compression occurs in %.0f%% of sessions", compressionRate*100),
			Impact:     "Fewer interruptions from compression",
			Confidence: 0.7,
			AutoApply:  true,
		}
	}

	return nil
}

// analyzeModePreference recommends mode based on usage patterns.
func (at *AutoTuner) analyzeModePreference(cfg *Config) *TuningRecommendation {
	if len(at.metrics.ModeUsage) < 2 {
		return nil
	}

	// Find most used mode
	var topMode string
	var topCount int
	var totalCount int

	for mode, count := range at.metrics.ModeUsage {
		totalCount += count
		if count > topCount {
			topCount = count
			topMode = mode
		}
	}

	// If a mode is used 70%+ of the time and it's not the default, suggest it
	if float64(topCount)/float64(totalCount) > 0.7 && topMode != cfg.Mode {
		return &TuningRecommendation{
			Field:      "mode",
			CurrentVal: cfg.Mode,
			SuggestVal: topMode,
			Reason:     fmt.Sprintf("You use %s mode %.0f%% of the time", topMode, float64(topCount)/float64(totalCount)*100),
			Impact:     "Sets your preferred mode as default",
			Confidence: 0.85,
			AutoApply:  false,
		}
	}

	return nil
}

// ApplyRecommendation applies a recommendation to the config.
func (at *AutoTuner) ApplyRecommendation(cfg *Config, rec TuningRecommendation) error {
	at.mu.Lock()
	defer at.mu.Unlock()

	// Apply the change
	if err := SetConfigField(cfg, rec.Field, rec.SuggestVal); err != nil {
		return fmt.Errorf("apply recommendation: %w", err)
	}

	// Record the application
	at.state.AppliedRecommendations = append(at.state.AppliedRecommendations, AppliedRecommendation{
		Recommendation: rec,
		AppliedAt:      time.Now(),
		PreviousValue:  rec.CurrentVal,
		Outcome:        "pending",
	})

	at.state.LastTuneTime = time.Now()
	at.state.Iteration++

	return nil
}

// GetAppliedRecommendations returns history of applied recommendations.
func (at *AutoTuner) GetAppliedRecommendations() []AppliedRecommendation {
	at.mu.RLock()
	defer at.mu.RUnlock()
	return at.state.AppliedRecommendations
}

// OnTune registers a callback for tuning recommendations.
func (at *AutoTuner) OnTune(fn func(TuningRecommendation)) {
	at.mu.Lock()
	defer at.mu.Unlock()
	at.onTune = append(at.onTune, fn)
}

// Reset clears all metrics and state.
func (at *AutoTuner) Reset() error {
	at.mu.Lock()
	defer at.mu.Unlock()

	at.metrics = at.newMetrics()
	at.state = &TuningState{}

	return at.Save()
}

// ExportMetrics exports metrics to a file.
func (at *AutoTuner) ExportMetrics(path string) error {
	at.mu.RLock()
	defer at.mu.RUnlock()

	data, err := yaml.Marshal(at.metrics)
	if err != nil {
		return fmt.Errorf("marshal metrics: %w", err)
	}

	return os.WriteFile(path, data, 0o644)
}

// Utility functions

func average(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	var sum float64
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

func maximum(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	max := values[0]
	for _, v := range values[1:] {
		if v > max {
			max = v
		}
	}
	return max
}

func averageDuration(values []time.Duration) time.Duration {
	if len(values) == 0 {
		return 0
	}
	var sum time.Duration
	for _, v := range values {
		sum += v
	}
	return sum / time.Duration(len(values))
}

// stdDev calculates standard deviation.
func stdDev(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	avg := average(values)
	var sumSq float64
	for _, v := range values {
		diff := v - avg
		sumSq += diff * diff
	}
	return math.Sqrt(sumSq / float64(len(values)))
}
