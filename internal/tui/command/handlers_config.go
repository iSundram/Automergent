package command

import (
	"fmt"
	"net/url"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// --- Configuration Handlers ---

func handleAPIKey(host Host, args []string) tea.Cmd {
	if len(args) == 0 || strings.TrimSpace(strings.Join(args, " ")) == "" {
		host.CommandUsage("/api-key <value>")
		return nil
	}

	provider := host.Provider()
	host.EnsureProviderConfig(provider)
	pc := host.ProviderConfig(provider)
	pc.APIKey = strings.TrimSpace(strings.Join(args, " "))
	host.SetProviderConfig(provider, pc)

	if err := host.SwitchProvider(provider, host.Model()); err != nil {
		host.CommandError(err.Error())
		return nil
	}

	host.PersistProjectConfig()
	host.AddSystemMessage(fmt.Sprintf("API key updated for %s. The value is hidden.", provider))
	host.SetStatus("API key updated")
	return nil
}

func handleBaseURL(host Host, args []string) tea.Cmd {
	if len(args) != 1 {
		host.CommandUsage("/base-url <url>")
		return nil
	}

	raw := strings.TrimSpace(args[0])
	parsed, err := url.ParseRequestURI(raw)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		host.CommandError("Base URL must be a valid http:// or https:// URL")
		return nil
	}

	provider := host.Provider()
	host.EnsureProviderConfig(provider)
	pc := host.ProviderConfig(provider)
	pc.BaseURL = strings.TrimRight(raw, "/")
	host.SetProviderConfig(provider, pc)

	if err := host.SwitchProvider(provider, host.Model()); err != nil {
		host.CommandError(err.Error())
		return nil
	}

	host.PersistProjectConfig()
	host.AddSystemMessage(fmt.Sprintf("Base URL updated for %s", provider))
	host.SetStatus("Base URL updated")
	return nil
}

func handleEffort(host Host, args []string) tea.Cmd {
	provider := host.Provider()
	host.EnsureProviderConfig(provider)
	pc := host.ProviderConfig(provider)

	if len(args) == 0 {
		current := pc.Effort
		if current == "" {
			current = "high (default)"
		}
		host.AddSystemMessage(fmt.Sprintf("Current effort for %s: %s\nUsage: /effort <minimal|low|medium|high>", provider, current))
		return nil
	}

	effort := strings.ToLower(args[0])
	switch effort {
	case "minimal", "low", "medium", "high":
		pc.Effort = effort
		host.SetProviderConfig(provider, pc)

		if err := host.SwitchProvider(provider, host.Model()); err != nil {
			host.CommandError(fmt.Sprintf("Error: %v", err))
			return nil
		}

		host.PersistProjectConfig()
		host.AddSystemMessage(fmt.Sprintf("Effort for %s set to %s", provider, effort))
		host.SetStatus("Effort updated")
	default:
		host.CommandError("Error: usage /effort <minimal|low|medium|high>")
	}
	return nil
}

func handleProviderAPIKey(host Host, args []string) tea.Cmd {
	if len(args) < 2 {
		host.CommandUsage("/provider-api-key <provider> <value>")
		return nil
	}

	provider := args[0]
	if !isSupportedProvider(provider) {
		host.CommandError(fmt.Sprintf("Unknown provider %q", provider))
		return nil
	}

	host.EnsureProviderConfig(provider)
	pc := host.ProviderConfig(provider)
	pc.APIKey = strings.TrimSpace(strings.Join(args[1:], " "))
	host.SetProviderConfig(provider, pc)

	if provider == host.Provider() {
		if err := host.SwitchProvider(provider, host.Model()); err != nil {
			host.CommandError(err.Error())
			return nil
		}
	}

	host.PersistProjectConfig()
	host.AddSystemMessage(fmt.Sprintf("API key updated for %s", provider))
	return nil
}

func handleProviderBaseURL(host Host, args []string) tea.Cmd {
	if len(args) != 2 {
		host.CommandUsage("/provider-base-url <provider> <url>")
		return nil
	}

	provider := args[0]
	if !isSupportedProvider(provider) {
		host.CommandError(fmt.Sprintf("Unknown provider %q", provider))
		return nil
	}

	raw := strings.TrimSpace(args[1])
	parsed, err := url.ParseRequestURI(raw)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		host.CommandError("Base URL must be a valid http:// or https:// URL")
		return nil
	}

	host.EnsureProviderConfig(provider)
	pc := host.ProviderConfig(provider)
	pc.BaseURL = strings.TrimRight(raw, "/")
	host.SetProviderConfig(provider, pc)

	if provider == host.Provider() {
		if err := host.SwitchProvider(provider, host.Model()); err != nil {
			host.CommandError(err.Error())
			return nil
		}
	}

	host.PersistProjectConfig()
	host.AddSystemMessage(fmt.Sprintf("Base URL updated for %s", provider))
	return nil
}

func isSupportedProvider(name string) bool {
	switch name {
	case "google":
		return true
	default:
		return false
	}
}