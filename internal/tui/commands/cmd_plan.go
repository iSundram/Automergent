package commands

import (
	"strings"
)

// /plan — enter plan mode (read-only analysis).

func planCommand() Command {
	return Command{
		Name:        "plan",
		Description: "Enter plan mode (read-only analysis)",
		Category:    "Workflow",
		Icon:        "󰈙",
		ArgsHint:    "[focus|copy]",
		SubPalette:  "plan",
		Tier:        TierPrimary,
		Type:        CmdPrompt,
		SubCommands: []SubCommand{
			{Name: "copy", Description: "Copy current plan to clipboard", Handler: handlePlan},
		},
		PromptTemplate: "Enter plan mode. Read-only analysis before making changes. Outline approach, files to touch, and confirm before editing. Focus: $ARGUMENTS",
		WhenToUse:      "Before making non-trivial changes",
	}
}

func handlePlan(host Host, args []string) Result {
	if len(args) > 0 && args[0] == "copy" {
		host.AddSystemMessage("Plan copy: not yet implemented — plan content would be copied to clipboard.")
		return Done(nil)
	}
	prompt := "Enter plan mode. Read-only analysis before making changes. Outline the approach, list files to touch, and ask for confirmation before editing."
	if len(args) > 0 {
		prompt += "\nFocus: " + strings.Join(args, " ")
	}
	host.SetStatus("Planning...")
	return Done(host.StartAgent(prompt))
}
