package commands

import (
	"fmt"
	"strings"

	"github.com/iSundram/Automergent/internal/tui/components"
)

// /commands — manage custom slash commands.
// The command closes over the registry so list/reload operate on the live
// command set rather than a snapshot.

func commandsCommand(r *Registry) Command {
	handler := commandsHandler(r)
	return Command{
		Name:          "commands",
		Description:   "Manage custom slash commands",
		Category:      "System",
		Icon:          "󰘳",
		ArgsHint:      "[list|reload]",
		Tier:          TierSecondary,
		Type:          CmdFullPage,
		FullPageTitle: "Custom Commands",
		SubCommands: []SubCommand{
			{Name: "list", Description: "List all custom commands", Handler: handler},
			{Name: "reload", Description: "Reload custom commands from disk", Handler: handler},
		},
		Completion: func(h Host, partial string) []string {
			return prefixFilter([]string{"list", "reload"}, partial)
		},
		Page: func(h Host) components.Page { return commandsPage(r, h) },
	}
}

// commandsHandler implements both /commands list (default) and /commands
// reload. A single handler keeps direct Registry.Dispatch calls (which bypass
// the app's sub-command resolution) working for both spellings.
func commandsHandler(r *Registry) Handler {
	return func(host Host, args []string) Result {
		if len(args) > 0 && args[0] == "reload" {
			removed := r.RemoveCustom()
			loaded, warnings := LoadProjectAndUserCommands(r, host.WorkDir())
			if removed > 0 || loaded > 0 {
				host.AddSystemMessage(fmt.Sprintf("Reloaded custom commands: %d removed, %d loaded.", removed, loaded))
			} else {
				host.AddSystemMessage("Custom commands reloaded — no changes found.")
			}
			for _, w := range warnings {
				host.CommandError("Reload warning: " + w)
			}
			host.SetStatus(fmt.Sprintf("%d custom commands loaded", loaded))
			return Done(nil)
		}

		customs := customCommands(r)
		if len(customs) == 0 {
			host.AddSystemMessage("No custom commands loaded.\nDrop *.md files in .automergent/commands/ or ~/.automergent/commands/, then run /commands reload.")
			return Done(nil)
		}
		host.AddSystemMessage(strings.Join(commandsPage(r, host).Lines(80), "\n"))
		host.SetStatus(fmt.Sprintf("%d custom commands", len(customs)))
		return Done(nil)
	}
}

// customCommands returns the registry's custom-category commands.
func customCommands(r *Registry) []Command {
	var out []Command
	for _, cmd := range r.List() {
		if cmd.Category == customCategory {
			out = append(out, cmd)
		}
	}
	return out
}

func commandTypeName(t CommandType) string {
	switch t {
	case CmdPrompt:
		return "prompt"
	case CmdFullPage:
		return "full-page"
	default:
		return "handler"
	}
}

// commandsPage builds the structured /commands page.
func commandsPage(r *Registry, h Host) components.Page {
	customs := customCommands(r)
	sections := []components.PageSection{}
	if len(customs) == 0 {
		sections = append(sections, components.PageSection{
			Heading: "Custom commands",
			Lines: []string{
				"None loaded.",
				"Drop *.md files in .automergent/commands/ or ~/.automergent/commands/",
				"and run /commands reload.",
			},
		})
	} else {
		rows := make([][2]string, 0, len(customs))
		for _, cmd := range customs {
			source := cmd.Source
			if source == "" {
				source = "(unknown source)"
			}
			rows = append(rows, components.Row("/"+cmd.Name+" · "+commandTypeName(cmd.Type), source))
		}
		sections = append(sections, components.PageSection{
			Heading: fmt.Sprintf("Custom commands (%d)", len(customs)),
			Rows:    rows,
		})
		sections = append(sections, components.PageSection{
			Heading: "Search roots",
			Lines: []string{
				".automergent/commands/   (project, walked up to the repo root)",
				"~/.automergent/commands/ (user)",
			},
		})
	}
	return components.Page{
		Title:    "Custom Commands",
		Subtitle: "Markdown commands loaded from disk",
		Sections: sections,
		Actions: []components.PageAction{
			{Key: "r", Label: "Reload", Command: "commands", Args: []string{"reload"}},
			{Key: "h", Label: "Help", Command: "help"},
		},
	}
}
