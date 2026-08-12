// Package performance provides TUI performance optimization primitives.
package performance

import (
	"strings"
	"sync"
)

// VirtualList provides efficient virtual scrolling for large lists.
type VirtualList[T any] struct {
	mu sync.RWMutex

	items        []T
	visibleStart int
	visibleCount int
	totalHeight  int
	itemHeight   int
	scrollOffset int
	overscan     int // Extra items to render above/below viewport
	renderCache  []string
	cacheValid   []bool
	renderItem   func(item T, index int, width int) string
	width        int

	// Performance optimization
	lastScrollPos int
	recyclePool   []int // Indices of recyclable rendered items
}

// VirtualListConfig configures a virtual list.
type VirtualListConfig[T any] struct {
	Items       []T
	ItemHeight  int
	TotalHeight int
	Overscan    int
	RenderFunc  func(item T, index int, width int) string
	Width       int
}

// NewVirtualList creates a new virtual scrolling list.
func NewVirtualList[T any](cfg VirtualListConfig[T]) *VirtualList[T] {
	overscan := cfg.Overscan
	if overscan <= 0 {
		overscan = 3 // Default overscan
	}

	itemHeight := cfg.ItemHeight
	if itemHeight <= 0 {
		itemHeight = 1
	}

	vl := &VirtualList[T]{
		items:       cfg.Items,
		itemHeight:  itemHeight,
		totalHeight: cfg.TotalHeight,
		overscan:    overscan,
		renderItem:  cfg.RenderFunc,
		width:       cfg.Width,
		renderCache: make([]string, len(cfg.Items)),
		cacheValid:  make([]bool, len(cfg.Items)),
		recyclePool: make([]int, 0, overscan*2),
	}
	vl.recalculate()
	return vl
}

// SetItems updates the list items and invalidates the cache.
func (v *VirtualList[T]) SetItems(items []T) {
	v.mu.Lock()
	defer v.mu.Unlock()

	v.items = items
	v.renderCache = make([]string, len(items))
	v.cacheValid = make([]bool, len(items))
	v.recalculate()
}

// SetSize updates viewport dimensions.
func (v *VirtualList[T]) SetSize(width, height int) {
	v.mu.Lock()
	defer v.mu.Unlock()

	if v.width != width {
		// Width changed - invalidate all cached renders
		v.width = width
		for i := range v.cacheValid {
			v.cacheValid[i] = false
		}
	}
	v.totalHeight = height
	v.recalculate()
}

// SetScroll updates the scroll position.
func (v *VirtualList[T]) SetScroll(offset int) {
	v.mu.Lock()
	defer v.mu.Unlock()

	if offset < 0 {
		offset = 0
	}
	maxScroll := len(v.items)*v.itemHeight - v.totalHeight
	if maxScroll < 0 {
		maxScroll = 0
	}
	if offset > maxScroll {
		offset = maxScroll
	}
	v.scrollOffset = offset
	v.recalculate()
}

// ScrollBy adjusts scroll position by delta.
func (v *VirtualList[T]) ScrollBy(delta int) {
	v.mu.Lock()
	v.scrollOffset += delta
	if v.scrollOffset < 0 {
		v.scrollOffset = 0
	}
	maxScroll := len(v.items)*v.itemHeight - v.totalHeight
	if maxScroll < 0 {
		maxScroll = 0
	}
	if v.scrollOffset > maxScroll {
		v.scrollOffset = maxScroll
	}
	v.recalculate()
	v.mu.Unlock()
}

// ScrollToItem scrolls to make an item visible.
func (v *VirtualList[T]) ScrollToItem(index int) {
	v.mu.Lock()
	defer v.mu.Unlock()

	if index < 0 || index >= len(v.items) {
		return
	}

	itemTop := index * v.itemHeight
	itemBottom := itemTop + v.itemHeight

	// Scroll up if item is above viewport
	if itemTop < v.scrollOffset {
		v.scrollOffset = itemTop
	}
	// Scroll down if item is below viewport
	if itemBottom > v.scrollOffset+v.totalHeight {
		v.scrollOffset = itemBottom - v.totalHeight
	}
	v.recalculate()
}

// recalculate updates visible range (must hold lock).
func (v *VirtualList[T]) recalculate() {
	if v.itemHeight <= 0 || len(v.items) == 0 {
		v.visibleStart = 0
		v.visibleCount = 0
		return
	}

	// Calculate visible range with overscan
	startIndex := v.scrollOffset / v.itemHeight
	startIndex -= v.overscan
	if startIndex < 0 {
		startIndex = 0
	}

	endIndex := (v.scrollOffset + v.totalHeight) / v.itemHeight
	endIndex += v.overscan + 1
	if endIndex > len(v.items) {
		endIndex = len(v.items)
	}

	v.visibleStart = startIndex
	v.visibleCount = endIndex - startIndex
}

// Render returns the rendered visible items.
func (v *VirtualList[T]) Render() string {
	v.mu.Lock()
	defer v.mu.Unlock()

	if len(v.items) == 0 || v.visibleCount == 0 {
		return ""
	}

	var sb strings.Builder
	endIndex := v.visibleStart + v.visibleCount
	if endIndex > len(v.items) {
		endIndex = len(v.items)
	}

	for i := v.visibleStart; i < endIndex; i++ {
		// Use cached render if valid
		if v.cacheValid[i] {
			sb.WriteString(v.renderCache[i])
		} else {
			// Render and cache
			rendered := v.renderItem(v.items[i], i, v.width)
			v.renderCache[i] = rendered
			v.cacheValid[i] = true
			sb.WriteString(rendered)
		}
		if i < endIndex-1 {
			sb.WriteByte('\n')
		}
	}

	return sb.String()
}

// InvalidateItem marks an item's cache as invalid.
func (v *VirtualList[T]) InvalidateItem(index int) {
	v.mu.Lock()
	if index >= 0 && index < len(v.cacheValid) {
		v.cacheValid[index] = false
	}
	v.mu.Unlock()
}

// InvalidateAll marks all items' caches as invalid.
func (v *VirtualList[T]) InvalidateAll() {
	v.mu.Lock()
	for i := range v.cacheValid {
		v.cacheValid[i] = false
	}
	v.mu.Unlock()
}

// InvalidateRange marks a range of items' caches as invalid.
func (v *VirtualList[T]) InvalidateRange(start, end int) {
	v.mu.Lock()
	if start < 0 {
		start = 0
	}
	if end > len(v.cacheValid) {
		end = len(v.cacheValid)
	}
	for i := start; i < end; i++ {
		v.cacheValid[i] = false
	}
	v.mu.Unlock()
}

// VisibleRange returns the range of visible item indices.
func (v *VirtualList[T]) VisibleRange() (start, end int) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	end = v.visibleStart + v.visibleCount
	if end > len(v.items) {
		end = len(v.items)
	}
	return v.visibleStart, end
}

// ItemCount returns the total number of items.
func (v *VirtualList[T]) ItemCount() int {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return len(v.items)
}

// ScrollOffset returns the current scroll offset.
func (v *VirtualList[T]) ScrollOffset() int {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.scrollOffset
}

// AtTop returns true if scrolled to the top.
func (v *VirtualList[T]) AtTop() bool {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.scrollOffset == 0
}

// AtBottom returns true if scrolled to the bottom.
func (v *VirtualList[T]) AtBottom() bool {
	v.mu.RLock()
	defer v.mu.RUnlock()
	maxScroll := len(v.items)*v.itemHeight - v.totalHeight
	if maxScroll < 0 {
		return true
	}
	return v.scrollOffset >= maxScroll
}

// TotalScrollableHeight returns the total scrollable content height.
func (v *VirtualList[T]) TotalScrollableHeight() int {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return len(v.items) * v.itemHeight
}

// ViewportHeight returns the viewport height.
func (v *VirtualList[T]) ViewportHeight() int {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.totalHeight
}

// CacheStats returns cache hit statistics.
func (v *VirtualList[T]) CacheStats() (valid, total int) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	total = len(v.cacheValid)
	for _, cv := range v.cacheValid {
		if cv {
			valid++
		}
	}
	return
}
