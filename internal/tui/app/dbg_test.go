package app

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/iSundram/Automergent/internal/ai"
	"github.com/iSundram/Automergent/internal/session"
	"time"
)

func TestDbgCursorMoves(t *testing.T) {
	app := newTestApp(t)
	storage, _ := session.NewStorage(t.TempDir())
	app.storage = storage
	for i := 0; i < 12; i++ {
		s := session.New()
		s.Title = "Session " + string(rune('A'+i))
		s.WorkDir = "/root/OweCode"
		s.UpdatedAt = time.Now().Add(-time.Duration(i) * time.Hour)
		s.AddMessage(ai.NewTextMessage(ai.RoleUser, "hi"))
		storage.Save(s)
	}
	var model tea.Model = app
	for _, ch := range "/sessions" {
		model, _ = model.Update(tea.KeyPressMsg{Code: ch, Text: string(ch)})
	}
	model, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	a := model.(*App)
	t.Logf("after enter: visible=%v cursor=%d swallow=%v", a.sessionBrowser.Visible(), a.cursorOf(), a.swallowNextKey)
	for i := 0; i < 5; i++ {
		model, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyDown})
		a = model.(*App)
		t.Logf("after down %d: cursor=%d", i+1, a.cursorOf())
	}
}

func (a *App) cursorOf() int { return a.sessionBrowser.debugCursor() }
