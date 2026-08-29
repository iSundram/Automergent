package commands

import (
	"fmt"
	"strings"

	"github.com/iSundram/Automergent/internal/tui/components"
)

// /context — show model and context usage.
// /cost — token and cost usage page.

func contextCommand() Command {
	return Command{
		Name:             "context",
		Aliases:          []string{"tokens"},
		Description:      "Show model and context usage",
		Category:         "AI & Model",
		Icon:             "󰚩",
		ArgsHint:         "[detail]",
		Tier:             TierTertiary,
		Type:             CmdFullPage,
		FullPageTitle:    "Context Usage",
		Immediate:        true,
		SupportsHeadless: true,
		Page:             contextPage,
	}
}

func costCommand() Command {
	return Command{
		Name:             "cost",
		Aliases:          []string{"usage"},
		Description:      "Show token and cost usage",
		Category:         "System",
		Icon:             "󰌧",
		Tier:             TierTertiary,
		Type:             CmdFullPage,
		FullPageTitle:    "Cost & Usage",
		Immediate:        true,
		SupportsHeadless: true,
		Page:             costPage,
	}
}

func handleContext(host Host, args []string) Result {
	if len(args) > 0 && args[0] == "detail" {
		host.ShowContextDetail()
		return Done(nil)
	}

	host.AddSystemMessage(fmt.Sprintf("Provider: %s\nModel: %s\nInput tokens: %d\nOutput tokens: %d\nTotal cost: $%.4f\nActive tokens: %d\n\nUse '/context detail' for telemetry breakdown.",
		host.Provider(), host.Model(), host.InputTokens(), host.OutputTokens(), host.TotalCost(), host.ActiveTokens()))
	return Done(nil)
}

func handleCost(host Host, args []string) Result {
	sessions, totalIn, totalOut := host.SessionTokenTotals()
	var b strings.Builder
	b.WriteString("Token usage:\n")
	fmt.Fprintf(&b, "Current session: in %d · out %d · $%.4f\n", host.InputTokens(), host.OutputTokens(), host.TotalCost())
	if sessions > 0 {
		fmt.Fprintf(&b, "All stored sessions (%d): in %d · out %d tokens\n", sessions, totalIn, totalOut)
	} else {
		b.WriteString("Stored-session totals unavailable (no storage attached).\n")
	}
	host.AddSystemMessage(strings.TrimRight(b.String(), "\n"))
	host.SetStatus("Usage listed")
	return Done(nil)
}

// contextPage builds the structured /context page.
func contextPage(h Host) components.Page {
	return components.Page{
		Title:    "Context Usage",
		Subtitle: h.Provider() + " · " + h.Model(),
		Sections: []components.PageSection{
			{
				Heading: "Session",
				Rows: [][2]string{
					components.Row("Input tokens", fmt.Sprintf("%d", h.InputTokens())),
					components.Row("Output tokens", fmt.Sprintf("%d", h.OutputTokens())),
					components.Row("Active tokens", fmt.Sprintf("%d", h.ActiveTokens())),
					components.Row("Total cost", fmt.Sprintf("$%.4f", h.TotalCost())),
				},
			},
		},
		Actions: []components.PageAction{
			{Key: "d", Label: "Detail", Command: "context", Args: []string{"detail"}},
			{Key: "c", Label: "Cost", Command: "cost"},
			{Key: "p", Label: "Compact", Command: "compact"},
		},
	}
}

// costPage builds the structured /cost page.
func costPage(h Host) components.Page {
	sessions, totalIn, totalOut := h.SessionTokenTotals()
	sections := []components.PageSection{
		{
			Heading: "Current session",
			Rows: [][2]string{
				components.Row("Input tokens", fmt.Sprintf("%d", h.InputTokens())),
				components.Row("Output tokens", fmt.Sprintf("%d", h.OutputTokens())),
				components.Row("Cost", fmt.Sprintf("$%.4f", h.TotalCost())),
			},
		},
	}
	if sessions > 0 {
		sections = append(sections, components.PageSection{
			Heading: "All stored sessions",
			Rows: [][2]string{
				components.Row("Sessions", fmt.Sprintf("%d", sessions)),
				components.Row("Total input", fmt.Sprintf("%d tokens", totalIn)),
				components.Row("Total output", fmt.Sprintf("%d tokens", totalOut)),
			},
		})
	} else {
		sections = append(sections, components.PageSection{
			Heading: "All stored sessions",
			Lines:   []string{"No storage attached — stored-session totals unavailable."},
		})
	}
	return components.Page{
		Title:    "Cost & Usage",
		Subtitle: h.Provider() + " · " + h.Model(),
		Sections: sections,
		Actions: []components.PageAction{
			{Key: "x", Label: "Context", Command: "context"},
			{Key: "p", Label: "Compact", Command: "compact"},
		},
	}
}
