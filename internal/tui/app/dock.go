package app

// Bottom dock: background shells + subagents tray under the input.
// ↓ from the input moves focus here; ↑ returns. Enter inspects the entry
// (shell output tail / agent transcript) in an overlay pane.

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	toolsagent "github.com/iSundram/Automergent/internal/tools/agent"
	toolsshell "github.com/iSundram/Automergent/internal/tools/shell"

	"github.com/iSundram/Automergent/internal/tui/components"
)

// refreshDock pulls live background shells and agents into the dock.
func (a *App) refreshDock() {
	if a.dock == nil {
		return
	}
	var entries []components.DockEntry

	for _, rec := range toolsshell.GetManager().ListRecords(true) {
		label := rec.Command
		if i := strings.IndexAny(label, "\n"); i >= 0 {
			label = label[:i]
		}
		detail := time.Since(rec.StartedAt).Round(time.Second).String()
		if rec.Status != "running" && !rec.CompletedAt.IsZero() {
			detail = fmt.Sprintf("exit %d", rec.ExitCode)
		}
		entries = append(entries, components.DockEntry{
			Kind:    components.DockShell,
			ID:      rec.ID,
			Label:   label,
			Status:  string(rec.Status),
			Detail:  detail,
			Created: rec.StartedAt,
		})
	}

	for _, inst := range toolsagent.GetAgentManager().List(true) {
		snap := inst.Snapshot()
		entries = append(entries, components.DockEntry{
			Kind:    components.DockAgent,
			ID:      snap.ID,
			Label:   firstNonEmptyDock(snap.Name, snap.ID),
			Status:  snap.Status,
			Detail:  fmt.Sprintf("%s · %s", snap.Type, snap.Elapsed),
			Created: inst.StartedAt,
		})
	}

	a.dock.SetEntries(entries)
}

func firstNonEmptyDock(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// dockFocusActive reports whether the dock owns the keyboard.
func (a *App) dockFocusActive() bool { return a.dock != nil && a.dock.Focused() }

// focusDock moves keyboard ownership from the input into the dock.
func (a *App) focusDock() bool {
	if a.dock == nil || !a.dock.HasContent() || a.inputFocused() == false {
		return false
	}
	a.refreshDock()
	if !a.dock.HasContent() {
		return false
	}
	a.input.SetPromptVisible(false)
	a.input.Blur()
	a.dock.SetFocused(true)
	a.statusBar.SetStatus("Background tasks — ↑/esc back, enter inspect")
	return true
}

// unfocusDock returns keyboard ownership to the input.
func (a *App) unfocusDock() {
	a.dock.SetFocused(false)
	a.input.SetPromptVisible(true)
	a.statusBar.SetStatus("Ready")
}

// handleDockKeys consumes keys while the dock is focused.
// Reports whether the key was handled.
func (a *App) handleDockKeys(m tea.KeyMsg) bool {
	switch m.String() {
	case "up", "esc":
		a.unfocusDock()
		return true
	case "down", "j":
		a.dock.MoveCursor(1)
		return true
	case "k":
		a.dock.MoveCursor(-1)
		return true
	case "enter":
		if entry, ok := a.dock.Selected(); ok {
			a.inspectDockEntry(entry)
		}
		return true
	}
	return false
}

// inspectDockEntry opens the entry's details in the diff overlay:
// shell → output tail; agent → transcript summary.
func (a *App) inspectDockEntry(entry components.DockEntry) {
	switch entry.Kind {
	case components.DockShell:
		tail := "(no output yet)"
		if session, ok := toolsshell.GetManager().Get(entry.ID); ok {
			out := session.Stdout.String()
			errOut := session.Stderr.String()
			combined := out
			if errOut != "" {
				combined += "\n[stderr]\n" + errOut
			}
			if lines := countLines(combined); lines > 200 {
				combined = lastLines(combined, 200)
			}
			if strings.TrimSpace(combined) != "" {
				tail = combined
			}
		}
		a.diffPane.SetContent(fmt.Sprintf("# shell %s\n$ %s\n\n%s", entry.ID, entry.Label, tail))
	case components.DockAgent:
		inst, ok := toolsagent.GetAgentManager().Get(entry.ID)
		if !ok {
			a.conversation.AddMessage("system", "agent no longer available: "+entry.ID, true)
			return
		}
		snap := inst.Snapshot()
		var b strings.Builder
		fmt.Fprintf(&b, "# agent %s [%s]\ntype: %s · turns: %d · elapsed: %s\n",
			snap.Name, snap.Status, snap.Type, snap.Turns, snap.Elapsed)
		b.WriteString("\n" + inst.LastOutput())
		a.diffPane.SetContent(b.String())
	}
	if !a.diffPane.Visible() {
		a.diffPane.Toggle()
	}
	a.layout()
	a.reviewingProposal = "" // plain inspection, not proposal review
}

func countLines(s string) int { return strings.Count(s, "\n") + 1 }

func lastLines(s string, n int) string {
	lines := strings.Split(s, "\n")
	if len(lines) <= n {
		return s
	}
	return strings.Join(lines[len(lines)-n:], "\n")
}
