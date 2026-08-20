package command

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/iSundram/Automergent/internal/ai"
	"github.com/iSundram/Automergent/internal/config"
)

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

	// Session-ish (existing methods from command_handlers_session.go - NOT TOUCHED)
	ExportConversation(path string) error
	HandleApprovalsCommand(args []string)

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