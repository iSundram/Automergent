package prompt

import (
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/iSundram/Automergent/internal/agent/agentdef"
	"github.com/iSundram/Automergent/internal/shared"
	"github.com/iSundram/Automergent/internal/tools"
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
	// registry gives the tool layer access to live tool metadata (Meta()),
	// the self-documenting per-tool prompt system. Nil is legal: the layer
	// then falls back to the static table only.
	registry *tools.Registry
}

// SetRegistry attaches the live tool registry so the tool layer renders the
// registry's own per-tool documentation (ToolMeta) for offered tools.
func (c *PromptComposer) SetRegistry(reg *tools.Registry) {
	c.registry = reg
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
			shared.LayerPlatform,
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
	c.buildPlatformLayer()
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

//go:embed bases/platform.txt
var basePlatform string

// buildPlatformLayer adds the shared platform core — system rules, task
// discipline, action care, and communication style. This content is
// model-agnostic and identical for every agent and phase, which keeps it
// inside the cacheable prompt prefix; the model bases carry only identity
// and tone.
func (c *PromptComposer) buildPlatformLayer() {
	c.layers[shared.LayerPlatform] = []string{basePlatform}
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
		sb.WriteString(c.gitStatusBlock())
	} else {
		sb.WriteString("  Is directory a git repo: no\n")
	}

	sb.WriteString(fmt.Sprintf("  Platform: %s\n", runtime.GOOS))
	sb.WriteString(fmt.Sprintf("  Today's date: %s\n", time.Now().Format("Mon Jan 2 2006")))
	sb.WriteString("</env>\n")

	c.layers[shared.LayerEnvironment] = []string{sb.String()}
}

// gitStatusBlock renders a compact, read-only git snapshot: current branch,
// the default branch, and the first lines of the working-tree status. Bounded
// so a dirty tree cannot flood the system prompt.
func (c *PromptComposer) gitStatusBlock() string {
	return GitStatusBlock(c.workingDir)
}

// GitStatusBlock renders a compact, read-only git snapshot of dir: current
// branch, the default branch, the first lines of the working-tree status,
// and recent commits. Returns "" when dir is not inside a git repository or
// git is unavailable — git context is a nicety, not a dependency.
func GitStatusBlock(dir string) string {
	const maxStatusLines = 20
	branch := gitOutput(dir, "rev-parse", "--abbrev-ref", "HEAD")
	if branch == "" {
		return ""
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("  Current branch: %s\n", branch))
	if main := gitOutput(dir, "rev-parse", "--abbrev-ref", "origin/HEAD"); main != "" {
		sb.WriteString(fmt.Sprintf("  Main branch: %s\n", strings.TrimPrefix(main, "origin/")))
	}
	if status := gitOutput(dir, "status", "--porcelain"); status != "" {
		lines := strings.Split(strings.TrimRight(status, "\n"), "\n")
		if len(lines) > maxStatusLines {
			lines = append(lines[:maxStatusLines], fmt.Sprintf("... and %d more changed files", len(lines)-maxStatusLines))
		}
		sb.WriteString("  Status (git status --porcelain):\n")
		for _, line := range lines {
			sb.WriteString("    " + line + "\n")
		}
	}
	if log := gitOutput(dir, "log", "--oneline", "-5"); log != "" {
		sb.WriteString("  Recent commits:\n")
		for _, line := range strings.Split(strings.TrimRight(log, "\n"), "\n") {
			sb.WriteString("    " + line + "\n")
		}
	}
	return sb.String()
}

// gitOutput runs one read-only git query in dir and returns its trimmed
// stdout, or "" on any failure. Never returns an error: git context is a
// nicety, not a dependency.
func gitOutput(dir string, args ...string) string {
	if dir == "" {
		return ""
	}
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// buildInstructionsLayer is deliberately EMPTY: project instructions are
// injected as a high-weight user message by the agent loop (see
// agent/usercontext.go), following the reference agent's design — burying
// them in the system prompt both lowers their instructional weight and puts
// volatile content inside the cacheable prompt prefix.
func (c *PromptComposer) buildInstructionsLayer() {}

// instructionFileNames lists the instruction files honored by the platform,
// in priority order. AUTOMERGENT.md is our native memory file; AGENTS.md and
// CLAUDE.md are honored so existing tooling keeps working.
var instructionFileNames = []string{"AUTOMERGENT.md", "AGENTS.md", "CLAUDE.md"}

// ProjectInstructions walks up from dir collecting every instruction file
// (AUTOMERGENT.md, AGENTS.md, CLAUDE.md) it finds, nearest first. Returns ""
// when none exist.
func ProjectInstructions(dir string) string {
	return findAndReadInstructions(dir, instructionFileNames)
}

// GlobalInstructions returns the user-level instructions file, or "".
func GlobalInstructions() string {
	homeDir, err := os.UserHomeDir()
	if err != nil || homeDir == "" {
		return ""
	}
	content, err := os.ReadFile(filepath.Join(homeDir, ".config", "automergent", "AGENTS.md"))
	if err != nil {
		return ""
	}
	return string(content)
}

// findAndReadInstructions walks up the directory tree looking for instruction files.
func findAndReadInstructions(startDir string, filenames []string) string {
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
		sb.WriteString(fmt.Sprintf("- **%s**: %s\n", skill.Name(), skill.Description()))
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
	
	// Default phase prompts live in phases/*.txt (Mission/Rules/Exit).
	if prompt := PhasePrompt(c.phase); prompt != "" {
		c.layers[shared.LayerPhase] = []string{"## Phase: " + string(c.phase) + "\n" + prompt}
	}
}

// buildBehavioralLayer adds behavioral prompts, grouped by dimension
// (phase discipline / context hygiene / verification / safety). See
// behavioral.go.
func (c *PromptComposer) buildBehavioralLayer() {
	var extras []string
	if c.agent != nil {
		extras = c.agent.BehavioralPrompts
	}
	if content := RenderBehavioralPrompts(c.phase, extras); content != "" {
		c.layers[shared.LayerBehavioral] = []string{"## Behavioral Rules\n" + content}
	}
}

// buildToolLayer adds per-tool guidance for the tools offered in this phase.
// Two sources stack: the live registry's ToolMeta documentation (primary —
// tools document themselves) and the static fallback table for tools that
// have not adopted Meta() (tool_prompts.go). Agent ToolPrompts override the
// fallback entries.
func (c *PromptComposer) buildToolLayer() {
	var overrides map[string]shared.ToolPromptConfig
	if c.agent != nil {
		overrides = c.agent.ToolPrompts
	}
	offered := c.getAvailableTools()

	var parts []string
	if c.registry != nil {
		if sections := RenderToolSectionsFor(c.registry, c.model.Name, offered); sections != "" {
			parts = append(parts, sections)
		}
	}
	if fallback := RenderToolPromptsFromRegistry(c.registry, offered, overrides); fallback != "" {
		parts = append(parts, "## Tool Guidance\n"+fallback)
	}
	if len(parts) > 0 {
		c.layers[shared.LayerTool] = parts
	}
}

// getAvailableTools returns the list of available tools for the current phase/agent.
func (c *PromptComposer) getAvailableTools() []string {
	if c.agent != nil && c.agent.PhaseTools != nil {
		if tools, ok := c.agent.PhaseTools[c.phase]; ok {
			return tools
		}
	}
	
	phaseTools := map[shared.AgentPhase][]string{
		shared.PhaseInit:    {"bash", "read_file", "glob", "grep"},                              // classify only, no task
		shared.PhaseExplore: {"glob", "grep", "read_file", "bash"},                              // read-only exploration, NO task
		shared.PhasePlan:    {"read_file", "write_file", "bash", "task"},                        // planning + can delegate research
		shared.PhaseBuild:   {"edit_file", "bash", "write_file", "read_file", "task", "glob", "grep"}, // full + todo
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