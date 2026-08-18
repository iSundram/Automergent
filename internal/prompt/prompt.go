package prompt

import (
	"context"
)

// PromptSystem provides a unified interface for all prompt operations.
// This is the main entry point for the prompt system.
type PromptSystem struct {
	Manager *PromptManager
	Config  *PromptConfig
}

// NewPromptSystem creates a new prompt system with default configuration.
func NewPromptSystem() *PromptSystem {
	config := DefaultPromptConfig()
	return &PromptSystem{
		Manager: NewPromptManager(config),
		Config:  config,
	}
}

// NewPromptSystemWithConfig creates a new prompt system with custom configuration.
func NewPromptSystemWithConfig(config *PromptConfig) *PromptSystem {
	return &PromptSystem{
		Manager: NewPromptManager(config),
		Config:  config,
	}
}

// ProcessUserMessage processes a user message through the prompt system.
func (ps *PromptSystem) ProcessUserMessage(ctx context.Context, userMessage, workingDir string, availableFiles []string) ([]PromptPart, error) {
	return ps.Manager.ProcessUserMessage(ctx, userMessage, workingDir, availableFiles)
}

// GetNextAction gets the next prompt part for execution.
func (ps *PromptSystem) GetNextAction() *PromptPart {
	// Try to get next todo
	if todoPrompt := ps.Manager.GetNextTodoPrompt(); todoPrompt != nil {
		return todoPrompt
	}
	return nil
}

// CompleteCurrentTask completes the current task with a result.
func (ps *PromptSystem) CompleteCurrentTask(result string) *PromptPart {
	return ps.Manager.CompleteRequest(result)
}

// StashCurrentContext stashes the current context.
func (ps *PromptSystem) StashCurrentContext(reason string) *PromptPart {
	return ps.Manager.StashContext(reason)
}

// ResumeStashedContext resumes from a stashed context.
func (ps *PromptSystem) ResumeStashedContext(stashID string) *PromptPart {
	return ps.Manager.ResumeContext(stashID)
}

// CreateFreshContext creates a new fresh context.
func (ps *PromptSystem) CreateFreshContext(initialPrompt string) *PromptPart {
	return ps.Manager.CreateNewContext(initialPrompt, true)
}

// GetCoderContext returns the coder context for direct access.
func (ps *PromptSystem) GetCoderContext() *CoderContext {
	return ps.Manager.GetCoderContext()
}

// GetAssistantContext returns the assistant context for direct access.
func (ps *PromptSystem) GetAssistantContext() *AssistantContext {
	return ps.Manager.GetAssistantContext()
}

// GetCurrentRequest returns the current categorized request.
func (ps *PromptSystem) GetCurrentRequest() *CategorizedRequest {
	return ps.Manager.GetCurrentRequest()
}

// GetStashedContexts returns all stashed contexts.
func (ps *PromptSystem) GetStashedContexts() []ContextStash {
	return ps.Manager.GetStashedContexts()
}