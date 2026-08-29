package commands

import (
	"strings"
)

// /commit — commit pending changes via the agent.
// The sub-palette picks the commit scope, which becomes $ARGUMENTS.

func commitCommand() Command {
	return Command{
		Name:             "commit",
		Description:      "Commit pending changes via the agent",
		Category:         "Workflow",
		Icon:             "󰊢",
		ArgsHint:         "[focus]",
		Tier:             TierPrimary,
		Type:             CmdPrompt,
		SubPalette:       "commit",
		SupportsHeadless: true,
		WhenToUse:        "To turn current workspace changes into a proper git commit",
		PromptTemplate:   "Create a git commit for the pending workspace changes.\n1. Run `git status` and `git diff` (staged and unstaged) to understand the change.\n2. Draft a concise message in the repository's existing style (check `git log`).\n3. Stage only files relevant to this change and commit. Never push.\n4. If the diff is empty or unrelated files are mixed in, ask before proceeding.\nFocus: $ARGUMENTS",
	}
}

func handleCommit(host Host, args []string) Result {
	var b strings.Builder
	b.WriteString("Create a git commit for the pending workspace changes.\n")
	b.WriteString("1. Run `git status` and `git diff` (staged and unstaged) to understand the change.\n")
	b.WriteString("2. Draft a concise message in the repository's existing style (check `git log`).\n")
	b.WriteString("3. Stage only files relevant to this change and commit. Never push.\n")
	b.WriteString("4. If the diff is empty or unrelated files are mixed in, ask before proceeding.\n")
	if scope := strings.TrimSpace(strings.Join(args, " ")); scope != "" {
		b.WriteString("\nFocus: " + scope)
	}
	host.SetStatus("Preparing commit")
	return Done(host.StartAgent(b.String()))
}
