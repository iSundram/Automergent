package commands

import (
	"fmt"
)

// /review-mode — toggle detailed change review.
// /cancel (alias /stop) — cancel the active request.

func reviewModeCommand() Command {
	return Command{
		Name:        "review-mode",
		Description: "Toggle detailed change review",
		Category:    "Workflow",
		Icon:        "󰄬",
		Tier:        TierSecondary,
		Immediate:   true,
		Current:     func(h Host) bool { return h.IsReviewMode() },
	}
}

func handleReviewMode(host Host, args []string) Result {
	host.ToggleReviewMode()
	host.SetStatus(fmt.Sprintf("Review mode %s", map[bool]string{true: "enabled", false: "disabled"}[host.IsReviewMode()]))
	return Done(nil)
}

func cancelCommand() Command {
	return Command{
		Name:             "cancel",
		Aliases:          []string{"stop"},
		Description:      "Cancel the active request",
		Category:         "Workflow",
		Icon:             "󰅙",
		Tier:             TierSecondary,
		Immediate:        true,
		SupportsHeadless: true,
		Enabled:          func(h Host) bool { return h.Thinking() },
		DisabledReason:   func(h Host) string { return "No active request" },
	}
}

func handleCancel(host Host, args []string) Result {
	if !host.Thinking() {
		host.SetStatus("No active request to cancel")
		return Done(nil)
	}
	host.CancelActiveRun("Cancelled by user")
	return Done(nil)
}
