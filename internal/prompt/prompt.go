package prompt

import (
	"context"

	contextpkg "github.com/iSundram/Automergent/internal/context"
	"github.com/iSundram/Automergent/internal/shared"
	"github.com/iSundram/Automergent/internal/taskstate"
	"github.com/iSundram/Automergent/internal/tools"
)

// PromptSystem provides a unified interface for all prompt operations.
// This is the main entry point for the prompt system.
type PromptSystem struct {
	Manager    *PromptManager
	Config     *PromptConfig
	workingDir string
}

// NewPromptSystemWithLLM creates a new prompt system with an LLM client for intent identification.
func NewPromptSystemWithLLM(config *PromptConfig, mgr *contextpkg.Manager, workingDir string, llmClient LLMClient, toolRegistry *tools.Registry) *PromptSystem {
	var toolExecutor ToolExecutor
	if toolRegistry != nil {
		toolExecutor = NewToolExecutorAdapter(toolRegistry)
	}
	return &PromptSystem{
		Manager:    NewPromptManager(config, mgr, workingDir, llmClient, toolExecutor),
		Config:     config,
		workingDir: workingDir,
	}
}

// ProcessUserMessage processes a user message through the prompt system.
func (ps *PromptSystem) ProcessUserMessage(ctx context.Context, userMessage, workingDir string, availableFiles []string) ([]PromptPart, error) {
	return ps.Manager.ProcessUserMessage(ctx, userMessage, workingDir, availableFiles)
}

// GetNextAction gets the next prompt part for execution.
func (ps *PromptSystem) GetNextAction() *PromptPart {
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

// SetLLMClient swaps the LLM client used by the internal pipelines (intent
// identification, task planning). Called on model/provider switches so the
// staged calls follow the new provider instead of the one captured at
// startup.
func (ps *PromptSystem) SetLLMClient(llmClient LLMClient) {
	ps.Manager.SetLLMClient(llmClient)
}

// GetTurnContext returns the unified turn context for direct access.
func (ps *PromptSystem) GetTurnContext() *TurnContext {
	return ps.Manager.GetTurnContext()
}

// GetCurrentIntentSet returns the current intent set.
func (ps *PromptSystem) GetCurrentIntentSet() *IntentSet {
	return ps.Manager.GetCurrentIntentSet()
}

// GetCurrentTasks returns the current generated tasks.
func (ps *PromptSystem) GetCurrentTasks() []TaskSpec {
	return ps.Manager.GetCurrentTasks()
}

// GetTaskState returns the task state store for tool access.
func (ps *PromptSystem) GetTaskState() *taskstate.Store {
	return ps.Manager.taskState
}

// GetStashedContexts returns all stashed contexts.
func (ps *PromptSystem) GetStashedContexts() []ContextStash {
	return ps.Manager.GetStashedContexts()
}

// GetInitResults returns the init phase results.
func (ps *PromptSystem) GetInitResults() *InitResults {
	return ps.Manager.GetInitResults()
}

// SetActionObserver registers a callback receiving structured init-phase tool
// events for UI rendering as native log entries.
func (ps *PromptSystem) SetActionObserver(fn func(shared.InitActionEvent)) {
	ps.Manager.SetActionObserver(fn)
}

// GetSelectedContext returns the selected context for the current tasks.
func (ps *PromptSystem) GetSelectedContext(ctx context.Context) (string, error) {
	return ps.Manager.GetSelectedContext(ctx)
}
