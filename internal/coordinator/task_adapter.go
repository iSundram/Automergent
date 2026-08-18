package coordinator

import (
	"github.com/iSundram/Automergent/internal/prompt"
)

// CoordinatorTaskAdapter adapts coordinator.Task to prompt.CategorizedRequest.
func CoordinatorTaskAdapter(task *Task) *prompt.CategorizedRequest {
	return &prompt.CategorizedRequest{
		Category:       mapTaskTypeToCategory(task.Type),
		Complexity:     inferComplexity(task),
		Strategy:       inferStrategy(task),
		AllowedTools:   inferToolSet(task),
		WorkingAreas:   task.Context.Files,
		OriginalPrompt: task.Prompt,
		RequiresCoder:  task.Role == RoleCoder,
		CreatedAt:      task.CreatedAt,
	}
}

func mapTaskTypeToCategory(t TaskType) prompt.RequestCategory {
	switch t {
	case TaskTypeImplement:
		return prompt.CategoryNewFeature
	case TaskTypeDebug:
		return prompt.CategoryDebug
	case TaskTypeExplore:
		return prompt.CategoryIssueSuspect
	case TaskTypeReview:
		return prompt.CategoryVerifyWork
	case TaskTypeTest:
		return prompt.CategoryPlan
	case TaskTypeDocument:
		return prompt.CategoryDirect
	case TaskTypeRefactor:
		return prompt.CategoryNewFeature
	case TaskTypeSynthesize:
		return prompt.CategoryPlan
	default:
		return prompt.CategoryUnknown
	}
}

func inferComplexity(task *Task) prompt.TaskComplexity {
	if len(task.Context.Files) > 10 || len(task.Context.CodeSnippets) > 5 {
		return prompt.ComplexityComplex
	}
	if len(task.Context.Files) > 3 || len(task.Context.CodeSnippets) > 2 {
		return prompt.ComplexityModerate
	}
	return prompt.ComplexitySimple
}

func inferStrategy(task *Task) prompt.ExecutionStrategy {
	if task.Role == RoleCoder {
		return prompt.StrategyCoderAgent
	}
	if len(task.Dependencies) > 0 {
		return prompt.StrategySequential
	}
	return prompt.StrategyDirect
}

func inferToolSet(task *Task) prompt.ToolSet {
	switch task.Role {
	case RoleResearcher:
		return prompt.ToolSetReadOnly
	case RoleCoder:
		return prompt.ToolSetModerate
	case RoleReviewer, RoleTester:
		return prompt.ToolSetBasic
	default:
		return prompt.ToolSetContextOnly
	}
}