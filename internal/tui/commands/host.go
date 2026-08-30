package commands

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/iSundram/Automergent/internal/ai"
	"github.com/iSundram/Automergent/internal/config"
)

// CheckpointInfo describes a conversation rewind point captured before an
// agent turn. Index is 1-based and chronological.
type CheckpointInfo struct {
	Index    int
	Label    string
	At       time.Time
	Messages int
}

// RecapInfo is a deterministic digest of the current conversation used by
// commands like /recap. It is computed by the host from session internals.
type RecapInfo struct {
	UserTurns       int
	AssistantTurns  int
	ToolCalls       int
	ToolsUsed       []string
	LastUserMessage string
	StartedAt       time.Time
	UpdatedAt       time.Time
}

// APIErrorInfo is one recorded provider API failure, surfaced by /error.
// Retried attempts and terminal failures are both recorded; Retrying tells
// them apart.
type APIErrorInfo struct {
	At time.Time
	// Code is the most specific identifier available: a transport status like
	// "429" when there was one, otherwise a classified code.
	Code string
	// Detail is a short qualifier, e.g. "overloaded", "rate limited".
	Detail string
	// Message is the provider's error text, already sanitized of credentials.
	Message string
	// Suggestion is the provider's remediation hint, when it gave one.
	Suggestion string
	// RequestID correlates the failure with provider-side logs.
	RequestID string

	Provider string
	Model    string

	Attempt     int
	MaxAttempts int
	Retrying    bool
}

// AgentRow is one live subagent in the roster backing /agents.
type AgentRow struct {
	ID      string
	Name    string
	Type    string
	Status  string
	// Activity is what the agent is doing right now ("in grep") or its
	// terminal outcome label.
	Activity string
	Elapsed  string
	// ToolCount and Turns size the work the agent has done.
	ToolCount int
	Turns     int
	Terminal  bool
}

// Host is the interface that the App must implement to work with the command package.
// This avoids import cycles: command package depends on this interface, App implements it.
type Host interface {
	// Messaging & Status
	AddSystemMessage(text string)
	AddAssistantMessage(text string)
	SetStatus(status string)
	CommandUsage(usage string)
	CommandError(message string)
	// AddUserCommandMessage records a prompt-command expansion in the
	// conversation with its command provenance (the "/commit" chip above the
	// expanded prompt body).
	AddUserCommandMessage(command, prompt string)
	// DispatchCommand runs another slash command by name from inside a
	// handler, sub-command or page action — this is the cross-command
	// invocation path. The host owns the depth guard against recursion.
	DispatchCommand(name string, args ...string) error
	// ReloadCustomCommands re-reads markdown custom commands from disk and
	// reports how many are registered after the reload.
	ReloadCustomCommands() int

	// Agent / Workflow
	StartAgent(prompt string) tea.Cmd
	CancelActiveRun(status string)
	Thinking() bool
	CompactContext() tea.Cmd

	// Provider & Model
	Provider() string
	Model() string
	Providers() []string
	SwitchProvider(provider, model string) error
	FetchModels() tea.Cmd
	ModelsAvailable() []ai.Model
	// RefreshModels forces a live re-fetch of the active provider's model
	// list (backs /model refresh).
	RefreshModels() tea.Cmd
	// TestProvider builds a provider from its stored config (without
	// switching the active provider) and runs a live connectivity check
	// against it (backs /provider test). The outcome is delivered
	// asynchronously as a system message.
	TestProvider(provider string) tea.Cmd
	// ProviderAuthSource reports where the provider's API key resolves from
	// ("config", "env NAME", "secret store") or "" when unset. Never reveals
	// the key itself.
	ProviderAuthSource(provider string) string
	// ProviderFallbacks is the configured fallback chain (backs
	// /provider fallback).
	ProviderFallbacks() []config.FallbackProvider
	// SetProviderFallbacks replaces the fallback chain. Callers persist and
	// re-apply the provider afterwards.
	SetProviderFallbacks(fps []config.FallbackProvider)

	// Token / Context
	InputTokens() int
	OutputTokens() int
	TotalCost() float64
	ShowContextDetail()
	ActiveTokens() int

	// Provider Config
	EnsureProviderConfig(provider string)
	ProviderConfig(provider string) config.ProviderConfig
	SetProviderConfig(provider string, pc config.ProviderConfig)
	PersistProjectConfig() error

	// Mode
	Mode() string
	SetMode(mode string)

	// Project UI
	ToggleFileTree()
	ToggleDiffPane()
	ToggleReviewMode()
	IsReviewMode() bool
	SearchWorkspace(query string) string

	// Session lifecycle
	NewSession()
	ShowSessions()
	// ShowArtifacts opens the /artifact review browser for artifacts the
	// agent produced this session.
	ShowArtifacts()
	ResumeSession(id string) error
	// DeleteSession removes a stored session. Implementations must refuse
	// the active session.
	DeleteSession(id string) error
	ExportConversation(path string) error
	HandleApprovalsCommand(args []string)

	// UI state queries (palette decoration)
	ShowingFileTree() bool
	DiffPaneVisible() bool

	// Environment & diagnostics
	WorkDir() string
	SessionID() string
	SessionTitle() string
	RenameSession(title string) error
	Version() string
	SandboxStatus() (kind string, available bool)
	GlobalConfigPath() string
	ProjectConfigPath() string
	ValidateConfig() []string
	StorageHealth() error

	// Recap is a deterministic digest of the current conversation, computed by
	// the host so session internals stay out of this package.
	RecapSnapshot() RecapInfo

	// Goal autonomy (backs /goal): SetGoal installs an objective with an
	// optional token budget (0 = none); GoalSnapshot renders current state;
	// GoalAction applies pause/resume/continue/clear and returns a
	// user-facing result message.
	SetGoal(objective string, tokenBudget int)
	GoalSnapshot() string
	GoalAction(action string) string

	// RecentFilePaths lists workspace files touched recently (diff-pane
	// tabs), backing path-gated command visibility (Command.Paths).
	RecentFilePaths() []string
	// StartForkedAgent runs a prompt in a background subagent with its own
	// context; the result arrives asynchronously as a system message
	// (backs Fork prompt-commands).
	StartForkedAgent(command, prompt string)

	// Conversation history: rewind points and branching
	Checkpoints() []CheckpointInfo
	RewindTo(index int) error
	BranchSession(name string) error

	// Context & workspace extras
	ContextFiles() []string
	AddSearchDir(path string) error
	ExtraSearchDirs() []string

	// Usage & policy introspection
	SessionTokenTotals() (sessions int, totalIn, totalOut int)
	SecurityPaths() (blocked, allowed []string)

	// API error history (backs /error)
	APIErrors() []APIErrorInfo
	ClearAPIErrors()

	// Interactive pickers (full-screen selector overlays)
	OpenRewindPicker()
	OpenPermissionsPicker()
	OpenSettingsPicker()

	// Live subagents (backs /agents)
	AgentRoster() []AgentRow
	OpenAgentView(agentID string)

	// Stats & Help
	ShowStats()
	ShowHelp()

	// New config commands
	SetTheme(name string) error
	AvailableThemes() []string
	CurrentTheme() string
	SetKeybindings(scheme string) error
	AvailableKeybindings() []string
	CurrentKeybindings() string

	// Context for background work
	Context() context.Context

	// MCP (Model Context Protocol)
	MCPStatus() []MCPServerStatus
	MCPTools(server string) []MCPToolInfo
	MCPResources() []MCPResourceInfo
	MCPPrompts() []MCPPromptInfo
	MCPReconnect(name string) error
	MCPRefresh(name string) error
	MCPEnable(name string) error
	MCPDisable(name string) error
	MCPCallTool(server, name string, args map[string]any) (string, error)
	MCPEvents() []MCPEventInfo
	MCPDeleteToolCache(pattern string)
	MCPAddServer(name, transport, urlOrCmd string, args []string) error
	MCPRemoveServer(name string) error
}

// MCPServerStatus is the status info for a connected MCP server.
type MCPServerStatus struct {
	Name      string
	Transport string
	Status    string
	Version   string
	Tools     int
	Resources int
	Prompts   int
	Latency   string
	LastError string
	Connected bool
}

// MCPToolInfo describes an MCP tool for the command layer.
type MCPToolInfo struct {
	Name        string
	Description string
	Server      string
	ReadOnly    bool
	Destructive bool
	Schema      string
}

// MCPResourceInfo describes an MCP resource.
type MCPResourceInfo struct {
	URI         string
	Name        string
	Description string
	MimeType    string
	Server      string
}

// MCPPromptInfo describes an MCP prompt.
type MCPPromptInfo struct {
	Name        string
	Description string
	Server      string
}

// MCPEventInfo is a recorded MCP event for /mcp events.
type MCPEventInfo struct {
	Type      string
	Server    string
	Message   string
	Error     string
	Timestamp string
}
