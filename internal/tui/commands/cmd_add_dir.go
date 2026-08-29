package commands

import (
	"path/filepath"
	"strings"
)

// /add-dir — add an extra search root for /search.

func addDirCommand() Command {
	return Command{
		Name:             "add-dir",
		Description:      "Add an extra search root for /search",
		Category:         "Project",
		Icon:             "󰉖",
		ArgsHint:         "<path>",
		Tier:             TierTertiary,
		SupportsHeadless: true,
		Completion: func(h Host, partial string) []string {
			return prefixFilter(h.ExtraSearchDirs(), partial)
		},
	}
}

func handleAddDir(host Host, args []string) Result {
	path := strings.TrimSpace(strings.Join(args, " "))
	if path == "" {
		usage := "/add-dir <path>"
		if dirs := host.ExtraSearchDirs(); len(dirs) > 0 {
			host.CommandUsage(usage)
			host.AddSystemMessage("Current extra search roots:\n  " + strings.Join(dirs, "\n  "))
			return Done(nil)
		}
		host.CommandUsage(usage)
		return Done(nil)
	}
	if err := host.AddSearchDir(path); err != nil {
		host.CommandError(err.Error())
		return Done(nil)
	}
	abs, _ := filepath.Abs(path)
	host.AddSystemMessage("Added search root: " + abs + "\n/search now also walks this directory.")
	host.SetStatus("Directory added")
	return Done(nil)
}
