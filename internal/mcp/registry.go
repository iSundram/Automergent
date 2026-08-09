package mcp

import (
	"sync"
)

// Registry manages MCP clients by name.
type Registry struct {
	mu      sync.RWMutex
	clients map[string]*Client
}

// NewRegistry creates an empty MCP registry.
func NewRegistry() *Registry {
	return &Registry{clients: make(map[string]*Client)}
}

// Register adds a client under the given name.
func (r *Registry) Register(name string, c *Client) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.clients[name] = c
}

// Get returns a client by name.
func (r *Registry) Get(name string) (*Client, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.clients[name]
	return c, ok
}

// Names returns all registered client names.
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.clients))
	for k := range r.clients {
		names = append(names, k)
	}
	return names
}
