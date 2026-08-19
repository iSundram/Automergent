package prompt

import (
	"time"
)

// RequestCategory categorizes the type of user request.
type RequestCategory string

const (
	CategoryNewFeature   RequestCategory = "new_feature"
	CategoryDebug        RequestCategory = "debug"
	CategoryIssueSuspect RequestCategory = "issue_suspect"
	CategoryUserAsking   RequestCategory = "user_asking"
	CategoryPlan         RequestCategory = "plan"
	CategoryVerifyWork   RequestCategory = "verify_work"
	CategoryDirect       RequestCategory = "direct"
	CategorySimple       RequestCategory = "simple"
	CategoryUnknown      RequestCategory = "unknown"
)

// TaskComplexity represents the complexity of a task.
type TaskComplexity string

const (
	ComplexitySimple   TaskComplexity = "simple"
	ComplexityModerate TaskComplexity = "moderate"
	ComplexityComplex  TaskComplexity = "complex"
)

// ExecutionStrategy defines how a task should be executed.
type ExecutionStrategy string

const (
	StrategyDirect       ExecutionStrategy = "direct"
	StrategyParallel     ExecutionStrategy = "parallel"
	StrategySequential   ExecutionStrategy = "sequential"
	StrategyCoderAgent   ExecutionStrategy = "coder_agent"
	StrategyTodoWalkthrough ExecutionStrategy = "todo_walkthrough"
)

// ContextAction represents an action on context.
type ContextAction string

const (
	ContextActionStash     ContextAction = "stash"
	ContextActionSeparate  ContextAction = "separate"
	ContextActionDelete    ContextAction = "delete"
	ContextActionNew       ContextAction = "new"
	ContextActionResume    ContextAction = "resume"
	ContextActionShare     ContextAction = "share"
)

// ToolSet represents a set of allowed tools.
type ToolSet string

const (
	ToolSetContextOnly ToolSet = "context_only"
	ToolSetReadOnly    ToolSet = "read_only"
	ToolSetBasic       ToolSet = "basic"
	ToolSetModerate    ToolSet = "moderate"
	ToolSetFull        ToolSet = "full"
)

// CategorizedRequest represents a categorized user request.
type CategorizedRequest struct {
	Category       RequestCategory
	Complexity     TaskComplexity
	Strategy       ExecutionStrategy
	AllowedTools   ToolSet
	WorkingAreas   []string
	OriginalPrompt string
	UserIntent     string
	RequiresCoder  bool
	TodoItems      []TodoItem
	ContextNeeds   []ContextNeed
	CreatedAt      time.Time
}

// TodoItem represents a todo item in the workflow.
type TodoItem struct {
	ID           string
	Description  string
	Status       TodoStatus
	Priority     int
	Dependencies []string
	Tools        []string
	ContextKeys  []string
	InjectLater  bool
	Injected     bool
}

// TodoStatus represents the status of a todo item.
type TodoStatus string

const (
	TodoStatusPending   TodoStatus = "pending"
	TodoStatusInProgress TodoStatus = "in_progress"
	TodoStatusCompleted TodoStatus = "completed"
	TodoStatusBlocked   TodoStatus = "blocked"
)

// ContextNeed represents a context requirement.
type ContextNeed struct {
	Key          string
	Description  string
	Required     bool
	Source       ContextSource
	InjectTiming InjectTiming
}

// ContextSource represents where context comes from.
type ContextSource string

const (
	ContextSourceWorkingDir  ContextSource = "working_dir"
	ContextSourceUserPrompt  ContextSource = "user_prompt"
	ContextSourceCodebase    ContextSource = "codebase"
	ContextSourceStashed     ContextSource = "stashed"
	ContextSourceShared      ContextSource = "shared"
	ContextSourceGenerated   ContextSource = "generated"
)

// InjectTiming represents when to inject context.
type InjectTiming string

const (
	InjectTimingImmediate InjectTiming = "immediate"
	InjectTimingDeferred  InjectTiming = "deferred"
	InjectTimingOnDemand  InjectTiming = "on_demand"
)

// ContextStash represents a stashed context with summary.
type ContextStash struct {
	ID          string
	Summary     string
	FullContext string
	CreatedAt   time.Time
	Tags        []string
	Resumable   bool
}

// AssistantContext represents the assistant's context (for talking to user).
type AssistantContext struct {
	ConversationHistory []Message
	UserPreferences     map[string]string
	CurrentTask         *CategorizedRequest
	StashedContexts     []ContextStash
}

// CoderContext represents the coder's context (for coding, separate from assistant).
type CoderContext struct {
	WorkingDir        string
	Files             []string
	CodeSnippets      map[string]string
	Constraints       []string
	TodoItems         []TodoItem
	SharedContext     map[string]string
	ParentAssistantID string
}

// Message represents a conversation message.
type Message struct {
	Role      string
	Content   string
	Timestamp time.Time
	Metadata  map[string]any
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
	StageInitialThinking  PromptStage = "initial_thinking"
	StageCategorization   PromptStage = "categorization"
	StageTaskDefinition   PromptStage = "task_definition"
	StageCoderInit        PromptStage = "coder_init"
	StageWorkflowPlan     PromptStage = "workflow_plan"
	StageExecution        PromptStage = "execution"
	StageContextManage    PromptStage = "context_manage"
	StageTodoInject       PromptStage = "todo_inject"
	StageCompletion       PromptStage = "completion"
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
	
	// Default profile for unknown categories
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
	AssistantModel     string
	CoderModel         string
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
		AssistantModel:     "default",
		CoderModel:         "default",
		MaxContextSize:     100000,
		EnableStashing:     true,
		EnableSharing:      true,
		Verbose:            false,
		TokenBudgetEnabled: true,
		MaxTotalTokens:     80000,
		ReserveTokens:      10000,
	}
}