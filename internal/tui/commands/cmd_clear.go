package commands

// /clear — clear the conversation view.
// /reset — reset current session history.

func clearCommand() Command {
	return Command{
		Name:             "clear",
		Description:      "Clear the view (keep history)",
		Category:         "Session",
		Icon:             "󰃢",
		Tier:             TierPrimary,
		Immediate:        true,
		SupportsHeadless: true,
	}
}

func handleClear(host Host, args []string) Result {
	host.ClearConversationView()
	// In-view feedback: /clear is cosmetic, and nothing else tells the user
	// their history and agent context are still live.
	host.AddSystemMessage("View cleared — history and context retained. Use /reset to wipe them.")
	return Done(nil)
}

func resetCommand() Command {
	return Command{
		Name:            "reset",
		Description:     "Reset current session history",
		Category:        "Session",
		Icon:            "󰑓",
		Tier:            TierSecondary,
		Immediate:       true,
		Enabled:         func(h Host) bool { return !h.Thinking() },
		DisabledReason:  func(h Host) string { return "Agent is running — /cancel it first" },
		SupportsHeadless: true,
	}
}

func handleReset(host Host, args []string) Result {
	if host.Thinking() {
		host.CommandError("Agent is running — /cancel it before resetting history")
		return Done(nil)
	}
	host.ResetSessionHistory()
	return Done(nil)
}
