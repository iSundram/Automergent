package commands

import (
	"context"
	"fmt"
	"strings"

	"github.com/iSundram/Automergent/internal/modelsdev"
)

// /effort — set or show provider thinking effort.
//
// The accepted levels come from the active model's models.dev catalog entry
// (reasoning_options.effort), so the command both documents and enforces
// what the model actually supports. Models without effort control fall back
// to the provider's generic set.

func effortCommand() Command {
	return Command{
		Name:             "effort",
		Description:      "Set or show thinking effort (model-aware)",
		Category:         "Configuration",
		Icon:             "󰓅",
		ArgsHint:         "[effort-level]",
		Tier:             TierSecondary,
		SubPalette:       "effort",
		SupportsHeadless: true,
	}
}

// providerEfforts returns the fallback effort set for providers whose models
// predate effort control in the catalog.
var providerEfforts = map[string][]string{
	"google":          {"minimal", "low", "medium", "high"},
	"google-aistudio": {"minimal", "low", "medium", "high"},
	"google-vertex":   {"minimal", "low", "medium", "high"},
	"anthropic":       {"low", "medium", "high"},
	"openai":          {"minimal", "low", "medium", "high"},
	"deepseek":        {"low", "medium", "high"},
	"ollama":          {"low", "medium", "high"},
}

// EffortsForModel resolves the effort levels the active provider+model
// accepts: the model's catalog entry first, then the provider default. It is
// the single source shared by /effort, the effort palette and the model hub.
func EffortsForModel(provider, model string) []string {
	if model != "" {
		if m, ok := modelsdev.ModelInfo(context.Background(), provider, model); ok && len(m.Efforts) > 0 {
			return m.Efforts
		}
	}
	if efforts := providerEfforts[provider]; len(efforts) > 0 {
		return efforts
	}
	return []string{"low", "medium", "high"}
}

func handleEffort(host Host, args []string) Result {
	provider := host.Provider()
	host.EnsureProviderConfig(provider)
	pc := host.ProviderConfig(provider)
	model := host.Model()

	efforts := EffortsForModel(provider, model)
	current := pc.Effort
	if current == "" {
		current = pc.ThinkingLevel
	}
	source := "provider default"
	if model != "" {
		if m, ok := modelsdev.ModelInfo(context.Background(), provider, model); ok && len(m.Efforts) > 0 {
			source = "models.dev catalog"
		}
	}

	if len(args) == 0 {
		currentDisplay := current
		if currentDisplay == "" {
			currentDisplay = "high (default)"
		}
		var b strings.Builder
		fmt.Fprintf(&b, "Current effort for %s/%s: %s", provider, model, currentDisplay)
		fmt.Fprintf(&b, "\nSupported by %s: %s", source, strings.Join(efforts, " · "))
		if !effortAllowed(efforts, current) && current != "" {
			fmt.Fprintf(&b, "\n⚠ current effort %q is not listed for this model", current)
		}
		b.WriteString("\nUsage: /effort <level>")
		host.AddSystemMessage(b.String())
		return Done(nil)
	}

	effort := strings.ToLower(args[0])
	if !effortAllowed(efforts, effort) {
		host.CommandError(fmt.Sprintf("Error: %q is not supported by %s (supported: %s)", effort, model, strings.Join(efforts, ", ")))
		return Done(nil)
	}

	// Set both legacy Effort and new ThinkingLevel for compatibility
	pc.Effort = effort
	pc.ThinkingLevel = effort
	host.SetProviderConfig(provider, pc)

	if err := host.SwitchProvider(provider, host.Model()); err != nil {
		host.CommandError(fmt.Sprintf("Error: %v", err))
		return Done(nil)
	}

	host.PersistProjectConfig()
	host.AddSystemMessage(fmt.Sprintf("Effort for %s/%s set to %s", provider, model, effort))
	host.SetStatus("Effort updated")
	return Done(nil)
}

// effortAllowed reports whether an effort level is in the supported set. An
// empty current value is always allowed (means "not set").
func effortAllowed(efforts []string, effort string) bool {
	if effort == "" {
		return true
	}
	for _, e := range efforts {
		if e == effort {
			return true
		}
	}
	return false
}
