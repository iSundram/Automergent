// Package cache provides prompt caching for AI providers.
// It implements multi-layer caching for system prompts, context, conversation
// history, and tool definitions to achieve ~50% latency reduction on cache hits.
package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"sync/atomic"
	"time"

	"github.com/iSundram/Automergent/internal/ai"
)

// CacheScope defines the visibility/TTL tier for cached content.
type CacheScope string

const (
	// ScopeEphemeral5m is for content cached for 5 minutes (default).
	ScopeEphemeral5m CacheScope = "ephemeral_5m"
	// ScopeEphemeral1h is for stable content cached for 1 hour.
	ScopeEphemeral1h CacheScope = "ephemeral_1h"
	// ScopeGlobal is for content shared across sessions (if supported).
	ScopeGlobal CacheScope = "global"
	// ScopeSession is for content valid for the current session only.
	ScopeSession CacheScope = "session"
)

// CacheControl describes caching behavior for a content block.
type CacheControl struct {
	Type  string     `json:"type"`            // "ephemeral" or "persistent"
	TTL   string     `json:"ttl,omitempty"`   // "5m" or "1h"
	Scope CacheScope `json:"scope,omitempty"` // optional scope
}

// DefaultCacheControl returns the default ephemeral cache control.
func DefaultCacheControl() *CacheControl {
	return &CacheControl{
		Type:  "ephemeral",
		Scope: ScopeEphemeral5m,
	}
}

// LongTTLCacheControl returns cache control for stable content (1h TTL).
func LongTTLCacheControl() *CacheControl {
	return &CacheControl{
		Type:  "ephemeral",
		TTL:   "1h",
		Scope: ScopeEphemeral1h,
	}
}

// ContentBlock represents a cacheable content block with optional cache control.
type ContentBlock struct {
	Type         string        `json:"type"`
	Text         string        `json:"text,omitempty"`
	CacheControl *CacheControl `json:"cache_control,omitempty"`
}

// CachedPrompt represents a system prompt split into cacheable blocks.
type CachedPrompt struct {
	Blocks    []ContentBlock `json:"blocks"`
	Hash      string         `json:"hash"` // Content hash for invalidation
	CreatedAt time.Time      `json:"created_at"`
	HitCount  int64          `json:"hit_count"`
}

// CacheEntry stores cached data with metadata.
type CacheEntry struct {
	Data       interface{}
	Hash       string
	Size       int64
	ExpiresAt  time.Time
	CreatedAt  time.Time
	LastAccess time.Time
	HitCount   int64
}

// IsExpired checks if the cache entry has expired.
func (e *CacheEntry) IsExpired() bool {
	return time.Now().After(e.ExpiresAt)
}

// Touch updates the last access time and increments hit count.
func (e *CacheEntry) Touch() {
	e.LastAccess = time.Now()
	atomic.AddInt64(&e.HitCount, 1)
}

// PromptCache is the main cache manager for prompt caching.
type PromptCache struct {
	mu sync.RWMutex

	// System prompt cache (static parts)
	systemPrompts map[string]*CacheEntry

	// Context cache (file content, project info)
	contexts map[string]*CacheEntry

	// Conversation history cache
	conversations map[string]*CacheEntry

	// Tool schema cache
	tools map[string]*CacheEntry

	// Configuration
	maxSize     int64 // Maximum total size in bytes
	defaultTTL  time.Duration
	longTTL     time.Duration
	currentSize int64

	// Analytics
	stats *CacheStats

	// Observability
	eventMu      sync.RWMutex
	events       []CacheEvent
	maxEvents    int
	eventHandler func(CacheEvent)

	invalidationMu      sync.RWMutex
	lastInvalidation    invalidationState
	invalidationVersion int64
}

// CacheStats tracks cache performance metrics.
type CacheStats struct {
	mu sync.RWMutex

	Hits           int64
	Misses         int64
	Evictions      int64
	TotalRequests  int64
	BytesSaved     int64
	LatencySavedMs int64

	// Per-category stats
	SystemPromptHits int64
	ContextHits      int64
	ConversationHits int64
	ToolHits         int64
}

// HitRate returns the cache hit rate as a percentage.
func (s *CacheStats) HitRate() float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.TotalRequests == 0 {
		return 0
	}
	return float64(s.Hits) / float64(s.TotalRequests) * 100
}

// NewPromptCache creates a new prompt cache with the given configuration.
func NewPromptCache(opts ...CacheOption) *PromptCache {
	c := &PromptCache{
		systemPrompts: make(map[string]*CacheEntry),
		contexts:      make(map[string]*CacheEntry),
		conversations: make(map[string]*CacheEntry),
		tools:         make(map[string]*CacheEntry),
		maxSize:       50 * 1024 * 1024, // 50MB default
		defaultTTL:    5 * time.Minute,
		longTTL:       time.Hour,
		stats:         &CacheStats{},
		maxEvents:     256,
	}

	for _, opt := range opts {
		opt(c)
	}

	// Start background cleanup
	go c.cleanupLoop()

	return c
}

// CacheOption configures the cache.
type CacheOption func(*PromptCache)

// WithMaxSize sets the maximum cache size in bytes.
func WithMaxSize(size int64) CacheOption {
	return func(c *PromptCache) {
		c.maxSize = size
	}
}

// WithDefaultTTL sets the default time-to-live for cache entries.
func WithDefaultTTL(ttl time.Duration) CacheOption {
	return func(c *PromptCache) {
		c.defaultTTL = ttl
	}
}

// WithLongTTL sets the long TTL for stable content.
func WithLongTTL(ttl time.Duration) CacheOption {
	return func(c *PromptCache) {
		c.longTTL = ttl
	}
}

// hashContent generates a SHA-256 hash for content.
func hashContent(content string) string {
	h := sha256.Sum256([]byte(content))
	return hex.EncodeToString(h[:])
}

// CacheSystemPrompt caches a system prompt split into cacheable blocks.
func (c *PromptCache) CacheSystemPrompt(key string, prompt string, longTTL bool) *CachedPrompt {
	c.mu.Lock()
	defer c.mu.Unlock()

	hash := hashContent(prompt)

	// Check if already cached with same hash
	if entry, ok := c.systemPrompts[key]; ok {
		if entry.Hash == hash && !entry.IsExpired() {
			entry.Touch()
			atomic.AddInt64(&c.stats.Hits, 1)
			atomic.AddInt64(&c.stats.SystemPromptHits, 1)
			atomic.AddInt64(&c.stats.TotalRequests, 1)
			c.emitHitMiss("system_prompt", key, true, "hash_match")
			return entry.Data.(*CachedPrompt)
		}
	}

	atomic.AddInt64(&c.stats.Misses, 1)
	atomic.AddInt64(&c.stats.TotalRequests, 1)
	c.emitHitMiss("system_prompt", key, false, "not_cached")

	// Build cached prompt with cache control
	blocks := splitPromptForCaching(prompt)
	cached := &CachedPrompt{
		Blocks:    blocks,
		Hash:      hash,
		CreatedAt: time.Now(),
	}

	ttl := c.defaultTTL
	if longTTL {
		ttl = c.longTTL
	}

	size := int64(len(prompt))
	c.maybeEvict(size)

	c.systemPrompts[key] = &CacheEntry{
		Data:       cached,
		Hash:       hash,
		Size:       size,
		ExpiresAt:  time.Now().Add(ttl),
		CreatedAt:  time.Now(),
		LastAccess: time.Now(),
	}
	c.currentSize += size

	return cached
}

// GetSystemPrompt retrieves a cached system prompt.
func (c *PromptCache) GetSystemPrompt(key string) (*CachedPrompt, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.systemPrompts[key]
	if !ok || entry.IsExpired() {
		c.emitHitMiss("system_prompt", key, false, "not_found_or_expired")
		return nil, false
	}

	entry.Touch()
	c.emitHitMiss("system_prompt", key, true, "get")
	return entry.Data.(*CachedPrompt), true
}

// CacheContext caches context data (file content, project info).
func (c *PromptCache) CacheContext(key string, content string, longTTL bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	hash := hashContent(content)

	if entry, ok := c.contexts[key]; ok {
		if entry.Hash == hash && !entry.IsExpired() {
			entry.Touch()
			atomic.AddInt64(&c.stats.Hits, 1)
			atomic.AddInt64(&c.stats.ContextHits, 1)
			atomic.AddInt64(&c.stats.TotalRequests, 1)
			c.emitHitMiss("context", key, true, "hash_match")
			return
		}
	}

	atomic.AddInt64(&c.stats.Misses, 1)
	atomic.AddInt64(&c.stats.TotalRequests, 1)
	c.emitHitMiss("context", key, false, "not_cached")

	ttl := c.defaultTTL
	if longTTL {
		ttl = c.longTTL
	}

	size := int64(len(content))
	c.maybeEvict(size)

	c.contexts[key] = &CacheEntry{
		Data:       content,
		Hash:       hash,
		Size:       size,
		ExpiresAt:  time.Now().Add(ttl),
		CreatedAt:  time.Now(),
		LastAccess: time.Now(),
	}
	c.currentSize += size
}

// GetContext retrieves cached context data.
func (c *PromptCache) GetContext(key string) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.contexts[key]
	if !ok || entry.IsExpired() {
		c.emitHitMiss("context", key, false, "not_found_or_expired")
		return "", false
	}

	entry.Touch()
	c.emitHitMiss("context", key, true, "get")
	return entry.Data.(string), true
}

// CacheToolSchemas caches tool definitions.
func (c *PromptCache) CacheToolSchemas(key string, tools []ai.ToolSchema) {
	c.mu.Lock()
	defer c.mu.Unlock()

	hash := hashToolSchemas(tools)

	if entry, ok := c.tools[key]; ok {
		if entry.Hash == hash && !entry.IsExpired() {
			entry.Touch()
			atomic.AddInt64(&c.stats.Hits, 1)
			atomic.AddInt64(&c.stats.ToolHits, 1)
			atomic.AddInt64(&c.stats.TotalRequests, 1)
			c.emitHitMiss("tools", key, true, "hash_match")
			return
		}
	}

	atomic.AddInt64(&c.stats.Misses, 1)
	atomic.AddInt64(&c.stats.TotalRequests, 1)
	c.emitHitMiss("tools", key, false, "not_cached")

	// Use long TTL for tools as they rarely change
	size := estimateToolSchemaSize(tools)
	c.maybeEvict(size)

	c.tools[key] = &CacheEntry{
		Data:       tools,
		Hash:       hash,
		Size:       size,
		ExpiresAt:  time.Now().Add(c.longTTL),
		CreatedAt:  time.Now(),
		LastAccess: time.Now(),
	}
	c.currentSize += size
}

// GetToolSchemas retrieves cached tool schemas.
func (c *PromptCache) GetToolSchemas(key string) ([]ai.ToolSchema, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.tools[key]
	if !ok || entry.IsExpired() {
		c.emitHitMiss("tools", key, false, "not_found_or_expired")
		return nil, false
	}

	entry.Touch()
	c.emitHitMiss("tools", key, true, "get")
	return entry.Data.([]ai.ToolSchema), true
}

// CacheConversation caches conversation history.
func (c *PromptCache) CacheConversation(sessionID string, messages []ai.Message) {
	c.mu.Lock()
	defer c.mu.Unlock()

	hash := hashMessages(messages)

	if entry, ok := c.conversations[sessionID]; ok {
		if entry.Hash == hash && !entry.IsExpired() {
			entry.Touch()
			atomic.AddInt64(&c.stats.Hits, 1)
			atomic.AddInt64(&c.stats.ConversationHits, 1)
			atomic.AddInt64(&c.stats.TotalRequests, 1)
			c.emitHitMiss("conversation", sessionID, true, "hash_match")
			return
		}
	}

	atomic.AddInt64(&c.stats.Misses, 1)
	atomic.AddInt64(&c.stats.TotalRequests, 1)
	c.emitHitMiss("conversation", sessionID, false, "not_cached")

	size := estimateMessagesSize(messages)
	c.maybeEvict(size)

	c.conversations[sessionID] = &CacheEntry{
		Data:       messages,
		Hash:       hash,
		Size:       size,
		ExpiresAt:  time.Now().Add(c.defaultTTL),
		CreatedAt:  time.Now(),
		LastAccess: time.Now(),
	}
	c.currentSize += size
}

// GetConversation retrieves cached conversation history.
func (c *PromptCache) GetConversation(sessionID string) ([]ai.Message, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.conversations[sessionID]
	if !ok || entry.IsExpired() {
		c.emitHitMiss("conversation", sessionID, false, "not_found_or_expired")
		return nil, false
	}

	entry.Touch()
	c.emitHitMiss("conversation", sessionID, true, "get")
	return entry.Data.([]ai.Message), true
}

// InvalidateContext removes a context entry from cache.
func (c *PromptCache) InvalidateContext(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if entry, ok := c.contexts[key]; ok {
		c.currentSize -= entry.Size
		delete(c.contexts, key)
		c.emitInvalidation("context", key, "manual_invalidation", 1, entry.Size)
	}
}

// InvalidateConversation removes a conversation from cache.
func (c *PromptCache) InvalidateConversation(sessionID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if entry, ok := c.conversations[sessionID]; ok {
		c.currentSize -= entry.Size
		delete(c.conversations, sessionID)
		c.emitInvalidation("conversation", sessionID, "manual_invalidation", 1, entry.Size)
	}
}

// Clear removes all entries from the cache.
func (c *PromptCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	clearedEntries := len(c.systemPrompts) + len(c.contexts) + len(c.conversations) + len(c.tools)
	clearedBytes := c.currentSize
	c.systemPrompts = make(map[string]*CacheEntry)
	c.contexts = make(map[string]*CacheEntry)
	c.conversations = make(map[string]*CacheEntry)
	c.tools = make(map[string]*CacheEntry)
	c.currentSize = 0
	c.emitInvalidation("all", "", "clear_all", clearedEntries, clearedBytes)
}

// Stats returns a copy of the cache statistics.
func (c *PromptCache) Stats() CacheStats {
	c.stats.mu.RLock()
	defer c.stats.mu.RUnlock()
	return CacheStats{
		Hits:             c.stats.Hits,
		Misses:           c.stats.Misses,
		Evictions:        c.stats.Evictions,
		TotalRequests:    c.stats.TotalRequests,
		BytesSaved:       c.stats.BytesSaved,
		LatencySavedMs:   c.stats.LatencySavedMs,
		SystemPromptHits: c.stats.SystemPromptHits,
		ContextHits:      c.stats.ContextHits,
		ConversationHits: c.stats.ConversationHits,
		ToolHits:         c.stats.ToolHits,
	}
}

// Size returns the current cache size in bytes.
func (c *PromptCache) Size() int64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.currentSize
}

// maybeEvict removes entries if we're over the size limit using LRU policy.
func (c *PromptCache) maybeEvict(incoming int64) {
	targetSize := c.maxSize - incoming
	if c.currentSize <= targetSize {
		return
	}

	// Collect all entries with their access times
	type entryInfo struct {
		key        string
		category   string
		lastAccess time.Time
		size       int64
	}
	var entries []entryInfo

	for k, e := range c.systemPrompts {
		entries = append(entries, entryInfo{k, "system", e.LastAccess, e.Size})
	}
	for k, e := range c.contexts {
		entries = append(entries, entryInfo{k, "context", e.LastAccess, e.Size})
	}
	for k, e := range c.conversations {
		entries = append(entries, entryInfo{k, "conversation", e.LastAccess, e.Size})
	}
	for k, e := range c.tools {
		entries = append(entries, entryInfo{k, "tool", e.LastAccess, e.Size})
	}

	// Sort by last access (oldest first) - simple bubble sort for small lists
	for i := 0; i < len(entries); i++ {
		for j := i + 1; j < len(entries); j++ {
			if entries[j].lastAccess.Before(entries[i].lastAccess) {
				entries[i], entries[j] = entries[j], entries[i]
			}
		}
	}

	// Evict until we're under the limit
	for _, e := range entries {
		if c.currentSize <= targetSize {
			break
		}

		switch e.category {
		case "system":
			delete(c.systemPrompts, e.key)
		case "context":
			delete(c.contexts, e.key)
		case "conversation":
			delete(c.conversations, e.key)
		case "tool":
			delete(c.tools, e.key)
		}
		c.currentSize -= e.size
		atomic.AddInt64(&c.stats.Evictions, 1)
		c.emitInvalidation(e.category, e.key, "eviction", 1, e.size)
	}
}

// cleanupLoop periodically removes expired entries.
func (c *PromptCache) cleanupLoop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		c.cleanup()
	}
}

// cleanup removes expired entries from the cache.
func (c *PromptCache) cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	var expiredEntries int
	var expiredBytes int64
	for k, e := range c.systemPrompts {
		if e.IsExpired() {
			c.currentSize -= e.Size
			delete(c.systemPrompts, k)
			expiredEntries++
			expiredBytes += e.Size
		}
	}
	for k, e := range c.contexts {
		if e.IsExpired() {
			c.currentSize -= e.Size
			delete(c.contexts, k)
			expiredEntries++
			expiredBytes += e.Size
		}
	}
	for k, e := range c.conversations {
		if e.IsExpired() {
			c.currentSize -= e.Size
			delete(c.conversations, k)
			expiredEntries++
			expiredBytes += e.Size
		}
	}
	for k, e := range c.tools {
		if e.IsExpired() {
			c.currentSize -= e.Size
			delete(c.tools, k)
			expiredEntries++
			expiredBytes += e.Size
		}
	}
	if expiredEntries > 0 {
		c.emitInvalidation("all", "", "ttl_expired", expiredEntries, expiredBytes)
	}
}
