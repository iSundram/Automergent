package commands

import (
	"strings"
)

// /run — run a project command via the agent's shell tool.

func runCommand() Command {
	return Command{
		Name:             "run",
		Description:      "Run a project command",
		Category:         "Workflow",
		Icon:             "󰆍",
		ArgsHint:         "<command>",
		Tier:             TierSecondary,
		SubPalette:       "run",
		SupportsHeadless: true,
	}
}

func handleRun(host Host, args []string) Result {
	if len(args) == 0 {
		host.CommandUsage("/run <command>")
		return Done(nil)
	}

	command := strings.TrimSpace(strings.Join(args, " "))
	host.SetStatus("Preparing command permission request")
	return Done(host.StartAgent("Run the following project command using the shell tool. Request permission before execution: " + command))
}
