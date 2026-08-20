package command

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

// --- Project Handlers ---

func handleTree(host Host, args []string) tea.Cmd {
	host.ToggleFileTree()
	host.SetStatus("File tree toggled")
	return nil
}

func handleDiff(host Host, args []string) tea.Cmd {
	host.ToggleDiffPane()
	host.SetStatus("Diff pane toggled")
	return nil
}

func handleLSP(host Host, args []string) tea.Cmd {
	host.ToggleLSPPanel()
	host.SetStatus("LSP panel toggled")
	return nil
}

func handleSearch(host Host, args []string) tea.Cmd {
	query := strings.TrimSpace(strings.Join(args, " "))
	if query == "" {
		host.CommandUsage("/search <query>")
		return nil
	}

	result := host.SearchWorkspace(query)
	host.AddSystemMessage(result)
	host.SetStatus("Search completed")
	return nil
}