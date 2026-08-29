package agent

import (
	"sort"
	"strings"
)

// CommandHint tells the agent about a user-invocable slash command so it can
// recommend the command when a task matches. Mirrors the skills-availability
// pattern: commands are recommendable, never executed by the model.
type CommandHint struct {
	Name        string
	Description string
	WhenToUse   string
}

// setCommandHints replaces the command-hint set (called by the TUI after the
// registry is built and after custom commands reload).
func (a *Agent) SetCommandHints(hints []CommandHint) {
	a.commandHintsMu.Lock()
	a.commandHints = hints
	a.commandHintsMu.Unlock()
}

// commandHintSnapshot returns a copy of the current hints (nil-safe).
func (a *Agent) commandHintSnapshot() []CommandHint {
	a.commandHintsMu.RLock()
	defer a.commandHintsMu.RUnlock()
	if a.commandHints == nil {
		return nil
	}
	out := make([]CommandHint, len(a.commandHints))
	copy(out, a.commandHints)
	return out
}

// commandHintsPromptBlock renders the "## Slash Commands" system-prompt block.
// Hints are rendered in a stable (sorted) order so the block's text — and
// therefore the provider's prompt cache prefix — stays stable across turns.
func commandHintsPromptBlock(hints []CommandHint) string {
	if len(hints) == 0 {
		return ""
	}
	sorted := make([]CommandHint, len(hints))
	copy(sorted, hints)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })

	var sb strings.Builder
	sb.WriteString("## Slash Commands\n")
	sb.WriteString("The user can run these slash commands. When a task clearly matches one, mention it (e.g. \"/commit can do this\") — you cannot run them yourself.\n")
	for _, h := range sorted {
		line := "- **/" + h.Name + "**"
		if h.Description != "" {
			line += ": " + h.Description
		}
		if h.WhenToUse != "" {
			line += " (" + h.WhenToUse + ")"
		}
		sb.WriteString(line + "\n")
	}
	sb.WriteString("\n")
	return sb.String()
}
