package commands

import (
	"fmt"
	"strings"
)

// /recap — summarize the session so far (local snapshot; no LLM call).

func recapCommand() Command {
	return Command{
		Name:             "recap",
		Description:      "Summarize the session so far",
		Category:         "Session",
		Icon:             "󰭗",
		Tier:             TierSecondary,
		Type:             CmdPrompt,
		Immediate:        true,
		SupportsHeadless: true,
		WhenToUse:        "To catch up on what happened in this session",
		PromptTemplate:   "Recap what has happened in this session so far. List the key points, decisions, and current state.",
	}
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
