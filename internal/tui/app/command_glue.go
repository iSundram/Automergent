package app

// Slash-command dispatch and palette glue.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/iSundram/Automergent/internal/tui/commands"
	"github.com/iSundram/Automergent/internal/tui/components"
	"github.com/sahilm/fuzzy"
)

// updatePalette resolves the input's trigger, builds the palette items for
// it and applies the fuzzy filter. The per-trigger item sets come from the
// sub-palette spec table (subpalettes.go); the only logic here is the shared
// pipeline: trigger → items → filter → empty-hint → SetItems.
func (a *App) updatePalette() {
	trigger := a.input.TriggerType()
	filter := a.input.TriggerValue()
	a.palette.SetQuery(filter)
	a.palette.SetEmptyHint(paletteEmptyHintFor(trigger))

	var items []components.PaletteItem
	switch trigger {
	case "help", "command":
		items = a.fuzzyFilter(a.commands.PaletteItems(a), filter)
		// A search over the command list also searches sub-commands: typing
		// "fallback" surfaces "/provider fallback" without knowing the
		// parent. Only while filtering — the unfiltered list stays the
		// top-level command surface.
		if strings.TrimSpace(filter) != "" {
			subItems := a.fuzzyFilter(a.commands.SearchableSubCommandItems(), filter)
			items = append(items, subItems...)
		}
	default:
		if spec, ok := subPaletteSpecFor(trigger); ok {
			items = a.fuzzyFilter(spec.Build(a), filter)
		}
	}

	// Argument completion: once a sub-command or argument is being typed
	// ("mcp enable fs"), the item set comes from the command's sub-commands,
	// dynamic value providers or Completion function instead of the trigger's
	// item list.
	if strings.Contains(filter, " ") || len(items) == 0 {
		if argItems := a.argumentCompletion(trigger, filter); argItems != nil {
			items = a.fuzzyFilter(argItems, argumentPartial(trigger, filter))
		}
	}

	a.palette.SetItems(items)
}

// argumentCompletion returns item candidates for the argument portion of the
// filter, or nil when the command has no completion path. For the command
// trigger the first filter token names the command ("/mcp enable fs"); for
// sub-palette triggers the command is the trigger itself.
func (a *App) argumentCompletion(trigger, filter string) []components.PaletteItem {
	name := trigger
	if trigger == "command" || trigger == "help" {
		parts := strings.Fields(filter)
		if len(parts) == 0 {
			return nil
		}
		name = parts[0]
		filter = strings.Join(parts[1:], " ")
	}
	if _, ok := a.commands.Lookup(name); !ok {
		return nil
	}
	return a.argumentItems(name, filter)
}

// argumentPartial extracts the partial argument text the completion items
// should be filtered by (everything after the command/sub-command token).
func argumentPartial(trigger, filter string) string {
	parts := strings.Fields(filter)
	if trigger == "command" || trigger == "help" {
		if len(parts) <= 1 {
			return ""
		}
		parts = parts[1:]
	}
	if len(parts) <= 1 {
		return ""
	}
	return strings.Join(parts[1:], " ")
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
		// Primary tier floats top, matching the recently-used pinning pattern.
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

// maxDispatchDepth bounds cross-command invocation chains (Host.DispatchCommand
// and page actions). Five lets a page action dispatch a command whose page
// offers another action, and stops accidental self-referencing loops from
// hanging the UI.
const maxDispatchDepth = 5

// handleSlashCommand runs one slash-command input and flushes any tea.Cmds
// accumulated by nested dispatches. It is the single entry point for typed
// commands, palette selections and page actions alike.
func (a *App) handleSlashCommand(input string) tea.Cmd {
	top := !a.dispatchActive
	a.dispatchActive = true
	cmd := a.dispatchSlashCommand(input)
	if top {
		a.dispatchActive = false
		if len(a.pendingDispatchCmds) > 0 {
			pending := a.pendingDispatchCmds
			a.pendingDispatchCmds = nil
			cmd = tea.Batch(append(pending, cmd)...)
		}
	}
	return cmd
}

func (a *App) dispatchSlashCommand(input string) tea.Cmd {
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

// handlePromptCommand injects the command's PromptTemplate into the agent
// conversation. The conversation entry carries the command as provenance (the
// "❯ /commit" chip) and the agent receives the expansion wrapped in a short
// origin header, so both the user and the model know where the prompt came
// from.
func (a *App) handlePromptCommand(cmd commands.Command, args []string) tea.Cmd {
	prompt := cmd.ExpandPrompt(args)
	if prompt == "" {
		// Fall back to handler if no template.
		return a.handleHandlerCommand(cmd, args)
	}
	// Fork commands run in a background subagent with their own context;
	// the expansion is recorded in the conversation for provenance and the
	// result arrives as a system message.
	if cmd.Fork {
		a.AddUserCommandMessage(cmd.Name, prompt)
		a.StartForkedAgent(cmd.Name, prompt)
		a.statusBar.SetStatus("❯ /" + cmd.Name + " running in background agent")
		return nil
	}
	agentPrompt := fmt.Sprintf("<command-message>/%s</command-message>\n%s", cmd.Name, prompt)
	// startAgentCommand renders the "/command" chip bubble in the
	// conversation; a plain startAgent here would echo the template a second
	// time (with the raw provenance tags) on top of it.
	return a.startAgentCommand(prompt, agentPrompt, cmd.Name)
}

// handleFullPageCommand opens a full-page overlay with command output.
func (a *App) handleFullPageCommand(cmd commands.Command, args []string) tea.Cmd {
	// /model and /provider are no longer full-page overlays: their picker
	// subpalettes (opened by the trailing space — "/model ", "/provider ")
	// and their handler status output are the single surfaces. The old
	// Model Hub / Provider Studio pages duplicated both, so a bare
	// "/model" opened a full page while "/model " opened a picker — two
	// surfaces for one job. Structured page: the command's view builder
	// renders sections and action shortcuts into the PageViewer. Args fall
	// through to the handler so argument paths (/context detail, /error
	// clear) keep working.
	if cmd.Page != nil && len(args) == 0 {
		a.pageViewer.Show(cmd.Page(a))
		a.fullPage.Hide()
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
	// An empty Result means the handler did its own UI (session browser, diff
	// pane, help overlay) — showing a "No output" page on top of it would
	// bury that.
	if result.Text == "" {
		return result.Cmd
	}
	a.pageViewer.Hide()
	a.fullPage.Show(title, result.Text)
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
	// ShouldQuery: the gathered state becomes the agent's next prompt.
	if result.ShouldQuery && result.Text != "" {
		return tea.Batch(result.Cmd, a.startAgent(result.Text))
	}
	return result.Cmd
}

// paletteEnter implements enter while the palette is open. The rule table,
// in priority order:
//
//	disabled item          → show reason in status bar; palette stays open
//	command list selection →
//	  sub-palette command  → insert "/<name> " and open its picker
//	  immediate command    → dispatch "/<name>" right away
//	  other command        → insert "/<name> " so arguments can follow
//	argument completion    → replace the partial argument token with the
//	                         selected value and dispatch the composed command
//	file mention           → insert "@<path> " into the message text
//	sub-palette value      → compose "/<trigger> <value>" and dispatch
//	no selection           → dispatch raw input if it names a command (or has
//	                         arguments); otherwise close and keep the text
//
// Typed input never silently overrides a highlighted selection: the raw-input
// dispatch only runs when there is no selection to act on.
func (a *App) paletteEnter() tea.Cmd {
	sel := a.palette.Selected()
	trigger := a.input.TriggerType()
	filter := strings.TrimSpace(a.input.TriggerValue())

	if sel == nil {
		// Free-form escape: dispatch raw input only when there's no
		// selection (the user bypassed the palette).
		raw := strings.TrimSpace(a.input.Value())
		if raw != "" && strings.HasPrefix(raw, "/") {
			name := strings.TrimPrefix(strings.Fields(raw)[0], "/")
			if _, known := a.commands.Lookup(name); known || strings.Contains(raw, " ") {
				a.input.Reset()
				a.palette.Hide()
				a.layout()
				return a.handleSlashCommand(raw)
			}
		}
		a.palette.Hide()
		a.layout()
		return nil
	}

	if sel.Disabled {
		a.statusBar.SetStatus(sel.DisabledReason)
		return nil
	}

	// File mentions insert into the message text; they are not commands.
	if trigger == "file" {
		a.input.InsertValue(sel.Value)
		a.updatePalette()
		a.palette.Hide()
		a.layout()
		return nil
	}

	// Command list (no argument typed yet): dispatch or insert the command.
	if trigger == "command" || trigger == "help" {
		if !strings.Contains(filter, " ") {
			// Sub-command search hit ("<parent> <sub>"): dispatch the
			// composed command directly.
			if fields := strings.Fields(sel.Value); len(fields) == 2 {
				if _, parentOK := a.commands.Lookup(fields[0]); parentOK {
					if _, subOK := a.commands.LookupSubCommand(fields[0], fields[1]); subOK {
						a.input.Reset()
						a.palette.Hide()
						a.layout()
						return a.handleSlashCommand("/" + sel.Value)
					}
				}
			}
			// Sub-palette command → insert value and open its picker.
			if components.SlashSubPalettes[sel.Value] {
				a.input.InsertValue(sel.Value)
				a.updatePalette()
				a.palette.Show(a.palette.Items(), a.input.TriggerValue())
				a.layout()
				if sel.Value == "model" && len(a.availableModels) == 0 {
					return a.fetchModels()
				}
				return nil
			}
			if definition, known := a.commands.Lookup(sel.Value); known {
				if definition.Immediate {
					a.input.Reset()
					a.palette.Hide()
					a.layout()
					return a.handleSlashCommand("/" + sel.Value)
				}
				// Non-immediate: insert into input so arguments can follow.
				a.input.InsertValue("/" + sel.Value)
				return nil
			}
		}
		// Argument completion selection: replace the trailing partial token
		// with the selected value and dispatch the composed command.
		raw := strings.TrimRight(a.input.Value(), " ")
		fields := strings.Fields(raw)
		composed := strings.Join(fields[:max(1, len(fields)-1)], " ") + " " + sel.Value
		a.input.Reset()
		a.palette.Hide()
		a.layout()
		return a.handleSlashCommand(composed)
	}

	// Sub-palette value (model name, provider, commit scope, …): compose
	// "/<trigger> <value>" and dispatch. Empty value keeps the bare command.
	input := "/" + trigger
	if sel.Value != "" {
		input += " " + sel.Value
	}
	a.input.Reset()
	a.palette.Hide()
	a.layout()
	return a.handleSlashCommand(input)
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
