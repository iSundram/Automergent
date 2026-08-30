package builtin

import (
	"github.com/iSundram/Automergent/internal/agent/agentdef"
	"github.com/iSundram/Automergent/internal/shared"
)

// ExploreAgent returns the explore/read-only agent definition.
func ExploreAgent() *agentdef.AgentDefinition {
	return &agentdef.AgentDefinition{
		Name:        "explore",
		Description: "Fast read-only codebase exploration and research",
		WhenToUse:   "Use for finding files, understanding code structure, searching patterns, researching implementations. Never modifies files.",
		SystemPrompt: `You are a codebase exploration specialist. Your job is to find information quickly and report findings.

Available tools: read, grep, glob, bash (read-only commands only).

Your task is to:
1. Explore and understand the relevant code
2. Gather context and dependencies
3. Identify patterns and architecture
4. Report findings concisely with file paths and line numbers

Rules:
- NEVER modify, write, or edit any files
- NEVER execute destructive commands
- Focus on accuracy and completeness of findings
- Report file paths as relative paths from the working directory
- Include line numbers for specific code references
- Summarize large code sections rather than dumping raw content
- If you encounter errors, investigate before reporting failure`,
		Model:       "",
		Tools:       []string{"read", "grep", "glob", "bash"},
		Color:       "blue",
		Effort:      agentdef.EffortMedium,
		Source:      agentdef.SourceBuiltin,
		MemoryScope: agentdef.MemoryScopeNone,
		Timeout:     0,
		// PhasePrompts intentionally unset: the shared per-phase files
		// (prompt/phases/*.txt) are the platform defaults and are richer than
		// the per-agent overrides that used to live here.
		
		PhaseTools: map[shared.AgentPhase][]string{
			shared.PhaseInit:     {"bash", "read", "glob", "grep"},
			shared.PhaseExplore:  {"glob", "grep", "read", "bash"},
			shared.PhasePlan:     {"read", "write"},
		},
		PhaseMaxSteps: map[shared.AgentPhase]int{
			shared.PhaseInit:     3,
			shared.PhaseExplore:  10,
			shared.PhasePlan:     5,
		},
		ToolPrompts: map[string]shared.ToolPromptConfig{
			"glob": {PreExecution: "Find files by pattern. Use ** for recursive."},
			"grep": {PreExecution: "Search content with regex. Use include filter for file types."},
			"read": {PreExecution: "Read files to understand code. Use offset/limit for large files."},
			"bash": {PreExecution: "Execute read-only shell commands. Use absolute paths."},
		},
		BehavioralPrompts: []string{
			"Phase Discipline: Stay in current phase. Explore=search only. Plan=design only.",
			"Context Management: Use grep/glob before read. Preserve recent context.",
			"No Hallucination: Never guess file contents. Use tools to verify.",
		},
		ViolationPolicy: shared.ViolationPolicy{
			MaxWarnings:    1,
			BlockOnPersist: true,
			AllowOverride:  true,
		},
	}
}
