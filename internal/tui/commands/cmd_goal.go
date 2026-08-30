package commands

import (
	"strconv"
	"strings"
)

// /goal — set or manage the thread goal.
//
// With an objective installed, the host injects a <goal-steering>
// continuation prompt whenever the agent goes idle, driving autonomous
// multi-turn progress until the model reports the goal complete or blocked
// (3 blocked turns pause it). Budget and turn caps bound runaway runs.

func goalCommand() Command {
	return Command{
		Name:        "goal",
		Description: "Set or manage the thread goal",
		Category:    "Session",
		Icon:        "󰘧",
		ArgsHint:    "[clear|pause|resume|continue|status|budget <n>|<objective>]",
		SubPalette:  "goal",
		Tier:        TierSecondary,
		SubCommands: []SubCommand{
			{Name: "clear", Description: "Clear the thread goal", Handler: handleGoal},
			{Name: "pause", Description: "Pause auto-continuation", Handler: handleGoal},
			{Name: "resume", Description: "Resume auto-continuation", Handler: handleGoal},
			{Name: "continue", Description: "Reset turn counter and continue", Handler: handleGoal},
			{Name: "status", Description: "Show goal progress", Handler: handleGoal},
			{Name: "budget", Description: "Set token budget", ArgsHint: "<tokens>", Handler: handleGoal},
		},
		Completion: func(h Host, partial string) []string {
			return prefixFilter([]string{"clear", "pause", "resume", "continue", "status", "budget"}, partial)
		},
	}
}

func handleGoal(host Host, args []string) Result {
	if len(args) == 0 {
		host.AddSystemMessage(host.GoalSnapshot() +
			"\n\nUsage: /goal <objective> to set · /goal budget <n> to cap tokens\n" +
			"/goal pause|resume|continue|clear to control the loop")
		return Done(nil)
	}

	switch args[0] {
	case "clear", "pause", "resume", "continue":
		if msg := host.GoalAction(args[0]); msg != "" {
			host.AddSystemMessage(msg)
			host.SetStatus("Goal " + args[0])
		}
		return Done(nil)
	case "status":
		host.AddSystemMessage(host.GoalSnapshot())
		return Done(nil)
	case "edit":
		host.AddSystemMessage("Goal edit: use /goal <new objective> to replace the goal.")
		return Done(nil)
	}

	// /goal [budget <n>] <objective...>
	objective := args
	budget := 0
	if args[0] == "budget" {
		if len(args) < 3 {
			host.AddSystemMessage("Budget is set with the objective: /goal budget <tokens> <objective>.\nTo change the budget of a running goal, clear and re-set it.")
			return Done(nil)
		}
		n, err := strconv.Atoi(args[1])
		if err != nil || n <= 0 {
			host.CommandUsage("/goal budget <tokens> <objective>")
			return Done(nil)
		}
		budget = n
		objective = args[2:]
	}
	if len(objective) == 0 {
		host.CommandUsage("/goal <objective>")
		return Done(nil)
	}

	host.SetGoal(strings.Join(objective, " "), budget)
	host.AddSystemMessage("Goal set — the agent will continue autonomously until it reports the goal complete.\n" + host.GoalSnapshot())
	host.SetStatus("Goal set")
	return Done(nil)
}
