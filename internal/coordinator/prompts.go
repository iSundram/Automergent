package coordinator

import (
	"fmt"
	"strings"
)

// BuildRolePrompt builds a role-specific prompt for the given task.
func BuildRolePrompt(role AgentRole, task *Task) string {
	switch role {
	case RoleResearcher:
		return buildResearcherPrompt(task)
	case RoleCoder:
		return buildCoderPrompt(task)
	case RoleReviewer:
		return buildReviewerPrompt(task)
	case RoleTester:
		return buildTesterPrompt(task)
	case RoleDocumenter:
		return buildDocumenterPrompt(task)
	default:
		return buildGenericPrompt(task)
	}
}

func buildGenericPrompt(task *Task) string {
	var sb strings.Builder

	sb.WriteString("You are an agent specialized in ")
	sb.WriteString(string(task.Role))
	sb.WriteString(".\n\n")

	sb.WriteString("Task: ")
	sb.WriteString(task.Prompt)
	sb.WriteString("\n")

	if task.Context.WorkingDir != "" {
		sb.WriteString("\nWorking Directory: ")
		sb.WriteString(task.Context.WorkingDir)
		sb.WriteString("\n")
	}

	if len(task.Context.Files) > 0 {
		sb.WriteString("\nRelevant Files:\n")
		for _, f := range task.Context.Files {
			sb.WriteString("- ")
			sb.WriteString(f)
			sb.WriteString("\n")
		}
	}

	if len(task.Context.CodeSnippets) > 0 {
		sb.WriteString("\nRelevant Code:\n")
		for path, code := range task.Context.CodeSnippets {
			sb.WriteString(fmt.Sprintf("\n--- %s ---\n%s\n", path, code))
		}
	}

	if len(task.Context.Constraints) > 0 {
		sb.WriteString("\nConstraints:\n")
		for _, c := range task.Context.Constraints {
			sb.WriteString("- ")
			sb.WriteString(c)
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

func buildResearcherPrompt(task *Task) string {
	var sb strings.Builder

	sb.WriteString("You are a Researcher agent specialized in exploring codebases and gathering context.\n\n")
	sb.WriteString("Your task is to:\n")
	sb.WriteString("1. Explore and understand the relevant code\n")
	sb.WriteString("2. Gather context and dependencies\n")
	sb.WriteString("3. Identify patterns and architecture\n")
	sb.WriteString("4. Report findings concisely\n\n")

	sb.WriteString("Task: ")
	sb.WriteString(task.Prompt)
	sb.WriteString("\n")

	if task.Context.WorkingDir != "" {
		sb.WriteString("\nWorking Directory: ")
		sb.WriteString(task.Context.WorkingDir)
		sb.WriteString("\n")
	}

	if len(task.Context.Files) > 0 {
		sb.WriteString("\nRelevant Files:\n")
		for _, f := range task.Context.Files {
			sb.WriteString("- ")
			sb.WriteString(f)
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

func buildCoderPrompt(task *Task) string {
	var sb strings.Builder

	sb.WriteString("You are a Coder agent specialized in implementing code changes.\n\n")
	sb.WriteString("Your task is to:\n")
	sb.WriteString("1. Implement the requested changes\n")
	sb.WriteString("2. Follow existing code style and patterns\n")
	sb.WriteString("3. Write clean, maintainable code\n")
	sb.WriteString("4. Include necessary imports and dependencies\n\n")

	sb.WriteString("Task: ")
	sb.WriteString(task.Prompt)
	sb.WriteString("\n")

	if len(task.Context.CodeSnippets) > 0 {
		sb.WriteString("\nRelevant Code:\n")
		for path, code := range task.Context.CodeSnippets {
			sb.WriteString(fmt.Sprintf("\n--- %s ---\n%s\n", path, code))
		}
	}

	if len(task.Context.Constraints) > 0 {
		sb.WriteString("\nConstraints:\n")
		for _, c := range task.Context.Constraints {
			sb.WriteString("- ")
			sb.WriteString(c)
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

func buildReviewerPrompt(task *Task) string {
	var sb strings.Builder

	sb.WriteString("You are a Reviewer agent specialized in code review.\n\n")
	sb.WriteString("Your task is to:\n")
	sb.WriteString("1. Review the code for bugs and issues\n")
	sb.WriteString("2. Check for security vulnerabilities\n")
	sb.WriteString("3. Verify adherence to best practices\n")
	sb.WriteString("4. Suggest improvements (only significant ones)\n")
	sb.WriteString("5. Focus on high-impact issues, not style\n\n")

	sb.WriteString("Task: ")
	sb.WriteString(task.Prompt)
	sb.WriteString("\n")

	if len(task.Context.CodeSnippets) > 0 {
		sb.WriteString("\nCode to Review:\n")
		for path, code := range task.Context.CodeSnippets {
			sb.WriteString(fmt.Sprintf("\n--- %s ---\n%s\n", path, code))
		}
	}

	return sb.String()
}

func buildTesterPrompt(task *Task) string {
	var sb strings.Builder

	sb.WriteString("You are a Tester agent specialized in testing and quality assurance.\n\n")
	sb.WriteString("Your task is to:\n")
	sb.WriteString("1. Write comprehensive tests\n")
	sb.WriteString("2. Cover edge cases and error conditions\n")
	sb.WriteString("3. Run existing tests if requested\n")
	sb.WriteString("4. Report test results and coverage\n\n")

	sb.WriteString("Task: ")
	sb.WriteString(task.Prompt)
	sb.WriteString("\n")

	if len(task.Context.CodeSnippets) > 0 {
		sb.WriteString("\nCode to Test:\n")
		for path, code := range task.Context.CodeSnippets {
			sb.WriteString(fmt.Sprintf("\n--- %s ---\n%s\n", path, code))
		}
	}

	return sb.String()
}

func buildDocumenterPrompt(task *Task) string {
	var sb strings.Builder

	sb.WriteString("You are a Documenter agent specialized in documentation.\n\n")
	sb.WriteString("Your task is to:\n")
	sb.WriteString("1. Write clear, concise documentation\n")
	sb.WriteString("2. Include usage examples\n")
	sb.WriteString("3. Document APIs and interfaces\n")
	sb.WriteString("4. Keep docs up-to-date with code\n\n")

	sb.WriteString("Task: ")
	sb.WriteString(task.Prompt)
	sb.WriteString("\n")

	if len(task.Context.CodeSnippets) > 0 {
		sb.WriteString("\nCode to Document:\n")
		for path, code := range task.Context.CodeSnippets {
			sb.WriteString(fmt.Sprintf("\n--- %s ---\n%s\n", path, code))
		}
	}

	return sb.String()
}