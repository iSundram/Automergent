package builtin

import "github.com/iSundram/Automergent/internal/agent/agentdef"

// RegisterAll registers all built-in agent definitions with the given registry.
func RegisterAll(r Registry) {
	agents := []*agentdef.AgentDefinition{
		GeneralAgent(),
		ExploreAgent(),
		ReviewAgent(),
		ContexterAgent(),
		CoordinatorAgent(),
	}

	for _, def := range agents {
		_ = r.Register(def)
	}
}

// DefaultDefinitions returns all built-in agent definitions without registering.
func DefaultDefinitions() []*agentdef.AgentDefinition {
	return []*agentdef.AgentDefinition{
		GeneralAgent(),
		ExploreAgent(),
		ReviewAgent(),
		ContexterAgent(),
		CoordinatorAgent(),
	}
}

// Registry is the interface that agent registries must implement.
type Registry interface {
	Register(def *agentdef.AgentDefinition) error
}
