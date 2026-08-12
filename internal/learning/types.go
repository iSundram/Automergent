package learning

import (
	"time"

	"github.com/iSundram/Automergent/internal/ai"
)

// Event represents a learning event captured from user interaction.
type Event struct {
	ID        string                 `json:"id"`
	SessionID string                 `json:"session_id"`
	Timestamp time.Time              `json:"timestamp"`
	Type      EventType              `json:"type"`
	Data      map[string]interface{} `json:"data"`
}

// EventType categorizes learning events.
type EventType string

const (
	EventTypeTaskStart      EventType = "task_start"
	EventTypeTaskComplete   EventType = "task_complete"
	EventTypeTaskFail       EventType = "task_fail"
	EventTypeToolUse        EventType = "tool_use"
	EventTypeToolSuccess    EventType = "tool_success"
	EventTypeToolError      EventType = "tool_error"
	EventTypeUserCorrection EventType = "user_correction"
	EventTypeUserFeedback   EventType = "user_feedback"
	EventTypeStrategySwitch EventType = "strategy_switch"
	EventTypeFileAccess     EventType = "file_access"
	EventTypeCommandExecute EventType = "command_execute"
	EventTypeProviderSwitch EventType = "provider_switch"
	EventTypeContextExpand  EventType = "context_expand"
)

// Strategy represents an adaptive approach to solving tasks.
type Strategy struct {
	ID            string                 `json:"id"`
	Name          string                 `json:"name"`
	Description   string                 `json:"description"`
	ProjectType   string                 `json:"project_type"`
	Framework     string                 `json:"framework,omitempty"`
	SuccessRate   float64                `json:"success_rate"`
	AvgDuration   time.Duration          `json:"avg_duration"`
	UseCount      int                    `json:"use_count"`
	LastUsed      time.Time              `json:"last_used"`
	Configuration map[string]interface{} `json:"configuration"`
}

// KnowledgeEntry represents a learned fact about a project or team.
type KnowledgeEntry struct {
	ID          string                 `json:"id"`
	Scope       KnowledgeScope         `json:"scope"`
	Category    KnowledgeCategory      `json:"category"`
	Key         string                 `json:"key"`
	Value       interface{}            `json:"value"`
	Confidence  float64                `json:"confidence"`
	Source      string                 `json:"source"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
	AccessCount int                    `json:"access_count"`
	Metadata    map[string]interface{} `json:"metadata"`
}

// KnowledgeScope defines the visibility of knowledge.
type KnowledgeScope string

const (
	ScopeLocal   KnowledgeScope = "local"
	ScopeProject KnowledgeScope = "project"
	ScopeTeam    KnowledgeScope = "team"
	ScopeGlobal  KnowledgeScope = "global"
)

// KnowledgeCategory categorizes knowledge entries.
type KnowledgeCategory string

const (
	CategoryConvention   KnowledgeCategory = "convention"
	CategoryPattern      KnowledgeCategory = "pattern"
	CategoryPitfall      KnowledgeCategory = "pitfall"
	CategoryBestPractice KnowledgeCategory = "best_practice"
	CategoryDecision     KnowledgeCategory = "decision"
	CategoryPreference   KnowledgeCategory = "preference"
)

// TimeRange represents a time window.
type TimeRange struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

// CommunicationStyle describes how the user prefers to interact.
type CommunicationStyle string

const (
	StyleConcise   CommunicationStyle = "concise"
	StyleDetailed  CommunicationStyle = "detailed"
	StyleTechnical CommunicationStyle = "technical"
	StyleBeginner  CommunicationStyle = "beginner"
)

// ProjectContext represents learned context about a project.
type ProjectContext struct {
	Path          string                 `json:"path"`
	Type          string                 `json:"type"`
	Language      string                 `json:"language"`
	Framework     string                 `json:"framework,omitempty"`
	BuildSystem   string                 `json:"build_system,omitempty"`
	TestFramework string                 `json:"test_framework,omitempty"`
	Conventions   map[string]interface{} `json:"conventions"`
	Structure     ProjectStructure       `json:"structure"`
	LastAnalyzed  time.Time              `json:"last_analyzed"`
}

// ProjectStructure describes the organization of a project.
type ProjectStructure struct {
	SourceDir      string   `json:"source_dir,omitempty"`
	TestDir        string   `json:"test_dir,omitempty"`
	ConfigFiles    []string `json:"config_files"`
	EntryPoints    []string `json:"entry_points"`
	ImportantFiles []string `json:"important_files"`
}

// Insight represents a high-level insight from learning.
type Insight struct {
	ID          string                 `json:"id"`
	Type        InsightType            `json:"type"`
	Title       string                 `json:"title"`
	Description string                 `json:"description"`
	Confidence  float64                `json:"confidence"`
	Impact      ImpactLevel            `json:"impact"`
	Actionable  bool                   `json:"actionable"`
	Action      string                 `json:"action,omitempty"`
	CreatedAt   time.Time              `json:"created_at"`
	Data        map[string]interface{} `json:"data"`
}

// InsightType categorizes insights.
type InsightType string

const (
	InsightTypePerformance      InsightType = "performance"
	InsightTypeCostOptimization InsightType = "cost_optimization"
	InsightTypeWorkflow         InsightType = "workflow"
	InsightTypeQuality          InsightType = "quality"
	InsightTypeUserBehavior     InsightType = "user_behavior"
)

// ImpactLevel describes the significance of an insight.
type ImpactLevel string

const (
	ImpactLow    ImpactLevel = "low"
	ImpactMedium ImpactLevel = "medium"
	ImpactHigh   ImpactLevel = "high"
)

// TaskOutcome records the result of a task execution.
type TaskOutcome struct {
	TaskID        string                 `json:"task_id"`
	SessionID     string                 `json:"session_id"`
	StartTime     time.Time              `json:"start_time"`
	EndTime       time.Time              `json:"end_time"`
	Duration      time.Duration          `json:"duration"`
	Success       bool                   `json:"success"`
	StrategyUsed  string                 `json:"strategy_used"`
	ToolsUsed     []string               `json:"tools_used"`
	FilesAccessed []string               `json:"files_accessed"`
	TokensUsed    ai.Usage               `json:"tokens_used"`
	ErrorType     string                 `json:"error_type,omitempty"`
	ErrorMessage  string                 `json:"error_message,omitempty"`
	Metadata      map[string]interface{} `json:"metadata"`
}

// Metric represents a tracked metric for continuous improvement.
type Metric struct {
	Name      string                 `json:"name"`
	Value     float64                `json:"value"`
	Timestamp time.Time              `json:"timestamp"`
	Labels    map[string]string      `json:"labels"`
	Metadata  map[string]interface{} `json:"metadata"`
}

// ABTest represents an A/B testing experiment.
type ABTest struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	StartDate   time.Time              `json:"start_date"`
	EndDate     *time.Time             `json:"end_date,omitempty"`
	Variants    []Variant              `json:"variants"`
	Status      ABTestStatus           `json:"status"`
	Results     map[string]interface{} `json:"results,omitempty"`
}

// Variant represents one variation in an A/B test.
type Variant struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Weight      float64                `json:"weight"`
	Config      map[string]interface{} `json:"config"`
	Impressions int                    `json:"impressions"`
	Conversions int                    `json:"conversions"`
}

// ABTestStatus represents the state of an A/B test.
type ABTestStatus string

const (
	ABTestStatusDraft    ABTestStatus = "draft"
	ABTestStatusRunning  ABTestStatus = "running"
	ABTestStatusComplete ABTestStatus = "complete"
	ABTestStatusPaused   ABTestStatus = "paused"
)
