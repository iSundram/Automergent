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

// Host is the interface that the App must implement to work with the command package.
// This avoids import cycles: command package depends on this interface, App implements it.
type Host interface {
	// Messaging & Status
	AddSystemMessage(text string)
	AddAssistantMessage(text string)
	SetStatus(status string)
	CommandUsage(usage string)
	CommandError(message string)

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
	ToggleLSPPanel()
	ToggleReviewMode()
	IsReviewMode() bool
	SearchWorkspace(query string) string

	// Session lifecycle
	NewSession()
	ShowSessions()
	ResumeSession(id string) error
	ClearConversationView()
	ResetSessionHistory()
	ExportConversation(path string) error
	HandleApprovalsCommand(args []string)

	// UI state queries (palette decoration)
	ShowingFileTree() bool
	DiffPaneVisible() bool
	LSPPanelVisible() bool

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

	// Interactive pickers (full-screen selector overlays)
	OpenRewindPicker()
	OpenPermissionsPicker()
	OpenSettingsPicker()

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
}
