package command

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/iSundram/Automergent/internal/agent"
)

// --- AI & Model Handlers ---

func handleModel(host Host, args []string) tea.Cmd {
	if len(args) == 0 {
		host.AddSystemMessage(fmt.Sprintf("Current model: %s (provider: %s)\nUsage: /model <model-name|reset>", host.Model(), host.Provider()))
		return nil
	}

	model := strings.Join(args, " ")
	if model == "reset" {
		provider := host.Provider()
		defaultModel := ""
		for _, m := range host.ModelsAvailable() {
			if strings.Contains(strings.ToLower(m.ID), strings.ToLower(provider)) {
				defaultModel = m.ID
				break
			}
		}
		if defaultModel == "" && len(host.ModelsAvailable()) > 0 {
			defaultModel = host.ModelsAvailable()[0].ID
		}
		if err := host.SwitchProvider(provider, defaultModel); err != nil {
			host.CommandError(fmt.Sprintf("Error resetting model: %v", err))
			return nil
		}
		host.AddSystemMessage(fmt.Sprintf("Model reset to default for %s: %s", provider, host.Model()))
		host.PersistProjectConfig()
		host.SetStatus("Model updated")
		return nil
	}

	if err := host.SwitchProvider(host.Provider(), model); err != nil {
		host.CommandError(fmt.Sprintf("Error: %v", err))
		return nil
	}
	host.AddSystemMessage(fmt.Sprintf("Model switched to %s", model))
	host.PersistProjectConfig()
	host.SetStatus("Model updated")
	return nil
}

func handleProvider(host Host, args []string) tea.Cmd {
	if len(args) == 0 {
		host.AddSystemMessage(fmt.Sprintf("Current provider: %s (model: %s)\nAvailable: %s\nUsage: /provider <name> [model]", host.Provider(), host.Model(), strings.Join(host.Providers(), ", ")))
		return nil
	}

	provider := args[0]
	if !isSupportedProvider(provider) {
		host.CommandError(fmt.Sprintf("Unknown provider %q", provider))
		return nil
	}
	model := ""
	if len(args) > 1 {
		model = args[1]
	}

	if err := host.SwitchProvider(provider, model); err != nil {
		host.CommandError(fmt.Sprintf("Error: %v", err))
		return nil
	}
	host.AddSystemMessage(fmt.Sprintf("Provider switched to %s", provider))
	if pc := host.ProviderConfig(provider); pc.APIKey == "" {
		host.AddSystemMessage(fmt.Sprintf("Warning: No API key set for %s. Use /api-key <key>.", provider))
		host.SetStatus("Provider updated (no API key)")
	} else {
		host.SetStatus("Provider updated")
	}
	host.PersistProjectConfig()
	return nil
}

func handleMode(host Host, args []string) tea.Cmd {
	if len(args) == 0 {
		host.AddSystemMessage(fmt.Sprintf("Current mode: %s\nUsage: /mode <edit|plan>", host.Mode()))
		return nil
	}

	mode := args[0]
	if !agent.IsValid(mode) {
		host.CommandError("Error: usage /mode <edit|plan>")
		return nil
	}

	host.SetMode(mode)
	host.AddSystemMessage(fmt.Sprintf("Mode switched to %s", mode))
	host.PersistProjectConfig()
	host.SetStatus("Mode updated")
	return nil
}

func handleContext(host Host, args []string) tea.Cmd {
	if len(args) > 0 && args[0] == "detail" {
		host.ShowContextDetail()
		return nil
	}

	host.AddSystemMessage(fmt.Sprintf("Provider: %s\nModel: %s\nInput tokens: %d\nOutput tokens: %d\nTotal cost: $%.4f\nActive tokens: %d\n\nUse '/context detail' for telemetry breakdown.",
		host.Provider(), host.Model(), host.InputTokens(), host.OutputTokens(), host.TotalCost(), host.ActiveTokens()))
	return nil
}