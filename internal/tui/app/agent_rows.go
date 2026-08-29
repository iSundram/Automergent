package app

// Live subagent rows in the main conversation.
//
// The dock is for background work the user steers; the conversation is where
// the user is looking. Every subagent gets one short row there — its type,
// its task, what it is doing right now — refreshed on the existing live tick
// and settled in place when the agent finishes. Rows never move once added.

import (
	"fmt"
	"strings"

	toolsagent "github.com/iSundram/Automergent/internal/tools/agent"
	"github.com/iSundram/Automergent/internal/tui/render"
)

// syncAgentRows mirrors the agent manager's live instances into conversation
// rows. Idempotent per tick: UpsertAgentRow skips agents whose observable
// facts did not change, so a quiet second costs nothing.
func (a *App) syncAgentRows() {
	instances := toolsagent.GetAgentManager().List(true)
	for _, inst := range instances {
		snap := inst.Snapshot()

		// The subject is the task's first line; the full prompt lives in the
		// agent viewer (/agents → open).
		prompt, _, result, _ := inst.Detail()
		subject := render.FirstLine(prompt)
		activity := agentRowActivity(snap)

		if render.CanonicalStatus(snap.Status).Terminal() {
			if a.settledAgentRows[snap.ID] {
				continue // settled: the row is history now
			}
			a.settledAgentRows[snap.ID] = true
		}

		a.conversation.UpsertAgentRow(
			snap.ID,
			firstNonEmptyDock(snap.Name, snap.ID),
			snap.Type,
			subject,
			agentRowSummary(snap, activity),
			snap.Status,
			render.FirstLine(result),
		)
	}
}

// agentRowSummary composes the activity cell: what it is doing now, plus how
// much work it has done and for how long.
func agentRowSummary(snap toolsagent.AgentSnapshot, activity string) string {
	parts := []string{}
	if strings.TrimSpace(activity) != "" {
		parts = append(parts, activity)
	}
	if snap.ToolCount > 0 {
		parts = append(parts, fmt.Sprintf("%d tools", snap.ToolCount))
	}
	if snap.Elapsed != "" {
		parts = append(parts, snap.Elapsed)
	}
	return strings.Join(parts, " · ")
}

// agentRowActivity answers "what is it doing right now" for one snapshot:
// the tool it is in, or — between tools — the last thing it said.
func agentRowActivity(snap toolsagent.AgentSnapshot) string {
	if snap.CurrentTool != "" {
		return "in " + snap.CurrentTool
	}
	status := render.CanonicalStatus(snap.Status)
	if status.Terminal() {
		return status.Label()
	}
	if line := snap.LastLine; line != "" {
		return render.FirstLine(line)
	}
	if n := len(toolsagent.GetAgentManager().Children(snap.ID)); n > 0 {
		return fmt.Sprintf("%d children", n)
	}
	return ""
}
