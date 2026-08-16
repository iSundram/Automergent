package context

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ContextBreakdown captures the token composition of a prompt.
type ContextBreakdown struct {
	Timestamp        time.Time `json:"timestamp"`
	Model            string    `json:"model"`
	TotalTokens      int       `json:"total_tokens"`
	SystemPrompt     int       `json:"system_prompt"`
	ToolDefinitions  int       `json:"tool_definitions"`
	Conversation     int       `json:"conversation"`
	ContextFiles     int       `json:"context_files"`
	ToolCalls        int       `json:"tool_calls"`
	Thinking         int       `json:"thinking"`
	OutputReserve    int       `json:"output_reserve"`
	SafetyMargin     int       `json:"safety_margin"`
	ProviderActual   int       `json:"provider_actual,omitempty"`
	EstimationWeight float64   `json:"estimation_weight,omitempty"`
}

// CompactionEvent records a compaction action.
type CompactionEvent struct {
	Timestamp   time.Time        `json:"timestamp"`
	Reason      CompactionReason `json:"reason"`
	Phase       CompactionPhase  `json:"phase"`
	Strategy    CompactionStrategy `json:"strategy"`
	TokensBefore int            `json:"tokens_before"`
	TokensAfter  int            `json:"tokens_after"`
	DurationMs  int64           `json:"duration_ms"`
	Success     bool            `json:"success"`
	Error       string          `json:"error,omitempty"`
}

// UsageEvent tracks token usage and cost.
type UsageEvent struct {
	Timestamp    time.Time `json:"timestamp"`
	Model        string    `json:"model"`
	InputTokens  int       `json:"input_tokens"`
	OutputTokens int       `json:"output_tokens"`
	TotalTokens  int       `json:"total_tokens"`
	CacheHits    int       `json:"cache_hits"`
	CostUSD      float64   `json:"cost_usd,omitempty"`
}

// TelemetryCollector aggregates context and usage telemetry.
type TelemetryCollector struct {
	mu              sync.RWMutex
	breakdowns      []ContextBreakdown
	compactionEvents []CompactionEvent
	usageEvents     []UsageEvent
	maxEvents       int
	path            string
	costTracker     *CostTracker
}

// NewTelemetryCollector creates a telemetry collector with persistence.
func NewTelemetryCollector(rootDir string, maxEvents int) *TelemetryCollector {
	dir := filepath.Join(rootDir, ".automergent", "telemetry")
	return &TelemetryCollector{
		breakdowns:       make([]ContextBreakdown, 0, maxEvents),
		compactionEvents: make([]CompactionEvent, 0, maxEvents),
		usageEvents:      make([]UsageEvent, 0, maxEvents),
		maxEvents:        maxEvents,
		path:             dir,
		costTracker:      NewCostTracker(),
	}
}

// RecordBreakdown records a context breakdown.
func (tc *TelemetryCollector) RecordBreakdown(b ContextBreakdown) {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	tc.breakdowns = append(tc.breakdowns, b)
	if len(tc.breakdowns) > tc.maxEvents {
		tc.breakdowns = tc.breakdowns[len(tc.breakdowns)-tc.maxEvents:]
	}
	_ = tc.flushBreakdowns()
}

// RecordCompaction records a compaction event.
func (tc *TelemetryCollector) RecordCompaction(e CompactionEvent) {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	tc.compactionEvents = append(tc.compactionEvents, e)
	if len(tc.compactionEvents) > tc.maxEvents {
		tc.compactionEvents = tc.compactionEvents[len(tc.compactionEvents)-tc.maxEvents:]
	}
	_ = tc.flushCompactionEvents()
}

// RecordUsage records token usage and cost.
func (tc *TelemetryCollector) RecordUsage(e UsageEvent) {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	tc.usageEvents = append(tc.usageEvents, e)
	tc.costTracker.Add(e.Model, e.InputTokens, e.OutputTokens)
	if len(tc.usageEvents) > tc.maxEvents {
		tc.usageEvents = tc.usageEvents[len(tc.usageEvents)-tc.maxEvents:]
	}
	_ = tc.flushUsageEvents()
}

// GetBreakdowns returns recent context breakdowns.
func (tc *TelemetryCollector) GetBreakdowns(limit int) []ContextBreakdown {
	tc.mu.RLock()
	defer tc.mu.RUnlock()
	if limit <= 0 || limit > len(tc.breakdowns) {
		limit = len(tc.breakdowns)
	}
	start := len(tc.breakdowns) - limit
	result := make([]ContextBreakdown, limit)
	copy(result, tc.breakdowns[start:])
	return result
}

// GetCompactionEvents returns recent compaction events.
func (tc *TelemetryCollector) GetCompactionEvents(limit int) []CompactionEvent {
	tc.mu.RLock()
	defer tc.mu.RUnlock()
	if limit <= 0 || limit > len(tc.compactionEvents) {
		limit = len(tc.compactionEvents)
	}
	start := len(tc.compactionEvents) - limit
	result := make([]CompactionEvent, limit)
	copy(result, tc.compactionEvents[start:])
	return result
}

// GetUsageEvents returns recent usage events.
func (tc *TelemetryCollector) GetUsageEvents(limit int) []UsageEvent {
	tc.mu.RLock()
	defer tc.mu.RUnlock()
	if limit <= 0 || limit > len(tc.usageEvents) {
		limit = len(tc.usageEvents)
	}
	start := len(tc.usageEvents) - limit
	result := make([]UsageEvent, limit)
	copy(result, tc.usageEvents[start:])
	return result
}

// GetCostSummary returns cost summary.
func (tc *TelemetryCollector) GetCostSummary() CostSummary {
	tc.mu.RLock()
	defer tc.mu.RUnlock()
	return tc.costTracker.Summary()
}

// --- Persistence ---

func (tc *TelemetryCollector) flushBreakdowns() error {
	if tc.path == "" {
		return nil
	}
	data, _ := json.MarshalIndent(tc.breakdowns, "", "  ")
	return atomicWriteFile(filepath.Join(tc.path, "breakdowns.json"), data, 0o600)
}

func (tc *TelemetryCollector) flushCompactionEvents() error {
	if tc.path == "" {
		return nil
	}
	data, _ := json.MarshalIndent(tc.compactionEvents, "", "  ")
	return atomicWriteFile(filepath.Join(tc.path, "compaction_events.json"), data, 0o600)
}

func (tc *TelemetryCollector) flushUsageEvents() error {
	if tc.path == "" {
		return nil
	}
	data, _ := json.MarshalIndent(tc.usageEvents, "", "  ")
	return atomicWriteFile(filepath.Join(tc.path, "usage_events.json"), data, 0o600)
}

// Load loads persisted telemetry.
func (tc *TelemetryCollector) Load() error {
	if tc.path == "" {
		return nil
	}

	breakdownsPath := filepath.Join(tc.path, "breakdowns.json")
	if data, err := os.ReadFile(breakdownsPath); err == nil {
		_ = json.Unmarshal(data, &tc.breakdowns)
	}

	compactionPath := filepath.Join(tc.path, "compaction_events.json")
	if data, err := os.ReadFile(compactionPath); err == nil {
		_ = json.Unmarshal(data, &tc.compactionEvents)
	}

	usagePath := filepath.Join(tc.path, "usage_events.json")
	if data, err := os.ReadFile(usagePath); err == nil {
		_ = json.Unmarshal(data, &tc.usageEvents)
		for _, e := range tc.usageEvents {
			tc.costTracker.Add(e.Model, e.InputTokens, e.OutputTokens)
		}
	}

	return nil
}

// --- Cost Tracking ---

// CostTracker maintains per-model cost accumulation.
type CostTracker struct {
	mu       sync.RWMutex
	models   map[string]*ModelCost
	pricing  map[string]ModelPricing
}

// ModelPricing defines cost per 1K tokens.
type ModelPricing struct {
	InputPer1K  float64 `json:"input_per_1k"`
	OutputPer1K float64 `json:"output_per_1k"`
}

// ModelCost tracks cost for a single model.
type ModelCost struct {
	Model        string  `json:"model"`
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	TotalCost    float64 `json:"total_cost_usd"`
}

// CostSummary aggregates cost across models.
type CostSummary struct {
	TotalCostUSD     float64            `json:"total_cost_usd"`
	TotalInputTokens int                `json:"total_input_tokens"`
	TotalOutputTokens int               `json:"total_output_tokens"`
	ByModel          map[string]ModelCost `json:"by_model"`
}

// DefaultPricing returns known model pricing (as of 2024).
func DefaultPricing() map[string]ModelPricing {
	return map[string]ModelPricing{
		"gemini-3.6-flash":   {InputPer1K: 0.000075, OutputPer1K: 0.0003},
		"gemini-3.6-pro":     {InputPer1K: 0.000125, OutputPer1K: 0.0005},
		"gemini-2.5-pro":     {InputPer1K: 0.000125, OutputPer1K: 0.0005},
		"gemini-2.5-flash":   {InputPer1K: 0.000075, OutputPer1K: 0.0003},
		"gpt-4o":             {InputPer1K: 0.005, OutputPer1K: 0.015},
		"gpt-4o-mini":        {InputPer1K: 0.00015, OutputPer1K: 0.0006},
		"claude-3.5-sonnet":  {InputPer1K: 0.003, OutputPer1K: 0.015},
		"claude-3.5-haiku":   {InputPer1K: 0.00025, OutputPer1K: 0.00125},
	}
}

// NewCostTracker creates a cost tracker with default pricing.
func NewCostTracker() *CostTracker {
	return &CostTracker{
		models:  make(map[string]*ModelCost),
		pricing: DefaultPricing(),
	}
}

// Add records usage for a model.
func (ct *CostTracker) Add(model string, inputTokens, outputTokens int) {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	if ct.models[model] == nil {
		ct.models[model] = &ModelCost{Model: model}
	}
	mc := ct.models[model]
	mc.InputTokens += inputTokens
	mc.OutputTokens += outputTokens

	if pricing, ok := ct.pricing[model]; ok {
		mc.TotalCost += float64(inputTokens) / 1000.0 * pricing.InputPer1K
		mc.TotalCost += float64(outputTokens) / 1000.0 * pricing.OutputPer1K
	}
}

// Summary returns the cost summary.
func (ct *CostTracker) Summary() CostSummary {
	ct.mu.RLock()
	defer ct.mu.RUnlock()

	summary := CostSummary{
		ByModel: make(map[string]ModelCost, len(ct.models)),
	}
	for model, mc := range ct.models {
		summary.TotalCostUSD += mc.TotalCost
		summary.TotalInputTokens += mc.InputTokens
		summary.TotalOutputTokens += mc.OutputTokens
		summary.ByModel[model] = *mc
	}
	return summary
}

// SetPricing updates pricing for a model.
func (ct *CostTracker) SetPricing(model string, p ModelPricing) {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	ct.pricing[model] = p
}