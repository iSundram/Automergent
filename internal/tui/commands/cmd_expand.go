package commands

// /expand and /collapse — one-move control over every collapsible block in
// the conversation: tool cards, thinking blocks, and shell output.
//
// Collapsed, a settled thinking block shows only its header
// ("✓ Thought for 4s · /expand"); a shell card shows the command row and a
// one-line tail. Expanded, everything opens to its full body. The hints
// rendered under clipped blocks always name the command that flips the
// current state, so the affordance is actionable even mid-typing.

func expandCommand() Command {
	return Command{
		Name:             "expand",
		Description:      "Expand tool cards, thinking blocks, and shell output",
		Category:         "Display",
		Icon:             "⌄",
		ArgsHint:         "",
		Tier:             TierSecondary,
		SupportsHeadless: false,
	}
}

func collapseCommand() Command {
	return Command{
		Name:             "collapse",
		Description:      "Collapse tool cards, thinking blocks, and shell output to single lines",
		Category:         "Display",
		Icon:             "⌃",
		ArgsHint:         "",
		Tier:             TierSecondary,
		SupportsHeadless: false,
	}
}

func handleExpand(host Host, args []string) Result {
	label := host.SetBlocksExpanded(true)
	host.AddSystemMessage(label)
	return Done(nil)
}

func handleCollapse(host Host, args []string) Result {
	label := host.SetBlocksExpanded(false)
	host.AddSystemMessage(label)
	return Done(nil)
}
