package app

// Dock data feed: turning the two background managers into dock rows.
//
// The dock renders; this file decides what it renders. Three things changed here
// beyond the styling. The dead init() hook that registered an empty shell
// callback is gone — real notification wiring lives in notifications.go, where
// there is a program to send to. Sampling no longer copies whole output buffers
// under the session lock once a second; it reads the cached tail the pump
// maintains. And an agent's children are nested under it rather than listed as
// unrelated siblings, which is the whole difference between a coordinator that
// looks like six random agents and one that looks like a plan.

import (
	"fmt"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	toolsagent "github.com/iSundram/Automergent/internal/tools/agent"
	toolsshell "github.com/iSundram/Automergent/internal/tools/shell"

	"github.com/iSundram/Automergent/internal/tui/components"
	"github.com/iSundram/Automergent/internal/tui/render"
)

// refreshDock pulls live background shells and agents into the dock.
func (a *App) refreshDock() {
	if a.dock == nil {
		return
	}
	entries := make([]components.DockEntry, 0, 8)
	entries = append(entries, a.shellEntries()...)
	entries = append(entries, a.agentEntries()...)
	a.dock.SetEntries(entries)
}

// shellEntries snapshots every background shell, running or finished.
func (a *App) shellEntries() []components.DockEntry {
	records := toolsshell.GetManager().ListRecords(true)
	out := make([]components.DockEntry, 0, len(records))
	for _, rec := range records {
		status := render.CanonicalStatus(string(rec.Status))

		// Activity is the newest thing the process said. For a finished command
		// there is nothing live left to report, so the cell carries the outcome
		// instead — an exit code is more useful than a stale log line.
		activity := ""
		hasStderr := false
		if session, ok := toolsshell.GetManager().Get(rec.ID); ok {
			activity = session.LastLine()
			hasStderr = session.SawStderr()
		}
		if status.Terminal() {
			if rec.ExitCode == 0 && status == render.StatusDone {
				activity = "exit 0"
			} else if status == render.StatusStopped {
				activity = "stopped"
			} else {
				activity = fmt.Sprintf("exit %d", rec.ExitCode)
			}
		}

		out = append(out, components.DockEntry{
			Kind:      components.DockShell,
			ID:        rec.ID,
			Label:     render.FirstLine(rec.Command),
			Status:    status,
			Activity:  activity,
			HasStderr: hasStderr,
			Started:   rec.StartedAt,
			Finished:  rec.CompletedAt,
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Started.Before(out[j].Started) })
	return out
}

// agentEntries snapshots every subagent, nesting children under their parent.
//
// The manager stores a flat map, so the ordering has to be rebuilt here: roots
// in spawn order, each root followed immediately by its own children. Depth is
// capped because a coordinator that spawns a coordinator is legal, and a dock
// row indented eight times is not readable.
func (a *App) agentEntries() []components.DockEntry {
	instances := toolsagent.GetAgentManager().List(true)
	snaps := make([]toolsagent.AgentSnapshot, 0, len(instances))
	byParent := map[string][]int{}
	known := map[string]bool{}

	for _, inst := range instances {
		snaps = append(snaps, inst.Snapshot())
	}
	sort.SliceStable(snaps, func(i, j int) bool { return snaps[i].StartedAt.Before(snaps[j].StartedAt) })
	for _, s := range snaps {
		known[s.ID] = true
	}
	for i, s := range snaps {
		// A parent that is no longer in the list (cleaned up, or never tracked)
		// would orphan its children off the display entirely, so they are
		// promoted to roots rather than dropped.
		parent := s.ParentID
		if parent != "" && !known[parent] {
			parent = ""
		}
		byParent[parent] = append(byParent[parent], i)
	}

	const maxDepth = 3
	out := make([]components.DockEntry, 0, len(snaps))
	var walk func(parent string, depth int)
	walk = func(parent string, depth int) {
		kids := byParent[parent]
		for n, idx := range kids {
			s := snaps[idx]
			out = append(out, a.agentEntry(s, depth, n == len(kids)-1, len(byParent[s.ID])))
			if depth < maxDepth {
				walk(s.ID, depth+1)
			}
		}
	}
	walk("", 0)
	return out
}

// agentEntry builds one agent row.
func (a *App) agentEntry(s toolsagent.AgentSnapshot, depth int, last bool, children int) components.DockEntry {
	status := render.CanonicalStatus(s.Status)

	// The activity cell answers "what is it doing right now", in descending
	// order of usefulness: the tool it is in, the number of children it is
	// waiting on, the last thing it said, or its outcome.
	activity := s.CurrentTool
	switch {
	case status.Terminal():
		activity = status.Label()
	case activity != "":
		// The tool name is the best possible answer; keep it.
	case children > 0:
		activity = fmt.Sprintf("%d children", children)
	case s.LastLine != "":
		activity = render.FirstLine(s.LastLine)
	}

	// An agent that is running but has said nothing for a while is the state
	// that most often means stuck, and a bare "running" hides it.
	if status == render.StatusRunning && s.Idle > idleAgentThreshold {
		activity = fmt.Sprintf("quiet %s", render.Elapsed(int(s.Idle.Seconds())))
	}

	return components.DockEntry{
		Kind:      components.DockAgent,
		ID:        s.ID,
		Label:     firstNonEmptyDock(s.Name, s.ID),
		AgentKind: render.CanonicalKind(s.Type),
		Status:    status,
		Activity:  activity,
		Depth:     depth,
		Last:      last,
		Children:  children,
		ToolCount: s.ToolCount,
		Started:   s.StartedAt,
	}
}

// idleAgentThreshold is how long a running agent may say nothing before the dock
// says so. Thirty seconds is long enough that a slow model response does not
// trip it and short enough that a wedged agent is noticed.
const idleAgentThreshold = 30 * time.Second

func firstNonEmptyDock(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// runningBackgroundCount reports how many background tasks are in flight, which
// is what the one-row rail summarises.
func (a *App) backgroundCounts() (running, failed, total int) {
	if a.dock == nil {
		return 0, 0, 0
	}
	for _, e := range a.dock.Entries() {
		total++
		switch e.Status {
		case render.StatusRunning, render.StatusIdle, render.StatusQueued:
			running++
		case render.StatusFailed:
			failed++
		}
	}
	return running, failed, total
}

// dockFocusActive reports whether the dock owns the keyboard.
func (a *App) dockFocusActive() bool { return a.dock != nil && a.dock.Focused() }

// dockRailView renders the always-on one-row summary of background work.
//
// This is the dock's resting state: exactly one dim row between the spinner and
// the prompt, no chrome, answering the only two questions an idle user has — is
// anything running, and did anything break. It never changes height, so unlike
// the old always-open dock it cannot reflow the conversation while work runs.
func (a *App) dockRailView() string {
	if a.dock == nil || a.zenMode || a.confirm.Visible() {
		return ""
	}
	text := a.dock.RailText()
	if text == "" {
		return ""
	}
	_, failed, _ := a.dock.Counts()
	mark := render.GlyphRun
	color := a.styles.T.Subtext
	if failed > 0 {
		mark = render.GlyphWarn
		color = a.styles.T.Red
	}
	line := lipgloss.NewStyle().Foreground(color).Render(mark) + " " + text
	if !a.dock.Focused() {
		line += render.GlyphSep + "↓ dock"
	}
	return "  " + a.styles.Dim.Render(line)
}

// focusDock moves keyboard ownership from the input into the dock tray.
func (a *App) focusDock() bool {
	if a.dock == nil || !a.inputFocused() {
		return false
	}
	a.refreshDock()
	if !a.dock.HasContent() {
		return false
	}
	a.input.SetPromptVisible(false)
	a.input.Blur()
	a.dock.SetFocused(true)
	a.layout()
	return true
}

// unfocusDock returns keyboard ownership to the input.
func (a *App) unfocusDock() {
	if a.dock == nil {
		return
	}
	a.dock.SetFocused(false)
	a.input.SetPromptVisible(true)
	a.statusBar.SetStatus("Ready")
	a.layout()
}

// stopDockEntry cancels the selected background task, shell or agent.
//
// Killing a shell used to Delete() the session immediately after marking it
// cancelled, which threw away the output the user had just been reading and
// removed the row before it could confirm anything happened. The record now
// survives as a stopped row with its output intact, which is also what makes it
// inspectable afterwards.
func (a *App) stopDockEntry(entry components.DockEntry) tea.Cmd {
	switch entry.Kind {
	case components.DockShell:
		session, ok := toolsshell.GetManager().Get(entry.ID)
		if !ok {
			return a.notice("warn", "Already gone", entry.ID)
		}
		if session.IsCompleted() {
			return a.notice("info", "Already finished", entry.Label)
		}
		session.Cancel()
		_ = session.Kill()
		_ = toolsshell.GetManager().UpdateStatus(entry.ID, toolsshell.SessionStatusCancelled, -1, nil)
		a.refreshDock()
		return a.notice("warn", "Stopped", entry.Label)

	case components.DockAgent:
		if entry.Status.Terminal() {
			return a.notice("info", "Already finished", entry.Label)
		}
		out := toolsagent.ControlAction("interrupt", entry.ID, "", "sync")
		a.refreshDock()
		return a.notice("warn", "Stopped", firstNonEmptyDock(out, entry.Label))
	}
	return nil
}

// stopAllRunning cancels every in-flight background task.
func (a *App) stopAllRunning() tea.Cmd {
	if a.dock == nil {
		return nil
	}
	stopped := 0
	for _, e := range a.dock.Entries() {
		if e.Status.Terminal() {
			continue
		}
		switch e.Kind {
		case components.DockShell:
			if session, ok := toolsshell.GetManager().Get(e.ID); ok && !session.IsCompleted() {
				session.Cancel()
				_ = session.Kill()
				_ = toolsshell.GetManager().UpdateStatus(e.ID, toolsshell.SessionStatusCancelled, -1, nil)
				stopped++
			}
		case components.DockAgent:
			toolsagent.ControlAction("interrupt", e.ID, "", "sync")
			stopped++
		}
	}
	a.refreshDock()
	if stopped == 0 {
		return a.notice("info", "Nothing running", "")
	}
	return a.notice("warn", "Stopped all", fmt.Sprintf("%d background %s", stopped, plural(stopped, "task")))
}

// notice pushes a transient toast, falling back to the status bar when the toast
// stack is unavailable. Dock actions are UI acknowledgements, so they belong
// here and not in the transcript.
func (a *App) notice(level, title, detail string) tea.Cmd {
	if a.toasts == nil {
		if detail != "" {
			a.statusBar.SetStatus(title + ": " + detail)
		} else {
			a.statusBar.SetStatus(title)
		}
		return nil
	}
	return a.toasts.Push(level, title, detail)
}

// plural renders a bare noun or its plural.
func plural(n int, noun string) string {
	if n == 1 {
		return noun
	}
	return noun + "s"
}

// handleDockKeys consumes keys while the dock tray is focused.
// Reports whether the key was handled, and any command it produced.
func (a *App) handleDockKeys(m tea.KeyMsg) (tea.Cmd, bool) {
	switch m.String() {
	case "esc", "q":
		a.unfocusDock()
		return nil, true
	case "up", "k":
		a.dock.MoveCursor(-1)
		return nil, true
	case "down", "j":
		a.dock.MoveCursor(1)
		return nil, true
	case "home", "g":
		a.dock.CursorTo(0)
		return nil, true
	case "end", "G":
		a.dock.CursorTo(a.dock.Len() - 1)
		return nil, true
	case "enter":
		if entry, ok := a.dock.Selected(); ok {
			a.openInspector(entry)
		}
		return nil, true
	case "s":
		if entry, ok := a.dock.Selected(); ok {
			return a.stopDockEntry(entry), true
		}
		return nil, true
	case "S":
		return a.stopAllRunning(), true
	}
	return nil, false
}
