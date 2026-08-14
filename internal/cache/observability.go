package cache

import (
	"sync/atomic"
	"time"
)

// EventType identifies a structured cache observability event.
type EventType string

const (
	EventTypeEligibility EventType = "cache_eligibility"
	EventTypeHitMiss     EventType = "cache_access"
	EventTypeInvalidated EventType = "cache_invalidation"
	EventTypeBreak       EventType = "cache_break"
)

// CacheEvent is a structured cache metric/log event.
type CacheEvent struct {
	Timestamp time.Time      `json:"timestamp"`
	Type      EventType      `json:"type"`
	Category  string         `json:"category,omitempty"`
	Key       string         `json:"key,omitempty"`
	Eligible  *bool          `json:"eligible,omitempty"`
	Hit       *bool          `json:"hit,omitempty"`
	Reason    string         `json:"reason,omitempty"`
	Metrics   map[string]any `json:"metrics,omitempty"`
}

type invalidationState struct {
	Version int64
	Reason  string
	At      time.Time
}

func (c *PromptCache) emitEvent(event CacheEvent) {
	event.Timestamp = time.Now()

	c.eventMu.Lock()
	if c.maxEvents <= 0 {
		c.maxEvents = 256
	}
	c.events = append(c.events, event)
	if len(c.events) > c.maxEvents {
		c.events = c.events[len(c.events)-c.maxEvents:]
	}
	handler := c.eventHandler
	c.eventMu.Unlock()

	if handler != nil {
		handler(event)
	}
}

func (c *PromptCache) emitEligibility(category, key string, eligible bool, reason string) {
	c.emitEvent(CacheEvent{
		Type:     EventTypeEligibility,
		Category: category,
		Key:      key,
		Eligible: ptrBool(eligible),
		Reason:   reason,
	})
}

func (c *PromptCache) emitHitMiss(category, key string, hit bool, reason string) {
	c.emitEvent(CacheEvent{
		Type:     EventTypeHitMiss,
		Category: category,
		Key:      key,
		Hit:      ptrBool(hit),
		Reason:   reason,
	})
}

func (c *PromptCache) emitInvalidation(category, key, reason string, entries int, bytes int64) {
	version := atomic.AddInt64(&c.invalidationVersion, 1)
	c.invalidationMu.Lock()
	c.lastInvalidation = invalidationState{
		Version: version,
		Reason:  reason,
		At:      time.Now(),
	}
	c.invalidationMu.Unlock()

	c.emitEvent(CacheEvent{
		Type:     EventTypeInvalidated,
		Category: category,
		Key:      key,
		Reason:   reason,
		Metrics: map[string]any{
			"invalidation_version": version,
			"entries":              entries,
			"bytes":                bytes,
		},
	})
}

func (c *PromptCache) emitBreak(reason string, metrics map[string]any) {
	c.emitEvent(CacheEvent{
		Type:    EventTypeBreak,
		Reason:  reason,
		Metrics: metrics,
	})
}

// Events returns a snapshot of recorded cache events.
func (c *PromptCache) Events() []CacheEvent {
	c.eventMu.RLock()
	defer c.eventMu.RUnlock()
	events := make([]CacheEvent, len(c.events))
	copy(events, c.events)
	return events
}

// InvalidationState returns the latest invalidation details.
func (c *PromptCache) InvalidationState() (version int64, reason string, at time.Time) {
	version = atomic.LoadInt64(&c.invalidationVersion)
	c.invalidationMu.RLock()
	defer c.invalidationMu.RUnlock()
	return version, c.lastInvalidation.Reason, c.lastInvalidation.At
}

// WithMaxEvents sets the in-memory cache event buffer size.
func WithMaxEvents(size int) CacheOption {
	return func(c *PromptCache) {
		if size > 0 {
			c.maxEvents = size
		}
	}
}

// WithEventHandler sets a callback for every cache event.
func WithEventHandler(handler func(CacheEvent)) CacheOption {
	return func(c *PromptCache) {
		c.eventHandler = handler
	}
}

func ptrBool(v bool) *bool { return &v }
