package commands

import (
	"fmt"
)

// /new — start a fresh session.

func newCommand() Command {
	return Command{
		Name:            "new",
		Description:     "Start a fresh session",
		Category:        "Session",
		Icon:            "󰐕",
		Tier:            TierPrimary,
		Immediate:       true,
		Enabled:         func(h Host) bool { return !h.Thinking() },
		DisabledReason:  func(h Host) string { return "Agent is running — /cancel it first" },
		SupportsHeadless: true,
	}
}

func handleNew(host Host, args []string) Result {
	if host.Thinking() {
		host.CommandError("Agent is running — /cancel it before starting a new session")
		return Done(nil)
	}
	oldID := host.SessionID()
	host.NewSession()
	if oldID != "" {
		host.AddSystemMessage(fmt.Sprintf("New session started — previous session %s saved", oldID))
	} else {
		host.AddSystemMessage("New session started")
	}
	return Done(nil)
}
