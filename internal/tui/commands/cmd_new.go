package commands

// /new — start a fresh session.

func newCommand() Command {
	return Command{
		Name:             "new",
		Description:      "Start a fresh session",
		Category:         "Session",
		Icon:             "󰐕",
		Tier:             TierPrimary,
		Immediate:        true,
		SupportsHeadless: true,
	}
}

func handleNew(host Host, args []string) Result {
	host.NewSession()
	return Done(nil)
}
