package commands

import (
	"fmt"
	"strings"
)

// /rewind — list or restore conversation checkpoints.
//
// Checkpoints are captured before each agent turn and re-derived from the
// message history on resume, so rewinding works across restarts. The
// checkpoint labeled with a turn's prompt restores the conversation to the
// state just before that turn ran.

func rewindCommand() Command {
	return Command{
		Name:             "rewind",
		Description:      "List or restore conversation checkpoints",
		Category:         "Session",
		Icon:             "󰤄",
		ArgsHint:         "[index|list]",
		Tier:             TierSecondary,
		Immediate:        true,
		SupportsHeadless: true,
		Enabled:          func(h Host) bool { return !h.Thinking() },
		DisabledReason:   func(h Host) string { return "Agent is running — /cancel it first" },
	}
}

func handleRewind(host Host, args []string) Result {
	if host.Thinking() {
		host.CommandError("Agent is running — /cancel it before rewinding")
		return Done(nil)
	}
	checkpoints := host.Checkpoints()
	if len(args) == 0 {
		if len(checkpoints) == 0 {
			host.AddSystemMessage("No checkpoints yet — they are captured before each agent turn and re-derived when a session is resumed.")
			return Done(nil)
		}
		host.OpenRewindPicker()
		return Done(nil)
	}
	if args[0] == "list" {
		if len(checkpoints) == 0 {
			host.AddSystemMessage("No checkpoints yet.")
			return Done(nil)
		}
		var b strings.Builder
		b.WriteString("Rewind checkpoints:\n")
		for _, cp := range checkpoints {
			fmt.Fprintf(&b, "  %d. %s\n", cp.Index, cp.Label)
		}
		b.WriteString("\n/rewind <index> restores; checkpoint N holds the state before the turn shown at N.")
		host.AddSystemMessage(b.String())
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
