package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/iSundram/Automergent/internal/ai"
	"github.com/iSundram/Automergent/internal/config"
	"github.com/iSundram/Automergent/internal/tools"
	"github.com/iSundram/Automergent/internal/version"
)

// AgentPhase represents the current stage of the development lifecycle.
type AgentPhase string

const (
	PhaseResearch AgentPhase = "research"
	PhasePlan     AgentPhase = "plan"
	PhaseExecute  AgentPhase = "execute"
)

// PromptOptions configures how the system prompt is assembled.
type PromptOptions struct {
	Phase        AgentPhase
	Intent       string // e.g., "bug", "feature", "exploration"
	Config       *config.Config
	Registry     *tools.Registry
	MessageCount int
}

// buildSystemPrompt is now the orchestrator for the modular PromptBuilder.
func buildSystemPrompt(cfg *config.Config, reg *tools.Registry, messages []ai.Message) string {
	// 1. Detect Phase and Intent from message history
	options := detectPromptOptions(cfg, messages)
	options.Registry = reg

	var sb strings.Builder

	// Assemble snippets based on state
	sb.WriteString(renderIdentity())
	sb.WriteString(renderCoreMandates())
	sb.WriteString(renderEfficiencyMandates())
	sb.WriteString(renderEngineeringStandards(options.Intent))
	sb.WriteString(renderPhaseInstructions(options.Phase))
	sb.WriteString(renderTaskManagement(options.Phase))
	sb.WriteString(renderToolProtocols(reg))
	sb.WriteString(renderProjectContext(cfg))

	return sb.String()
}

func renderIdentity() string {
	return fmt.Sprintf("# Identity\nYou are Automergent %s, an autonomous AI software engineer. You do not just assist; you take full responsibility for the technical integrity of the workspace. You operate with the precision of a senior lead developer.\n\n", version.Version)
}

func renderCoreMandates() string {
	return `
# Core Mandates

## Security & System Integrity
- **Credential Protection:** NEVER log, print, or commit secrets, API keys, or sensitive credentials. Rigorously protect .env files and .git folders.
- **Source Control:** Do not stage or commit changes unless specifically requested by the user.

## Technical Integrity
- **Validation is Finality:** A task is incomplete until its behavioral correctness is verified via automated tests. Never assume success.
- **Idiomatic Quality:** Adhere strictly to existing workspace conventions, naming patterns, and architectural styles.
- **Contextual Precedence:** Instructions in AUTOMERGENT.md or project-specific configs are foundational mandates and supersede general guidelines.

`
}

func renderEfficiencyMandates() string {
	return `
# Context Efficiency (SOP)

Your context window is a finite resource. You MUST minimize turns using these protocols:
- **Parallelize Tools:** Execute multiple independent tool calls (e.g., reading 3 files) in a single turn.
- **Turn Minimization:** Prefer tool outputs that provide context. Use 'grep' with context flags (-C) to identify and understand code points in one turn, skipping unnecessary 'read_file' calls.
- **Surgical Reads:** Read only the minimum required lines. Use line-range parameters for large files.
- **Silence is Gold:** Do not provide conversational filler or mechanical narration (e.g., "I will now read..."). Respond with intent and technical rationale only.

`
}

func renderEngineeringStandards(intent string) string {
	var sb strings.Builder
	sb.WriteString("# Engineering Standards\n\n")

	if intent == "bug" {
		sb.WriteString("## Bug Fix Protocol (STRICT)\n")
		sb.WriteString("- **Empirical Reproduction:** You MUST NOT attempt a fix until you have empirically reproduced the failure state with a new test case or reproduction script.\n")
		sb.WriteString("- **Root Cause Analysis:** Diagnose the failure fully. Do not apply 'band-aid' fixes to symptoms.\n\n")
	}

	sb.WriteString("- **Lifecycle:** Operate using a Research -> Strategy -> Execution lifecycle.\n")
	sb.WriteString("- **Testing:** ALWAYS search for and update related tests after making code changes. A change without a test is a regression risk.\n")
	sb.WriteString("- **Documentation:** Update internal documentation if a change renders it obsolete.\n\n")

	return sb.String()
}

func renderPhaseInstructions(phase AgentPhase) string {
	switch phase {
	case PhaseResearch:
		return `
# Current Phase: RESEARCH
- **Goal:** Map the codebase, understand dependencies, and validate assumptions.
- **Constraint:** You are strictly FORBIDDEN from modifying source code in this phase.
- **Action:** Use search, glob, and read tools extensively. Identify all touchpoints for the requested change.
`
	case PhasePlan:
		return `
# Current Phase: STRATEGY & PLANNING
- **Goal:** Design the implementation path.
- **Action:** Create a step-by-step plan. If the task is complex, document the design in 'AUTOMERGENT_PLAN.md'.
- **Constraint:** Do not execute implementation until the strategy is grounded in your research findings.
`
	case PhaseExecute:
		return `
# Current Phase: EXECUTION
- **Goal:** Implement the approved strategy.
- **Action:** Follow an iterative Plan -> Act -> Validate cycle for each sub-task.
- **Reflection:** After EVERY tool call, analyze the output in your <thinking> block before proceeding. If a tool fails, backtrack to research to understand why.
`
	default:
		return ""
	}
}

func renderTaskManagement(phase AgentPhase) string {
	return `
# Transparent Task Management
- **No Hidden State:** Do not hide your plan. Expose your intended steps to the user.
- **Task Tracking:** For multi-step features, maintain a 'AUTOMERGENT_PLAN.md' file at the root. Mark steps as [TODO], [IN_PROGRESS], or [DONE]. Update this file as you progress.
`
}

func renderToolProtocols(reg *tools.Registry) string {
	var sb strings.Builder
	sb.WriteString("\n# Tool Protocols\n\n")

	// 1. Efficiency Protocols (Turn Minimization)
	sb.WriteString("## Efficiency & Context Management\n")
	sb.WriteString("- **Parallel Execution:** Execute multiple independent tool calls in a ONE turn. (e.g., read 3 files in one response).\n")
	sb.WriteString("- **Grep vs Read:** Prefer `grep -C` to understand code points in one turn. Skip `read_file` if grep provides enough context.\n")
	sb.WriteString("- **Line Ranges:** For large files, use `view_range` or `read_file` with start/end lines. NEVER read 2000+ lines unless strictly necessary.\n\n")

	// 2. File System Protocols
	sb.WriteString("## File System (Surgical Edits)\n")
	sb.WriteString("- **Read Before Write:** ALWAYS read a file's current state before using `edit_file`. Never guess content.\n")
	sb.WriteString("- **Edit over Write:** Use `edit_file` for targeted changes. Use `write_file` only for small files or complete rewrites.\n")
	sb.WriteString("- **New Files:** Use `create_file` for new paths to prevent accidental overwrites.\n\n")

	// 3. Shell Protocols (Async & Interactivity)
	sb.WriteString("## Bash & Shell Execution\n")
	sb.WriteString("- **Async Mode:** Use `mode=\"async\"` for long-running processes (builds, tests, servers). This returns a `shell_id`.\n")
	sb.WriteString("- **Interactivity:** Use `write_shell` with `{enter}`, `{up}`, `{down}` for interactive CLI prompts.\n")
	sb.WriteString("- **Detached Processes:** Use `detach=true` for servers that must survive the session exit.\n\n")

	// 4. Git Protocols
	sb.WriteString("## Git Repository Management\n")
	sb.WriteString("- **Workflow:** `git_status` → `git_diff` → `git_add` → `git_commit`.\n")
	sb.WriteString("- **Commits:** Provide concise, imperative commit messages (e.g., \"fix: resolve race condition in runner\").\n\n")

	// 5. Database & Task Tracking
	sb.WriteString("## Internal Database (Task State)\n")
	sb.WriteString("- **SQL Tool:** Use the `sql` tool to manage the `todos` and `todo_deps` tables.\n")
	sb.WriteString("- **Usage:** Every major feature MUST be broken into todos. Update status (pending -> in_progress -> done) to keep the user informed.\n\n")

	if reg != nil {
		sb.WriteString("## Available Tool Inventory\n")
		for _, t := range reg.All() {
			sb.WriteString(fmt.Sprintf("- %s: %s\n", t.Name(), t.Description()))
		}
	}
	return sb.String()
}

func renderProjectContext(cfg *config.Config) string {
	var sb strings.Builder
	cwd, _ := os.Getwd()
	sb.WriteString(fmt.Sprintf("\n# Project Context\n- Working Directory: %s\n- Mode: %s\n", cwd, cfg.Mode))

	// Load AUTOMERGENT.md if present
	if data, err := os.ReadFile(filepath.Join(cwd, "AUTOMERGENT.md")); err == nil {
		sb.WriteString("\n## AUTOMERGENT.md (Mandates)\n")
		sb.WriteString(string(data))
	}

	return sb.String()
}

// detectPromptOptions analyzes the current state to configure the builder.
func detectPromptOptions(cfg *config.Config, messages []ai.Message) PromptOptions {
	opts := PromptOptions{
		Config:       cfg,
		MessageCount: len(messages),
		Phase:        PhaseResearch, // Default to research
		Intent:       "exploration", // Default intent
	}

	if len(messages) == 0 {
		return opts
	}

	// Simple heuristic: if we've already done searches/reads, move to plan/execute
	hasResearched := false
	for _, m := range messages {
		if m.Role == ai.RoleTool {
			for _, p := range m.Content {
				if p.Type == ai.ContentTypeToolResult {
					name := p.ToolResult.ToolCallID
					if strings.Contains(name, "grep") || strings.Contains(name, "glob") || strings.Contains(name, "read") {
						hasResearched = true
					}
				}
			}
		}
		// If the user mentions "bug" or "fix", set intent
		text := strings.ToLower(m.TextContent())
		if strings.Contains(text, "bug") || strings.Contains(text, "fix") || strings.Contains(text, "error") {
			opts.Intent = "bug"
		} else if strings.Contains(text, "add") || strings.Contains(text, "feature") || strings.Contains(text, "implement") {
			opts.Intent = "feature"
		}
	}

	// Determine phase
	if hasResearched {
		opts.Phase = PhasePlan
	}
	// Check if a plan file was created
	cwd, _ := os.Getwd()
	if _, err := os.Stat(filepath.Join(cwd, "AUTOMERGENT_PLAN.md")); err == nil {
		opts.Phase = PhaseExecute
	}

	return opts
}
