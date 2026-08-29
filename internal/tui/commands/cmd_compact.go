package commands

// /compact — compact context to fit the token budget.

func compactCommand() Command {
	return Command{
		Name:             "compact",
		Description:      "Compact context to fit token budget",
		Category:         "AI & Model",
		Icon:             "󰕳",
		Tier:             TierSecondary,
		Type:             CmdPrompt,
		Immediate:        true,
		SupportsHeadless: true,
		WhenToUse:        "When the conversation is close to the context budget",
		PromptTemplate:   "Compact the current conversation context. Summarize key points, decisions, and file references so far. Preserve all important context while reducing token count.",
	}
}

func handleCompact(host Host, args []string) Result {
	host.SetStatus("Compacting context...")
	return Done(host.CompactContext())
}
