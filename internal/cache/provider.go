package cache

import (
	"context"
	"sync"
	"time"

	"github.com/iSundram/Automergent/internal/ai"
)

// CachingProvider wraps an AI provider with prompt caching support.
type CachingProvider struct {
	provider ai.Provider
	cache    *PromptCache
	mu       sync.RWMutex
	lastCall *cacheCallSnapshot

	// Configuration
	enableSystemPromptCache bool
	enableContextCache      bool
	enableToolCache         bool
	enableConversationCache bool
}

type cacheCallSnapshot struct {
	At                  time.Time
	Model               string
	SystemHash          string
	ToolsHash           string
	CacheHits           int
	InvalidationVersion int64
}

// NewCachingProvider creates a provider wrapper with caching support.
func NewCachingProvider(provider ai.Provider, cache *PromptCache, opts ...ProviderOption) *CachingProvider {
	cp := &CachingProvider{
		provider:                provider,
		cache:                   cache,
		enableSystemPromptCache: true,
		enableContextCache:      true,
		enableToolCache:         true,
		enableConversationCache: true,
	}

	for _, opt := range opts {
		opt(cp)
	}

	return cp
}

// ProviderOption configures a caching provider.
type ProviderOption func(*CachingProvider)

// WithSystemPromptCache enables/disables system prompt caching.
func WithSystemPromptCache(enable bool) ProviderOption {
	return func(cp *CachingProvider) {
		cp.enableSystemPromptCache = enable
	}
}

// WithContextCache enables/disables context caching.
func WithContextCache(enable bool) ProviderOption {
	return func(cp *CachingProvider) {
		cp.enableContextCache = enable
	}
}

// WithToolCache enables/disables tool schema caching.
func WithToolCache(enable bool) ProviderOption {
	return func(cp *CachingProvider) {
		cp.enableToolCache = enable
	}
}

// WithConversationCache enables/disables conversation history caching.
func WithConversationCache(enable bool) ProviderOption {
	return func(cp *CachingProvider) {
		cp.enableConversationCache = enable
	}
}

// Name returns the underlying provider name.
func (cp *CachingProvider) Name() string {
	return cp.provider.Name()
}

// ContextLimit returns the underlying provider's context limit.
func (cp *CachingProvider) ContextLimit() int {
	return cp.provider.ContextLimit()
}

// Models returns available models from the underlying provider.
func (cp *CachingProvider) Models(ctx context.Context) ([]ai.Model, error) {
	return cp.provider.Models(ctx)
}

// TokenCount returns the token count for messages.
func (cp *CachingProvider) TokenCount(messages []ai.Message) (int, error) {
	return cp.provider.TokenCount(messages)
}

// Complete performs a completion with caching support.
func (cp *CachingProvider) Complete(ctx context.Context, req ai.CompletionRequest) (ai.CompletionResponse, error) {
	snapshot := cp.buildSnapshot(req)

	// Apply caching optimizations
	optimizedReq := cp.optimizeRequest(req)

	// Execute completion
	resp, err := cp.provider.Complete(ctx, optimizedReq)
	if err != nil {
		return nil, err
	}

	// Update cache metrics from response
	usage := resp.Usage()
	if cp.cache != nil && usage.CacheHits > 0 {
		cp.cache.stats.mu.Lock()
		cp.cache.stats.BytesSaved += int64(usage.CacheHits * 4) // Approximate bytes
		cp.cache.stats.mu.Unlock()
	}
	if cp.cache != nil {
		cp.detectCacheBreak(snapshot, usage.CacheHits)
	}

	return resp, nil
}

// optimizeRequest applies caching optimizations to the request.
func (cp *CachingProvider) optimizeRequest(req ai.CompletionRequest) ai.CompletionRequest {
	if cp.cache == nil {
		return req
	}

	// Cache system prompt if enabled
	if req.System != "" {
		key := CreateCacheKey("system", req.System[:min(64, len(req.System))])
		classification := NewBoundaryDetector().ClassifyContent(req.System)
		eligible := cp.enableSystemPromptCache && ShouldCache(req.System, classification)
		reason := "eligible"
		switch {
		case !cp.enableSystemPromptCache:
			reason = "system_cache_disabled"
		case !eligible && classification == ClassificationVolatile:
			reason = "volatile_content"
		case !eligible && len(req.System) < 100:
			reason = "content_too_small"
		case !eligible && len(req.System) > 1024*1024:
			reason = "content_too_large"
		case !eligible:
			reason = "not_cacheable"
		}
		cp.cache.emitEligibility("system_prompt", key, eligible, reason)
		if eligible {
			cached := cp.cache.CacheSystemPrompt(key, req.System, true)
			if cached != nil {
				// System prompt is cached, provider will use cache_control markers
				_ = cached
			}
		}
	}

	// Cache tool schemas if enabled
	if len(req.Tools) > 0 {
		key := CreateCacheKey("tools", cp.provider.Name())
		reason := "eligible"
		if !cp.enableToolCache {
			reason = "tool_cache_disabled"
		}
		cp.cache.emitEligibility("tools", key, cp.enableToolCache, reason)
	}
	if cp.enableToolCache && len(req.Tools) > 0 {
		key := CreateCacheKey("tools", cp.provider.Name())
		cp.cache.CacheToolSchemas(key, req.Tools)
	}

	return req
}

// GetCache returns the underlying cache instance.
func (cp *CachingProvider) GetCache() *PromptCache {
	return cp.cache
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (cp *CachingProvider) buildSnapshot(req ai.CompletionRequest) cacheCallSnapshot {
	if cp.cache == nil {
		return cacheCallSnapshot{
			At:         time.Now(),
			Model:      cp.provider.Name(),
			SystemHash: hashContent(req.System),
			ToolsHash:  hashToolSchemas(req.Tools),
		}
	}

	version, _, _ := cp.cache.InvalidationState()
	return cacheCallSnapshot{
		At:                  time.Now(),
		Model:               cp.provider.Name(),
		SystemHash:          hashContent(req.System),
		ToolsHash:           hashToolSchemas(req.Tools),
		InvalidationVersion: version,
	}
}

func (cp *CachingProvider) detectCacheBreak(snapshot cacheCallSnapshot, cacheHits int) {
	cp.mu.Lock()
	defer cp.mu.Unlock()

	prev := cp.lastCall
	snapshot.CacheHits = cacheHits
	cp.lastCall = &snapshot
	if prev == nil || prev.CacheHits <= 0 || cacheHits >= prev.CacheHits {
		return
	}

	drop := prev.CacheHits - cacheHits
	dropPct := float64(drop) / float64(prev.CacheHits)
	if dropPct < 0.05 {
		return
	}

	reasons := make([]string, 0, 4)
	if prev.Model != snapshot.Model {
		reasons = append(reasons, "model_changed")
	}
	if prev.SystemHash != snapshot.SystemHash {
		reasons = append(reasons, "system_prompt_changed")
	}
	if prev.ToolsHash != snapshot.ToolsHash {
		reasons = append(reasons, "tool_schemas_changed")
	}
	if prev.InvalidationVersion != snapshot.InvalidationVersion {
		reasons = append(reasons, "cache_invalidated")
	}
	if len(reasons) == 0 {
		gap := snapshot.At.Sub(prev.At)
		if gap >= cp.cache.longTTL {
			reasons = append(reasons, "ttl_expired_1h")
		} else if gap >= cp.cache.defaultTTL {
			reasons = append(reasons, "ttl_expired_5m")
		} else {
			reasons = append(reasons, "likely_server_side")
		}
	}

	cp.cache.emitBreak(
		joinReasons(reasons),
		map[string]any{
			"previous_cache_hits": prev.CacheHits,
			"current_cache_hits":  cacheHits,
			"drop_tokens":         drop,
			"drop_percent":        dropPct * 100,
			"provider":            cp.provider.Name(),
		},
	)
}

func joinReasons(reasons []string) string {
	if len(reasons) == 0 {
		return "unknown"
	}
	if len(reasons) == 1 {
		return reasons[0]
	}
	out := reasons[0]
	for i := 1; i < len(reasons); i++ {
		out += "," + reasons[i]
	}
	return out
}
