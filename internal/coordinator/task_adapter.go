package coordinator

import (
	"github.com/google/uuid"
	"github.com/iSundram/Automergent/internal/prompt"
)

// CoordinatorTaskAdapter adapts coordinator.Task to prompt.CategorizedRequest (legacy).
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

// TaskSpecAdapter adapts coordinator.Task to prompt.TaskSpec for the new intent-based system.
func TaskSpecAdapter(task *Task) *prompt.TaskSpec {
	return &prompt.TaskSpec{
		ID:           task.ID,
		IntentID:     task.ID,
		Type:         string(task.Type),
		Role:         string(task.Role),
		Priority:     int(task.Priority),
		Dependencies: task.Dependencies,
		Prompt:       task.Prompt,
		Context: map[string]any{
			"working_dir":   task.Context.WorkingDir,
			"files":         task.Context.Files,
			"code_snippets": task.Context.CodeSnippets,
			"messages":      task.Context.Messages,
			"parent_task":   task.Context.ParentTaskID,
			"project_info":  task.Context.ProjectInfo,
			"constraints":   task.Context.Constraints,
		},
		Tools:       inferTools(task.Role),
		Description: task.Prompt,
	}
}

// IntentSetFromTasks creates an IntentSet from a list of coordinator tasks.
func IntentSetFromTasks(tasks []*Task, originalPrompt string) *prompt.IntentSet {
	var intents []prompt.Intent

	for _, task := range tasks {
		intentType := mapTaskTypeToIntentType(task.Type)
		intents = append(intents, prompt.Intent{
			ID:           task.ID,
			Type:         intentType,
			Priority:     int(task.Priority),
			Dependencies: task.Dependencies,
			Parameters: map[string]any{
				"role":        string(task.Role),
				"task_type":   string(task.Type),
				"working_dir": task.Context.WorkingDir,
				"files":       task.Context.Files,
			},
			RawText:    task.Prompt,
			Confidence: 0.9,
		})
	}

	// Determine if init phase is needed
	requiresInit := false
	for _, intent := range intents {
		if intent.Type == prompt.IntentExplore || intent.Type == prompt.IntentFix || intent.Type == prompt.IntentDebug || intent.Type == prompt.IntentReview {
			requiresInit = true
			break
		}
	}

	var initPhase *prompt.InitPhase
	if requiresInit {
		initPhase = &prompt.InitPhase{
			ID:      uuid.New().String()[:8],
			Goal:    "Understand codebase for: " + originalPrompt,
			Actions: []prompt.InitAction{},
			SuccessCriteria: []string{
				"Found relevant files",
				"Understood the issue/requirement",
				"Identified key code areas",
			},
		}
	}

	return &prompt.IntentSet{
		Intents:        intents,
		RequiresInit:   requiresInit,
		InitPhase:      initPhase,
		OriginalPrompt: originalPrompt,
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

func mapTaskTypeToIntentType(t TaskType) prompt.IntentType {
	switch t {
	case TaskTypeImplement:
		return prompt.IntentImplement
	case TaskTypeDebug:
		return prompt.IntentDebug
	case TaskTypeExplore:
		return prompt.IntentExplore
	case TaskTypeReview:
		return prompt.IntentReview
	case TaskTypeTest:
		return prompt.IntentTest
	case TaskTypeDocument:
		return prompt.IntentDocument
	case TaskTypeRefactor:
		return prompt.IntentRefactor
	case TaskTypeSynthesize:
		return prompt.IntentPlan
	default:
		return prompt.IntentQuestion
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
		return prompt.StrategyDelegate
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

func inferTools(role AgentRole) []string {
	switch role {
	case RoleResearcher:
		return []string{"glob", "grep", "read", "bash"}
	case RoleCoder:
		return []string{"write", "edit", "bash", "read"}
	case RoleReviewer:
		return []string{"read", "grep"}
	case RoleTester:
		return []string{"write", "bash", "read"}
	case RoleDocumenter:
		return []string{"write", "read"}
	default:
		return []string{"read"}
	}
}