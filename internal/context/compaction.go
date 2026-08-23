package context

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/iSundram/Automergent/internal/ai"
)

// CompactionReason is why compaction was triggered.
type CompactionReason string

const (
	ReasonUserRequested   CompactionReason = "user_requested"
	ReasonContextLimit    CompactionReason = "context_limit"
	ReasonModelDownshift  CompactionReason = "model_downshift"
	ReasonCompHashChanged CompactionReason = "comp_hash_changed"
)

// CompactionPhase is when compaction runs.
type CompactionPhase string

const (
	PhaseStandaloneTurn CompactionPhase = "standalone"
	PhasePreTurn        CompactionPhase = "pre_turn"
	PhaseMidTurn        CompactionPhase = "mid_turn"
)

// CompactionStrategy is the implementation tier.
type CompactionStrategy string

const (
	StrategyGhost          CompactionStrategy = "ghost"           // persist-then-preview
	StrategyTruncateMiddle CompactionStrategy = "truncate_middle" // keep head+tail with marker
	StrategyDistill        CompactionStrategy = "distill"         // LLM summarization
	StrategySnapshot       CompactionStrategy = "snapshot"        // Master State JSON
	StrategyMicrocompact   CompactionStrategy = "microcompact"    // compact tool results only
	StrategyFullCompact    CompactionStrategy = "full_compact"    // full summary + probe verify
)

// CompactionTier defines a tier in the progressive degradation ladder.
type CompactionTier struct {
	Name      string
	Strategy  CompactionStrategy
	Threshold float64 // fraction of window at which to trigger
	Priority  int     // lower = runs first
}

// DefaultTiers returns the progressive degradation ladder.
func DefaultTiers() []CompactionTier {
	return []CompactionTier{
		{Name: "ghost", Strategy: StrategyGhost, Threshold: 0.50, Priority: 1},
		{Name: "truncate_middle", Strategy: StrategyTruncateMiddle, Threshold: 0.65, Priority: 2},
		{Name: "distill", Strategy: StrategyDistill, Threshold: 0.75, Priority: 3},
		{Name: "snapshot", Strategy: StrategySnapshot, Threshold: 0.85, Priority: 4},
		{Name: "microcompact", Strategy: StrategyMicrocompact, Threshold: 0.90, Priority: 5},
		{Name: "full_compact", Strategy: StrategyFullCompact, Threshold: 0.95, Priority: 6},
	}
}

// CompactionPipeline orchestrates the tiered compaction process.
type CompactionPipeline struct {
	mu           sync.RWMutex
	manager      *Manager
	provider     Provider
	tiers        []CompactionTier
	toolResults  *ToolResultStore
	snapshots    *SnapshotStore
	llmSummarize func(ctx context.Context, prompt string, maxTokens int) (string, error)
}

// Provider interface for LLM-based compaction tiers.
type Provider interface {
	Complete(ctx context.Context, req CompletionRequest) (CompletionResponse, error)
}

// CompletionRequest for compaction.
type CompletionRequest struct {
	Messages    []ai.Message
	System      string
	Temperature float64
	MaxTokens   int
	Stream      bool
}

// CompletionResponse for compaction.
type CompletionResponse interface {
	Stream() <-chan Chunk
	Usage() ai.Usage
}

// Chunk for compaction streaming.
type Chunk struct {
	Text  string
	Error error
	Done  bool
}

// NewCompactionPipeline creates a new compaction pipeline.
func NewCompactionPipeline(mgr *Manager, provider Provider, llmSummarize func(ctx context.Context, prompt string, maxTokens int) (string, error)) *CompactionPipeline {
	p := &CompactionPipeline{
		manager:      mgr,
		provider:     provider,
		tiers:        DefaultTiers(),
		toolResults:  NewToolResultStore(mgr.rootDir),
		snapshots:    NewSnapshotStore(mgr.rootDir),
		llmSummarize: llmSummarize,
	}
	return p
}

// ShouldCompact checks if any tier threshold is crossed.
func (cp *CompactionPipeline) ShouldCompact(used, total int) (CompactionReason, []CompactionTier) {
	if total <= 0 {
		return "", nil
	}
	frac := float64(used) / float64(total)
	var triggered []CompactionTier
	for _, t := range cp.tiers {
		if frac >= t.Threshold {
			triggered = append(triggered, t)
		}
	}
	if len(triggered) == 0 {
		return "", nil
	}
	return ReasonContextLimit, triggered
}

// Compact runs the triggered tiers in priority order.
func (cp *CompactionPipeline) Compact(ctx context.Context, reason CompactionReason, messages []ai.Message, windowSize int) ([]ai.Message, error) {
	_, tiers := cp.ShouldCompact(cp.estimateTokens(messages), windowSize)
	if len(tiers) == 0 {
		return messages, nil
	}

	sort.Slice(tiers, func(i, j int) bool {
		return tiers[i].Priority < tiers[j].Priority
	})

	current := messages
	for _, tier := range tiers {
		var err error
		current, err = cp.runTier(ctx, tier, current, windowSize)
		if err != nil {
			return current, fmt.Errorf("tier %s: %w", tier.Name, err)
		}
		// Check if we've freed enough space
		used := cp.estimateTokens(current)
		if float64(used)/float64(windowSize) < tier.Threshold*0.8 {
			break
		}
	}
	return current, nil
}

func (cp *CompactionPipeline) runTier(ctx context.Context, tier CompactionTier, messages []ai.Message, windowSize int) ([]ai.Message, error) {
	switch tier.Strategy {
	case StrategyGhost:
		return cp.ghostLargeOutputs(messages), nil
	case StrategyTruncateMiddle:
		return cp.truncateMiddle(messages, windowSize), nil
	case StrategyDistill:
		return cp.distillOldReads(ctx, messages, windowSize)
	case StrategySnapshot:
		return cp.emitSnapshot(ctx, messages, windowSize)
	case StrategyMicrocompact:
		return cp.microcompact(ctx, messages, windowSize)
	case StrategyFullCompact:
		return cp.fullCompactWithVerify(ctx, messages, windowSize)
	default:
		return messages, nil
	}
}

// Tier 1: Ghost - persist large tool outputs to disk, replace with stub.
func (cp *CompactionPipeline) ghostLargeOutputs(messages []ai.Message) []ai.Message {
	maxChars := 32768
	// Try to get from manager's config if available (via agent)
	// For now use default
	if maxChars <= 0 {
		maxChars = 32768
	}

	ghosted := make([]ai.Message, len(messages))
	for i, msg := range messages {
		if msg.Role != ai.RoleTool {
			ghosted[i] = msg
			continue
		}

		newMsg := msg
		newMsg.Content = make([]ai.ContentPart, len(msg.Content))
		for j, part := range msg.Content {
			if part.Type == ai.ContentTypeToolResult && part.ToolResult != nil && len(part.ToolResult.Content) > maxChars {
				// Persist to disk
				path, _ := cp.toolResults.Store(part.ToolResult.Content)
				limit := 500
				if len(part.ToolResult.Content) < limit {
					limit = len(part.ToolResult.Content)
				}
				stub := fmt.Sprintf("[Output persisted to %s (%d chars). Preview: %s...]\n\nUse 'read_file' to retrieve full output.",
					path, len(part.ToolResult.Content), part.ToolResult.Content[:limit])

				newResult := *part.ToolResult
				newResult.Content = stub
				newMsg.Content[j] = ai.ContentPart{
					Type:       ai.ContentTypeToolResult,
					ToolResult: &newResult,
				}
			} else {
				newMsg.Content[j] = part
			}
		}
		ghosted[i] = newMsg
	}
	return ghosted
}

// Tier 2: Truncate Middle - keep head + tail with marker.
func (cp *CompactionPipeline) truncateMiddle(messages []ai.Message, windowSize int) []ai.Message {
	// Target: reduce to 60% of window
	target := int(float64(windowSize) * 0.6)
	current := cp.estimateTokens(messages)
	if current <= target {
		return messages
	}

	// Work backwards from newest, truncating tool results first
	result := make([]ai.Message, len(messages))
	copy(result, messages)

	for i := len(result) - 1; i >= 0 && current > target; i-- {
		msg := &result[i]
		if msg.Role != ai.RoleTool {
			continue
		}
		for j, part := range msg.Content {
			if part.Type != ai.ContentTypeToolResult || part.ToolResult == nil {
				continue
			}
			content := part.ToolResult.Content
			if len(content) < 100 {
				continue
			}
			// Keep 40% head + 40% tail with marker
			head := content[:len(content)*4/10]
			tail := content[len(content)*6/10:]
			truncated := head + "\n\n... [truncated " + fmt.Sprintf("%d", len(content)-len(head)-len(tail)) + " chars] ...\n\n" + tail
			newResult := *part.ToolResult
			newResult.Content = truncated
			msg.Content[j] = ai.ContentPart{Type: ai.ContentTypeToolResult, ToolResult: &newResult}
			current = cp.estimateTokens(result)
		}
	}
	return result
}

// Tier 3: Distill - LLM summarize old file reads (read_file results older than 2 turns).
func (cp *CompactionPipeline) distillOldReads(ctx context.Context, messages []ai.Message, windowSize int) ([]ai.Message, error) {
	// Find tool results from read_file older than the last 2 user turns
	userTurnCount := 0
	cutoffIdx := -1
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == ai.RoleUser {
			userTurnCount++
			if userTurnCount >= 3 {
				cutoffIdx = i
				break
			}
		}
	}
	if cutoffIdx < 0 {
		return messages, nil
	}

	// Collect read_file results to distill
	type readResult struct {
		msgIdx   int
		partIdx  int
		callID   string
		filePath string
		content  string
	}
	var toDistill []readResult
	for i := 0; i <= cutoffIdx; i++ {
		msg := messages[i]
		for j, part := range msg.Content {
			if part.Type == ai.ContentTypeToolResult && part.ToolResult != nil {
				// Check if it's a read_file tool
				// We can't easily tell without the call, so distill all tool results before cutoff
				content := part.ToolResult.Content
				if len(content) > 2000 {
					toDistill = append(toDistill, readResult{
						msgIdx:  i,
						partIdx: j,
						callID:  part.ToolResult.ToolCallID,
						content: content,
					})
				}
			}
		}
	}

	if len(toDistill) == 0 {
		return messages, nil
	}

	// Batch distill via LLM
	for _, rd := range toDistill {
		prompt := fmt.Sprintf(`Summarize this tool output concisely, preserving key technical details:

%s

Keep under 500 words. Focus on: key findings, code snippets, errors, file structure.`, rd.content)

		summary, err := cp.llmSummarize(ctx, prompt, 500)
		if err != nil {
			continue // Keep original on error
		}
		messages[rd.msgIdx].Content[rd.partIdx] = ai.ContentPart{
			Type: ai.ContentTypeToolResult,
			ToolResult: &ai.ToolResult{
				ToolCallID: rd.callID,
				Content:    "[DISTILLED] " + summary,
				IsError:    false,
			},
		}
	}
	return messages, nil
}

// Tier 4: Snapshot - emit Master State JSON.
func (cp *CompactionPipeline) emitSnapshot(ctx context.Context, messages []ai.Message, windowSize int) ([]ai.Message, error) {
	snap := &MasterSnapshot{
		ActiveTasks:         cp.extractActiveTasks(messages),
		DiscoveredFacts:     cp.extractFacts(messages),
		ConstraintsAndPrefs: cp.extractConstraints(messages),
		RecentArc:           cp.extractRecentArc(messages),
		CreatedAt:           time.Now(),
	}

	// Persist snapshot
	if err := cp.snapshots.Store(snap); err != nil {
		return messages, err
	}

	// Inject snapshot summary as a system message
	summary := snap.Summarize()
	snapMsg := ai.NewTextMessage(ai.RoleSystem,
		"# Context Snapshot (auto-generated)\n\n"+summary+
			"\n\n[This snapshot preserves the essential context state. Full details available via /snapshot command.]")

	result := append(messages, snapMsg)
	return result, nil
}

// Tier 5: Microcompact - compact only tool results via non-streaming call.
func (cp *CompactionPipeline) microcompact(ctx context.Context, messages []ai.Message, windowSize int) ([]ai.Message, error) {
	// Find all tool results
	var toolResults []ai.Message
	for _, m := range messages {
		if m.Role == ai.RoleTool {
			toolResults = append(toolResults, m)
		}
	}
	if len(toolResults) <= 2 {
		return messages, nil
	}

	// Keep last 2 tool results verbatim, compact the rest
	toCompact := toolResults[:len(toolResults)-2]
	prompt := "Summarize these tool outputs concisely:\n\n"
	for _, tr := range toCompact {
		prompt += tr.PlaintextForHistory() + "\n---\n"
	}
	prompt += "\nKeep under 1000 words. Preserve: errors, key findings, file paths."

	summary, err := cp.llmSummarize(ctx, prompt, 1000)
	if err != nil {
		return messages, err
	}

	// Replace compacted tool results with summary
	newMessages := make([]ai.Message, 0, len(messages)-len(toCompact)+1)
	compacted := false
	for _, m := range messages {
		if m.Role == ai.RoleTool && !compacted {
			// Check if this is in the toCompact set
			isCompacted := false
			for _, tc := range toCompact {
				if &tc == &m {
					isCompacted = true
					break
				}
			}
			if isCompacted {
				if !compacted {
					newMessages = append(newMessages, ai.NewTextMessage(ai.RoleSystem,
						"# Microcompacted Tool Outputs\n\n"+summary))
					compacted = true
				}
				continue
			}
		}
		newMessages = append(newMessages, m)
	}
	return newMessages, nil
}

// Tier 6: Full Compact - summarize entire history with probe-verify.
func (cp *CompactionPipeline) fullCompactWithVerify(ctx context.Context, messages []ai.Message, windowSize int) ([]ai.Message, error) {
	// Use existing CompactSessionMessages logic but with probe-verify
	// This is a simplified version - full implementation would be more complex
	keepRecent := 8
	if len(messages) <= keepRecent+4 {
		return messages, nil
	}

	// First, run ghost and truncate middle
	messages = cp.ghostLargeOutputs(messages)
	messages = cp.truncateMiddle(messages, windowSize)

	// Build summary of middle section
	startIdx := len(messages) - keepRecent
	if startIdx < 1 {
		startIdx = 1
	}
	middle := messages[1:startIdx]

	prompt := `Summarize this conversation segment for context continuity.
Focus on:
1. The problem being solved
2. Key files modified/investigated
3. Decisions and rationale
4. Constraints and requirements
5. Successes and failures
Max 1000 words.`

	var middleText strings.Builder
	for _, m := range middle {
		middleText.WriteString(m.PlaintextForHistory())
		middleText.WriteString("\n---\n")
	}

	summary, err := cp.llmSummarize(ctx, prompt+"\n\n"+middleText.String(), 1000)
	if err != nil {
		return messages, err
	}

	// Probe-verify: ask model to check the summary
	verifyPrompt := `Critically evaluate this summary. Did you omit any specific technical details, file paths, error messages, or decisions that would be needed to continue the task?

Summary:
` + summary + `

Respond with "COMPLETE" if adequate, or list what's missing.`

	verify, err := cp.llmSummarize(ctx, verifyPrompt, 500)
	if err != nil || strings.Contains(strings.ToUpper(verify), "MISSING") {
		// Fallback: keep more messages
		return messages, nil
	}

	// Build compacted message list
	compacted := []ai.Message{messages[0]}
	compacted = append(compacted, ai.NewTextMessage(ai.RoleSystem,
		"# Neural Context Summary\n\n"+summary))
	// Add important preserved messages
	for _, m := range middle {
		if isImportant(m) {
			compacted = append(compacted, m)
		}
	}
	compacted = append(compacted, messages[startIdx:]...)

	return compacted, nil
}

// estimateTokens uses the manager's adaptive calculator if available.
func (cp *CompactionPipeline) estimateTokens(messages []ai.Message) int {
	if cp.manager != nil && cp.manager.adaptiveCalc != nil {
		var total int
		for _, m := range messages {
			total += cp.manager.adaptiveCalc.Estimate(m.PlaintextForHistory())
		}
		return total
	}
	// Fallback
	total := 0
	for _, m := range messages {
		total += len(m.PlaintextForHistory()) / 4
	}
	return total
}

// Helper extraction methods for snapshots.
func (cp *CompactionPipeline) extractActiveTasks(messages []ai.Message) []string {
	var tasks []string
	for _, m := range messages {
		if m.Role == ai.RoleUser {
			text := strings.ToLower(m.TextContent())
			if strings.Contains(text, "task") || strings.Contains(text, "implement") || strings.Contains(text, "fix") {
				tasks = append(tasks, m.TextContent())
			}
		}
	}
	return tasks
}

func (cp *CompactionPipeline) extractFacts(messages []ai.Message) []string {
	var facts []string
	for _, m := range messages {
		if m.Role == ai.RoleAssistant {
			for _, p := range m.Content {
				if p.Type == ai.ContentTypeToolResult && p.ToolResult != nil {
					if strings.Contains(strings.ToLower(p.ToolResult.Content), "found") ||
						strings.Contains(strings.ToLower(p.ToolResult.Content), "error") {
						facts = append(facts, p.ToolResult.Content[:min(200, len(p.ToolResult.Content))])
					}
				}
			}
		}
	}
	return facts
}

func (cp *CompactionPipeline) extractConstraints(messages []ai.Message) []string {
	var constraints []string
	for _, m := range messages {
		if m.Role == ai.RoleUser {
			text := strings.ToLower(m.TextContent())
			if strings.Contains(text, "must") || strings.Contains(text, "should") ||
				strings.Contains(text, "constraint") || strings.Contains(text, "require") {
				constraints = append(constraints, m.TextContent())
			}
		}
	}
	return constraints
}

func (cp *CompactionPipeline) extractRecentArc(messages []ai.Message) []string {
	var arc []string
	for i := len(messages) - 1; i >= 0 && len(arc) < 5; i-- {
		if messages[i].Role == ai.RoleAssistant || messages[i].Role == ai.RoleTool {
			arc = append(arc, messages[i].PlaintextForHistory()[:min(100, len(messages[i].PlaintextForHistory()))])
		}
	}
	return arc
}

func isImportant(m ai.Message) bool {
	if m.Role == ai.RoleUser {
		text := strings.ToLower(m.TextContent())
		return strings.Contains(text, "confirm") || strings.Contains(text, "constraint") ||
			strings.Contains(text, "must") || strings.Contains(text, "should")
	}
	if m.Role == ai.RoleTool {
		for _, p := range m.Content {
			if p.Type == ai.ContentTypeToolResult && p.ToolResult != nil && p.ToolResult.IsError {
				return true
			}
		}
	}
	if m.Role == ai.RoleAssistant {
		text := strings.ToLower(m.TextContent())
		if strings.Contains(text, "plan") || strings.Contains(text, "approach") || strings.Contains(text, "strategy") {
			return true
		}
		for _, tc := range m.ToolCallParts() {
			if tc.Name == "edit_file" || tc.Name == "create_file" || tc.Name == "write_file" {
				return true
			}
		}
	}
	return false
}
