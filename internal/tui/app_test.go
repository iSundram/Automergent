package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/iSundram/Automergent/internal/agent"
	"github.com/iSundram/Automergent/internal/ai"
	googleProvider "github.com/iSundram/Automergent/internal/ai/google"
	"github.com/iSundram/Automergent/internal/config"
	"github.com/iSundram/Automergent/internal/session"
	"github.com/iSundram/Automergent/internal/tools"
	"github.com/iSundram/Automergent/internal/tui/components"
)

func newTestApp(t *testing.T) *App {
	t.Helper()
	cfg := config.Default()
	cfg.Provider = "google"
	cfg.Model = "gemini-3.6-flash"
	cfg.Providers["google"] = config.ProviderConfig{APIKey: "test-key"}
	sess := session.New()
	reg := tools.NewRegistry()
	ag := agent.New(cfg, googleProvider.New(ai.ProviderConfig{
		APIKey:       cfg.Providers["google"].APIKey,
		DefaultModel: cfg.Model,
	}), sess, reg)
	app := NewApp(cfg, ag, sess, nil, nil, "", false)
	return app
}

func TestSwitchProviderRejectsUnknown(t *testing.T) {
	app := newTestApp(t)
	if err := app.switchProvider("unknown-provider", ""); err == nil {
		t.Fatalf("expected error for unknown provider")
	}
}

func TestNewAppReplaysLoadedSession(t *testing.T) {
	app := newTestApp(t)
	app.sess.AddMessage(ai.NewTextMessage(ai.RoleUser, "loaded before TUI"))
	app = NewApp(app.cfg, app.ag, app.sess, nil, nil, "", false)
	if !strings.Contains(ansi.Strip(app.conversation.View()), "loaded before TUI") {
		t.Fatal("directly loaded session should be visible when TUI starts")
	}
}

func TestSlashProviderSwitchesProviderAndDefaultModel(t *testing.T) {
	app := newTestApp(t)
	app.handleSlashCommand("/provider google")
	if app.cfg.Provider != "google" {
		t.Fatalf("expected provider google, got %s", app.cfg.Provider)
	}
	if app.cfg.Model != "gemini-3.6-flash" {
		t.Fatalf("expected default gemini model, got %s", app.cfg.Model)
	}
}

func TestSlashModelSwitchesRuntimeModel(t *testing.T) {
	app := newTestApp(t)
	app.handleSlashCommand("/model gemini-3.6-flash")
	if app.cfg.Model != "gemini-3.6-flash" {
		t.Fatalf("expected model switch, got %s", app.cfg.Model)
	}
}

func TestSlashAPIAndBaseURLCommandsSetProviderConfig(t *testing.T) {
	app := newTestApp(t)
	app.handleSlashCommand("/api-key abc123")
	app.handleSlashCommand("/base-url https://example.local/v1")
	pc := app.cfg.Providers[app.cfg.Provider]
	if pc.APIKey != "abc123" {
		t.Fatalf("expected api key update, got %q", pc.APIKey)
	}
	if pc.BaseURL != "https://example.local/v1" {
		t.Fatalf("expected base url update, got %q", pc.BaseURL)
	}
}

func TestSlashProviderScopedConfigCommands(t *testing.T) {
	app := newTestApp(t)
	app.handleSlashCommand("/provider-api-key google xyz")
	app.handleSlashCommand("/provider-base-url google https://generativelanguage.googleapis.com/v1beta")
	pc := app.cfg.Providers["google"]
	if pc.APIKey != "xyz" {
		t.Fatalf("expected provider api key update, got %q", pc.APIKey)
	}
	if pc.BaseURL != "https://generativelanguage.googleapis.com/v1beta" {
		t.Fatalf("expected provider base url update, got %q", pc.BaseURL)
	}
}

func TestHandleAgentEventDoneSkipsDuplicateAfterStreaming(t *testing.T) {
	app := newTestApp(t)
	app.handleAgentEvent(agent.Event{Type: agent.EventToken, Payload: "hello"})
	before := app.conversation.MessageCount()
	app.handleAgentEvent(agent.Event{Type: agent.EventDone, Payload: "hello"})
	after := app.conversation.MessageCount()
	if after != before {
		t.Fatalf("expected no duplicate message after streamed done, before=%d after=%d", before, after)
	}
}

func TestHandleAgentEventDoneAddsMessageWhenNoStreaming(t *testing.T) {
	app := newTestApp(t)
	app.handleAgentEvent(agent.Event{Type: agent.EventDone, Payload: "standalone"})
	last, ok := app.conversation.LastMessage()
	if !ok {
		t.Fatalf("expected a message")
	}
	if last.Role != "assistant" || last.Content == "" {
		t.Fatalf("expected assistant completion message, got role=%s content=%q", last.Role, last.Content)
	}
}

func TestConfirmationReplacesInputFooter(t *testing.T) {
	app := newTestApp(t)
	app.width = 100
	app.height = 30
	app.confirm.ShowSimple("Allow writes in this project?")
	app.layout()
	content := ansi.Strip(app.View().Content)
	if !strings.Contains(content, "Allow writes in this project?") {
		t.Fatal("confirmation footer is not visible")
	}
	if strings.Contains(content, "Message Automergent") {
		t.Fatal("input must be hidden while confirmation is visible")
	}
}

func TestProjectApprovalUpdatesAllowedPaths(t *testing.T) {
	app := newTestApp(t)
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	app.cfg.ConfigFile = configPath
	projectPath := filepath.Join(t.TempDir(), "project")
	app.pendingProjectPath = projectPath
	_, _ = app.Update(projectApprovalMsg{response: agent.ConfirmationResponse{Allow: true}})
	if len(app.cfg.Security.AllowedWritePaths) == 0 || app.cfg.Security.AllowedWritePaths[len(app.cfg.Security.AllowedWritePaths)-1] != projectPath {
		t.Fatalf("project path was not allowed: %v", app.cfg.Security.AllowedWritePaths)
	}
	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		t.Fatalf("session-only trust must not persist config, stat error: %v", err)
	}
}

func TestRememberedProjectApprovalPersistsConfig(t *testing.T) {
	app := newTestApp(t)
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(configPath, []byte("mode: edit\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	app.cfg.ConfigFile = configPath
	app.pendingProjectPath = filepath.Join(t.TempDir(), "project")
	_, _ = app.Update(projectApprovalMsg{response: agent.ConfirmationResponse{Allow: true, Always: true}})
	if _, err := os.Stat(configPath); err != nil {
		t.Fatalf("remembered trust did not persist config: %v", err)
	}
}

func TestShellPermissionIncludesCommandAndRisk(t *testing.T) {
	info := permissionInfoForTool(ai.ToolCall{
		Name: "run_shell_command",
		Args: map[string]any{"command": "go test ./...", "working_directory": "/workspace"},
	}, "Run")
	if info.Action != "Execute shell command" || info.Risk == "" {
		t.Fatalf("unexpected shell permission metadata: %+v", info)
	}
	if len(info.Fields) < 2 {
		t.Fatalf("shell permission is missing command context: %+v", info.Fields)
	}
}

func TestHandleAgentEventToolDoneRendersResult(t *testing.T) {
	app := newTestApp(t)
	app.handleAgentEvent(agent.Event{
		Type: agent.EventToolDone,
		Payload: agent.ToolDoneEvent{
			ID:       "t1",
			Name:     "read_file",
			Duration: 10,
			Result:   tools.Result{Content: "tool output", IsError: false},
		},
	})
	last, ok := app.conversation.LastMessage()
	if !ok {
		t.Fatalf("expected a tool result message")
	}
	if last.Role != "tool_call" || last.Status != "done" {
		t.Fatalf("expected completed tool_call, got role=%s status=%s", last.Role, last.Status)
	}
}

func TestHandleAgentEventToolDoneRendersError(t *testing.T) {
	app := newTestApp(t)
	app.handleAgentEvent(agent.Event{
		Type: agent.EventToolDone,
		Payload: agent.ToolDoneEvent{
			ID:       "t2",
			Name:     "run_command",
			Duration: 20,
			Result:   tools.Result{Content: "boom", IsError: true},
		},
	})
	last, ok := app.conversation.LastMessage()
	if !ok {
		t.Fatalf("expected an error message")
	}
	if last.Role != "tool_call" || last.Status != "error" || !last.IsError {
		t.Fatalf("expected errored tool_call message, got role=%s status=%s isError=%v", last.Role, last.Status, last.IsError)
	}
}

func TestToolResultTruncatesWhenNotInReviewMode(t *testing.T) {
	app := newTestApp(t)
	long := ""
	for i := 0; i < 700; i++ {
		long += "a"
	}
	app.handleAgentEvent(agent.Event{Type: agent.EventToolDone, Payload: tools.Result{Content: long, IsError: false}})
	last, ok := app.conversation.LastMessage()
	if !ok || last.Role != "tool_result" {
		t.Fatalf("expected tool_result")
	}
	if len(last.Content) >= len(long) {
		t.Fatalf("expected truncated content")
	}
}

func TestToolResultNotTruncatedInReviewMode(t *testing.T) {
	app := newTestApp(t)
	app.conversation.SetReviewMode(true)
	long := ""
	for i := 0; i < 700; i++ {
		long += "b"
	}
	app.handleAgentEvent(agent.Event{Type: agent.EventToolDone, Payload: tools.Result{Content: long, IsError: false}})
	last, ok := app.conversation.LastMessage()
	if !ok || last.Role != "tool_result" {
		t.Fatalf("expected tool_result")
	}
	if last.Content != long {
		t.Fatalf("expected full content in review mode")
	}
}

func TestCtrlRTogglesReviewMode(t *testing.T) {
	app := newTestApp(t)
	if app.conversation.ReviewMode() {
		t.Fatalf("expected review mode off initially")
	}
	app.handleKey(tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
	if !app.conversation.ReviewMode() {
		t.Fatalf("expected review mode on after ctrl+r")
	}
	app.handleKey(tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
	if app.conversation.ReviewMode() {
		t.Fatalf("expected review mode off after second ctrl+r")
	}
}

func TestPersistProjectConfigDoesNotCreateHomeConfig(t *testing.T) {
	app := newTestApp(t)
	home := t.TempDir()

	origWD, _ := os.Getwd()
	origHome := os.Getenv("HOME")
	defer func() { _ = os.Chdir(origWD) }()
	defer func() { _ = os.Setenv("HOME", origHome) }()
	if err := os.Chdir(home); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	if err := os.Setenv("HOME", home); err != nil {
		t.Fatalf("set HOME: %v", err)
	}

	app.cfg.Provider = "google"
	app.cfg.Model = "gemini-3.6-flash"
	app.ensureProviderConfig("google")
	pc := app.cfg.Providers["google"]
	pc.APIKey = "sk-test"
	app.cfg.Providers["google"] = pc

	if err := app.persistProjectConfig(); err != nil {
		t.Fatalf("persistProjectConfig: %v", err)
	}

	path := filepath.Join(home, ".automergent", "config.yaml")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("missing config was created, stat error: %v", err)
	}
}

func TestLayoutThinkingReservesHeightWithoutPaletteShift(t *testing.T) {
	app := newTestApp(t)
	app.width = 120
	app.height = 40
	app.input.SetValue("x")
	app.thinking = false
	app.layout()
	withoutThinking := app.conversation.View()
	withoutThinkingLines := strings.Count(withoutThinking, "\n")

	app.thinking = true
	app.layout()
	withThinking := app.conversation.View()
	withThinkingLines := strings.Count(withThinking, "\n")

	if withThinkingLines >= withoutThinkingLines {
		t.Fatalf("expected thinking layout to reserve space; got without=%d with=%d", withoutThinkingLines, withThinkingLines)
	}
}

func TestPaletteVisibilityResizesConversation(t *testing.T) {
	app := newTestApp(t)
	app.width = 120
	app.height = 40
	app.layout()
	baseView := app.conversation.View()
	baseLines := strings.Count(baseView, "\n")

	app.palette.Show([]components.PaletteItem{
		{Label: "/help", Description: "Show help", Value: "help"},
		{Label: "/clear", Description: "Clear conversation", Value: "clear"},
	}, "")
	app.layout()
	withPaletteView := app.conversation.View()
	withPaletteLines := strings.Count(withPaletteView, "\n")

	if withPaletteLines >= baseLines {
		t.Fatalf("expected inline palette to shrink conversation; base=%d with_palette=%d", baseLines, withPaletteLines)
	}
}

func TestThinkingLineNotDuplicatedInView(t *testing.T) {
	app := newTestApp(t)
	app.width = 120
	app.height = 40
	app.thinking = true
	app.spin.Start()
	app.layout()
	view := app.View()
	if strings.Contains(view.Content, "Thinking...") {
		t.Fatalf("expected no duplicate hardcoded Thinking label in view")
	}
}

func TestIgnoresStaleTransientStatusAfterDone(t *testing.T) {
	app := newTestApp(t)
	app.thinking = true
	app.spin.Start()
	app.handleAgentEvent(agent.Event{Type: agent.EventDone, Payload: ""})
	if app.thinking {
		t.Fatalf("expected thinking false after done")
	}
	before := app.statusBar.View()
	app.handleAgentEvent(agent.Event{Type: agent.EventStatus, Payload: "thinking"})
	after := app.statusBar.View()
	if after != before {
		t.Fatalf("expected stale transient status to be ignored after done")
	}
}

func TestEscInterruptsActiveRun(t *testing.T) {
	app := newTestApp(t)
	app.thinking = true
	app.spin.Start()
	app.handleKey(tea.KeyPressMsg{Code: tea.KeyEsc})
	if app.thinking {
		t.Fatalf("expected esc to interrupt active run")
	}
}

func TestCtrlCSingleInterruptsAndDoubleExits(t *testing.T) {
	app := newTestApp(t)
	app.thinking = true
	app.spin.Start()
	cmd := app.handleKey(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if cmd != nil {
		t.Fatalf("expected first ctrl+c to interrupt but not quit")
	}
	if app.thinking {
		t.Fatalf("expected interrupted run after first ctrl+c")
	}
	app.lastCtrlCAt = time.Now()
	cmd = app.handleKey(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if cmd == nil {
		t.Fatalf("expected second ctrl+c to quit")
	}
}
