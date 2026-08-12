package testing

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/iSundram/Automergent/internal/tools"
)

// Benchmark provides benchmarking utilities for tools.
type Benchmark struct {
	tool tools.Tool
	ctx  context.Context
}

// NewBenchmark creates a new benchmark runner.
func NewBenchmark(tool tools.Tool) *Benchmark {
	return &Benchmark{
		tool: tool,
		ctx:  context.Background(),
	}
}

// WithContext sets the context for benchmarks.
func (b *Benchmark) WithContext(ctx context.Context) *Benchmark {
	b.ctx = ctx
	return b
}

// Run executes the standard benchmark.
func (b *Benchmark) Run(bm *testing.B, args map[string]any) {
	bm.ResetTimer()
	for i := 0; i < bm.N; i++ {
		_, _ = b.tool.Execute(b.ctx, args)
	}
}

// RunParallel executes a parallel benchmark.
func (b *Benchmark) RunParallel(bm *testing.B, args map[string]any) {
	bm.ResetTimer()
	bm.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = b.tool.Execute(b.ctx, args)
		}
	})
}

// Stats holds benchmark statistics.
type Stats struct {
	Iterations int
	TotalTime  time.Duration
	MinTime    time.Duration
	MaxTime    time.Duration
	AvgTime    time.Duration
	Throughput float64 // operations per second
	P50        time.Duration
	P95        time.Duration
	P99        time.Duration
}

// String returns a formatted string of stats.
func (s *Stats) String() string {
	return fmt.Sprintf(
		"Iterations: %d, Total: %v, Avg: %v, Min: %v, Max: %v, P50: %v, P95: %v, P99: %v, Throughput: %.2f ops/sec",
		s.Iterations, s.TotalTime, s.AvgTime, s.MinTime, s.MaxTime, s.P50, s.P95, s.P99, s.Throughput,
	)
}

// MeasureStats runs the tool multiple times and collects statistics.
func (b *Benchmark) MeasureStats(iterations int, args map[string]any) *Stats {
	if iterations <= 0 {
		iterations = 100
	}

	durations := make([]time.Duration, iterations)
	var total time.Duration

	for i := 0; i < iterations; i++ {
		start := time.Now()
		_, _ = b.tool.Execute(b.ctx, args)
		d := time.Since(start)
		durations[i] = d
		total += d
	}

	// Sort for percentiles
	sortDurations(durations)

	stats := &Stats{
		Iterations: iterations,
		TotalTime:  total,
		MinTime:    durations[0],
		MaxTime:    durations[iterations-1],
		AvgTime:    total / time.Duration(iterations),
		P50:        durations[iterations/2],
		P95:        durations[int(float64(iterations)*0.95)],
		P99:        durations[int(float64(iterations)*0.99)],
	}

	if total > 0 {
		stats.Throughput = float64(iterations) / total.Seconds()
	}

	return stats
}

// MeasureParallelStats measures performance under parallel load.
func (b *Benchmark) MeasureParallelStats(iterations, concurrency int, args map[string]any) *Stats {
	if iterations <= 0 {
		iterations = 100
	}
	if concurrency <= 0 {
		concurrency = 4
	}

	var wg sync.WaitGroup
	durations := make(chan time.Duration, iterations)

	iterPerWorker := iterations / concurrency
	start := time.Now()

	for w := 0; w < concurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < iterPerWorker; i++ {
				opStart := time.Now()
				_, _ = b.tool.Execute(b.ctx, args)
				durations <- time.Since(opStart)
			}
		}()
	}

	wg.Wait()
	totalTime := time.Since(start)
	close(durations)

	// Collect all durations
	var allDurations []time.Duration
	var sum time.Duration
	for d := range durations {
		allDurations = append(allDurations, d)
		sum += d
	}

	n := len(allDurations)
	sortDurations(allDurations)

	stats := &Stats{
		Iterations: n,
		TotalTime:  totalTime,
		MinTime:    allDurations[0],
		MaxTime:    allDurations[n-1],
		AvgTime:    sum / time.Duration(n),
		P50:        allDurations[n/2],
		P95:        allDurations[int(float64(n)*0.95)],
		P99:        allDurations[int(float64(n)*0.99)],
	}

	if totalTime > 0 {
		stats.Throughput = float64(n) / totalTime.Seconds()
	}

	return stats
}

func sortDurations(durations []time.Duration) {
	for i := 1; i < len(durations); i++ {
		for j := i; j > 0 && durations[j] < durations[j-1]; j-- {
			durations[j], durations[j-1] = durations[j-1], durations[j]
		}
	}
}

// CompareBenchmark compares two tool implementations.
type CompareBenchmark struct {
	baseline tools.Tool
	test     tools.Tool
	ctx      context.Context
}

// NewCompareBenchmark creates a comparison benchmark.
func NewCompareBenchmark(baseline, test tools.Tool) *CompareBenchmark {
	return &CompareBenchmark{
		baseline: baseline,
		test:     test,
		ctx:      context.Background(),
	}
}

// CompareResult holds comparison results.
type CompareResult struct {
	BaselineStats *Stats
	TestStats     *Stats
	SpeedupRatio  float64 // > 1 means test is faster
}

// Run executes the comparison benchmark.
func (cb *CompareBenchmark) Run(iterations int, args map[string]any) *CompareResult {
	baselineBench := NewBenchmark(cb.baseline).WithContext(cb.ctx)
	testBench := NewBenchmark(cb.test).WithContext(cb.ctx)

	baselineStats := baselineBench.MeasureStats(iterations, args)
	testStats := testBench.MeasureStats(iterations, args)

	speedup := 1.0
	if testStats.AvgTime > 0 {
		speedup = float64(baselineStats.AvgTime) / float64(testStats.AvgTime)
	}

	return &CompareResult{
		BaselineStats: baselineStats,
		TestStats:     testStats,
		SpeedupRatio:  speedup,
	}
}

// String returns a formatted comparison.
func (cr *CompareResult) String() string {
	comparison := "slower"
	if cr.SpeedupRatio > 1 {
		comparison = "faster"
	}
	return fmt.Sprintf(
		"Baseline: %v avg, Test: %v avg, Speedup: %.2fx (%s)",
		cr.BaselineStats.AvgTime, cr.TestStats.AvgTime, cr.SpeedupRatio, comparison,
	)
}

// MemoryBenchmark profiles memory allocation.
type MemoryBenchmark struct {
	tool tools.Tool
	ctx  context.Context
}

// NewMemoryBenchmark creates a memory benchmark.
func NewMemoryBenchmark(tool tools.Tool) *MemoryBenchmark {
	return &MemoryBenchmark{
		tool: tool,
		ctx:  context.Background(),
	}
}

// Run executes the memory benchmark.
func (mb *MemoryBenchmark) Run(bm *testing.B, args map[string]any) {
	bm.ReportAllocs()
	bm.ResetTimer()
	for i := 0; i < bm.N; i++ {
		_, _ = mb.tool.Execute(mb.ctx, args)
	}
}
