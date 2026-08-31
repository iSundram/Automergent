package agent

import (
	"fmt"
	"strings"

	"github.com/iSundram/Automergent/internal/agent/agentdef"
	"github.com/iSundram/Automergent/internal/shared"
)

// Fleet listing: the subagent roster rendered into the main prompt so the
// model knows WHO it can delegate to and WHEN — the reference agent's
// pattern. Without this the task tool's enum is just names with no judgment
// attached, and delegation quality collapses.

// FleetEntry is one renderable line of the roster.
type FleetEntry struct {
	Name        string
	Description string
	WhenToUse   string
}

// Fleet renders the registry's agent definitions as a prompt block, with
// the main agent listed first as the inline executor.
func Fleet(registry *Registry) string {
	entries := []FleetEntry{
		{
			Name:        "main",
			Description: "The main agent running the phase arc (you)",
			WhenToUse:   "Default: do the work inline unless a specialized agent fits better.",
		},
	}
	seen := map[string]bool{"main": true}

	appendDef := func(def *agentdef.AgentDefinition) {
		if def == nil || seen[def.Name] {
			return
		}
		seen[def.Name] = true
		entries = append(entries, FleetEntry{
			Name:        def.Name,
			Description: def.Description,
			WhenToUse:   def.WhenToUse,
		})
	}

	if registry != nil {
		for _, def := range registry.List() {
			appendDef(def)
		}
	}

	var sb strings.Builder
	sb.WriteString("## Available Agents\n")
	sb.WriteString("Delegate work with the task tool. Choose the agent that matches the job:\n")
	for _, e := range entries {
		sb.WriteString("- **" + e.Name + "**: " + e.Description)
		if e.WhenToUse != "" {
			sb.WriteString(" — " + e.WhenToUse)
		}
		sb.WriteString("\n")
	}
	sb.WriteString(delegationProtocol)
	return sb.String()
}

// delegationProtocol is the orchestration contract for the main agent,
// distilled from the reference orchestrator: when to answer directly vs
// delegate, how to brief subagents, and how to handle their completions.
// The notification rules matter as much as the delegation rules — an
// orchestrator that re-summarizes every subagent reply doubles the output
// the user has to read, and one that answers its own prompt instead of
// delegating starves the fleet.
const delegationProtocol = `
Delegation protocol:
- Answer WITHOUT delegating only when the reply is fully contained in what you already know: status of running agents, a summary of completed work, or clarifying something you already said. Everything else — when in doubt, delegate.
- Prefer a NEW agent per independent job (parallelism, clean context). RESUME an existing agent only for follow-ups on its own work.
- Brief subagents self-contained: the user's primary request VERBATIM plus only the context you are confident is relevant. Never let extra context overshadow the request; never mention orchestration mechanics in the brief.
- Don't duplicate delegated work: once you hand a search or implementation to an agent, do not also run the same searches yourself. Wait for its result or work on a different task.
- When you start background work, say nothing unless you are directly answering the user or asking a necessary question.
- When a delegated agent reports back: never rephrase or re-summarize its report — the user already sees it. Act on it or queue the next step; if there is nothing to add, end your turn without commentary.
`

// FleetFromRegistry renders the fleet listing for the global registry.
func FleetFromRegistry() string {
	return Fleet(GlobalRegistry())
}

// currentTaskBlock renders the task a phase run is executing: what to do,
// the full instruction the planner wrote for it (the model's definition of
// "done"), the constraints the user attached, the file map init discovered,
// and the routing decision INIT made (which subagent should own it).
func currentTaskBlock(spec shared.TaskSpec) string {
	if strings.TrimSpace(spec.Description) == "" {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("## Current Task\n")
	sb.WriteString(spec.Description + "\n")
	// The planner's detailed instruction carries the actual requirements and
	// referenced files; the one-line description alone is not enough for the
	// model to know what "done" means.
	if p := strings.TrimSpace(spec.Prompt); p != "" && p != strings.TrimSpace(spec.Description) {
		sb.WriteString("\nFull instruction:\n" + p + "\n")
	}
	if files, ok := spec.Context["files_found"].([]string); ok && len(files) > 0 {
		sb.WriteString("\nFiles init discovered for this task:\n")
		for i, f := range files {
			if i >= 30 {
				sb.WriteString(fmt.Sprintf("- ... and %d more\n", len(files)-30))
				break
			}
			sb.WriteString("- " + f + "\n")
		}
	}
	if criteria, ok := spec.Context["success_criteria"].([]string); ok && len(criteria) > 0 {
		sb.WriteString("\nSuccess criteria — the task is done only when all are met:\n")
		for _, c := range criteria {
			sb.WriteString("- " + c + "\n")
		}
	}
	if constraints, ok := spec.Context["constraints"].([]string); ok && len(constraints) > 0 {
		sb.WriteString("Constraints from the user (binding):\n")
		for _, c := range constraints {
			sb.WriteString("- " + c + "\n")
		}
	}
	agentName := spec.Agent
	if agentName == "" {
		agentName = spec.Role
	}
	if agentName != "" && agentName != "main" {
		sb.WriteString("\nRouting: the INIT router assigned this task to the `" + agentName + "` agent. Delegate it with the task tool unless doing it inline is clearly cheaper.\n")
	}
	if reason, ok := spec.Context["reason"].(string); ok && reason != "" {
		sb.WriteString("Router rationale: " + reason + "\n")
	}
	return strings.TrimRight(sb.String(), "\n")
}
