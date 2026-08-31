package commands

import (
	"strings"
)

// /plan — plan a task before any changes are made.
//
// A thin launcher for the real plan machinery: the model enters plan mode
// with enter_plan_mode (read-only enforcement, previous mode remembered),
// writes the plan as a plan artifact, and presents it with exit_plan_mode
// for approval in the /artifact browser. Approval restores the previous
// mode and unlocks implementation.

func planCommand() Command {
	return Command{
		Name:        "plan",
		Description: "Plan a task before changing anything",
		Category:    "Workflow",
		Icon:        "󰈙",
		ArgsHint:    "<task>",
		Tier:        TierPrimary,
		Type:        CmdPrompt,
		PromptTemplate: "Plan this task before making any changes. " +
			"Call enter_plan_mode first, explore read-only, then write the plan as a plan artifact " +
			"(.automergent/artifacts/plan.md) and call exit_plan_mode to present it for my approval. " +
			"Task: $ARGUMENTS",
		WhenToUse: "Before making non-trivial changes",
	}
}

func handlePlan(host Host, args []string) Result {
	// Fallback for dispatch paths that bypass template expansion; the same
	// flow as the PromptTemplate.
	var b strings.Builder
	b.WriteString("Plan this task before making any changes. ")
	b.WriteString("Call enter_plan_mode first, explore read-only, then write the plan as a plan artifact ")
	b.WriteString("(.automergent/artifacts/plan.md) and call exit_plan_mode to present it for my approval.")
	if focus := strings.TrimSpace(strings.Join(args, " ")); focus != "" {
		b.WriteString("\nTask: " + focus)
	}
	host.SetStatus("Planning...")
	return Done(host.StartAgent(b.String()))
}
