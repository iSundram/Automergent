package commands

import (
	"fmt"
	"strings"

	"github.com/iSundram/Automergent/internal/agent"
)

// /mode — change the agent mode (manual, accept-edits, auto, plan).

func modeCommand() Command {
	return Command{
		Name:          "mode",
		Description:   "Change agent mode",
		Category:      "AI & Model",
		Icon:          "󰒓",
		ArgsHint:      "<edit|plan>",
		Tier:          TierSecondary,
		SubPalette:    "mode",
		SubCommands: []SubCommand{
			{Name: "edit", Description: "Edit mode (manual approval)", Handler: handleMode},
			{Name: "plan", Description: "Plan mode (read-only)", Handler: handleMode},
		},
		Completion: func(h Host, partial string) []string {
			return prefixFilter([]string{"edit", "plan"}, partial)
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
	host.AddSystemMessage(fmt.Sprintf("Mode switched to %s — %s", mode, agent.ModeDescription(mode)))
	host.PersistProjectConfig()
	host.SetStatus("Mode updated")
	return Done(nil)
}
