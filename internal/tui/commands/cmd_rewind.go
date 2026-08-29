package commands

import (
	"fmt"
)

// /rewind — list or restore conversation checkpoints.

func rewindCommand() Command {
	return Command{
		Name:             "rewind",
		Description:      "List or restore conversation checkpoints",
		Category:         "Session",
		Icon:             "󰤄",
		ArgsHint:         "[index]",
		Tier:             TierSecondary,
		Immediate:        true,
		SupportsHeadless: true,
	}
}

func handleRewind(host Host, args []string) Result {
	checkpoints := host.Checkpoints()
	if len(args) == 0 {
		if len(checkpoints) == 0 {
			host.AddSystemMessage("No checkpoints yet — they are captured automatically before each agent turn.")
			return Done(nil)
		}
		host.OpenRewindPicker()
		return Done(nil)
	}
	if args[0] == "list" {
		for _, cp := range checkpoints {
			host.AddSystemMessage(fmt.Sprintf("%d. %s", cp.Index, cp.Label))
		}
		return Done(nil)
	}
	n := 0
	if _, err := fmt.Sscanf(args[0], "%d", &n); err != nil || n < 1 {
		host.CommandUsage("/rewind <index>")
		return Done(nil)
	}
	known := false
	for _, cp := range checkpoints {
		if cp.Index == n {
			known = true
			break
		}
	}
	if !known {
		host.CommandError(fmt.Sprintf("no checkpoint at index %d (see /rewind list)", n))
		return Done(nil)
	}
	if err := host.RewindTo(n); err != nil {
		host.CommandError(err.Error())
	}
	return Done(nil)
}
