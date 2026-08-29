package commands

import (
	"strings"
)

// /summary — generate an LLM summary of the session.

func summaryCommand() Command {
	return Command{
		Name:             "summary",
		Description:      "Generate an LLM summary of the session",
		Category:         "Session",
		Icon:             "󰑝",
		ArgsHint:         "[emphasis]",
		Tier:             TierTertiary,
		Type:             CmdPrompt,
		SupportsHeadless: true,
		WhenToUse:        "When a thorough written recap of goals, changes and open items is needed",
		PromptTemplate:   "Generate a comprehensive summary of this session. Include: goals discussed, key decisions made, files modified, outstanding issues, and next steps.$ARGUMENTS",
	}
}

func handleSummary(host Host, args []string) Result {
	var b strings.Builder
	b.WriteString("Summarize this session so far for the user.\n")
	b.WriteString("Cover: 1) the goals pursued, 2) changes made (files and why), 3) key decisions and their rationale, 4) open items and risks, 5) suggested next steps.\n")
	b.WriteString("Be factual — only claim what is visible in this conversation. Keep it under 300 words.")
	if focus := strings.TrimSpace(strings.Join(args, " ")); focus != "" {
		b.WriteString("\nEmphasis: " + focus)
	}
	host.SetStatus("Preparing summary")
	return Done(host.StartAgent(b.String()))
}
