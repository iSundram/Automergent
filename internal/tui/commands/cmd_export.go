package commands

// /export — export the conversation as Markdown.

func exportCommand() Command {
	return Command{
		Name:             "export",
		Description:      "Export conversation as Markdown",
		Category:         "Session",
		Icon:             "󰈇",
		ArgsHint:         "[path]",
		Tier:             TierSecondary,
		Immediate:        true,
		SupportsHeadless: true,
	}
}

func handleExport(host Host, args []string) Result {
	path := ""
	if len(args) > 0 {
		path = args[0]
	}
	if err := host.ExportConversation(path); err != nil {
		host.CommandError(err.Error())
		return Done(nil)
	}
	if path == "" {
		path = "conversation.md"
	}
	host.AddSystemMessage("Conversation exported to " + path)
	host.SetStatus("Conversation exported")
	return Done(nil)
}
