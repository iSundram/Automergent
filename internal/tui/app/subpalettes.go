package app

// Sub-palette specs: one entry per argument-picker trigger. This table is the
// single source of truth for what the palette shows after "/<trigger> " —
// updatePalette() resolves the trigger here and renders whatever Build
// returns; nothing else knows about individual triggers. Adding a picker is
// one spec here plus SubPalette on the command definition.

import (
	"fmt"
	"strings"

	"github.com/iSundram/Automergent/internal/agent"
	"github.com/iSundram/Automergent/internal/config"
	"github.com/iSundram/Automergent/internal/tui/components"
)

// SubPaletteSpec describes one argument-picker palette.
type SubPaletteSpec struct {
	// Trigger is the slash command name that opens this palette
	// (e.g. "model" for "/model ").
	Trigger string
	// EmptyHint replaces the generic "no matching results" line.
	EmptyHint string
	// Build produces the items for the current app state.
	Build func(*App) []components.PaletteItem
}

// subPaletteSpecs is the spec table for every picker trigger.
var subPaletteSpecs = map[string]SubPaletteSpec{
	"model": {
		Trigger:   "model",
		EmptyHint: "No matching models",
		Build:     buildModelItems,
	},
	"provider": {
		Trigger:   "provider",
		EmptyHint: "No matching providers",
		Build:     buildProviderItems,
	},
	"mode": {
		Trigger:   "mode",
		EmptyHint: "No matching modes",
		Build:     buildModeItems,
	},
	"theme": {
		Trigger:   "theme",
		EmptyHint: "No matching themes",
		Build:     buildThemeItems,
	},
	"keybindings": {
		Trigger:   "keybindings",
		EmptyHint: "No matching keybinding schemes",
		Build:     buildKeybindingsItems,
	},
	"effort": {
		Trigger:   "effort",
		EmptyHint: "No matching effort levels",
		Build:     buildEffortItems,
	},
	"file": {
		Trigger:   "file",
		EmptyHint: "No matching files",
		Build:     buildFileItems,
	},
	"run": {
		Trigger:   "run",
		EmptyHint: "No matching project commands",
		Build:     buildRunItems,
	},
	"commit": {
		Trigger:   "commit",
		EmptyHint: "Choose a commit scope",
		Build: func(a *App) []components.PaletteItem {
			return []components.PaletteItem{
				{Label: "all changes", Description: "Stage and commit all modified files", Value: "", Icon: "⎿", Category: "Commit Scope"},
				{Label: "staged only", Description: "Commit only already-staged files", Value: "--staged", Icon: "⎿", Category: "Commit Scope"},
			}
		},
	},
	"review": {
		Trigger:   "review",
		EmptyHint: "Choose a review target",
		Build: func(a *App) []components.PaletteItem {
			return []components.PaletteItem{
				{Label: "uncommitted changes", Description: "Review pending workspace changes", Value: "", Icon: "→", Category: "Review Target"},
				{Label: "latest commit", Description: "Review the most recent commit", Value: "HEAD", Icon: "→", Category: "Review Target"},
			}
		},
	},
	"mcp": {
		Trigger:   "mcp",
		EmptyHint: "No matching MCP sub-command",
		Build:     buildSubCommandItems("mcp"),
	},
	"commands": {
		Trigger:   "commands",
		EmptyHint: "No matching sub-command",
		Build:     buildSubCommandItems("commands"),
	},
	"directory": {
		Trigger:   "directory",
		EmptyHint: "No matching sub-command",
		Build:     buildSubCommandItems("directory"),
	},
	"plan": {
		Trigger:   "plan",
		EmptyHint: "Type a plan focus, or pick /plan copy",
		Build: buildSubCommandItems("plan", func(items []components.PaletteItem) []components.PaletteItem {
			if len(items) == 0 {
				return []components.PaletteItem{{Label: "plan", Description: "Enter plan mode", Value: "plan", Icon: "◌", Category: "plan"}}
			}
			return items
		}),
	},
	"goal": {
		Trigger:   "goal",
		EmptyHint: "No matching sub-command",
		Build:     buildSubCommandItems("goal"),
	},
}

// buildSubCommandItems returns a Build func listing a command's sub-commands,
// with an optional tail hook for special cases.
func buildSubCommandItems(name string, tail ...func([]components.PaletteItem) []components.PaletteItem) func(*App) []components.PaletteItem {
	return func(a *App) []components.PaletteItem {
		items := a.commands.SubCommandPaletteItems(a, name)
		if len(tail) > 0 && tail[0] != nil {
			items = tail[0](items)
		}
		return items
	}
}

// subPaletteSpecFor resolves a trigger to its spec.
func subPaletteSpecFor(trigger string) (SubPaletteSpec, bool) {
	spec, ok := subPaletteSpecs[trigger]
	return spec, ok
}

// paletteEmptyHintFor resolves the empty-state hint for a trigger.
func paletteEmptyHintFor(trigger string) string {
	if spec, ok := subPaletteSpecFor(trigger); ok && spec.EmptyHint != "" {
		return spec.EmptyHint
	}
	switch trigger {
	case "command", "help":
		return "No matching command — enter sends it as a message"
	}
	return ""
}

// --- Item builders (moved verbatim from updatePalette) ---

func buildModelItems(a *App) []components.PaletteItem {
	var items []components.PaletteItem
	for _, m := range a.modelsAvailable() {
		desc := fmt.Sprintf("Limit: %d", m.ContextLimit)
		if m.InputPrice > 0 || m.OutputPrice > 0 {
			desc += fmt.Sprintf(" $%.4g/$%.4g", m.InputPrice, m.OutputPrice)
		}
		items = append(items, components.PaletteItem{
			Label:       m.ID,
			Description: desc,
			Value:       m.ID,
			Icon:        "●",
			Category:    "Models",
			Current:     m.ID == a.cfg.Model,
		})
	}
	if len(items) == 0 && a.fetchingModels {
		return []components.PaletteItem{{Label: "Loading…", Description: "Fetching models from provider", Value: "", Icon: "◌", Category: "Models"}}
	}
	return items
}

func buildProviderItems(a *App) []components.PaletteItem {
	var items []components.PaletteItem
	for _, p := range a.availableProviders {
		spec, _ := config.ProviderSpecFor(p)
		desc := spec.Description
		if desc == "" {
			desc = "AI provider"
		}
		items = append(items, components.PaletteItem{
			Label: p, Description: desc, Value: p, Icon: config.ProviderIcon(p),
			Category: "Providers", Current: p == a.cfg.Provider,
		})
	}
	return items
}

func buildModeItems(a *App) []components.PaletteItem {
	modeIcons := map[string]string{
		"manual":       "○",
		"accept-edits": "✓",
		"auto":         "▸",
		"plan":         "◌",
	}
	current := agent.CanonicalMode(a.cfg.Mode)
	var items []components.PaletteItem
	for _, mode := range agent.AllModes() {
		items = append(items, components.PaletteItem{
			Label:       mode,
			Description: agent.ModeDescription(mode),
			Value:       mode,
			Icon:        modeIcons[mode],
			Category:    "Modes",
			Current:     mode == current,
		})
	}
	return items
}

func buildThemeItems(a *App) []components.PaletteItem {
	var items []components.PaletteItem
	for _, t := range a.AvailableThemes() {
		items = append(items, components.PaletteItem{
			Label: t, Description: "UI theme", Value: t, Icon: "●", Category: "Themes",
			Current: t == a.cfg.Theme,
		})
	}
	return items
}

func buildKeybindingsItems(a *App) []components.PaletteItem {
	var items []components.PaletteItem
	for _, k := range a.AvailableKeybindings() {
		items = append(items, components.PaletteItem{
			Label: k, Description: "Keybinding scheme", Value: k, Icon: "→", Category: "Keybindings",
			Current: k == a.cfg.Keybindings,
		})
	}
	return items
}

func buildEffortItems(a *App) []components.PaletteItem {
	pc := a.ProviderConfig(a.Provider())
	currentEffort := pc.Effort
	if currentEffort == "" {
		currentEffort = pc.ThinkingLevel
	}
	var items []components.PaletteItem
	for _, e := range []string{"minimal", "low", "medium", "high"} {
		items = append(items, components.PaletteItem{
			Label: e, Description: "Thinking effort", Value: e, Icon: "●", Category: "Effort",
			Current: e == currentEffort,
		})
	}
	return items
}

func buildFileItems(a *App) []components.PaletteItem {
	var items []components.PaletteItem
	for _, item := range a.fileTree.Items() {
		if !item.IsDir {
			items = append(items, components.PaletteItem{
				Label:       item.Name,
				Description: item.Path,
				Value:       item.Path,
				Icon:        "●",
				Category:    "Files",
			})
		}
	}
	return items
}

func buildRunItems(a *App) []components.PaletteItem {
	return a.detectRunCommands()
}

// argumentItems returns palette items for a command's arguments: its
// sub-commands, then its Completion function for the current partial, then
// per-sub-command dynamic providers (e.g. MCP server names). This is the one
// path argument completion flows through — no trigger-specific special cases
// in updatePalette.
func (a *App) argumentItems(trigger, filter string) []components.PaletteItem {
	// Sub-command list (shown before a sub-command is chosen).
	parts := strings.Fields(filter)
	subChosen := false
	if len(parts) > 0 {
		if _, ok := a.commands.LookupSubCommand(trigger, parts[0]); ok {
			subChosen = true
		}
	}

	if !subChosen {
		items := a.commands.SubCommandPaletteItems(a, trigger)
		if len(items) > 0 {
			return items
		}
		// Fall through to Completion for argument-less commands.
	}

	// A sub-command (or the command itself) is chosen: offer argument values.
	var partial string
	if len(parts) > 1 {
		partial = strings.Join(parts[1:], " ")
	}
	// Dynamic providers: sub-commands that enumerate host state.
	if subChosen {
		if items := a.subCommandValueItems(trigger, parts[0], partial); len(items) > 0 {
			return items
		}
	}
	// Generic Completion function on the command definition.
	if cmd, ok := a.commands.Lookup(trigger); ok && cmd.Completion != nil {
		var items []components.PaletteItem
		for _, c := range cmd.Completion(a, partial) {
			items = append(items, components.PaletteItem{
				Label: c, Value: c, Icon: cmd.Icon, Category: cmd.Name,
			})
		}
		return items
	}
	return nil
}

// subCommandValueItems provides dynamic value lists for sub-commands whose
// arguments are enumerable (currently MCP enable/disable server names).
func (a *App) subCommandValueItems(trigger, sub, partial string) []components.PaletteItem {
	if trigger == "mcp" && (sub == "enable" || sub == "disable") {
		var items []components.PaletteItem
		for _, s := range a.MCPStatus() {
			items = append(items, components.PaletteItem{
				Label: s.Name, Description: s.Status, Value: s.Name, Icon: "→", Category: "MCP Servers",
			})
		}
		return items
	}
	return nil
}
