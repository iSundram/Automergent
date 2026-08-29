package commands

import (
	"fmt"
	"strings"

	"github.com/iSundram/Automergent/internal/tui/components"
)

// /skills — browse available skills (custom markdown commands plus built-in
// prompt commands, both derived from the live registry).
// /agents — browse available agents.

// skillsCommand closes over the registry so its inventory is the live command
// set, not a snapshot.
func skillsCommand(r *Registry) Command {
	return Command{
		Name:          "skills",
		Description:   "Browse available skills",
		Category:      "Knowledge",
		Icon:          "󰚩",
		Tier:          TierSecondary,
		Type:          CmdFullPage,
		FullPageTitle: "Skills",
		Page:          func(h Host) components.Page { return skillsPage(r, h) },
		Immediate:     true,
	}
}

// handleSkills lists the skill-like inventory from the registry: custom
// markdown commands (runnable skills) and built-in prompt commands (agent
// workflows). The registry is the single source of truth — nothing here is
// hardcoded.
func handleSkills(r *Registry) Handler {
	return func(host Host, args []string) Result {
		host.AddSystemMessage(strings.Join(skillsPage(r, host).Lines(80), "\n"))
		host.SetStatus("Skills listed")
		return Done(nil)
	}
}

// skillsPage builds the structured /skills page from the registry.
func skillsPage(r *Registry, h Host) components.Page {
	page := components.Page{
		Title:    "Skills",
		Subtitle: "Runnable skills and agent workflows",
	}

	var custom, prompts []Command
	for _, cmd := range r.List() {
		if cmd.Category == customCategory {
			custom = append(custom, cmd)
		} else if cmd.Type == CmdPrompt && !cmd.Hidden {
			prompts = append(prompts, cmd)
		}
	}

	if len(custom) == 0 && len(prompts) == 0 {
		page.Sections = append(page.Sections, components.PageSection{
			Lines: []string{"No skills available."},
		})
		return page
	}

	if len(custom) > 0 {
		rows := make([][2]string, 0, len(custom))
		for _, cmd := range custom {
			source := cmd.Source
			if source == "" {
				source = "(unknown source)"
			}
			rows = append(rows, components.Row("/"+cmd.Name, source))
		}
		page.Sections = append(page.Sections, components.PageSection{
			Heading: fmt.Sprintf("Custom skills (%d)", len(custom)),
			Rows:    rows,
			Lines:   []string{"Loaded from .automergent/commands/ and ~/.automergent/commands/ — run /commands reload after editing."},
		})
	} else {
		page.Sections = append(page.Sections, components.PageSection{
			Heading: "Custom skills",
			Lines: []string{
				"None loaded.",
				"Drop *.md files in .automergent/commands/ or ~/.automergent/commands/, then run /commands reload.",
			},
		})
	}

	if len(prompts) > 0 {
		rows := make([][2]string, 0, len(prompts))
		for _, cmd := range prompts {
			desc := cmd.Description
			if desc == "" {
				desc = cmd.WhenToUse
			}
			rows = append(rows, components.Row("/"+cmd.Name, desc))
		}
		page.Sections = append(page.Sections, components.PageSection{
			Heading: "Built-in workflows",
			Rows:    rows,
		})
	}

	page.Actions = []components.PageAction{
		{Key: "c", Label: "Custom commands", Command: "commands"},
		{Key: "r", Label: "Reload", Command: "commands", Args: []string{"reload"}},
	}
	return page
}

func agentsCommand() Command {
	return Command{
		Name:          "agents",
		Description:   "Browse live subagents and agent types",
		Category:      "Knowledge",
		Icon:          "󰧑",
		ArgsHint:      "[open <id>]",
		Tier:          TierSecondary,
		Type:          CmdFullPage,
		FullPageTitle: "Agents",
		Page:          agentsPage,
		Immediate:     true,
	}
}

// handleAgents covers the argument paths: "/agents open <id>" jumps straight
// into an agent's live view; with no args the Page builder in
// cmd_agents_view.go renders the roster and the handler is not reached.
func handleAgents(host Host, args []string) Result {
	if len(args) >= 2 && args[0] == "open" {
		host.OpenAgentView(args[1])
		return Done(nil)
	}
	host.AddSystemMessage("Usage: /agents [open <id>] — run /agents for the roster.")
	return Done(nil)
}
