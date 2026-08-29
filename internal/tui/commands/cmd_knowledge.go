package commands

import (
	"strings"
)

// /skills — browse available skills.
// /agents — browse available agents.

func skillsCommand() Command {
	return Command{
		Name:          "skills",
		Description:   "Browse available skills",
		Category:      "Knowledge",
		Icon:          "󰚩",
		Tier:          TierSecondary,
		Type:          CmdFullPage,
		FullPageTitle: "Skills",
		Immediate:     true,
	}
}

func handleSkills(host Host, args []string) Result {
	var b strings.Builder
	b.WriteString("Available skills:\n")
	// Skills are markdown commands in .automergent/commands — list via custom category.
	b.WriteString("(custom skills are loaded from .automergent/commands/ and ~/.automergent/commands/)\n")
	b.WriteString("Run /commands list to see custom skills, or see /help for built-ins with when-to-use hints.")
	host.AddSystemMessage(b.String())
	return Done(nil)
}

func agentsCommand() Command {
	return Command{
		Name:          "agents",
		Description:   "Browse available agents",
		Category:      "Knowledge",
		Icon:          "󰧑",
		Tier:          TierSecondary,
		Type:          CmdFullPage,
		FullPageTitle: "Agents",
		Immediate:     true,
	}
}

func handleAgents(host Host, args []string) Result {
	var b strings.Builder
	b.WriteString("Available agents:\n")
	b.WriteString("  general-purpose — full-capability agent for complex tasks\n")
	b.WriteString("  explore         — fast read-only codebase exploration\n")
	b.WriteString("  review          — code review, bug detection, security\n")
	b.WriteString("  contexter       — context compaction & memory\n")
	b.WriteString("  coordinator     — orchestrates other agents\n")
	host.AddSystemMessage(b.String())
	return Done(nil)
}
