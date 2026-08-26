package prompt

import (
	"fmt"
	"strings"
)

// BuildTaskPrompts converts planned TaskSpecs into executable prompt parts.
// The tasks themselves are produced by LLMTaskPlanner; this only formats them.
func BuildTaskPrompts(tasks []TaskSpec, initResults *InitResults) []PromptPart {
	var parts []PromptPart

	for _, task := range tasks {
		var sb strings.Builder
		sb.WriteString("TASK: ")
		sb.WriteString(task.Description)
		sb.WriteString("\n\n")
		sb.WriteString("Role: ")
		sb.WriteString(task.Role)
		sb.WriteString("\n")
		sb.WriteString("Type: ")
		sb.WriteString(task.Type)
		sb.WriteString("\n\n")
		sb.WriteString("Prompt:\n")
		sb.WriteString(task.Prompt)
		sb.WriteString("\n\n")

		if len(task.Context) > 0 {
			sb.WriteString("Context:\n")
			for k, v := range task.Context {
				sb.WriteString(fmt.Sprintf("  %s: %v\n", k, v))
			}
		}

		parts = append(parts, PromptPart{
			Stage:    StageExecution,
			Content:  sb.String(),
			Tools:    ToolSetModerate,
			Metadata: map[string]any{"task_spec": task},
		})
	}

	return parts
}
