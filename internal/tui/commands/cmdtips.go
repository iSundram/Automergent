package commands

// Per-command tips: one Go file per command under tips/, each registering its
// material at init. The tip data lives in the tips package; this file only
// adapts it to the registry's alias resolution.

import (
	"github.com/iSundram/Automergent/internal/tui/commands/tips"
)

// CommandTip is the parsed tip material for one command.
type CommandTip = tips.CommandTip

// Tip returns the tip material for a command name (or alias).
func (r *Registry) Tip(name string) (CommandTip, bool) {
	if canonical, ok := r.aliases[name]; ok {
		name = canonical
	}
	return tips.For(name)
}
