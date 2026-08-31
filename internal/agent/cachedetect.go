package agent

import (
	"encoding/json"
	"fmt"
	"hash/fnv"

	"github.com/iSundram/Automergent/internal/ai"
)

// Prompt-cache break detection (minimal port of the reference agent's
// promptCacheBreakDetection).
//
// Providers cache the request prefix across calls. When consecutive calls
// with an IDENTICAL cache-relevant prefix (system prompt + tool schemas +
// model) show a large drop in cache-hit tokens, the cache was broken —
// either by us (something in the prefix changed) or by the server (TTL
// expiry, eviction). The full reference implementation attributes blame
// per-field and writes diffs; this port keeps the actionable core:
// unchanged prefix + big drop ⇒ server-side or TTL; changed prefix ⇒ ours,
// and the event says by how much the prompt grew.
//
// Compaction legitimately reduces the message tail (not the prefix), so
// compaction resets the baseline — a post-compact drop is expected, not a
// break.

const (
	// cacheBreakRelativeDrop is the fraction of previous cache-hit tokens
	// that must survive for the cache to be considered intact.
	cacheBreakRelativeDrop = 0.95
	// cacheBreakMinTokens ignores small absolute drops (normal variance).
	cacheBreakMinTokens = 2000
)

// promptCacheState tracks the previous call's cache-relevant inputs so the
// next call can explain a cache-hit drop.
type promptCacheState struct {
	prefixHash  uint64
	promptChars int
	cacheHits   int
	valid       bool
}

// cachePrefixHash hashes everything the provider keys its prompt cache on
// from our side: model, system prompt, and tool schemas. Message history is
// deliberately excluded — it is the part that is SUPPOSED to grow.
func cachePrefixHash(model, systemPrompt string, toolSchemas []ai.ToolSchema) (uint64, int) {
	h := fnv.New64a()
	h.Write([]byte(model))
	h.Write([]byte{0})
	h.Write([]byte(systemPrompt))
	h.Write([]byte{0})
	for _, ts := range toolSchemas {
		h.Write([]byte(ts.Name))
		h.Write([]byte{0})
		b, _ := json.Marshal(ts.Parameters)
		h.Write(b)
		h.Write([]byte{0})
	}
	return h.Sum64(), len(systemPrompt)
}

// observePromptCache records one provider call's cache inputs and usage,
// reporting a human-readable cache-break explanation when the cache-hit
// tokens collapsed between two calls. Returns "" when nothing notable
// happened.
func (a *Agent) observePromptCache(model, systemPrompt string, toolSchemas []ai.ToolSchema, usage ai.Usage) string {
	hash, chars := cachePrefixHash(model, systemPrompt, toolSchemas)

	a.mu.Lock()
	prev := a.promptCache
	a.promptCache = promptCacheState{
		prefixHash:  hash,
		promptChars: chars,
		cacheHits:   usage.CacheHits,
		valid:       true,
	}
	a.mu.Unlock()

	if !prev.valid || prev.cacheHits <= 0 {
		return ""
	}
	drop := prev.cacheHits - usage.CacheHits
	if float64(usage.CacheHits) >= float64(prev.cacheHits)*cacheBreakRelativeDrop || drop < cacheBreakMinTokens {
		return ""
	}

	if hash != prev.prefixHash {
		return fmt.Sprintf(
			"prompt cache broken by us: system prompt changed (Δ%d chars) — cache hits %d → %d",
			chars-prev.promptChars, prev.cacheHits, usage.CacheHits)
	}
	return fmt.Sprintf(
		"prompt cache broken server-side (prefix unchanged): cache hits %d → %d — likely TTL expiry or eviction",
		prev.cacheHits, usage.CacheHits)
}

// resetPromptCacheBaseline clears the cache-break baseline after an event
// that legitimately changes the message tail (compaction) or the whole
// conversation (new session), so the expected drop is not flagged.
func (a *Agent) resetPromptCacheBaseline() {
	a.mu.Lock()
	a.promptCache = promptCacheState{}
	a.mu.Unlock()
}
