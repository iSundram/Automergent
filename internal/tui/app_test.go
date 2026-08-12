package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/iSundram/Automergent/internal/agent"
	"github.com/iSundram/Automergent/internal/ai"
	googleProvider "github.com/iSundram/Automergent/internal/ai/google"
	"github.com/iSundram/Automergent/internal/config"
	"github.com/iSundram/Automergent/internal/session"
	"github.com/iSundram/Automergent/internal/tools"
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
	app := NewApp(cfg, ag, sess, nil, "")
	return app
}

func TestSwitchProviderRejectsUnknown(t *testing.T) {
	app := newTestApp(t)
	if err := app.switchProvider("unknown-provider", ""); err == nil {
		t.Fatalf("expected error for unknown provider")
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
	if last.Role != "tool_result" {
		t.Fatalf("expected tool_result role, got %s", last.Role)
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
	if last.Role != "tool_result" || !last.IsError {
		t.Fatalf("expected tool_result error message, got role=%s isError=%v", last.Role, last.IsError)
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

func TestPersistProjectConfigWritesHomeAutomergentConfig(t *testing.T) {
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
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read persisted config: %v", err)
	}
	if len(data) == 0 {
		t.Fatalf("persisted config is empty")
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

func TestPaletteVisibilityDoesNotResizeConversation(t *testing.T) {
	app := newTestApp(t)
	app.width = 120
	app.height = 40
	app.layout()
	baseView := app.conversation.View()
	baseLines := strings.Count(baseView, "\n")

	app.palette.Show()
	app.layout()
	withPaletteView := app.conversation.View()
	withPaletteLines := strings.Count(withPaletteView, "\n")

	if withPaletteLines != baseLines {
		t.Fatalf("expected palette to avoid resizing conversation; base=%d with_palette=%d", baseLines, withPaletteLines)
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
	if strings.Contains(view, "Thinking...") {
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

func TestMouseMsgIsIgnoredForKeyboardOnlyScrolling(t *testing.T) {
	app := newTestApp(t)
	app.width = 120
	app.height = 40
	app.layout()
	before := app.conversation.View()
	_, cmd := app.Update(tea.MouseMsg{})
	after := app.conversation.View()
	if cmd != nil {
		t.Fatalf("expected no command from mouse msg")
	}
	if after != before {
		t.Fatalf("expected mouse msg to be ignored")
	}
}
