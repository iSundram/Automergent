package app

// Inspector wiring: the live sources behind the background-task viewer.
//
// The pane takes a source rather than a string so that what it shows is whatever
// the task is currently doing, not what it was doing when the pane opened. The
// old inspection path formatted a snapshot into the diff pane and could never
// update, which for a running build is the least useful possible moment to
// freeze.

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	toolsagent "github.com/iSundram/Automergent/internal/tools/agent"
	toolsshell "github.com/iSundram/Automergent/internal/tools/shell"

	"github.com/iSundram/Automergent/internal/tui/components"
	"github.com/iSundram/Automergent/internal/tui/render"
)

// inspectorTailLines is how much history the inspector holds. Enough to scroll
// back through a test run, bounded so a chatty process cannot grow the pane's
// working set without limit.
const inspectorTailLines = 2000

// shellSource follows one background shell.
type shellSource struct {
	id      string
	command string
}

func (s shellSource) Title() string {
	return "$ " + render.FirstLine(s.command)
}

func (s shellSource) Facts() []string {
	facts := []string{s.id}
	if rec, ok := toolsshell.GetManager().GetRecord(s.id); ok {
		status := render.CanonicalStatus(string(rec.Status))
		facts = append(facts, status.Label())
		if status.Terminal() {
			facts = append(facts, fmt.Sprintf("exit %d", rec.ExitCode))
			if !rec.CompletedAt.IsZero() {
				facts = append(facts, render.Elapsed(int(rec.CompletedAt.Sub(rec.StartedAt).Seconds())))
			}
			if rec.ErrMessage != "" {
				facts = append(facts, rec.ErrMessage)
			}
		} else if !rec.StartedAt.IsZero() {
			facts = append(facts, render.Elapsed(int(time.Since(rec.StartedAt).Seconds())))
		}
	}
	return facts
}

func (s shellSource) Lines() []string {
	session, ok := toolsshell.GetManager().Get(s.id)
	if !ok {
		return []string{"(session released — output is no longer retained)"}
	}
	lines, hidden := session.TailLines(inspectorTailLines)
	if len(lines) == 0 {
		return []string{"(no output yet)"}
	}
	if hidden > 0 {
		return append([]string{fmt.Sprintf("%s %d earlier lines dropped",
			render.GlyphUp, hidden)}, lines...)
	}
	return lines
}

func (s shellSource) Live() bool {
	session, ok := toolsshell.GetManager().Get(s.id)
	return ok && !session.IsCompleted()
}

// agentSource follows one subagent.
type agentSource struct {
	id string
}

func (s agentSource) snapshot() (toolsagent.AgentSnapshot, bool) {
	inst, ok := toolsagent.GetAgentManager().Get(s.id)
	if !ok {
		return toolsagent.AgentSnapshot{}, false
	}
	return inst.Snapshot(), true
}

func (s agentSource) Title() string {
	snap, ok := s.snapshot()
	if !ok {
		return "agent " + s.id
	}
	return firstNonEmptyDock(snap.Name, snap.ID) + "  " +
		string(render.CanonicalKind(snap.Type))
}

func (s agentSource) Facts() []string {
	snap, ok := s.snapshot()
	if !ok {
		return []string{s.id, "released"}
	}
	facts := []string{snap.ID, render.CanonicalStatus(snap.Status).Label()}
	if snap.Elapsed != "" {
		facts = append(facts, snap.Elapsed)
	}
	if snap.ToolCount > 0 {
		facts = append(facts, fmt.Sprintf("%d tools", snap.ToolCount))
	}
	if snap.Turns > 0 {
		facts = append(facts, fmt.Sprintf("%d turns", snap.Turns))
	}
	if snap.CurrentTool != "" {
		facts = append(facts, "in "+snap.CurrentTool)
	}
	if snap.ParentID != "" {
		facts = append(facts, "child of "+snap.ParentID)
	}
	if n := len(toolsagent.GetAgentManager().Children(snap.ID)); n > 0 {
		facts = append(facts, fmt.Sprintf("%d children", n))
	}
	return facts
}

// Lines renders the agent's task, then its turns, newest last. A subagent's
// observable life is its prompt and what it reported back; showing the prompt
// first is what makes a stalled agent diagnosable.
func (s agentSource) Lines() []string {
	inst, ok := toolsagent.GetAgentManager().Get(s.id)
	if !ok {
		return []string{"(agent released)"}
	}
	snap := inst.Snapshot()

	out := []string{"task:"}
	prompt, turns, result, err := inst.Detail()

	for _, l := range strings.Split(strings.TrimRight(prompt, "\n"), "\n") {
		out = append(out, "  "+l)
	}

	for _, turn := range turns {
		out = append(out, "", fmt.Sprintf("turn %d  %s", turn.Index, turn.Duration.Round(1e6)))
		for _, l := range strings.Split(strings.TrimRight(turn.Output, "\n"), "\n") {
			out = append(out, "  "+l)
		}
	}

	if len(turns) == 0 && strings.TrimSpace(result) != "" {
		out = append(out, "", "result:")
		for _, l := range strings.Split(strings.TrimRight(result, "\n"), "\n") {
			out = append(out, "  "+l)
		}
	}
	if err != nil {
		out = append(out, "", "error:", "  "+err.Error())
	}
	if len(turns) == 0 && strings.TrimSpace(result) == "" && err == nil {
		activity := snap.CurrentTool
		if activity == "" {
			activity = snap.LastLine
		}
		if activity == "" {
			activity = "(working — nothing reported yet)"
		}
		out = append(out, "", activity)
	}
	return out
}

func (s agentSource) Live() bool {
	snap, ok := s.snapshot()
	return ok && !render.CanonicalStatus(snap.Status).Terminal()
}

// openInspector opens the full-screen viewer on a dock entry.
func (a *App) openInspector(entry components.DockEntry) {
	if a.inspector == nil {
		return
	}
	switch entry.Kind {
	case components.DockShell:
		a.inspector.Show(shellSource{id: entry.ID, command: entry.Label})
	case components.DockAgent:
		a.inspector.Show(agentSource{id: entry.ID})
	default:
		return
	}
	a.inspectorFilterMode = false
	a.layout()
}

// handleInspectorKeys drives the viewer while it owns the screen. Reports
// whether the key was consumed.
func (a *App) handleInspectorKeys(m tea.KeyMsg) (tea.Cmd, bool) {
	if a.inspector == nil || !a.inspector.Visible() {
		return nil, false
	}

	// Filter entry is a modal sub-state: while it is on, printable keys build the
	// pattern rather than triggering commands, so a filter containing "s" does
	// not stop the task it is filtering.
	if a.inspectorFilterMode {
		switch m.String() {
		case "esc":
			a.inspectorFilterMode = false
			a.inspector.SetFilter("")
			return nil, true
		case "enter":
			a.inspectorFilterMode = false
			return nil, true
		case "backspace":
			f := a.inspector.Filter()
			if f != "" {
				r := []rune(f)
				a.inspector.SetFilter(string(r[:len(r)-1]))
			}
			return nil, true
		default:
			if s := m.String(); len([]rune(s)) == 1 {
				a.inspector.SetFilter(a.inspector.Filter() + s)
				return nil, true
			}
			return nil, true
		}
	}

	switch m.String() {
	case "esc", "q":
		a.inspector.Hide()
		a.layout()
		return nil, true
	case "up", "k":
		a.inspector.Scroll(-1)
		return nil, true
	case "down", "j":
		a.inspector.Scroll(1)
		return nil, true
	case "pgup":
		a.inspector.Scroll(-10)
		return nil, true
	case "pgdown":
		a.inspector.Scroll(10)
		return nil, true
	case "home", "g":
		a.inspector.Scroll(-1 << 20)
		return nil, true
	case "end", "G":
		a.inspector.GotoEnd()
		return nil, true
	case "f":
		a.inspector.ToggleFollow()
		return nil, true
	case "/":
		a.inspectorFilterMode = true
		a.inspector.SetFilter("")
		return nil, true
	case "s":
		if entry, ok := a.dock.Selected(); ok {
			return a.stopDockEntry(entry), true
		}
		return nil, true
	}
	return nil, true // the pane is modal: swallow anything it does not use
}
