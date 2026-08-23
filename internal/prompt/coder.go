package prompt

import (
	"fmt"
	"strings"
)

// TaskPrompts generates execution prompts for the agent's own task work.
// There is no separate coder persona: the same Automergent agent plans,
// implements, and verifies using these prompts.
type TaskPrompts struct {
	config *PromptConfig
}

// NewTaskPrompts creates a new task execution prompt generator.
func NewTaskPrompts(config *PromptConfig) *TaskPrompts {
	if config == nil {
		config = DefaultPromptConfig()
	}
	return &TaskPrompts{config: config}
}

// CoderPrompts is a deprecated alias kept for call-site compatibility.
type CoderPrompts = TaskPrompts

// BuildCoderInitPrompt creates the initial prompt for the coder agent.
func (c *TaskPrompts) BuildTaskInitPrompt(ctx *TurnContext, categorized *CategorizedRequest) *PromptPart {
	var sb strings.Builder

	if categorized != nil {
		sb.WriteString("Task: ")
		sb.WriteString(categorized.OriginalPrompt)
		sb.WriteString("\n\n")

		sb.WriteString("Category: ")
		sb.WriteString(string(categorized.Category))
		sb.WriteString("\n")
		sb.WriteString("Complexity: ")
		sb.WriteString(string(categorized.Complexity))
		sb.WriteString("\n")
		sb.WriteString("Strategy: ")
		sb.WriteString(string(categorized.Strategy))
		sb.WriteString("\n\n")
	} else {
		sb.WriteString("Task: (intent-based system)\n\n")
	}

	sb.WriteString("Working Directory: ")
	sb.WriteString(ctx.WorkingDir)
	sb.WriteString("\n\n")

	if len(ctx.Files) > 0 {
		sb.WriteString("Relevant Files:\n")
		for _, f := range ctx.Files {
			sb.WriteString("  - ")
			sb.WriteString(f)
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	if len(ctx.CodeSnippets) > 0 {
		sb.WriteString("Code Snippets:\n")
		for path, code := range ctx.CodeSnippets {
			sb.WriteString(fmt.Sprintf("\n--- %s ---\n%s\n", path, code))
		}
		sb.WriteString("\n")
	}

	if len(ctx.Constraints) > 0 {
		sb.WriteString("Constraints:\n")
		for _, constraint := range ctx.Constraints {
			sb.WriteString("  - ")
			sb.WriteString(constraint)
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	if len(ctx.TodoItems) > 0 {
		sb.WriteString("Todo Items:\n")
		for _, todo := range ctx.TodoItems {
			status := " "
			if todo.Status == TodoStatusInProgress {
				status = ">"
			} else if todo.Status == TodoStatusCompleted {
				status = "x"
			}
			sb.WriteString(fmt.Sprintf("  [%s] %s (priority: %d)\n", status, todo.Description, todo.Priority))
			if len(todo.Tools) > 0 {
				sb.WriteString(fmt.Sprintf("      Tools: %s\n", strings.Join(todo.Tools, ", ")))
			}
			if len(todo.ContextKeys) > 0 {
				sb.WriteString(fmt.Sprintf("      Context: %s\n", strings.Join(todo.ContextKeys, ", ")))
			}
			if todo.InjectLater && !todo.Injected {
				sb.WriteString("      [Context to be injected later]\n")
			}
		}
		sb.WriteString("\n")
	}

	if len(ctx.SharedContext) > 0 {
		sb.WriteString("Shared Context:\n")
		for key, value := range ctx.SharedContext {
			sb.WriteString(fmt.Sprintf("  %s: %s\n", key, value))
		}
		sb.WriteString("\n")
	}

	tools := ToolSetModerate
	if categorized != nil {
		tools = categorized.AllowedTools
	}

	sb.WriteString("Allowed Tools: ")
	sb.WriteString(string(tools))
	sb.WriteString("\n\n")

	sb.WriteString("Instructions:\n")
	sb.WriteString("1. Follow existing code style and patterns in the codebase\n")
	sb.WriteString("2. Write clean, maintainable, well-structured code\n")
	sb.WriteString("3. Include necessary imports and dependencies\n")
	sb.WriteString("4. Handle errors appropriately\n")
	sb.WriteString("5. Update todo status as you progress\n")
	sb.WriteString("6. Request additional context if needed (use context keys)\n")
	sb.WriteString("7. Report progress concisely in the conversation log as you go\n")
	sb.WriteString("8. Verify your work with tests or builds before declaring completion\n")

	return &PromptPart{
		Stage:    StageTaskInit,
		Content:  sb.String(),
		Tools:    tools,
		Metadata: map[string]any{"turn_context": ctx, "categorized": categorized},
	}
}

// BuildWorkflowPlanPrompt creates a prompt for planning the workflow.
func (c *TaskPrompts) BuildWorkflowPlanPrompt(ctx *TurnContext, categorized *CategorizedRequest) *PromptPart {
	var sb strings.Builder

	sb.WriteString("Plan the execution workflow for this task.\n\n")

	if categorized != nil {
		sb.WriteString("Task: ")
		sb.WriteString(categorized.OriginalPrompt)
		sb.WriteString("\n")
		sb.WriteString("Strategy: ")
		sb.WriteString(string(categorized.Strategy))
		sb.WriteString("\n\n")
	} else {
		sb.WriteString("Task: (intent-based system)\n\n")
	}

	sb.WriteString("Current Todos:\n")
	for _, todo := range ctx.TodoItems {
		sb.WriteString(fmt.Sprintf("  - [%s] %s\n", todo.Status, todo.Description))
	}
	sb.WriteString("\n")

	sb.WriteString("Determine:\n")
	sb.WriteString("1. Which todos can run in parallel?\n")
	sb.WriteString("2. Which todos have dependencies?\n")
	sb.WriteString("3. Should sub-agents be dispatched for parallel work?\n")
	sb.WriteString("4. What's the optimal execution order?\n")
	sb.WriteString("5. What context needs to be shared between parallel tasks?\n\n")

	sb.WriteString("Provide a workflow plan with phases. Each phase can have parallel tasks.")

	return &PromptPart{
		Stage:    StageWorkflowPlan,
		Content:  sb.String(),
		Tools:    ToolSetContextOnly,
		Metadata: map[string]any{"turn_context": ctx, "categorized": categorized},
	}
}

// BuildExecutionPrompt creates a prompt for executing a specific todo.
func (c *TaskPrompts) BuildExecutionPrompt(ctx *TurnContext, todo *TodoItem, categorized *CategorizedRequest) *PromptPart {
	var sb strings.Builder

	sb.WriteString("Execute this todo item.\n\n")
	sb.WriteString("Todo: ")
	sb.WriteString(todo.Description)
	sb.WriteString("\n")
	sb.WriteString("Priority: ")
	sb.WriteString(fmt.Sprintf("%d", todo.Priority))
	sb.WriteString("\n")

	if len(todo.Tools) > 0 {
		sb.WriteString("Allowed Tools: ")
		sb.WriteString(strings.Join(todo.Tools, ", "))
		sb.WriteString("\n")
	} else if categorized != nil {
		sb.WriteString("Allowed Tools: ")
		sb.WriteString(string(categorized.AllowedTools))
		sb.WriteString("\n")
	} else {
		sb.WriteString("Allowed Tools: moderate\n")
	}

	if len(todo.ContextKeys) > 0 {
		sb.WriteString("Required Context Keys: ")
		sb.WriteString(strings.Join(todo.ContextKeys, ", "))
		sb.WriteString("\n")
	}

	if todo.InjectLater && !todo.Injected {
		sb.WriteString("\n[NOTE: This todo has deferred context injection. Request context when needed.]\n")
	}

	sb.WriteString("\nCurrent Working Directory: ")
	sb.WriteString(ctx.WorkingDir)
	sb.WriteString("\n\n")

	sb.WriteString("Execute the task. Update todo status to in_progress when starting, completed when done.")

	tools := c.determineToolSet(todo.Tools, ToolSetModerate)
	if categorized != nil {
		tools = c.determineToolSet(todo.Tools, categorized.AllowedTools)
	}

	return &PromptPart{
		Stage:    StageExecution,
		Content:  sb.String(),
		Tools:    tools,
		Metadata: map[string]any{"todo": todo, "turn_context": ctx},
	}
}

// BuildTodoInjectPrompt creates a prompt for injecting deferred context.
func (c *TaskPrompts) BuildTodoInjectPrompt(ctx *TurnContext, todo *TodoItem, contextKey string, contextValue string) *PromptPart {
	var sb strings.Builder

	sb.WriteString("Injecting deferred context for todo: ")
	sb.WriteString(todo.Description)
	sb.WriteString("\n\n")
	sb.WriteString("Context Key: ")
	sb.WriteString(contextKey)
	sb.WriteString("\n")
	sb.WriteString("Context Value:\n")
	sb.WriteString(contextValue)
	sb.WriteString("\n\n")
	sb.WriteString("This context was marked for deferred injection. Use it now to complete the todo.")
	sb.WriteString("\nMark the todo as having injected context.")

	return &PromptPart{
		Stage:    StageTodoInject,
		Content:  sb.String(),
		Tools:    ToolSetContextOnly,
		Metadata: map[string]any{"todo": todo, "context_key": contextKey, "context_value": contextValue},
	}
}

// BuildParallelDispatchPrompt creates a prompt for dispatching parallel sub-agents.
func (c *TaskPrompts) BuildParallelDispatchPrompt(ctx *TurnContext, todos []TodoItem, sharedContext map[string]string) *PromptPart {
	var sb strings.Builder

	sb.WriteString("Dispatch parallel sub-agents for these independent todos:\n\n")

	for i, todo := range todos {
		sb.WriteString(fmt.Sprintf("Agent %d: %s\n", i+1, todo.Description))
		if len(todo.Tools) > 0 {
			sb.WriteString(fmt.Sprintf("  Tools: %s\n", strings.Join(todo.Tools, ", ")))
		}
		if len(todo.ContextKeys) > 0 {
			sb.WriteString(fmt.Sprintf("  Context: %s\n", strings.Join(todo.ContextKeys, ", ")))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("Shared Context (available to all agents):\n")
	for key, value := range sharedContext {
		sb.WriteString(fmt.Sprintf("  %s: %s\n", key, value))
	}
	sb.WriteString("\n")

	sb.WriteString("Each agent works independently. Results will be synthesized after completion.")

	return &PromptPart{
		Stage:    StageExecution,
		Content:  sb.String(),
		Tools:    ToolSetFull,
		Metadata: map[string]any{"todos": todos, "shared_context": sharedContext},
	}
}

// BuildModerateTaskPrompt creates a prompt for moderate complexity tasks done by coder directly.
func (c *TaskPrompts) BuildModerateTaskPrompt(ctx *TurnContext, categorized *CategorizedRequest) *PromptPart {
	var sb strings.Builder

	sb.WriteString("Execute this moderate complexity task directly.\n\n")

	if categorized != nil {
		sb.WriteString("Task: ")
		sb.WriteString(categorized.OriginalPrompt)
		sb.WriteString("\n\n")
	} else {
		sb.WriteString("Task: (intent-based system)\n\n")
	}

	sb.WriteString("Working Directory: ")
	sb.WriteString(ctx.WorkingDir)
	sb.WriteString("\n\n")

	sb.WriteString("Available Tools: edit, write, bash, sql, search, read_file, read_many_files, read_file_lines\n\n")

	if len(ctx.Files) > 0 {
		sb.WriteString("Relevant Files:\n")
		for _, f := range ctx.Files {
			sb.WriteString("  - ")
			sb.WriteString(f)
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	if len(ctx.CodeSnippets) > 0 {
		sb.WriteString("Code Context:\n")
		for path, code := range ctx.CodeSnippets {
			sb.WriteString(fmt.Sprintf("\n--- %s ---\n%s\n", path, code))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("Instructions:\n")
	sb.WriteString("1. Use the appropriate tools for the task\n")
	sb.WriteString("2. Read files before editing\n")
	sb.WriteString("3. Write clean, idiomatic code\n")
	sb.WriteString("4. Run tests if applicable\n")
	sb.WriteString("5. Do not over-engineer - complete the task efficiently\n")

	return &PromptPart{
		Stage:    StageExecution,
		Content:  sb.String(),
		Tools:    ToolSetModerate,
		Metadata: map[string]any{"turn_context": ctx, "categorized": categorized},
	}
}

func (c *TaskPrompts) determineToolSet(todoTools []string, defaultTools ToolSet) ToolSet {
	if len(todoTools) == 0 {
		return defaultTools
	}

	// Check if any advanced tools are needed
	advancedTools := map[string]bool{
		"edit": true, "write": true, "bash": true, "sql": true,
		"search": true, "read_file": true, "read_many_files": true, "read_file_lines": true,
	}

	hasAdvanced := false
	for _, t := range todoTools {
		if advancedTools[t] {
			hasAdvanced = true
			break
		}
	}

	if hasAdvanced {
		return ToolSetModerate
	}
	return ToolSetBasic
}
