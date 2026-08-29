package commands

import (
	"fmt"
	"strings"
)

// /rename — rename the current session.

func renameCommand() Command {
	return Command{
		Name:             "rename",
		Description:      "Rename the current session",
		Category:         "Session",
		Icon:             "󰘎",
		ArgsHint:         "<title>",
		Tier:             TierTertiary,
		SupportsHeadless: true,
	}
}

func handleRename(host Host, args []string) Result {
	title := strings.TrimSpace(strings.Join(args, " "))
	if title == "" {
		host.CommandUsage("/rename <title>")
		return Done(nil)
	}
	if err := host.RenameSession(title); err != nil {
		host.CommandError(err.Error())
		return Done(nil)
	}
	host.AddSystemMessage(fmt.Sprintf("Session renamed to %q", title))
	host.SetStatus("Session renamed")
	return Done(nil)
}
