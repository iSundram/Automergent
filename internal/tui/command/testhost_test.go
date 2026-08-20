package command

import (
	"context"
	"sync"

	tea "charm.land/bubbletea/v2"

	"github.com/iSundram/Automergent/internal/ai"
	"github.com/iSundram/Automergent/internal/config"
)

// mockHost implements Host for testing command handlers.
type mockHost struct {
	mu sync.Mutex

	// State
	provider    string
	model       string
	providers   []string
	models      []ai.Model
	mode        string
	theme       string
	keybindings string
	thinking    bool

	// Call recording
	systemMessages    []string
	assistantMessages []string
	statusMessages    []string
	usageMessages     []string
	errorMessages     []string

	startAgentCalls    []string
	cancelActiveRuns   []string
	compactContextCalls int

	switchProviderCalls []struct{ provider, model string }
	fetchModelsCalls    int

	ensureProviderConfigCalls []string
	providerConfigs           map[string]config.ProviderConfig
	persistProjectConfigCalls int

	setModeCalls []string

	toggleFileTreeCalls int
	toggleDiffPaneCalls int
	toggleLSPPanelCalls int
	toggleReviewModeCalls int
	searchWorkspaceCalls []string

	exportConversationCalls []string
	handleApprovalsCalls   [][]string

	showStatsCalls int
	showHelpCalls  int

	setThemeCalls []string
	setKeybindingCalls []string

	ctx context.Context
}

// NewMockHost creates a new mock host for testing.
func NewMockHost() *mockHost {
	return &mockHost{
		provider:      "google",
		model:         "gemini-3.6-flash",
		providers:     []string{"google"},
		models:        []ai.Model{{ID: "gemini-3.6-flash", ContextLimit: 1000000}},
		mode:          "edit",
		theme:         "modern",
		keybindings:   "default",
		providerConfigs: map[string]config.ProviderConfig{
			"google": {APIKey: "test-key"},
		},
		ctx: context.Background(),
	}
}

// Reset clears all recorded calls.
func (m *mockHost) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.systemMessages = nil
	m.assistantMessages = nil
	m.statusMessages = nil
	m.usageMessages = nil
	m.errorMessages = nil
	m.startAgentCalls = nil
	m.cancelActiveRuns = nil
	m.compactContextCalls = 0
	m.switchProviderCalls = nil
	m.fetchModelsCalls = 0
	m.ensureProviderConfigCalls = nil
	m.persistProjectConfigCalls = 0
	m.setModeCalls = nil
	m.toggleFileTreeCalls = 0
	m.toggleDiffPaneCalls = 0
	m.toggleLSPPanelCalls = 0
	m.toggleReviewModeCalls = 0
	m.searchWorkspaceCalls = nil
	m.exportConversationCalls = nil
	m.handleApprovalsCalls = nil
	m.showStatsCalls = 0
	m.showHelpCalls = 0
	m.setThemeCalls = nil
	m.setKeybindingCalls = nil
}

// --- Host interface implementation ---

func (m *mockHost) AddSystemMessage(text string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.systemMessages = append(m.systemMessages, text)
}

func (m *mockHost) AddAssistantMessage(text string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.assistantMessages = append(m.assistantMessages, text)
}

func (m *mockHost) SetStatus(status string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.statusMessages = append(m.statusMessages, status)
}

func (m *mockHost) CommandUsage(usage string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.usageMessages = append(m.usageMessages, usage)
}

func (m *mockHost) CommandError(message string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.errorMessages = append(m.errorMessages, message)
}

func (m *mockHost) StartAgent(prompt string) tea.Cmd {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.startAgentCalls = append(m.startAgentCalls, prompt)
	return nil
}

func (m *mockHost) CancelActiveRun(status string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cancelActiveRuns = append(m.cancelActiveRuns, status)
}

func (m *mockHost) Thinking() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.thinking
}

func (m *mockHost) CompactContext() tea.Cmd {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.compactContextCalls++
	return nil
}

func (m *mockHost) Provider() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.provider
}

func (m *mockHost) Model() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.model
}

func (m *mockHost) Providers() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.providers
}

func (m *mockHost) SwitchProvider(provider, model string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.switchProviderCalls = append(m.switchProviderCalls, struct{ provider, model string }{provider, model})
	m.provider = provider
	if model != "" {
		m.model = model
	} else {
		m.model = "gemini-3.6-flash"
	}
	return nil
}

func (m *mockHost) FetchModels() tea.Cmd {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.fetchModelsCalls++
	return nil
}

func (m *mockHost) ModelsAvailable() []ai.Model {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.models
}

func (m *mockHost) InputTokens() int {
	return 100
}

func (m *mockHost) OutputTokens() int {
	return 50
}

func (m *mockHost) TotalCost() float64 {
	return 0.001
}

func (m *mockHost) ShowContextDetail() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.systemMessages = append(m.systemMessages, "[detail] context detail")
}

func (m *mockHost) ActiveTokens() int {
	return 150
}

func (m *mockHost) EnsureProviderConfig(provider string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ensureProviderConfigCalls = append(m.ensureProviderConfigCalls, provider)
	if _, ok := m.providerConfigs[provider]; !ok {
		m.providerConfigs[provider] = config.ProviderConfig{}
	}
}

func (m *mockHost) ProviderConfig(provider string) config.ProviderConfig {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.providerConfigs[provider]
}

func (m *mockHost) SetProviderConfig(provider string, pc config.ProviderConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.providerConfigs[provider] = pc
}

func (m *mockHost) PersistProjectConfig() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.persistProjectConfigCalls++
	return nil
}

func (m *mockHost) Mode() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.mode
}

func (m *mockHost) SetMode(mode string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.setModeCalls = append(m.setModeCalls, mode)
	m.mode = mode
}

func (m *mockHost) ToggleFileTree() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.toggleFileTreeCalls++
}

func (m *mockHost) ToggleDiffPane() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.toggleDiffPaneCalls++
}

func (m *mockHost) ToggleLSPPanel() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.toggleLSPPanelCalls++
}

func (m *mockHost) ToggleReviewMode() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.toggleReviewModeCalls++
}

func (m *mockHost) IsReviewMode() bool {
	return false
}

func (m *mockHost) SearchWorkspace(query string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.searchWorkspaceCalls = append(m.searchWorkspaceCalls, query)
	return "search results for: " + query
}

func (m *mockHost) ExportConversation(path string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.exportConversationCalls = append(m.exportConversationCalls, path)
	return nil
}

func (m *mockHost) HandleApprovalsCommand(args []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.handleApprovalsCalls = append(m.handleApprovalsCalls, args)
}

func (m *mockHost) ShowStats() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.showStatsCalls++
}

func (m *mockHost) ShowHelp() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.showHelpCalls++
}

func (m *mockHost) SetTheme(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.setThemeCalls = append(m.setThemeCalls, name)
	m.theme = name
	return nil
}

func (m *mockHost) AvailableThemes() []string {
	return []string{"catppuccin", "dracula", "nord", "modern"}
}

func (m *mockHost) CurrentTheme() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.theme
}

func (m *mockHost) SetKeybindings(scheme string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.setKeybindingCalls = append(m.setKeybindingCalls, scheme)
	m.keybindings = scheme
	return nil
}

func (m *mockHost) AvailableKeybindings() []string {
	return []string{"default", "vim", "emacs"}
}

func (m *mockHost) CurrentKeybindings() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.keybindings
}

func (m *mockHost) Context() context.Context {
	return m.ctx
}