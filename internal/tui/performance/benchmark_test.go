package performance

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func BenchmarkMetricsBeginFrame(b *testing.B) {
	m := NewMetrics()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.BeginFrame()
	}
}

func BenchmarkMetricsRenderCycle(b *testing.B) {
	m := NewMetrics()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.BeginRender()
		m.EndRender()
	}
}

func BenchmarkMetricsRecordInputLatency(b *testing.B) {
	m := NewMetrics()
	latency := 5 * time.Millisecond
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.RecordInputLatency(latency)
	}
}

func BenchmarkContentDifferSmallNoChange(b *testing.B) {
	d := NewContentDiffer()
	content := "line1\nline2\nline3\nline4\nline5"
	d.Diff(content)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d.Diff(content)
	}
}

func BenchmarkContentDifferSmallChange(b *testing.B) {
	d := NewContentDiffer()
	contents := []string{
		"line1\nline2\nline3\nline4\nline5",
		"line1\nmodified\nline3\nline4\nline5",
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d.Diff(contents[i%2])
	}
}

func BenchmarkContentDifferLargeNoChange(b *testing.B) {
	d := NewContentDiffer()
	var sb strings.Builder
	for i := 0; i < 1000; i++ {
		sb.WriteString(fmt.Sprintf("This is line %d with some content\n", i))
	}
	content := sb.String()
	d.Diff(content)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d.Diff(content)
	}
}

func BenchmarkContentDifferLargeChange(b *testing.B) {
	d := NewContentDiffer()
	var sb1, sb2 strings.Builder
	for i := 0; i < 1000; i++ {
		sb1.WriteString(fmt.Sprintf("This is line %d with some content\n", i))
		if i == 500 {
			sb2.WriteString("This is a modified line\n")
		} else {
			sb2.WriteString(fmt.Sprintf("This is line %d with some content\n", i))
		}
	}
	contents := []string{sb1.String(), sb2.String()}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d.Diff(contents[i%2])
	}
}

func BenchmarkVirtualListRender10Items(b *testing.B) {
	benchmarkVirtualListRender(b, 10, 10)
}

func BenchmarkVirtualListRender100Items(b *testing.B) {
	benchmarkVirtualListRender(b, 100, 20)
}

func BenchmarkVirtualListRender1000Items(b *testing.B) {
	benchmarkVirtualListRender(b, 1000, 20)
}

func BenchmarkVirtualListRender10000Items(b *testing.B) {
	benchmarkVirtualListRender(b, 10000, 20)
}

func benchmarkVirtualListRender(b *testing.B, itemCount, viewportHeight int) {
	items := make([]string, itemCount)
	for i := range items {
		items[i] = fmt.Sprintf("Item %d with some text content", i)
	}

	vl := NewVirtualList(VirtualListConfig[string]{
		Items:       items,
		ItemHeight:  1,
		TotalHeight: viewportHeight,
		Width:       80,
		RenderFunc: func(item string, index, width int) string {
			return item
		},
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		vl.Render()
	}
}

func BenchmarkVirtualListScrollAndRender(b *testing.B) {
	items := make([]string, 1000)
	for i := range items {
		items[i] = fmt.Sprintf("Item %d with some text content", i)
	}

	vl := NewVirtualList(VirtualListConfig[string]{
		Items:       items,
		ItemHeight:  1,
		TotalHeight: 20,
		Width:       80,
		RenderFunc: func(item string, index, width int) string {
			return item
		},
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		vl.SetScroll(i % 980)
		vl.Render()
	}
}

func BenchmarkVirtualListCachedRender(b *testing.B) {
	items := make([]string, 1000)
	for i := range items {
		items[i] = fmt.Sprintf("Item %d", i)
	}

	vl := NewVirtualList(VirtualListConfig[string]{
		Items:       items,
		ItemHeight:  1,
		TotalHeight: 20,
		Width:       80,
		RenderFunc: func(item string, index, width int) string {
			// Simulate expensive render
			return strings.Repeat(item, 10)
		},
	})

	// Prime cache
	vl.Render()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		vl.Render()
	}
}

func BenchmarkBatchDifferUpdate(b *testing.B) {
	bd := NewBatchDiffer()
	components := []string{"header", "content", "sidebar", "footer", "statusbar"}
	for _, c := range components {
		bd.Register(c)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bd.Update(components[i%len(components)], fmt.Sprintf("content %d", i))
	}
}

func BenchmarkOffscreenManagerVisibility(b *testing.B) {
	om := NewOffscreenManager()
	components := make([]*mockComponent, 100)
	for i := range components {
		components[i] = &mockComponent{}
		om.Register(fmt.Sprintf("comp%d", i), components[i])
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		om.SetVisible(fmt.Sprintf("comp%d", i%100), i%2 == 0)
	}
}

func BenchmarkOffscreenManagerBatchVisibility(b *testing.B) {
	om := NewOffscreenManager()
	components := make([]*mockComponent, 100)
	for i := range components {
		components[i] = &mockComponent{}
		om.Register(fmt.Sprintf("comp%d", i), components[i])
	}

	visible1 := make([]string, 50)
	visible2 := make([]string, 50)
	for i := 0; i < 50; i++ {
		visible1[i] = fmt.Sprintf("comp%d", i)
		visible2[i] = fmt.Sprintf("comp%d", i+50)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if i%2 == 0 {
			om.UpdateVisibility(visible1)
		} else {
			om.UpdateVisibility(visible2)
		}
	}
}

func BenchmarkComponentPool(b *testing.B) {
	pool := NewComponentPool(100, func() []byte {
		return make([]byte, 1024)
	}, func(buf []byte) {
		// Reset
		for i := range buf {
			buf[i] = 0
		}
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		item := pool.Get()
		pool.Put(item)
	}
}

// Benchmark that simulates a full render cycle
func BenchmarkFullRenderCycle(b *testing.B) {
	m := NewMetrics()
	d := NewContentDiffer()

	items := make([]string, 100)
	for i := range items {
		items[i] = fmt.Sprintf("Item %d with content", i)
	}

	vl := NewVirtualList(VirtualListConfig[string]{
		Items:       items,
		ItemHeight:  1,
		TotalHeight: 20,
		Width:       80,
		RenderFunc: func(item string, index, width int) string {
			return item
		},
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.BeginFrame()
		m.BeginRender()

		rendered := vl.Render()
		d.Diff(rendered)

		m.EndRender()
	}
}

// Memory allocation benchmarks
func BenchmarkMetricsAllocs(b *testing.B) {
	m := NewMetrics()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.BeginFrame()
		m.BeginRender()
		m.EndRender()
		m.RecordInputLatency(time.Millisecond)
	}
}

func BenchmarkContentDifferAllocs(b *testing.B) {
	d := NewContentDiffer()
	content := "line1\nline2\nline3"
	d.Diff(content)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d.Diff(content)
	}
}

func BenchmarkVirtualListRenderAllocs(b *testing.B) {
	items := make([]string, 100)
	for i := range items {
		items[i] = fmt.Sprintf("Item %d", i)
	}

	vl := NewVirtualList(VirtualListConfig[string]{
		Items:       items,
		ItemHeight:  1,
		TotalHeight: 20,
		Width:       80,
		RenderFunc: func(item string, index, width int) string {
			return item
		},
	})

	// Prime cache
	vl.Render()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		vl.Render()
	}
}
