package commands

import (
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/iSundram/Automergent/internal/tui/components"
)

// /stats — show session statistics.
// /help — open keyboard and command help.
// /quit (alias /exit) — exit Automergent.
// /version — show the Automergent version.

func statsCommand() Command {
	return Command{
		Name:          "stats",
		Description:   "Show session statistics",
		Category:      "System",
		Icon:          "󰄪",
		Tier:          TierTertiary,
		Type:          CmdFullPage,
		FullPageTitle: "Statistics",
		Page:          statsPage,
		Immediate:     true,
	}
}

func helpCommand() Command {
	return Command{
		Name:          "help",
		Description:   "Open keyboard and command help",
		Category:      "System",
		Icon:          "󰋖",
		Tier:          TierSecondary,
		Type:          CmdFullPage,
		FullPageTitle: "Help",
		Immediate:     true,
	}
}

func quitCommand() Command {
	return Command{
		Name:        "quit",
		Aliases:     []string{"exit"},
		Description: "Exit Automergent",
		Category:    "System",
		Icon:        "󰗼",
		Tier:        TierSecondary,
		Immediate:   true,
	}
}

func handleStats(host Host, args []string) Result {
	host.ShowStats()
	return Done(nil)
}

// statsPage builds the structured usage page: current session tokens/cost plus
// all-time totals across stored sessions.
func statsPage(h Host) components.Page {
	page := components.Page{
		Title:    "Statistics",
		Subtitle: "Token usage and cost",
		Sections: []components.PageSection{
			{
				Heading: "This Session",
				Rows: [][2]string{
					components.Row("Input tokens", fmt.Sprintf("%d", h.InputTokens())),
					components.Row("Output tokens", fmt.Sprintf("%d", h.OutputTokens())),
					components.Row("Cost", fmt.Sprintf("$%.4f", h.TotalCost())),
				},
			},
		},
	}
	sessions, totalIn, totalOut := h.SessionTokenTotals()
	if sessions > 0 {
		page.Sections = append(page.Sections, components.PageSection{
			Heading: "All Sessions",
			Rows: [][2]string{
				components.Row("Sessions", fmt.Sprintf("%d", sessions)),
				components.Row("Input tokens", fmt.Sprintf("%d", totalIn)),
				components.Row("Output tokens", fmt.Sprintf("%d", totalOut)),
			},
		})
	}
	page.Actions = []components.PageAction{
		{Key: "c", Label: "Context", Command: "context"},
		{Key: "e", Label: "Errors", Command: "error"},
	}
	return page
}

func handleHelp(host Host, args []string) Result {
	host.ShowHelp()
	return Done(nil)
}

func handleQuit(host Host, args []string) Result {
	return Done(tea.Quit)
}
