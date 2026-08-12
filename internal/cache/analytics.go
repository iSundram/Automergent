package cache

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// Analytics provides detailed cache performance tracking.
type Analytics struct {
	mu sync.RWMutex

	// Overall metrics
	TotalRequests    int64
	TotalHits        int64
	TotalMisses      int64
	TotalEvictions   int64
	TotalBytesStored int64
	TotalBytesSaved  int64

	// Latency tracking
	TotalLatencyMs    int64
	CacheLatencyMs    int64
	NonCacheLatencyMs int64
	RequestCount      int64
	CacheRequestCount int64

	// Time series data (last 24 hours, 5-minute buckets)
	timeSeries []TimeSeriesEntry

	// Per-category metrics
	CategoryStats map[string]*CategoryMetrics

	// Session tracking
	SessionID string
	StartTime time.Time
}

// TimeSeriesEntry represents metrics for a time bucket.
type TimeSeriesEntry struct {
	Timestamp    time.Time
	Hits         int64
	Misses       int64
	HitRate      float64
	AvgLatencyMs float64
}

// CategoryMetrics tracks metrics for a specific cache category.
type CategoryMetrics struct {
	Hits         int64
	Misses       int64
	Evictions    int64
	BytesStored  int64
	AvgEntrySize int64
	OldestEntry  time.Time
	NewestEntry  time.Time
}

// NewAnalytics creates a new analytics tracker.
func NewAnalytics(sessionID string) *Analytics {
	return &Analytics{
		SessionID:     sessionID,
		StartTime:     time.Now(),
		CategoryStats: make(map[string]*CategoryMetrics),
		timeSeries:    make([]TimeSeriesEntry, 0, 288), // 24h * 12 (5-min buckets)
	}
}

// RecordHit records a cache hit.
func (a *Analytics) RecordHit(category string, latencyMs int64, bytesSaved int64) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.TotalRequests++
	a.TotalHits++
	a.TotalBytesSaved += bytesSaved
	a.CacheLatencyMs += latencyMs
	a.CacheRequestCount++

	a.ensureCategory(category)
	a.CategoryStats[category].Hits++

	a.updateTimeSeries(1, 0, latencyMs)
}

// RecordMiss records a cache miss.
func (a *Analytics) RecordMiss(category string, latencyMs int64) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.TotalRequests++
	a.TotalMisses++
	a.NonCacheLatencyMs += latencyMs
	a.RequestCount++

	a.ensureCategory(category)
	a.CategoryStats[category].Misses++

	a.updateTimeSeries(0, 1, latencyMs)
}

// RecordEviction records a cache eviction.
func (a *Analytics) RecordEviction(category string, bytesFreed int64) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.TotalEvictions++

	a.ensureCategory(category)
	a.CategoryStats[category].Evictions++
}

// RecordStore records data being stored in cache.
func (a *Analytics) RecordStore(category string, bytes int64) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.TotalBytesStored += bytes

	a.ensureCategory(category)
	stats := a.CategoryStats[category]
	stats.BytesStored += bytes
	stats.NewestEntry = time.Now()
	if stats.OldestEntry.IsZero() {
		stats.OldestEntry = time.Now()
	}
}

func (a *Analytics) ensureCategory(category string) {
	if _, ok := a.CategoryStats[category]; !ok {
		a.CategoryStats[category] = &CategoryMetrics{}
	}
}

func (a *Analytics) updateTimeSeries(hits, misses int64, latencyMs int64) {
	now := time.Now().Truncate(5 * time.Minute)

	// Find or create current bucket
	if len(a.timeSeries) == 0 || a.timeSeries[len(a.timeSeries)-1].Timestamp != now {
		// Create new bucket
		entry := TimeSeriesEntry{
			Timestamp: now,
			Hits:      hits,
			Misses:    misses,
		}
		if hits+misses > 0 {
			entry.HitRate = float64(hits) / float64(hits+misses) * 100
			entry.AvgLatencyMs = float64(latencyMs)
		}
		a.timeSeries = append(a.timeSeries, entry)

		// Keep only last 24 hours
		if len(a.timeSeries) > 288 {
			a.timeSeries = a.timeSeries[1:]
		}
	} else {
		// Update current bucket
		entry := &a.timeSeries[len(a.timeSeries)-1]
		entry.Hits += hits
		entry.Misses += misses
		total := entry.Hits + entry.Misses
		if total > 0 {
			entry.HitRate = float64(entry.Hits) / float64(total) * 100
		}
	}
}

// HitRate returns the overall cache hit rate as a percentage.
func (a *Analytics) HitRate() float64 {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.TotalRequests == 0 {
		return 0
	}
	return float64(a.TotalHits) / float64(a.TotalRequests) * 100
}

// AverageLatencySaved returns the average latency saved per cached request.
func (a *Analytics) AverageLatencySaved() float64 {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.CacheRequestCount == 0 || a.RequestCount == 0 {
		return 0
	}

	avgCacheLatency := float64(a.CacheLatencyMs) / float64(a.CacheRequestCount)
	avgNonCacheLatency := float64(a.NonCacheLatencyMs) / float64(a.RequestCount)

	return avgNonCacheLatency - avgCacheLatency
}

// LatencyReduction returns the percentage reduction in latency due to caching.
func (a *Analytics) LatencyReduction() float64 {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.RequestCount == 0 || a.NonCacheLatencyMs == 0 {
		return 0
	}

	avgNonCache := float64(a.NonCacheLatencyMs) / float64(a.RequestCount)
	avgCache := float64(0)
	if a.CacheRequestCount > 0 {
		avgCache = float64(a.CacheLatencyMs) / float64(a.CacheRequestCount)
	}

	if avgNonCache == 0 {
		return 0
	}
	return (avgNonCache - avgCache) / avgNonCache * 100
}

// CostSavings estimates cost savings based on cached tokens.
func (a *Analytics) CostSavings(inputPricePerMillion float64) float64 {
	a.mu.RLock()
	defer a.mu.RUnlock()

	// Estimate tokens from bytes (4 bytes per token)
	tokensSaved := a.TotalBytesSaved / 4
	return float64(tokensSaved) * inputPricePerMillion / 1_000_000
}

// Report generates a human-readable analytics report.
func (a *Analytics) Report() string {
	a.mu.RLock()
	defer a.mu.RUnlock()

	report := fmt.Sprintf(`Cache Analytics Report
======================
Session: %s
Duration: %s

Overall Metrics:
  Total Requests: %d
  Cache Hits: %d (%.1f%%)
  Cache Misses: %d
  Evictions: %d
  Bytes Stored: %s
  Bytes Saved: %s

Latency:
  Latency Reduction: %.1f%%
  Avg Latency Saved: %.2fms

Category Breakdown:
`,
		a.SessionID,
		time.Since(a.StartTime).Round(time.Second),
		a.TotalRequests,
		a.TotalHits, a.HitRate(),
		a.TotalMisses,
		a.TotalEvictions,
		formatBytes(a.TotalBytesStored),
		formatBytes(a.TotalBytesSaved),
		a.LatencyReduction(),
		a.AverageLatencySaved(),
	)

	for category, stats := range a.CategoryStats {
		hitRate := float64(0)
		if stats.Hits+stats.Misses > 0 {
			hitRate = float64(stats.Hits) / float64(stats.Hits+stats.Misses) * 100
		}
		report += fmt.Sprintf("  %s:\n    Hits: %d (%.1f%%), Misses: %d, Evictions: %d, Stored: %s\n",
			category, stats.Hits, hitRate, stats.Misses, stats.Evictions, formatBytes(stats.BytesStored))
	}

	return report
}

// JSON returns the analytics as JSON.
func (a *Analytics) JSON() ([]byte, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	data := map[string]interface{}{
		"session_id":        a.SessionID,
		"start_time":        a.StartTime,
		"duration_seconds":  time.Since(a.StartTime).Seconds(),
		"total_requests":    a.TotalRequests,
		"total_hits":        a.TotalHits,
		"total_misses":      a.TotalMisses,
		"hit_rate":          a.HitRate(),
		"total_evictions":   a.TotalEvictions,
		"bytes_stored":      a.TotalBytesStored,
		"bytes_saved":       a.TotalBytesSaved,
		"latency_reduction": a.LatencyReduction(),
		"category_stats":    a.CategoryStats,
		"time_series":       a.timeSeries,
	}

	return json.MarshalIndent(data, "", "  ")
}

// Snapshot returns a point-in-time snapshot of key metrics.
type MetricsSnapshot struct {
	Timestamp        time.Time
	HitRate          float64
	LatencyReduction float64
	BytesSaved       int64
	TotalRequests    int64
}

// Snapshot returns current metrics snapshot.
func (a *Analytics) Snapshot() MetricsSnapshot {
	a.mu.RLock()
	defer a.mu.RUnlock()

	return MetricsSnapshot{
		Timestamp:        time.Now(),
		HitRate:          a.HitRate(),
		LatencyReduction: a.LatencyReduction(),
		BytesSaved:       a.TotalBytesSaved,
		TotalRequests:    a.TotalRequests,
	}
}

// GetTimeSeries returns the time series data.
func (a *Analytics) GetTimeSeries() []TimeSeriesEntry {
	a.mu.RLock()
	defer a.mu.RUnlock()

	result := make([]TimeSeriesEntry, len(a.timeSeries))
	copy(result, a.timeSeries)
	return result
}

// Reset clears all analytics data.
func (a *Analytics) Reset() {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.TotalRequests = 0
	a.TotalHits = 0
	a.TotalMisses = 0
	a.TotalEvictions = 0
	a.TotalBytesStored = 0
	a.TotalBytesSaved = 0
	a.TotalLatencyMs = 0
	a.CacheLatencyMs = 0
	a.NonCacheLatencyMs = 0
	a.RequestCount = 0
	a.CacheRequestCount = 0
	a.timeSeries = make([]TimeSeriesEntry, 0, 288)
	a.CategoryStats = make(map[string]*CategoryMetrics)
	a.StartTime = time.Now()
}

func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
