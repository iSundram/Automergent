package commands

import (
	"fmt"
	"strings"
)

// /provider-api-key — set a specific provider's API key.
// /provider-base-url — set a specific provider's base URL.

func providerAPIKeyCommand() Command {
	return Command{
		Name:             "provider-api-key",
		Description:      "Set an AI provider API key",
		Category:         "Configuration",
		Icon:             "󰌋",
		ArgsHint:         "<provider> <value>",
		Tier:             TierTertiary,
		Sensitive:        true,
		SupportsHeadless: true,
	}
}

func handleProviderAPIKey(host Host, args []string) Result {
	if len(args) < 2 {
		host.CommandUsage("/provider-api-key <provider> <value>")
		return Done(nil)
	}

	provider := args[0]
	host.EnsureProviderConfig(provider)
	pc := host.ProviderConfig(provider)
	pc.APIKey = strings.TrimSpace(strings.Join(args[1:], " "))
	host.SetProviderConfig(provider, pc)

	if provider == host.Provider() {
		if err := host.SwitchProvider(provider, host.Model()); err != nil {
			host.CommandError(err.Error())
			return Done(nil)
		}
	}

	host.PersistProjectConfig()
	host.AddSystemMessage(fmt.Sprintf("API key updated for %s", provider))
	return Done(nil)
}

func providerBaseURLCommand() Command {
	return Command{
		Name:             "provider-base-url",
		Description:      "Set an AI provider base URL",
		Category:         "Configuration",
		Icon:             "󰌷",
		ArgsHint:         "<provider> <url>",
		Tier:             TierTertiary,
		SupportsHeadless: true,
	}
}

func handleProviderBaseURL(host Host, args []string) Result {
	if len(args) != 2 {
		host.CommandUsage("/provider-base-url <provider> <url>")
		return Done(nil)
	}

	provider := args[0]
	clean, ok := parseURL(strings.TrimSpace(args[1]))
	if !ok {
		host.CommandError("Base URL must be a valid http:// or https:// URL")
		return Done(nil)
	}

	host.EnsureProviderConfig(provider)
	pc := host.ProviderConfig(provider)
	pc.BaseURL = clean
	host.SetProviderConfig(provider, pc)

	if provider == host.Provider() {
		if err := host.SwitchProvider(provider, host.Model()); err != nil {
			host.CommandError(err.Error())
			return Done(nil)
		}
	}

	host.PersistProjectConfig()
	host.AddSystemMessage(fmt.Sprintf("Base URL updated for %s", provider))
	return Done(nil)
}
