package prompt

import (
	"fmt"
	"strings"
)

// WorkflowPrompts generates prompts for workflow management.
type WorkflowPrompts struct {
	config *PromptConfig
}

// NewWorkflowPrompts creates a new workflow prompt generator.
func NewWorkflowPrompts(config *PromptConfig) *WorkflowPrompts {
	if config == nil {
		config = DefaultPromptConfig()
	}
	return &WorkflowPrompts{config: config}
}

// BuildTodoWalkthroughPrompt creates a prompt for walking through todos.
func (w *WorkflowPrompts) BuildTodoWalkthroughPrompt(todos []TodoItem, currentIndex int) *PromptPart {
	var sb strings.Builder

	sb.WriteString("TODO WALKTHROUGH\n\n")

	sb.WriteString("All Todos:\n")
	for i, todo := range todos {
		marker := "  "
		if i == currentIndex {
			marker = "→ "
		}
		status := " "
		switch todo.Status {
		case TodoStatusCompleted:
			status = "x"
		case TodoStatusInProgress:
			status = ">"
		case TodoStatusBlocked:
			status = "!"
		}
		sb.WriteString(fmt.Sprintf("%s[%s] %s (priority: %d)\n", marker, status, todo.Description, todo.Priority))
		if len(todo.Tools) > 0 {
			sb.WriteString(fmt.Sprintf("      Tools: %s\n", strings.Join(todo.Tools, ", ")))
		}
		if len(todo.Dependencies) > 0 {
			sb.WriteString(fmt.Sprintf("      Depends on: %s\n", strings.Join(todo.Dependencies, ", ")))
		}
		if len(todo.ContextKeys) > 0 {
			sb.WriteString(fmt.Sprintf("      Context: %s\n", strings.Join(todo.ContextKeys, ", ")))
		}
		if todo.InjectLater && !todo.Injected {
			sb.WriteString("      [Deferred context injection pending]\n")
		}
	}
	sb.WriteString("\n")

	if currentIndex < len(todos) {
		current := todos[currentIndex]
		sb.WriteString("Current Todo: ")
		sb.WriteString(current.Description)
		sb.WriteString("\n\n")

		sb.WriteString("Decide:\n")
		sb.WriteString("1. Can this todo be started now? (dependencies met?)\n")
		sb.WriteString("2. What tools are needed?\n")
		sb.WriteString("3. What context is required? (request if needed)\n")
		sb.WriteString("4. Can any subsequent todos run in parallel after this?\n")
		sb.WriteString("5. Should a sub-agent be dispatched for this todo?\n\n")
		sb.WriteString("Provide execution decision.")
	} else {
		sb.WriteString("All todos completed. Ready for synthesis.")
	}

	return &PromptPart{
		Stage:    StageWorkflowPlan,
		Content:  sb.String(),
		Tools:    ToolSetContextOnly,
		Metadata: map[string]any{"todos": todos, "current_index": currentIndex},
	}
}

// BuildParallelDispatchDecisionPrompt creates a prompt for deciding parallel dispatch.
func (w *WorkflowPrompts) BuildParallelDispatchDecisionPrompt(todos []TodoItem, completedIndices []int) *PromptPart {
	var sb strings.Builder

	sb.WriteString("PARALLEL DISPATCH DECISION\n\n")

	var pendingTodos []TodoItem
	for _, todo := range todos {
		if todo.Status == TodoStatusPending {
			// Check if dependencies are met
			depsMet := true
			for _, dep := range todo.Dependencies {
				found := false
				for _, ci := range completedIndices {
					if todos[ci].ID == dep {
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
				pendingTodos = append(pendingTodos, todo)
			}
		}
	}

	sb.WriteString(fmt.Sprintf("Ready for parallel execution: %d todos\n\n", len(pendingTodos)))

	for i, todo := range pendingTodos {
		sb.WriteString(fmt.Sprintf("Todo %d: %s\n", i+1, todo.Description))
		sb.WriteString(fmt.Sprintf("  Priority: %d\n", todo.Priority))
		if len(todo.Tools) > 0 {
			sb.WriteString(fmt.Sprintf("  Tools: %s\n", strings.Join(todo.Tools, ", ")))
		}
		if len(todo.ContextKeys) > 0 {
			sb.WriteString(fmt.Sprintf("  Context: %s\n", strings.Join(todo.ContextKeys, ", ")))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("Decide:\n")
	sb.WriteString("1. Which todos can run in parallel? (group by independence)\n")
	sb.WriteString("2. Which should run sequentially?\n")
	sb.WriteString("3. What shared context is needed?\n")
	sb.WriteString("4. Should sub-agents be created? (for true parallelism)\n")
	sb.WriteString("5. What's the optimal batch size?\n\n")
	sb.WriteString("Provide dispatch plan with phases.")

	return &PromptPart{
		Stage:    StageWorkflowPlan,
		Content:  sb.String(),
		Tools:    ToolSetContextOnly,
		Metadata: map[string]any{"ready_todos": pendingTodos, "completed": completedIndices},
	}
}

// BuildSynthesisPrompt creates a prompt for synthesizing parallel results.
func (w *WorkflowPrompts) BuildSynthesisPrompt(results []TodoResult, originalTask *CategorizedRequest) *PromptPart {
	var sb strings.Builder

	sb.WriteString("SYNTHESIZE PARALLEL RESULTS\n\n")
	sb.WriteString(fmt.Sprintf("Original Task: %s\n", originalTask.OriginalPrompt))
	sb.WriteString(fmt.Sprintf("Category: %s\n", originalTask.Category))
	sb.WriteString(fmt.Sprintf("Number of Results: %d\n\n", len(results)))

	for i, result := range results {
		sb.WriteString(fmt.Sprintf("Result %d (Todo: %s):\n", i+1, result.TodoID))
		sb.WriteString(fmt.Sprintf("  Success: %v\n", result.Success))
		sb.WriteString(fmt.Sprintf("  Output: %s\n", truncate(result.Output, 500)))
		if len(result.Artifacts) > 0 {
			sb.WriteString(fmt.Sprintf("  Artifacts: %d\n", len(result.Artifacts)))
		}
		if len(result.Errors) > 0 {
			sb.WriteString(fmt.Sprintf("  Errors: %s\n", strings.Join(result.Errors, "; ")))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("Synthesize into a coherent result:\n")
	sb.WriteString("1. Combine outputs logically\n")
	sb.WriteString("2. Resolve any conflicts\n")
	sb.WriteString("3. Verify completeness against original request\n")
	sb.WriteString("4. Identify any remaining work\n\n")
	sb.WriteString("Provide final synthesized output.")

	return &PromptPart{
		Stage:    StageCompletion,
		Content:  sb.String(),
		Tools:    ToolSetContextOnly,
		Metadata: map[string]any{"results": results, "original_task": originalTask},
	}
}

// TodoResult represents the result of a todo execution.
type TodoResult struct {
	TodoID    string
	Success   bool
	Output    string
	Artifacts []Artifact
	Errors    []string
	Warnings  []string
}

// Artifact represents a produced artifact (reused from coordinator).
type Artifact struct {
	Type     string
	Path     string
	Content  string
	Language string
	Metadata map[string]any
}

// BuildModerateDirectPrompt creates a prompt for direct moderate task execution.
func (w *WorkflowPrompts) BuildModerateDirectPrompt(categorized *CategorizedRequest, coderContext *CoderContext) *PromptPart {
	var sb strings.Builder

	sb.WriteString("DIRECT MODERATE TASK EXECUTION\n\n")
	sb.WriteString("Task: ")
	sb.WriteString(categorized.OriginalPrompt)
	sb.WriteString("\n")
	sb.WriteString("Complexity: ")
	sb.WriteString(string(categorized.Complexity))
	sb.WriteString("\n")
	sb.WriteString("Strategy: Direct execution by coder\n\n")

	sb.WriteString("Available Tools: edit, write, bash, sql, search, read_file, read_many_files, read_file_lines\n\n")

	if len(coderContext.Files) > 0 {
		sb.WriteString("Relevant Files:\n")
		for _, f := range coderContext.Files {
			sb.WriteString("  - ")
			sb.WriteString(f)
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	if len(coderContext.CodeSnippets) > 0 {
		sb.WriteString("Code Context:\n")
		for path, code := range coderContext.CodeSnippets {
			sb.WriteString(fmt.Sprintf("\n--- %s ---\n%s\n", path, code))
		}
		sb.WriteString("\n")
	}

	if len(coderContext.TodoItems) > 0 {
		sb.WriteString("Todo Items:\n")
		for _, todo := range coderContext.TodoItems {
			sb.WriteString(fmt.Sprintf("  - [%s] %s\n", todo.Status, todo.Description))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("Instructions:\n")
	sb.WriteString("1. Execute the task using appropriate tools\n")
	sb.WriteString("2. Read files before editing\n")
	sb.WriteString("3. Write clean, idiomatic code following project patterns\n")
	sb.WriteString("4. Run tests/validation if applicable\n")
	sb.WriteString("5. Complete efficiently - this is moderate complexity, not a full project\n")
	sb.WriteString("6. Update todo status as you progress\n\n")
	sb.WriteString("Begin execution.")

	return &PromptPart{
		Stage:    StageExecution,
		Content:  sb.String(),
		Tools:    ToolSetModerate,
		Metadata: map[string]any{"categorized": categorized, "coder_context": coderContext},
	}
}

// BuildToolSelectionPrompt creates a prompt for selecting tools for a task.
func (w *WorkflowPrompts) BuildToolSelectionPrompt(taskDescription string, availableTools []string) *PromptPart {
	var sb strings.Builder

	sb.WriteString("TOOL SELECTION\n\n")
	sb.WriteString("Task: ")
	sb.WriteString(taskDescription)
	sb.WriteString("\n\n")
	sb.WriteString("Available Tools:\n")
	for _, tool := range availableTools {
		sb.WriteString("  - ")
		sb.WriteString(tool)
		sb.WriteString("\n")
	}
	sb.WriteString("\n")

	sb.WriteString("Select the minimal set of tools needed for this task. ")
	sb.WriteString("Prefer simpler tools when possible. ")
	sb.WriteString("Consider: read before write, search before read_many, etc.\n\n")
	sb.WriteString("Provide selected tools with brief justification.")

	return &PromptPart{
		Stage:    StageExecution,
		Content:  sb.String(),
		Tools:    ToolSetContextOnly,
		Metadata: map[string]any{"available_tools": availableTools},
	}
}

// BuildContextShareDecisionPrompt creates a prompt for deciding context sharing.
func (w *WorkflowPrompts) BuildContextShareDecisionPrompt(sourceContext, targetContext map[string]string, shareRules []string) *PromptPart {
	var sb strings.Builder

	sb.WriteString("CONTEXT SHARING DECISION\n\n")

	sb.WriteString("Source Context Keys:\n")
	for k, v := range sourceContext {
		sb.WriteString(fmt.Sprintf("  %s: %s\n", k, truncate(v, 100)))
	}
	sb.WriteString("\n")

	sb.WriteString("Target Context Keys (current):\n")
	for k, v := range targetContext {
		sb.WriteString(fmt.Sprintf("  %s: %s\n", k, truncate(v, 100)))
	}
	sb.WriteString("\n")

	sb.WriteString("Sharing Rules:\n")
	for _, rule := range shareRules {
		sb.WriteString("  - ")
		sb.WriteString(rule)
		sb.WriteString("\n")
	}
	sb.WriteString("\n")

	sb.WriteString("Decide what to share:\n")
	sb.WriteString("1. Which keys from source should be copied to target?\n")
	sb.WriteString("2. Should any target keys be overridden?\n")
	sb.WriteString("3. Are there any transformations needed?\n")
	sb.WriteString("4. What about deferred injection items?\n\n")
	sb.WriteString("Provide sharing plan with specific key mappings.")

	return &PromptPart{
		Stage:    StageContextManage,
		Content:  sb.String(),
		Tools:    ToolSetContextOnly,
		Metadata: map[string]any{"source_keys": keys(sourceContext), "target_keys": keys(targetContext)},
	}
}

func keys(m map[string]string) []string {
	result := make([]string, 0, len(m))
	for k := range m {
		result = append(result, k)
	}
	return result
}