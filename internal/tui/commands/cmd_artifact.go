package commands

// /artifact — review artifacts the agent produced (plans, reviews, docs).

func artifactCommand() Command {
	return Command{
		Name:        "artifact",
		Aliases:     []string{"artifacts"},
		Description: "Review agent artifacts (plans, reviews, docs)",
		Category:    "Workflow",
		Icon:        "󰗧",
		ArgsHint:    "",
		Tier:        TierPrimary,
		Immediate:   true,
		WhenToUse:   "When the agent has written a plan, review or document that needs approval",
	}
}

func handleArtifact(host Host, args []string) Result {
	host.ShowArtifacts()
	return Done(nil)
}
