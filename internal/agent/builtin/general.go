package builtin

import (
	"github.com/iSundram/Automergent/internal/agent/agentdef"
	"github.com/iSundram/Automergent/internal/shared"
)

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
		// PhasePrompts intentionally unset: the shared per-phase files
		// (prompt/phases/*.txt) are the platform defaults and are richer than
		// the per-agent overrides that used to live here.
		
		PhaseTools: map[shared.AgentPhase][]string{
			shared.PhaseInit:    {"bash", "read", "glob", "grep", "task"},
			shared.PhasePlan:    {"read", "write", "bash", "task"},
			shared.PhaseBuild:   {"edit", "bash", "write", "read", "task", "glob", "grep"},
		},
		PhaseMaxSteps: map[shared.AgentPhase]int{
			shared.PhaseInit:    3,
			shared.PhasePlan:    5,
			shared.PhaseBuild:   20,
		},
		ToolPrompts: map[string]shared.ToolPromptConfig{
			"edit":  {PreExecution: "Make minimal, focused edits. Match exact indentation.", Rules: []string{"Use replaceAll only for renaming", "Preserve existing code style", "No comments unless asked"}},
			"bash":  {PreExecution: "Execute shell commands. Prefer non-interactive. Use absolute paths.", Rules: []string{"Never curl/wget | sh in one command", "Use && for chaining", "Check exit codes"}},
			"task":  {PreExecution: "Delegate to subagent. Provide clear description and context.", Rules: []string{"One task per subagent", "Include relevant file paths"}},
			"write": {PreExecution: "Create new files. Follow existing patterns.", Rules: []string{"Check if file exists first", "Match project structure"}}},
		BehavioralPrompts: []string{
			"Phase Discipline: Stay in current phase. Init=classify. Plan=design. Build=code+test+todo.",
			"Context Management: Read files before editing. Use grep/glob before read. Compact when >80%.",
			"No Hallucination: Never guess file contents. Use tools to verify. Don't make up APIs.",
			"Code Conventions: Follow existing patterns. Check imports, naming. Never assume libraries.",
			"Todo Management: In build phase, CREATE todo list, UPDATE on progress, COMPLETE on done.",
			"Test-Driven: In build phase, RUN tests after EVERY change. Lint + typecheck.",
		},
		ViolationPolicy: shared.ViolationPolicy{
			MaxWarnings:    2,
			BlockOnPersist: true,
			AllowOverride:  true,
		},
	}
}
