package prompt

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	contextpkg "github.com/iSundram/Automergent/internal/context"
	"github.com/iSundram/Automergent/internal/shared"
	"github.com/iSundram/Automergent/internal/taskstate"
)

// PromptManager is the main entry point for the prompt system.
// It orchestrates all prompt generators and manages the prompt lifecycle.
type PromptManager struct {
	config           *PromptConfig
	intentIdentifier *LLMIntentIdentifier
	initExecutor     *InitPhaseExecutor
	taskPlanner      *LLMTaskPlanner
	assistantPrompts *AssistantPrompts
	coderPrompts     *CoderPrompts
	contextPrompts   *ContextPrompts
	workflowPrompts  *WorkflowPrompts
	toolPrompts      *ToolPrompts
	contextSelector  *ContextSelector
	toolExecutor     ToolExecutor

	// State — one unified context (no assistant/coder split)
	turnCtx            *TurnContext
	currentIntentSet   *shared.IntentSet
	currentInitPhase   *InitPhase
	currentInitResults *shared.InitResults
	currentTasks       []shared.TaskSpec
	promptHistory      []PromptPart
	stashedContexts    []ContextStash
	workingDir         string
	progress           func(stage, detail string)
	actionObserver     func(shared.InitActionEvent)
	mu                 sync.RWMutex

	// Task state store for tools
	taskState *taskstate.Store
}

// NewPromptManager creates a new prompt manager with an LLM client for intent identification.
func NewPromptManager(config *PromptConfig, contextManager *contextpkg.Manager, workingDir string, llmClient LLMClient, toolExecutor ToolExecutor) *PromptManager {
	if config == nil {
		config = DefaultPromptConfig()
	}
	if llmClient == nil {
		panic("LLMClient is required for intent identification")
	}

	pm := &PromptManager{
		config:           config,
		intentIdentifier: NewLLMIntentIdentifier(config, llmClient),
		initExecutor:     NewInitPhaseExecutor(config),
		taskPlanner:      NewLLMTaskPlanner(config, llmClient),
		assistantPrompts: NewAssistantPrompts(config),
		coderPrompts:     NewTaskPrompts(config),
		contextPrompts:   NewContextPrompts(config),
		workflowPrompts:  NewWorkflowPrompts(config),
		toolPrompts:      NewToolPrompts(config),
		toolExecutor:     toolExecutor,
		turnCtx: &TurnContext{
			ConversationHistory: []Message{},
			UserPreferences:     make(map[string]string),
			StashedContexts:     []ContextStash{},
		},
		promptHistory:   []PromptPart{},
		stashedContexts: []ContextStash{},
		workingDir:      workingDir,
		taskState:       taskstate.NewStore(),
	}

	if contextManager != nil {
		pm.contextSelector = NewContextSelector(contextManager, workingDir, config)
	}

	return pm
}

// ProcessUserMessage processes a user message and returns the appropriate prompt parts.
func (pm *PromptManager) ProcessUserMessage(ctx context.Context, userMessage string, workingDir string, availableFiles []string) ([]PromptPart, error) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	pm.turnCtx.ConversationHistory = append(pm.turnCtx.ConversationHistory, Message{
		Role:      "user",
		Content:   userMessage,
		Timestamp: time.Now(),
	})

	if pm.currentIntentSet == nil {
		return pm.startNewIntentFlow(ctx, userMessage, workingDir, availableFiles)
	}

	if isNewTaskMessage(userMessage) {
		pm.saveStashLocked("Auto-stash before independent task", []string{"auto", "independent-task"})
		pm.currentIntentSet = nil
		pm.currentInitPhase = nil
		pm.currentInitResults = nil
		pm.currentTasks = nil
		pm.taskState = taskstate.NewStore()
		pm.turnCtx = nil
		return pm.startNewIntentFlow(ctx, userMessage, workingDir, availableFiles)
	}

	return pm.continueRequest(userMessage)
}

// startNewIntentFlow starts processing a new user request using intent identification.
func (pm *PromptManager) startNewIntentFlow(ctx context.Context, userMessage, workingDir string, availableFiles []string) ([]PromptPart, error) {
	var parts []PromptPart

	thinkingPrompt := *pm.assistantPrompts.BuildInitialThinkingPrompt(userMessage)
	parts = append(parts, thinkingPrompt)
	pm.promptHistory = append(pm.promptHistory, thinkingPrompt)

	intentSet := pm.intentIdentifier.IdentifyIntents(ctx, userMessage, workingDir, availableFiles)
	pm.currentIntentSet = intentSet

	var intentNames []string
	for _, intent := range intentSet.Intents {
		intentNames = append(intentNames, string(intent.Type))
	}
	pm.notifyProgress("Intents", strings.Join(intentNames, ", "))

	pm.taskState.SetIntentAndInit(intentSet, nil)

	pm.initExecutor.OnStart = func(action InitAction) {
		pm.notifyAction(shared.InitActionEvent{
			RawTool: action.Tool,
			Tool:    NormalizeInitTool(action.Tool),
			Target:  action.Target,
			Running: true,
		})
	}

	pm.initExecutor.OnAction = func(action InitAction, execErr error, duration time.Duration) {
		evt := shared.InitActionEvent{
			RawTool:  action.Tool,
			Tool:     NormalizeInitTool(action.Tool),
			Target:   action.Target,
			Duration: duration,
		}
		if execErr != nil {
			evt.Failed = true
			evt.Err = execErr.Error()
			pm.notifyAction(evt)
			return
		}
		switch action.Tool {
		case "glob", "grep":
			lines := strings.Count(strings.TrimSpace(action.Result), "\n") + 1
			if strings.TrimSpace(action.Result) == "" {
				lines = 0
			}
			evt.Summary = fmt.Sprintf("%d results", lines)
		case "read":
			evt.Summary = fmt.Sprintf("%d chars", len(action.Result))
		default:
			if out := strings.TrimSpace(action.Result); out != "" {
				if idx := strings.IndexByte(out, '\n'); idx >= 0 {
					out = out[:idx]
				}
				evt.Summary = out
			}
		}
		pm.notifyAction(evt)
	}

	intentPrompt := pm.buildIntentIdentificationPrompt(intentSet)
	parts = append(parts, *intentPrompt)
	pm.promptHistory = append(pm.promptHistory, *intentPrompt)

	if intentSet.RequiresInit && intentSet.InitPhase != nil {
		pm.currentInitPhase = intentSet.InitPhase
		initPrompt := BuildInitPrompt(intentSet.InitPhase)
		parts = append(parts, *initPrompt)
		pm.promptHistory = append(pm.promptHistory, *initPrompt)

		initResults, err := pm.initExecutor.Execute(ctx, intentSet.InitPhase, workingDir, pm.toolExecutor)
		if err != nil {
			return parts, fmt.Errorf("init phase failed: %w", err)
		}
		pm.currentInitResults = initResults
		pm.taskState.SetIntentAndInit(intentSet, initResults)

		tasks, err := pm.taskPlanner.PlanTasks(ctx, intentSet, initResults)
		if err != nil {
			return parts, fmt.Errorf("task planning failed: %w", err)
		}
		pm.currentTasks = tasks
		pm.taskState.SetPlan(tasks, pm.convertTasksToTodos(tasks))
		pm.notifyTaskPlan(tasks)

		taskPrompt := BuildInitResultsPrompt(intentSet.InitPhase, intentSet)
		parts = append(parts, *taskPrompt)
		pm.promptHistory = append(pm.promptHistory, *taskPrompt)

		taskPrompts := BuildTaskPrompts(tasks, initResults)
		parts = append(parts, taskPrompts...)
		for _, tp := range taskPrompts {
			pm.promptHistory = append(pm.promptHistory, tp)
		}

		pm.attachTodos(workingDir, initResults.FilesFound, initResults.CodeSnippets, tasks)
	} else {
		tasks, err := pm.taskPlanner.PlanTasks(ctx, intentSet, &shared.InitResults{})
		if err != nil {
			return parts, fmt.Errorf("task planning failed: %w", err)
		}
		pm.currentTasks = tasks
		pm.taskState.SetPlan(tasks, pm.convertTasksToTodos(tasks))
		pm.taskState.SetIntentAndInit(intentSet, &shared.InitResults{})
		pm.notifyTaskPlan(tasks)

		taskPrompts := BuildTaskPrompts(tasks, &shared.InitResults{})
		parts = append(parts, taskPrompts...)
		for _, tp := range taskPrompts {
			pm.promptHistory = append(pm.promptHistory, tp)
		}

		pm.attachTodos(workingDir, nil, nil, tasks)
	}

	return parts, nil
}

// buildIntentIdentificationPrompt creates a prompt showing identified intents.
func (pm *PromptManager) buildIntentIdentificationPrompt(intentSet *shared.IntentSet) *PromptPart {
	var sb strings.Builder

	sb.WriteString("INTENT IDENTIFICATION RESULTS\n\n")
	sb.WriteString("Original Request: ")
	sb.WriteString(intentSet.OriginalPrompt)
	sb.WriteString("\n\n")
	sb.WriteString("Identified Intents:\n")

	for _, intent := range intentSet.Intents {
		sb.WriteString(fmt.Sprintf("- %s (priority: %d, confidence: %.0f%%)", intent.Type, intent.Priority, intent.Confidence*100))
		if len(intent.Dependencies) > 0 {
			sb.WriteString(fmt.Sprintf(", depends on: %s", strings.Join(intent.Dependencies, ", ")))
		}
		sb.WriteString("\n")
		sb.WriteString(fmt.Sprintf("  Parameters: %v\n", intent.Parameters))
		sb.WriteString(fmt.Sprintf("  Text: %s\n\n", intent.RawText))
	}

	sb.WriteString(fmt.Sprintf("Requires Initialization: %v\n", intentSet.RequiresInit))

	return &PromptPart{
		Stage:    StageCategorization,
		Content:  sb.String(),
		Tools:    ToolSetContextOnly,
		Metadata: map[string]any{"intent_set": intentSet},
	}
}

// requiresTodoTracking reports whether the plan contains implementation work
// that should be tracked as todos on the unified turn context.
func (pm *PromptManager) requiresTodoTracking(tasks []shared.TaskSpec) bool {
	for _, task := range tasks {
		switch task.Role {
		case "coder", "implementer", "tester":
			return true
		}
	}
	return false
}

// attachTodos loads a planned task list onto the unified turn context without
// discarding conversation state. This replaces the old behavior of spinning up
// a separate "coder" context: one persona, one context, one loop.
func (pm *PromptManager) attachTodos(workingDir string, files []string, snippets map[string]string, tasks []shared.TaskSpec) {
	if pm.turnCtx == nil {
		pm.turnCtx = &TurnContext{}
	}
	if workingDir != "" {
		pm.turnCtx.WorkingDir = workingDir
	}
	if len(files) > 0 {
		pm.turnCtx.Files = files
	}
	if len(snippets) > 0 {
		pm.turnCtx.CodeSnippets = snippets
	}
	if pm.turnCtx.Constraints == nil {
		pm.turnCtx.Constraints = []string{}
	}
	pm.turnCtx.TodoItems = pm.convertTasksToTodos(tasks)
	if pm.turnCtx.SharedContext == nil {
		pm.turnCtx.SharedContext = make(map[string]string)
	}
}

func (pm *PromptManager) convertTasksToTodos(tasks []shared.TaskSpec) []shared.TodoItem {
	var todos []shared.TodoItem
	for _, task := range tasks {
		todos = append(todos, shared.TodoItem{
			ID:           task.ID,
			Description:  task.Description,
			Status:       shared.TodoStatusPending,
			Priority:     task.Priority,
			Dependencies: task.Dependencies,
			Tools:        task.Tools,
			ContextKeys:  []string{},
			InjectLater:  false,
			Injected:     false,
		})
	}
	return todos
}

// continueRequest continues processing an existing request.
func (pm *PromptManager) continueRequest(userMessage string) ([]PromptPart, error) {
	if pm.currentIntentSet == nil {
		return []PromptPart{}, nil
	}

	contextPrompt := *pm.assistantPrompts.BuildContextManagementPrompt(
		ContextActionShare,
		pm.turnCtx,
		userMessage,
	)
	return []PromptPart{contextPrompt}, nil
}

// GetNextTodoPrompt gets the prompt for the next todo item.
func (pm *PromptManager) GetNextTodoPrompt() *PromptPart {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if pm.turnCtx == nil || len(pm.turnCtx.TodoItems) == 0 {
		return nil
	}

	for i, todo := range pm.turnCtx.TodoItems {
		if todo.Status == shared.TodoStatusPending {
			depsMet := true
			for _, depID := range todo.Dependencies {
				found := false
				for _, t := range pm.turnCtx.TodoItems {
					if t.ID == depID && t.Status == shared.TodoStatusCompleted {
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
				pm.turnCtx.TodoItems[i].Status = shared.TodoStatusInProgress
				prompt := pm.coderPrompts.BuildExecutionPrompt(pm.turnCtx, &pm.turnCtx.TodoItems[i], nil)
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

	if pm.turnCtx == nil {
		return nil
	}

	for i, todo := range pm.turnCtx.TodoItems {
		if todo.ID == todoID && todo.InjectLater && !todo.Injected {
			pm.turnCtx.TodoItems[i].Injected = true
			pm.turnCtx.SharedContext[contextKey] = contextValue
			prompt := pm.coderPrompts.BuildTodoInjectPrompt(pm.turnCtx, &pm.turnCtx.TodoItems[i], contextKey, contextValue)
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

	if pm.turnCtx == nil {
		return nil
	}

	var todos []shared.TodoItem
	for _, id := range todoIDs {
		for _, todo := range pm.turnCtx.TodoItems {
			if todo.ID == id && todo.Status == shared.TodoStatusPending {
				todos = append(todos, todo)
				break
			}
		}
	}

	if len(todos) == 0 {
		return nil
	}

	prompt := pm.coderPrompts.BuildParallelDispatchPrompt(pm.turnCtx, todos, pm.turnCtx.SharedContext)
	if prompt != nil {
		promptCopy := *prompt
		if promptCopy.Metadata == nil {
			promptCopy.Metadata = map[string]any{}
		}
		promptCopy.Metadata["ephemeral"] = true
		return &promptCopy
	}
	return nil
}

// CompleteTodo marks a todo as completed.
func (pm *PromptManager) CompleteTodo(todoID string, result string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if pm.turnCtx == nil {
		return
	}

	for i, todo := range pm.turnCtx.TodoItems {
		if todo.ID == todoID {
			pm.turnCtx.TodoItems[i].Status = shared.TodoStatusCompleted
			break
		}
	}
}

// CompleteActiveTodo advances the scoped workflow after a tool batch.
func (pm *PromptManager) CompleteActiveTodo(success bool) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	if pm.turnCtx == nil {
		return
	}
	status := shared.TodoStatusCompleted
	if !success {
		status = shared.TodoStatusBlocked
	}
	for i := range pm.turnCtx.TodoItems {
		if pm.turnCtx.TodoItems[i].Status != shared.TodoStatusInProgress {
			continue
		}
		pm.turnCtx.TodoItems[i].Status = status
		return
	}
}

// StashContext stashes the current context with a summary.
func (pm *PromptManager) StashContext(reason string) *PromptPart {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	stashPrompt := pm.contextPrompts.BuildStashPrompt(pm.turnCtx, reason)
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
	pm.turnCtx.StashedContexts = pm.stashedContexts

	return &stash
}

// ResumeContext resumes from a stashed context.
func (pm *PromptManager) ResumeContext(stashID string) *PromptPart {
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
		return nil
	}

	resumePrompt := pm.contextPrompts.BuildResumeContextPrompt(stash, pm.turnCtx)
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

	if !merge {
		pm.turnCtx.ConversationHistory = []Message{}
	}

	return nil
}

// ShareContextWithCoder shares context from assistant to coder.
func (pm *PromptManager) ShareContextWithCoder(shareSpec string) *PromptPart {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if pm.turnCtx == nil {
		return nil
	}

	sharePrompt := pm.contextPrompts.BuildShareContextPrompt(pm.turnCtx, shareSpec)
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

	if pm.turnCtx == nil {
		return
	}

	for k, v := range keyValues {
		pm.turnCtx.SharedContext[k] = v
	}
}

// GetTurnContext returns the unified turn context.
func (pm *PromptManager) GetTurnContext() *TurnContext {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.turnCtx
}

// TaskPrompts returns the task execution prompt generator.
func (pm *PromptManager) TaskPrompts() *TaskPrompts {
	return pm.coderPrompts
}

// AssistantPrompts returns the assistant prompt generator.
func (pm *PromptManager) AssistantPrompts() *AssistantPrompts {
	return pm.assistantPrompts
}

// GetCurrentRequest returns the current categorized request (legacy compatibility).
func (pm *PromptManager) GetCurrentRequest() *CategorizedRequest {
	return nil
}

// GetCurrentIntentSet returns the current intent set.
func (pm *PromptManager) GetCurrentIntentSet() *shared.IntentSet {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.currentIntentSet
}

// GetCurrentTasks returns the current generated tasks.
func (pm *PromptManager) GetCurrentTasks() []shared.TaskSpec {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.currentTasks
}

// CompleteRequest marks the current request as complete and returns a response prompt.
func (pm *PromptManager) CompleteRequest(result string) *PromptPart {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if pm.currentIntentSet == nil {
		return nil
	}

	responsePrompt := &PromptPart{
		Stage:    StageCompletion,
		Content:  "Task completed: " + result,
		Tools:    ToolSetContextOnly,
		Metadata: map[string]any{"result": result},
	}

	if responsePrompt != nil {
		promptCopy := *responsePrompt
		pm.promptHistory = append(pm.promptHistory, promptCopy)

		pm.turnCtx.ConversationHistory = append(pm.turnCtx.ConversationHistory, Message{
			Role:      "assistant",
			Content:   result,
			Timestamp: time.Now(),
		})

		pm.currentIntentSet = nil
		pm.currentInitPhase = nil
		pm.currentInitResults = nil
		pm.currentTasks = nil
		pm.taskState = taskstate.NewStore()
		pm.turnCtx = nil
		pm.cleanupEphemeralPromptsLocked()

		return &promptCopy
	}
	return nil
}

// CleanupEphemeralPrompts removes injected/staged prompt parts after their
// consuming task completes.
func (pm *PromptManager) CleanupEphemeralPrompts() {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.cleanupEphemeralPromptsLocked()
}

func (pm *PromptManager) cleanupEphemeralPromptsLocked() {
	kept := pm.promptHistory[:0]
	for _, part := range pm.promptHistory {
		if part.Metadata != nil {
			if ephemeral, ok := part.Metadata["ephemeral"].(bool); ok && ephemeral {
				continue
			}
		}
		kept = append(kept, part)
	}
	pm.promptHistory = kept
}

func isNewTaskMessage(message string) bool {
	return containsAny(strings.ToLower(message), []string{"new task", "separate task", "unrelated task", "start a fresh", "forget the previous"})
}

func containsAny(s string, substrings []string) bool {
	for _, sub := range substrings {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// SetLLMClient swaps the LLM client used by every internal call (intent
// identification, task planning). Called when the user switches model or
// provider: the staged pipelines cache adapters, and without this swap they
// keep calling the retired provider after /model or /provider.
func (pm *PromptManager) SetLLMClient(llmClient LLMClient) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	if llmClient == nil {
		return
	}
	pm.intentIdentifier = NewLLMIntentIdentifier(pm.config, llmClient)
	pm.taskPlanner = NewLLMTaskPlanner(pm.config, llmClient)
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

// SetProgress registers a callback invoked at each pipeline stage for UI visibility.
func (pm *PromptManager) SetProgress(fn func(stage, detail string)) {
	pm.progress = fn
}

// SetActionObserver registers a callback receiving structured init-phase tool
// events so the UI can render them as native tool-call log entries.
func (pm *PromptManager) SetActionObserver(fn func(shared.InitActionEvent)) {
	pm.actionObserver = fn
}

func (pm *PromptManager) notifyProgress(stage, detail string) {
	if pm.progress != nil {
		pm.progress(stage, detail)
	}
}

func (pm *PromptManager) notifyAction(evt shared.InitActionEvent) {
	if pm.actionObserver != nil {
		pm.actionObserver(evt)
	}
}

// NormalizeInitTool maps pipeline-side init tool names onto the native
// registry tool names the UI renders cards for.
func NormalizeInitTool(tool string) string {
	switch tool {
	case "read":
		return "read_file"
	default:
		return tool
	}
}

func (pm *PromptManager) notifyTaskPlan(tasks []shared.TaskSpec) {
	if pm.progress == nil || len(tasks) == 0 {
		return
	}
	var sb strings.Builder
	for i, t := range tasks {
		deps := ""
		if len(t.Dependencies) > 0 {
			deps = fmt.Sprintf(" (after %s)", strings.Join(t.Dependencies, ", "))
		}
		sb.WriteString(fmt.Sprintf("%d. [%s] %s%s\n", i+1, t.Type, t.Description, deps))
	}
	pm.notifyProgress("Plan", strings.TrimSpace(sb.String()))
}

// GetInitResults returns the init phase results.
func (pm *PromptManager) GetInitResults() *shared.InitResults {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.currentInitResults
}

// DeleteStash deletes a stashed context.
func (pm *PromptManager) DeleteStash(stashID string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	for i, s := range pm.stashedContexts {
		if s.ID == stashID {
			pm.stashedContexts = append(pm.stashedContexts[:i], pm.stashedContexts[i+1:]...)
			pm.turnCtx.StashedContexts = pm.stashedContexts
			return nil
		}
	}
	return fmt.Errorf("stash not found: %s", stashID)
}

// CreateNewContext creates a new fresh context.
func (pm *PromptManager) CreateNewContext(initialPrompt string, inheritPrefs bool) *PromptPart {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	newPrompt := pm.contextPrompts.BuildNewContextPrompt(pm.turnCtx, initialPrompt)
	var promptCopy PromptPart
	if newPrompt != nil {
		promptCopy = *newPrompt
		pm.promptHistory = append(pm.promptHistory, promptCopy)
	}

	if len(pm.turnCtx.ConversationHistory) > 0 {
		pm.saveStashLocked("Auto-stash before new context", []string{"auto", "pre-new-context"})
	}

	prefs := make(map[string]string)
	if inheritPrefs {
		for k, v := range pm.turnCtx.UserPreferences {
			prefs[k] = v
		}
	}

	pm.turnCtx = &TurnContext{
		ConversationHistory: []Message{},
		UserPreferences:     prefs,
		StashedContexts:     pm.stashedContexts,
	}
	pm.currentIntentSet = nil
	pm.currentInitPhase = nil
	pm.currentInitResults = nil
	pm.currentTasks = nil

	if newPrompt != nil {
		return &promptCopy
	}
	return nil
}

// SetUserPreference sets a user preference.
func (pm *PromptManager) SetUserPreference(key, value string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.turnCtx.UserPreferences[key] = value
}

// GetUserPreference gets a user preference.
func (pm *PromptManager) GetUserPreference(key string) (string, bool) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	val, ok := pm.turnCtx.UserPreferences[key]
	return val, ok
}

// serializeContext serializes the current context for stashing.
func (pm *PromptManager) serializeContext() string {
	if pm.currentIntentSet != nil {
		return fmt.Sprintf("Context with %d messages, intents: %d", len(pm.turnCtx.ConversationHistory), len(pm.currentIntentSet.Intents))
	}
	return fmt.Sprintf("Context with %d messages", len(pm.turnCtx.ConversationHistory))
}

// GetSelectedContext returns the context selected for the current tasks using the context selector.
func (pm *PromptManager) GetSelectedContext(ctx context.Context) (string, error) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	if pm.contextSelector == nil {
		return "", nil
	}

	if pm.currentIntentSet != nil && len(pm.currentTasks) > 0 {
		return pm.contextSelector.SelectContextForTasks(ctx, pm.currentTasks, pm.workingDir, pm.config.MaxTotalTokens)
	}

	return "", nil
}

// GetTaskState returns the task state store.
func (pm *PromptManager) GetTaskState() *taskstate.Store {
	return pm.taskState
}

// NotifyProgress exposes the progress hook externally.
func (pm *PromptManager) NotifyProgress(stage, detail string) {
	pm.notifyProgress(stage, detail)
}
