package tui

import (
	"regexp"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/iSundram/Automergent/internal/ai"
	"github.com/iSundram/Automergent/internal/session"
	"github.com/iSundram/Automergent/internal/tui/components"
)

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func ansiStrip(s string) string { return ansiRe.ReplaceAllString(s, "") }

func TestPaletteSessionsFlow(t *testing.T) {
	app := newTestApp(t)
	storage, err := session.NewStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	app.storage = storage
	saved := session.New()
	saved.Title = "My saved chat"
	saved.AddMessage(ai.NewTextMessage(ai.RoleUser, "hello"))
	if err := storage.Save(saved); err != nil {
		t.Fatal(err)
	}

	var model tea.Model = app
	for _, ch := range "/sessions" {
		model, _ = model.Update(tea.KeyPressMsg{Code: ch, Text: string(ch)})
	}
	model, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	a := model.(*App)
	if !a.sessionBrowser.Visible() || a.sessionBrowser.ItemCount() != 1 {
		t.Fatalf("picker should be open with 1 session, got visible=%v items=%d", a.sessionBrowser.Visible(), a.sessionBrowser.ItemCount())
	}
	if a.input.Value() != "" {
		t.Fatalf("input should be reset after palette command, got %q", a.input.Value())
	}

	model, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	a = model.(*App)
	if a.sessionBrowser.Visible() {
		t.Fatal("picker should close on select")
	}
	if cmd == nil {
		t.Fatal("expected SessionSelectedMsg cmd")
	}
	var got components.SessionSelectedMsg
	found := false
	var walk func(c tea.Cmd)
	walk = func(c tea.Cmd) {
		if c == nil || found {
			return
		}
		msg := c()
		if sm, ok := msg.(components.SessionSelectedMsg); ok {
			got, found = sm, true
			return
		}
		if bm, ok := msg.(tea.BatchMsg); ok {
			for _, sub := range bm {
				walk(sub)
			}
		}
	}
	walk(cmd)
	if !found || got.Session == nil || got.Session.Title != "My saved chat" {
		t.Fatalf("expected SessionSelectedMsg for %q, got %#v", "My saved chat", cmd())
	}

	model = app
	for _, ch := range "/sessions" {
		model, _ = model.Update(tea.KeyPressMsg{Code: ch, Text: string(ch)})
	}
	model, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	a = model.(*App)
	model, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	a = model.(*App)
	if a.sessionBrowser.Visible() {
		t.Fatal("picker should close on esc")
	}
}

func TestSessionPickerRowContent(t *testing.T) {
	app := newTestApp(t)
	app.workDir = "/root/OweCode"
	storage, err := session.NewStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	app.storage = storage
	s := session.New()
	s.Title = "Gemini SDK rewrite"
	s.WorkDir = "/root/OweCode"
	s.Provider, s.Model = "google", "gemini-2.5-pro"
	s.TotalInputTokens, s.TotalOutputTokens = 1234567, 8901
	for i := 0; i < 4; i++ {
		s.AddMessage(ai.NewTextMessage(ai.RoleUser, "u"))
		s.AddMessage(ai.NewTextMessage(ai.RoleAssistant, "a"))
	}
	s.CreatedAt = time.Now().Add(-48 * time.Hour)
	s.UpdatedAt = time.Now().Add(-3 * time.Hour)
	if err := storage.Save(s); err != nil {
		t.Fatal(err)
	}

	var model tea.Model = app
	for _, ch := range "/sessions" {
		model, _ = model.Update(tea.KeyPressMsg{Code: ch, Text: string(ch)})
	}
	model, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	a := model.(*App)
	a.sessionBrowser.SetSize(130, 20)
	view := a.sessionBrowser.View()
	view = ansiStrip(view)
	t.Logf("VIEW:\n%s", view)
	for _, want := range []string{"Gemini SDK rewrite", "created", "modified", "8 msgs", "google/gemini-2.5-pro", "1.2M in/8.9k out", "/root/OweCode"} {
		if !strings.Contains(view, want) {
			t.Fatalf("picker view missing %q:\n%s", want, view)
		}
	}
}

func TestSessionPickerScrollAndTheme(t *testing.T) {
	app := newTestApp(t)
	app.workDir = "/root/OweCode"
	storage, err := session.NewStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	app.storage = storage
	for i := 0; i < 12; i++ {
		s := session.New()
		s.Title = "Session " + string(rune('A'+i))
		s.WorkDir = "/root/OweCode"
		s.UpdatedAt = time.Now().Add(-time.Duration(i) * time.Hour)
		s.AddMessage(ai.NewTextMessage(ai.RoleUser, "hi"))
		if err := storage.Save(s); err != nil {
			t.Fatal(err)
		}
	}

	var model tea.Model = app
	for _, ch := range "/sessions" {
		model, _ = model.Update(tea.KeyPressMsg{Code: ch, Text: string(ch)})
	}
	model, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	a := model.(*App)
	a.sessionBrowser.SetSize(90, 24)
	view := ansiStrip(a.sessionBrowser.View())
	if !strings.Contains(view, "┃") || !strings.Contains(view, "│") {
		t.Fatalf("scrollbar missing:\n%s", view)
	}

	for i := 0; i < 5; i++ {
		model, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	}
	a = model.(*App)
	view = ansiStrip(a.sessionBrowser.View())
	if !strings.Contains(view, "Session G") {
		t.Fatalf("cursor should follow navigation, view:\n%s", view)
	}
	if !strings.Contains(view, "6 of 12") {
		t.Fatalf("count should read 6 of 12, view:\n%s", view)
	}

	// Selected row must use the accent indicator (▍) rather than a boxed border.
	if !strings.Contains(a.sessionBrowser.View(), "▍") {
		t.Fatalf("selected row must use accent indicator")
	}
}

func TestResumeRestoresFullConversation(t *testing.T) {
	app := newTestApp(t)
	app.workDir = "/root/OweCode"
	storage, err := session.NewStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	app.storage = storage

	s := session.New()
	s.Title = "Rich session"
	s.WorkDir = "/root/OweCode"
	s.AddMessage(ai.NewTextMessage(ai.RoleUser, "hello agent"))
	s.AddMessage(ai.NewTextMessage(ai.RoleSystem, "hidden model-only instruction"))
	assistant := ai.Message{Role: ai.RoleAssistant, Content: []ai.ContentPart{
		{Type: ai.ContentTypeThought, Thought: "let me think about this"},
		{Type: ai.ContentTypeText, Text: "I will check the file"},
		{Type: ai.ContentTypeToolCall, ToolCall: &ai.ToolCall{
			ID: "call-1", Name: "read_file",
			Args: map[string]any{"path": "main.go"},
		}},
	}}
	s.AddMessage(assistant)
	s.AddMessage(ai.Message{Role: ai.RoleTool, Content: []ai.ContentPart{
		{Type: ai.ContentTypeToolResult, ToolResult: &ai.ToolResult{ToolCallID: "call-1", Content: "package main", IsError: false}},
	}})
	if err := storage.Save(s); err != nil {
		t.Fatal(err)
	}

	var model tea.Model = app
	for _, ch := range "/sessions" {
		model, _ = model.Update(tea.KeyPressMsg{Code: ch, Text: string(ch)})
	}
	model, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	a := model.(*App)
	model, cmd = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	a = model.(*App)

	var got *session.Session
	var walk func(c tea.Cmd)
	walk = func(c tea.Cmd) {
		if c == nil || got != nil {
			return
		}
		if sm, ok := c().(components.SessionSelectedMsg); ok {
			got = sm.Session
			return
		}
		if bm, ok := c().(tea.BatchMsg); ok {
			for _, sub := range bm {
				walk(sub)
			}
		}
	}
	walk(cmd)
	if got == nil {
		t.Fatal("expected session selection")
	}
	model, _ = model.Update(components.SessionSelectedMsg{Session: got})
	a = model.(*App)

	a.width, a.height = 100, 40
	a.layout()
	conv := ansiStrip(a.conversation.View())
	for _, want := range []string{"hello agent", "let me think about this", "I will check the file", "Readfile", "reading main.go", "Completed"} {
		if !strings.Contains(conv, want) {
			t.Fatalf("resumed conversation missing %q:\n%s", want, conv)
		}
	}
	// read_file is a lightweight tool: its output stays hidden unless review
	// mode is on, exactly as it was while running.
	if strings.Contains(conv, "package main") {
		t.Fatalf("lightweight tool output should stay hidden, got:\n%s", conv)
	}
	if strings.Contains(conv, "hidden model-only instruction") {
		t.Fatalf("model-only system messages must not appear after resume:\n%s", conv)
	}
	t.Logf("CONV:\n%s", conv)
}
