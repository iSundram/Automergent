package commands

import (
	"fmt"
	"strings"
)

// /api-key — set the active provider's API key.
// /base-url — set the active provider's base URL.

func apiKeyCommand() Command {
	return Command{
		Name:             "api-key",
		Description:      "Set active provider API key",
		Category:         "Configuration",
		Icon:             "󰌆",
		ArgsHint:         "<value>",
		Tier:             TierSecondary,
		Sensitive:        true,
		SupportsHeadless: true,
	}
}

func handleAPIKey(host Host, args []string) Result {
	if len(args) == 0 || strings.TrimSpace(strings.Join(args, " ")) == "" {
		host.CommandUsage("/api-key <value>")
		return Done(nil)
	}

	provider := host.Provider()
	host.EnsureProviderConfig(provider)
	pc := host.ProviderConfig(provider)
	pc.APIKey = strings.TrimSpace(strings.Join(args, " "))
	host.SetProviderConfig(provider, pc)

	if err := host.SwitchProvider(provider, host.Model()); err != nil {
		host.CommandError(err.Error())
		return Done(nil)
	}

	host.PersistProjectConfig()
	host.AddSystemMessage(fmt.Sprintf("API key updated for %s. The value is hidden.", provider))
	host.SetStatus("API key updated")
	return Done(nil)
}

func baseURLCommand() Command {
	return Command{
		Name:             "base-url",
		Description:      "Set active provider base URL",
		Category:         "Configuration",
		Icon:             "󰖟",
		ArgsHint:         "<url>",
		Tier:             TierTertiary,
		SupportsHeadless: true,
	}
}

func handleBaseURL(host Host, args []string) Result {
	if len(args) != 1 {
		host.CommandUsage("/base-url <url>")
		return Done(nil)
	}

	raw := strings.TrimSpace(args[0])
	clean, ok := parseURL(raw)
	if !ok {
		host.CommandError("Base URL must be a valid http:// or https:// URL")
		return Done(nil)
	}

	provider := host.Provider()
	host.EnsureProviderConfig(provider)
	pc := host.ProviderConfig(provider)
	pc.BaseURL = clean
	host.SetProviderConfig(provider, pc)

	if err := host.SwitchProvider(provider, host.Model()); err != nil {
		host.CommandError(err.Error())
		return Done(nil)
	}

	host.PersistProjectConfig()
	host.AddSystemMessage(fmt.Sprintf("Base URL updated for %s", provider))
	host.SetStatus("Base URL updated")
	return Done(nil)
}
