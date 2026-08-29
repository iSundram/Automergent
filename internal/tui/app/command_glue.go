package app

// Slash-command dispatch and palette glue.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/iSundram/Automergent/internal/agent"
	"github.com/iSundram/Automergent/internal/config"
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
		items = a.fuzzyFilter(a.commands.PaletteItems(a), filter)

	case "model":
		var modelItems []components.PaletteItem
		for _, m := range a.modelsAvailable() {
			desc := fmt.Sprintf("Limit: %d", m.ContextLimit)
			if m.InputPrice > 0 || m.OutputPrice > 0 {
				desc += fmt.Sprintf(" $%.4g/$%.4g", m.InputPrice, m.OutputPrice)
			}
			modelItems = append(modelItems, components.PaletteItem{
				Label:       m.ID,
				Description: desc,
				Value:       m.ID,
				Icon:        "●",
				Category:    "Models",
				Current:     m.ID == a.cfg.Model,
			})
		}
		if len(modelItems) == 0 && a.fetchingModels {
			items = []components.PaletteItem{{Label: "Loading…", Description: "Fetching models from provider", Value: "", Icon: "◌", Category: "Models"}}
		} else {
			items = a.fuzzyFilter(modelItems, filter)
		}

	case "provider":
		var providerItems []components.PaletteItem
		for _, p := range a.availableProviders {
			spec, _ := config.ProviderSpecFor(p)
			desc := spec.Description
			if desc == "" {
				desc = "AI provider"
			}
			icon := config.ProviderIcon(p)
			providerItems = append(providerItems, components.PaletteItem{
				Label: p, Description: desc, Value: p, Icon: icon, Category: "Providers", Current: p == a.cfg.Provider,
			})
		}
		items = a.fuzzyFilter(providerItems, filter)

	case "mode":
		modeIcons := map[string]string{
			"manual":       "○",
			"accept-edits": "✓",
			"auto":         "▸",
			"plan":         "◌",
		}
		current := agent.CanonicalMode(a.cfg.Mode)
		var modeItems []components.PaletteItem
		for _, mode := range agent.AllModes() {
			modeItems = append(modeItems, components.PaletteItem{
				Label:       mode,
				Description: agent.ModeDescription(mode),
				Value:       mode,
				Icon:        modeIcons[mode],
				Category:    "Modes",
				Current:     mode == current,
			})
		}
		items = a.fuzzyFilter(modeItems, filter)

	case "theme":
		var themeItems []components.PaletteItem
		for _, t := range a.AvailableThemes() {
			themeItems = append(themeItems, components.PaletteItem{
				Label: t, Description: "UI theme", Value: t, Icon: "●", Category: "Themes",
				Current: t == a.cfg.Theme,
			})
		}
		items = a.fuzzyFilter(themeItems, filter)

	case "keybindings":
		var keyItems []components.PaletteItem
		for _, k := range a.AvailableKeybindings() {
			keyItems = append(keyItems, components.PaletteItem{
				Label: k, Description: "Keybinding scheme", Value: k, Icon: "→", Category: "Keybindings",
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
				Label: e, Description: "Thinking effort", Value: e, Icon: "●", Category: "Effort",
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
					Icon:        "●",
					Category:    "Files",
				})
			}
		}
		items = a.fuzzyFilter(fileItems, filter)

	case "run":
		runItems := a.detectRunCommands()
		items = a.fuzzyFilter(runItems, filter)

	case "commit":
		commitItems := []components.PaletteItem{
			{Label: "all changes", Description: "Stage and commit all modified files", Value: "", Icon: "⎿", Category: "Commit Scope"},
			{Label: "staged only", Description: "Commit only already-staged files", Value: "--staged", Icon: "⎿", Category: "Commit Scope"},
		}
		items = a.fuzzyFilter(commitItems, filter)

	case "review":
		reviewItems := []components.PaletteItem{
			{Label: "uncommitted changes", Description: "Review pending workspace changes", Value: "", Icon: "→", Category: "Review Target"},
			{Label: "latest commit", Description: "Review the most recent commit", Value: "HEAD", Icon: "→", Category: "Review Target"},
		}
		items = a.fuzzyFilter(reviewItems, filter)

	case "mcp":
		// First try sub-commands. If filter looks like "enable <partial>", delegate to Completion/second-level.
		subItems := a.commands.SubCommandPaletteItems(a, "mcp")
		// If user already typed a sub-command plus space, try Completion for argument completion.
		if strings.Contains(filter, " ") {
			parts := strings.Fields(filter)
			if len(parts) >= 1 {
				subName := parts[0]
				remaining := ""
				if len(parts) > 1 {
					remaining = strings.Join(parts[1:], " ")
				} else if strings.HasSuffix(filter, " ") {
					remaining = ""
				}
				if _, ok := a.commands.LookupSubCommand("mcp", subName); ok {
					// For enable/disable show server names.
					if subName == "enable" || subName == "disable" {
						var serverItems []components.PaletteItem
						for _, s := range a.MCPStatus() {
							serverItems = append(serverItems, components.PaletteItem{
								Label: s.Name, Description: s.Status, Value: s.Name, Icon: "→", Category: "MCP Servers",
							})
						}
						if len(serverItems) > 0 {
							items = a.fuzzyFilter(serverItems, remaining)
							break
						}
					}
				}
			}
		}
		items = a.fuzzyFilter(subItems, filter)

	case "commands":
		cmdItems := a.commands.SubCommandPaletteItems(a, "commands")
		items = a.fuzzyFilter(cmdItems, filter)

	case "directory":
		dirItems := a.commands.SubCommandPaletteItems(a, "directory")
		items = a.fuzzyFilter(dirItems, filter)

	case "plan":
		planItems := a.commands.SubCommandPaletteItems(a, "plan")
		// If user typed "plan " with no sub, also show prompt hint.
		if len(planItems) == 0 {
			planItems = []components.PaletteItem{{Label: "plan", Description: "Enter plan mode", Value: "plan", Icon: "◌", Category: "plan"}}
		}
		items = a.fuzzyFilter(planItems, filter)

	case "goal":
		goalItems := a.commands.SubCommandPaletteItems(a, "goal")
		items = a.fuzzyFilter(goalItems, filter)
	}

	// Generic Completion fallback: if we are in "command" mode and the filter
	// contains a command prefix that has a Completion function, show those results.
	if len(items) == 0 && trigger == "command" && strings.Contains(filter, " ") {
		parts := strings.Fields(filter)
		if len(parts) >= 1 {
			if cmd, ok := a.commands.Lookup(parts[0]); ok && cmd.Completion != nil {
				partial := ""
				if len(parts) > 1 {
					partial = strings.Join(parts[1:], " ")
				}
				if completions := cmd.Completion(a, partial); len(completions) > 0 {
					var compItems []components.PaletteItem
					for _, c := range completions {
						compItems = append(compItems, components.PaletteItem{
							Label: c, Value: c, Icon: cmd.Icon, Category: cmd.Name,
						})
					}
					items = a.fuzzyFilter(compItems, partial)
				}
			}
		}
	}

	a.palette.SetItems(items)
}

func (a *App) detectRunCommands() []components.PaletteItem {
	wd := a.workDir
	if wd == "" {
		wd = "."
	}
	has := func(name string) bool {
		_, err := os.Stat(filepath.Join(wd, name))
		return err == nil
	}
	var items []components.PaletteItem
	if has("package.json") {
		items = append(items,
			components.PaletteItem{Label: "npm run", Description: "Run npm script", Value: "npm run ", Icon: "▸", Category: "Project Commands"},
			components.PaletteItem{Label: "npm test", Description: "Run npm test", Value: "npm test", Icon: "▸", Category: "Project Commands"},
			components.PaletteItem{Label: "npm run build", Description: "Build via npm", Value: "npm run build", Icon: "▸", Category: "Project Commands"},
		)
	}
	if has("Makefile") || has("makefile") || has("GNUmakefile") {
		items = append(items,
			components.PaletteItem{Label: "make", Description: "Run make target", Value: "make ", Icon: "▸", Category: "Project Commands"},
			components.PaletteItem{Label: "make test", Description: "Run make test", Value: "make test", Icon: "▸", Category: "Project Commands"},
		)
	}
	if has("go.mod") {
		items = append(items,
			components.PaletteItem{Label: "go run", Description: "Run Go program", Value: "go run ", Icon: "▸", Category: "Project Commands"},
			components.PaletteItem{Label: "go test ./...", Description: "Run Go tests", Value: "go test ./...", Icon: "▸", Category: "Project Commands"},
		)
	}
	if has("Cargo.toml") {
		items = append(items,
			components.PaletteItem{Label: "cargo run", Description: "Run Rust program", Value: "cargo run", Icon: "▸", Category: "Project Commands"},
			components.PaletteItem{Label: "cargo test", Description: "Run Rust tests", Value: "cargo test", Icon: "▸", Category: "Project Commands"},
		)
	}
	if has("pyproject.toml") || has("requirements.txt") || has("setup.py") {
		items = append(items,
			components.PaletteItem{Label: "pytest", Description: "Run Python tests", Value: "pytest", Icon: "▸", Category: "Project Commands"},
			components.PaletteItem{Label: "python -m pytest", Description: "Run pytest via python", Value: "python -m pytest", Icon: "▸", Category: "Project Commands"},
		)
	}
	if len(items) == 0 {
		items = []components.PaletteItem{
			{Label: "npm run", Description: "Run npm script", Value: "npm run ", Icon: "▸", Category: "Project Commands"},
			{Label: "make", Description: "Run make target", Value: "make ", Icon: "▸", Category: "Project Commands"},
			{Label: "go run", Description: "Run Go program", Value: "go run ", Icon: "▸", Category: "Project Commands"},
			{Label: "cargo run", Description: "Run Rust program", Value: "cargo run", Icon: "▸", Category: "Project Commands"},
			{Label: "pytest", Description: "Run Python tests", Value: "pytest", Icon: "▸", Category: "Project Commands"},
		}
	}
	return items
}

func (a *App) fuzzyFilter(items []components.PaletteItem, filter string) []components.PaletteItem {
	if filter == "" {
		// Surface suggested/tier-prominent items first when unfiltered.
		// Primary tier floats top, matching Claude's recently-used pinning.
		if len(items) <= 1 {
			return items
		}
		// Stable tier sort: Primary -> Secondary -> Tertiary, preserving original order within tier.
		var primary, secondary, tertiary []components.PaletteItem
		for _, it := range items {
			switch it.Tier {
			case components.TierPrimary:
				primary = append(primary, it)
			case components.TierTertiary:
				tertiary = append(tertiary, it)
			default:
				secondary = append(secondary, it)
			}
		}
		if len(primary) > 0 && len(primary) < len(items) {
			return append(append(primary, secondary...), tertiary...)
		}
		return items
	}
	// Ranked fuzzy: exact > prefix > substring > fuzzy, weighted by tier.
	type scored struct {
		item  components.PaletteItem
		score int
		idx   int
	}
	filterLower := strings.ToLower(filter)
	var scoredItems []scored
	targets := make([]string, len(items))
	for i, item := range items {
		targets[i] = strings.ToLower(item.Label + " " + item.SearchTerms)
	}
	matches := fuzzy.Find(filter, targets)
	// Build score map for fuzzy matches.
	fuzzyScore := make(map[int]int, len(matches))
	for _, m := range matches {
		// sahilm/fuzzy Score is higher for better match (0 is worst). Normalize.
		fuzzyScore[m.Index] = m.Score + 100
	}
	for i, item := range items {
		labelLower := strings.ToLower(item.Label)
		base := 0
		switch {
		case labelLower == filterLower:
			base = 1000
		case strings.HasPrefix(labelLower, filterLower):
			base = 100
		case strings.Contains(labelLower, filterLower):
			base = 10
		default:
			if s, ok := fuzzyScore[i]; ok {
				base = s
			} else {
				continue
			}
		}
		// Tier weight.
		switch item.Tier {
		case components.TierPrimary:
			base = base * 3 / 2
		case components.TierTertiary:
			base = base * 2 / 3
		}
		scoredItems = append(scoredItems, scored{item: item, score: base, idx: i})
	}
	// Sort descending by score, stable by original index.
	for i := 0; i < len(scoredItems)-1; i++ {
		for j := i + 1; j < len(scoredItems); j++ {
			if scoredItems[j].score > scoredItems[i].score {
				scoredItems[i], scoredItems[j] = scoredItems[j], scoredItems[i]
			}
		}
	}
	filtered := make([]components.PaletteItem, 0, len(scoredItems))
	for _, s := range scoredItems {
		filtered = append(filtered, s.item)
	}
	return filtered
}

func (a *App) handleSlashCommand(input string) tea.Cmd {
	name, args := commands.Parse(input)
	if name == "" {
		return nil
	}
	host := a

	// Check for sub-commands first.
	parentName, subName, subArgs := commands.ParseWithSubCommand(a.commands, input)
	if subName != "" {
		result, err := a.commands.DispatchSubCommand(host, parentName, subName, subArgs)
		if err != nil {
			a.CommandError(err.Error())
			return nil
		}
		return a.deliverCommandResult(result)
	}

	// Look up the command to check its type.
	cmd, ok := a.commands.Lookup(name)
	if !ok {
		// Try reloading custom commands.
		if a.refreshCustomCommands() > 0 {
			if result, err := a.commands.Dispatch(host, name, args); err == nil {
				return a.deliverCommandResult(result)
			}
		}
		a.conversation.AddMessage("assistant", fmt.Sprintf("Unknown command: /%s", name), true)
		return nil
	}

	// Handle based on command type.
	switch cmd.Type {
	case commands.CmdPrompt:
		return a.handlePromptCommand(cmd, args)
	case commands.CmdFullPage:
		return a.handleFullPageCommand(cmd, args)
	default:
		return a.handleHandlerCommand(cmd, args)
	}
}

// handlePromptCommand injects the command's PromptTemplate into the agent conversation.
func (a *App) handlePromptCommand(cmd commands.Command, args []string) tea.Cmd {
	prompt := cmd.ExpandPrompt(args)
	if prompt == "" {
		// Fall back to handler if no template.
		return a.handleHandlerCommand(cmd, args)
	}
	a.conversation.AddMessage("user", prompt, false)
	return a.startAgent(prompt)
}

// handleFullPageCommand opens a full-page overlay with command output.
func (a *App) handleFullPageCommand(cmd commands.Command, args []string) tea.Cmd {
	// Special-case: open interactive overlays instead of text.
	switch cmd.Name {
	case "provider":
		a.providerStudio.Show()
		a.layout()
		return nil
	case "model":
		a.modelHub.Show()
		a.layout()
		return nil
	}
	// Execute the handler to get output text.
	result, err := a.commands.Dispatch(a, cmd.Name, args)
	if err != nil {
		a.CommandError(err.Error())
		return nil
	}
	title := cmd.FullPageTitle
	if title == "" {
		title = cmd.Name
	}
	content := result.Text
	if content == "" {
		content = "No output."
	}
	a.fullPage.Show(title, content)
	a.layout()
	return result.Cmd
}

// handleHandlerCommand dispatches via the standard handler.
func (a *App) handleHandlerCommand(cmd commands.Command, args []string) tea.Cmd {
	result, err := a.commands.Dispatch(a, cmd.Name, args)
	if err != nil {
		switch e := err.(type) {
		case commands.ErrUnknownCommand:
			a.conversation.AddMessage("assistant", fmt.Sprintf("Unknown command: /%s", cmd.Name), true)
		case commands.ErrCommandDisabled:
			a.statusBar.SetStatus(e.Reason)
		default:
			a.CommandError(err.Error())
		}
		return nil
	}
	return a.deliverCommandResult(result)
}

func (a *App) deliverCommandResult(result commands.Result) tea.Cmd {
	if result.Text != "" {
		a.conversation.AddMessage("system", result.Text, false)
	}
	return result.Cmd
}

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
		Icon:        "●",
	}, func(h commands.Host, args []string) commands.Result {
		a.zenMode = !a.zenMode
		a.statusBar.SetSegmentsVisible(!a.zenMode)
		a.statusBar.SetStatus(map[bool]string{true: "Zen mode", false: "Chrome restored"}[a.zenMode])
		a.layout()
		return commands.Done(nil)
	})
}
