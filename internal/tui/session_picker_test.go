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
