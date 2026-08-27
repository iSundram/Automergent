package custom

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/iSundram/Automergent/internal/agent/agentdef"
)

// ValidationResult holds the result of validating an agent definition.
type ValidationResult struct {
	Valid   bool
	Errors  []string
	Warnings []string
}

// ValidateAgentDefinition validates an agent definition.
func ValidateAgentDefinition(def *agentdef.AgentDefinition) *ValidationResult {
	vr := &ValidationResult{Valid: true}

	// Name validation
	if def.Name == "" {
		vr.Errors = append(vr.Errors, "name is required")
		vr.Valid = false
	} else if len(def.Name) < 3 {
		vr.Errors = append(vr.Errors, "name must be at least 3 characters")
		vr.Valid = false
	} else if len(def.Name) > 50 {
		vr.Errors = append(vr.Errors, "name must be at most 50 characters")
		vr.Valid = false
	} else if !regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9\-]*[a-zA-Z0-9])?$`).MatchString(def.Name) {
		vr.Errors = append(vr.Errors, "name must be alphanumeric with hyphens, starting and ending with alphanumeric")
		vr.Valid = false
	}

	// Description validation
	if def.Description == "" {
		vr.Warnings = append(vr.Warnings, "description is recommended")
	} else if len(def.Description) < 10 {
		vr.Warnings = append(vr.Warnings, "description should be at least 10 characters")
	}

	// WhenToUse validation
	if def.WhenToUse == "" {
		vr.Warnings = append(vr.Warnings, "when_to_use is recommended for automatic agent selection")
	} else if len(def.WhenToUse) < 10 {
		vr.Warnings = append(vr.Warnings, "when_to_use should be at least 10 characters for effective matching")
	}

	// System prompt validation
	if def.SystemPrompt == "" {
		vr.Errors = append(vr.Errors, "system_prompt (body) is required")
		vr.Valid = false
	} else if len(def.SystemPrompt) < 20 {
		vr.Errors = append(vr.Errors, "system_prompt should be at least 20 characters")
		vr.Valid = false
	}

	// Tools validation
	validTools := map[string]bool{
		"read": true, "write": true, "edit": true, "bash": true,
		"grep": true, "glob": true, "websearch": true, "webfetch": true,
		"task": true, "read_agent": true, "list_agents": true, "agent_control": true,
		"context_compact": true, "context_bucket": true, "context_memory": true, "context_transcript": true,
	}
	if def.Tools != nil {
		for _, tool := range def.Tools {
			if !validTools[tool] {
				vr.Warnings = append(vr.Warnings, fmt.Sprintf("unknown tool: %s", tool))
			}
		}
	}

	// Model validation (optional)
	if def.Model != "" && !strings.Contains(def.Model, "-") {
		vr.Warnings = append(vr.Warnings, "model should be in format like 'gemini-3-flash'")
	}

	// Effort validation
	switch def.Effort {
	case "", agentdef.EffortLow, agentdef.EffortMedium, agentdef.EffortHigh:
		// valid
	default:
		vr.Errors = append(vr.Errors, fmt.Sprintf("invalid effort: %s (must be low, medium, or high)", def.Effort))
		vr.Valid = false
	}

	// Memory scope validation
	switch def.MemoryScope {
	case "", agentdef.MemoryScopeGlobal, agentdef.MemoryScopeProject, agentdef.MemoryScopeNone:
		// valid
	default:
		vr.Errors = append(vr.Errors, fmt.Sprintf("invalid memory_scope: %s", def.MemoryScope))
		vr.Valid = false
	}

	return vr
}

// ValidateAgentFile validates an agent file and returns detailed results.
func ValidateAgentFile(path string) *ValidationResult {
	af, err := ParseAgentFile(path)
	if err != nil {
		return &ValidationResult{
			Valid:  false,
			Errors: []string{fmt.Sprintf("failed to parse: %v", err)},
		}
	}

	if !af.IsValid {
		return &ValidationResult{
			Valid:  false,
			Errors: af.Errors,
		}
	}

	return ValidateAgentDefinition(af.Def)
}

// ValidateAgentName checks if a name is valid for an agent.
func ValidateAgentName(name string) error {
	if name == "" {
		return fmt.Errorf("name is required")
	}
	if len(name) < 3 {
		return fmt.Errorf("name must be at least 3 characters")
	}
	if len(name) > 50 {
		return fmt.Errorf("name must be at most 50 characters")
	}
	if !regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9\-]*[a-zA-Z0-9])?$`).MatchString(name) {
		return fmt.Errorf("name must be alphanumeric with hyphens")
	}
	return nil
}

// ValidateAgentTools checks if the tool list is valid.
func ValidateAgentTools(tools []string) error {
	if tools == nil {
		return nil // nil means all tools
	}

	validTools := map[string]bool{
		"read": true, "write": true, "edit": true, "bash": true,
		"grep": true, "glob": true, "websearch": true, "webfetch": true,
		"task": true, "read_agent": true, "list_agents": true, "agent_control": true,
		"context_compact": true, "context_bucket": true, "context_memory": true, "context_transcript": true,
	}

	for _, tool := range tools {
		if !validTools[tool] {
			return fmt.Errorf("unknown tool: %s", tool)
		}
	}
	return nil
}
