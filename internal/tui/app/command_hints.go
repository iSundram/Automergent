package app

// Slash-command hints wiring: the registry drives the agent's system-prompt
// "## Slash Commands" block, so the model can recommend commands (never run
// them) when a task matches.

import (
	"github.com/iSundram/Automergent/internal/agent"
	"github.com/iSundram/Automergent/internal/tui/commands"
)

// syncCommandHints pushes the registry's recommendable commands into the
// agent's system prompt. Called at startup and after custom commands reload.
func (a *App) syncCommandHints() {
	if a.ag == nil || a.commands == nil {
		return
	}
	hints := commandHints(a.commands)
	a.ag.SetCommandHints(hints)
}

// commandHints selects the commands worth recommending to the model:
// prompt-type commands (they map to a task shape), custom markdown commands,
// and any command with explicit when-to-use guidance.
func commandHints(reg *commands.Registry) []agent.CommandHint {
	var hints []agent.CommandHint
	for _, cmd := range reg.List() {
		if cmd.Hidden {
			continue
		}
		if cmd.Type == commands.CmdPrompt || cmd.WhenToUse != "" || cmd.Category == commands.CustomCategory {
			hints = append(hints, agent.CommandHint{
				Name:        cmd.Name,
				Description: cmd.Description,
				WhenToUse:   cmd.WhenToUse,
			})
		}
	}
	return hints
}
