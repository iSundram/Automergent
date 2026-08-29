package commands

import (
	"strings"
)

// /permissions — view or revoke tool permissions and write-path rules.

func permissionsCommand() Command {
	return Command{
		Name:             "permissions",
		Aliases:          []string{"approvals"},
		Description:      "View or revoke tool permissions and write-path rules",
		Category:         "Session",
		Icon:             "󰌑",
		ArgsHint:         "[revoke <index>]",
		Tier:             TierSecondary,
		Immediate:        true,
		SupportsHeadless: true,
	}
}

// handleApprovals forwards an approvals invocation to the host's approvals
// command implementation (the /permissions handler uses it for both the bare
// picker path and the scriptable arg path).
func handleApprovals(host Host, args []string) Result {
	host.HandleApprovalsCommand(args)
	return Done(nil)
}

func handlePermissions(host Host, args []string) Result {
	if len(args) == 0 {
		// Log the current grants so they are part of the transcript, then
		// open the interactive picker on top.
		host.HandleApprovalsCommand(nil)
		host.OpenPermissionsPicker()
		return Done(nil)
	}
	// Argument path keeps the scriptable list/revoke flow.
	handleApprovals(host, args)
	blocked, allowed := host.SecurityPaths()
	if len(blocked) == 0 && len(allowed) == 0 {
		return Done(nil)
	}
	var b strings.Builder
	b.WriteString("\nConfigured write-path rules:")
	for _, p := range blocked {
		b.WriteString("\n  ✗ blocked: " + p)
	}
	for _, p := range allowed {
		b.WriteString("\n  ✓ allowed: " + p)
	}
	host.AddSystemMessage(b.String())
	return Done(nil)
}
