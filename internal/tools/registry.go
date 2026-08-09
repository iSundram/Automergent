package tools

import (
	"fmt"
	"sync"
)

// Registry holds all registered tools.
type Registry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

// NewRegistry creates an empty registry.
func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]Tool)}
}

var global = NewRegistry()

// Register adds a tool to the global registry.
func Register(t Tool) { global.Register(t) }

// Get retrieves a tool by name from the global registry.
func Get(name string) (Tool, bool) { return global.Get(name) }

// All returns all tools in the global registry.
func All() []Tool { return global.All() }

func (r *Registry) Register(t Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[t.Name()] = t
}

func (r *Registry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tools[name]
	return t, ok
}

func (r *Registry) MustGet(name string) Tool {
	t, ok := r.Get(name)
	if !ok {
		panic(fmt.Sprintf("tools: %q not registered", name))
	}
	return t
}

func (r *Registry) All() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Tool, 0, len(r.tools))
	for _, t := range r.tools {
		out = append(out, t)
	}
	return out
}
