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
		Description:   "Browse available agents",
		Category:      "Knowledge",
		Icon:          "󰧑",
		Tier:          TierSecondary,
		Type:          CmdFullPage,
		FullPageTitle: "Agents",
		Immediate:     true,
	}
}

func handleAgents(host Host, args []string) Result {
	var b strings.Builder
	b.WriteString("Available agents:\n")
	b.WriteString("  general-purpose — full-capability agent for complex tasks\n")
	b.WriteString("  explore         — fast read-only codebase exploration\n")
	b.WriteString("  review          — code review, bug detection, security\n")
	b.WriteString("  contexter       — context compaction & memory\n")
	b.WriteString("  coordinator     — orchestrates other agents\n")
	host.AddSystemMessage(b.String())
	return Done(nil)
}
