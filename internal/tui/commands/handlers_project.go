package commands

import (
	"strings"
)

// --- Project Handlers ---

func handleTree(host Host, args []string) Result {
	host.ToggleFileTree()
	host.SetStatus("File tree toggled")
	return Done(nil)
}

func handleDiff(host Host, args []string) Result {
	host.ToggleDiffPane()
	host.SetStatus("Diff pane toggled")
	return Done(nil)
}

func handleSearch(host Host, args []string) Result {
	query := strings.TrimSpace(strings.Join(args, " "))
	if query == "" {
		host.CommandUsage("/search <query>")
		return Done(nil)
	}

	result := host.SearchWorkspace(query)
	host.AddSystemMessage(result)
	host.SetStatus("Search completed")
	return Done(nil)
}
