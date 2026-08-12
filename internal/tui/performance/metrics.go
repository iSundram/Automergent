// Package performance provides TUI performance monitoring and metrics.
package performance

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// Metrics tracks TUI performance metrics for monitoring and optimization.
type Metrics struct {
	mu sync.RWMutex

	// Frame timing
	frameCount     atomic.Uint64
	lastFrameTime  time.Time
	frameDurations []time.Duration
	maxFrames      int

	// Render metrics
	totalRenderTime   atomic.Int64
	renderCount       atomic.Uint64
	lastRenderStart   time.Time
	currentRenderTime time.Duration

	// Memory (tracked externally)
	lastMemoryUsage uint64

	// Input latency
	inputLatencies []time.Duration
	maxLatencies   int

	// Timestamps
	startTime time.Time
}

// NewMetrics creates a new performance metrics tracker.
func NewMetrics() *Metrics {
	return &Metrics{
		maxFrames:      120, // Track last 2 seconds at 60fps
		maxLatencies:   100,
		frameDurations: make([]time.Duration, 0, 120),
		inputLatencies: make([]time.Duration, 0, 100),
		startTime:      time.Now(),
		lastFrameTime:  time.Now(),
	}
}

// BeginFrame marks the start of a new frame.
func (m *Metrics) BeginFrame() {
	m.mu.Lock()
	now := time.Now()
	if !m.lastFrameTime.IsZero() {
		duration := now.Sub(m.lastFrameTime)
		m.frameDurations = append(m.frameDurations, duration)
		if len(m.frameDurations) > m.maxFrames {
			m.frameDurations = m.frameDurations[1:]
		}
	}
	m.lastFrameTime = now
	m.frameCount.Add(1)
	m.mu.Unlock()
}

// BeginRender marks the start of a render operation.
func (m *Metrics) BeginRender() {
	m.mu.Lock()
	m.lastRenderStart = time.Now()
	m.mu.Unlock()
}

// EndRender marks the end of a render operation.
func (m *Metrics) EndRender() {
	m.mu.Lock()
	if !m.lastRenderStart.IsZero() {
		m.currentRenderTime = time.Since(m.lastRenderStart)
		m.totalRenderTime.Add(int64(m.currentRenderTime))
		m.renderCount.Add(1)
	}
	m.mu.Unlock()
}

// RecordInputLatency records the latency between input and response.
func (m *Metrics) RecordInputLatency(latency time.Duration) {
	m.mu.Lock()
	m.inputLatencies = append(m.inputLatencies, latency)
	if len(m.inputLatencies) > m.maxLatencies {
		m.inputLatencies = m.inputLatencies[1:]
	}
	m.mu.Unlock()
}

// SetMemoryUsage updates the tracked memory usage.
func (m *Metrics) SetMemoryUsage(bytes uint64) {
	m.mu.Lock()
	m.lastMemoryUsage = bytes
	m.mu.Unlock()
}

// FPS returns the current frames per second.
func (m *Metrics) FPS() float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if len(m.frameDurations) < 2 {
		return 60.0 // Default assumption
	}

	var total time.Duration
	for _, d := range m.frameDurations {
		total += d
	}
	avg := total / time.Duration(len(m.frameDurations))
	if avg == 0 {
		return 60.0
	}
	return float64(time.Second) / float64(avg)
}

// AvgFrameTime returns the average frame time.
func (m *Metrics) AvgFrameTime() time.Duration {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if len(m.frameDurations) == 0 {
		return 0
	}
	var total time.Duration
	for _, d := range m.frameDurations {
		total += d
	}
	return total / time.Duration(len(m.frameDurations))
}

// LastRenderTime returns the most recent render duration.
func (m *Metrics) LastRenderTime() time.Duration {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.currentRenderTime
}

// AvgRenderTime returns the average render time.
func (m *Metrics) AvgRenderTime() time.Duration {
	count := m.renderCount.Load()
	if count == 0 {
		return 0
	}
	return time.Duration(m.totalRenderTime.Load() / int64(count))
}

// AvgInputLatency returns the average input latency.
func (m *Metrics) AvgInputLatency() time.Duration {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if len(m.inputLatencies) == 0 {
		return 0
	}
	var total time.Duration
	for _, l := range m.inputLatencies {
		total += l
	}
	return total / time.Duration(len(m.inputLatencies))
}

// P99InputLatency returns the 99th percentile input latency.
func (m *Metrics) P99InputLatency() time.Duration {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if len(m.inputLatencies) == 0 {
		return 0
	}

	// Simple implementation: sort and take 99th percentile
	sorted := make([]time.Duration, len(m.inputLatencies))
	copy(sorted, m.inputLatencies)
	for i := 0; i < len(sorted)-1; i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[i] > sorted[j] {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}
	idx := int(float64(len(sorted)) * 0.99)
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

// MemoryUsage returns the last recorded memory usage in bytes.
func (m *Metrics) MemoryUsage() uint64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.lastMemoryUsage
}

// FrameCount returns the total number of frames rendered.
func (m *Metrics) FrameCount() uint64 {
	return m.frameCount.Load()
}

// Uptime returns the duration since metrics tracking started.
func (m *Metrics) Uptime() time.Duration {
	return time.Since(m.startTime)
}

// IsHealthy returns true if performance metrics are within acceptable bounds.
func (m *Metrics) IsHealthy() bool {
	fps := m.FPS()
	renderTime := m.LastRenderTime()
	mem := m.MemoryUsage()

	return fps >= 30 && // At least 30 FPS
		renderTime < 50*time.Millisecond && // Under 50ms render
		(mem == 0 || mem < 100*1024*1024) // Under 100MB (0 means not tracked)
}

// Summary returns a formatted summary of all metrics.
func (m *Metrics) Summary() string {
	fps := m.FPS()
	frameTime := m.AvgFrameTime()
	renderTime := m.AvgRenderTime()
	inputLatency := m.AvgInputLatency()
	mem := m.MemoryUsage()

	memStr := "N/A"
	if mem > 0 {
		if mem >= 1024*1024 {
			memStr = fmt.Sprintf("%.1fMB", float64(mem)/(1024*1024))
		} else {
			memStr = fmt.Sprintf("%.1fKB", float64(mem)/1024)
		}
	}

	return fmt.Sprintf(
		"FPS: %.1f | Frame: %v | Render: %v | Input: %v | Memory: %s",
		fps, frameTime.Round(time.Microsecond),
		renderTime.Round(time.Microsecond),
		inputLatency.Round(time.Microsecond),
		memStr,
	)
}

// Reset clears all collected metrics.
func (m *Metrics) Reset() {
	m.mu.Lock()
	m.frameDurations = m.frameDurations[:0]
	m.inputLatencies = m.inputLatencies[:0]
	m.lastFrameTime = time.Now()
	m.startTime = time.Now()
	m.mu.Unlock()
	m.frameCount.Store(0)
	m.renderCount.Store(0)
	m.totalRenderTime.Store(0)
}
