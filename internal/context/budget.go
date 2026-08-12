package context

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
)

// ModelTokenLimits defines context limits for different model tiers.
type ModelTokenLimits struct {
	Name          string
	ContextWindow int
	MaxOutput     int
}

// Common model token limits.
var (
	ModelLimitsGeminiFlash = ModelTokenLimits{Name: "gemini-3.6-flash", ContextWindow: 1_048_576, MaxOutput: 64_000}
	ModelLimitsGeminiPro   = ModelTokenLimits{Name: "gemini-3.6-pro", ContextWindow: 2_097_152, MaxOutput: 128_000}
	ModelLimitsDefault     = ModelTokenLimits{Name: "default", ContextWindow: 128_000, MaxOutput: 8_192}
)

// TokenBudget manages token allocation across different context components.
type TokenBudget struct {
	mu sync.RWMutex

	// Total available tokens for context
	TotalBudget int `json:"total_budget"`

	// Reserved allocations
	SystemPromptReserve   int `json:"system_prompt_reserve"`
	ToolDefinitionReserve int `json:"tool_definition_reserve"`
	OutputReserve         int `json:"output_reserve"`
	SafetyMargin          int `json:"safety_margin"`

	// Current usage
	SystemPromptUsed   int `json:"system_prompt_used"`
	ToolDefinitionUsed int `json:"tool_definition_used"`
	ConversationUsed   int `json:"conversation_used"`
	ContextFilesUsed   int `json:"context_files_used"`
}

// BudgetAllocation represents how tokens should be distributed.
type BudgetAllocation struct {
	MaxSystemPrompt int `json:"max_system_prompt"`
	MaxToolDefs     int `json:"max_tool_defs"`
	MaxConversation int `json:"max_conversation"`
	MaxContextFiles int `json:"max_context_files"`
	MaxOutput       int `json:"max_output"`
	Available       int `json:"available"`
}

// TokenBudgetConfig configures budget allocation percentages.
type TokenBudgetConfig struct {
	SystemPromptPercent   float64 `json:"system_prompt_percent"`   // e.g., 0.05 for 5%
	ToolDefinitionPercent float64 `json:"tool_definition_percent"` // e.g., 0.05
	ConversationPercent   float64 `json:"conversation_percent"`    // e.g., 0.40
	ContextFilesPercent   float64 `json:"context_files_percent"`   // e.g., 0.30
	OutputPercent         float64 `json:"output_percent"`          // e.g., 0.15
	SafetyMarginPercent   float64 `json:"safety_margin_percent"`   // e.g., 0.05
}

// DefaultBudgetConfig returns sensible default allocation.
func DefaultBudgetConfig() TokenBudgetConfig {
	return TokenBudgetConfig{
		SystemPromptPercent:   0.05,
		ToolDefinitionPercent: 0.05,
		ConversationPercent:   0.40,
		ContextFilesPercent:   0.30,
		OutputPercent:         0.15,
		SafetyMarginPercent:   0.05,
	}
}

// StreamingBudgetConfig returns allocation optimized for streaming responses.
func StreamingBudgetConfig() TokenBudgetConfig {
	return TokenBudgetConfig{
		SystemPromptPercent:   0.05,
		ToolDefinitionPercent: 0.05,
		ConversationPercent:   0.35,
		ContextFilesPercent:   0.25,
		OutputPercent:         0.25, // More room for streaming output
		SafetyMarginPercent:   0.05,
	}
}

// NewTokenBudget creates a new token budget manager.
func NewTokenBudget(modelLimits ModelTokenLimits, cfg TokenBudgetConfig) *TokenBudget {
	total := modelLimits.ContextWindow

	return &TokenBudget{
		TotalBudget:           total,
		SystemPromptReserve:   int(float64(total) * cfg.SystemPromptPercent),
		ToolDefinitionReserve: int(float64(total) * cfg.ToolDefinitionPercent),
		OutputReserve:         min(modelLimits.MaxOutput, int(float64(total)*cfg.OutputPercent)),
		SafetyMargin:          int(float64(total) * cfg.SafetyMarginPercent),
	}
}

// Allocate computes the available allocation for each component.
func (tb *TokenBudget) Allocate(cfg TokenBudgetConfig) BudgetAllocation {
	tb.mu.RLock()
	defer tb.mu.RUnlock()

	total := tb.TotalBudget - tb.SafetyMargin

	return BudgetAllocation{
		MaxSystemPrompt: int(float64(total) * cfg.SystemPromptPercent),
		MaxToolDefs:     int(float64(total) * cfg.ToolDefinitionPercent),
		MaxConversation: int(float64(total) * cfg.ConversationPercent),
		MaxContextFiles: int(float64(total) * cfg.ContextFilesPercent),
		MaxOutput:       tb.OutputReserve,
		Available:       tb.AvailableTokens(),
	}
}

// AvailableTokens returns remaining tokens after current usage.
func (tb *TokenBudget) AvailableTokens() int {
	tb.mu.RLock()
	defer tb.mu.RUnlock()

	used := tb.SystemPromptUsed + tb.ToolDefinitionUsed + tb.ConversationUsed + tb.ContextFilesUsed
	available := tb.TotalBudget - used - tb.OutputReserve - tb.SafetyMargin

	return max(0, available)
}

// AvailableForContext returns tokens available for context files.
func (tb *TokenBudget) AvailableForContext() int {
	tb.mu.RLock()
	defer tb.mu.RUnlock()

	used := tb.SystemPromptUsed + tb.ToolDefinitionUsed + tb.ConversationUsed
	available := tb.TotalBudget - used - tb.OutputReserve - tb.SafetyMargin

	return max(0, available)
}

// UseSystemPrompt records system prompt token usage.
func (tb *TokenBudget) UseSystemPrompt(tokens int) error {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	if tokens > tb.SystemPromptReserve {
		return fmt.Errorf("system prompt exceeds budget: %d > %d", tokens, tb.SystemPromptReserve)
	}
	tb.SystemPromptUsed = tokens
	return nil
}

// UseToolDefinitions records tool definition token usage.
func (tb *TokenBudget) UseToolDefinitions(tokens int) error {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	if tokens > tb.ToolDefinitionReserve {
		return fmt.Errorf("tool definitions exceed budget: %d > %d", tokens, tb.ToolDefinitionReserve)
	}
	tb.ToolDefinitionUsed = tokens
	return nil
}

// UseConversation records conversation token usage.
func (tb *TokenBudget) UseConversation(tokens int) {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	tb.ConversationUsed = tokens
}

// UseContextFiles records context file token usage.
func (tb *TokenBudget) UseContextFiles(tokens int) {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	tb.ContextFilesUsed = tokens
}

// Summary returns a summary of current budget usage.
func (tb *TokenBudget) Summary() BudgetSummary {
	tb.mu.RLock()
	defer tb.mu.RUnlock()

	totalUsed := tb.SystemPromptUsed + tb.ToolDefinitionUsed + tb.ConversationUsed + tb.ContextFilesUsed
	available := tb.TotalBudget - totalUsed - tb.OutputReserve - tb.SafetyMargin

	return BudgetSummary{
		TotalBudget:     tb.TotalBudget,
		TotalUsed:       totalUsed,
		Available:       max(0, available),
		SystemPrompt:    tb.SystemPromptUsed,
		ToolDefinitions: tb.ToolDefinitionUsed,
		Conversation:    tb.ConversationUsed,
		ContextFiles:    tb.ContextFilesUsed,
		OutputReserve:   tb.OutputReserve,
		UsagePercent:    float64(totalUsed) / float64(tb.TotalBudget) * 100,
	}
}

// BudgetSummary provides a snapshot of budget usage.
type BudgetSummary struct {
	TotalBudget     int     `json:"total_budget"`
	TotalUsed       int     `json:"total_used"`
	Available       int     `json:"available"`
	SystemPrompt    int     `json:"system_prompt"`
	ToolDefinitions int     `json:"tool_definitions"`
	Conversation    int     `json:"conversation"`
	ContextFiles    int     `json:"context_files"`
	OutputReserve   int     `json:"output_reserve"`
	UsagePercent    float64 `json:"usage_percent"`
}

// ContextItem represents a piece of context with its token count.
type ContextItem struct {
	Path       string  `json:"path"`
	Content    string  `json:"content"`
	Tokens     int     `json:"tokens"`
	Priority   float64 `json:"priority"`
	IsSummary  bool    `json:"is_summary"`
	Required   bool    `json:"required"`
	Freshness  float64 `json:"freshness,omitempty"`
	Dependency float64 `json:"dependency,omitempty"`
	Staleness  string  `json:"staleness,omitempty"`
}

// TruncationStrategy defines how to handle context overflow.
type TruncationStrategy int

const (
	TruncateLowestPriority TruncationStrategy = iota
	TruncateSummarize
	TruncateHeadTail
	TruncateNone
)

// Truncator handles context truncation when budget is exceeded.
type Truncator struct {
	strategy       TruncationStrategy
	summarizer     func(content string, maxTokens int) (string, int, error)
	tokenEstimator func(content string) int
}

// NewTruncator creates a new context truncator.
func NewTruncator(strategy TruncationStrategy) *Truncator {
	return &Truncator{
		strategy:       strategy,
		tokenEstimator: EstimateTokens,
	}
}

// SetSummarizer sets a custom summarization function.
func (t *Truncator) SetSummarizer(fn func(content string, maxTokens int) (string, int, error)) {
	t.summarizer = fn
}

// SetTokenEstimator sets a custom token estimation function.
func (t *Truncator) SetTokenEstimator(fn func(content string) int) {
	t.tokenEstimator = fn
}

// FitToBudget fits context items within the token budget.
func (t *Truncator) FitToBudget(items []ContextItem, budget int) ([]ContextItem, error) {
	if budget <= 0 {
		return nil, nil
	}

	// Sort by required-first, then priority (highest first)
	sorted := make([]ContextItem, len(items))
	copy(sorted, items)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Required != sorted[j].Required {
			return sorted[i].Required
		}
		return sorted[i].Priority > sorted[j].Priority
	})

	var result []ContextItem
	usedTokens := 0

	for _, item := range sorted {
		if usedTokens+item.Tokens <= budget {
			// Item fits completely
			result = append(result, item)
			usedTokens += item.Tokens
		} else if t.strategy == TruncateSummarize && t.summarizer != nil {
			// Try to summarize
			remaining := budget - usedTokens
			if remaining > 100 { // Minimum useful summary size
				summary, tokens, err := t.summarizer(item.Content, remaining)
				if err == nil && tokens <= remaining {
					result = append(result, ContextItem{
						Path:      item.Path,
						Content:   summary,
						Tokens:    tokens,
						Priority:  item.Priority,
						IsSummary: true,
					})
					usedTokens += tokens
				}
			}
		} else if t.strategy == TruncateHeadTail {
			// Take head and tail of content
			remaining := budget - usedTokens
			if remaining > 50 {
				truncated := t.truncateHeadTail(item.Content, remaining)
				tokens := t.tokenEstimator(truncated)
				result = append(result, ContextItem{
					Path:     item.Path,
					Content:  truncated,
					Tokens:   tokens,
					Priority: item.Priority,
				})
				usedTokens += tokens
			}
		} else if item.Required && budget-usedTokens > 0 {
			remaining := budget - usedTokens
			truncated := item.Content
			if len(truncated) > remaining*4 {
				truncated = truncated[:remaining*4]
			}
			tokens := t.tokenEstimator(truncated)
			if tokens > remaining {
				tokens = remaining
			}
			result = append(result, ContextItem{
				Path:       item.Path,
				Content:    truncated,
				Tokens:     tokens,
				Priority:   item.Priority,
				Required:   item.Required,
				Freshness:  item.Freshness,
				Dependency: item.Dependency,
				Staleness:  item.Staleness,
			})
			usedTokens += tokens
		}
		// TruncateLowestPriority: just skip items that don't fit
	}

	return result, nil
}

// truncateHeadTail keeps the beginning and end of content.
func (t *Truncator) truncateHeadTail(content string, maxTokens int) string {
	// Estimate characters per token (~4)
	maxChars := maxTokens * 4
	if len(content) <= maxChars {
		return content
	}

	headSize := maxChars * 40 / 100 // 40% for head
	tailSize := maxChars * 40 / 100 // 40% for tail
	// 20% for truncation marker

	if headSize+tailSize >= len(content) {
		return content
	}

	head := content[:headSize]
	tail := content[len(content)-tailSize:]

	return head + "\n\n... [truncated] ...\n\n" + tail
}

// EstimateTokens provides a rough token estimate (1 token ≈ 4 chars).
func EstimateTokens(content string) int {
	return len(content) / 4
}

// EstimateTokensAccurate provides a more accurate estimate using word/symbol counting.
func EstimateTokensAccurate(content string) int {
	// More accurate heuristic based on:
	// - Words tend to be 1-2 tokens
	// - Symbols and punctuation are usually 1 token
	// - Numbers vary

	words := strings.Fields(content)
	tokens := 0

	for _, word := range words {
		if len(word) <= 4 {
			tokens += 1
		} else if len(word) <= 8 {
			tokens += 2
		} else {
			tokens += int(math.Ceil(float64(len(word)) / 4.0))
		}
	}

	return tokens
}

// GetModelLimits returns token limits for a model name.
func GetModelLimits(model string) ModelTokenLimits {
	modelLower := strings.ToLower(model)

	switch {
	case strings.Contains(modelLower, "gemini") && strings.Contains(modelLower, "pro"):
		return ModelLimitsGeminiPro
	case strings.Contains(modelLower, "gemini"):
		return ModelLimitsGeminiFlash
	default:
		return ModelLimitsDefault
	}
}

// DynamicBudgetAdjuster adjusts budget based on runtime conditions.
type DynamicBudgetAdjuster struct {
	baseBudget *TokenBudget
	mu         sync.RWMutex
	history    []int // Recent conversation lengths for prediction
}

// NewDynamicBudgetAdjuster creates a new adjuster.
func NewDynamicBudgetAdjuster(budget *TokenBudget) *DynamicBudgetAdjuster {
	return &DynamicBudgetAdjuster{
		baseBudget: budget,
		history:    make([]int, 0, 10),
	}
}

// RecordConversationLength records the token count of a conversation turn.
func (d *DynamicBudgetAdjuster) RecordConversationLength(tokens int) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.history = append(d.history, tokens)
	if len(d.history) > 10 {
		d.history = d.history[1:]
	}
}

// PredictNextTurnBudget estimates token need for next turn.
func (d *DynamicBudgetAdjuster) PredictNextTurnBudget() int {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if len(d.history) == 0 {
		return 1000 // Default estimate
	}

	// Use exponential moving average weighted toward recent values
	var weightedSum float64
	var weightSum float64
	for i, tokens := range d.history {
		weight := math.Pow(1.5, float64(i)) // More recent = higher weight
		weightedSum += float64(tokens) * weight
		weightSum += weight
	}

	// Add 20% buffer
	return int(weightedSum / weightSum * 1.2)
}

// ShouldCompact returns true if conversation should be compacted.
func (d *DynamicBudgetAdjuster) ShouldCompact(currentUsage int) bool {
	available := d.baseBudget.AvailableTokens()
	predicted := d.PredictNextTurnBudget()

	// Compact if predicted need exceeds available space with buffer
	return currentUsage+predicted > available-d.baseBudget.SafetyMargin
}
