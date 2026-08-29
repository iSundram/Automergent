package commands

import (
	"strings"
)

// /branch — fork this conversation into a new session.

func branchCommand() Command {
	return Command{
		Name:             "branch",
		Description:      "Fork this conversation into a new session",
		Category:         "Session",
		Icon:             "󰘬",
		ArgsHint:         "<name>",
		Tier:             TierTertiary,
		SupportsHeadless: true,
	}
}

func handleBranch(host Host, args []string) Result {
	name := strings.TrimSpace(strings.Join(args, " "))
	if name == "" {
		host.CommandUsage("/branch <name>")
		return Done(nil)
	}
	if err := host.BranchSession(name); err != nil {
		host.CommandError(err.Error())
	}
	return Done(nil)
}
