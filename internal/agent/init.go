package agent

import (
	"sync"

	"github.com/iSundram/Automergent/internal/agent/agentdef"
	"github.com/iSundram/Automergent/internal/agent/builtin"
	toolsAgent "github.com/iSundram/Automergent/internal/tools/agent"
)

var initOnce sync.Once

// Init initializes the agent framework:
// 1. Registers all built-in agents
// 2. Wires the registry to tools/agent type validation
// 3. Wires the registry to tools/agent model lookup
func Init() {
	initOnce.Do(func() {
		// Register built-in agents
		builtin.RegisterAll(GlobalRegistry())

		// Wire registry to tools/agent for type validation
		toolsAgent.SetAgentTypeValidator(func(typeName string) bool {
			_, ok := GlobalRegistry().Get(AgentType(typeName))
			return ok
		})

		// Wire registry to tools/agent for model lookup
		toolsAgent.SetAgentModelLookup(func(typeName string) string {
			if def, ok := GlobalRegistry().Get(AgentType(typeName)); ok {
				return def.Model
			}
			return ""
		})
	})
}

// InitWithCustomAgents initializes and also registers custom agents from definitions.
func InitWithCustomAgents(customDefs []*agentdef.AgentDefinition) {
	Init()

	for _, def := range customDefs {
		if def.Source == agentdef.SourceUser || def.Source == agentdef.SourceProject || def.Source == agentdef.SourcePlugin {
			GlobalRegistry().RegisterOverride(def)
		}
	}
}
