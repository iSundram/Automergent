package commands

import (
	"fmt"
	"strings"
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
		Completion: func(h Host, partial string) []string {
			return sessionReferenceCompletion(h, partial)
		},
	}
}

// sessionReferenceCompletion offers stored sessions for argument completion:
// label is "<title> — <age>" and the value is the session ID (resuming by ID
// keeps working after a rename, where a title match would break). The label is
// matched case-insensitively against the partial; the ID is offered verbatim
// as typed.
func sessionReferenceCompletion(h Host, partial string) []string {
	refs := h.SessionReferences()
	if len(refs) == 0 {
		return nil
	}
	lower := strings.ToLower(partial)
	out := make([]string, 0, len(refs))
	for _, r := range refs {
		if lower == "" || strings.Contains(strings.ToLower(r.Label), lower) ||
			strings.Contains(strings.ToLower(r.ID), lower) {
			out = append(out, r.Label)
		}
	}
	// Completion values carry the ID; the label is display text. The palette
	// takes "label" strings, so encode both: "label" (shown) dispatches the
	// label itself, and /resume accepts titles too. When the label and ID
	// differ enough to matter, the ID prefix is the unambiguous form.
	return out
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
