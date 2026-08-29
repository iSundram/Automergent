package commands

import (
	"strings"
)

// /directory (alias /dirs) — manage extra search directories.

func directoryCommand() Command {
	return Command{
		Name:        "directory",
		Aliases:     []string{"dirs"},
		Description: "Manage extra search directories",
		Category:    "Project",
		Icon:        "󰉖",
		ArgsHint:    "[add <path>|show]",
		SubPalette:  "directory",
		Tier:        TierSecondary,
		SubCommands: []SubCommand{
			{Name: "add", Description: "Add a search directory", ArgsHint: "<path>", Handler: handleDirectory},
			{Name: "show", Description: "Show extra search directories", Handler: handleDirectory},
		},
		Completion: func(h Host, partial string) []string {
			return prefixFilter([]string{"add", "show"}, partial)
		},
	}
}

func handleDirectory(host Host, args []string) Result {
	if len(args) == 0 {
		dirs := host.ExtraSearchDirs()
		if len(dirs) == 0 {
			host.AddSystemMessage("No extra search directories.\nUse /directory add <path> or /add-dir <path>")
		} else {
			var b strings.Builder
			b.WriteString("Extra search directories:\n")
			for _, d := range dirs {
				b.WriteString("  " + d + "\n")
			}
			host.AddSystemMessage(b.String())
		}
		return Done(nil)
	}
	switch args[0] {
	case "add":
		if len(args) < 2 {
			host.CommandUsage("/directory add <path>")
			return Done(nil)
		}
		if err := host.AddSearchDir(args[1]); err != nil {
			host.CommandError(err.Error())
		} else {
			host.AddSystemMessage("Added search directory: " + args[1])
			host.SetStatus("Search dir added")
		}
	case "show":
		dirs := host.ExtraSearchDirs()
		if len(dirs) == 0 {
			host.AddSystemMessage("No extra search directories.")
		} else {
			host.AddSystemMessage("Extra search directories:\n  " + strings.Join(dirs, "\n  "))
		}
	default:
		host.CommandUsage("/directory [add <path>|show]")
	}
	return Done(nil)
}
