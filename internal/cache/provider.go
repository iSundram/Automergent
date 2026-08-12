package cache

import (
	"context"
	"sync"

	"github.com/iSundram/Automergent/internal/ai"
)

// CachingProvider wraps an AI provider with prompt caching support.
type CachingProvider struct {
	provider ai.Provider
	cache    *PromptCache
	mu       sync.RWMutex

	// Configuration
	enableSystemPromptCache bool
	enableContextCache      bool
	enableToolCache         bool
	enableConversationCache bool
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
	// Apply caching optimizations
	optimizedReq := cp.optimizeRequest(req)

	// Execute completion
	resp, err := cp.provider.Complete(ctx, optimizedReq)
	if err != nil {
		return nil, err
	}

	// Update cache metrics from response
	usage := resp.Usage()
	if usage.CacheHits > 0 {
		cp.cache.stats.mu.Lock()
		cp.cache.stats.BytesSaved += int64(usage.CacheHits * 4) // Approximate bytes
		cp.cache.stats.mu.Unlock()
	}

	return resp, nil
}

// optimizeRequest applies caching optimizations to the request.
func (cp *CachingProvider) optimizeRequest(req ai.CompletionRequest) ai.CompletionRequest {
	// Cache system prompt if enabled
	if cp.enableSystemPromptCache && req.System != "" {
		key := CreateCacheKey("system", req.System[:min(64, len(req.System))])
		cached := cp.cache.CacheSystemPrompt(key, req.System, true)
		if cached != nil {
			// System prompt is cached, provider will use cache_control markers
			_ = cached
		}
	}

	// Cache tool schemas if enabled
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
