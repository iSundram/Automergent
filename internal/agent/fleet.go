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
	return sb.String()
}

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
