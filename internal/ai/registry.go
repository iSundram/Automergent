package ai

import (
	"fmt"
	"sync"
)

// Registry stores named providers.
type Registry struct {
	mu        sync.RWMutex
	providers map[string]Provider
}

var globalRegistry = &Registry{providers: make(map[string]Provider)}

// Register adds a provider under the given name.
func Register(name string, p Provider) {
	globalRegistry.Register(name, p)
}

// Get retrieves a provider by name.
func Get(name string) (Provider, bool) {
	return globalRegistry.Get(name)
}

// MustGet retrieves a provider by name or panics.
func MustGet(name string) Provider {
	p, ok := Get(name)
	if !ok {
		panic(fmt.Sprintf("ai: provider %q not registered", name))
	}
	return p
}

func (r *Registry) Register(name string, p Provider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[name] = p
}

func (r *Registry) Get(name string) (Provider, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.providers[name]
	return p, ok
}

// Names returns all registered provider names.
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.providers))
	for k := range r.providers {
		names = append(names, k)
	}
	return names
}

// Names returns all globally registered provider names.
func Names() []string {
	return globalRegistry.Names()
}
