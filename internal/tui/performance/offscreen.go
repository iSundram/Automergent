package performance

import (
	"sync"
)

// ComponentState tracks the state of an off-screen component.
type ComponentState int

const (
	StateActive ComponentState = iota
	StateFrozen
	StateRecycled
)

// FreezeableComponent interface for components that can be frozen.
type FreezeableComponent interface {
	Freeze()
	Unfreeze()
	IsFrozen() bool
}

// OffscreenManager manages off-screen component optimization.
type OffscreenManager struct {
	mu sync.RWMutex

	components    map[string]FreezeableComponent
	states        map[string]ComponentState
	lastRendered  map[string]string
	visibleSet    map[string]bool
	recycleQueue  []string
	maxRecycled   int
	frozenCount   int
	recycledCount int
}

// NewOffscreenManager creates a new offscreen manager.
func NewOffscreenManager() *OffscreenManager {
	return &OffscreenManager{
		components:   make(map[string]FreezeableComponent),
		states:       make(map[string]ComponentState),
		lastRendered: make(map[string]string),
		visibleSet:   make(map[string]bool),
		recycleQueue: make([]string, 0, 16),
		maxRecycled:  10,
	}
}

// Register adds a component to be managed.
func (m *OffscreenManager) Register(id string, component FreezeableComponent) {
	m.mu.Lock()
	m.components[id] = component
	m.states[id] = StateActive
	m.mu.Unlock()
}

// Unregister removes a component from management.
func (m *OffscreenManager) Unregister(id string) {
	m.mu.Lock()
	delete(m.components, id)
	delete(m.states, id)
	delete(m.lastRendered, id)
	delete(m.visibleSet, id)
	m.mu.Unlock()
}

// SetVisible marks a component as visible or hidden.
func (m *OffscreenManager) SetVisible(id string, visible bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	wasVisible := m.visibleSet[id]
	m.visibleSet[id] = visible

	component, exists := m.components[id]
	if !exists {
		return
	}

	if visible && !wasVisible {
		// Coming into view - unfreeze
		if m.states[id] == StateFrozen {
			component.Unfreeze()
			m.states[id] = StateActive
			m.frozenCount--
		}
	} else if !visible && wasVisible {
		// Going out of view - freeze
		if m.states[id] == StateActive {
			component.Freeze()
			m.states[id] = StateFrozen
			m.frozenCount++
		}
	}
}

// UpdateVisibility batch-updates visibility for multiple components.
func (m *OffscreenManager) UpdateVisibility(visibleIDs []string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Build new visible set
	newVisible := make(map[string]bool, len(visibleIDs))
	for _, id := range visibleIDs {
		newVisible[id] = true
	}

	// Freeze newly hidden components
	for id, wasVisible := range m.visibleSet {
		if wasVisible && !newVisible[id] {
			if component, exists := m.components[id]; exists {
				if m.states[id] == StateActive {
					component.Freeze()
					m.states[id] = StateFrozen
					m.frozenCount++
				}
			}
		}
	}

	// Unfreeze newly visible components
	for _, id := range visibleIDs {
		if !m.visibleSet[id] {
			if component, exists := m.components[id]; exists {
				if m.states[id] == StateFrozen {
					component.Unfreeze()
					m.states[id] = StateActive
					m.frozenCount--
				}
			}
		}
	}

	m.visibleSet = newVisible
}

// CacheRendered stores the last rendered output for a component.
func (m *OffscreenManager) CacheRendered(id, rendered string) {
	m.mu.Lock()
	m.lastRendered[id] = rendered
	m.mu.Unlock()
}

// GetCachedRendered returns the cached rendered output if available.
func (m *OffscreenManager) GetCachedRendered(id string) (string, bool) {
	m.mu.RLock()
	rendered, ok := m.lastRendered[id]
	m.mu.RUnlock()
	return rendered, ok
}

// IsVisible returns true if a component is currently visible.
func (m *OffscreenManager) IsVisible(id string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.visibleSet[id]
}

// IsFrozen returns true if a component is frozen.
func (m *OffscreenManager) IsFrozen(id string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.states[id] == StateFrozen
}

// GetState returns the current state of a component.
func (m *OffscreenManager) GetState(id string) ComponentState {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.states[id]
}

// FreezeAll freezes all registered components.
func (m *OffscreenManager) FreezeAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for id, component := range m.components {
		if m.states[id] == StateActive {
			component.Freeze()
			m.states[id] = StateFrozen
			m.frozenCount++
		}
	}
}

// UnfreezeAll unfreezes all registered components.
func (m *OffscreenManager) UnfreezeAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for id, component := range m.components {
		if m.states[id] == StateFrozen {
			component.Unfreeze()
			m.states[id] = StateActive
			m.frozenCount--
		}
	}
}

// Stats returns statistics about managed components.
func (m *OffscreenManager) Stats() (total, frozen, recycled, visible int) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	total = len(m.components)
	frozen = m.frozenCount
	recycled = m.recycledCount
	for _, v := range m.visibleSet {
		if v {
			visible++
		}
	}
	return
}

// ComponentPool provides object pooling for component reuse.
type ComponentPool[T any] struct {
	mu        sync.Mutex
	pool      []T
	maxSize   int
	newFunc   func() T
	resetFunc func(T)
}

// NewComponentPool creates a new component pool.
func NewComponentPool[T any](maxSize int, newFunc func() T, resetFunc func(T)) *ComponentPool[T] {
	return &ComponentPool[T]{
		pool:      make([]T, 0, maxSize),
		maxSize:   maxSize,
		newFunc:   newFunc,
		resetFunc: resetFunc,
	}
}

// Get retrieves a component from the pool or creates a new one.
func (p *ComponentPool[T]) Get() T {
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.pool) > 0 {
		item := p.pool[len(p.pool)-1]
		p.pool = p.pool[:len(p.pool)-1]
		return item
	}
	return p.newFunc()
}

// Put returns a component to the pool.
func (p *ComponentPool[T]) Put(item T) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.pool) < p.maxSize {
		if p.resetFunc != nil {
			p.resetFunc(item)
		}
		p.pool = append(p.pool, item)
	}
}

// Size returns the current pool size.
func (p *ComponentPool[T]) Size() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.pool)
}

// Clear empties the pool.
func (p *ComponentPool[T]) Clear() {
	p.mu.Lock()
	p.pool = p.pool[:0]
	p.mu.Unlock()
}
