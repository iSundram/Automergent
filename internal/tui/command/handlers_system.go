package command

import (
	tea "charm.land/bubbletea/v2"
)

// --- System Handlers ---

func handleStats(host Host, args []string) tea.Cmd {
	host.ShowStats()
	return nil
}

func handleHelp(host Host, args []string) tea.Cmd {
	host.ShowHelp()
	return nil
}

func handleQuit(host Host, args []string) tea.Cmd {
	return tea.Quit
}