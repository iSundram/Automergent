package commands

// /tree (alias /files) — toggle the project file tree.
// /diff (alias /changes) — review workspace changes.

func treeCommand() Command {
	return Command{
		Name:        "tree",
		Aliases:     []string{"files"},
		Description: "Toggle project file tree",
		Category:    "Project",
		Icon:        "󰙅",
		Tier:        TierSecondary,
		Immediate:   true,
		Current:     func(h Host) bool { return h.ShowingFileTree() },
	}
}

func handleTree(host Host, args []string) Result {
	host.ToggleFileTree()
	host.SetStatus("File tree toggled")
	return Done(nil)
}

func diffCommand() Command {
	return Command{
		Name:          "diff",
		Aliases:       []string{"changes"},
		Description:   "Review workspace changes",
		Category:      "Project",
		Icon:          "󰈙",
		Tier:          TierPrimary,
		Type:          CmdFullPage,
		FullPageTitle: "Diff",
		Immediate:     true,
		Current:       func(h Host) bool { return h.DiffPaneVisible() },
	}
}

func handleDiff(host Host, args []string) Result {
	host.ToggleDiffPane()
	host.SetStatus("Diff pane toggled")
	return Done(nil)
}
