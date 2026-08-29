package commands

import (
	"strings"
)

// /tldr (alias /explain) — explain current code concisely.

func tldrCommand() Command {
	return Command{
		Name:           "tldr",
		Aliases:        []string{"explain"},
		Description:    "Explain current code concisely",
		Category:       "Knowledge",
		Icon:           "󰋗",
		ArgsHint:       "[target]",
		Tier:           TierTertiary,
		Type:           CmdPrompt,
		PromptTemplate: "Explain the following concisely (tldr). Cover what it does, key edge cases, and risks. Target: $ARGUMENTS",
	}
}

func handleTldr(host Host, args []string) Result {
	prompt := "Explain the current code/selection concisely (tldr). Focus on what it does, key edge cases, and any risks."
	if len(args) > 0 {
		prompt += "\nTarget: " + strings.Join(args, " ")
	}
	return Done(host.StartAgent(prompt))
}
