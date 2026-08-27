package agent

import (
	"context"
	"sync"
)

// childCancelEntry tracks a child agent's cancellation function.
type childCancelEntry struct {
	cancel  context.CancelFunc
	agentID string
}

// ContextIsolation provides parent-child agent context isolation.
// It manages abort chains so cancelling a parent cascades to all children,
// and provides state cloning for isolated execution.
type ContextIsolation struct {
	mu       sync.RWMutex
	children map[string]*childCancelEntry // key: cancelKey
	parentID string
}

// NewContextIsolation creates a new context isolation manager.
func NewContextIsolation(parentID string) *ContextIsolation {
	return &ContextIsolation{
		children: make(map[string]*childCancelEntry),
		parentID: parentID,
	}
}

// registerChildCancel registers a child's cancel function.
func (a *Agent) registerChildCancel(key string, cancel context.CancelFunc) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.childCancels == nil {
		a.childCancels = make(map[string]context.CancelFunc)
	}
	a.childCancels[key] = cancel
}

// unregisterChildCancel removes a child's cancel function.
func (a *Agent) unregisterChildCancel(key string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.childCancels != nil {
		delete(a.childCancels, key)
	}
}

// cancelAllChildren cancels all registered child agents.
func (a *Agent) cancelAllChildren() {
	a.mu.RLock()
	cancels := make([]context.CancelFunc, 0, len(a.childCancels))
	for _, cancel := range a.childCancels {
		cancels = append(cancels, cancel)
	}
	a.mu.RUnlock()

	for _, cancel := range cancels {
		cancel()
	}
}

// ChildCount returns the number of active child agents.
func (a *Agent) ChildCount() int {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return len(a.childCancels)
}

// IsolateFileCache creates a snapshot of the current file cache state.
// The child agent gets this snapshot and any modifications are invisible
// to the parent until explicitly merged.
type FileCacheSnapshot struct {
	Entries map[string]*CachedFileEntry
}

// CachedFileEntry represents a cached file entry.
type CachedFileEntry struct {
	Path    string
	Content string
	ModTime int64
	Size    int64
}

// CloneFileCache creates a deep copy of the file cache.
func CloneFileCache(src map[string]*CachedFileEntry) map[string]*CachedFileEntry {
	if src == nil {
		return nil
	}
	dst := make(map[string]*CachedFileEntry, len(src))
	for k, v := range src {
		if v == nil {
			continue
		}
		cp := *v
		dst[k] = &cp
	}
	return dst
}

// ToolProfileSnapshot captures the tool access mask at spawn time.
type ToolProfileSnapshot struct {
	Allowed map[string]bool
}

// CloneToolProfile creates a copy of the tool profile.
func CloneToolProfile(src map[string]bool) map[string]bool {
	if src == nil {
		return nil
	}
	dst := make(map[string]bool, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

// FilterToolsForAgent restricts the tool set based on agent definition.
// If def.Tools is nil, all tools are allowed.
// If def.Tools is set, only those tools are available.
func FilterToolsForAgent(allTools map[string]bool, agentTools []string) map[string]bool {
	if agentTools == nil {
		return allTools
	}

	filtered := make(map[string]bool, len(agentTools))
	for _, t := range agentTools {
		if _, ok := allTools[t]; ok {
			filtered[t] = true
		} else {
			// Tool doesn't exist in parent's set, but agent requested it
			// Include it anyway - the tool registry will handle missing tools
			filtered[t] = true
		}
	}
	return filtered
}

// ChildAgentState holds the isolated state for a child agent.
type ChildAgentState struct {
	ParentID      string
	ChildID       string
	AgentType     string
	ToolProfile   map[string]bool
	FileCache     map[string]*CachedFileEntry
	AbortChain    context.CancelFunc
	Depth         int
	IsIsolated    bool
}

// CloneSessionMetadata creates a copy of session metadata for the child.
func CloneSessionMetadata(src map[string]string) map[string]string {
	if src == nil {
		return nil
	}
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}
