package components

import (
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/iSundram/Automergent/internal/session"
	"github.com/iSundram/Automergent/internal/tui/themes"
)

// SessionSelectedMsg is sent when the user selects a session.
type SessionSelectedMsg struct {
	Session *session.Session
}

// sessionItem implements list.Item for a session.
type sessionItem struct {
	sess *session.Session
}

func (s sessionItem) Title() string { return s.sess.Title }
func (s sessionItem) Description() string {
	desc := fmt.Sprintf("%d messages", len(s.sess.Messages))
	if s.sess.Provider != "" {
		desc += fmt.Sprintf(" | %s", s.sess.Provider)
		if s.sess.Model != "" {
			desc += fmt.Sprintf("/%s", s.sess.Model)
		}
	}
	if !s.sess.UpdatedAt.IsZero() {
		desc += fmt.Sprintf(" | %s", formatRelativeTime(s.sess.UpdatedAt))
	}
	return desc
}
func (s sessionItem) FilterValue() string { return s.sess.Title }

// formatRelativeTime returns a human-friendly relative time string.
func formatRelativeTime(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		mins := int(d.Minutes())
		if mins == 1 {
			return "1 min ago"
		}
		return fmt.Sprintf("%d mins ago", mins)
	case d < 24*time.Hour:
		hrs := int(d.Hours())
		if hrs == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", hrs)
	case d < 7*24*time.Hour:
		days := int(d.Hours() / 24)
		if days == 1 {
			return "yesterday"
		}
		return fmt.Sprintf("%d days ago", days)
	default:
		return t.Format("Jan 2, 2006")
	}
}

// SessionBrowser shows a list of sessions.
type SessionBrowser struct {
	list    list.Model
	styles  *themes.Styles
	visible bool
}

// NewSessionBrowser creates a new SessionBrowser.
func NewSessionBrowser(styles *themes.Styles) SessionBrowser {
	l := list.New(nil, list.NewDefaultDelegate(), 60, 20)
	l.Title = "Sessions"
	return SessionBrowser{list: l, styles: styles}
}

// SetSessions populates the list.
func (sb *SessionBrowser) SetSessions(sessions []*session.Session) {
	items := make([]list.Item, len(sessions))
	for i, s := range sessions {
		items[i] = sessionItem{sess: s}
	}
	sb.list.SetItems(items)
}

// SetSize updates dimensions.
func (sb *SessionBrowser) SetSize(w, h int) {
	sb.list.SetSize(w, h)
}

// Show displays the browser.
func (sb *SessionBrowser) Show() { sb.visible = true }

// Hide hides the browser.
func (sb *SessionBrowser) Hide() { sb.visible = false }

// Visible reports visibility.
func (sb SessionBrowser) Visible() bool { return sb.visible }

// Update handles list events.
func (sb SessionBrowser) Update(msg tea.Msg) (SessionBrowser, tea.Cmd) {
	if !sb.visible {
		return sb, nil
	}
	if km, ok := msg.(tea.KeyMsg); ok {
		switch km.String() {
		case "esc", "q":
			sb.visible = false
			return sb, nil
		case "enter":
			if item, ok := sb.list.SelectedItem().(sessionItem); ok {
				sb.visible = false
				return sb, func() tea.Msg { return SessionSelectedMsg{Session: item.sess} }
			}
		}
	}
	l, cmd := sb.list.Update(msg)
	sb.list = l
	return sb, cmd
}

// View renders the session browser.
func (sb SessionBrowser) View() string {
	if !sb.visible {
		return ""
	}
	return sb.styles.Border.Render(sb.list.View())
}
