package prompt

import (
	"fmt"
	"strings"
)

// AssistantPrompts generates prompts for the assistant (user-facing, not for coding).
type AssistantPrompts struct {
	config *PromptConfig
}

// NewAssistantPrompts creates a new assistant prompt generator.
func NewAssistantPrompts(config *PromptConfig) *AssistantPrompts {
	if config == nil {
		config = DefaultPromptConfig()
	}
	return &AssistantPrompts{config: config}
}

// BuildInitialThinkingPrompt creates the first prompt asking the assistant to think about the request.
func (a *AssistantPrompts) BuildInitialThinkingPrompt(userPrompt string) *PromptPart {
	var sb strings.Builder

	sb.WriteString("Think carefully about the user's request. Do not respond yet - just analyze.\n\n")
	sb.WriteString("User Request: ")
	sb.WriteString(userPrompt)
	sb.WriteString("\n\n")
	sb.WriteString("Consider:\n")
	sb.WriteString("1. What is the user actually asking for?\n")
	sb.WriteString("2. What category does this fall into? (new feature, debug, question, plan, verify, simple task)\n")
	sb.WriteString("3. How complex is this likely to be?\n")
	sb.WriteString("4. What tools or context might be needed?\n")
	sb.WriteString("5. Should this be handled directly, or does it need a coder agent?\n\n")
	sb.WriteString("Provide your analysis in a structured way. Do not execute any tools yet.")

	return &PromptPart{
		Stage:    StageInitialThinking,
		Content:  sb.String(),
		Tools:    ToolSetContextOnly,
		Metadata: map[string]any{"user_prompt": userPrompt},
	}
}

// BuildCategorizationPrompt creates a prompt for categorizing the request.
func (a *AssistantPrompts) BuildCategorizationPrompt(userPrompt string, workingDir string, availableFiles []string) *PromptPart {
	var sb strings.Builder

	sb.WriteString("Based on the user's request, categorize it into ONE of these categories:\n\n")
	sb.WriteString("- new_feature: Adding new functionality, creating components, implementing features\n")
	sb.WriteString("- debug: Fixing bugs, errors, crashes, things not working\n")
	sb.WriteString("- issue_suspect: Investigating potential issues, verifying hypotheses\n")
	sb.WriteString("- user_asking: General questions, explanations, how-to\n")
	sb.WriteString("- plan: Creating plans, designs, architectures, roadmaps\n")
	sb.WriteString("- verify_work: Reviewing, validating, auditing existing work\n")
	sb.WriteString("- direct: Simple direct actions (read, show, list, find)\n")
	sb.WriteString("- simple: Trivial, quick, one-line tasks\n\n")
	sb.WriteString("User Request: ")
	sb.WriteString(userPrompt)
	sb.WriteString("\n\n")
	sb.WriteString("Working Directory: ")
	sb.WriteString(workingDir)
	sb.WriteString("\n")
	sb.WriteString("Available Files: ")
	sb.WriteString(strings.Join(availableFiles, ", "))
	sb.WriteString("\n\n")
	sb.WriteString("Respond with ONLY the category name and a brief reason. Format: CATEGORY|reason")

	return &PromptPart{
		Stage:    StageCategorization,
		Content:  sb.String(),
		Tools:    ToolSetContextOnly,
		Metadata: map[string]any{"user_prompt": userPrompt, "working_dir": workingDir},
	}
}

// BuildTaskDefinitionPrompt creates a prompt for defining the task and todos.
func (a *AssistantPrompts) BuildTaskDefinitionPrompt(categorized *CategorizedRequest) *PromptPart {
	var sb strings.Builder

	sb.WriteString("Define the task breakdown for this request.\n\n")
	sb.WriteString("Category: ")
	sb.WriteString(string(categorized.Category))
	sb.WriteString("\n")
	sb.WriteString("Complexity: ")
	sb.WriteString(string(categorized.Complexity))
	sb.WriteString("\n")
	sb.WriteString("Strategy: ")
	sb.WriteString(string(categorized.Strategy))
	sb.WriteString("\n")
	sb.WriteString("Requires Coder: ")
	sb.WriteString(fmt.Sprintf("%v", categorized.RequiresCoder))
	sb.WriteString("\n\n")
	sb.WriteString("Original Request: ")
	sb.WriteString(categorized.OriginalPrompt)
	sb.WriteString("\n\n")
	sb.WriteString("Working Areas: ")
	sb.WriteString(strings.Join(categorized.WorkingAreas, ", "))
	sb.WriteString("\n\n")

	if len(categorized.TodoItems) > 0 {
		sb.WriteString("Initial Todo Items:\n")
		for _, todo := range categorized.TodoItems {
			sb.WriteString(fmt.Sprintf("  - [%s] %s (priority: %d)\n", todo.Status, todo.Description, todo.Priority))
			if len(todo.Tools) > 0 {
				sb.WriteString(fmt.Sprintf("    Tools: %s\n", strings.Join(todo.Tools, ", ")))
			}
			if todo.InjectLater {
				sb.WriteString("    Note: Context to be injected later\n")
			}
		}
		sb.WriteString("\n")
	}

	sb.WriteString("Context Needs:\n")
	for _, need := range categorized.ContextNeeds {
		sb.WriteString(fmt.Sprintf("  - %s: %s (required: %v, timing: %s)\n", need.Key, need.Description, need.Required, need.InjectTiming))
	}

	sb.WriteString("\nProvide a refined task definition with any additional todos or context needs.")

	return &PromptPart{
		Stage:    StageTaskDefinition,
		Content:  sb.String(),
		Tools:    categorized.AllowedTools,
		Metadata: map[string]any{"categorized": categorized},
	}
}

// BuildContextManagementPrompt creates prompts for context operations.
func (a *AssistantPrompts) BuildContextManagementPrompt(action ContextAction, context *AssistantContext, details string) *PromptPart {
	var sb strings.Builder

	sb.WriteString("Context Management: ")
	sb.WriteString(string(action))
	sb.WriteString("\n\n")

	switch action {
	case ContextActionStash:
		sb.WriteString("Stash the current context with a summary for later resumption.\n")
		sb.WriteString("Current conversation length: ")
		sb.WriteString(fmt.Sprintf("%d messages\n", len(context.ConversationHistory)))
		sb.WriteString("Details: ")
		sb.WriteString(details)
		sb.WriteString("\n\nProvide a concise summary that captures the essential state.")

	case ContextActionSeparate:
		sb.WriteString("Separate the current context into a new independent context.\n")
		sb.WriteString("Details: ")
		sb.WriteString(details)
		sb.WriteString("\n\nDefine what goes into the new context vs what stays.")

	case ContextActionDelete:
		sb.WriteString("Delete the specified context.\n")
		sb.WriteString("Details: ")
		sb.WriteString(details)
		sb.WriteString("\n\nConfirm what should be removed.")

	case ContextActionNew:
		sb.WriteString("Create a fresh context.\n")
		sb.WriteString("Details: ")
		sb.WriteString(details)
		sb.WriteString("\n\nDefine initial state for the new context.")

	case ContextActionResume:
		sb.WriteString("Resume from a stashed context.\n")
		sb.WriteString("Stash Summary: ")
		sb.WriteString(details)
		sb.WriteString("\n\nRestore the essential state from the summary.")

	case ContextActionShare:
		sb.WriteString("Share context with another agent (e.g., coder).\n")
		sb.WriteString("Details: ")
		sb.WriteString(details)
		sb.WriteString("\n\nDefine what context to share and what to keep private.")
	}

	return &PromptPart{
		Stage:    StageContextManage,
		Content:  sb.String(),
		Tools:    ToolSetContextOnly,
		Metadata: map[string]any{"action": action, "details": details},
	}
}

// BuildUserResponsePrompt creates a prompt for responding to the user.
func (a *AssistantPrompts) BuildUserResponsePrompt(categorized *CategorizedRequest, result string) *PromptPart {
	var sb strings.Builder

	sb.WriteString("Respond to the user based on the completed work.\n\n")
	sb.WriteString("Original Request: ")
	sb.WriteString(categorized.OriginalPrompt)
	sb.WriteString("\n")
	sb.WriteString("Category: ")
	sb.WriteString(string(categorized.Category))
	sb.WriteString("\n")
	sb.WriteString("Result: ")
	sb.WriteString(result)
	sb.WriteString("\n\n")
	sb.WriteString("Provide a clear, concise response. Do not use technical jargon unless the user asked for it. ")
	sb.WriteString("If there are follow-up actions needed, mention them briefly.")

	return &PromptPart{
		Stage:    StageCompletion,
		Content:  sb.String(),
		Tools:    ToolSetContextOnly,
		Metadata: map[string]any{"categorized": categorized, "result": result},
	}
}

// BuildSimpleTaskPrompt creates a prompt for simple direct tasks.
func (a *AssistantPrompts) BuildSimpleTaskPrompt(categorized *CategorizedRequest) *PromptPart {
	var sb strings.Builder

	sb.WriteString("Execute this simple task directly.\n\n")
	sb.WriteString("Request: ")
	sb.WriteString(categorized.OriginalPrompt)
	sb.WriteString("\n")
	sb.WriteString("Allowed Tools: ")
	sb.WriteString(string(categorized.AllowedTools))
	sb.WriteString("\n\n")
	sb.WriteString("Complete the task and provide the result. Do not over-engineer.")

	return &PromptPart{
		Stage:    StageExecution,
		Content:  sb.String(),
		Tools:    categorized.AllowedTools,
		Metadata: map[string]any{"categorized": categorized},
	}
}