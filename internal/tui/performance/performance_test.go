package performance

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestMetricsBasic(t *testing.T) {
	m := NewMetrics()

	// Test frame tracking
	for i := 0; i < 10; i++ {
		m.BeginFrame()
		time.Sleep(time.Millisecond)
	}

	fps := m.FPS()
	if fps <= 0 || fps > 1000 {
		t.Errorf("unexpected FPS: %f", fps)
	}

	if m.FrameCount() != 10 {
		t.Errorf("expected 10 frames, got %d", m.FrameCount())
	}
}

func TestMetricsRenderTime(t *testing.T) {
	m := NewMetrics()

	m.BeginRender()
	time.Sleep(5 * time.Millisecond)
	m.EndRender()

	renderTime := m.LastRenderTime()
	if renderTime < 5*time.Millisecond {
		t.Errorf("render time too short: %v", renderTime)
	}

	avg := m.AvgRenderTime()
	if avg != renderTime {
		t.Errorf("avg render time mismatch: got %v, want %v", avg, renderTime)
	}
}

func TestMetricsInputLatency(t *testing.T) {
	m := NewMetrics()

	latencies := []time.Duration{
		5 * time.Millisecond,
		10 * time.Millisecond,
		15 * time.Millisecond,
	}

	for _, l := range latencies {
		m.RecordInputLatency(l)
	}

	avg := m.AvgInputLatency()
	expected := 10 * time.Millisecond
	if avg != expected {
		t.Errorf("expected avg latency %v, got %v", expected, avg)
	}
}

func TestMetricsMemory(t *testing.T) {
	m := NewMetrics()

	m.SetMemoryUsage(50 * 1024 * 1024) // 50MB
	if m.MemoryUsage() != 50*1024*1024 {
		t.Errorf("memory usage not set correctly")
	}
}

func TestMetricsHealthy(t *testing.T) {
	m := NewMetrics()

	// Initialize some healthy metrics
	for i := 0; i < 10; i++ {
		m.BeginFrame()
		time.Sleep(time.Millisecond)
	}
	m.SetMemoryUsage(30 * 1024 * 1024) // 30MB

	if !m.IsHealthy() {
		t.Error("metrics should be healthy")
	}

	// Set unhealthy memory
	m.SetMemoryUsage(150 * 1024 * 1024) // 150MB
	if m.IsHealthy() {
		t.Error("metrics should be unhealthy with high memory")
	}
}

func TestMetricsSummary(t *testing.T) {
	m := NewMetrics()
	m.BeginFrame()
	m.SetMemoryUsage(10 * 1024 * 1024)

	summary := m.Summary()
	if summary == "" {
		t.Error("summary should not be empty")
	}
	if !strings.Contains(summary, "FPS") {
		t.Error("summary should contain FPS")
	}
}

func TestMetricsReset(t *testing.T) {
	m := NewMetrics()

	for i := 0; i < 5; i++ {
		m.BeginFrame()
	}

	m.Reset()

	if m.FrameCount() != 0 {
		t.Errorf("frame count should be 0 after reset, got %d", m.FrameCount())
	}
}

func TestContentDifferNoChange(t *testing.T) {
	d := NewContentDiffer()

	content := "line 1\nline 2\nline 3"
	result1 := d.Diff(content)
	if !result1.HasChanges {
		t.Error("first diff should show changes")
	}

	result2 := d.Diff(content)
	if result2.HasChanges {
		t.Error("second diff with same content should show no changes")
	}
}

func TestContentDifferChanges(t *testing.T) {
	d := NewContentDiffer()

	d.Diff("line 1\nline 2\nline 3")
	result := d.Diff("line 1\nmodified\nline 3")

	if !result.HasChanges {
		t.Error("should detect changes")
	}
	if len(result.ChangedLines) != 1 {
		t.Errorf("expected 1 changed line, got %d", len(result.ChangedLines))
	}
	if result.ChangedLines[0] != 1 {
		t.Errorf("expected line 1 changed, got %d", result.ChangedLines[0])
	}
}

func TestContentDifferAddedLines(t *testing.T) {
	d := NewContentDiffer()

	d.Diff("line 1\nline 2")
	result := d.Diff("line 1\nline 2\nline 3")

	if !result.HasChanges {
		t.Error("should detect changes")
	}
	if len(result.AddedLines) != 1 {
		t.Errorf("expected 1 added line, got %d", len(result.AddedLines))
	}
}

func TestContentDifferRemovedLines(t *testing.T) {
	d := NewContentDiffer()

	d.Diff("line 1\nline 2\nline 3")
	result := d.Diff("line 1\nline 2")

	if !result.HasChanges {
		t.Error("should detect changes")
	}
	if len(result.RemovedLines) != 1 {
		t.Errorf("expected 1 removed line, got %d", len(result.RemovedLines))
	}
}

func TestBatchDiffer(t *testing.T) {
	b := NewBatchDiffer()

	b.Register("header")
	b.Register("content")
	b.Register("footer")

	// Initial updates
	if !b.Update("header", "Header content") {
		t.Error("first header update should show changes")
	}
	if !b.Update("content", "Main content") {
		t.Error("first content update should show changes")
	}

	// No change update
	if b.Update("header", "Header content") {
		t.Error("same header content should not show changes")
	}

	// Check dirty components
	dirty := b.DirtyComponents()
	if len(dirty) == 0 {
		t.Error("should have dirty components")
	}

	// Clear dirty
	b.ClearDirty("header")
	if b.IsDirty("header") {
		t.Error("header should not be dirty after clear")
	}
}

func TestVirtualListBasic(t *testing.T) {
	items := make([]string, 100)
	for i := range items {
		items[i] = fmt.Sprintf("Item %d", i)
	}

	vl := NewVirtualList(VirtualListConfig[string]{
		Items:       items,
		ItemHeight:  1,
		TotalHeight: 10,
		Overscan:    2,
		Width:       80,
		RenderFunc: func(item string, index, width int) string {
			return item
		},
	})

	// Check initial state
	start, end := vl.VisibleRange()
	if start != 0 {
		t.Errorf("expected start 0, got %d", start)
	}
	// end should be around 10 + overscan
	if end < 10 || end > 15 {
		t.Errorf("unexpected visible end: %d", end)
	}

	// Test render
	rendered := vl.Render()
	if rendered == "" {
		t.Error("render should not be empty")
	}
}

func TestVirtualListScrolling(t *testing.T) {
	items := make([]string, 100)
	for i := range items {
		items[i] = fmt.Sprintf("Item %d", i)
	}

	vl := NewVirtualList(VirtualListConfig[string]{
		Items:       items,
		ItemHeight:  1,
		TotalHeight: 10,
		RenderFunc: func(item string, index, width int) string {
			return item
		},
	})

	// Scroll down
	vl.SetScroll(50)
	if !vl.AtTop() == false {
		// Should not be at top
	}

	start, _ := vl.VisibleRange()
	if start < 47 || start > 50 { // Accounting for overscan
		t.Errorf("scroll position incorrect, start: %d", start)
	}

	// Scroll to item
	vl.ScrollToItem(0)
	if !vl.AtTop() {
		t.Error("should be at top after scrolling to item 0")
	}
}

func TestVirtualListCache(t *testing.T) {
	renderCount := 0
	items := make([]string, 20)
	for i := range items {
		items[i] = fmt.Sprintf("Item %d", i)
	}

	vl := NewVirtualList(VirtualListConfig[string]{
		Items:       items,
		ItemHeight:  1,
		TotalHeight: 10,
		RenderFunc: func(item string, index, width int) string {
			renderCount++
			return item
		},
	})

	// First render
	vl.Render()
	initialCount := renderCount

	// Second render should use cache
	vl.Render()
	if renderCount != initialCount {
		t.Error("second render should use cache")
	}

	// Invalidate and re-render
	vl.InvalidateAll()
	vl.Render()
	if renderCount <= initialCount {
		t.Error("render count should increase after invalidation")
	}
}

func TestVirtualListInvalidateItem(t *testing.T) {
	items := []string{"a", "b", "c"}
	renderCounts := make([]int, 3)

	vl := NewVirtualList(VirtualListConfig[string]{
		Items:       items,
		ItemHeight:  1,
		TotalHeight: 3,
		RenderFunc: func(item string, index, width int) string {
			renderCounts[index]++
			return item
		},
	})

	// First render
	vl.Render()

	// Invalidate only item 1
	vl.InvalidateItem(1)
	vl.Render()

	if renderCounts[0] != 1 {
		t.Errorf("item 0 should be rendered once, got %d", renderCounts[0])
	}
	if renderCounts[1] != 2 {
		t.Errorf("item 1 should be rendered twice, got %d", renderCounts[1])
	}
	if renderCounts[2] != 1 {
		t.Errorf("item 2 should be rendered once, got %d", renderCounts[2])
	}
}

// Mock freeable component for testing
type mockComponent struct {
	frozen bool
}

func (m *mockComponent) Freeze()        { m.frozen = true }
func (m *mockComponent) Unfreeze()      { m.frozen = false }
func (m *mockComponent) IsFrozen() bool { return m.frozen }

func TestOffscreenManager(t *testing.T) {
	om := NewOffscreenManager()

	c1 := &mockComponent{}
	c2 := &mockComponent{}
	c3 := &mockComponent{}

	om.Register("comp1", c1)
	om.Register("comp2", c2)
	om.Register("comp3", c3)

	// All visible initially
	om.SetVisible("comp1", true)
	om.SetVisible("comp2", true)
	om.SetVisible("comp3", true)

	if c1.frozen || c2.frozen || c3.frozen {
		t.Error("all components should be unfrozen when visible")
	}

	// Hide comp2
	om.SetVisible("comp2", false)
	if !c2.frozen {
		t.Error("comp2 should be frozen when hidden")
	}

	// Make comp2 visible again
	om.SetVisible("comp2", true)
	if c2.frozen {
		t.Error("comp2 should be unfrozen when visible again")
	}
}

func TestOffscreenManagerBatchUpdate(t *testing.T) {
	om := NewOffscreenManager()

	components := make([]*mockComponent, 5)
	for i := range components {
		components[i] = &mockComponent{}
		om.Register(fmt.Sprintf("comp%d", i), components[i])
		om.SetVisible(fmt.Sprintf("comp%d", i), true)
	}

	// Update visibility: only 0, 1, 2 visible
	om.UpdateVisibility([]string{"comp0", "comp1", "comp2"})

	for i := 0; i < 3; i++ {
		if components[i].frozen {
			t.Errorf("comp%d should not be frozen", i)
		}
	}
	for i := 3; i < 5; i++ {
		if !components[i].frozen {
			t.Errorf("comp%d should be frozen", i)
		}
	}
}

func TestComponentPool(t *testing.T) {
	createCount := 0
	pool := NewComponentPool(5,
		func() string {
			createCount++
			return fmt.Sprintf("item%d", createCount)
		},
		func(s string) {
			// Reset function
		},
	)

	// Get should create new
	item1 := pool.Get()
	if createCount != 1 {
		t.Error("should have created one item")
	}

	// Put back
	pool.Put(item1)
	if pool.Size() != 1 {
		t.Error("pool should have 1 item")
	}

	// Get should reuse
	item2 := pool.Get()
	if item2 != item1 {
		t.Error("should have reused pooled item")
	}
	if createCount != 1 {
		t.Error("should not have created new item")
	}
}

func TestComponentPoolMaxSize(t *testing.T) {
	pool := NewComponentPool(2, func() int { return 0 }, nil)

	pool.Put(1)
	pool.Put(2)
	pool.Put(3) // Should be ignored

	if pool.Size() != 2 {
		t.Errorf("pool size should be limited to 2, got %d", pool.Size())
	}
}
