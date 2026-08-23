package app

// Slash-command dispatch and palette glue.
// Moved verbatim from internal/tui/app.go.

import (
	tea "charm.land/bubbletea/v2"
	"fmt"
	"github.com/iSundram/Automergent/internal/tui/commands"
	"github.com/iSundram/Automergent/internal/tui/components"
	"github.com/sahilm/fuzzy"
)

func (a *App) updatePalette() {
	trigger := a.input.TriggerType()
	filter := a.input.TriggerValue()
	a.palette.SetQuery(filter)

	var items []components.PaletteItem
	switch trigger {
	case "help", "command":
		// Decorations (Current/Disabled) come from the registry definitions.
		items = a.fuzzyFilter(a.commands.PaletteItems(a), filter)

	case "model":
		var modelItems []components.PaletteItem
		for _, m := range a.availableModels {
			modelItems = append(modelItems, components.PaletteItem{
				Label:       m.ID,
				Description: fmt.Sprintf("Model (Limit: %d)", m.ContextLimit),
				Value:       m.ID,
				Icon:        "󰊕",
				Category:    "Models",
				Current:     m.ID == a.cfg.Model,
			})
		}
		if len(modelItems) == 0 && a.fetchingModels {
			items = []components.PaletteItem{{Label: "Loading…", Description: "Fetching models from provider", Value: "", Icon: "󰔟", Category: "Models"}}
		} else {
			items = a.fuzzyFilter(modelItems, filter)
		}
	case "provider":
		providerDescriptions := map[string]string{
			"google": "Gemini models by Google",
		}
		providerIcons := map[string]string{
			"google": "󰊭",
		}
		var providerItems []components.PaletteItem
		for _, p := range a.availableProviders {
			desc := providerDescriptions[p]
			if desc == "" {
				desc = "AI provider"
			}
			icon := providerIcons[p]
			if icon == "" {
				icon = "🔌"
			}
			providerItems = append(providerItems, components.PaletteItem{
				Label: p, Description: desc, Value: p, Icon: icon, Category: "Providers", Current: p == a.cfg.Provider,
			})
		}
		items = a.fuzzyFilter(providerItems, filter)

	case "mode":
		modeItems := []components.PaletteItem{
			{Label: "edit", Description: "Allow edits with permission", Value: "edit", Icon: "󰏫", Category: "Modes", Current: a.cfg.Mode == "edit"},
			{Label: "plan", Description: "Plan and inspect without edits", Value: "plan", Icon: "󰈙", Category: "Modes", Current: a.cfg.Mode == "plan"},
		}
		items = a.fuzzyFilter(modeItems, filter)

	case "theme":
		var themeItems []components.PaletteItem
		for _, t := range a.AvailableThemes() {
			themeItems = append(themeItems, components.PaletteItem{
				Label: t, Description: "UI theme", Value: t, Icon: "󰏘", Category: "Themes",
				Current: t == a.cfg.Theme,
			})
		}
		items = a.fuzzyFilter(themeItems, filter)

	case "keybindings":
		var keyItems []components.PaletteItem
		for _, k := range a.AvailableKeybindings() {
			keyItems = append(keyItems, components.PaletteItem{
				Label: k, Description: "Keybinding scheme", Value: k, Icon: "󰌌", Category: "Keybindings",
				Current: k == a.cfg.Keybindings,
			})
		}
		items = a.fuzzyFilter(keyItems, filter)

	case "effort":
		pc := a.ProviderConfig(a.Provider())
		currentEffort := pc.Effort
		if currentEffort == "" {
			currentEffort = pc.ThinkingLevel
		}
		var effortItems []components.PaletteItem
		for _, e := range []string{"minimal", "low", "medium", "high"} {
			effortItems = append(effortItems, components.PaletteItem{
				Label: e, Description: "Thinking effort", Value: e, Icon: "󰓅", Category: "Effort",
				Current: e == currentEffort,
			})
		}
		items = a.fuzzyFilter(effortItems, filter)

	case "file":
		var fileItems []components.PaletteItem
		for _, item := range a.fileTree.Items() {
			if !item.IsDir {
				fileItems = append(fileItems, components.PaletteItem{
					Label:       item.Name,
					Description: item.Path,
					Value:       item.Path,
					Icon:        "󰈔",
					Category:    "Files",
				})
			}
		}
		items = a.fuzzyFilter(fileItems, filter)
	}

	a.palette.SetItems(items)
}

func (a *App) fuzzyFilter(items []components.PaletteItem, filter string) []components.PaletteItem {
	if filter == "" {
		return items
	}
	var targets []string
	for _, item := range items {
		targets = append(targets, item.Label+" "+item.SearchTerms)
	}
	matches := fuzzy.Find(filter, targets)
	var filtered []components.PaletteItem
	for _, match := range matches {
		filtered = append(filtered, items[match.Index])
	}
	return filtered
}

// restoreSession switches the active runtime and rebuilds the conversation
// view from the structured message history.

func (a *App) handleSlashCommand(input string) tea.Cmd {
	name, args := commands.Parse(input)
	if name == "" {
		return nil
	}
	host := a
	result, err := a.commands.Dispatch(host, name, args)
	if err != nil {
		// A command unknown to the registry may have been added to disk after
		// startup: reload custom commands once and retry before failing.
		if _, unknown := err.(commands.ErrUnknownCommand); unknown {
			if a.refreshCustomCommands() > 0 {
				if result, err = a.commands.Dispatch(host, name, args); err == nil {
					return a.deliverCommandResult(result)
				}
			}
		}
		switch e := err.(type) {
		case commands.ErrUnknownCommand:
			a.conversation.AddMessage("assistant", fmt.Sprintf("Unknown command: /%s", name), true)
		case commands.ErrCommandDisabled:
			a.statusBar.SetStatus(e.Reason)
		default:
			a.CommandError(err.Error())
		}
		return nil
	}
	return a.deliverCommandResult(result)
}

// deliverCommandResult relays a successful dispatch outcome to the UI.
func (a *App) deliverCommandResult(result commands.Result) tea.Cmd {
	if result.Text != "" {
		a.conversation.AddMessage("system", result.Text, false)
	}
	return result.Cmd
}

// dispatchByName runs a registered command by canonical name so keyboard
// shortcuts and slash commands share one code path. Failures are silent:
// shortcuts only bind to commands that cannot fail dispatch.
func (a *App) dispatchByName(name string) tea.Cmd {
	result, err := a.commands.Dispatch(a, name, nil)
	if err != nil {
		return nil
	}
	return a.deliverCommandResult(result)
}

func (a *App) registerSessionCommands() {
	must := func(cmd commands.Command, h commands.Handler) {
		_ = a.commands.RegisterCustom(cmd, h)
	}
	must(commands.Command{
		Name:        "zen",
		Aliases:     []string{"focus"},
		Description: "Toggle zen mode (hide header/HUD chrome)",
		Category:    "View",
		Icon:        "󰍽",
	}, func(h commands.Host, args []string) commands.Result {
		a.zenMode = !a.zenMode
		a.statusBar.SetSegmentsVisible(!a.zenMode)
		a.statusBar.SetStatus(map[bool]string{true: "Zen mode", false: "Chrome restored"}[a.zenMode])
		a.layout()
		return commands.Done(nil)
	})
}
