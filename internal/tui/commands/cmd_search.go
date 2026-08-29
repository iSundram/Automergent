package commands

import (
	"strings"
)

// /search — search workspace content.

func searchCommand() Command {
	return Command{
		Name:             "search",
		Description:      "Search workspace content",
		Category:         "Project",
		Icon:             "󰍉",
		ArgsHint:         "<query>",
		Tier:             TierSecondary,
		SupportsHeadless: true,
		Completion: func(h Host, partial string) []string {
			// Suggest recent search dirs and common terms.
			return prefixFilter(h.ExtraSearchDirs(), partial)
		},
	}
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
