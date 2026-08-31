package commands

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/iSundram/Automergent/internal/config"
)

// /model — switch, list, or manage models for the active provider.
// The bare command shows status; sub-commands cover the registry operations
// and a bare model name switches providers to it.

func modelCommand() Command {
	return Command{
		Name:        "model",
		Description: "Switch, list, or manage models for the active provider",
		Category:    "AI & Model",
		Icon:        "󰊕",
		ArgsHint:    "[name|list|add|remove|refresh|reset]",
		Tier:        TierSecondary,
		SubPalette:  "model",
		SubCommands: []SubCommand{
			{Name: "list", Description: "List available models", Handler: handleModel},
			{Name: "add", Description: "Add a custom model", ArgsHint: "<name>", Handler: handleModel},
			{Name: "remove", Description: "Remove a custom model", ArgsHint: "<name>", Handler: handleModel, ValueCompletion: customModelCompletion},
			{Name: "refresh", Description: "Refresh model list from provider", Handler: handleModel},
			{Name: "reset", Description: "Reset models to defaults", Handler: handleModel},
		},
		SupportsHeadless: true,
	}
}

// handleModel is the entry point for /model. Subcommands: (no args → status),
// list, info, add, remove, refresh, reset, and <name> (switch).
func handleModel(host Host, args []string) Result {
	provider := host.Provider()

	if len(args) == 0 {
		host.AddSystemMessage(fmt.Sprintf(
			"Current model: %s (provider: %s)\nUsage: /model <name> to switch · /model list · /model add <id> — register custom model · /model refresh · /model reset",
			host.Model(), provider))
		return Done(nil)
	}

	switch args[0] {
	case "list", "ls":
		return modelList(host, provider)
	case "info":
		if len(args) < 2 {
			host.CommandUsage("/model info <name>")
			return Done(nil)
		}
		return modelInfo(host, provider, strings.Join(args[1:], " "))
	case "refresh":
		host.AddSystemMessage(fmt.Sprintf("Refreshing model list from %s…", provider))
		return Done(host.RefreshModels())
	case "add":
		return modelAdd(host, provider, args[1:])
	case "remove", "rm":
		if len(args) < 2 {
			host.CommandUsage("/model remove <id>")
			return Done(nil)
		}
		return modelRemove(host, provider, args[1])
	case "reset":
		return modelReset(host, provider)
	}

	// /model <name>: switch the model of the active provider. Unknown names
	// are accepted (models may launch faster than catalogues update) and
	// remembered as the per-provider default for /provider use.
	model := strings.Join(args, " ")
	host.EnsureProviderConfig(provider)
	pc := host.ProviderConfig(provider)
	pc.DefaultModel = model
	host.SetProviderConfig(provider, pc)
	if err := host.SwitchProvider(provider, model); err != nil {
		host.CommandError(fmt.Sprintf("Error: %v", err))
		return Done(nil)
	}
	host.AddSystemMessage(fmt.Sprintf("Model switched to %s", model))
	host.PersistProjectConfig()
	host.SetStatus("Model updated")
	return Done(nil)
}

func modelList(host Host, provider string) Result {
	models := host.ModelsAvailable()
	current := host.Model()
	pc := host.ProviderConfig(provider)

	var b strings.Builder
	fmt.Fprintf(&b, "Models for %s (current: %s):\n", provider, current)
	if len(models) == 0 {
		b.WriteString("  (no models fetched yet — run /model refresh)\n")
	}
	for _, m := range models {
		marker := "  "
		if m.ID == current {
			marker = "▸ "
		}
		line := fmt.Sprintf("%s%-36s %s", marker, m.ID, formatContextLimit(m.ContextLimit))
		if pc.Models != nil {
			if mc, ok := pc.Models[m.ID]; ok {
				if mc.DisplayName != "" {
					line += " " + mc.DisplayName
				}
				line += " (custom)"
				_ = mc
			}
		}
		if m.InputPrice > 0 || m.OutputPrice > 0 {
			line += fmt.Sprintf(" $%.4g/$%.4g", m.InputPrice, m.OutputPrice)
		}
		b.WriteString(line + "\n")
	}
	b.WriteString("\nUse /model <name> to switch · /model info <name> · /model add <id> for custom models")
	host.AddSystemMessage(strings.TrimRight(b.String(), "\n"))
	return Done(nil)
}

func modelInfo(host Host, provider, name string) Result {
	for _, m := range host.ModelsAvailable() {
		if m.ID == name {
			var b strings.Builder
			fmt.Fprintf(&b, "Model: %s\nName: %s\nContext limit: %s", m.ID, m.Name, formatContextLimit(m.ContextLimit))
			if m.InputPrice > 0 || m.OutputPrice > 0 {
				fmt.Fprintf(&b, "\nPrice: $%.4g input / $%.4g output per 1M tokens", m.InputPrice, m.OutputPrice)
			}
			if pc := host.ProviderConfig(provider); pc.Models != nil {
				if mc, ok := pc.Models[name]; ok {
					b.WriteString("\nSource: custom (user-registered)")
					if mc.ContextLimit > 0 {
						fmt.Fprintf(&b, "\nRegistered context limit: %s", formatContextLimit(mc.ContextLimit))
					}
				}
			}
			b.WriteString(fmt.Sprintf("\nProvider: %s", provider))
			host.AddSystemMessage(b.String())
			return Done(nil)
		}
	}
	host.CommandError(fmt.Sprintf("Unknown model %q for provider %s — run /model list or /model refresh", name, provider))
	return Done(nil)
}

func modelAdd(host Host, provider string, args []string) Result {
	if len(args) == 0 {
		host.CommandUsage("/model add <id> [--context <tokens>] [--name <label>] [--price-in <usd>] [--price-out <usd>]")
		return Done(nil)
	}
	spec, ok := config.ProviderSpecFor(provider)
	if ok && !spec.CustomModels {
		host.CommandError(fmt.Sprintf("Provider %q does not support custom models", provider))
		return Done(nil)
	}
	id := args[0]
	if strings.HasPrefix(id, "--") || strings.ContainsAny(id, " \t") {
		host.CommandError("Model ID must be a single token without spaces")
		return Done(nil)
	}
	mc := config.ModelConfig{}
	for i := 1; i < len(args); {
		flag := args[i]
		if !strings.HasPrefix(flag, "--") {
			host.CommandUsage("/model add <id> [--context <tokens>] [--name <label>] [--price-in <usd>] [--price-out <usd>]")
			return Done(nil)
		}
		if i+1 >= len(args) {
			host.CommandError(fmt.Sprintf("Flag %q requires a value", flag))
			return Done(nil)
		}
		val := args[i+1]
		i += 2
		switch strings.TrimPrefix(flag, "--") {
		case "context":
			n, err := strconv.Atoi(val)
			if err != nil || n <= 0 {
				host.CommandError("--context must be a positive integer")
				return Done(nil)
			}
			// The platform ceiling is 1M tokens: larger values make the
			// context ladder unreliable (estimation drift compounds), so
			// they are clamped, not stored.
			mc.ContextLimit = config.ClampContextLimit(n)
		case "name":
			mc.DisplayName = val
		case "price-in":
			p, err := strconv.ParseFloat(val, 64)
			if err != nil || p < 0 {
				host.CommandError("--price-in must be a non-negative number")
				return Done(nil)
			}
			mc.InputPrice = p
		case "price-out":
			p, err := strconv.ParseFloat(val, 64)
			if err != nil || p < 0 {
				host.CommandError("--price-out must be a non-negative number")
				return Done(nil)
			}
			mc.OutputPrice = p
		default:
			host.CommandError(fmt.Sprintf("Unknown flag %q", flag))
			return Done(nil)
		}
	}
	host.EnsureProviderConfig(provider)
	pc := host.ProviderConfig(provider)
	if pc.Models == nil {
		pc.Models = map[string]config.ModelConfig{}
	}
	if old, ok := pc.Models[id]; ok {
		if mc.APIKey == "" {
			mc.APIKey = old.APIKey
		}
		if mc.BaseURL == "" {
			mc.BaseURL = old.BaseURL
		}
	}
	pc.Models[id] = mc
	host.SetProviderConfig(provider, pc)
	host.PersistProjectConfig()
	host.AddSystemMessage(fmt.Sprintf("Custom model %s registered for %s — /model refresh to list it", id, provider))
	host.SetStatus("Model registered")
	return Done(host.FetchModels())
}

func modelRemove(host Host, provider, id string) Result {
	host.EnsureProviderConfig(provider)
	pc := host.ProviderConfig(provider)
	if pc.Models == nil {
		host.CommandError(fmt.Sprintf("%q is not a custom model for %s", id, provider))
		return Done(nil)
	}
	if _, ok := pc.Models[id]; !ok {
		host.CommandError(fmt.Sprintf("%q is not a custom model for %s — run /model list to see registered models", id, provider))
		return Done(nil)
	}
	delete(pc.Models, id)
	host.SetProviderConfig(provider, pc)
	host.PersistProjectConfig()
	host.AddSystemMessage(fmt.Sprintf("Custom model %s removed from %s", id, provider))
	host.SetStatus("Model removed")
	if id == host.Model() {
		host.AddSystemMessage("Note: the removed model is still the active model until you switch to another one")
	}
	return Done(host.FetchModels())
}

func modelReset(host Host, provider string) Result {
	defaultModel := config.DefaultModelFor(provider)
	if defaultModel == "" {
		host.CommandError(fmt.Sprintf("No default model known for provider %q", provider))
		return Done(nil)
	}
	host.EnsureProviderConfig(provider)
	pc := host.ProviderConfig(provider)
	pc.DefaultModel = defaultModel
	host.SetProviderConfig(provider, pc)
	if err := host.SwitchProvider(provider, defaultModel); err != nil {
		host.CommandError(fmt.Sprintf("Error resetting model: %v", err))
		return Done(nil)
	}
	host.AddSystemMessage(fmt.Sprintf("Model reset to default for %s: %s", provider, host.Model()))
	host.PersistProjectConfig()
	host.SetStatus("Model updated")
	return Done(nil)
}

// customModelCompletion offers the provider's registered custom model ids.
func customModelCompletion(h Host, _ string) []string {
	pc := h.ProviderConfig(h.Provider())
	var names []string
	for name := range pc.Models {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
