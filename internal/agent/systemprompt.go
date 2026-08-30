package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/iSundram/Automergent/internal/ai"
	"github.com/iSundram/Automergent/internal/prompt"
	"github.com/iSundram/Automergent/internal/shared"
)

// getSystemPrompt returns THE system prompt using layered composition via PromptComposer.
// This is the single source of truth: there is no assistant/coder split and
// no legacy fallback path. The prompt is composed from layers:
// BaseModel → Environment → Instructions → Skills → MCP → AgentCustom → Phase → Behavioral → Tool → Dynamic
func (a *Agent) getSystemPrompt(ctx context.Context, provider ai.Provider) string {
	var systemPrompt string
	// If PromptComposer is available (phase-aware mode), use it
	if a.promptComposer != nil {
		systemPrompt = a.promptComposer.Compose()
	} else {
		// Fallback: Build using legacy method but with layered approach
		systemPrompt = a.buildLayeredSystemPrompt(ctx, provider)
	}

	// Slash-command hints are registry-derived (app-level) and independent of
	// the composer's agent definition, so they are appended here for both
	// paths. The block renders hints in sorted order to keep the prompt-cache
	// prefix stable.
	if hints := a.commandHintSnapshot(); len(hints) > 0 {
		if block := commandHintsPromptBlock(hints); block != "" {
			systemPrompt = strings.TrimRight(systemPrompt, "\n") + "\n\n" + strings.TrimRight(block, "\n")
		}
	}
	return systemPrompt
}

// buildLayeredSystemPrompt builds the system prompt using layered composition (fallback).
func (a *Agent) buildLayeredSystemPrompt(ctx context.Context, provider ai.Provider) string {
	var layers []promptLayerContent

	// Layer 1: Base Model Prompt
	layers = append(layers, promptLayerContent{
		Layer:  shared.LayerBaseModel,
		Prompt: a.getBaseModelPrompt(),
	})

	// Layer 2: Environment
	layers = append(layers, promptLayerContent{
		Layer:  shared.LayerEnvironment,
		Prompt: a.getEnvironmentPrompt(),
	})

	// Layer 3: Instructions (AGENTS.md, CLAUDE.md)
	if instrPrompt := a.getInstructionsPrompt(); instrPrompt != "" {
		layers = append(layers, promptLayerContent{
			Layer:  shared.LayerInstructions,
			Prompt: instrPrompt,
		})
	}

	// Layer 4: Skills
	if skillsPrompt := a.getSkillsPrompt(); skillsPrompt != "" {
		layers = append(layers, promptLayerContent{
			Layer:  shared.LayerSkills,
			Prompt: skillsPrompt,
		})
	}

	// Layer 5: MCP
	if mcpPrompt := a.getMCPPrompt(); mcpPrompt != "" {
		layers = append(layers, promptLayerContent{
			Layer:  shared.LayerMCP,
			Prompt: mcpPrompt,
		})
	}

	// Layer 6: Agent Custom Prompt
	if a.currentAgentDef != nil && a.currentAgentDef.SystemPrompt != "" {
		layers = append(layers, promptLayerContent{
			Layer:  shared.LayerAgentCustom,
			Prompt: "## Agent Instructions\n" + a.currentAgentDef.SystemPrompt,
		})
	}

	// Layer 7: Phase-specific
	if a.phaseManager != nil {
		phase := a.phaseManager.CurrentPhase()
		if phasePrompt := a.getPhasePrompt(phase); phasePrompt != "" {
			layers = append(layers, promptLayerContent{
				Layer:  shared.LayerPhase,
				Prompt: phasePrompt,
			})
		}
	}

	// Layer 8: Behavioral
	layers = append(layers, promptLayerContent{
		Layer:  shared.LayerBehavioral,
		Prompt: a.getBehavioralPrompt(),
	})

	// Layer 9: Tool-specific
	if toolPrompt := a.getToolPrompt(); toolPrompt != "" {
		layers = append(layers, promptLayerContent{
			Layer:  shared.LayerTool,
			Prompt: toolPrompt,
		})
	}

	// Layer 10: Dynamic (prep pipeline results, etc.)
	if dynamicPrompt := a.getDynamicPrompt(); dynamicPrompt != "" {
		layers = append(layers, promptLayerContent{
			Layer:  shared.LayerDynamic,
			Prompt: dynamicPrompt,
		})
	}

	// Compose all layers
	var parts []string
	for _, layer := range layers {
		if layer.Prompt != "" {
			parts = append(parts, layer.Prompt)
		}
	}

	return strings.Join(parts, "\n\n---\n\n")
}

type promptLayerContent struct {
	Layer  shared.PromptLayer
	Prompt string
}

// getBaseModelPrompt loads the model-specific base prompt from prompt package.
func (a *Agent) getBaseModelPrompt() string {
	modelName := ""
	if a.cfg != nil {
		modelName = a.cfg.Model
	}
	providerName := a.provider.Name()
	return prompt.GetBasePrompt(modelName, providerName)
}

// getEnvironmentPrompt returns the environment context.
func (a *Agent) getEnvironmentPrompt() string {
	var sb strings.Builder
	sb.WriteString("## Environment Context\n")
	sb.WriteString(fmt.Sprintf("You are powered by the model named %s. The exact model ID is %s/%s\n", a.provider.Name(), a.provider.Name(), a.cfg.Model))
	sb.WriteString("Here is some useful information about the environment you are running in:\n")
	sb.WriteString("<env>\n")
	sb.WriteString(fmt.Sprintf("  Working directory: %s\n", a.workDir))
	sb.WriteString(fmt.Sprintf("  Workspace root folder: %s\n", a.workDir))
	
	// Check if git repo
	if _, err := os.Stat(filepath.Join(a.workDir, ".git")); err == nil {
		sb.WriteString("  Is directory a git repo: yes\n")
		// Branch / status / recent commits — the same snapshot the prompt
		// composer injects, so every prompt path sees identical context.
		sb.WriteString(prompt.GitStatusBlock(a.workDir))
	} else {
		sb.WriteString("  Is directory a git repo: no\n")
	}
	
	sb.WriteString(fmt.Sprintf("  Platform: %s\n", runtime.GOOS))
	sb.WriteString(fmt.Sprintf("  Today's date: %s\n", time.Now().Format("Mon Jan 2 2006")))
	sb.WriteString("</env>\n")
	
	return sb.String()
}

// getInstructionsPrompt is deliberately empty: project instructions are
// injected as a high-weight user message by the agent loop (usercontext.go),
// not duplicated into the system prompt.
func (a *Agent) getInstructionsPrompt() string {
	return ""
}

// findAndReadInstructions walks up the directory tree looking for instruction files.
func (a *Agent) findAndReadInstructions(startDir string, filenames []string) string {
	var parts []string
	dir := startDir
	for {
		for _, filename := range filenames {
			path := filepath.Join(dir, filename)
			if content, err := os.ReadFile(path); err == nil {
				parts = append(parts, fmt.Sprintf("### %s (from %s)\n%s", filename, dir, string(content)))
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return strings.Join(parts, "\n\n")
}

// getSkillsPrompt returns the skills availability prompt.
func (a *Agent) getSkillsPrompt() string {
	if len(a.skills) == 0 {
		return ""
	}
	
	var sb strings.Builder
	sb.WriteString("## Available Skills\n")
	sb.WriteString("Skills provide specialized instructions and workflows for specific tasks.\n")
	sb.WriteString("Use the skill tool to load a skill when a task matches its description.\n\n")
	
	for _, skill := range a.skills {
		sb.WriteString(fmt.Sprintf("- **%s**: %s\n", skill.Name, skill.Description))
	}
	
	return sb.String()
}

// getMCPPrompt returns MCP server instructions.
func (a *Agent) getMCPPrompt() string {
	mcpServers := a.getMCPServers()
	if len(mcpServers) == 0 {
		return ""
	}
	
	var sb strings.Builder
	sb.WriteString("## MCP Server Instructions\n")
	sb.WriteString("<mcp_instructions>\n")
	for _, server := range mcpServers {
		sb.WriteString(fmt.Sprintf("  <server name=\"%s\">\n", server.Name))
		for _, line := range strings.Split(server.Instructions, "\n") {
			sb.WriteString(fmt.Sprintf("    %s\n", line))
		}
		sb.WriteString("  </server>\n")
	}
	sb.WriteString("</mcp_instructions>\n")
	
	return sb.String()
}

// getPhasePrompt returns the phase-specific prompt. Defaults come from the
// shared per-phase files (prompt/phases/*.txt); agent definitions may
// override via PhasePrompts.
func (a *Agent) getPhasePrompt(phase shared.AgentPhase) string {
	if a.currentAgentDef != nil {
		if phasePrompt, ok := a.currentAgentDef.PhasePrompts[phase]; ok && phasePrompt != "" {
			return "## Phase: " + string(phase) + "\n" + phasePrompt
		}
	}
	if p := prompt.PhasePrompt(phase); p != "" {
		return "## Phase: " + string(phase) + "\n" + p
	}
	return ""
}

// getBehavioralPrompt returns the behavioral rules prompt.
func (a *Agent) getBehavioralPrompt() string {
	var prompts []string
	
	// Global behavioral prompts (always included)
	prompts = append(prompts, `**Conciseness**: Answer concisely (<4 lines unless detail requested). No preamble/postamble. One-word answers best.`)
	
	// Phase discipline
	if a.phaseManager != nil {
		prompts = append(prompts, fmt.Sprintf("**Phase Discipline**: Stay in %s phase. Transition only via PhaseManager.", a.phaseManager.CurrentPhase()))
	}
	
	// Context management
	prompts = append(prompts, `**Context Management**: Read files before editing. Use grep/glob before read. Compact when >80% context. Preserve recent context.`)
	
	// No hallucination
	prompts = append(prompts, `**No Hallucination**: Never guess file contents. Use tools to verify. Don't make up APIs.`)
	
	// Code conventions
	prompts = append(prompts, `**Code Conventions**: Follow existing code conventions. Check imports, naming, patterns. Never assume libraries. Check package.json/cargo.toml/go.mod first.`)
	
	// Violation detection
	prompts = append(prompts, `**Violation Detection**: If user requests hacking, illegal acts, harmful code, credential theft → VIOLATION.
Call violation_detected({type, severity, user_message, agent_response}).
If user insists after 2nd warning → block_session({reason}).
If user convinces with valid context → override_violation({reason}).`)
	
	// Agent-specific behavioral prompts
	if a.currentAgentDef != nil && len(a.currentAgentDef.BehavioralPrompts) > 0 {
		for _, bp := range a.currentAgentDef.BehavioralPrompts {
			prompts = append(prompts, bp)
		}
	}
	
	return "## Behavioral Rules\n" + strings.Join(prompts, "\n\n")
}

// getToolPrompt returns tool-specific guidance.
func (a *Agent) getToolPrompt() string {
	var prompts []string
	
	// Default tool prompts
	defaultToolPrompts := map[string]shared.ToolPromptConfig{
		"bash": {
			PreExecution: "Execute shell commands. Prefer non-interactive. Use absolute paths. Quote paths with spaces.",
			Rules: []string{
				"Never curl/wget | sh in one command",
				"Use && for chaining, not ;",
				"Check exit codes",
			},
		},
		"read": {
			PreExecution: "Read files to understand code. Use offset/limit for large files.",
			Rules: []string{
				"Don't read entire large files - use offset/limit",
				"Prefer grep/glob for finding specific content first",
			},
		},
		"edit": {
			PreExecution: "Make minimal, focused edits. Match exact indentation.",
			Rules: []string{
				"Use replaceAll only for renaming",
				"Preserve existing code style",
				"No comments unless asked",
			},
		},
		"task": {
			PreExecution: "Delegate to subagent. Provide clear description and context.",
			Rules: []string{
				"One task per subagent",
				"Include relevant file paths in description",
			},
		},
		"glob": {
			PreExecution: "Find files by pattern. Use ** for recursive.",
		},
		"grep": {
			PreExecution: "Search content with regex. Use include filter for file types.",
		},
		"webfetch": {
			PreExecution: "Fetch web content. Use for documentation, not user requests.",
		},
		"websearch": {
			PreExecution: "Search web for current info. Use for recent events, not codebase questions.",
		},
	}
	
	// Merge with agent-specific tool prompts
	if a.currentAgentDef != nil && a.currentAgentDef.ToolPrompts != nil {
		for tool, config := range a.currentAgentDef.ToolPrompts {
			defaultToolPrompts[tool] = config
		}
	}
	
	// Get available tools for current phase
	availableTools := a.getAvailableToolsForPhase()
	for _, toolName := range availableTools {
		if config, ok := defaultToolPrompts[toolName]; ok {
			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("### %s\n", toolName))
			if config.PreExecution != "" {
				sb.WriteString(config.PreExecution + "\n")
			}
			if len(config.Rules) > 0 {
				sb.WriteString("Rules:\n")
				for _, rule := range config.Rules {
					sb.WriteString(fmt.Sprintf("- %s\n", rule))
				}
			}
			prompts = append(prompts, sb.String())
		}
	}
	
	if len(prompts) > 0 {
		return "## Tool-Specific Guidance\n" + strings.Join(prompts, "\n\n")
	}
	return ""
}

// getAvailableToolsForPhase returns available tools for current phase.
func (a *Agent) getAvailableToolsForPhase() []string {
	if a.phaseManager != nil {
		config := a.phaseManager.GetPhaseConfig(a.phaseManager.CurrentPhase())
		return config.Tools
	}
	
	// Default tools per phase
	if a.phaseManager != nil {
	phase := a.phaseManager.CurrentPhase()
	phaseTools := map[shared.AgentPhase][]string{
		shared.PhaseInit:     {"bash", "read", "glob", "grep"},
		shared.PhaseExplore:  {"glob", "grep", "read", "bash"},
		shared.PhasePlan:     {"read", "write", "bash", "task"},
		shared.PhaseBuild:    {"edit", "bash", "write", "read", "task", "glob", "grep"},
	}
	if tools, ok := phaseTools[phase]; ok {
		return tools
	}
	return []string{"bash", "read", "glob", "grep"}
	}
	
	return []string{"bash", "read", "glob", "grep", "task", "edit", "write"}
}

// getDynamicPrompt returns dynamic reminders (prep pipeline results, etc.).
func (a *Agent) getDynamicPrompt() string {
	var parts []string
	
	// Pre-explored context notice
	if initResults := a.promptSystem.GetInitResults(); initResults != nil && len(initResults.FilesFound) > 0 {
		var sb strings.Builder
		sb.WriteString("## Pre-Explored Context (Prep Phase ALREADY Executed)\n")
		sb.WriteString("IMPORTANT: The codebase was already explored for this request. Do NOT call glob, list_directory, or broad search tools.\n")
		sb.WriteString("START by reading the most relevant files below with read_file.\n\n")
		sb.WriteString(fmt.Sprintf("Discovered files (%d):\n", len(initResults.FilesFound)))
		for i, f := range initResults.FilesFound {
			if i >= 30 {
				sb.WriteString(fmt.Sprintf("- ... and %d more\n", len(initResults.FilesFound)-30))
				break
			}
			sb.WriteString(fmt.Sprintf("- %s\n", f))
		}
		if len(initResults.CodeSnippets) > 0 {
			sb.WriteString("\nContents already loaded (no need to re-read):\n")
			for path := range initResults.CodeSnippets {
				sb.WriteString(fmt.Sprintf("- %s\n", path))
			}
		}
		parts = append(parts, sb.String())
	}
	
	// Current request and intents
	if intentSet := a.promptSystem.GetCurrentIntentSet(); intentSet != nil {
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("## Current Request\n%s\n\n", intentSet.OriginalPrompt))
		sb.WriteString("## Identified Intents\n")
		for _, intent := range intentSet.Intents {
			sb.WriteString(fmt.Sprintf("- %s (priority: %d)\n", intent.Type, intent.Priority))
		}
		parts = append(parts, sb.String())
	}
	
	// Task progress
	if tasks := a.promptSystem.GetCurrentTasks(); len(tasks) > 0 {
		var sb strings.Builder
		sb.WriteString("## Task Progress\n")
		for _, task := range tasks {
			sb.WriteString(fmt.Sprintf("⏳ %s (priority: %d)\n", task.Description, task.Priority))
		}
		parts = append(parts, sb.String())
	}
	
	// Working areas
	if tasks := a.promptSystem.GetCurrentTasks(); len(tasks) > 0 {
		var workingAreas []string
		for _, task := range tasks {
			if files, ok := task.Context["files_found"].([]string); ok {
				workingAreas = append(workingAreas, files...)
			}
		}
		if len(workingAreas) > 0 {
			var sb strings.Builder
			sb.WriteString("## Working Areas\n")
			for _, f := range workingAreas {
				sb.WriteString(fmt.Sprintf("- %s\n", f))
			}
			parts = append(parts, sb.String())
		}
	}
	
	// Long history note
	if len(a.sess.Messages) > 15 {
		parts = append(parts, "[Note: Conversation history is long. Focus on recent state and established plan.]")
	}
	
	if len(parts) > 0 {
		return strings.Join(parts, "\n\n")
	}
	return ""
}