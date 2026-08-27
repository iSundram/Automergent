package agent

import (
	"fmt"
	"strings"
	"sync"

	"github.com/iSundram/Automergent/internal/agent/agentdef"
)

// Re-export agentdef types for convenience
type AgentType = agentdef.AgentType
type AgentDefinition = agentdef.AgentDefinition
type AgentSource = agentdef.AgentSource
type AgentEffort = agentdef.AgentEffort
type MemoryScope = agentdef.MemoryScope
type AgentConfig = agentdef.AgentConfig
type AgentStatus = agentdef.AgentStatus
type AgentResult = agentdef.AgentResult
type AgentFilter = agentdef.AgentFilter

const (
	AgentTypeGeneral     = agentdef.AgentTypeGeneral
	AgentTypeExplore     = agentdef.AgentTypeExplore
	AgentTypeReview      = agentdef.AgentTypeReview
	AgentTypeContexter   = agentdef.AgentTypeContexter
	AgentTypeCoordinator = agentdef.AgentTypeCoordinator
	AgentTypeCustom      = agentdef.AgentTypeCustom
)

// Registry manages all registered agent definitions.
type Registry struct {
	mu      sync.RWMutex
	agents  map[AgentType]*AgentDefinition
	byName  map[string]*AgentType
	ordered []AgentType
}

// NewRegistry creates an empty agent registry.
func NewRegistry() *Registry {
	return &Registry{
		agents: make(map[AgentType]*AgentDefinition),
		byName: make(map[string]*AgentType),
	}
}

// globalRegistry is the default registry populated with built-in agents.
var globalRegistry *Registry

func init() {
	globalRegistry = NewRegistry()
}

// GlobalRegistry returns the global agent registry.
func GlobalRegistry() *Registry {
	return globalRegistry
}

// Register adds an agent definition to the registry.
func (r *Registry) Register(def *AgentDefinition) error {
	if def == nil {
		return fmt.Errorf("agent definition is nil")
	}
	if def.Name == "" {
		return fmt.Errorf("agent name is required")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	agentType := AgentType(def.Name)

	if existing, ok := r.agents[agentType]; ok {
		return fmt.Errorf("agent type %q already registered (from %s)", agentType, existing.Source)
	}

	if existingType, ok := r.byName[def.Name]; ok {
		return fmt.Errorf("agent name %q already registered as type %q", def.Name, *existingType)
	}

	r.agents[agentType] = def
	r.byName[def.Name] = &agentType
	r.ordered = append(r.ordered, agentType)
	return nil
}

// RegisterOverride adds or replaces an agent definition.
func (r *Registry) RegisterOverride(def *AgentDefinition) {
	if def == nil || def.Name == "" {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	agentType := AgentType(def.Name)

	if existing, ok := r.agents[agentType]; ok {
		delete(r.byName, existing.Name)
	} else {
		r.ordered = append(r.ordered, agentType)
	}

	r.agents[agentType] = def
	r.byName[def.Name] = &agentType
}

// Get retrieves an agent definition by type.
func (r *Registry) Get(agentType AgentType) (*AgentDefinition, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	def, ok := r.agents[agentType]
	return def, ok
}

// GetByName retrieves an agent definition by name (case-insensitive).
func (r *Registry) GetByName(name string) (*AgentDefinition, AgentType, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	lower := strings.ToLower(name)
	for n, t := range r.byName {
		if strings.ToLower(n) == lower {
			return r.agents[*t], *t, true
		}
	}
	return nil, "", false
}

// List returns all registered agent definitions in insertion order.
func (r *Registry) List() []*AgentDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*AgentDefinition, 0, len(r.ordered))
	for _, t := range r.ordered {
		if def, ok := r.agents[t]; ok {
			result = append(result, def)
		}
	}
	return result
}

// ListBySource returns agent definitions filtered by source.
func (r *Registry) ListBySource(source AgentSource) []*AgentDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*AgentDefinition, 0)
	for _, t := range r.ordered {
		if def, ok := r.agents[t]; ok && def.Source == source {
			result = append(result, def)
		}
	}
	return result
}

// Remove removes an agent definition by type.
func (r *Registry) Remove(agentType AgentType) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	def, ok := r.agents[agentType]
	if !ok {
		return false
	}

	delete(r.agents, agentType)
	delete(r.byName, def.Name)

	for i, t := range r.ordered {
		if t == agentType {
			r.ordered = append(r.ordered[:i], r.ordered[i+1:]...)
			break
		}
	}
	return true
}

// Len returns the number of registered agents.
func (r *Registry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.agents)
}

// ResolveAgentType resolves a string to an AgentType.
func ResolveAgentType(input string) (AgentType, bool) {
	lower := strings.ToLower(strings.TrimSpace(input))

	switch AgentType(lower) {
	case AgentTypeGeneral, AgentTypeExplore, AgentTypeReview,
		AgentTypeContexter, AgentTypeCoordinator:
		return AgentType(lower), true
	}

	switch lower {
	case "general", "gen", "default":
		return AgentTypeGeneral, true
	case "explore", "search", "find", "research":
		return AgentTypeExplore, true
	case "review", "audit", "check":
		return AgentTypeReview, true
	case "context", "ctx", "compact":
		return AgentTypeContexter, true
	case "orchestrate", "orch", "coord", "swarm":
		return AgentTypeCoordinator, true
	}

	if def, t, ok := globalRegistry.GetByName(lower); ok && def != nil {
		return t, true
	}

	return "", false
}
