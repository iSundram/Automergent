package commands

import (
	tea "charm.land/bubbletea/v2"
)

// /stats — show session statistics.
// /help — open keyboard and command help.
// /quit (alias /exit) — exit Automergent.
// /version — show the Automergent version.

func statsCommand() Command {
	return Command{
		Name:          "stats",
		Description:   "Show session statistics",
		Category:      "System",
		Icon:          "󰄪",
		Tier:          TierTertiary,
		Type:          CmdFullPage,
		FullPageTitle: "Statistics",
		Immediate:     true,
	}
}

func helpCommand() Command {
	return Command{
		Name:          "help",
		Description:   "Open keyboard and command help",
		Category:      "System",
		Icon:          "󰋖",
		Tier:          TierSecondary,
		Type:          CmdFullPage,
		FullPageTitle: "Help",
		Immediate:     true,
	}
}

func quitCommand() Command {
	return Command{
		Name:        "quit",
		Aliases:     []string{"exit"},
		Description: "Exit Automergent",
		Category:    "System",
		Icon:        "󰗼",
		Tier:        TierSecondary,
		Immediate:   true,
	}
}

func handleStats(host Host, args []string) Result {
	host.ShowStats()
	return Done(nil)
}

func handleHelp(host Host, args []string) Result {
	host.ShowHelp()
	return Done(nil)
}

func handleQuit(host Host, args []string) Result {
	return Done(tea.Quit)
}
