package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/iSundram/Automergent/internal/ai"
	"github.com/iSundram/Automergent/internal/errors"
)

// Context-window management for the phase loop.
//
// This is our production-grade auto-compact design, adapted to the
// multi-phase arc: every phase loop iteration passes through
// manageContextWindow before building the provider request, which runs the
// same ladder leading terminal agents run — tool-result budgeting (ghost +
// micro compact), auto-compact with a summary and a boundary marker, warning
// and blocking states — and the loop additionally reacts to provider
// CONTEXT_TOO_LONG errors with an emergency compaction retry.
//
// The knobs mirror proven production values:
//   - the effective window reserves tokens for the compaction summary itself,
//   - the auto-compact trigger fires early enough to leave room for one more
//     turn of tool output (predictive growth),
//   - the blocking limit reserves a manual-compact buffer,
//   - repeated compaction failures trip a circuit breaker so a broken
//     summarizer degrades to ghosting instead of looping forever.

const (
	// maxOutputTokensForSummary caps the tokens reserved so a compaction
	// summary can still be generated inside the same window.
	maxOutputTokensForSummary = 20_000

	// autocompactBufferTokens is the default headroom below the effective
	// window at which auto-compact triggers.
	autocompactBufferTokens = 13_000

	// largeWindowBufferTokens / hugeWindowBufferTokens scale the buffer for
	// models with very large windows, where a turn can grow much faster.
	largeWindowBufferTokens = 30_000 // effective window >= 400k
	hugeWindowBufferTokens  = 50_000 // effective window >= 800k

	// warningThresholdBufferTokens is the headroom at which the user is
	// warned that compaction is imminent.
	warningThresholdBufferTokens = 20_000

	// manualCompactBufferTokens is reserved at the very top of the window so
	// a manual /compact still fits after the loop refuses to continue.
	manualCompactBufferTokens = 3_000

	// maxConsecutiveCompactionFailures trips the compaction circuit breaker.
	maxConsecutiveCompactionFailures = 3

	// toolResultGrowthEstimate is the worst-case token growth of one more
	// turn (assistant reply + tool results) used by the predictive trigger.
	toolResultGrowthEstimate = 15_000

	// microCompactKeepRecent is how many recent tool-result messages stay
	// verbatim when old tool results are content-cleared.
	microCompactKeepRecent = 6

	// microCompactTriggerFraction is the window usage above which old tool
	// results get cleared. Below it we leave history untouched so prompt
	// caches stay valid.
	microCompactTriggerFraction = 0.55

	// microCompactMinChars is the smallest tool result worth clearing.
	microCompactMinChars = 400

	// microCompactCacheTTL is the idle gap after which the provider's prompt
	// cache is considered expired. Clearing old tool results is normally
	// deferred (it invalidates a warm cache prefix); once the cache is cold
	// anyway, clearing is free. Mirrors the reference agent's time-based
	// microcompact trigger (tengu_slate_heron).
	microCompactCacheTTL = 60 * time.Minute

	// post-compact file restoration budgets (recently read files are
	// re-attached after a compaction so the model does not re-read them).
	postCompactMaxFilesToRestore   = 5
	postCompactTokenBudget         = 50_000
	postCompactMaxTokensPerFile    = 5_000
	postCompactMaxCharsPerFileHint = postCompactMaxTokensPerFile * 4
)

// compactionMetadataKey marks the message that closes a compaction boundary:
// everything before it has been summarized and must not be summarized again.
const compactionBoundaryKey = "compact_boundary"

// contextLimit resolves the hard context window for the current model.
func (a *Agent) contextLimit(provider ai.Provider) int {
	if a.cfg != nil && a.cfg.MaxContextTokens > 0 {
		return a.cfg.MaxContextTokens
	}
	if provider != nil {
		return provider.ContextLimit()
	}
	return 0
}

// effectiveContextWindow subtracts the tokens reserved for the compaction
// summary from the hard limit. All thresholds below are expressed against
// this effective window, never the raw one.
func (a *Agent) effectiveContextWindow(provider ai.Provider) int {
	limit := a.contextLimit(provider)
	if limit <= 0 {
		return 0
	}
	reserved := maxOutputTokensForSummary
	if reserved >= limit {
		return limit / 2 // degenerate config; keep half the window usable
	}
	return limit - reserved
}

// autocompactBuffer scales the trigger headroom with the window size.
func autocompactBuffer(effective int) int {
	switch {
	case effective >= 800_000:
		return hugeWindowBufferTokens
	case effective >= 400_000:
		return largeWindowBufferTokens
	default:
		return autocompactBufferTokens
	}
}

// autoCompactThreshold is the token count at which auto-compact fires. A
// configured AutoCompressAt percentage can only make it fire EARLIER, never
// later than the buffered threshold.
func (a *Agent) autoCompactThreshold(provider ai.Provider) int {
	effective := a.effectiveContextWindow(provider)
	if effective <= 0 {
		return 0
	}
	threshold := effective - autocompactBuffer(effective)
	if a.cfg != nil && a.cfg.AutoCompressAt > 0 && a.cfg.AutoCompressAt < 1 {
		pct := int(float64(effective) * a.cfg.AutoCompressAt)
		if pct < threshold {
			threshold = pct
		}
	}
	return threshold
}

// blockingLimit is the token count past which the loop refuses to call the
// provider at all, reserving room for a manual compaction.
func (a *Agent) blockingLimit(provider ai.Provider) int {
	effective := a.effectiveContextWindow(provider)
	if effective <= 0 {
		return 0
	}
	return effective - manualCompactBufferTokens
}

// tokenCountWithEstimation is the canonical context measurement: the last
// provider-reported usage anchored at the message it covered, plus a rough
// estimate of everything appended since. Cheaper than a full TokenCount call
// every iteration and more accurate than pure estimation.
func (a *Agent) tokenCountWithEstimation(messages []ai.Message) int {
	a.mu.RLock()
	anchor := a.usageAnchor
	anchored := a.usageAnchoredTokens
	a.mu.RUnlock()

	if anchor > 0 && anchor <= len(messages) {
		return anchored + ai.ApproximateTokenCount(messages[anchor:])
	}
	return ai.ApproximateTokenCount(messages)
}

// recordUsageAnchor stores the provider-reported usage for the request that
// covered exactly the first n messages of the session.
func (a *Agent) recordUsageAnchor(usage ai.Usage, coveredMessages int) {
	a.mu.Lock()
	a.usageAnchor = coveredMessages
	a.usageAnchoredTokens = usage.InputTokens + usage.OutputTokens + usage.CacheHits
	a.mu.Unlock()
}

// invalidateUsageAnchor is called whenever messages are rewritten in place
// (compaction, ghosting): the anchor no longer describes the prefix. It also
// resets the prompt-cache baseline — a post-rewrite cache-hit drop is
// expected, not a break (see cachedetect.go).
func (a *Agent) invalidateUsageAnchor() {
	a.mu.Lock()
	a.usageAnchor = 0
	a.usageAnchoredTokens = 0
	a.promptCache = promptCacheState{}
	a.mu.Unlock()
}

// InvalidateUsageAnchor exposes the invalidation for external callers that
// rewrite session messages directly (the TUI's manual /compact).
func (a *Agent) InvalidateUsageAnchor() { a.invalidateUsageAnchor() }

// ContextWindowStats is a read-only snapshot of the context engine's live
// state for observability surfaces (the TUI's /context report).
type ContextWindowStats struct {
	RawLimit              int
	EffectiveWindow       int
	AutoCompactThreshold  int
	BlockingLimit         int
	EstimatedTokens       int
	CompactionFailures    int
	LastCompactedAt       time.Time
	CircuitBreakerTripped bool
}

// ContextWindowStats reports the current thresholds and usage estimate.
// Provider may be nil; the config limit is preferred when set.
func (a *Agent) ContextWindowStats(provider ai.Provider) ContextWindowStats {
	stats := ContextWindowStats{
		RawLimit:             a.contextLimit(provider),
		EffectiveWindow:      a.effectiveContextWindow(provider),
		AutoCompactThreshold: a.autoCompactThreshold(provider),
		BlockingLimit:        a.blockingLimit(provider),
		EstimatedTokens:      a.tokenCountWithEstimation(a.sess.Messages),
	}
	a.mu.RLock()
	stats.CompactionFailures = a.compactionFailures
	stats.LastCompactedAt = a.lastCompactedAt
	a.mu.RUnlock()
	stats.CircuitBreakerTripped = stats.CompactionFailures >= maxConsecutiveCompactionFailures
	return stats
}

// manageContextWindow runs the pre-request context ladder for one loop
// iteration: ghost oversized outputs, micro-compact old tool results,
// auto-compact past the threshold, and report warning/blocking state. It
// returns an error only at the blocking limit, where calling the provider
// would certainly fail.
func (a *Agent) manageContextWindow(ctx context.Context, provider ai.Provider) error {
	messages := a.sess.Messages
	effective := a.effectiveContextWindow(provider)
	if effective <= 0 || provider == nil {
		return nil // window unknown; let the provider be the judge
	}

	// Tier 0: ghost oversized tool outputs (cheap, idempotent).
	ghosted := a.GhostLargeOutputs(messages)

	// Tier 1: micro-compact — content-clear old tool results once usage
	// crosses the cache-friendly fraction, or unconditionally when the
	// provider cache has gone cold (a long idle gap expired it — clearing
	// no longer costs a cache rewrite). Never touches the most recent
	// results, tool calls, or user/assistant text.
	tokens := a.tokenCountWithEstimation(ghosted)
	cacheCold := a.cacheLikelyCold()
	if cacheCold || float64(tokens)/float64(effective) >= microCompactTriggerFraction {
		cleared, freed := microCompactToolResults(ghosted, microCompactKeepRecent)
		if freed > 0 {
			// Boundary event with real accounting so /context and the HUD
			// report reclaimed space, and so a microcompact that freed
			// nothing is visible as the no-op it was.
			a.Emit(EventCompacted, map[string]any{
				"micro":       true,
				"tokens_freed": freed,
				"cache_cold":  cacheCold,
			})
			ghosted = cleared
		}
	}

	if len(ghosted) != len(messages) || messagesDiffer(ghosted, messages) {
		a.sess.SetMessages(ghosted)
		a.invalidateUsageAnchor()
		messages = ghosted
	}

	// Tier 2: auto-compact. The circuit breaker stops repeated failures from
	// wedging the loop; ghosting above already bought some headroom.
	tokens = a.tokenCountWithEstimation(messages)
	if threshold := a.autoCompactThreshold(provider); threshold > 0 && tokens > threshold {
		if a.compactionFailures < maxConsecutiveCompactionFailures {
			before := tokens
			compacted := a.CompactSessionMessages(ctx, messages)
			after := a.tokenCountWithEstimation(compacted)
			a.sess.SetMessages(compacted)
			a.invalidateUsageAnchor()
			a.lastCompactedAt = time.Now()

			if after >= before && before > 0 {
				a.compactionFailures++
			} else {
				a.compactionFailures = 0
			}

			a.Emit(EventCompacted, map[string]any{
				"tokens_before": before,
				"tokens_after":  after,
				"threshold":     threshold,
				"boundary":      len(compacted),
			})
			messages = compacted
			tokens = after
		}
	}

	// Warning + blocking states, expressed as headroom against the effective
	// window exactly like the reference implementation.
	warningAt := effective - warningThresholdBufferTokens
	if warningAt > 0 && tokens >= warningAt {
		a.Emit(EventStatus, fmt.Sprintf(
			"⚠ Context is %d%% full (~%d/%d tokens). Auto-compact will trigger soon.",
			tokens*100/effective, tokens, effective))
	}
	if limit := a.blockingLimit(provider); limit > 0 && tokens >= limit {
		return fmt.Errorf("context window exhausted (~%d tokens, blocking limit %d): compaction could not free enough space; start a new session or reduce the task scope", tokens, limit)
	}
	return nil
}

// predictContextOverflow reports whether one more full turn (assistant reply
// plus tool-result growth) would overflow the effective window, so
// compaction runs BEFORE the request instead of after a failed one.
func (a *Agent) predictContextOverflow(provider ai.Provider) bool {
	effective := a.effectiveContextWindow(provider)
	if effective <= 0 {
		return false
	}
	maxOut := 8192
	estimatedGrowth := maxOut + toolResultGrowthEstimate
	tokens := a.tokenCountWithEstimation(a.sess.Messages)
	return tokens+estimatedGrowth > effective
}

// isContextOverflowError reports whether a provider error means the request
// did not fit the context window. Providers classify these inconsistently
// (some as 400 INVALID_ARGUMENT, some as typed errors, some as plain text),
// so match on the structured code first and fall back to message heuristics.
func isContextOverflowError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, errors.CodeContextTooLong) {
		return true
	}
	msg := strings.ToLower(err.Error())
	for _, marker := range []string{
		"context length", "context window", "prompt too long",
		"too many tokens", "exceeds the maximum number of tokens",
		"input tokens exceed", "request too large",
		"maximum context length", "token limit exceeded",
	} {
		if strings.Contains(msg, marker) {
			return true
		}
	}
	return false
}

// reactiveCompact handles a provider context-overflow rejection: compact
// immediately and report whether a retry is worthwhile. This is the escape
// hatch for when estimation drifted from the provider's real counting.
func (a *Agent) reactiveCompact(ctx context.Context, err error, provider ai.Provider) bool {
	if !isContextOverflowError(err) {
		return false
	}
	a.Emit(EventStatus, "context overflow reported by provider — emergency compaction")
	before := len(a.sess.Messages)
	compacted := a.CompactSessionMessages(ctx, a.sess.Messages)
	if len(compacted) >= before {
		// Nothing to summarize (short but dense conversation): ghost and
		// micro-compact are all we can do; retrying would fail identically.
		compacted, _ = microCompactToolResults(a.GhostLargeOutputs(compacted), 2)
	}
	a.sess.SetMessages(compacted)
	a.invalidateUsageAnchor()
	a.Emit(EventCompacted, map[string]any{
		"tokens_before": before,
		"tokens_after":  len(compacted),
		"reactive":      true,
	})
	return a.tokenCountWithEstimation(compacted) < a.blockingLimit(provider) || a.blockingLimit(provider) <= 0
}

// cacheLikelyCold reports whether the gap since the last assistant response
// exceeds the provider's prompt-cache TTL. When the cache is cold the full
// prefix will be rewritten on the next request regardless, so clearing old
// tool results first shrinks what gets rewritten (the reference agent's
// time-based microcompact trigger).
func (a *Agent) cacheLikelyCold() bool {
	a.mu.RLock()
	last := a.lastAssistantAt
	a.mu.RUnlock()
	return !last.IsZero() && time.Since(last) > microCompactCacheTTL
}

// noteAssistantResponse records the time of the last assistant response,
// feeding the cache-coldness check above.
func (a *Agent) noteAssistantResponse() {
	a.mu.Lock()
	a.lastAssistantAt = time.Now()
	a.mu.Unlock()
}

// compactableTools lists the tools whose results may be content-cleared by
// micro-compact, ported from the reference implementation: file reads,
// searches, shell output, web fetches, and file edits. Deliberately absent:
// subagent results (expensive to reproduce), ask_user replies (user-authored
// content), and todo/context state (tiny and load-bearing).
var compactableTools = map[string]bool{
	"read_file": true, "grep": true, "glob": true, "list_directory": true,
	"bash": true, "write_shell": true, "read_shell": true,
	"web_search": true, "web_fetch": true,
	"edit_file": true, "write_file": true, "multi_edit": true,
}

// toolCallNames maps tool-call IDs to the tool name that issued them, by
// walking assistant messages in order.
func toolCallNames(messages []ai.Message) map[string]string {
	names := make(map[string]string)
	for _, m := range messages {
		if m.Role != ai.RoleAssistant {
			continue
		}
		for _, tc := range m.ToolCallParts() {
			if tc.ID != "" {
				names[tc.ID] = tc.Name
			}
		}
	}
	return names
}

// microCompactToolResults content-clears old tool results, keeping the most
// recent keepRecent compactable tool-result messages verbatim. Only results
// from compactableTools are eligible; tool calls and the call/result pairing
// are never touched, so the message sequence stays valid for strict
// providers. Idempotent: already-cleared results are skipped. Returns the
// new slice and an estimate of the tokens freed (chars/4 of the cleared
// content).
func microCompactToolResults(messages []ai.Message, keepRecent int) ([]ai.Message, int) {
	callNames := toolCallNames(messages)

	// Index the compactable tool-result messages from the end.
	var toolMsgIdx []int
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role != ai.RoleTool {
			continue
		}
		if !messageHasCompactableResult(messages[i], callNames) {
			continue
		}
		toolMsgIdx = append(toolMsgIdx, i)
		if len(toolMsgIdx) >= keepRecent {
			break
		}
	}
	recent := make(map[int]bool, len(toolMsgIdx))
	for _, i := range toolMsgIdx {
		recent[i] = true
	}

	out := make([]ai.Message, len(messages))
	changed := false
	freedChars := 0
	for i, msg := range messages {
		if msg.Role != ai.RoleTool || recent[i] {
			out[i] = msg
			continue
		}
		newMsg := msg
		newMsg.Content = make([]ai.ContentPart, len(msg.Content))
		for j, part := range msg.Content {
			if part.Type == ai.ContentTypeToolResult && part.ToolResult != nil &&
				compactableTools[callNames[part.ToolResult.ToolCallID]] &&
				len(part.ToolResult.Content) > microCompactMinChars &&
				!strings.Contains(part.ToolResult.Content, "[Old tool result cleared") {
				cleared := *part.ToolResult
				preview := cleared.Content
				if len(preview) > 200 {
					preview = preview[:200]
				}
				freedChars += len(cleared.Content) - 200
				cleared.Content = fmt.Sprintf(
					"[Old tool result cleared to reclaim context. First %d chars retained: %s]",
					len(preview), preview)
				newMsg.Content[j] = ai.ContentPart{Type: ai.ContentTypeToolResult, ToolResult: &cleared}
				changed = true
			} else {
				newMsg.Content[j] = part
			}
		}
		out[i] = newMsg
	}
	if !changed {
		return messages, 0
	}
	return out, freedChars / 4
}

// messageHasCompactableResult reports whether a tool message contains at
// least one result from a compactable tool.
func messageHasCompactableResult(msg ai.Message, callNames map[string]string) bool {
	for _, part := range msg.Content {
		if part.Type == ai.ContentTypeToolResult && part.ToolResult != nil &&
			compactableTools[callNames[part.ToolResult.ToolCallID]] {
			return true
		}
	}
	return false
}

// messagesDiffer reports whether two same-length message slices diverge in
// any tool-result content (the only thing the tiers above rewrite in place).
func messagesDiffer(a, b []ai.Message) bool {
	for i := range a {
		if len(a[i].Content) != len(b[i].Content) {
			return true
		}
		for j := range a[i].Content {
			pa, pb := a[i].Content[j], b[i].Content[j]
			if pa.ToolResult != nil && pb.ToolResult != nil && pa.ToolResult.Content != pb.ToolResult.Content {
				return true
			}
			if pa.Text != pb.Text {
				return true
			}
		}
	}
	return false
}

// buildPostCompactAttachments re-attaches recently read files after a
// compaction so the model can keep working without re-reading them from
// disk. Bounded by file count and per-file token caps.
//
// The recent-file list comes from the agent's own tool activity (the skill
// tracker), falling back to the context manager's access log when no tool
// has touched a file path yet.
func (a *Agent) buildPostCompactAttachments() string {
	files := a.skillSnapshot()
	if len(files) == 0 {
		if mgr := a.ContextManager(); mgr != nil {
			files = mgr.RecentFiles(postCompactMaxFilesToRestore)
		}
	}
	if len(files) == 0 {
		return ""
	}

	// Most recent first; de-duplicated.
	seen := make(map[string]bool)
	ordered := make([]string, 0, len(files))
	for i := len(files) - 1; i >= 0; i-- {
		p := files[i]
		if seen[p] {
			continue
		}
		seen[p] = true
		ordered = append(ordered, p)
	}

	var sb strings.Builder
	sb.WriteString("## Files restored after compaction\n")
	sb.WriteString("These files were recently read and are re-attached (possibly truncated) so you can continue without re-reading them:\n\n")

	budget := postCompactTokenBudget
	attached := 0
	for _, path := range ordered {
		if budget <= 0 || attached >= postCompactMaxFilesToRestore {
			break
		}
		content, err := readCappedFile(path, postCompactMaxCharsPerFileHint)
		if err != nil || strings.TrimSpace(content) == "" {
			continue
		}
		tokens := len(content) / 4
		if tokens > postCompactMaxTokensPerFile {
			tokens = postCompactMaxTokensPerFile
		}
		sb.WriteString(fmt.Sprintf("### %s\n```%s\n%s\n```\n\n", path, fileLangHint(path), content))
		budget -= tokens
		attached++
	}
	if attached == 0 {
		return ""
	}
	return sb.String()
}

// readCappedFile reads at most maxChars bytes of a file, keeping the head of
// the content (the part most likely to hold imports/signatures).
func readCappedFile(path string, maxChars int) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	content := string(data)
	if len(content) > maxChars {
		content = content[:maxChars] + "\n... [truncated]"
	}
	return content, nil
}

// fileLangHint maps a file extension to a markdown fence language tag.
func fileLangHint(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go":
		return "go"
	case ".ts", ".tsx":
		return "ts"
	case ".js", ".jsx":
		return "js"
	case ".py":
		return "python"
	case ".rs":
		return "rust"
	case ".md":
		return "markdown"
	case ".json":
		return "json"
	case ".yaml", ".yml":
		return "yaml"
	default:
		return ""
	}
}
