package commands

import (
	"context"
	"errors"
	"strings"
	"sync"

	tea "charm.land/bubbletea/v2"

	"github.com/iSundram/Automergent/internal/ai"
	"github.com/iSundram/Automergent/internal/config"
)

// goalCall records one /goal SetGoal invocation.
type goalCall struct {
	objective   string
	tokenBudget int
}

// forkedAgentCall records one StartForkedAgent invocation.
type forkedAgentCall struct {
	command string
	prompt  string
}

// runWorkflowCall records one RunWorkflow invocation.
type runWorkflowCall struct {
	path   string
	args   string
	resume bool
}

// mockHost implements Host for testing command handlers.
type mockHost struct {
	mu sync.Mutex

	// State
	provider     string
	model        string
	providers    []string
	models       []ai.Model
	mode         string
	theme        string
	keybindings  string
	thinking     bool
	defaultModel string
	apiKey       string

	// Call recording
	systemMessages    []string
	assistantMessages []string
	statusMessages    []string
	usageMessages     []string
	errorMessages     []string

	startAgentCalls     []string
	cancelActiveRuns    []string
	compactContextCalls int
	blocksExpanded      bool

	// Cross-command invocation (DispatchCommand) and custom reload state.
	commandMessages   [][2]string
	dispatchDepth     int
	reloadCustomCalls int
	reloadCustomCount int
	registry          *Registry

	// MCP state
	mcpServers          []MCPServerStatus
	mcpTools            []MCPToolInfo
	mcpResources        []MCPResourceInfo
	mcpPrompts          []MCPPromptInfo
	mcpEvents           []MCPEventInfo
	mcpReconnectCalls   []string
	mcpRefreshCalls     []string
	mcpEnableCalls      []string
	mcpDisableCalls     []string
	mcpAddCalls         []string
	mcpRemoveCalls      []string
	mcpCacheDeleteCalls int

	switchProviderCalls []struct{ provider, model string }
	fetchModelsCalls    int

	ensureProviderConfigCalls []string
	providerConfigs           map[string]config.ProviderConfig
	persistProjectConfigCalls int
	// persistProjectConfigErr, when non-nil, makes PersistProjectConfig fail
	// so tests can pin failure-path handling.
	persistProjectConfigErr error

	setModeCalls []string

	// Goal tracking
	setGoalCalls []goalCall
	goalActions  []string

	// Forked agents + recent files
	forkedAgentCalls []forkedAgentCall
	recentFilePaths  []string

	// Workflow engine state
	workflowSpecs    []WorkflowSpecInfo
	runWorkflowCalls []runWorkflowCall
	runWorkflowErr   error
	workflowRuns     []WorkflowRunInfo

	consolidateMemoryCalls int

	refreshModelsCalls int
	testProviderCalls  []string
	vertexAuthOK       bool
	vertexAuthDetail   string
	authSources        map[string]string
	fallbacks          []config.FallbackProvider

	apiErrors          []APIErrorInfo
	clearAPIErrorCalls int

	toggleFileTreeCalls   int
	toggleDiffPaneCalls   int
	toggleReviewModeCalls int
	searchWorkspaceCalls  []string

	exportConversationCalls []string
	exportErr               error
	handleApprovalsCalls    [][]string

	newSessionCalls        int
	showSessionsCalls      int
	showArtifactCalls      int
	resumeSessionCalls     []string
	deleteSessionCalls     []string
	sessionReferences      []SessionReference

	reviewMode      bool
	showingFileTree bool
	diffPaneVisible bool

	// Environment & diagnostics state
	workDir           string
	sessionID         string
	sessionTitle      string
	renamedTitles     []string
	version           string
	sandboxKind       string
	sandboxAvailable  bool
	globalConfigPath  string
	projectConfigPath string
	configProblems    []string
	storageErr        error
	recap             RecapInfo

	// History & workspace state
	checkpoints   []CheckpointInfo
	rewindToCalls []int
	rewindErr     error
	branchCalls   []string
	branchErr     error
	contextFiles  []string
	addDirCalls   []string
	addDirErr     error
	extraDirs     []string
	tokenSessions int
	tokenTotalIn  int
	tokenTotalOut int
	secBlocked    []string
	secAllowed    []string

	rewindPickerCalls      int
	permissionsPickerCalls int
	settingsPickerCalls    int

	showStatsCalls int
	showHelpCalls  int

	openAgentViewCalls []string

	setThemeCalls      []string
	setKeybindingCalls []string

	ctx context.Context
}

// NewMockHost creates a new mock host for testing.
func NewMockHost() *mockHost {
	return &mockHost{
		provider:         "google",
		model:            "gemini-3.6-flash",
		providers:        []string{"google"},
		models:           []ai.Model{{ID: "gemini-3.6-flash", ContextLimit: 1000000}},
		mode:             "edit",
		theme:            "modern",
		keybindings:      "default",
		version:          "test-1.0",
		sandboxKind:      "auto",
		sandboxAvailable: true,
		sessionID:        "sess-test",
		providerConfigs: map[string]config.ProviderConfig{
			"google": {APIKey: "test-key"},
		},
		authSources: map[string]string{"google": "config"},
		ctx:         context.Background(),
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
	m.setGoalCalls = nil
	m.goalActions = nil
	m.forkedAgentCalls = nil
	m.apiErrors = nil
	m.clearAPIErrorCalls = 0
	m.toggleFileTreeCalls = 0
	m.toggleDiffPaneCalls = 0
	m.toggleReviewModeCalls = 0
	m.searchWorkspaceCalls = nil
	m.exportConversationCalls = nil
	m.handleApprovalsCalls = nil
	m.newSessionCalls = 0
	m.showSessionsCalls = 0
	m.resumeSessionCalls = nil
	m.deleteSessionCalls = nil
	m.showStatsCalls = 0
	m.showHelpCalls = 0
	m.setThemeCalls = nil
	m.setKeybindingCalls = nil
	m.refreshModelsCalls = 0
	m.testProviderCalls = nil
	m.fallbacks = nil
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

func (m *mockHost) AddUserCommandMessage(command, prompt string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.commandMessages = append(m.commandMessages, [2]string{command, prompt})
}

func (m *mockHost) DispatchCommand(name string, args ...string) error {
	m.mu.Lock()
	if m.dispatchDepth >= 5 {
		m.mu.Unlock()
		return errors.New("dispatch depth exceeded")
	}
	m.dispatchDepth++
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		m.dispatchDepth--
		m.mu.Unlock()
	}()
	// The mock host dispatches straight through a registry the test sets up;
	// Default() when no override is configured.
	reg := m.registry
	if reg == nil {
		reg = defaultTestRegistry()
	}
	if _, err := reg.Dispatch(m, name, args); err != nil {
		return err
	}
	return nil
}

func (m *mockHost) ReloadCustomCommands() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.reloadCustomCalls++
	return m.reloadCustomCount
}

// defaultTestRegistry memoizes a Default() registry for DispatchCommand.
var defaultTestReg *Registry

func defaultTestRegistry() *Registry {
	if defaultTestReg == nil {
		defaultTestReg = Default()
	}
	return defaultTestReg
}

// --- MCP ---

func (m *mockHost) MCPStatus() []MCPServerStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]MCPServerStatus, len(m.mcpServers))
	copy(out, m.mcpServers)
	return out
}

func (m *mockHost) MCPTools(server string) []MCPToolInfo {
	m.mu.Lock()
	defer m.mu.Unlock()
	if server == "" {
		return append([]MCPToolInfo{}, m.mcpTools...)
	}
	var out []MCPToolInfo
	for _, t := range m.mcpTools {
		if t.Server == server {
			out = append(out, t)
		}
	}
	return out
}

func (m *mockHost) MCPResources() []MCPResourceInfo {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]MCPResourceInfo{}, m.mcpResources...)
}

func (m *mockHost) MCPPrompts() []MCPPromptInfo {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]MCPPromptInfo{}, m.mcpPrompts...)
}

func (m *mockHost) MCPReconnect(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.mcpReconnectCalls = append(m.mcpReconnectCalls, name)
	return nil
}

func (m *mockHost) MCPRefresh(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.mcpRefreshCalls = append(m.mcpRefreshCalls, name)
	return nil
}

func (m *mockHost) MCPEnable(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.mcpEnableCalls = append(m.mcpEnableCalls, name)
	return nil
}

func (m *mockHost) MCPDisable(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.mcpDisableCalls = append(m.mcpDisableCalls, name)
	return nil
}

func (m *mockHost) MCPCallTool(server, name string, args map[string]any) (string, error) {
	return "", errors.New("not implemented in mock")
}

func (m *mockHost) MCPEvents() []MCPEventInfo {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]MCPEventInfo{}, m.mcpEvents...)
}

func (m *mockHost) MCPDeleteToolCache(pattern string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.mcpCacheDeleteCalls++
}

func (m *mockHost) MCPAddServer(name, transport, urlOrCmd string, args []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.mcpAddCalls = append(m.mcpAddCalls, name)
	return nil
}

func (m *mockHost) MCPRemoveServer(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.mcpRemoveCalls = append(m.mcpRemoveCalls, name)
	return nil
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

func (m *mockHost) SetBlocksExpanded(expanded bool) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.blocksExpanded = expanded
	if expanded {
		return "Expanded — /collapse to collapse blocks"
	}
	return "Collapsed — /expand to expand blocks"
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
	return m.persistProjectConfigErr
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

func (m *mockHost) ToggleReviewMode() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.toggleReviewModeCalls++
	m.reviewMode = !m.reviewMode
}

func (m *mockHost) IsReviewMode() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.reviewMode
}

func (m *mockHost) NewSession() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.newSessionCalls++
}

func (m *mockHost) ShowSessions() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.showSessionsCalls++
}

func (m *mockHost) SessionReferences() []SessionReference {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sessionReferences
}

func (m *mockHost) ShowArtifacts() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.showArtifactCalls++
}

func (m *mockHost) ResumeSession(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.resumeSessionCalls = append(m.resumeSessionCalls, id)
	return nil
}

func (m *mockHost) DeleteSession(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.deleteSessionCalls = append(m.deleteSessionCalls, id)
	return nil
}

func (m *mockHost) ShowingFileTree() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.showingFileTree
}

func (m *mockHost) DiffPaneVisible() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.diffPaneVisible
}

func (m *mockHost) WorkDir() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.workDir
}

func (m *mockHost) SessionID() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sessionID
}

func (m *mockHost) SessionTitle() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sessionTitle
}

func (m *mockHost) RenameSession(title string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if title == "" {
		return errors.New("title must not be empty")
	}
	m.renamedTitles = append(m.renamedTitles, title)
	m.sessionTitle = title
	return nil
}

func (m *mockHost) Version() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.version
}

func (m *mockHost) SandboxStatus() (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sandboxKind, m.sandboxAvailable
}

func (m *mockHost) GlobalConfigPath() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.globalConfigPath
}

func (m *mockHost) ProjectConfigPath() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.projectConfigPath
}

func (m *mockHost) ValidateConfig() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string{}, m.configProblems...)
}

func (m *mockHost) StorageHealth() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.storageErr
}

func (m *mockHost) RecapSnapshot() RecapInfo {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.recap
}

func (m *mockHost) SetGoal(objective string, tokenBudget int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.setGoalCalls = append(m.setGoalCalls, goalCall{objective, tokenBudget})
}

func (m *mockHost) GoalSnapshot() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.setGoalCalls) == 0 {
		return "No goal set. Use /goal <objective> to set one."
	}
	last := m.setGoalCalls[len(m.setGoalCalls)-1]
	return "Goal (active): " + last.objective
}

func (m *mockHost) GoalAction(action string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.setGoalCalls) == 0 {
		return "No goal set."
	}
	m.goalActions = append(m.goalActions, action)
	switch action {
	case "clear":
		m.setGoalCalls = nil
		return "Goal cleared."
	case "pause":
		return "Goal paused — continuation loop stopped. Use /goal resume to restart."
	case "resume":
		return "Goal resumed — continuation loop restarts after the next turn."
	case "continue":
		return "Goal continuation counter reset. Continuing..."
	}
	return ""
}

func (m *mockHost) RecentFilePaths() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, len(m.recentFilePaths))
	copy(out, m.recentFilePaths)
	return out
}

func (m *mockHost) StartForkedAgent(command, prompt string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.forkedAgentCalls = append(m.forkedAgentCalls, forkedAgentCall{command, prompt})
}

func (m *mockHost) WorkflowSpecs() []WorkflowSpecInfo {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]WorkflowSpecInfo, len(m.workflowSpecs))
	copy(out, m.workflowSpecs)
	return out
}

func (m *mockHost) RunWorkflow(path string, args []string, resume bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.runWorkflowCalls = append(m.runWorkflowCalls, runWorkflowCall{path, strings.Join(args, " "), resume})
	return m.runWorkflowErr
}

func (m *mockHost) WorkflowRunHistory() []WorkflowRunInfo {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]WorkflowRunInfo, len(m.workflowRuns))
	copy(out, m.workflowRuns)
	return out
}

func (m *mockHost) ConsolidateMemory() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.consolidateMemoryCalls++
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
	return m.exportErr
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

func (m *mockHost) AgentRoster() []AgentRow { return nil }

func (m *mockHost) OpenAgentView(agentID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.openAgentViewCalls = append(m.openAgentViewCalls, agentID)
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

func (m *mockHost) Checkpoints() []CheckpointInfo {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]CheckpointInfo{}, m.checkpoints...)
}

func (m *mockHost) RewindTo(index int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if index < 1 || index > len(m.checkpoints) {
		return errors.New("checkpoint out of range")
	}
	m.rewindToCalls = append(m.rewindToCalls, index)
	return m.rewindErr
}

func (m *mockHost) BranchSession(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if name == "" {
		return errors.New("branch name must not be empty")
	}
	m.branchCalls = append(m.branchCalls, name)
	return m.branchErr
}

func (m *mockHost) ContextFiles() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string{}, m.contextFiles...)
}

func (m *mockHost) AddSearchDir(path string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if path == "" {
		return errors.New("path must not be empty")
	}
	for _, d := range m.extraDirs {
		if d == path {
			return errors.New("already added")
		}
	}
	m.addDirCalls = append(m.addDirCalls, path)
	m.extraDirs = append(m.extraDirs, path)
	return m.addDirErr
}

func (m *mockHost) ExtraSearchDirs() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string{}, m.extraDirs...)
}

func (m *mockHost) SessionTokenTotals() (int, int, int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.tokenSessions, m.tokenTotalIn, m.tokenTotalOut
}

func (m *mockHost) SecurityPaths() ([]string, []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string{}, m.secBlocked...), append([]string{}, m.secAllowed...)
}

func (m *mockHost) APIErrors() []APIErrorInfo {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]APIErrorInfo{}, m.apiErrors...)
}

func (m *mockHost) ClearAPIErrors() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.apiErrors = nil
	m.clearAPIErrorCalls++
}

func (m *mockHost) OpenRewindPicker() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rewindPickerCalls++
}

func (m *mockHost) OpenPermissionsPicker() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.permissionsPickerCalls++
}

func (m *mockHost) OpenSettingsPicker() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.settingsPickerCalls++
}

func (m *mockHost) RefreshModels() tea.Cmd {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.refreshModelsCalls++
	return nil
}

func (m *mockHost) CheckVertexAuth() (bool, string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.vertexAuthOK, m.vertexAuthDetail
}

func (m *mockHost) TestProvider(provider string) tea.Cmd {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.testProviderCalls = append(m.testProviderCalls, provider)
	return nil
}

func (m *mockHost) ProviderAuthSource(provider string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if src, ok := m.authSources[provider]; ok {
		return src
	}
	return ""
}

func (m *mockHost) ProviderFallbacks() []config.FallbackProvider {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]config.FallbackProvider, len(m.fallbacks))
	copy(out, m.fallbacks)
	return out
}

func (m *mockHost) SetProviderFallbacks(fps []config.FallbackProvider) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.fallbacks = make([]config.FallbackProvider, len(fps))
	copy(m.fallbacks, fps)
}
