package commands

import (
	"fmt"
)

// /sessions — browse previous sessions.
// /resume — browse and resume a session.

func sessionsCommand() Command {
	return Command{
		Name:          "sessions",
		Aliases:       []string{"session"},
		Description:   "Browse previous sessions",
		Category:      "Session",
		Icon:          "󰆓",
		Tier:          TierPrimary,
		Type:          CmdFullPage,
		FullPageTitle: "Sessions",
		Immediate:     true,
	}
}

func handleSessions(host Host, args []string) Result {
	host.ShowSessions()
	return Done(nil)
}

func resumeCommand() Command {
	return Command{
		Name:        "resume",
		Description: "Browse and resume a session",
		Category:    "Session",
		Icon:        "󰑐",
		ArgsHint:    "[id|prefix|title]",
		Tier:        TierSecondary,
		Immediate:   true,
	}
}

func handleResume(host Host, args []string) Result {
	if host.Thinking() {
		host.CommandError("Agent is running — /cancel it before resuming another session")
		return Done(nil)
	}
	if len(args) == 0 {
		host.ShowSessions()
		return Done(nil)
	}
	if err := host.ResumeSession(args[0]); err != nil {
		host.CommandError(fmt.Sprintf("Unable to resume session: %v", err))
	}
	return Done(nil)
}
