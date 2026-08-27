package context

import (
	"context"
	"fmt"
	"time"

	"github.com/iSundram/Automergent/internal/ai"
)

// PipelineConfig configures the compaction pipeline integration.
type PipelineConfig struct {
	// Thresholds (fraction of context window)
	GhostThreshold      float64
	TruncateThreshold   float64
	DistillThreshold    float64
	SnapshotThreshold   float64
	MicroThreshold      float64
	FullCompactThreshold float64

	// Behavior
	MaxGhostChars   int
	TruncateKeepPct float64 // fraction to keep (head+tail)
	DistillMaxWords int
	ProbeVerify     bool // run verification after full compact

	// Timing
	CheckInterval time.Duration
	MaxCompactions int // per session
}

// DefaultPipelineConfig returns sensible defaults.
func DefaultPipelineConfig() PipelineConfig {
	return PipelineConfig{
		GhostThreshold:       0.50,
		TruncateThreshold:    0.65,
		DistillThreshold:     0.75,
		SnapshotThreshold:    0.85,
		MicroThreshold:       0.90,
		FullCompactThreshold: 0.95,
		MaxGhostChars:        32768,
		TruncateKeepPct:      0.6,
		DistillMaxWords:      800,
		ProbeVerify:          true,
		CheckInterval:        30 * time.Second,
		MaxCompactions:       10,
	}
}

// PipelineIntegration wires the CompactionPipeline to the agent loop.
type PipelineIntegration struct {
	pipeline *CompactionPipeline
	config   PipelineConfig
	provider ai.Provider
	stats    CompactionStats
}

// CompactionStats tracks compaction activity.
type CompactionStats struct {
	TotalCompactions int
	TierCounts       map[CompactionStrategy]int
	TotalSaved       int // tokens saved
	LastCompaction   time.Time
}

// NewPipelineIntegration creates a new integration.
func NewPipelineIntegration(mgr *Manager, provider ai.Provider, llmSummarize func(ctx context.Context, prompt string, maxTokens int) (string, error)) *PipelineIntegration {
	return &PipelineIntegration{
		pipeline: NewCompactionPipeline(mgr, nil, llmSummarize),
		config:   DefaultPipelineConfig(),
		provider: provider,
		stats: CompactionStats{
			TierCounts: make(map[CompactionStrategy]int),
		},
	}
}

// ShouldCompact checks if compaction should be triggered.
func (pi *PipelineIntegration) ShouldCompact(messages []ai.Message, windowSize int) (bool, CompactionReason, []CompactionTier) {
	used := pi.pipeline.estimateTokens(messages)
	reason, tiers := pi.pipeline.ShouldCompact(used, windowSize)
	return len(tiers) > 0, reason, tiers
}

// CompactIfNeeded runs compaction if thresholds are crossed.
// Returns the (possibly compacted) messages and whether compaction occurred.
func (pi *PipelineIntegration) CompactIfNeeded(ctx context.Context, messages []ai.Message, windowSize int) ([]ai.Message, bool, error) {
	if pi.stats.TotalCompactions >= pi.config.MaxCompactions {
		return messages, false, nil
	}

	should, reason, tiers := pi.ShouldCompact(messages, windowSize)
	if !should {
		return messages, false, nil
	}

	startTokens := pi.pipeline.estimateTokens(messages)
	compacted, err := pi.pipeline.Compact(ctx, reason, messages, windowSize)
	if err != nil {
		return messages, false, fmt.Errorf("compaction failed: %w", err)
	}

	endTokens := pi.pipeline.estimateTokens(compacted)
	saved := startTokens - endTokens

	// Update stats
	pi.stats.TotalCompactions++
	pi.stats.TotalSaved += saved
	pi.stats.LastCompaction = time.Now()
	for _, tier := range tiers {
		pi.stats.TierCounts[tier.Strategy]++
	}

	return compacted, true, nil
}

// GetStats returns current compaction statistics.
func (pi *PipelineIntegration) GetStats() CompactionStats {
	return pi.stats
}

// GhostLargeOutputs persists large tool outputs to disk and replaces with stubs.
func (pi *PipelineIntegration) GhostLargeOutputs(messages []ai.Message) []ai.Message {
	return pi.pipeline.ghostLargeOutputs(messages)
}

// TruncateMiddle keeps head+tail of messages, truncating the middle.
func (pi *PipelineIntegration) TruncateMiddle(messages []ai.Message, windowSize int) []ai.Message {
	return pi.pipeline.truncateMiddle(messages, windowSize)
}

// DistillOldReads uses LLM to summarize old read-only tool outputs.
func (pi *PipelineIntegration) DistillOldReads(ctx context.Context, messages []ai.Message, windowSize int) ([]ai.Message, error) {
	return pi.pipeline.distillOldReads(ctx, messages, windowSize)
}

// EmitSnapshot creates a Master State JSON snapshot.
func (pi *PipelineIntegration) EmitSnapshot(ctx context.Context, messages []ai.Message, windowSize int) ([]ai.Message, error) {
	return pi.pipeline.emitSnapshot(ctx, messages, windowSize)
}

// Microcompact compacts only tool results.
func (pi *PipelineIntegration) Microcompact(ctx context.Context, messages []ai.Message, windowSize int) ([]ai.Message, error) {
	return pi.pipeline.microcompact(ctx, messages, windowSize)
}

// FullCompactWithVerify does full LLM summary with probe verification.
func (pi *PipelineIntegration) FullCompactWithVerify(ctx context.Context, messages []ai.Message, windowSize int) ([]ai.Message, error) {
	return pi.pipeline.fullCompactWithVerify(ctx, messages, windowSize)
}
