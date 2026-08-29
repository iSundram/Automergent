package commands

import (
	"fmt"
	"strings"
)

// /theme — switch or list UI themes.
// /keybindings (alias /keys) — switch or list keybinding schemes.

func themeCommand() Command {
	return Command{
		Name:             "theme",
		Description:      "Switch or list UI themes",
		Category:         "Configuration",
		Icon:             "󰏘",
		ArgsHint:         "[name]",
		Tier:             TierSecondary,
		SubPalette:       "theme",
		SupportsHeadless: true,
	}
}

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
	if !containsString(available, name) {
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

func keybindingsCommand() Command {
	return Command{
		Name:             "keybindings",
		Aliases:          []string{"keys"},
		Description:      "Switch or list keybinding schemes",
		Category:         "Configuration",
		Icon:             "󰌌",
		ArgsHint:         "[default|vim|emacs]",
		Tier:             TierSecondary,
		SubPalette:       "keybindings",
		SupportsHeadless: true,
	}
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
	if !containsString(available, scheme) {
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
