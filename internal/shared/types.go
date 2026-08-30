package shared

import "time"

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
	StrategyDirect          ExecutionStrategy = "direct"
	StrategyParallel        ExecutionStrategy = "parallel"
	StrategySequential      ExecutionStrategy = "sequential"
	StrategyDelegate        ExecutionStrategy = "delegate"
	StrategyTodoWalkthrough ExecutionStrategy = "todo_walkthrough"
)

// ContextAction represents an action on context.
type ContextAction string

const (
	ContextActionStash    ContextAction = "stash"
	ContextActionSeparate ContextAction = "separate"
	ContextActionDelete   ContextAction = "delete"
	ContextActionNew      ContextAction = "new"
	ContextActionResume   ContextAction = "resume"
	ContextActionShare    ContextAction = "share"
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

// TodoStatus represents the status of a todo item.
type TodoStatus string

const (
	TodoStatusPending    TodoStatus = "pending"
	TodoStatusInProgress TodoStatus = "in_progress"
	TodoStatusCompleted  TodoStatus = "completed"
	TodoStatusBlocked    TodoStatus = "blocked"
)

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
	ContextSourceWorkingDir ContextSource = "working_dir"
	ContextSourceUserPrompt ContextSource = "user_prompt"
	ContextSourceCodebase   ContextSource = "codebase"
	ContextSourceStashed    ContextSource = "stashed"
	ContextSourceShared     ContextSource = "shared"
	ContextSourceGenerated  ContextSource = "generated"
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

// IntentType represents the type of intent identified in a user message.
type IntentType string

const (
	IntentExplore   IntentType = "explore"
	IntentImplement IntentType = "implement"
	IntentFix       IntentType = "fix"
	IntentTest      IntentType = "test"
	IntentCommit    IntentType = "commit"
	IntentReview    IntentType = "review"
	IntentDocument  IntentType = "document"
	IntentRefactor  IntentType = "refactor"
	IntentDebug     IntentType = "debug"
	IntentQuestion  IntentType = "question"
	IntentPlan      IntentType = "plan"
	IntentDirect    IntentType = "direct"
)

// Intent represents a single identified intent from the user message.
type Intent struct {
	ID           string
	Type         IntentType
	Priority     int
	Dependencies []string
	Parameters   map[string]any
	RawText      string
	Confidence   float64
}

// IntentSet represents a collection of intents identified from a message.
type IntentSet struct {
	Intents        []Intent
	RequiresInit   bool
	InitPhase      *InitPhase
	OriginalPrompt string
}

// InitPhase represents the exploration phase before task execution.
type InitPhase struct {
	ID              string
	Actions         []InitAction
	Goal            string
	SuccessCriteria []string
	Results         *InitResults
}

// InitAction represents a single exploration action.
type InitAction struct {
	ID     string
	Tool   string
	Target string
	Reason string
	Status InitActionStatus
	Result string
}

// InitActionStatus represents the status of an init action.
type InitActionStatus string

const (
	InitActionPending    InitActionStatus = "pending"
	InitActionInProgress InitActionStatus = "in_progress"
	InitActionCompleted  InitActionStatus = "completed"
	InitActionFailed     InitActionStatus = "failed"
)

// InitActionEvent surfaces one init-phase tool execution to the UI as a
// first-class log entry. Emitted twice per action: Running=true on start,
// Running=false on completion (with Summary/Duration/Failed populated).
type InitActionEvent struct {
	Tool     string // normalized native tool name: read_file|grep|glob|bash
	RawTool  string // pipeline-side name: read|grep|glob|bash
	Target   string
	Running  bool
	Failed   bool
	Err      string
	Summary  string
	Duration time.Duration
}

// InitResults holds the results of the initialization phase.
type InitResults struct {
	FilesFound   []string
	CodeSnippets map[string]string
	Errors       []string
	Summary      string
	CompletedAt  time.Time
}

// TaskSpec represents a generated task specification.
type TaskSpec struct {
	ID           string
	IntentID     string
	Type         string
	Role         string
	Priority     int
	Dependencies []string
	Prompt       string
	Context      map[string]any
	Tools        []string
	Description  string
	// Agent is the subagent type the INIT decomposer routed this task to:
	// "main" (inline in the main agent), "explore", "general-purpose",
	// "review", "contexter", or "coordinator". Empty behaves as "main".
	Agent string
	// Phase is the arc phase this task runs in: explore | plan | build.
	// Empty means infer from Type.
	Phase AgentPhase
}

// PromptPart represents a part of a prompt sent at a specific stage.
type PromptPart struct {
	Stage      PromptStage
	Content    string
	Metadata   map[string]any
	Tools      ToolSet
	ContextKey string
}

// Message represents a conversation message.
type Message struct {
	Role      string
	Content   string
	Timestamp time.Time
	Metadata  map[string]any
}

// ============================================================================
// PHASE-AWARE AGENT LOOP TYPES
// ============================================================================

// AgentPhase represents the current phase of the agent loop.
type AgentPhase string

const (
	PhaseInit     AgentPhase = "init"      // First message: classify, route, handle direct Q&A
	PhaseExplore  AgentPhase = "explore"   // Codebase search/read (read-only tools)
	PhasePlan     AgentPhase = "plan"      // Design, clarify, create task graph
	PhaseBuild    AgentPhase = "build"     // Implementation + testing + todo (full tools)
)

// PromptLayer represents a layer in the composed system prompt.
type PromptLayer int

const (
	LayerBaseModel      PromptLayer = iota // Model-specific base (gpt.txt, claude.txt, gemini.txt)
	LayerEnvironment                        // Working dir, git, platform, date, model info
	LayerInstructions                       // AGENTS.md, CLAUDE.md, custom instructions
	LayerSkills                             // Available skills with descriptions
	LayerMCP                                // MCP server instructions
	LayerAgentCustom                        // Per-agent custom prompt from config
	LayerPhase                              // Phase-specific (init/explore/plan/build)
	LayerBehavioral                         // Behavioral rules (violations, tone, etc.)
	LayerTool                               // Per-tool prompts (pre/post execution)
	LayerDynamic                            // Plan mode reminders, compaction notices
)

// PhaseConfig holds tool and behavior config for a specific phase.
type PhaseConfig struct {
	Tools       []string
	ToolSet     ToolSet
	PromptStyle string
	Agent       string
	MaxSteps    int
}

// ViolationType represents the type of policy violation.
type ViolationType string

const (
	ViolationHacking      ViolationType = "hacking"
	ViolationIllegal      ViolationType = "illegal"
	ViolationHarmful      ViolationType = "harmful_code"
	ViolationCredentials  ViolationType = "credential_theft"
	ViolationSecurity     ViolationType = "security_bypass"
	ViolationPersistence  ViolationType = "persistent_violation"
)

// ViolationSeverity represents the severity of a violation.
type ViolationSeverity string

const (
	ViolationSeverityLow      ViolationSeverity = "low"
	ViolationSeverityMedium   ViolationSeverity = "medium"
	ViolationSeverityHigh     ViolationSeverity = "high"
	ViolationSeverityCritical ViolationSeverity = "critical"
)

// ViolationCheck represents a detected violation.
type ViolationCheck struct {
	Type        ViolationType
	Severity    ViolationSeverity
	UserMessage string
	AgentResponse string
	Count       int
	Action      string // "warn", "block_imminent", "blocked", "overridden"
}

// PhaseTransition represents a transition between phases.
type PhaseTransition struct {
	From      AgentPhase
	To        AgentPhase
	Reason    string
	Trigger   string // "user_request", "tool_result", "violation", "clarification"
	Timestamp time.Time
}

// BehavioralTrigger represents when a behavioral prompt should be injected.
type BehavioralTrigger string

const (
	TriggerAlways         BehavioralTrigger = "always"
	TriggerViolation      BehavioralTrigger = "violation"
	TriggerClarification  BehavioralTrigger = "clarification"
	TriggerPhaseChange    BehavioralTrigger = "phase_change"
	TriggerToolUse        BehavioralTrigger = "tool_use"
	TriggerError          BehavioralTrigger = "error"
)

// BehavioralPrompt represents a behavioral rule prompt.
type BehavioralPrompt struct {
	Name     string
	Content  string
	Priority int
	Triggers []BehavioralTrigger
}

// ToolPromptConfig holds per-tool behavioral prompts.
type ToolPromptConfig struct {
	PreExecution    string
	PostExecution   string
	Rules           []string
	RequiresContext []string
}

// ViolationPolicy defines how an agent handles violations.
type ViolationPolicy struct {
	MaxWarnings    int  // Before block (default 2)
	BlockOnPersist bool
	AllowOverride  bool // User can convince
}

// ModelInfo holds model-specific information.
type ModelInfo struct {
	Name           string
	Provider       string
	ContextWindow  int
	MaxOutputTokens int
	SupportsThinking bool
}
