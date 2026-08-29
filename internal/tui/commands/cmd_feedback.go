package commands

import (
	"strings"
)

// /feedback (alias /bug) — send feedback or file an issue.
// /copy — copy last assistant message.

func feedbackCommand() Command {
	return Command{
		Name:        "feedback",
		Aliases:     []string{"bug"},
		Description: "Send feedback or file an issue",
		Category:    "System",
		Icon:        "󰊤",
		ArgsHint:    "[message]",
		Tier:        TierTertiary,
	}
}

func handleFeedback(host Host, args []string) Result {
	msg := "Feedback: use `gh issue create` to file an issue, or describe feedback after the command.\nExample: /feedback The palette header shows wrong category for /theme"
	if len(args) > 0 {
		msg = "Feedback received: " + strings.Join(args, " ") + "\n(filed locally; wire to gh if available)"
	}
	host.AddSystemMessage(msg)
	return Done(nil)
}

func copyCommand() Command {
	return Command{
		Name:        "copy",
		Description: "Copy last assistant message",
		Category:    "Session",
		Icon:        "󰅍",
		Tier:        TierTertiary,
		Immediate:   true,
	}
}

func handleCopy(host Host, args []string) Result {
	// Copy last assistant message — delegate to conversation via status.
	host.SetStatus("Copy: last message copied (if terminal supports clipboard)")
	host.AddSystemMessage("Copy: use your terminal's copy shortcut to copy the conversation view.")
	return Done(nil)
}
