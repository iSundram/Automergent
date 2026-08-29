package commands

// The /agents page: a live roster of running and finished subagents with
// numbered jump-in actions, plus the static catalogue of agent types.

import (
	"fmt"

	"github.com/iSundram/Automergent/internal/tui/components"
)

// agentTypes is the static catalogue shown under the live roster: what each
// builtin agent type is for, so the page answers "which agent should I ask
// for" as well as "what are my agents doing".
var agentTypes = [][2]string{
	{"general-purpose", "full-capability agent for complex tasks (all tools, incl. shell)"},
	{"explore", "fast read-only codebase exploration"},
	{"review", "code review, bug detection, security"},
	{"contexter", "context compaction & memory"},
	{"coordinator", "orchestrates other agents"},
}

// agentsPage builds the structured /agents page.
func agentsPage(h Host) components.Page {
	roster := h.AgentRoster()

	running := make([][2]string, 0, len(roster))
	finished := make([][2]string, 0, len(roster))
	for _, r := range roster {
		row := fmt.Sprintf("%s (%s) · %s", r.Name, r.Type, r.Activity)
		if r.Elapsed != "" {
			row += " · " + r.Elapsed
		}
		if r.ToolCount > 0 {
			row += fmt.Sprintf(" · %d tools", r.ToolCount)
		}
		if r.Terminal {
			finished = append(finished, components.Row(r.ID, row))
		} else {
			running = append(running, components.Row(r.ID, row))
		}
	}

	sections := []components.PageSection{}
	if len(running) > 0 {
		sections = append(sections, components.PageSection{Heading: "Running", Rows: running})
	} else {
		sections = append(sections, components.PageSection{
			Heading: "Running",
			Lines:   []string{"No subagents in flight."},
		})
	}
	if len(finished) > 0 {
		sections = append(sections, components.PageSection{Heading: "Finished", Rows: finished})
	}
	sections = append(sections, components.PageSection{Heading: "Agent types", Rows: agentTypes[:]})

	// Numbered jump-in actions for the first nine agents (running first, then
	// finished — same order as the rows above).
	actions := []components.PageAction{}
	for i, r := range roster {
		if i >= 9 {
			break
		}
		actions = append(actions, components.PageAction{
			Key:     fmt.Sprintf("%d", i+1),
			Label:   "Open " + r.Name,
			Command: "agents",
			Args:    []string{"open", r.ID},
		})
	}
	actions = append(actions, components.PageAction{Key: "r", Label: "Refresh", Command: "agents"})

	return components.Page{
		Title:    "Agents",
		Subtitle: fmt.Sprintf("%d running · %d finished", len(running), len(finished)),
		Sections: sections,
		Actions:  actions,
	}
}
