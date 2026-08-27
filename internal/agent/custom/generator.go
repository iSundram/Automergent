package custom

import (
	"context"
	"fmt"
	"strings"

	"github.com/iSundram/Automergent/internal/agent/agentdef"
)

// GeneratorConfig configures the AI agent generator.
type GeneratorConfig struct {
	Model       string
	MaxTokens   int
	Temperature float64
}

// DefaultGeneratorConfig returns sensible defaults.
func DefaultGeneratorConfig() GeneratorConfig {
	return GeneratorConfig{
		Model:       "gemini-3-flash",
		MaxTokens:   1024,
		Temperature: 0.7,
	}
}

// Generator creates agent definitions using AI.
type Generator struct {
	llm     LLMClient
	config  GeneratorConfig
}

// LLMClient is the interface for LLM calls.
type LLMClient interface {
	Complete(ctx context.Context, prompt string, opts ...any) (string, error)
}

// NewGenerator creates a new agent generator.
func NewGenerator(llm LLMClient, config GeneratorConfig) *Generator {
	return &Generator{
		llm:    llm,
		config: config,
	}
}

// GenerateRequest describes what kind of agent to generate.
type GenerateRequest struct {
	Description string `json:"description"`
	AgentName   string `json:"agent_name,omitempty"`
	Tools       []string `json:"tools,omitempty"`
	Model       string `json:"model,omitempty"`
}

// GenerateResponse contains the generated agent definition.
type GenerateResponse struct {
	Definition *agentdef.AgentDefinition `json:"definition"`
	Explanation string                    `json:"explanation"`
}

const generationPrompt = `You are an AI agent configuration generator. Based on the user's description, create an agent definition.

User description: %s

Respond in this exact JSON format (no markdown, just JSON):
{
  "name": "agent-name-slug",
  "description": "Short description of what this agent does",
  "when_to_use": "When to use this agent (for automatic selection)",
  "system_prompt": "You are a specialized agent that...",
  "tools": ["tool1", "tool2"],
  "model": "",
  "effort": "medium",
  "color": "green"
}

Guidelines:
- name: lowercase, hyphens only, 3-50 chars
- description: concise, 1-2 sentences
- when_to_use: describe trigger conditions
- system_prompt: detailed role instructions (at least 50 words)
- tools: subset of [read, write, edit, bash, grep, glob, websearch, webfetch, task]
- model: leave empty for default, or specify like "gemini-3-flash"
- effort: low, medium, or high
- color: terminal color name`

// Generate creates an agent definition from a natural language description.
func (g *Generator) Generate(ctx context.Context, req GenerateRequest) (*GenerateResponse, error) {
	prompt := fmt.Sprintf(generationPrompt, req.Description)

	result, err := g.llm.Complete(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("LLM generation failed: %w", err)
	}

	// Parse the JSON response
	def, err := parseGeneratedJSON(result)
	if err != nil {
		// Fallback: create a basic definition from the description
		def = fallbackDefinition(req)
	}

	// Apply overrides
	if req.AgentName != "" {
		def.Name = req.AgentName
	}
	if req.Tools != nil {
		def.Tools = req.Tools
	}
	if req.Model != "" {
		def.Model = req.Model
	}

	def.Source = agentdef.SourceUser

	return &GenerateResponse{
		Definition: def,
		Explanation: fmt.Sprintf("Generated agent '%s' based on: %s", def.Name, req.Description),
	}, nil
}

// parseGeneratedJSON parses the LLM's JSON response into an AgentDefinition.
func parseGeneratedJSON(s string) (*agentdef.AgentDefinition, error) {
	// Find JSON in the response
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start < 0 || end < 0 || end <= start {
		return nil, fmt.Errorf("no JSON found in response")
	}

	jsonStr := s[start : end+1]
	def := &agentdef.AgentDefinition{}

	// Simple JSON parsing (without importing encoding/json for this)
	if err := parseSimpleJSON(jsonStr, def); err != nil {
		return nil, err
	}

	return def, nil
}

// parseSimpleJSON does basic JSON parsing for agent definitions.
func parseSimpleJSON(s string, def *agentdef.AgentDefinition) error {
	// Extract fields using string manipulation
	def.Name = extractJSONString(s, "name")
	def.Description = extractJSONString(s, "description")
	def.WhenToUse = extractJSONString(s, "when_to_use")
	def.SystemPrompt = extractJSONString(s, "system_prompt")
	def.Model = extractJSONString(s, "model")
	def.Color = extractJSONString(s, "color")

	// Parse effort
	effort := extractJSONString(s, "effort")
	switch strings.ToLower(effort) {
	case "low":
		def.Effort = agentdef.EffortLow
	case "high":
		def.Effort = agentdef.EffortHigh
	default:
		def.Effort = agentdef.EffortMedium
	}

	// Parse tools array
	toolsStr := extractJSONArray(s, "tools")
	if toolsStr != "" {
		def.Tools = strings.Split(toolsStr, ",")
		for i := range def.Tools {
			def.Tools[i] = strings.Trim(strings.TrimSpace(def.Tools[i]), "\"")
		}
	}

	if def.Name == "" {
		return fmt.Errorf("name is required")
	}
	return nil
}

func extractJSONString(s, key string) string {
	search := fmt.Sprintf(`"%s":`, key)
	idx := strings.Index(s, search)
	if idx < 0 {
		return ""
	}
	rest := s[idx+len(search):]
	rest = strings.TrimSpace(rest)

	if !strings.HasPrefix(rest, `"`) {
		return ""
	}
	rest = rest[1:]

	// Find closing quote (handle escaped quotes)
	end := 0
	for end < len(rest) {
		if rest[end] == '\\' && end+1 < len(rest) {
			end += 2
			continue
		}
		if rest[end] == '"' {
			break
		}
		end++
	}

	return rest[:end]
}

func extractJSONArray(s, key string) string {
	search := fmt.Sprintf(`"%s":`, key)
	idx := strings.Index(s, search)
	if idx < 0 {
		return ""
	}
	rest := s[idx+len(search):]
	rest = strings.TrimSpace(rest)

	if !strings.HasPrefix(rest, `[`) {
		return ""
	}
	end := strings.Index(rest, `]`)
	if end < 0 {
		return ""
	}
	return rest[1:end]
}

// fallbackDefinition creates a basic definition when JSON parsing fails.
func fallbackDefinition(req GenerateRequest) *agentdef.AgentDefinition {
	name := req.AgentName
	if name == "" {
		// Generate a name from description
		name = slugify(req.Description)
		if len(name) > 30 {
			name = name[:30]
		}
	}

	return &agentdef.AgentDefinition{
		Name:         name,
		Description:  req.Description,
		WhenToUse:    req.Description,
		SystemPrompt: fmt.Sprintf("You are a specialized agent for: %s\n\n%s", req.Description, "Follow existing code patterns and report findings concisely."),
		Tools:        req.Tools,
		Model:        req.Model,
		Effort:       agentdef.EffortMedium,
		Source:       agentdef.SourceUser,
		MemoryScope:  agentdef.MemoryScopeProject,
	}
}

func slugify(s string) string {
	s = strings.ToLower(s)
	s = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			return r
		}
		if r == ' ' || r == '-' {
			return '-'
		}
		return -1
	}, s)
	// Collapse multiple hyphens
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	s = strings.Trim(s, "-")
	return s
}
