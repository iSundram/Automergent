package commands

import (
	"fmt"
	"strings"

	"github.com/iSundram/Automergent/internal/agent"
)

// /mode — change the agent mode (manual, accept-edits, auto, plan).
// "edit" stays accepted as a legacy input alias for "manual" (agent.IsValid
// and CanonicalMode handle it) but is never offered as a choice.

func modeCommand() Command {
	modes := agent.AllModes()
	subs := make([]SubCommand, 0, len(modes))
	for _, m := range modes {
		subs = append(subs, SubCommand{Name: m, Description: agent.ModeDescription(m), Handler: handleMode})
	}
	return Command{
		Name:        "mode",
		Description: "Change agent mode",
		Category:    "AI & Model",
		Icon:        "󰒓",
		ArgsHint:    "<manual|accept-edits|auto|plan>",
		Tier:        TierSecondary,
		SubPalette:  "mode",
		SubCommands: subs,
		Completion: func(h Host, partial string) []string {
			return prefixFilter(modes, partial)
		},
		SupportsHeadless: true,
	}
}

func handleMode(host Host, args []string) Result {
	if len(args) == 0 {
		var b strings.Builder
		fmt.Fprintf(&b, "Current mode: %s — %s\n\nAvailable modes:\n",
			host.Mode(), agent.ModeDescription(host.Mode()))
		for _, mode := range agent.AllModes() {
			marker := "  "
			if agent.CanonicalMode(host.Mode()) == mode {
				marker = "▸ "
			}
			fmt.Fprintf(&b, "%s%-13s %s\n", marker, mode, agent.ModeDescription(mode))
		}
		b.WriteString("\nUsage: /mode <manual|accept-edits|auto|plan> · shift+tab cycles")
		host.AddSystemMessage(b.String())
		return Done(nil)
	}

	mode := args[0]
	if !agent.IsValid(mode) {
		host.CommandError("Error: usage /mode <manual|accept-edits|auto|plan>")
		return Done(nil)
	}

	mode = agent.CanonicalMode(mode)
	host.SetMode(mode)
	msg := fmt.Sprintf("Mode switched to %s — %s", mode, agent.ModeDescription(mode))
	// A mode that fails to persist silently reverts on restart — say so,
	// matching the shift+tab cycle's "(not saved: ...)" notice.
	if err := host.PersistProjectConfig(); err != nil {
		msg += " (not saved: " + err.Error() + ")"
	}
	host.AddSystemMessage(msg)
	host.SetStatus("Mode updated")
	return Done(nil)
}
