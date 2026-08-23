package commands

import (
	tea "charm.land/bubbletea/v2"
)

// --- System Handlers ---

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
