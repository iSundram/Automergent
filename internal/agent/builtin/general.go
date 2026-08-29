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
		PhasePrompts: map[shared.AgentPhase]string{
			shared.PhaseInit: `
## Phase: INIT - Request Classification
Classify the user's request:
- If direct Q&A ("what does X do", "hello"): Answer directly, no tools needed
- If exploration needed (find files, search code): Transition to explore phase
- If plan needed (design, architecture): Transition to plan phase
- If clear implementation: Transition to build phase
- If violation detected: Call violation_detected tool immediately
- If ambiguous: Ask clarifying questions

Be concise. Use minimal tools. Prioritize the task.`,
			shared.PhasePlan: `
## Phase: PLAN - Design & Planning
You are in planning mode. Your job is to:
1. Review exploration results
2. Create a detailed implementation plan
3. Identify specific files to modify
4. Ask clarifying questions if requirements are ambiguous
5. Define task dependencies and order
6. Create todo list for implementation
7. When plan is complete, transition to build phase

Be structured and analytical.`,
			shared.PhaseBuild: `
## Phase: BUILD - Implementation + Testing + Todo Management
You are in build mode. Your job is to:
1. Implement the plan with minimal, focused changes
2. Follow existing code style and patterns
3. MANAGE TODO LIST: Create, update, complete todos for each task
4. Run tests/lint/typecheck AFTER each change
5. If bugs found, transition to explore phase
6. When all todos complete and tests pass, task is DONE

Be focused, pragmatic, and test-driven.`,
		},
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
