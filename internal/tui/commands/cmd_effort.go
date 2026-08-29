package commands

import (
	"fmt"
	"strings"
)

// /effort — set or show provider thinking effort.

func effortCommand() Command {
	return Command{
		Name:             "effort",
		Description:      "Set or show provider thinking effort",
		Category:         "Configuration",
		Icon:             "󰓅",
		ArgsHint:         "[minimal|low|medium|high]",
		Tier:             TierSecondary,
		SubPalette:       "effort",
		SupportsHeadless: true,
	}
}

func handleEffort(host Host, args []string) Result {
	provider := host.Provider()
	host.EnsureProviderConfig(provider)
	pc := host.ProviderConfig(provider)

	if len(args) == 0 {
		current := pc.Effort
		if current == "" {
			current = pc.ThinkingLevel
		}
		if current == "" {
			current = "high (default)"
		}
		host.AddSystemMessage(fmt.Sprintf("Current effort for %s: %s\nUsage: /effort <minimal|low|medium|high>", provider, current))
		return Done(nil)
	}

	effort := strings.ToLower(args[0])
	switch effort {
	case "minimal", "low", "medium", "high":
		// Set both legacy Effort and new ThinkingLevel for compatibility
		pc.Effort = effort
		pc.ThinkingLevel = effort
		host.SetProviderConfig(provider, pc)

		if err := host.SwitchProvider(provider, host.Model()); err != nil {
			host.CommandError(fmt.Sprintf("Error: %v", err))
			return Done(nil)
		}

		host.PersistProjectConfig()
		host.AddSystemMessage(fmt.Sprintf("Effort for %s set to %s", provider, effort))
		host.SetStatus("Effort updated")
	default:
		host.CommandError("Error: usage /effort <minimal|low|medium|high>")
	}
	return Done(nil)
}
