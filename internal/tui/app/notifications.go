package app

// Background-task notifications.
//
// Both backends already emitted completion events and neither was ever
// connected. The shell manager's hook was registered from a package init() with
// an empty body — a comment saying the tea program would pick it up, and no code
// that did — and shellNotificationMsg was declared, never constructed and never
// handled. The agent manager's RegisterCompletionHook had exactly one caller, in
// its own test file. So a background command or subagent could succeed, fail, or
// die and the user would learn about it only by noticing a row change colour in
// the dock, if the dock happened to be open.
//
// Registration has to happen after tea.Program exists, because the hooks fire on
// arbitrary goroutines and the only safe way into the event loop is p.Send. That
// is why this cannot live in an init(): at package-init time there is no program
// to send to. installNotifications is called from Run once sendToProgram is set.

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	toolsagent "github.com/iSundram/Automergent/internal/tools/agent"
	toolsshell "github.com/iSundram/Automergent/internal/tools/shell"
	"github.com/iSundram/Automergent/internal/tui/render"
)

// shellNotificationMsg reports a background shell reaching a terminal state.
type shellNotificationMsg struct {
	id       string
	command  string
	status   render.Status
	exitCode int
	errMsg   string
	duration time.Duration
}

// agentNotificationMsg reports a background subagent reaching a terminal state.
type agentNotificationMsg struct {
	id       string
	name     string
	kind     render.AgentKind
	status   render.Status
	summary  string
	errMsg   string
	duration time.Duration
}

// installNotifications connects both managers' completion hooks to the event
// loop. Safe to call once per program; the managers append hooks rather than
// replacing them, so calling it twice would double every notification.
func (a *App) installNotifications() {
	send := a.sendToProgram
	if send == nil {
		return
	}

	toolsshell.GetManager().RegisterStatusHook(func(n toolsshell.SessionNotification) {
		send(shellNotificationMsg{
			id:       n.ID,
			command:  render.FirstLine(n.Command),
			status:   render.CanonicalStatus(string(n.Status)),
			exitCode: n.ExitCode,
			errMsg:   n.ErrMessage,
			duration: n.Duration,
		})
	})

	toolsagent.GetAgentManager().RegisterCompletionHook(func(n toolsagent.AgentNotification) {
		send(agentNotificationMsg{
			id:       n.AgentID,
			name:     firstNonEmptyDock(n.Name, n.AgentID),
			kind:     render.CanonicalKind(string(n.Type)),
			status:   render.CanonicalStatus(string(n.Status)),
			summary:  render.FirstLine(n.Result),
			errMsg:   n.ErrMessage,
			duration: n.Duration,
		})
	})
}

// handleShellNotification reports a finished background command.
//
// The message goes to the toast stack, not the transcript. A background shell
// finishing is something the UI noticed, not something the conversation said,
// and the previous habit of writing these through conversation.AddMessage put UI
// bookkeeping in the middle of the model's reasoning where it later had to be
// read back as context.
func (a *App) handleShellNotification(m shellNotificationMsg) tea.Cmd {
	a.refreshDock()

	label := m.command
	if label == "" {
		label = m.id
	}
	detail := render.Elapsed(int(m.duration.Seconds()))
	level := "info"
	title := "Command finished"
	switch m.status {
	case render.StatusFailed:
		level = "error"
		title = fmt.Sprintf("Command failed (exit %d)", m.exitCode)
		if m.errMsg != "" {
			detail = m.errMsg
		}
	case render.StatusStopped:
		level = "warn"
		title = "Command stopped"
	}
	if a.toasts == nil {
		a.statusBar.SetStatus(title + ": " + label)
		return nil
	}
	return a.toasts.Push(level, title, label+render.GlyphSep+detail)
}

// handleAgentNotification reports a finished background subagent.
func (a *App) handleAgentNotification(m agentNotificationMsg) tea.Cmd {
	a.refreshDock()

	detail := render.Elapsed(int(m.duration.Seconds()))
	if m.summary != "" {
		detail = render.Clip(m.summary, 60)
	}
	level := "info"
	title := fmt.Sprintf("Agent %s finished", m.name)
	switch m.status {
	case render.StatusFailed:
		level = "error"
		title = fmt.Sprintf("Agent %s failed", m.name)
		if m.errMsg != "" {
			detail = render.Clip(m.errMsg, 60)
		}
	case render.StatusStopped:
		level = "warn"
		title = fmt.Sprintf("Agent %s stopped", m.name)
	}
	if a.toasts == nil {
		a.statusBar.SetStatus(title)
		return nil
	}
	return a.toasts.Push(level, title, strings.TrimSpace(detail))
}
