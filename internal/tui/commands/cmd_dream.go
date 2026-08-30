package commands

// /dream — consolidate project memory now.
//
// Memory (AUTOMERGENT.md) also consolidates automatically when the agent
// idles with enough new conversation since the last pass; this command
// forces a pass immediately.

func dreamCommand() Command {
	return Command{
		Name:        "dream",
		Description: "Consolidate project memory now",
		Category:    "Configuration",
		Icon:        "󰋘",
		Tier:        TierTertiary,
		Immediate:   true,
	}
}

func handleDream(host Host, args []string) Result {
	host.ConsolidateMemory()
	host.SetStatus("Memory consolidation started")
	return Done(nil)
}
