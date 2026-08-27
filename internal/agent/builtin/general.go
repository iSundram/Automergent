package builtin

import "github.com/iSundram/Automergent/internal/agent/agentdef"

// GeneralAgent returns the general-purpose agent definition.
func GeneralAgent() *agentdef.AgentDefinition {
	return &agentdef.AgentDefinition{
		Name:        "general-purpose",
		Description: "Full-capability agent for complex tasks requiring all tools",
		WhenToUse:   "Use for any task not matching a specialized agent. Default agent for coding, implementation, debugging, and multi-step tasks.",
		SystemPrompt: `You are a general-purpose coding assistant with full access to all tools.

Your capabilities:
- Read, write, and edit files
- Execute shell commands
- Search codebases with grep and glob
- Fetch web content
- Spawn sub-agents for parallel work

Guidelines:
- Follow existing code style and patterns in the project
- Make minimal, targeted changes unless explicitly asked for refactoring
- Verify changes compile/build before reporting completion
- Use parallel tool calls when operations are independent
- Prefer editing existing files over creating new ones
- Never commit secrets, keys, or credentials
- Run lint/typecheck commands after making changes`,
		Model:       "",
		Tools:       nil, // nil = all tools
		Color:       "green",
		Effort:      agentdef.EffortHigh,
		Source:      agentdef.SourceBuiltin,
		MemoryScope: agentdef.MemoryScopeProject,
		Timeout:     0,
	}
}
