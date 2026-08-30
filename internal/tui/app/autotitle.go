package app

import (
	"context"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/iSundram/Automergent/internal/ai"
)

// sessionTitledMsg carries a generated title for the session that was active
// when generation was requested.
type sessionTitledMsg struct {
	sessionID string
	title     string
}

// maybeGenerateSessionTitle fires the background title generator after a
// completed turn, when the session is still unnamed. It is called from the
// EventDone handler so the title lands right after the first exchange.
func (a *App) maybeGenerateSessionTitle() tea.Cmd {
	if a.sess == nil || a.sess.Title != "" {
		return nil
	}
	for _, m := range a.sess.Messages {
		if m.Role == ai.RoleUser {
			id := a.sess.ID
			messages := append([]ai.Message(nil), a.sess.Messages...)
			return func() tea.Msg {
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				title := a.ag.GenerateSessionTitle(ctx, messages)
				if title == "" {
					return nil
				}
				return sessionTitledMsg{sessionID: id, title: title}
			}
		}
	}
	return nil
}

// applySessionTitle stores a generated title. A manual /rename in the interim
// (or a session switch) wins: the title is only applied when the session is
// still active and still unnamed.
func (a *App) applySessionTitle(m sessionTitledMsg) {
	title := strings.TrimSpace(m.title)
	if title == "" || a.sess == nil || a.sess.ID != m.sessionID || a.sess.Title != "" {
		return
	}
	a.sess.Title = title
	if a.storage != nil {
		if err := a.storage.Save(a.sess); err != nil {
			a.statusBar.SetStatus("Session titled (save failed): " + title)
			return
		}
	}
	a.statusBar.SetStatus("Session titled: " + title)
}
