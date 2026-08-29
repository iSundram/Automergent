package commands

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
		Name:             "resume",
		Description:      "Browse and resume a session",
		Category:         "Session",
		Icon:             "󰑐",
		ArgsHint:         "[id]",
		Tier:             TierSecondary,
		Immediate:        true,
		SupportsHeadless: true,
	}
}

func handleResume(host Host, args []string) Result {
	if len(args) == 0 {
		host.ShowSessions()
		return Done(nil)
	}
	if err := host.ResumeSession(args[0]); err != nil {
		host.SetStatus("Unable to resume session: " + err.Error())
	}
	return Done(nil)
}
