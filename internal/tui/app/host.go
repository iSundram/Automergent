package app

import (
	tea "charm.land/bubbletea/v2"
	"context"
	"fmt"
	"github.com/iSundram/Automergent/internal/agent"
	"github.com/iSundram/Automergent/internal/ai"
	"github.com/iSundram/Automergent/internal/config"
	"github.com/iSundram/Automergent/internal/sandbox"
	"github.com/iSundram/Automergent/internal/session"
	toolsagent "github.com/iSundram/Automergent/internal/tools/agent"
	"github.com/iSundram/Automergent/internal/tui/commands"
	"github.com/iSundram/Automergent/internal/tui/components"
	"github.com/iSundram/Automergent/internal/tui/keys"
	"github.com/iSundram/Automergent/internal/tui/render"
	"github.com/iSundram/Automergent/internal/tui/themes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strings"
	"time"
)

// Compile-time guarantee that App satisfies the command host contract.
var _ commands.Host = (*App)(nil)

// Host implementations on *App. The App satisfies commands.Host directly so
// command handlers operate on the live application without an adapter layer.

func (a *App) AddSystemMessage(text string) {
	a.conversation.AddMessage("system", text, false)
}

func (a *App) AddAssistantMessage(text string) {
	a.conversation.AddMessage("assistant", text, true)
}

func (a *App) SetStatus(status string) {
	a.statusBar.SetStatus(status)
}

// commandUsage and commandError report malformed or failed commands.
func (a *App) commandUsage(usage string) {
	a.statusBar.SetStatus("Command needs more information")
	a.conversation.AddMessage("system", "Usage: "+usage, false)
}

func (a *App) commandError(message string) {
	a.statusBar.SetStatus("Command not applied")
	a.conversation.AddMessage("assistant", "Error: "+message, true)
}

// Host interface entry points for command feedback.
func (a *App) CommandUsage(usage string) { a.commandUsage(usage) }

func (a *App) CommandError(message string) {
	if message != "" && !strings.HasPrefix(message, "Error: ") {
		message = "Error: " + message
	}
	a.commandError(message)
}

func (a *App) StartAgent(prompt string) tea.Cmd {
	return a.startAgent(prompt)
}

// AddUserCommandMessage records a prompt-command expansion in the conversation
// with its command provenance (the accent "❯ /command" chip).
func (a *App) AddUserCommandMessage(command, prompt string) {
	a.conversation.AddUserCommandMessage(command, prompt)
}

// DispatchCommand runs another slash command by name from inside a handler,
// sub-command or page action. The depth counter stops runaway cross-command
// recursion; nested tea.Cmds are flushed by the outermost handleSlashCommand
// frame so they execute exactly once.
func (a *App) DispatchCommand(name string, args ...string) error {
	if a.dispatchDepth >= maxDispatchDepth {
		return fmt.Errorf("command dispatch nested more than %d deep — aborting", maxDispatchDepth)
	}
	if _, ok := a.commands.Lookup(name); !ok {
		return commands.ErrUnknownCommand(name)
	}
	a.dispatchDepth++
	input := "/" + name
	if len(args) > 0 {
		input += " " + strings.Join(args, " ")
	}
	cmd := a.handleSlashCommand(input)
	a.dispatchDepth--
	if cmd != nil {
		a.pendingDispatchCmds = append(a.pendingDispatchCmds, cmd)
	}
	return nil
}

// ReloadCustomCommands re-reads markdown custom commands from disk and
// reports how many are registered after the reload.
func (a *App) ReloadCustomCommands() int {
	return a.refreshCustomCommands()
}

func (a *App) CancelActiveRun(status string) {
	a.cancelActiveRun(status)
}

func (a *App) Thinking() bool {
	return a.thinking
}

func (a *App) CompactContext() tea.Cmd {
	return a.compactContext()
}

// SetBlocksExpanded collapses or expands every collapsible conversation
// block (tool cards, thinking blocks, shell output) in one move. The
// returned label names the inverse command so the status line always tells
// the user what to type next.
func (a *App) SetBlocksExpanded(expanded bool) string {
	label := a.conversation.SetExpand(expanded)
	a.conversation.RenderIfDirty()
	return label
}

func (a *App) Provider() string {
	return a.cfg.Provider
}

func (a *App) Model() string {
	return a.cfg.Model
}

func (a *App) Providers() []string {
	return a.availableProviders
}

func (a *App) SwitchProvider(provider, model string) error {
	return a.switchProvider(provider, model)
}

// ProviderAuthSource reports where a provider's key comes from, never what it
// is. config.ProviderAPIKeySource mirrors the resolution order
// config.GetProviderAPIKey uses, so "config" here means the same config the
// runtime would actually read — and returning the source rather than a bool
// answers the question users actually have when a key is set in two places.
func (a *App) ProviderAuthSource(provider string) string {
	if a.cfg == nil {
		return ""
	}
	return config.ProviderAPIKeySource(a.cfg, provider)
}

func (a *App) FetchModels() tea.Cmd {
	return a.fetchModels()
}

func (a *App) ModelsAvailable() []ai.Model {
	return a.modelsAvailable()
}

func (a *App) AvailableModels() []ai.Model {
	return a.modelsAvailable()
}

func (a *App) InputTokens() int {
	return a.sess.TotalInputTokens
}

func (a *App) OutputTokens() int {
	return a.sess.TotalOutputTokens
}

func (a *App) TotalCost() float64 {
	if tel := a.ag.Telemetry(); tel != nil {
		return tel.GetCostSummary().TotalCostUSD
	}
	return 0
}

func (a *App) ShowContextDetail() {
	a.showContextDetail()
}

func (a *App) ActiveTokens() int {
	if a.ag == nil {
		return 0
	}
	mgr := a.ag.ContextManager()
	if mgr == nil {
		return 0
	}
	calc := mgr.AdaptiveCalculator()
	if calc == nil {
		return 0
	}
	if a.sess == nil {
		return 0
	}
	active := calc.EstimateMessages(a.sess.Messages)
	if pending := a.input.Value(); pending != "" {
		active += calc.Estimate(pending)
	}
	return active
}

func (a *App) EnsureProviderConfig(provider string) {
	a.ensureProviderConfig(provider)
}

func (a *App) ProviderConfig(provider string) config.ProviderConfig {
	if a.cfg.Providers == nil {
		return config.ProviderConfig{}
	}
	return a.cfg.Providers[provider]
}

func (a *App) SetProviderConfig(provider string, pc config.ProviderConfig) {
	if a.cfg.Providers == nil {
		a.cfg.Providers = map[string]config.ProviderConfig{}
	}
	a.cfg.Providers[provider] = pc
}

func (a *App) RefreshModels() tea.Cmd {
	return a.refreshModels()
}

func (a *App) TestProvider(provider string) tea.Cmd {
	return a.testProvider(provider)
}

// CheckVertexAuth verifies Vertex AI credentials the way the genai SDK
// resolves them: Application Default Credentials via the gcloud CLI. A
// missing gcloud is reported distinctly from present-but-invalid creds so
// the login guidance names the actual next step.
func (a *App) CheckVertexAuth() (bool, string) {
	if _, err := exec.LookPath("gcloud"); err != nil {
		return false, "gcloud CLI not found — install it from https://cloud.google.com/sdk/docs/install"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "gcloud", "auth", "application-default", "print-access-token").CombinedOutput()
	if err != nil {
		return false, "no valid Application Default Credentials — run: gcloud auth application-default login"
	}
	if strings.TrimSpace(string(out)) == "" {
		return false, "gcloud returned an empty token — re-run: gcloud auth application-default login"
	}
	return true, "Application Default Credentials valid"
}

func (a *App) ProviderFallbacks() []config.FallbackProvider {
	out := make([]config.FallbackProvider, len(a.cfg.ProviderFallback))
	copy(out, a.cfg.ProviderFallback)
	return out
}

func (a *App) SetProviderFallbacks(fps []config.FallbackProvider) {
	a.cfg.ProviderFallback = make([]config.FallbackProvider, len(fps))
	copy(a.cfg.ProviderFallback, fps)
}

func (a *App) PersistProjectConfig() error {
	return a.persistProjectConfig()
}

func (a *App) Mode() string {
	return a.cfg.Mode
}

func (a *App) SetMode(mode string) {
	mode = agent.CanonicalMode(mode)
	a.cfg.Mode = mode
	a.header.SetMode(mode)
	a.statusBar.SetMode(mode)
	a.refreshChrome()
}

func (a *App) ToggleFileTree() {
	a.showFileTree = !a.showFileTree
	a.layout()
}

// ToggleDiffPane backs /diff. It mirrors the ctrl+w keybinding semantics:
// an empty pane is never opened — with no edits there is nothing to review —
// and opening reports how many modified files are queued.
func (a *App) ToggleDiffPane() {
	if !a.diffPane.Visible() && a.diffPane.TabCount() == 0 {
		a.statusBar.SetStatus("No modified files yet")
		return
	}
	a.diffPane.Toggle()
	a.layout()
	if a.diffPane.Visible() {
		a.statusBar.SetStatus(fmt.Sprintf("%d modified file(s)", a.diffPane.TabCount()))
	} else {
		a.statusBar.SetStatus("Diff pane closed")
	}
}

func (a *App) ToggleReviewMode() {
	a.conversation.SetReviewMode(!a.conversation.ReviewMode())
}

func (a *App) IsReviewMode() bool {
	return a.conversation.ReviewMode()
}

func (a *App) SearchWorkspace(query string) string {
	return a.searchWorkspace(query)
}

func (a *App) NewSession() {
	a.newSession()
}

func (a *App) ShowSessions() {
	a.showSessions()
}

// SessionReferences lists stored sessions (most recently updated first) as
// completion entries for /resume: "<title> — <age>" when titled, otherwise
// the first user line serves as the label. The active session is skipped —
// resuming it is a no-op that would only confuse the picker.
func (a *App) SessionReferences() []commands.SessionReference {
	if a.storage == nil {
		return nil
	}
	sessions, err := a.storage.List()
	if err != nil {
		return nil
	}
	sessions = a.projectSessions(sessions)
	refs := make([]commands.SessionReference, 0, len(sessions))
	for _, s := range sessions {
		if s == nil || s.ID == a.sess.ID {
			continue
		}
		label := s.Title
		if label == "" {
			// Untitled sessions still show their opening ask; trimmed to a
			// single short line so one long message doesn't blow out the
			// palette column.
			label = firstUserLineOf(s)
		}
		if label == "" {
			label = s.ID
		}
		if !s.UpdatedAt.IsZero() {
			label += " — " + formatRelativeAge(s.UpdatedAt)
		}
		refs = append(refs, commands.SessionReference{ID: s.ID, Label: label})
	}
	return refs
}

// firstUserLineOf returns the first line of a session's first user message,
// capped to a completion-friendly length. Empty when the session has no user
// turns (its ID will serve as the label instead).
func firstUserLineOf(s *session.Session) string {
	if s == nil {
		return ""
	}
	for _, m := range s.Messages {
		if m.Role != ai.RoleUser {
			continue
		}
		line := strings.TrimSpace(m.PlaintextForHistory())
		if idx := strings.IndexAny(line, "\n"); idx >= 0 {
			line = line[:idx]
		}
		if len(line) > 40 {
			line = line[:37] + "..."
		}
		return line
	}
	return ""
}

// formatRelativeAge renders a session's age for completion labels. Shorter
// than the browser's formatRelativeTime since palette rows are tighter.
func formatRelativeAge(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Hour:
		return "today"
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	default:
		return t.Format("Jan 2")
	}
}

func (a *App) ShowArtifacts() {
	a.showArtifacts()
}

func (a *App) ResumeSession(id string) error {
	return a.resumeSession(id)
}

func (a *App) DeleteSession(id string) error {
	return a.deleteSession(id)
}

func (a *App) ExportConversation(path string) error {
	return a.exportConversation(path)
}

func (a *App) HandleApprovalsCommand(args []string) {
	a.handleApprovalsCommand(args)
}

func (a *App) ShowingFileTree() bool {
	return a.showFileTree
}

func (a *App) DiffPaneVisible() bool {
	return a.diffPane.Visible()
}

func (a *App) ShowStats() {
	a.stats.InputTokens = a.sess.TotalInputTokens
	a.stats.OutputTokens = a.sess.TotalOutputTokens
	if tel := a.ag.Telemetry(); tel != nil {
		a.stats.TotalCost = tel.GetCostSummary().TotalCostUSD
	}
	a.conversation.AddMessage("system", a.stats.View(), false)
}

func (a *App) ShowHelp() {
	a.showHelp = true
}

func (a *App) SetTheme(name string) error {
	theme := themes.Get(name)
	if theme.Name != name && name != "catppuccin" {
		return fmt.Errorf("unknown theme %q", name)
	}
	*a.styles = *themes.NewStyles(theme)
	a.theme = theme
	a.cfg.Theme = name
	render.SetTheme(theme)
	a.conversation.Invalidate()
	return nil
}

func (a *App) AvailableThemes() []string {
	engine := themes.NewThemeEngine()
	names := engine.AvailableThemes()
	sort.Strings(names)
	return names
}

func (a *App) CurrentTheme() string {
	return a.cfg.Theme
}

func (a *App) SetKeybindings(scheme string) error {
	a.keys = keys.Get(scheme)
	a.cfg.Keybindings = scheme
	return nil
}

func (a *App) AvailableKeybindings() []string {
	return []string{"default", "vim", "emacs"}
}

func (a *App) CurrentKeybindings() string {
	return a.cfg.Keybindings
}

func (a *App) Context() context.Context {
	return a.ctx
}

// refreshCustomCommands reloads markdown custom commands from disk, replacing
// previously loaded customs. It reports how many are registered after the
// reload and surfaces new warnings exactly once.
func (a *App) refreshCustomCommands() int {
	a.commands.RemoveCustom()
	loaded, warnings := commands.LoadProjectAndUserCommands(a.commands, a.workDir)
	if key := strings.Join(warnings, "\n"); key != a.customWarnKey {
		a.customWarnKey = key
		if len(warnings) > 0 {
			a.conversation.AddMessage("system", "Custom command problems:\n"+strings.Join(warnings, "\n"), false)
		}
	}
	if loaded != a.customCmdCount {
		a.customCmdCount = loaded
		a.helpOverlay.SetSlashCommands(a.commands.HelpRows())
		a.helpOverlay.SetSlashSections(a.commands.HelpSections())
		a.syncCommandHints()
	}
	return loaded
}

func (a *App) WorkDir() string {
	return a.workDir
}

func (a *App) SessionID() string {
	if a.sess == nil {
		return ""
	}
	return a.sess.ID
}

func (a *App) SessionTitle() string {
	if a.sess == nil {
		return ""
	}
	return a.sess.Title
}

func (a *App) RenameSession(title string) error {
	title = strings.TrimSpace(title)
	if title == "" {
		return fmt.Errorf("title must not be empty")
	}
	if a.sess == nil {
		return fmt.Errorf("no active session")
	}
	a.sess.Title = title
	if a.storage != nil {
		return a.storage.Save(a.sess)
	}
	return nil
}

func (a *App) Version() string {
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return "dev"
}

func (a *App) SandboxStatus() (string, bool) {
	kind := a.cfg.Security.Sandbox
	if kind == "" {
		kind = "auto"
	}
	return kind, sandbox.New(kind).IsAvailable()
}

func (a *App) GlobalConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".automergent", "config.yaml")
}

func (a *App) ProjectConfigPath() string {
	return filepath.Join(a.workDir, ".automergent", "config.yaml")
}

func (a *App) ValidateConfig() []string {
	var problems []string
	for _, err := range config.DefaultSchema().Validate(a.cfg) {
		problems = append(problems, err.Error())
	}
	return problems
}

func (a *App) StorageHealth() error {
	if a.storage == nil {
		return fmt.Errorf("session storage unavailable")
	}
	if _, err := a.storage.List(); err != nil {
		return err
	}
	return nil
}

// SetGoal installs a new autonomy objective (backs /goal <objective>).
func (a *App) SetGoal(objective string, tokenBudget int) {
	a.setGoal(objective, tokenBudget)
}

// GoalSnapshot renders the current goal state for /goal status output.
func (a *App) GoalSnapshot() string {
	return a.goalSnapshot()
}

// GoalAction applies a goal lifecycle action and returns the message to
// show the user. Unknown actions return "" (the command reports usage).
func (a *App) GoalAction(action string) string {
	g := a.goal
	switch action {
	case "pause":
		if g == nil {
			return "No goal set."
		}
		g.paused = true
		return "Goal paused — continuation loop stopped. Use /goal resume to restart."
	case "resume":
		if g == nil {
			return "No goal set."
		}
		g.paused = false
		return "Goal resumed — continuation loop restarts after the next turn."
	case "continue":
		if g == nil {
			return "No goal set."
		}
		g.paused = false
		g.turns = 0
		g.blocked = 0
		return "Goal continuation counter reset. Continuing..."
	case "clear":
		if g == nil {
			return "No goal set."
		}
		a.goal = nil
		return "Goal cleared."
	}
	return ""
}

// RecentFilePaths surfaces the files tracked by the diff pane — the
// workspace's recently touched set — for path-gated command visibility.
func (a *App) RecentFilePaths() []string {
	return a.diffPane.FilePaths()
}

// StartForkedAgent runs a Fork prompt-command in a background subagent.
// The result is delivered asynchronously and rendered as a system message
// with the command's provenance.
func (a *App) StartForkedAgent(command, prompt string) {
	go func() {
		out, err := a.ag.Execute(a.ctx, toolsagent.AgentTypeGeneralPurpose, prompt, "")
		if a.sendToProgram != nil {
			a.sendToProgram(forkedAgentDoneMsg{command: command, output: out, err: err})
		}
	}()
}

// forkedAgentDoneMsg carries a forked command's result back to the main loop.
type forkedAgentDoneMsg struct {
	command string
	output  string
	err     error
}

// handleForkedAgentDone renders a forked command's outcome in the main view.
func (a *App) handleForkedAgentDone(m forkedAgentDoneMsg) {
	if m.err != nil {
		a.conversation.AddMessage("system", fmt.Sprintf("❯ /%s fork failed: %v", m.command, m.err), false)
		a.statusBar.SetStatus("Forked command failed")
		return
	}
	a.conversation.AddMessage("system", "❯ /"+m.command+" (forked agent):\n"+m.output, false)
	a.statusBar.SetStatus("Forked command done")
}

// RecapSnapshot builds the deterministic recap digest from session internals.
func (a *App) RecapSnapshot() commands.RecapInfo {
	info := commands.RecapInfo{}
	if a.sess == nil {
		return info
	}
	info.StartedAt = a.sess.CreatedAt
	info.UpdatedAt = a.sess.UpdatedAt
	seenTools := map[string]bool{}
	for i, message := range a.sess.Messages {
		switch message.Role {
		case ai.RoleUser:
			info.UserTurns++
			if text := message.TextContent(); text != "" {
				info.LastUserMessage = text
			}
		case ai.RoleAssistant:
			info.AssistantTurns++
		}
		_ = i
		for _, part := range message.Content {
			if part.Type == ai.ContentTypeToolCall && part.ToolCall != nil && !seenTools[part.ToolCall.Name] {
				seenTools[part.ToolCall.Name] = true
				info.ToolsUsed = append(info.ToolsUsed, part.ToolCall.Name)
			}
			if part.Type == ai.ContentTypeToolCall {
				info.ToolCalls++
			}
		}
	}
	return info
}

func (a *App) Checkpoints() []commands.CheckpointInfo {
	return a.checkpointSummaries()
}

func (a *App) RewindTo(index int) error {
	return a.rewindTo(index)
}

func (a *App) BranchSession(name string) error {
	return a.branchSession(name)
}

func (a *App) ContextFiles() []string {
	if a.sess == nil {
		return nil
	}
	return a.touchedFiles()
}

func (a *App) AddSearchDir(path string) error {
	return a.addSearchDir(path)
}

func (a *App) ExtraSearchDirs() []string {
	return append([]string{}, a.extraSearchDirs...)
}

// SessionTokenTotals aggregates stored session token usage. Sessions=0 with
// nil error means storage is unavailable.
func (a *App) SessionTokenTotals() (int, int, int) {
	if a.storage == nil {
		return 0, 0, 0
	}
	sessions, err := a.storage.List()
	if err != nil {
		return 0, 0, 0
	}
	var totalIn, totalOut int
	for _, s := range sessions {
		totalIn += s.TotalInputTokens
		totalOut += s.TotalOutputTokens
	}
	return len(sessions), totalIn, totalOut
}

func (a *App) SecurityPaths() ([]string, []string) {
	return append([]string{}, a.cfg.Security.BlockedWritePaths...),
		append([]string{}, a.cfg.Security.AllowedWritePaths...)
}

// --- Interactive picker overlays ---

// openSelector shows the selector overlay with a confirmation action.
func (a *App) openSelector(title string, items []components.SelectorItem, hint string, action func(int)) {
	a.selector.SetTitle(title)
	a.selector.SetHint(hint)
	a.selector.SetItems(items)
	a.selector.Show()
	a.selectorAction = action
	a.layout()
}

func (a *App) OpenRewindPicker() {
	checkpoints := a.Checkpoints()
	if len(checkpoints) == 0 {
		a.AddSystemMessage("No checkpoints yet — they are captured automatically before each agent turn.")
		return
	}
	items := make([]components.SelectorItem, len(checkpoints))
	for i, cp := range checkpoints {
		items[i] = components.SelectorItem{
			Label:  fmt.Sprintf("%d. %s", cp.Index, cp.Label),
			Detail: fmt.Sprintf("%s · %d msgs", cp.At.Format("15:04:05"), cp.Messages),
		}
	}
	a.openSelector("Rewind checkpoints", items, "enter restore · esc close", func(i int) {
		if err := a.rewindTo(i + 1); err != nil {
			a.commandError(err.Error())
		}
	})
}

func (a *App) OpenPermissionsPicker() {
	var items []components.SelectorItem
	approvals := a.ag.Approvals()
	for i, ap := range approvals {
		items = append(items, components.SelectorItem{
			Label:  fmt.Sprintf("Revoke approval %d", i+1),
			Detail: formatApprovalScope(ap.Scope),
		})
	}
	blocked, allowed := a.SecurityPaths()
	for _, p := range blocked {
		items = append(items, components.SelectorItem{
			Label:          "✗ blocked " + p,
			Detail:         "write-path rule",
			Disabled:       true,
			DisabledReason: "edit config.yaml to change",
		})
	}
	for _, p := range allowed {
		items = append(items, components.SelectorItem{
			Label:          "✓ allowed " + p,
			Detail:         "write-path rule",
			Disabled:       true,
			DisabledReason: "edit config.yaml to change",
		})
	}
	if len(items) == 0 {
		a.AddSystemMessage("No always-allow approvals or write-path rules configured.")
		return
	}
	a.openSelector("Permissions & rules", items, "enter revoke · esc close", func(i int) {
		if i >= len(approvals) {
			return // disabled rule rows cannot be confirmed
		}
		scope := approvals[i].Scope
		if a.ag.RevokeApproval(scope) {
			if a.storage != nil {
				_ = a.storage.Save(a.sess)
			}
			a.AddSystemMessage("Revoked approval: " + formatApprovalScope(scope))
			a.SetStatus("Approval revoked")
		}
	})
}

// AgentRoster snapshots the live subagents for the /agents page.
func (a *App) AgentRoster() []commands.AgentRow {
	instances := toolsagent.GetAgentManager().List(true)
	out := make([]commands.AgentRow, 0, len(instances))
	for _, inst := range instances {
		snap := inst.Snapshot()
		activity := snap.CurrentTool
		status := render.CanonicalStatus(snap.Status)
		if activity == "" {
			activity = render.FirstLine(snap.LastLine)
		}
		if status.Terminal() {
			activity = status.Label()
		}
		out = append(out, commands.AgentRow{
			ID:        snap.ID,
			Name:      firstNonEmptyDock(snap.Name, snap.ID),
			Type:      snap.Type,
			Status:    snap.Status,
			Activity:  activity,
			Elapsed:   snap.Elapsed,
			ToolCount: snap.ToolCount,
			Turns:     snap.Turns,
			Terminal:  status.Terminal(),
		})
	}
	return out
}

// OpenAgentView opens the live inspector on one subagent (the /agents page's
// numbered actions land here).
func (a *App) OpenAgentView(agentID string) {
	if _, ok := toolsagent.GetAgentManager().Get(agentID); !ok {
		a.AddSystemMessage("No such agent: " + agentID)
		return
	}
	a.openInspector(components.DockEntry{Kind: components.DockAgent, ID: agentID})
}

func (a *App) OpenSettingsPicker() {
	pc := a.ProviderConfig(a.Provider())
	effort := pc.Effort
	if effort == "" {
		effort = pc.ThinkingLevel
	}
	if effort == "" {
		effort = "default"
	}
	items := []components.SelectorItem{
		{Label: "Theme", Detail: "currently " + a.CurrentTheme()},
		{Label: "Keybindings", Detail: "currently " + a.CurrentKeybindings()},
		{Label: "Mode", Detail: "edit/plan — currently " + a.Mode()},
		{Label: "Effort", Detail: "minimal..high — currently " + effort},
	}
	a.openSelector("Settings", items, "enter adjust · esc close", func(i int) {
		switch i {
		case 0:
			a.launchSubPalette("theme")
		case 1:
			a.launchSubPalette("keybindings")
		case 2:
			a.launchSubPalette("mode")
		case 3:
			a.launchSubPalette("effort")
		}
	})
}

// launchSubPalette seeds the input with "/<name>" and opens its palette.
func (a *App) launchSubPalette(name string) {
	a.input.SetValue("/" + name)
	a.updatePalette()
	a.palette.Show(a.palette.Items(), a.input.TriggerValue())
	a.layout()
}
