package agent

import "sync"

// grants holds the always-allow tool approval scopes ("name=bash;…;cmd=git …")
// for one agent tree. It exists as a shared, separately-locked object rather
// than a per-Agent map so a subagent and its parent see one grant set: a
// prefix granted while answering a subagent's confirmation covers the main
// agent and every sibling, which is the only sane reading of "always allow".
//
// The lock is deliberately not the Agent's own mutex: grants.Add runs the
// persistence hook (tryPersist takes the agent RLock), so nesting the two
// would deadlock.
type grants struct {
	mu     sync.RWMutex
	scopes map[string]bool
	// persist, when non-nil, records a newly granted scope in the owning
	// session. Children share the parent's grants object, so grants made
	// inside a subagent persist to the real session, not the child's
	// throwaway one.
	persist func(scope string)
}

func newGrants(persist func(string)) *grants {
	return &grants{scopes: make(map[string]bool), persist: persist}
}

// newGrantsWithScopes is the test/fixture constructor: pre-granted scopes, no
// persistence.
func newGrantsWithScopes(scopes ...string) *grants {
	g := newGrants(nil)
	for _, s := range scopes {
		g.scopes[s] = true
	}
	return g
}

// Has reports whether the exact scope has been granted.
func (g *grants) Has(scope string) bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.scopes[scope]
}

// Add grants the scope and persists it if it is new. Returns whether the
// scope was newly added.
func (g *grants) Add(scope string) bool {
	g.mu.Lock()
	if g.scopes[scope] {
		g.mu.Unlock()
		return false
	}
	g.scopes[scope] = true
	persist := g.persist
	g.mu.Unlock()
	if persist != nil {
		persist(scope)
	}
	return true
}

// Delete revokes the scope.
func (g *grants) Delete(scope string) {
	g.mu.Lock()
	delete(g.scopes, scope)
	g.mu.Unlock()
}

// Reset replaces the whole grant set (used when a session is swapped in and
// approvals are re-seeded from it). Persistence is the caller's business —
// the incoming scopes already live in that session.
func (g *grants) Reset(scopes []string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.scopes = make(map[string]bool, len(scopes))
	for _, s := range scopes {
		g.scopes[s] = true
	}
}

// Each iterates the granted scopes. Used by prefix matching, which cannot
// answer with a single lookup: it has to inspect every shell grant.
func (g *grants) Each(fn func(scope string)) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	for scope := range g.scopes {
		fn(scope)
	}
}

// Len reports how many scopes are granted (used by the approvals list UI).
func (g *grants) Len() int {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return len(g.scopes)
}
