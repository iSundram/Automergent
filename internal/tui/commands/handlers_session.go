package commands

import (
	"fmt"
	"strings"
)

// --- Session Handlers ---

func handleNew(host Host, args []string) Result {
	host.NewSession()
	return Done(nil)
}

func handleSessions(host Host, args []string) Result {
	host.ShowSessions()
	return Done(nil)
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

func handleClear(host Host, args []string) Result {
	host.ClearConversationView()
	return Done(nil)
}

func handleReset(host Host, args []string) Result {
	host.ResetSessionHistory()
	return Done(nil)
}

func handleExport(host Host, args []string) Result {
	path := ""
	if len(args) > 0 {
		path = args[0]
	}
	if err := host.ExportConversation(path); err != nil {
		host.CommandError(err.Error())
		return Done(nil)
	}
	if path == "" {
		path = "conversation.md"
	}
	host.AddSystemMessage("Conversation exported to " + path)
	host.SetStatus("Conversation exported")
	return Done(nil)
}

func handleApprovals(host Host, args []string) Result {
	host.HandleApprovalsCommand(args)
	return Done(nil)
}

func handleRename(host Host, args []string) Result {
	title := strings.TrimSpace(strings.Join(args, " "))
	if title == "" {
		host.CommandUsage("/rename <title>")
		return Done(nil)
	}
	if err := host.RenameSession(title); err != nil {
		host.CommandError(err.Error())
		return Done(nil)
	}
	host.AddSystemMessage(fmt.Sprintf("Session renamed to %q", title))
	host.SetStatus("Session renamed")
	return Done(nil)
}

func handleRecap(host Host, args []string) Result {
	snap := host.RecapSnapshot()
	if snap.UserTurns == 0 {
		host.AddSystemMessage("Nothing to recap yet — start a conversation first.")
		return Done(nil)
	}

	var b strings.Builder
	b.WriteString("Session recap:\n")
	if !snap.StartedAt.IsZero() {
		fmt.Fprintf(&b, "Started %s · ", snap.StartedAt.Format("15:04"))
	}
	fmt.Fprintf(&b, "%d user turns · %d replies · %d tool calls\n", snap.UserTurns, snap.AssistantTurns, snap.ToolCalls)
	if len(snap.ToolsUsed) > 0 {
		b.WriteString("Tools used: " + strings.Join(snap.ToolsUsed, ", ") + "\n")
	}
	if last := strings.TrimSpace(snap.LastUserMessage); last != "" {
		if len(last) > 160 {
			last = last[:157] + "..."
		}
		b.WriteString("Last request: \"" + last + "\"")
	}

	host.AddSystemMessage(strings.TrimRight(b.String(), "\n"))
	host.SetStatus("Recap generated")
	return Done(nil)
}
