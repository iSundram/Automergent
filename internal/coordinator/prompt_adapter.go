package coordinator

import (
	"strings"

	"github.com/iSundram/Automergent/internal/prompt"
)

// PromptAdapter adapts the new prompt system for use with the coordinator.
type PromptAdapter struct {
	manager *prompt.PromptManager
}

// NewPromptAdapter creates a new prompt adapter.
func NewPromptAdapter(manager *prompt.PromptManager) *PromptAdapter {
	return &PromptAdapter{manager: manager}
}

// BuildRolePrompt builds a role-specific prompt for a coordinator task using the new prompt system.
func (pa *PromptAdapter) BuildRolePrompt(role AgentRole, task *Task) string {
	// Adapt the coordinator task to our categorized request
	categorized := CoordinatorTaskAdapter(task)

	// Build appropriate prompt based on role
	switch role {
	case RoleResearcher:
		return pa.buildResearcherPrompt(categorized)
	case RoleCoder:
		return pa.buildCoderPrompt(categorized)
	case RoleReviewer:
		return pa.buildReviewerPrompt(categorized)
	case RoleTester:
		return pa.buildTesterPrompt(categorized)
	case RoleDocumenter:
		return pa.buildDocumenterPrompt(categorized)
	default:
		return pa.buildGenericPrompt(categorized)
	}
}

func (pa *PromptAdapter) buildGenericPrompt(categorized *prompt.CategorizedRequest) string {
	var sb strings.Builder

	sb.WriteString("You are an agent specialized in ")
	sb.WriteString(string(categorized.Category))
	sb.WriteString(".\n\n")

	sb.WriteString("Task: ")
	sb.WriteString(categorized.OriginalPrompt)
	sb.WriteString("\n")

	if len(categorized.WorkingAreas) > 0 {
		sb.WriteString("\nRelevant Files:\n")
		for _, f := range categorized.WorkingAreas {
			sb.WriteString("- ")
			sb.WriteString(f)
			sb.WriteString("\n")
		}
	}

	if len(categorized.ContextNeeds) > 0 {
		sb.WriteString("\nContext Needs:\n")
		for _, need := range categorized.ContextNeeds {
			sb.WriteString("- ")
			sb.WriteString(need.Key)
			sb.WriteString(": ")
			sb.WriteString(need.Description)
			sb.WriteString("\n")
		}
	}

	if len(categorized.TodoItems) > 0 {
		sb.WriteString("\nTodo Items:\n")
		for _, todo := range categorized.TodoItems {
			sb.WriteString("- [")
			sb.WriteString(string(todo.Status))
			sb.WriteString("] ")
			sb.WriteString(todo.Description)
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

func (pa *PromptAdapter) buildResearcherPrompt(categorized *prompt.CategorizedRequest) string {
	var sb strings.Builder

	sb.WriteString("You are a Researcher agent specialized in exploring codebases and gathering context.\n\n")
	sb.WriteString("Your task is to:\n")
	sb.WriteString("1. Explore and understand the relevant code\n")
	sb.WriteString("2. Gather context and dependencies\n")
	sb.WriteString("3. Identify patterns and architecture\n")
	sb.WriteString("4. Report findings concisely\n\n")

	sb.WriteString("Task: ")
	sb.WriteString(categorized.OriginalPrompt)
	sb.WriteString("\n")

	if len(categorized.WorkingAreas) > 0 {
		sb.WriteString("\nRelevant Files:\n")
		for _, f := range categorized.WorkingAreas {
			sb.WriteString("- ")
			sb.WriteString(f)
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

func (pa *PromptAdapter) buildCoderPrompt(categorized *prompt.CategorizedRequest) string {
	var sb strings.Builder

	sb.WriteString("You are a Coder agent specialized in implementing code changes.\n\n")
	sb.WriteString("Your task is to:\n")
	sb.WriteString("1. Implement the requested changes\n")
	sb.WriteString("2. Follow existing code style and patterns\n")
	sb.WriteString("3. Write clean, maintainable code\n")
	sb.WriteString("4. Include necessary imports and dependencies\n\n")

	sb.WriteString("Task: ")
	sb.WriteString(categorized.OriginalPrompt)
	sb.WriteString("\n")

	if len(categorized.ContextNeeds) > 0 {
		sb.WriteString("\nContext Available:\n")
		for _, need := range categorized.ContextNeeds {
			if need.InjectTiming != prompt.InjectTimingDeferred {
				sb.WriteString("- ")
				sb.WriteString(need.Key)
				sb.WriteString(": ")
				sb.WriteString(need.Description)
				sb.WriteString("\n")
			}
		}
	}

	if len(categorized.TodoItems) > 0 {
		sb.WriteString("\nImplementation Plan:\n")
		for _, todo := range categorized.TodoItems {
			sb.WriteString("- [")
			sb.WriteString(string(todo.Status))
			sb.WriteString("] ")
			sb.WriteString(todo.Description)
			if len(todo.Tools) > 0 {
				sb.WriteString(" (Tools: ")
				sb.WriteString(strings.Join(todo.Tools, ", "))
				sb.WriteString(")")
			}
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

func (pa *PromptAdapter) buildReviewerPrompt(categorized *prompt.CategorizedRequest) string {
	var sb strings.Builder

	sb.WriteString("You are a Reviewer agent specialized in code review.\n\n")
	sb.WriteString("Your task is to:\n")
	sb.WriteString("1. Review the code for bugs and issues\n")
	sb.WriteString("2. Check for security vulnerabilities\n")
	sb.WriteString("3. Verify adherence to best practices\n")
	sb.WriteString("4. Suggest improvements (only significant ones)\n")
	sb.WriteString("5. Focus on high-impact issues, not style\n\n")

	sb.WriteString("Task: ")
	sb.WriteString(categorized.OriginalPrompt)
	sb.WriteString("\n")

	if len(categorized.ContextNeeds) > 0 {
		sb.WriteString("\nCode to Review Context:\n")
		for _, need := range categorized.ContextNeeds {
			sb.WriteString("- ")
			sb.WriteString(need.Key)
			sb.WriteString(": ")
			sb.WriteString(need.Description)
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

func (pa *PromptAdapter) buildTesterPrompt(categorized *prompt.CategorizedRequest) string {
	var sb strings.Builder

	sb.WriteString("You are a Tester agent specialized in testing and quality assurance.\n\n")
	sb.WriteString("Your task is to:\n")
	sb.WriteString("1. Write comprehensive tests\n")
	sb.WriteString("2. Cover edge cases and error conditions\n")
	sb.WriteString("3. Run existing tests if requested\n")
	sb.WriteString("4. Report test results and coverage\n\n")

	sb.WriteString("Task: ")
	sb.WriteString(categorized.OriginalPrompt)
	sb.WriteString("\n")

	if len(categorized.ContextNeeds) > 0 {
		sb.WriteString("\nCode to Test Context:\n")
		for _, need := range categorized.ContextNeeds {
			sb.WriteString("- ")
			sb.WriteString(need.Key)
			sb.WriteString(": ")
			sb.WriteString(need.Description)
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

func (pa *PromptAdapter) buildDocumenterPrompt(categorized *prompt.CategorizedRequest) string {
	var sb strings.Builder

	sb.WriteString("You are a Documenter agent specialized in documentation.\n\n")
	sb.WriteString("Your task is to:\n")
	sb.WriteString("1. Write clear, concise documentation\n")
	sb.WriteString("2. Include usage examples\n")
	sb.WriteString("3. Document APIs and interfaces\n")
	sb.WriteString("4. Keep docs up-to-date with code\n\n")

	sb.WriteString("Task: ")
	sb.WriteString(categorized.OriginalPrompt)
	sb.WriteString("\n")

	if len(categorized.ContextNeeds) > 0 {
		sb.WriteString("\nCode to Document Context:\n")
		for _, need := range categorized.ContextNeeds {
			sb.WriteString("- ")
			sb.WriteString(need.Key)
			sb.WriteString(": ")
			sb.WriteString(need.Description)
			sb.WriteString("\n")
		}
	}

	return sb.String()
}