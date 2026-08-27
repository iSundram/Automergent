package agentdef

import (
	"time"
)

// AgentType defines the type of agent.
type AgentType string

const (
	AgentTypeGeneral     AgentType = "general-purpose"
	AgentTypeExplore     AgentType = "explore"
	AgentTypeReview      AgentType = "review"
	AgentTypeContexter   AgentType = "contexter"
	AgentTypeCoordinator AgentType = "coordinator"
	AgentTypeCustom      AgentType = "custom"
)

// AgentSource indicates where an agent definition originated.
type AgentSource string

const (
	SourceBuiltin AgentSource = "builtin"
	SourceUser    AgentSource = "user"
	SourceProject AgentSource = "project"
	SourcePlugin  AgentSource = "plugin"
)

// AgentEffort controls token budget allocation.
type AgentEffort string

const (
	EffortLow    AgentEffort = "low"
	EffortMedium AgentEffort = "medium"
	EffortHigh   AgentEffort = "high"
)

// MemoryScope controls what context memory the agent can access.
type MemoryScope string

const (
	MemoryScopeGlobal  MemoryScope = "global"
	MemoryScopeProject MemoryScope = "project"
	MemoryScopeNone    MemoryScope = "none"
)

// AgentDefinition describes an agent's capabilities, constraints, and behavior.
type AgentDefinition struct {
	Name         string        `json:"name"`
	Description  string        `json:"description"`
	WhenToUse    string        `json:"when_to_use"`
	SystemPrompt string        `json:"system_prompt"`
	Model        string        `json:"model,omitempty"`
	Tools        []string      `json:"tools,omitempty"`
	Color        string        `json:"color,omitempty"`
	Effort       AgentEffort   `json:"effort,omitempty"`
	Source       AgentSource   `json:"source"`
	MemoryScope  MemoryScope   `json:"memory_scope,omitempty"`
	MaxTokens    int           `json:"max_tokens,omitempty"`
	Timeout      time.Duration `json:"timeout,omitempty"`
}

// AgentConfig holds runtime configuration for agent execution.
type AgentConfig struct {
	Definition    *AgentDefinition
	Model         string
	WorkDir       string
	ParentAgentID string
	IsChild       bool
	StreamEvents  bool
}

// AgentStatus represents the current state of an agent.
type AgentStatus string

const (
	AgentStatusRunning   AgentStatus = "running"
	AgentStatusIdle      AgentStatus = "idle"
	AgentStatusCompleted AgentStatus = "completed"
	AgentStatusFailed    AgentStatus = "failed"
	AgentStatusCancelled AgentStatus = "cancelled"
)

// AgentResult holds the outcome of an agent execution.
type AgentResult struct {
	Output     string
	Error      error
	Duration   time.Duration
	TokensUsed int
	ToolCalls  int
}

// AgentFilter for querying agents.
type AgentFilter struct {
	Types  []AgentType
	Status []AgentStatus
	Limit  int
}
