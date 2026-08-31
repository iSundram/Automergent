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
// still active and still unnamed. The title lands silently — no status noise —
// and is saved immediately so it survives exit; when the session browser is
// open it is refreshed so the new name shows up live.
func (a *App) applySessionTitle(m sessionTitledMsg) tea.Cmd {
	title := strings.TrimSpace(m.title)
	if title == "" || a.sess == nil || a.sess.ID != m.sessionID || a.sess.Title != "" {
		return nil
	}
	a.sess.Title = title
	if a.storage != nil {
		if err := a.storage.Save(a.sess); err != nil {
			// Silent on success; a lost save is worth one status line
			// because the title would silently vanish on exit.
			a.statusBar.SetStatus("Session title save failed")
			return nil
		}
	}
	if a.sessionBrowser.Visible() {
		return func() tea.Msg {
			sessions, err := a.storage.List()
			if err != nil {
				return nil
			}
			return sessionsLoadedMsg{sessions: a.projectSessions(sessions)}
		}
	}
	return nil
}
