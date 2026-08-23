package commands

import (
	"fmt"
	"strings"
)

// --- New Commands Handlers ---

func handleTheme(host Host, args []string) Result {
	current := host.CurrentTheme()
	available := host.AvailableThemes()

	if len(args) == 0 {
		var b strings.Builder
		b.WriteString(fmt.Sprintf("Current theme: %s\n\nAvailable themes:\n", current))
		for _, t := range available {
			marker := "  "
			if t == current {
				marker = "→ "
			}
			b.WriteString(fmt.Sprintf("%s%s\n", marker, t))
		}
		b.WriteString("\nUsage: /theme <name>")
		host.AddSystemMessage(b.String())
		return Done(nil)
	}

	name := strings.ToLower(args[0])
	found := false
	for _, t := range available {
		if t == name {
			found = true
			break
		}
	}
	if !found {
		host.CommandError(fmt.Sprintf("Unknown theme %q. Available: %s", name, strings.Join(available, ", ")))
		return Done(nil)
	}

	if err := host.SetTheme(name); err != nil {
		host.CommandError(err.Error())
		return Done(nil)
	}

	host.PersistProjectConfig()
	host.AddSystemMessage(fmt.Sprintf("Theme switched to %s", name))
	host.SetStatus("Theme updated")
	return Done(nil)
}

func handleKeybindings(host Host, args []string) Result {
	current := host.CurrentKeybindings()
	available := host.AvailableKeybindings()

	if len(args) == 0 {
		var b strings.Builder
		b.WriteString(fmt.Sprintf("Current keybindings: %s\n\nAvailable schemes:\n", current))
		for _, k := range available {
			marker := "  "
			if k == current {
				marker = "→ "
			}
			b.WriteString(fmt.Sprintf("%s%s\n", marker, k))
		}
		b.WriteString("\nUsage: /keybindings <default|vim|emacs>")
		host.AddSystemMessage(b.String())
		return Done(nil)
	}

	scheme := strings.ToLower(args[0])
	found := false
	for _, k := range available {
		if k == scheme {
			found = true
			break
		}
	}
	if !found {
		host.CommandError(fmt.Sprintf("Unknown keybinding scheme %q. Available: %s", scheme, strings.Join(available, ", ")))
		return Done(nil)
	}

	if err := host.SetKeybindings(scheme); err != nil {
		host.CommandError(err.Error())
		return Done(nil)
	}

	host.PersistProjectConfig()
	host.AddSystemMessage(fmt.Sprintf("Keybindings switched to %s", scheme))
	host.SetStatus("Keybindings updated")
	return Done(nil)
}

func handleCompact(host Host, args []string) Result {
	host.SetStatus("Compacting context...")
	return Done(host.CompactContext())
}
