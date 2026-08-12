package cache

import (
	"sync"
	"time"
)

// Memoizer provides memoization for expensive computations.
type Memoizer[K comparable, V any] struct {
	mu      sync.RWMutex
	entries map[K]*memoEntry[V]
	maxSize int
	ttl     time.Duration
}

type memoEntry[V any] struct {
	value      V
	expiresAt  time.Time
	lastAccess time.Time
	hitCount   int64
}

// NewMemoizer creates a new memoizer with the given configuration.
func NewMemoizer[K comparable, V any](maxSize int, ttl time.Duration) *Memoizer[K, V] {
	m := &Memoizer[K, V]{
		entries: make(map[K]*memoEntry[V]),
		maxSize: maxSize,
		ttl:     ttl,
	}

	// Start cleanup goroutine
	go m.cleanupLoop()

	return m
}

// Get retrieves a memoized value or computes it if not present.
func (m *Memoizer[K, V]) Get(key K, compute func() V) V {
	// Try read path first
	m.mu.RLock()
	if entry, ok := m.entries[key]; ok && time.Now().Before(entry.expiresAt) {
		entry.lastAccess = time.Now()
		entry.hitCount++
		value := entry.value
		m.mu.RUnlock()
		return value
	}
	m.mu.RUnlock()

	// Compute and store
	m.mu.Lock()
	defer m.mu.Unlock()

	// Double-check after acquiring write lock
	if entry, ok := m.entries[key]; ok && time.Now().Before(entry.expiresAt) {
		entry.lastAccess = time.Now()
		entry.hitCount++
		return entry.value
	}

	// Evict if at capacity
	if len(m.entries) >= m.maxSize {
		m.evictLRU()
	}

	// Compute new value
	value := compute()
	m.entries[key] = &memoEntry[V]{
		value:      value,
		expiresAt:  time.Now().Add(m.ttl),
		lastAccess: time.Now(),
	}

	return value
}

// GetIfPresent retrieves a memoized value without computing.
func (m *Memoizer[K, V]) GetIfPresent(key K) (V, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	entry, ok := m.entries[key]
	if !ok || time.Now().After(entry.expiresAt) {
		var zero V
		return zero, false
	}

	entry.lastAccess = time.Now()
	entry.hitCount++
	return entry.value, true
}

// Set stores a value in the memoizer.
func (m *Memoizer[K, V]) Set(key K, value V) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.entries) >= m.maxSize {
		m.evictLRU()
	}

	m.entries[key] = &memoEntry[V]{
		value:      value,
		expiresAt:  time.Now().Add(m.ttl),
		lastAccess: time.Now(),
	}
}

// Invalidate removes a key from the memoizer.
func (m *Memoizer[K, V]) Invalidate(key K) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.entries, key)
}

// Clear removes all entries.
func (m *Memoizer[K, V]) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries = make(map[K]*memoEntry[V])
}

// Size returns the number of cached entries.
func (m *Memoizer[K, V]) Size() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.entries)
}

// evictLRU removes the least recently used entry.
func (m *Memoizer[K, V]) evictLRU() {
	var oldestKey K
	var oldestTime time.Time

	for k, e := range m.entries {
		if oldestTime.IsZero() || e.lastAccess.Before(oldestTime) {
			oldestKey = k
			oldestTime = e.lastAccess
		}
	}

	delete(m.entries, oldestKey)
}

// cleanupLoop periodically removes expired entries.
func (m *Memoizer[K, V]) cleanupLoop() {
	ticker := time.NewTicker(m.ttl / 2)
	defer ticker.Stop()

	for range ticker.C {
		m.cleanup()
	}
}

// cleanup removes expired entries.
func (m *Memoizer[K, V]) cleanup() {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	for k, e := range m.entries {
		if now.After(e.expiresAt) {
			delete(m.entries, k)
		}
	}
}

// ToolSchemaMemoizer is a specialized memoizer for tool schemas.
type ToolSchemaMemoizer struct {
	*Memoizer[string, []byte]
}

// NewToolSchemaMemoizer creates a memoizer for tool schemas.
func NewToolSchemaMemoizer() *ToolSchemaMemoizer {
	return &ToolSchemaMemoizer{
		Memoizer: NewMemoizer[string, []byte](1000, time.Hour),
	}
}

// SystemPromptMemoizer is a specialized memoizer for system prompts.
type SystemPromptMemoizer struct {
	*Memoizer[string, *CachedPrompt]
}

// NewSystemPromptMemoizer creates a memoizer for system prompts.
func NewSystemPromptMemoizer() *SystemPromptMemoizer {
	return &SystemPromptMemoizer{
		Memoizer: NewMemoizer[string, *CachedPrompt](100, time.Hour),
	}
}

// ContentHashMemoizer memoizes content hashes to avoid recomputation.
type ContentHashMemoizer struct {
	*Memoizer[string, string]
}

// NewContentHashMemoizer creates a memoizer for content hashes.
func NewContentHashMemoizer() *ContentHashMemoizer {
	return &ContentHashMemoizer{
		Memoizer: NewMemoizer[string, string](10000, 30*time.Minute),
	}
}

// GetOrComputeHash returns a cached hash or computes it.
func (m *ContentHashMemoizer) GetOrComputeHash(content string) string {
	// Use content prefix as key to avoid storing full content
	key := content
	if len(key) > 256 {
		key = content[:256]
	}

	return m.Get(key, func() string {
		return hashContent(content)
	})
}

// BatchMemoizer supports batch lookups and insertions.
type BatchMemoizer[K comparable, V any] struct {
	*Memoizer[K, V]
}

// NewBatchMemoizer creates a memoizer with batch support.
func NewBatchMemoizer[K comparable, V any](maxSize int, ttl time.Duration) *BatchMemoizer[K, V] {
	return &BatchMemoizer[K, V]{
		Memoizer: NewMemoizer[K, V](maxSize, ttl),
	}
}

// GetBatch retrieves multiple values, computing missing ones.
func (m *BatchMemoizer[K, V]) GetBatch(keys []K, compute func(K) V) map[K]V {
	results := make(map[K]V, len(keys))
	var missing []K

	// First pass: get cached values
	m.mu.RLock()
	for _, k := range keys {
		if entry, ok := m.entries[k]; ok && time.Now().Before(entry.expiresAt) {
			entry.lastAccess = time.Now()
			entry.hitCount++
			results[k] = entry.value
		} else {
			missing = append(missing, k)
		}
	}
	m.mu.RUnlock()

	// Second pass: compute and store missing values
	if len(missing) > 0 {
		m.mu.Lock()
		for _, k := range missing {
			// Double-check
			if entry, ok := m.entries[k]; ok && time.Now().Before(entry.expiresAt) {
				entry.lastAccess = time.Now()
				entry.hitCount++
				results[k] = entry.value
				continue
			}

			// Evict if needed
			if len(m.entries) >= m.maxSize {
				m.evictLRU()
			}

			// Compute
			value := compute(k)
			m.entries[k] = &memoEntry[V]{
				value:      value,
				expiresAt:  time.Now().Add(m.ttl),
				lastAccess: time.Now(),
			}
			results[k] = value
		}
		m.mu.Unlock()
	}

	return results
}

// SetBatch stores multiple values.
func (m *BatchMemoizer[K, V]) SetBatch(values map[K]V) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for k, v := range values {
		if len(m.entries) >= m.maxSize {
			m.evictLRU()
		}
		m.entries[k] = &memoEntry[V]{
			value:      v,
			expiresAt:  time.Now().Add(m.ttl),
			lastAccess: time.Now(),
		}
	}
}
