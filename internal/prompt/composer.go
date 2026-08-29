package prompt

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/iSundram/Automergent/internal/agent/agentdef"
	"github.com/iSundram/Automergent/internal/shared"
)

//go:embed bases/default.txt
var baseDefault string

//go:embed bases/gpt.txt
var baseGPT string

//go:embed bases/claude.txt
var baseClaude string

//go:embed bases/gemini.txt
var baseGemini string

//go:embed bases/beast.txt
var baseBeast string

//go:embed bases/codex.txt
var baseCodex string

//go:embed bases/trinity.txt
var baseTrinity string

//go:embed bases/kimi.txt
var baseKimi string

//go:embed bases/copilot-gpt-5.txt
var baseCopilotGPT5 string

//go:embed bases/meta.txt
var baseMeta string

// PromptComposer builds the system prompt from layered components.
type PromptComposer struct {
	layers     map[shared.PromptLayer][]string
	layerOrder []shared.PromptLayer
	model      shared.ModelInfo
	agent      *agentdef.AgentDefinition
	phase      shared.AgentPhase
	workingDir string
	skills     []Skill
	mcpServers []MCPServerInfo
	initResults *shared.InitResults
	intentSet  *shared.IntentSet
	tasks      []shared.TaskSpec
}

// MCPServerInfo represents an MCP server for prompt injection.
type MCPServerInfo struct {
	Name         string
	Instructions string
}

// NewPromptComposer creates a new prompt composer.
func NewPromptComposer(
	model shared.ModelInfo,
	agent *agentdef.AgentDefinition,
	phase shared.AgentPhase,
	workingDir string,
	skills []Skill,
	mcpServers []MCPServerInfo,
	initResults *shared.InitResults,
	intentSet *shared.IntentSet,
	tasks []shared.TaskSpec,
) *PromptComposer {
	c := &PromptComposer{
		model:       model,
		agent:       agent,
		phase:       phase,
		workingDir:  workingDir,
		skills:      skills,
		mcpServers:  mcpServers,
		initResults: initResults,
		intentSet:   intentSet,
		tasks:       tasks,
		layers:      make(map[shared.PromptLayer][]string),
		layerOrder: []shared.PromptLayer{
			shared.LayerBaseModel,
			shared.LayerEnvironment,
			shared.LayerInstructions,
			shared.LayerSkills,
			shared.LayerMCP,
			shared.LayerAgentCustom,
			shared.LayerPhase,
			shared.LayerBehavioral,
			shared.LayerTool,
			shared.LayerDynamic,
		},
	}
	c.buildAllLayers()
	return c
}

// buildAllLayers constructs all prompt layers.
func (c *PromptComposer) buildAllLayers() {
	c.buildBaseModelLayer()
	c.buildEnvironmentLayer()
	c.buildInstructionsLayer()
	c.buildSkillsLayer()
	c.buildMCPLayer()
	c.buildAgentCustomLayer()
	c.buildPhaseLayer()
	c.buildBehavioralLayer()
	c.buildToolLayer()
	c.buildDynamicLayer()
}

// Compose returns the fully composed system prompt.
func (c *PromptComposer) Compose() string {
	var parts []string
	for _, layer := range c.layerOrder {
		if content, ok := c.layers[layer]; ok && len(content) > 0 {
			parts = append(parts, strings.Join(content, "\n\n"))
		}
	}
	return strings.Join(parts, "\n\n---\n\n")
}

// AddLayer adds content to a specific layer.
func (c *PromptComposer) AddLayer(layer shared.PromptLayer, content ...string) {
	c.layers[layer] = append(c.layers[layer], content...)
}

// GetLayer returns the content for a specific layer.
func (c *PromptComposer) GetLayer(layer shared.PromptLayer) []string {
	return c.layers[layer]
}

// buildBaseModelLayer loads the model-specific base prompt.
func (c *PromptComposer) buildBaseModelLayer() {
	basePrompt := c.loadModelBasePrompt(c.model.Name, c.model.Provider)
	c.layers[shared.LayerBaseModel] = []string{basePrompt}
}

// loadModelBasePrompt returns the embedded base prompt for the model.
func (c *PromptComposer) loadModelBasePrompt(modelName, provider string) string {
	return GetBasePrompt(modelName, provider)
}

// GetBasePrompt returns the model-specific base prompt from embedded content.
// This is a package-level function that can be called from other packages.
func GetBasePrompt(modelName, provider string) string {
	modelLower := strings.ToLower(modelName)
	providerLower := strings.ToLower(provider)
	
	var basePrompt string
	switch {
	case strings.Contains(modelLower, "gpt-4") || strings.Contains(modelLower, "o1") || strings.Contains(modelLower, "o3"):
		basePrompt = baseBeast
	case strings.Contains(modelLower, "gpt") || strings.Contains(providerLower, "openai"):
		basePrompt = baseGPT
	case strings.Contains(modelLower, "claude") || strings.Contains(providerLower, "anthropic"):
		basePrompt = baseClaude
	case strings.Contains(modelLower, "gemini") || strings.Contains(providerLower, "google"):
		basePrompt = baseGemini
	case strings.Contains(modelLower, "trinity"):
		basePrompt = baseTrinity
	case strings.Contains(modelLower, "kimi") || strings.Contains(modelLower, "moonshot"):
		basePrompt = baseKimi
	case strings.Contains(modelLower, "copilot") && strings.Contains(modelLower, "gpt-5"):
		basePrompt = baseCopilotGPT5
	case strings.Contains(modelLower, "meta") || strings.Contains(providerLower, "meta"):
		basePrompt = baseMeta
	case strings.Contains(modelLower, "codex"):
		basePrompt = baseCodex
	default:
		basePrompt = baseDefault
	}
	
	// Replace template variables
	result := basePrompt
	result = strings.ReplaceAll(result, "{{MODEL_NAME}}", modelName)
	result = strings.ReplaceAll(result, "{{MODEL_ID}}", modelName)
	result = strings.ReplaceAll(result, "{{PROVIDER}}", provider)
	
	return result
}

// buildEnvironmentLayer adds dynamic environment context.
func (c *PromptComposer) buildEnvironmentLayer() {
	var sb strings.Builder
	sb.WriteString("## Environment Context\n")
	sb.WriteString(fmt.Sprintf("You are powered by the model named %s. The exact model ID is %s/%s\n", c.model.Name, c.model.Provider, c.model.Name))
	sb.WriteString("Here is some useful information about the environment you are running in:\n")
	sb.WriteString("<env>\n")
	sb.WriteString(fmt.Sprintf("  Working directory: %s\n", c.workingDir))
	sb.WriteString(fmt.Sprintf("  Workspace root folder: %s\n", c.workingDir))
	
	// Check if git repo
	if _, err := os.Stat(filepath.Join(c.workingDir, ".git")); err == nil {
		sb.WriteString("  Is directory a git repo: yes\n")
	} else {
		sb.WriteString("  Is directory a git repo: no\n")
	}
	
	sb.WriteString(fmt.Sprintf("  Platform: %s\n", runtime.GOOS))
	sb.WriteString(fmt.Sprintf("  Today's date: %s\n", time.Now().Format("Mon Jan 2 2006")))
	sb.WriteString("</env>\n")
	
	c.layers[shared.LayerEnvironment] = []string{sb.String()}
}

// buildInstructionsLayer adds AGENTS.md and custom instructions.
func (c *PromptComposer) buildInstructionsLayer() {
	var parts []string
	
	// Check for AGENTS.md in working directory and parents
	agentsContent := c.findAndReadInstructions(c.workingDir, []string{"AGENTS.md", "CLAUDE.md"})
	if agentsContent != "" {
		parts = append(parts, "## Project Instructions (AGENTS.md/CLAUDE.md)\n"+agentsContent)
	}
	
	// Check for global instructions
	homeDir, _ := os.UserHomeDir()
	if homeDir != "" {
		globalAgents := filepath.Join(homeDir, ".config", "automergent", "AGENTS.md")
		if content, err := os.ReadFile(globalAgents); err == nil {
			parts = append(parts, "## Global Instructions\n"+string(content))
		}
	}
	
	if len(parts) > 0 {
		c.layers[shared.LayerInstructions] = parts
	}
}

// findAndReadInstructions walks up the directory tree looking for instruction files.
func (c *PromptComposer) findAndReadInstructions(startDir string, filenames []string) string {
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

// buildSkillsLayer adds available skills.
func (c *PromptComposer) buildSkillsLayer() {
	if len(c.skills) == 0 {
		return
	}
	
	var sb strings.Builder
	sb.WriteString("## Available Skills\n")
	sb.WriteString("Skills provide specialized instructions and workflows for specific tasks.\n")
	sb.WriteString("Use the skill tool to load a skill when a task matches its description.\n\n")
	
	for _, skill := range c.skills {
		sb.WriteString(fmt.Sprintf("- **%s**: %s\n", skill.Name, skill.Description))
	}
	
	c.layers[shared.LayerSkills] = []string{sb.String()}
}

// buildMCPLayer adds MCP server instructions.
func (c *PromptComposer) buildMCPLayer() {
	if len(c.mcpServers) == 0 {
		return
	}
	
	var sb strings.Builder
	sb.WriteString("## MCP Server Instructions\n")
	sb.WriteString("<mcp_instructions>\n")
	for _, server := range c.mcpServers {
		sb.WriteString(fmt.Sprintf("  <server name=\"%s\">\n", server.Name))
		for _, line := range strings.Split(server.Instructions, "\n") {
			sb.WriteString(fmt.Sprintf("    %s\n", line))
		}
		sb.WriteString("  </server>\n")
	}
	sb.WriteString("</mcp_instructions>\n")
	
	c.layers[shared.LayerMCP] = []string{sb.String()}
}

// buildAgentCustomLayer adds the agent's custom prompt.
func (c *PromptComposer) buildAgentCustomLayer() {
	if c.agent == nil || c.agent.SystemPrompt == "" {
		return
	}
	c.layers[shared.LayerAgentCustom] = []string{"## Agent Instructions\n" + c.agent.SystemPrompt}
}

// buildPhaseLayer adds phase-specific prompt.
func (c *PromptComposer) buildPhaseLayer() {
	if c.agent == nil {
		return
	}
	
	// Check for phase-specific prompt in agent definition
	if phasePrompt, ok := c.agent.PhasePrompts[c.phase]; ok && phasePrompt != "" {
		c.layers[shared.LayerPhase] = []string{"## Phase: " + string(c.phase) + "\n" + phasePrompt}
		return
	}
	
	// Default phase prompts
	defaultPhasePrompts := map[shared.AgentPhase]string{
		shared.PhaseInit: `
## Phase: INIT - Request Classification
This is the FIRST message. Your job is to:
1. Classify the user's request into: direct, explore, plan, build, verify, question, violation
2. If DIRECT (simple Q&A, "what does X do", "hello"): Answer directly, no tools needed
3. If EXPLORE (needs codebase search): Transition to explore phase
4. If PLAN (needs design): Transition to plan phase  
5. If BUILD (clear implementation): Transition to build phase
6. If VIOLATION (hacking, illegal, harmful): Call violation_detected tool immediately
7. If AMBIGUOUS: Ask clarifying questions

Be concise. Use minimal tools. Prioritize the task.`,
		shared.PhaseExplore: `
## Phase: EXPLORE - Codebase Exploration
You are in exploration mode. Your job is to:
1. Search and read relevant files using glob, grep, read
2. Understand the codebase structure and patterns
3. Report findings with file paths and line numbers
4. NEVER modify files - read only
5. When exploration is complete, transition to plan or build phase`,
		shared.PhasePlan: `
## Phase: PLAN - Design & Planning
You are in planning mode. Your job is to:
1. Review exploration results
2. Create a detailed implementation plan
3. Identify specific files to modify
4. Ask clarifying questions if requirements are ambiguous
5. Define task dependencies and order
6. When plan is complete, transition to build phase`,
		shared.PhaseBuild: `
## Phase: BUILD - Implementation
You are in build mode. Your job is to:
1. Implement the plan with minimal, focused changes
2. Follow existing code style and patterns
3. MANAGE TODO LIST: Create, update, complete todos for each task
4. Run tests/lint/typecheck AFTER each change
5. If bugs found, transition to explore phase
6. When all todos complete and tests pass, task is DONE`,
	}
	
	if prompt, ok := defaultPhasePrompts[c.phase]; ok {
		c.layers[shared.LayerPhase] = []string{prompt}
	}
}

// buildBehavioralLayer adds behavioral prompts.
func (c *PromptComposer) buildBehavioralLayer() {
	behavioralPrompts := c.getBehavioralPrompts()
	if len(behavioralPrompts) > 0 {
		c.layers[shared.LayerBehavioral] = []string{"## Behavioral Rules\n" + strings.Join(behavioralPrompts, "\n\n")}
	}
}

// getBehavioralPrompts returns the behavioral prompts for the current context.
func (c *PromptComposer) getBehavioralPrompts() []string {
	var prompts []string
	
	// Global behavioral prompts (always included)
	prompts = append(prompts, `**Conciseness**: Answer concisely (<4 lines unless detail requested). No preamble/postamble. One-word answers best.`)
	
	// Phase-specific behavioral prompts (SPECIALIZED per phase like Traycer/Devin)
	phasePrompts := c.getPhaseSpecificBehavioralPrompts(c.phase)
	prompts = append(prompts, phasePrompts...)
	
	// Global behavioral prompts
	prompts = append(prompts, `**Context Management**: Read files before editing. Use grep/glob before read. Compact when >80% context. Preserve recent context.`)
	prompts = append(prompts, `**No Hallucination**: Never guess file contents. Use tools to verify. Don't make up APIs.`)
	prompts = append(prompts, `**Code Conventions**: Follow existing code conventions. Check imports, naming, patterns. Never assume libraries. Check package.json/cargo.toml/go.mod first.`)
	prompts = append(prompts, `**Violation Detection**: If user requests hacking, illegal acts, harmful code, credential theft → VIOLATION.
Call violation_detected({type, severity, user_message, agent_response}).
If user insists after 2nd warning → block_session({reason}).
If user convinces with valid context → override_violation({reason}).`)
	
	// Agent-specific behavioral prompts
	if c.agent != nil && len(c.agent.BehavioralPrompts) > 0 {
		for _, bp := range c.agent.BehavioralPrompts {
			prompts = append(prompts, bp)
		}
	}
	
	return prompts
}

// getPhaseSpecificBehavioralPrompts returns specialized behavioral prompts per phase.
func (c *PromptComposer) getPhaseSpecificBehavioralPrompts(phase shared.AgentPhase) []string {
	switch phase {
	case shared.PhaseInit:
		return []string{
			`**PHASE INIT - CLASSIFIER**: You are the ENTRY POINT. Your ONLY job: classify the request.
- DIRECT Q&A ("what does X do", "hello") → ANSWER DIRECTLY, no tools
- CODING TASK ("implement X", "fix bug Y") → ROUTE to explore/plan/build
- VIOLATION (hacking, illegal) → CALL violation_detected IMMEDIATELY
- AMBIGUOUS → ASK clarifying questions with OPTIONS
- NEVER start coding in init phase. Be decisive.`,
		}
	case shared.PhaseExplore:
		return []string{
			`**PHASE EXPLORE - READ-ONLY INVESTIGATOR**: You are a CODEBASE ARCHAEOLOGIST.
- SEARCH first (grep/glob) → READ second → NEVER edit
- MAP the codebase: find patterns, dependencies, entry points
- REPORT findings with file:line references
- NO task tool, NO edits, NO commits
- When you UNDERSTAND the problem → transition to plan`,
		}
	case shared.PhasePlan:
		return []string{
			`**PHASE PLAN - ARCHITECT**: You are the TECH LEAD designing the solution.
- REVIEW exploration results thoroughly
- DESIGN before coding: identify files, dependencies, risks
- CREATE todo list with DEPENDENCIES (task A blocks task B)
- ASK clarifying questions if requirements unclear
- SPECIFY: files to change, test strategy, rollback plan
- When plan is SOLID → transition to build`,
		}
	case shared.PhaseBuild:
		return []string{
			`**PHASE BUILD - IMPLEMENTER + TESTER + TODO MANAGER**: You are the LEAD ENGINEER.
- TODO MANAGEMENT: CREATE todo list → UPDATE on progress → COMPLETE on done
- TEST-DRIVEN: Write test → Make it pass → Refactor → RUN tests/lint/typecheck
- ONE task at a time: complete current todo before starting next
- MINIMAL changes: focused edits, follow existing patterns
- If BLOCKED: transition to explore, DON'T guess
- When ALL todos DONE + tests PASS → task COMPLETE`,
		}
	default:
		return []string{}
	}
}

// buildToolLayer adds per-tool prompts.
func (c *PromptComposer) buildToolLayer() {
	toolPrompts := c.getToolPrompts()
	if len(toolPrompts) > 0 {
		c.layers[shared.LayerTool] = []string{"## Tool-Specific Guidance\n" + strings.Join(toolPrompts, "\n\n")}
	}
}

// getToolPrompts returns tool-specific prompts for available tools.
func (c *PromptComposer) getToolPrompts() []string {
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
	if c.agent != nil && c.agent.ToolPrompts != nil {
		for tool, config := range c.agent.ToolPrompts {
			defaultToolPrompts[tool] = config
		}
	}
	
	// Only include prompts for tools that are available
	availableTools := c.getAvailableTools()
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
	
	return prompts
}

// getAvailableTools returns the list of available tools for the current phase/agent.
func (c *PromptComposer) getAvailableTools() []string {
	if c.agent != nil && c.agent.PhaseTools != nil {
		if tools, ok := c.agent.PhaseTools[c.phase]; ok {
			return tools
		}
	}
	
	phaseTools := map[shared.AgentPhase][]string{
		shared.PhaseInit:     {"bash", "read", "glob", "grep"},           // classify only, no task
		shared.PhaseExplore:  {"glob", "grep", "read", "bash"},           // read-only exploration, NO task
		shared.PhasePlan:     {"read", "write", "bash", "task"},          // planning + can delegate research
		shared.PhaseBuild:    {"edit", "bash", "write", "read", "task", "glob", "grep"}, // full + todo
	}
	
	if tools, ok := phaseTools[c.phase]; ok {
		return tools
	}
	return []string{"bash", "read", "glob", "grep"}
}

// buildDynamicLayer adds dynamic reminders (plan mode, compaction, etc.).
func (c *PromptComposer) buildDynamicLayer() {
	var parts []string
	
	// Pre-explored context notice
	if c.initResults != nil && len(c.initResults.FilesFound) > 0 {
		var sb strings.Builder
		sb.WriteString("## Pre-Explored Context (Prep Phase ALREADY Executed)\n")
		sb.WriteString("IMPORTANT: The codebase was already explored for this request. Do NOT call glob, list_directory, or broad search tools.\n")
		sb.WriteString("START by reading the most relevant files below with read_file.\n\n")
		sb.WriteString(fmt.Sprintf("Discovered files (%d):\n", len(c.initResults.FilesFound)))
		for i, f := range c.initResults.FilesFound {
			if i >= 30 {
				sb.WriteString(fmt.Sprintf("- ... and %d more\n", len(c.initResults.FilesFound)-30))
				break
			}
			sb.WriteString(fmt.Sprintf("- %s\n", f))
		}
		if len(c.initResults.CodeSnippets) > 0 {
			sb.WriteString("\nContents already loaded (no need to re-read):\n")
			for path := range c.initResults.CodeSnippets {
				sb.WriteString(fmt.Sprintf("- %s\n", path))
			}
		}
		parts = append(parts, sb.String())
	}
	
	// Current request and intents
	if c.intentSet != nil {
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("## Current Request\n%s\n\n", c.intentSet.OriginalPrompt))
		sb.WriteString("## Identified Intents\n")
		for _, intent := range c.intentSet.Intents {
			sb.WriteString(fmt.Sprintf("- %s (priority: %d)\n", intent.Type, intent.Priority))
		}
		parts = append(parts, sb.String())
	}
	
	// Task progress
	if len(c.tasks) > 0 {
		var sb strings.Builder
		sb.WriteString("## Task Progress\n")
		for _, task := range c.tasks {
			sb.WriteString(fmt.Sprintf("⏳ %s (priority: %d)\n", task.Description, task.Priority))
		}
		parts = append(parts, sb.String())
	}
	
	// Working areas
	if len(c.tasks) > 0 {
		var workingAreas []string
		for _, task := range c.tasks {
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
	if c.intentSet != nil && len(c.tasks) > 15 {
		parts = append(parts, "[Note: Conversation history is long. Focus on recent state and established plan.]")
	}
	
	if len(parts) > 0 {
		c.layers[shared.LayerDynamic] = parts
	}
}

// Context for loading skills (interface to avoid circular deps)
type Skill interface {
	Name() string
	Description() string
}