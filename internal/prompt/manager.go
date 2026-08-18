package prompt

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// PromptManager is the main entry point for the prompt system.
// It orchestrates all prompt generators and manages the prompt lifecycle.
type PromptManager struct {
	config         *PromptConfig
	categorizer    *Categorizer
	assistantPrompts *AssistantPrompts
	coderPrompts   *CoderPrompts
	contextPrompts *ContextPrompts
	workflowPrompts *WorkflowPrompts
	toolPrompts    *ToolPrompts

	// State
	assistantContext *AssistantContext
	coderContext     *CoderContext
	currentRequest   *CategorizedRequest
	promptHistory    []PromptPart
	stashedContexts  []ContextStash
	mu               sync.RWMutex
}

// NewPromptManager creates a new prompt manager.
func NewPromptManager(config *PromptConfig) *PromptManager {
	if config == nil {
		config = DefaultPromptConfig()
	}

	pm := &PromptManager{
		config:          config,
		categorizer:     NewCategorizer(config),
		assistantPrompts: NewAssistantPrompts(config),
		coderPrompts:    NewCoderPrompts(config),
		contextPrompts:  NewContextPrompts(config),
		workflowPrompts: NewWorkflowPrompts(config),
		toolPrompts:     NewToolPrompts(config),
		assistantContext: &AssistantContext{
			ConversationHistory: []Message{},
			UserPreferences:     make(map[string]string),
			StashedContexts:     []ContextStash{},
		},
		promptHistory:   []PromptPart{},
		stashedContexts: []ContextStash{},
	}

	return pm
}

// ProcessUserMessage processes a user message and returns the appropriate prompt parts.
func (pm *PromptManager) ProcessUserMessage(ctx context.Context, userMessage string, workingDir string, availableFiles []string) ([]PromptPart, error) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	// Add to conversation history
	pm.assistantContext.ConversationHistory = append(pm.assistantContext.ConversationHistory, Message{
		Role:      "user",
		Content:   userMessage,
		Timestamp: time.Now(),
	})

	// If this is the first message or no current request, categorize
	if pm.currentRequest == nil {
		return pm.startNewRequest(userMessage, workingDir, availableFiles)
	}

	// Otherwise, continue existing request
	return pm.continueRequest(userMessage)
}

// startNewRequest starts processing a new user request.
func (pm *PromptManager) startNewRequest(userMessage, workingDir string, availableFiles []string) ([]PromptPart, error) {
	var parts []PromptPart

	// Stage 1: Initial thinking
	thinkingPrompt := *pm.assistantPrompts.BuildInitialThinkingPrompt(userMessage)
	parts = append(parts, thinkingPrompt)
	pm.promptHistory = append(pm.promptHistory, thinkingPrompt)

	// Stage 2: Categorization
	categorizePrompt := *pm.assistantPrompts.BuildCategorizationPrompt(userMessage, workingDir, availableFiles)
	parts = append(parts, categorizePrompt)
	pm.promptHistory = append(pm.promptHistory, categorizePrompt)

	// Note: In actual use, the categorization would be done by the LLM
	// For now, we do it programmatically
	categorized := pm.categorizer.Categorize(userMessage, workingDir, availableFiles)
	pm.currentRequest = categorized

	// Initialize coder context if needed
	if categorized.RequiresCoder {
		pm.coderContext = &CoderContext{
			WorkingDir:        workingDir,
			Files:             categorized.WorkingAreas,
			CodeSnippets:      make(map[string]string),
			Constraints:       []string{},
			TodoItems:         categorized.TodoItems,
			SharedContext:     make(map[string]string),
			ParentAssistantID: "assistant-main",
		}
	}

	// Stage 3: Task definition
	taskDefPrompt := *pm.assistantPrompts.BuildTaskDefinitionPrompt(categorized)
	parts = append(parts, taskDefPrompt)
	pm.promptHistory = append(pm.promptHistory, taskDefPrompt)

	// Stage 4: Initialize coder if needed
	if categorized.RequiresCoder {
		coderInitPrompt := *pm.coderPrompts.BuildCoderInitPrompt(pm.coderContext, categorized)
		parts = append(parts, coderInitPrompt)
		pm.promptHistory = append(pm.promptHistory, coderInitPrompt)
	}

	// Stage 5: Workflow plan or direct execution
	switch categorized.Strategy {
	case StrategyParallel, StrategyTodoWalkthrough:
		workflowPrompt := *pm.coderPrompts.BuildWorkflowPlanPrompt(pm.coderContext, categorized)
		parts = append(parts, workflowPrompt)
		pm.promptHistory = append(pm.promptHistory, workflowPrompt)
	case StrategyCoderAgent:
		// For coder agent, workflow plan is part of coder init
	case StrategyDirect:
		if categorized.Complexity == ComplexitySimple {
			simplePrompt := *pm.assistantPrompts.BuildSimpleTaskPrompt(categorized)
			parts = append(parts, simplePrompt)
			pm.promptHistory = append(pm.promptHistory, simplePrompt)
		} else {
			moderatePrompt := *pm.coderPrompts.BuildModerateTaskPrompt(pm.coderContext, categorized)
			parts = append(parts, moderatePrompt)
			pm.promptHistory = append(pm.promptHistory, moderatePrompt)
		}
	}

	return parts, nil
}

// continueRequest continues processing an existing request.
func (pm *PromptManager) continueRequest(userMessage string) ([]PromptPart, error) {
	// Add to conversation history
	pm.assistantContext.ConversationHistory = append(pm.assistantContext.ConversationHistory, Message{
		Role:      "user",
		Content:   userMessage,
		Timestamp: time.Now(),
	})

	// For now, treat as new context management or clarification
	// In practice, this would be more sophisticated
	contextPrompt := *pm.assistantPrompts.BuildContextManagementPrompt(
		ContextActionShare,
		pm.assistantContext,
		userMessage,
	)
	return []PromptPart{contextPrompt}, nil
}

// GetNextTodoPrompt gets the prompt for the next todo item.
func (pm *PromptManager) GetNextTodoPrompt() *PromptPart {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	if pm.coderContext == nil || len(pm.coderContext.TodoItems) == 0 {
		return nil
	}

	// Find next pending todo
	for i, todo := range pm.coderContext.TodoItems {
		if todo.Status == TodoStatusPending {
			// Check dependencies
			depsMet := true
			for _, depID := range todo.Dependencies {
				found := false
				for _, t := range pm.coderContext.TodoItems {
					if t.ID == depID && t.Status == TodoStatusCompleted {
						found = true
						break
					}
				}
				if !found {
					depsMet = false
					break
				}
			}
			if depsMet {
				// Mark as in progress
				pm.coderContext.TodoItems[i].Status = TodoStatusInProgress
				prompt := pm.coderPrompts.BuildExecutionPrompt(pm.coderContext, &pm.coderContext.TodoItems[i], pm.currentRequest)
				if prompt != nil {
					promptCopy := *prompt
					return &promptCopy
				}
			}
		}
	}

	return nil
}

// InjectTodoContext injects deferred context for a todo.
func (pm *PromptManager) InjectTodoContext(todoID, contextKey, contextValue string) *PromptPart {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if pm.coderContext == nil {
		return nil
	}

	for i, todo := range pm.coderContext.TodoItems {
		if todo.ID == todoID && todo.InjectLater && !todo.Injected {
			pm.coderContext.TodoItems[i].Injected = true
			// Add to shared context
			pm.coderContext.SharedContext[contextKey] = contextValue
			prompt := pm.coderPrompts.BuildTodoInjectPrompt(pm.coderContext, &pm.coderContext.TodoItems[i], contextKey, contextValue)
			if prompt != nil {
				promptCopy := *prompt
				return &promptCopy
			}
		}
	}

	return nil
}

// DispatchParallelTodos dispatches multiple todos in parallel.
func (pm *PromptManager) DispatchParallelTodos(todoIDs []string) *PromptPart {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	if pm.coderContext == nil {
		return nil
	}

	var todos []TodoItem
	for _, id := range todoIDs {
		for _, todo := range pm.coderContext.TodoItems {
			if todo.ID == id && todo.Status == TodoStatusPending {
				todos = append(todos, todo)
				break
			}
		}
	}

	if len(todos) == 0 {
		return nil
	}

	prompt := pm.coderPrompts.BuildParallelDispatchPrompt(pm.coderContext, todos, pm.coderContext.SharedContext)
	if prompt != nil {
		promptCopy := *prompt
		return &promptCopy
	}
	return nil
}

// CompleteTodo marks a todo as completed.
func (pm *PromptManager) CompleteTodo(todoID string, result string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if pm.coderContext == nil {
		return
	}

	for i, todo := range pm.coderContext.TodoItems {
		if todo.ID == todoID {
			pm.coderContext.TodoItems[i].Status = TodoStatusCompleted
			break
		}
	}
}

// StashContext stashes the current context with a summary.
func (pm *PromptManager) StashContext(reason string) *PromptPart {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	stashPrompt := pm.contextPrompts.BuildStashPrompt(pm.assistantContext, reason)
	if stashPrompt != nil {
		promptCopy := *stashPrompt
		pm.promptHistory = append(pm.promptHistory, promptCopy)
		return &promptCopy
	}
	return nil
}

// SaveStash saves a stashed context with the provided summary.
func (pm *PromptManager) SaveStash(summary string, tags []string) *ContextStash {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	return pm.saveStashLocked(summary, tags)
}

// saveStashLocked saves a stash assuming the lock is already held.
func (pm *PromptManager) saveStashLocked(summary string, tags []string) *ContextStash {
	stash := ContextStash{
		ID:          fmt.Sprintf("stash-%d", time.Now().UnixNano()),
		Summary:     summary,
		FullContext: pm.serializeContext(),
		CreatedAt:   time.Now(),
		Tags:        tags,
		Resumable:   true,
	}

	pm.stashedContexts = append(pm.stashedContexts, stash)
	pm.assistantContext.StashedContexts = pm.stashedContexts

	return &stash
}

// ResumeContext resumes from a stashed context.
func (pm *PromptManager) ResumeContext(stashID string) *PromptPart {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	var stash *ContextStash
	for _, s := range pm.stashedContexts {
		if s.ID == stashID {
			stash = &s
			break
		}
	}

	if stash == nil {
		return nil
	}

	resumePrompt := pm.contextPrompts.BuildResumeContextPrompt(stash, pm.assistantContext)
	if resumePrompt != nil {
		promptCopy := *resumePrompt
		pm.promptHistory = append(pm.promptHistory, promptCopy)
		return &promptCopy
	}
	return nil
}

// ApplyStash applies a stashed context to the current context.
func (pm *PromptManager) ApplyStash(stashID string, merge bool) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	var stash *ContextStash
	for _, s := range pm.stashedContexts {
		if s.ID == stashID {
			stash = &s
			break
		}
	}

	if stash == nil {
		return fmt.Errorf("stash not found: %s", stashID)
	}

	// In a real implementation, this would deserialize and apply the context
	// For now, we just note it
	if !merge {
		// Replace context
		pm.assistantContext.ConversationHistory = []Message{}
	}

	return nil
}

// ShareContextWithCoder shares context from assistant to coder.
func (pm *PromptManager) ShareContextWithCoder(shareSpec string) *PromptPart {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	if pm.coderContext == nil {
		return nil
	}

	sharePrompt := pm.contextPrompts.BuildShareContextPrompt(pm.assistantContext, pm.coderContext, shareSpec)
	if sharePrompt != nil {
		promptCopy := *sharePrompt
		pm.promptHistory = append(pm.promptHistory, promptCopy)
		return &promptCopy
	}
	return nil
}

// ApplySharedContext applies shared context to coder context.
func (pm *PromptManager) ApplySharedContext(keyValues map[string]string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if pm.coderContext == nil {
		return
	}

	for k, v := range keyValues {
		pm.coderContext.SharedContext[k] = v
	}
}

// GetCoderContext returns the current coder context.
func (pm *PromptManager) GetCoderContext() *CoderContext {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.coderContext
}

// GetAssistantContext returns the current assistant context.
func (pm *PromptManager) GetAssistantContext() *AssistantContext {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.assistantContext
}

// CoderPrompts returns the coder prompt generator.
func (pm *PromptManager) CoderPrompts() *CoderPrompts {
	return pm.coderPrompts
}

// AssistantPrompts returns the assistant prompt generator.
func (pm *PromptManager) AssistantPrompts() *AssistantPrompts {
	return pm.assistantPrompts
}

// GetCurrentRequest returns the current categorized request.
func (pm *PromptManager) GetCurrentRequest() *CategorizedRequest {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.currentRequest
}

// CompleteRequest marks the current request as complete and returns a response prompt.
func (pm *PromptManager) CompleteRequest(result string) *PromptPart {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if pm.currentRequest == nil {
		return nil
	}

	responsePrompt := pm.assistantPrompts.BuildUserResponsePrompt(pm.currentRequest, result)
	if responsePrompt != nil {
		promptCopy := *responsePrompt
		pm.promptHistory = append(pm.promptHistory, promptCopy)

		// Add to conversation history
		pm.assistantContext.ConversationHistory = append(pm.assistantContext.ConversationHistory, Message{
			Role:      "assistant",
			Content:   result,
			Timestamp: time.Now(),
		})

		// Clear current request
		pm.currentRequest = nil
		pm.coderContext = nil

		return &promptCopy
	}
	return nil
}

// GetPromptHistory returns the history of prompt parts.
func (pm *PromptManager) GetPromptHistory() []PromptPart {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.promptHistory
}

// GetStashedContexts returns all stashed contexts.
func (pm *PromptManager) GetStashedContexts() []ContextStash {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.stashedContexts
}

// DeleteStash deletes a stashed context.
func (pm *PromptManager) DeleteStash(stashID string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	for i, s := range pm.stashedContexts {
		if s.ID == stashID {
			pm.stashedContexts = append(pm.stashedContexts[:i], pm.stashedContexts[i+1:]...)
			pm.assistantContext.StashedContexts = pm.stashedContexts
			return nil
		}
	}
	return fmt.Errorf("stash not found: %s", stashID)
}

// CreateNewContext creates a new fresh context.
func (pm *PromptManager) CreateNewContext(initialPrompt string, inheritPrefs bool) *PromptPart {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	newPrompt := pm.contextPrompts.BuildNewContextPrompt(pm.assistantContext, initialPrompt)
	var promptCopy PromptPart
	if newPrompt != nil {
		promptCopy = *newPrompt
		pm.promptHistory = append(pm.promptHistory, promptCopy)
	}

	// Save current context as stash if not empty
	if len(pm.assistantContext.ConversationHistory) > 0 {
		pm.saveStashLocked("Auto-stash before new context", []string{"auto", "pre-new-context"})
	}

	// Reset assistant context
	prefs := make(map[string]string)
	if inheritPrefs {
		for k, v := range pm.assistantContext.UserPreferences {
			prefs[k] = v
		}
	}

	pm.assistantContext = &AssistantContext{
		ConversationHistory: []Message{},
		UserPreferences:     prefs,
		StashedContexts:     pm.stashedContexts,
	}

	pm.currentRequest = nil
	pm.coderContext = nil

	if newPrompt != nil {
		return &promptCopy
	}
	return nil
}

// SetUserPreference sets a user preference.
func (pm *PromptManager) SetUserPreference(key, value string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.assistantContext.UserPreferences[key] = value
}

// GetUserPreference gets a user preference.
func (pm *PromptManager) GetUserPreference(key string) (string, bool) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	val, ok := pm.assistantContext.UserPreferences[key]
	return val, ok
}

// serializeContext serializes the current context for stashing.
func (pm *PromptManager) serializeContext() string {
	// In a real implementation, this would serialize to JSON
	return fmt.Sprintf("Context with %d messages, task: %v", len(pm.assistantContext.ConversationHistory), pm.currentRequest)
}