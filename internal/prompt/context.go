package prompt

import (
	"fmt"
	"strings"
	"time"
)

// ContextPrompts generates prompts for context management operations.
type ContextPrompts struct {
	config *PromptConfig
}

// NewContextPrompts creates a new context prompt generator.
func NewContextPrompts(config *PromptConfig) *ContextPrompts {
	if config == nil {
		config = DefaultPromptConfig()
	}
	return &ContextPrompts{config: config}
}

// BuildStashPrompt creates a prompt for stashing context with summary.
func (c *ContextPrompts) BuildStashPrompt(context *AssistantContext, reason string) *PromptPart {
	var sb strings.Builder

	sb.WriteString("STASH CONTEXT\n\n")
	sb.WriteString("Reason: ")
	sb.WriteString(reason)
	sb.WriteString("\n\n")

	sb.WriteString("Current Conversation:\n")
	for i, msg := range context.ConversationHistory {
		if i >= 20 { // Limit to last 20 messages for summary
			sb.WriteString(fmt.Sprintf("  ... (%d more messages)\n", len(context.ConversationHistory)-i))
			break
		}
		sb.WriteString(fmt.Sprintf("  [%s] %s: %s\n", msg.Timestamp.Format(time.RFC3339), msg.Role, truncate(msg.Content, 200)))
	}
	sb.WriteString("\n")

	if context.CurrentTask != nil {
		sb.WriteString("Current Task:\n")
		sb.WriteString(fmt.Sprintf("  Category: %s\n", context.CurrentTask.Category))
		sb.WriteString(fmt.Sprintf("  Complexity: %s\n", context.CurrentTask.Complexity))
		sb.WriteString(fmt.Sprintf("  Original Prompt: %s\n", truncate(context.CurrentTask.OriginalPrompt, 500)))
		if len(context.CurrentTask.TodoItems) > 0 {
			sb.WriteString("  Todos:\n")
			for _, todo := range context.CurrentTask.TodoItems {
				sb.WriteString(fmt.Sprintf("    - [%s] %s\n", todo.Status, todo.Description))
			}
		}
		sb.WriteString("\n")
	}

	sb.WriteString("User Preferences:\n")
	for k, v := range context.UserPreferences {
		sb.WriteString(fmt.Sprintf("  %s: %s\n", k, v))
	}
	sb.WriteString("\n")

	sb.WriteString("Create a concise summary that captures:\n")
	sb.WriteString("1. What was the user's original request?\n")
	sb.WriteString("2. What has been done so far?\n")
	sb.WriteString("3. What is the current state?\n")
	sb.WriteString("4. What are the next steps?\n")
	sb.WriteString("5. Any important context or decisions made?\n\n")
	sb.WriteString("The summary should be detailed enough to resume work later without losing context.\n")
	sb.WriteString("Format: Provide the summary only, no extra commentary.")

	return &PromptPart{
		Stage:    StageContextManage,
		Content:  sb.String(),
		Tools:    ToolSetContextOnly,
		Metadata: map[string]any{"action": ContextActionStash, "reason": reason},
	}
}

// BuildSeparateContextPrompt creates a prompt for separating context.
func (c *ContextPrompts) BuildSeparateContextPrompt(context *AssistantContext, splitCriteria string) *PromptPart {
	var sb strings.Builder

	sb.WriteString("SEPARATE CONTEXT\n\n")
	sb.WriteString("Split Criteria: ")
	sb.WriteString(splitCriteria)
	sb.WriteString("\n\n")

	sb.WriteString("Current Context:\n")
	sb.WriteString(fmt.Sprintf("  Messages: %d\n", len(context.ConversationHistory)))
	if context.CurrentTask != nil {
		sb.WriteString(fmt.Sprintf("  Current Task: %s (%s)\n", context.CurrentTask.Category, context.CurrentTask.Complexity))
	}
	sb.WriteString(fmt.Sprintf("  Stashed Contexts: %d\n", len(context.StashedContexts)))
	sb.WriteString("\n")

	sb.WriteString("Define the separation:\n")
	sb.WriteString("1. What goes into Context A (continuing context)?\n")
	sb.WriteString("2. What goes into Context B (new separate context)?\n")
	sb.WriteString("3. What shared context (if any) should both have access to?\n\n")
	sb.WriteString("Provide a clear division plan.")

	return &PromptPart{
		Stage:    StageContextManage,
		Content:  sb.String(),
		Tools:    ToolSetContextOnly,
		Metadata: map[string]any{"action": ContextActionSeparate, "criteria": splitCriteria},
	}
}

// BuildDeleteContextPrompt creates a prompt for deleting context.
func (c *ContextPrompts) BuildDeleteContextPrompt(context *AssistantContext, target string) *PromptPart {
	var sb strings.Builder

	sb.WriteString("DELETE CONTEXT\n\n")
	sb.WriteString("Target: ")
	sb.WriteString(target)
	sb.WriteString("\n\n")

	sb.WriteString("Current Context:\n")
	sb.WriteString(fmt.Sprintf("  Messages: %d\n", len(context.ConversationHistory)))
	if context.CurrentTask != nil {
		sb.WriteString(fmt.Sprintf("  Current Task: %s\n", context.CurrentTask.Category))
	}
	sb.WriteString(fmt.Sprintf("  Stashed Contexts: %d\n", len(context.StashedContexts)))
	for i, stash := range context.StashedContexts {
		sb.WriteString(fmt.Sprintf("  Stash %d: %s (tags: %s)\n", i, truncate(stash.Summary, 100), strings.Join(stash.Tags, ", ")))
	}
	sb.WriteString("\n")

	sb.WriteString("Confirm deletion. Specify exactly what to delete:")
	sb.WriteString("\n- Specific messages? (range or criteria)")
	sb.WriteString("\n- Current task?")
sb.WriteString("\n- Specific stashed contexts? (by index or tag)")
	sb.WriteString("\n- All context?")
	sb.WriteString("\n\nProvide deletion specification.")

	return &PromptPart{
		Stage:    StageContextManage,
		Content:  sb.String(),
		Tools:    ToolSetContextOnly,
		Metadata: map[string]any{"action": ContextActionDelete, "target": target},
	}
}

// BuildNewContextPrompt creates a prompt for creating a new context.
func (c *ContextPrompts) BuildNewContextPrompt(parentContext *AssistantContext, initialPrompt string) *PromptPart {
	var sb strings.Builder

	sb.WriteString("NEW CONTEXT\n\n")
	sb.WriteString("Initial Prompt: ")
	sb.WriteString(initialPrompt)
	sb.WriteString("\n\n")

	if parentContext != nil {
		sb.WriteString("Parent Context (for reference, not automatically inherited):\n")
		sb.WriteString(fmt.Sprintf("  Messages: %d\n", len(parentContext.ConversationHistory)))
		if parentContext.CurrentTask != nil {
			sb.WriteString(fmt.Sprintf("  Parent Task: %s\n", parentContext.CurrentTask.Category))
		}
		sb.WriteString(fmt.Sprintf("  Stashed Contexts: %d\n", len(parentContext.StashedContexts)))
		sb.WriteString("\n")

		sb.WriteString("What from parent context should be carried over?\n")
		sb.WriteString("- User preferences?\n")
		sb.WriteString("- Specific stashed contexts?\n")
		sb.WriteString("- Working directory?\n")
		sb.WriteString("- Nothing (fresh start)?\n\n")
	}

	sb.WriteString("Define initial state for new context:")
	sb.WriteString("\n- Initial task category (if known)")
	sb.WriteString("\n- Working directory")
	sb.WriteString("\n- Any pre-loaded context")
	sb.WriteString("\n- User preferences to inherit")

	return &PromptPart{
		Stage:    StageContextManage,
		Content:  sb.String(),
		Tools:    ToolSetContextOnly,
		Metadata: map[string]any{"action": ContextActionNew, "initial_prompt": initialPrompt},
	}
}

// BuildResumeContextPrompt creates a prompt for resuming from stashed context.
func (c *ContextPrompts) BuildResumeContextPrompt(stash *ContextStash, currentContext *AssistantContext) *PromptPart {
	var sb strings.Builder

	sb.WriteString("RESUME FROM STASHED CONTEXT\n\n")

	sb.WriteString("Stash Summary:\n")
	sb.WriteString(stash.Summary)
	sb.WriteString("\n\n")
	sb.WriteString(fmt.Sprintf("Stashed at: %s\n", stash.CreatedAt.Format(time.RFC3339)))
	sb.WriteString(fmt.Sprintf("Tags: %s\n", strings.Join(stash.Tags, ", ")))
	sb.WriteString(fmt.Sprintf("Resumable: %v\n\n", stash.Resumable))

	if currentContext != nil {
		sb.WriteString("Current Context (will be replaced or merged):\n")
		sb.WriteString(fmt.Sprintf("  Messages: %d\n", len(currentContext.ConversationHistory)))
		if currentContext.CurrentTask != nil {
			sb.WriteString(fmt.Sprintf("  Current Task: %s\n", currentContext.CurrentTask.Category))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("Restore the context from this stash. Decide:\n")
	sb.WriteString("1. Replace current context entirely?\n")
	sb.WriteString("2. Merge with current context?\n")
	sb.WriteString("3. What specific state to restore?\n\n")
	sb.WriteString("Provide restoration plan.")

	return &PromptPart{
		Stage:    StageContextManage,
		Content:  sb.String(),
		Tools:    ToolSetContextOnly,
		Metadata: map[string]any{"action": ContextActionResume, "stash_id": stash.ID},
	}
}

// BuildShareContextPrompt creates a prompt for sharing context with coder agent.
func (c *ContextPrompts) BuildShareContextPrompt(assistantCtx *AssistantContext, coderCtx *CoderContext, shareSpec string) *PromptPart {
	var sb strings.Builder

	sb.WriteString("SHARE CONTEXT WITH CODER AGENT\n\n")
	sb.WriteString("Share Specification: ")
	sb.WriteString(shareSpec)
	sb.WriteString("\n\n")

	sb.WriteString("Assistant Context (source):\n")
	if assistantCtx.CurrentTask != nil {
		sb.WriteString(fmt.Sprintf("  Task: %s (%s)\n", assistantCtx.CurrentTask.Category, assistantCtx.CurrentTask.Complexity))
		sb.WriteString(fmt.Sprintf("  Original Prompt: %s\n", truncate(assistantCtx.CurrentTask.OriginalPrompt, 500)))
		if len(assistantCtx.CurrentTask.WorkingAreas) > 0 {
			sb.WriteString(fmt.Sprintf("  Working Areas: %s\n", strings.Join(assistantCtx.CurrentTask.WorkingAreas, ", ")))
		}
		if len(assistantCtx.CurrentTask.ContextNeeds) > 0 {
			sb.WriteString("  Context Needs:\n")
			for _, need := range assistantCtx.CurrentTask.ContextNeeds {
				sb.WriteString(fmt.Sprintf("    - %s: %s (timing: %s)\n", need.Key, need.Description, need.InjectTiming))
			}
		}
	}
	sb.WriteString("\n")

	sb.WriteString("Coder Context (target - current state):\n")
	sb.WriteString(fmt.Sprintf("  Working Dir: %s\n", coderCtx.WorkingDir))
	sb.WriteString(fmt.Sprintf("  Files: %d\n", len(coderCtx.Files)))
	sb.WriteString(fmt.Sprintf("  Code Snippets: %d\n", len(coderCtx.CodeSnippets)))
	sb.WriteString(fmt.Sprintf("  Todo Items: %d\n", len(coderCtx.TodoItems)))
	sb.WriteString(fmt.Sprintf("  Shared Context Keys: %d\n", len(coderCtx.SharedContext)))
	sb.WriteString("\n")

	sb.WriteString("Determine what to share:\n")
	sb.WriteString("- Task definition and requirements\n")
	sb.WriteString("- Working areas and files\n")
sb.WriteString("- Code snippets and patterns\n")
	sb.WriteString("- Constraints and requirements\n")
	sb.WriteString("- Todo items and priorities\n")
	sb.WriteString("- Deferred context injection specs\n\n")
	sb.WriteString("Provide the sharing plan with specific key-value pairs to transfer.")

	return &PromptPart{
		Stage:    StageContextManage,
		Content:  sb.String(),
		Tools:    ToolSetContextOnly,
		Metadata: map[string]any{"action": ContextActionShare, "spec": shareSpec},
	}
}

// BuildContextSummaryPrompt creates a prompt for generating a context summary.
func (c *ContextPrompts) BuildContextSummaryPrompt(context *AssistantContext, maxLength int) *PromptPart {
	var sb strings.Builder

	sb.WriteString("Generate a concise context summary.\n\n")
	sb.WriteString(fmt.Sprintf("Max length: %d characters\n\n", maxLength))

	sb.WriteString("Conversation History:\n")
	for _, msg := range context.ConversationHistory {
		sb.WriteString(fmt.Sprintf("  %s: %s\n", msg.Role, truncate(msg.Content, 150)))
	}
	sb.WriteString("\n")

	if context.CurrentTask != nil {
		sb.WriteString(fmt.Sprintf("Current Task: %s (%s)\n", context.CurrentTask.Category, context.CurrentTask.Complexity))
		sb.WriteString(fmt.Sprintf("Original Request: %s\n", truncate(context.CurrentTask.OriginalPrompt, 300)))
	}

	sb.WriteString("\nProvide a summary that captures the essential state for resumption.")

	return &PromptPart{
		Stage:    StageContextManage,
		Content:  sb.String(),
		Tools:    ToolSetContextOnly,
		Metadata: map[string]any{"max_length": maxLength},
	}
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}