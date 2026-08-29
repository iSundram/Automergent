package commands

// /clear — clear the conversation view.
// /reset — reset current session history.

func clearCommand() Command {
	return Command{
		Name:             "clear",
		Description:      "Clear the conversation view",
		Category:         "Session",
		Icon:             "󰃢",
		Tier:             TierPrimary,
		Immediate:        true,
		SupportsHeadless: true,
	}
}

func handleClear(host Host, args []string) Result {
	host.ClearConversationView()
	return Done(nil)
}

func resetCommand() Command {
	return Command{
		Name:             "reset",
		Description:      "Reset current session history",
		Category:         "Session",
		Icon:             "󰑓",
		Tier:             TierSecondary,
		Immediate:        true,
		SupportsHeadless: true,
	}
}

func handleReset(host Host, args []string) Result {
	host.ResetSessionHistory()
	return Done(nil)
}
