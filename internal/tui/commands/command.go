package commands

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/iSundram/Automergent/internal/tui/components"
)

// CommandTier indicates visual importance in the palette.
type CommandTier int

const (
	TierPrimary   CommandTier = iota // accent-styled, prominent
	TierSecondary                    // normal styling (default)
	TierTertiary                     // dimmed
)

// CommandType determines how a command is executed and displayed.
type CommandType int

const (
	// CmdHandler dispatches to a registered Handler function (default).
	CmdHandler CommandType = iota
	// CmdPrompt injects PromptTemplate into the agent conversation as context.
	CmdPrompt
	// CmdFullPage opens a full-page overlay for the command output.
	CmdFullPage
)

// SubCommand represents a nested sub-command under a parent command.
type SubCommand struct {
	Name        string
	Description string
	ArgsHint    string
	Handler     Handler
	// ValueCompletion returns candidate values for the subcommand's
	// argument (provider names for "provider setup", server names for
	// "mcp enable", ...). The partial is the text typed after the
	// subcommand; callers filter it.
	ValueCompletion func(Host, string) []string
}

// Command represents a slash command definition. The registry is the single
// source of truth: handlers, palette metadata and help documentation are all
// derived from it and must not be duplicated elsewhere.
type Command struct {
	Name        string
	Aliases     []string
	Description string
	Category    string
	Icon        string

	// ArgsHint is the argument hint shown in the palette and help (e.g. "<name|reset>").
	ArgsHint string
	// WhenToUse describes when a model should invoke this command.
	WhenToUse string
	// Tier controls visual prominence in the palette.
	Tier CommandTier
	// Type determines execution mode: handler, prompt injection, or full-page overlay.
	Type CommandType
	// Immediate reports whether selecting the command in the palette runs it
	// right away instead of completing its name into the input.
	Immediate bool
	// SubPalette names a sub-palette that opens after the command is selected.
	SubPalette string
	// SubCommands defines nested sub-commands (e.g. /mcp enable, /mcp disable).
	SubCommands []SubCommand
	// PromptTemplate is injected into the agent conversation when Type == CmdPrompt.
	// Supports $ARGUMENTS and $1..$9 placeholder expansion.
	PromptTemplate string
	// Fork runs a prompt command as a background subagent with its own
	// context instead of expanding it inline — the main conversation stays
	// clean, and the result lands as a system message when the fork returns.
	// Only meaningful with Type == CmdPrompt.
	Fork bool
	// Paths gates palette visibility on workspace activity: the command is
	// only offered after a recently touched file matches one of these globs
	// (e.g. ["*.go", "go.mod"] for Go-specific commands). Empty = always
	// visible. Matching is path.Match semantics against both the full path
	// and the base name.
	Paths []string
	// FullPageTitle is the title shown in the full-page overlay when Type == CmdFullPage.
	FullPageTitle string
	// Page builds the structured full-page view for this command when
	// Type == CmdFullPage. When nil the handler's plain-text Result.Text is
	// shown in the legacy FullPage overlay instead.
	Page func(Host) components.Page
	// Completion returns tab-completion suggestions for the given partial argument.
	Completion func(Host, string) []string
	// Source records where a custom command was loaded from (markdown path);
	// empty for built-ins.
	Source string
	// Hidden excludes the command from the palette and help overlay.
	Hidden bool
	// Sensitive marks commands whose arguments must never be logged verbatim.
	Sensitive bool
	// SupportsHeadless marks commands that can run in -p / no-tui mode.
	SupportsHeadless bool

	// Enabled gates execution against live host state. When nil the command is
	// always enabled.
	Enabled func(Host) bool
	// DisabledReason explains why a disabled command cannot run.
	DisabledReason func(Host) string
	// Current decorates stateful toggle commands with their on/off status.
	Current func(Host) bool
}

// kindBadge returns the palette glyph hinting how the command executes:
// "↵" for prompt commands (they start an agent run), "⤢" for full-page
// commands, "" for plain handlers.
func (c Command) kindBadge() string {
	switch c.Type {
	case CmdPrompt:
		return "↵"
	case CmdFullPage:
		return "⤢"
	default:
		return ""
	}
}

// ExpandPrompt replaces $ARGUMENTS and $1..$9 placeholders in the prompt template.
func (c *Command) ExpandPrompt(args []string) string {
	if c.PromptTemplate == "" {
		return ""
	}
	text := c.PromptTemplate
	joined := strings.Join(args, " ")
	text = strings.ReplaceAll(text, "$ARGUMENTS", joined)
	for i, arg := range args {
		if i >= 9 {
			break
		}
		placeholder := fmt.Sprintf("$%d", i+1)
		text = strings.ReplaceAll(text, placeholder, arg)
	}
	return text
}

// Handler is a command handler function.
type Handler func(Host, []string) Result

// Result is the outcome of executing a command handler.
type Result struct {
	Cmd  tea.Cmd
	Text string
	// ShouldQuery sends Text to the agent as the next prompt after the
	// command completes — for commands that gather state the model should
	// act on.
	ShouldQuery bool
}

func TextResult(text string) Result { return Result{Text: text} }
func Done(cmd tea.Cmd) Result       { return Result{Cmd: cmd} }
func (r Result) IsZero() bool       { return r.Cmd == nil && r.Text == "" }

// ErrUnknownCommand is returned when a command is not registered.
type ErrUnknownCommand string

func (e ErrUnknownCommand) Error() string {
	return "unknown command: " + string(e)
}

// ErrCommandDisabled is returned when a command's Enabled gate rejects dispatch.
type ErrCommandDisabled struct {
	Name   string
	Reason string
}

func (e ErrCommandDisabled) Error() string {
	if e.Reason == "" {
		return fmt.Sprintf("command %q is unavailable", e.Name)
	}
	return fmt.Sprintf("command %q is unavailable: %s", e.Name, e.Reason)
}

// Registry manages command definitions and handlers.
type Registry struct {
	commands []Command
	handlers map[string]Handler
	aliases  map[string]string
}

func NewRegistry() *Registry {
	return &Registry{
		handlers: make(map[string]Handler),
		aliases:  make(map[string]string),
	}
}

func (r *Registry) Register(cmd Command, handler Handler) {
	for _, existing := range r.commands {
		if existing.Name == cmd.Name {
			panic(fmt.Sprintf("command %q already registered", cmd.Name))
		}
	}
	for _, alias := range cmd.Aliases {
		if owner, ok := r.aliases[alias]; ok {
			panic(fmt.Sprintf("alias %q already used by %q", alias, owner))
		}
		r.aliases[alias] = cmd.Name
	}
	if handler == nil {
		panic(fmt.Sprintf("command %q registered without handler", cmd.Name))
	}
	r.commands = append(r.commands, cmd)
	r.handlers[cmd.Name] = handler
}

func (r *Registry) MustRegister(cmd Command, handler Handler) {
	r.Register(cmd, handler)
}

func (r *Registry) RegisterCustom(cmd Command, handler Handler) error {
	taken := make(map[string]string)
	for _, existing := range r.commands {
		taken[existing.Name] = existing.Name
		for _, alias := range existing.Aliases {
			taken[alias] = existing.Name
		}
	}
	if owner := taken[cmd.Name]; owner != "" {
		return fmt.Errorf("custom command %q conflicts with %q", cmd.Name, owner)
	}

	clean := Command{
		Name: cmd.Name, Description: cmd.Description, Category: cmd.Category,
		Icon: cmd.Icon, ArgsHint: cmd.ArgsHint, WhenToUse: cmd.WhenToUse,
		Tier: cmd.Tier, Type: cmd.Type, Immediate: cmd.Immediate,
		SubPalette: cmd.SubPalette, SubCommands: cmd.SubCommands,
		PromptTemplate: cmd.PromptTemplate, FullPageTitle: cmd.FullPageTitle,
		Fork:  cmd.Fork,
		Paths: cmd.Paths,
		Page:  cmd.Page,
		Completion: cmd.Completion,
		Source:     cmd.Source,
		Hidden:     cmd.Hidden, Sensitive: cmd.Sensitive,
		SupportsHeadless: cmd.SupportsHeadless,
	}
	seen := map[string]bool{cmd.Name: true}
	for _, alias := range cmd.Aliases {
		if alias == cmd.Name {
			continue
		}
		if seen[alias] {
			return fmt.Errorf("custom command %q: duplicate alias %q", cmd.Name, alias)
		}
		if owner := taken[alias]; owner != "" {
			return fmt.Errorf("custom command %q: alias %q already used by %q", cmd.Name, alias, owner)
		}
		seen[alias] = true
		clean.Aliases = append(clean.Aliases, alias)
	}
	r.Register(clean, handler)
	return nil
}

func (r *Registry) Lookup(name string) (Command, bool) {
	if canonical, ok := r.aliases[name]; ok {
		name = canonical
	}
	for _, cmd := range r.commands {
		if cmd.Name == name {
			return cmd, true
		}
	}
	return Command{}, false
}

func (r *Registry) List() []Command { return r.commands }

// SubPaletteNames returns the names of every command that declares a
// SubPalette, in registration order. The app derives the input layer's
// trigger set from this so the two never drift.
func (r *Registry) SubPaletteNames() []string {
	var out []string
	for _, cmd := range r.commands {
		if cmd.SubPalette != "" {
			out = append(out, cmd.SubPalette)
		}
	}
	return out
}

func (r *Registry) HasHandler(name string) bool {
	_, ok := r.handlers[name]
	return ok
}

func (r *Registry) RemoveCustom() int {
	kept := make([]Command, 0, len(r.commands))
	removed := 0
	for _, cmd := range r.commands {
		if cmd.Category == customCategory {
			removed++
			delete(r.handlers, cmd.Name)
			continue
		}
		kept = append(kept, cmd)
	}
	r.commands = kept
	r.aliases = make(map[string]string)
	for _, cmd := range r.commands {
		for _, alias := range cmd.Aliases {
			r.aliases[alias] = cmd.Name
		}
	}
	return removed
}

func (r *Registry) HeadlessCommands() []Command {
	out := make([]Command, 0, len(r.commands))
	for _, cmd := range r.commands {
		if cmd.SupportsHeadless {
			out = append(out, cmd)
		}
	}
	return out
}

// DispatchSubCommand resolves a sub-command by name and executes it.
// The subcommand name is prepended to args so generic handlers (like handleMCP)
// that switch on args[0] work unchanged whether dispatched via sub-command or
// via the parent handler.
func (r *Registry) DispatchSubCommand(host Host, parentName string, subName string, args []string) (Result, error) {
	if canonical, ok := r.aliases[parentName]; ok {
		parentName = canonical
	}
	cmd, ok := r.Lookup(parentName)
	if !ok {
		return Result{}, ErrUnknownCommand(parentName)
	}
	for _, sub := range cmd.SubCommands {
		if sub.Name == subName {
			fullArgs := append([]string{subName}, args...)
			return sub.Handler(host, fullArgs), nil
		}
	}
	return Result{}, ErrUnknownCommand(parentName + " " + subName)
}

// LookupSubCommand finds a sub-command within a parent command.
func (r *Registry) LookupSubCommand(parentName, subName string) (SubCommand, bool) {
	if canonical, ok := r.aliases[parentName]; ok {
		parentName = canonical
	}
	cmd, ok := r.Lookup(parentName)
	if !ok {
		return SubCommand{}, false
	}
	for _, sub := range cmd.SubCommands {
		if sub.Name == subName {
			return sub, true
		}
	}
	return SubCommand{}, false
}

func (r *Registry) Dispatch(host Host, name string, args []string) (Result, error) {
	if canonical, ok := r.aliases[name]; ok {
		name = canonical
	}
	cmd, ok := r.Lookup(name)
	if !ok {
		return Result{}, ErrUnknownCommand(name)
	}
	if cmd.Enabled != nil && !cmd.Enabled(host) {
		reason := ""
		if cmd.DisabledReason != nil {
			reason = cmd.DisabledReason(host)
		}
		if reason == "" {
			reason = cmd.Description
		}
		return Result{}, ErrCommandDisabled{Name: cmd.Name, Reason: reason}
	}
	handler := r.handlers[cmd.Name]
	if handler == nil {
		return Result{}, fmt.Errorf("command %q has no handler", cmd.Name)
	}
	return handler(host, args), nil
}

func (r *Registry) PaletteItems(host Host) []components.PaletteItem {
	items := make([]components.PaletteItem, 0, len(r.commands))
	var recent []string
	for _, cmd := range r.commands {
		if cmd.Hidden {
			continue
		}
		// Path-gated commands stay out of the palette until a recently
		// touched file matches one of their globs. A nil host carries no
		// workspace signal (offline palette construction, tests), so gating
		// is skipped there.
		if len(cmd.Paths) > 0 && host != nil {
			if recent == nil {
				recent = host.RecentFilePaths()
			}
			if !pathSetMatches(cmd.Paths, recent) {
				continue
			}
		}
		description := cmd.Description
		if description == "" {
			// WhenToUse doubles as the palette blurb for commands that only
			// define guidance (custom markdown commands without a description).
			description = cmd.WhenToUse
		}
		item := components.PaletteItem{
			Label:       cmd.Name,
			Value:       cmd.Name,
			Description: description,
			Icon:        cmd.Icon,
			Category:    cmd.Category,
			Hint:        cmd.ArgsHint,
			Kind:        cmd.kindBadge(),
			Tier:        components.CommandTier(cmd.Tier),
			SearchTerms: strings.Join(append(append([]string{}, cmd.Aliases...), cmd.Description, cmd.Category, cmd.WhenToUse), " "),
		}
		if host != nil && cmd.Current != nil {
			item.Current = cmd.Current(host)
		}
		if host != nil && cmd.Enabled != nil && !cmd.Enabled(host) {
			item.Disabled = true
			item.DisabledReason = cmd.Description
			if cmd.DisabledReason != nil {
				if reason := cmd.DisabledReason(host); reason != "" {
					item.DisabledReason = reason
				}
			}
		}
		items = append(items, item)
	}
	return items
}

// pathSetMatches reports whether any file path matches any glob. Matching
// runs against both the full (slash-normalized) path and the base name, so
// "*.go" matches "internal/agent/agent.go" as well as "agent.go". Malformed
// globs are skipped rather than failing the match.
func pathSetMatches(globs, paths []string) bool {
	for _, p := range paths {
		full := filepath.ToSlash(p)
		base := path.Base(full)
		for _, g := range globs {
			if ok, err := path.Match(g, full); err == nil && ok {
				return true
			}
			if ok, err := path.Match(g, base); err == nil && ok {
				return true
			}
		}
	}
	return false
}

// SubCommandPaletteItems returns sub-commands as PaletteItems for the palette.
func (r *Registry) SubCommandPaletteItems(host Host, parentName string) []components.PaletteItem {
	cmd, ok := r.Lookup(parentName)
	if !ok {
		return nil
	}
	items := make([]components.PaletteItem, 0, len(cmd.SubCommands))
	for _, sub := range cmd.SubCommands {
		items = append(items, components.PaletteItem{
			Label:       sub.Name,
			Value:       sub.Name,
			Description: sub.Description,
			Icon:        cmd.Icon,
			Category:    cmd.Name,
			Hint:        sub.ArgsHint,
			Tier:        components.CommandTier(cmd.Tier),
		})
	}
	return items
}

// SearchableSubCommandItems returns every sub-command as a palette item whose
// value is "<parent> <sub>" — these join the command list during palette
// searches so typing "fallback" surfaces "/provider fallback" without first
// knowing the parent. Label carries the parent for context; SearchTerms
// covers parent name, sub name and descriptions both ways.
func (r *Registry) SearchableSubCommandItems() []components.PaletteItem {
	var items []components.PaletteItem
	for _, cmd := range r.commands {
		if cmd.Hidden {
			continue
		}
		for _, sub := range cmd.SubCommands {
			items = append(items, components.PaletteItem{
				Label:       cmd.Name + " " + sub.Name,
				Value:       cmd.Name + " " + sub.Name,
				Description: sub.Description,
				Icon:        cmd.Icon,
				Category:    cmd.Category,
				Hint:        sub.ArgsHint,
				Tier:        components.CommandTier(cmd.Tier),
				SearchTerms: cmd.Name + " " + sub.Name + " " + sub.Description + " " + cmd.Description,
			})
		}
	}
	return items
}
