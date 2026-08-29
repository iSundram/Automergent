package commands

import (
	"strings"
)

// /goal — set or manage the thread goal.

func goalCommand() Command {
	return Command{
		Name:        "goal",
		Description: "Set or manage the thread goal",
		Category:    "Session",
		Icon:        "󰘧",
		ArgsHint:    "[clear|edit|pause|resume|<objective>]",
		SubPalette:  "goal",
		Tier:        TierSecondary,
		SubCommands: []SubCommand{
			{Name: "clear", Description: "Clear the thread goal", Handler: handleGoal},
			{Name: "edit", Description: "Edit the thread goal", Handler: handleGoal},
			{Name: "pause", Description: "Pause the thread goal", Handler: handleGoal},
			{Name: "resume", Description: "Resume the thread goal", Handler: handleGoal},
		},
		Completion: func(h Host, partial string) []string {
			return prefixFilter([]string{"clear", "edit", "pause", "resume"}, partial)
		},
	}
}

func handleGoal(host Host, args []string) Result {
	if len(args) == 0 {
		host.AddSystemMessage("Goal: no goal set.\nUse /goal <objective> to set, /goal clear to clear, /goal pause/resume to toggle.")
		return Done(nil)
	}
	switch args[0] {
	case "clear":
		host.AddSystemMessage("Goal cleared.")
		host.SetStatus("Goal cleared")
	case "pause":
		host.AddSystemMessage("Goal paused.")
		host.SetStatus("Goal paused")
	case "resume":
		host.AddSystemMessage("Goal resumed.")
		host.SetStatus("Goal resumed")
	case "edit":
		host.AddSystemMessage("Goal edit: use /goal <new objective> to set a new goal.")
	default:
		prompt := "Set the thread goal to: " + strings.Join(args, " ") + "\nKeep this objective in context for subsequent turns."
		host.SetStatus("Goal set")
		return Done(host.StartAgent(prompt))
	}
	return Done(nil)
}
