package prompt

import (
	"time"

	"github.com/iSundram/Automergent/internal/shared"
)

// Re-export shared types for backward compatibility within this package
type RequestCategory = shared.RequestCategory
type TaskComplexity = shared.TaskComplexity
type ExecutionStrategy = shared.ExecutionStrategy
type ContextAction = shared.ContextAction
type ToolSet = shared.ToolSet
type TodoItem = shared.TodoItem
type TodoStatus = shared.TodoStatus
type ContextNeed = shared.ContextNeed
type ContextSource = shared.ContextSource
type InjectTiming = shared.InjectTiming
type IntentType = shared.IntentType
type Intent = shared.Intent
type IntentSet = shared.IntentSet
type InitPhase = shared.InitPhase
type InitAction = shared.InitAction
type InitActionStatus = shared.InitActionStatus
type InitResults = shared.InitResults
type TaskSpec = shared.TaskSpec
type ContextStash = shared.ContextStash
type Message = shared.Message

const (
	CategoryNewFeature   = shared.CategoryNewFeature
	CategoryDebug        = shared.CategoryDebug
	CategoryIssueSuspect = shared.CategoryIssueSuspect
	CategoryUserAsking   = shared.CategoryUserAsking
	CategoryPlan         = shared.CategoryPlan
	CategoryVerifyWork   = shared.CategoryVerifyWork
	CategoryDirect       = shared.CategoryDirect
	CategorySimple       = shared.CategorySimple
	CategoryUnknown      = shared.CategoryUnknown

	ComplexitySimple   = shared.ComplexitySimple
	ComplexityModerate = shared.ComplexityModerate
	ComplexityComplex  = shared.ComplexityComplex

	StrategyDirect          = shared.StrategyDirect
	StrategyParallel        = shared.StrategyParallel
	StrategySequential      = shared.StrategySequential
	StrategyDelegate        = shared.StrategyDelegate
	StrategyTodoWalkthrough = shared.StrategyTodoWalkthrough

	ContextActionStash    = shared.ContextActionStash
	ContextActionSeparate = shared.ContextActionSeparate
	ContextActionDelete   = shared.ContextActionDelete
	ContextActionNew      = shared.ContextActionNew
	ContextActionResume   = shared.ContextActionResume
	ContextActionShare    = shared.ContextActionShare

	ToolSetContextOnly = shared.ToolSetContextOnly
	ToolSetReadOnly    = shared.ToolSetReadOnly
	ToolSetBasic       = shared.ToolSetBasic
	ToolSetModerate    = shared.ToolSetModerate
	ToolSetFull        = shared.ToolSetFull

	TodoStatusPending    = shared.TodoStatusPending
	TodoStatusInProgress = shared.TodoStatusInProgress
	TodoStatusCompleted  = shared.TodoStatusCompleted
	TodoStatusBlocked    = shared.TodoStatusBlocked

	ContextSourceWorkingDir = shared.ContextSourceWorkingDir
	ContextSourceUserPrompt = shared.ContextSourceUserPrompt
	ContextSourceCodebase   = shared.ContextSourceCodebase
	ContextSourceStashed    = shared.ContextSourceStashed
	ContextSourceShared     = shared.ContextSourceShared
	ContextSourceGenerated  = shared.ContextSourceGenerated

	InjectTimingImmediate = shared.InjectTimingImmediate
	InjectTimingDeferred  = shared.InjectTimingDeferred
	InjectTimingOnDemand  = shared.InjectTimingOnDemand

	IntentExplore   = shared.IntentExplore
	IntentImplement = shared.IntentImplement
	IntentFix       = shared.IntentFix
	IntentTest      = shared.IntentTest
	IntentCommit    = shared.IntentCommit
	IntentReview    = shared.IntentReview
	IntentDocument  = shared.IntentDocument
	IntentRefactor  = shared.IntentRefactor
	IntentDebug     = shared.IntentDebug
	IntentQuestion  = shared.IntentQuestion
	IntentPlan      = shared.IntentPlan
	IntentDirect    = shared.IntentDirect

	InitActionPending    = shared.InitActionPending
	InitActionInProgress = shared.InitActionInProgress
	InitActionCompleted  = shared.InitActionCompleted
	InitActionFailed     = shared.InitActionFailed
)

// Prompt-specific types (not in shared)

// CategorizedRequest represents a categorized user request.
type CategorizedRequest struct {
	Category       RequestCategory
	Relation       RequestRelation
	ContextShare   ContextShareMode
	Complexity     TaskComplexity
	Strategy       ExecutionStrategy
	AllowedTools   ToolSet
	WorkingAreas   []string
	OriginalPrompt string
	UserIntent     string
	RequiresCoder  bool
	TodoItems      []shared.TodoItem
	ContextNeeds   []shared.ContextNeed
	CreatedAt      time.Time
}

type RequestRelation string

const (
	RequestRelationNew      RequestRelation = "new_task"
	RequestRelationFollowUp RequestRelation = "follow_up"
	RequestRelationRelated  RequestRelation = "related"
)

type ContextShareMode string

const (
	ContextShareNone    ContextShareMode = "none"
	ContextShareSummary ContextShareMode = "summary"
	ContextSharePartial ContextShareMode = "partial"
	ContextShareFull    ContextShareMode = "full"
)

// TurnContext is the single unified working context for the agent.
// There is no assistant/coder split: one persona, one context, one loop.
// It carries everything the current turn needs — conversation state,
// discovered files/snippets, constraints, todos and shared scratch space.
type TurnContext struct {
	WorkingDir          string
	Files               []string
	CodeSnippets        map[string]string
	Constraints         []string
	TodoItems           []shared.TodoItem
	SharedContext       map[string]string
	ConversationHistory []shared.Message
	UserPreferences     map[string]string
	CurrentTask         *CategorizedRequest
	StashedContexts     []ContextStash
}

// PromptPart represents a part of a prompt sent at a specific stage.
type PromptPart struct {
	Stage      PromptStage
	Content    string
	Metadata   map[string]any
	Tools      ToolSet
	ContextKey string
}

// PromptStage represents the stage of prompt delivery.
type PromptStage string

const (
	StageInitialThinking PromptStage = "initial_thinking"
	StageCategorization  PromptStage = "categorization"
	StageTaskDefinition  PromptStage = "task_definition"
	StageTaskInit        PromptStage = "task_init"
	StageWorkflowPlan    PromptStage = "workflow_plan"
	StageExecution       PromptStage = "execution"
	StageContextManage   PromptStage = "context_manage"
	StageTodoInject      PromptStage = "todo_inject"
	StageCompletion      PromptStage = "completion"
)

// ContextProfile defines what context to include for a specific category.
type ContextProfile struct {
	Category              RequestCategory
	MaxFiles              int
	IncludeSymbols        bool
	IncludeDependencies   bool
	IncludeRecentFiles    bool
	IncludeFrequentFiles  bool
	IncludeStashedContext bool
	IncludeProjectContext bool
	TokenBudget           int
	MinRelevanceScore     float64
}

// GetContextProfile returns the context profile for a category.
func GetContextProfile(category RequestCategory) ContextProfile {
	profiles := map[RequestCategory]ContextProfile{
		CategoryNewFeature: {
			Category:              CategoryNewFeature,
			MaxFiles:              15,
			IncludeSymbols:        true,
			IncludeDependencies:   true,
			IncludeRecentFiles:    true,
			IncludeFrequentFiles:  true,
			IncludeStashedContext: true,
			IncludeProjectContext: true,
			TokenBudget:           30000,
			MinRelevanceScore:     0.3,
		},
		CategoryDebug: {
			Category:              CategoryDebug,
			MaxFiles:              10,
			IncludeSymbols:        true,
			IncludeDependencies:   true,
			IncludeRecentFiles:    true,
			IncludeFrequentFiles:  false,
			IncludeStashedContext: true,
			IncludeProjectContext: false,
			TokenBudget:           25000,
			MinRelevanceScore:     0.4,
		},
		CategoryIssueSuspect: {
			Category:              CategoryIssueSuspect,
			MaxFiles:              12,
			IncludeSymbols:        true,
			IncludeDependencies:   true,
			IncludeRecentFiles:    true,
			IncludeFrequentFiles:  false,
			IncludeStashedContext: true,
			IncludeProjectContext: false,
			TokenBudget:           20000,
			MinRelevanceScore:     0.35,
		},
		CategoryUserAsking: {
			Category:              CategoryUserAsking,
			MaxFiles:              5,
			IncludeSymbols:        false,
			IncludeDependencies:   false,
			IncludeRecentFiles:    true,
			IncludeFrequentFiles:  false,
			IncludeStashedContext: false,
			IncludeProjectContext: true,
			TokenBudget:           10000,
			MinRelevanceScore:     0.2,
		},
		CategoryPlan: {
			Category:              CategoryPlan,
			MaxFiles:              20,
			IncludeSymbols:        true,
			IncludeDependencies:   true,
			IncludeRecentFiles:    true,
			IncludeFrequentFiles:  true,
			IncludeStashedContext: true,
			IncludeProjectContext: true,
			TokenBudget:           35000,
			MinRelevanceScore:     0.25,
		},
		CategoryVerifyWork: {
			Category:              CategoryVerifyWork,
			MaxFiles:              15,
			IncludeSymbols:        true,
			IncludeDependencies:   false,
			IncludeRecentFiles:    false,
			IncludeFrequentFiles:  false,
			IncludeStashedContext: true,
			IncludeProjectContext: false,
			TokenBudget:           20000,
			MinRelevanceScore:     0.4,
		},
		CategoryDirect: {
			Category:              CategoryDirect,
			MaxFiles:              3,
			IncludeSymbols:        false,
			IncludeDependencies:   false,
			IncludeRecentFiles:    false,
			IncludeFrequentFiles:  false,
			IncludeStashedContext: false,
			IncludeProjectContext: false,
			TokenBudget:           5000,
			MinRelevanceScore:     0.5,
		},
		CategorySimple: {
			Category:              CategorySimple,
			MaxFiles:              2,
			IncludeSymbols:        false,
			IncludeDependencies:   false,
			IncludeRecentFiles:    false,
			IncludeFrequentFiles:  false,
			IncludeStashedContext: false,
			IncludeProjectContext: false,
			TokenBudget:           3000,
			MinRelevanceScore:     0.6,
		},
	}

	if profile, ok := profiles[category]; ok {
		return profile
	}

	return ContextProfile{
		Category:              CategoryUnknown,
		MaxFiles:              10,
		IncludeSymbols:        true,
		IncludeDependencies:   true,
		IncludeRecentFiles:    true,
		IncludeFrequentFiles:  true,
		IncludeStashedContext: true,
		IncludeProjectContext: true,
		TokenBudget:           20000,
		MinRelevanceScore:     0.3,
	}
}

// PromptConfig holds configuration for prompt generation.
type PromptConfig struct {
	PlanningModel      string
	MaxContextSize     int
	EnableStashing     bool
	EnableSharing      bool
	Verbose            bool
	TokenBudgetEnabled bool
	MaxTotalTokens     int
	ReserveTokens      int
}

// DefaultPromptConfig returns default configuration.
func DefaultPromptConfig() *PromptConfig {
	return &PromptConfig{
		PlanningModel:      "default",
		MaxContextSize:     100000,
		EnableStashing:     true,
		EnableSharing:      true,
		Verbose:            false,
		TokenBudgetEnabled: true,
		MaxTotalTokens:     80000,
		ReserveTokens:      10000,
	}
}
