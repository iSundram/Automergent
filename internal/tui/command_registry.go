package tui

import (
	"strings"

	"github.com/iSundram/Automergent/internal/tui/components"
)

type slashCommand struct {
	Name        string
	Aliases     []string
	Description string
	Category    string
	Icon        string
	Usage       string
	Immediate   bool
}

var slashCommands = []slashCommand{
	{Name: "model", Description: "Switch AI model", Category: "AI & Model", Icon: "󰊕", Usage: "<name>"},
	{Name: "provider", Description: "Switch AI provider", Category: "AI & Model", Icon: "󰒋", Usage: "<name> [model]"},
	{Name: "mode", Description: "Change agent mode", Category: "AI & Model", Icon: "󰒓", Usage: "<edit|plan>"},
	{Name: "context", Aliases: []string{"tokens"}, Description: "Show model and context usage", Category: "AI & Model", Icon: "󰚩", Immediate: true},

	{Name: "new", Description: "Start a fresh session", Category: "Session", Icon: "󰐕", Immediate: true},
	{Name: "sessions", Description: "Browse previous sessions", Category: "Session", Icon: "󰆓", Immediate: true},
	{Name: "resume", Description: "Browse and resume a session", Category: "Session", Icon: "󰑐", Immediate: true},
	{Name: "export", Description: "Export conversation as Markdown", Category: "Session", Icon: "󰈇", Usage: "[path]", Immediate: true},
	{Name: "approvals", Description: "View or revoke always-allow tool approvals", Category: "Session", Icon: "󰌑", Usage: "[revoke <index>]", Immediate: true},
	{Name: "clear", Description: "Clear the conversation view", Category: "Session", Icon: "󰃢", Immediate: true},
	{Name: "reset", Description: "Reset current session history", Category: "Session", Icon: "󰑓", Immediate: true},

	{Name: "tree", Aliases: []string{"files"}, Description: "Toggle project file tree", Category: "Project", Icon: "󰙅", Immediate: true},
	{Name: "diff", Aliases: []string{"changes"}, Description: "Review workspace changes", Category: "Project", Icon: "󰈙", Immediate: true},
	{Name: "lsp", Aliases: []string{"diagnostics"}, Description: "Toggle project diagnostics", Category: "Project", Icon: "󰒓", Immediate: true},
	{Name: "search", Description: "Search workspace content", Category: "Project", Icon: "󰍉", Usage: "<query>"},

	{Name: "run", Description: "Run a project command", Category: "Workflow", Icon: "󰆍", Usage: "<command>"},
	{Name: "test", Description: "Detect and run project tests", Category: "Workflow", Icon: "󰙨", Usage: "[target]", Immediate: true},
	{Name: "build", Description: "Detect and build the project", Category: "Workflow", Icon: "󰒋", Usage: "[target]", Immediate: true},
	{Name: "review", Description: "Toggle detailed change review", Category: "Workflow", Icon: "󰄬", Immediate: true},
	{Name: "cancel", Aliases: []string{"stop"}, Description: "Cancel the active request", Category: "Workflow", Icon: "󰅙", Immediate: true},

	{Name: "api-key", Description: "Set active provider API key", Category: "Configuration", Icon: "󰌆", Usage: "<value>"},
	{Name: "base-url", Description: "Set active provider base URL", Category: "Configuration", Icon: "󰖟", Usage: "<url>"},
	{Name: "provider-api-key", Description: "Set an AI provider API key", Category: "Configuration", Icon: "󰌋", Usage: "<provider> <value>"},
	{Name: "provider-base-url", Description: "Set an AI provider base URL", Category: "Configuration", Icon: "󰌷", Usage: "<provider> <url>"},

	{Name: "stats", Description: "Show session statistics", Category: "System", Icon: "󰄪", Immediate: true},
	{Name: "help", Description: "Open keyboard and command help", Category: "System", Icon: "󰋖", Immediate: true},
	{Name: "quit", Aliases: []string{"exit"}, Description: "Exit Automergent", Category: "System", Icon: "󰗼", Immediate: true},
}

func commandPaletteItems() []components.PaletteItem {
	items := make([]components.PaletteItem, 0, len(slashCommands))
	for _, command := range slashCommands {
		hint := ""
		if command.Usage != "" {
			hint = command.Usage
		}
		items = append(items, components.PaletteItem{
			Label: command.Name, Value: command.Name, Description: command.Description,
			Icon: command.Icon, Category: command.Category, Hint: hint,
			SearchTerms: strings.Join(append(append([]string{}, command.Aliases...), command.Description, command.Category), " "),
		})
	}
	return items
}

func lookupSlashCommand(name string) (slashCommand, bool) {
	for _, command := range slashCommands {
		if name == command.Name {
			return command, true
		}
		for _, alias := range command.Aliases {
			if name == alias {
				return command, true
			}
		}
	}
	return slashCommand{}, false
}
