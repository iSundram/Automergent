package commands

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/iSundram/Automergent/internal/tui/components"
)

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
	// WhenToUse describes when a model should invoke this command. It is
	// model-facing metadata reserved for future skill exposure.
	WhenToUse string
	// Immediate reports whether selecting the command in the palette runs it
	// right away instead of completing its name into the input.
	Immediate bool
	// Hidden excludes the command from the palette and help overlay.
	Hidden bool
	// Sensitive marks commands whose arguments must never be logged verbatim.
	Sensitive bool
	// SupportsHeadless marks commands that can run in -p / no-tui mode where
	// overlays do not exist and only text output is visible.
	SupportsHeadless bool

	// Enabled gates execution against live host state. When nil the command is
	// always enabled.
	Enabled func(Host) bool
	// DisabledReason explains why a disabled command cannot run. Only consulted
	// when Enabled returns false; an empty result falls back to the command name.
	DisabledReason func(Host) string
	// Current decorates stateful toggle commands with their on/off status.
	Current func(Host) bool
}

// Handler is a command handler function. It receives the host (App) and parsed
// arguments and returns a Result carrying optional text output and/or an async
// continuation. All other reporting happens through Host methods.
type Handler func(Host, []string) Result

// Result is the outcome of executing a command handler. Handlers normally
// report through the Host; Result carries anything extra the dispatcher must
// relay to the front end.
type Result struct {
	// Cmd is an optional asynchronous continuation (bubbletea command) that the
	// caller must run.
	Cmd tea.Cmd
	// Text is optional textual output. In interactive mode it is rendered as a
	// system message by the caller; in headless mode it is printed to stdout.
	Text string
}

// TextResult builds a pure-text result.
func TextResult(text string) Result { return Result{Text: text} }

// Done wraps an async continuation into a result.
func Done(cmd tea.Cmd) Result { return Result{Cmd: cmd} }

// IsZero reports whether the result carries nothing for the caller to do.
func (r Result) IsZero() bool { return r.Cmd == nil && r.Text == "" }

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

// NewRegistry creates a new command registry.
func NewRegistry() *Registry {
	return &Registry{
		handlers: make(map[string]Handler),
		aliases:  make(map[string]string),
	}
}

// Register adds a command with its handler. Duplicate names or aliases and
// nil handlers panic: registration errors are programmer errors and must fail
// loudly at startup instead of surfacing as runtime dispatch failures.
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

// MustRegister registers and panics on duplicate. Kept for call-site clarity.
func (r *Registry) MustRegister(cmd Command, handler Handler) {
	r.Register(cmd, handler)
}

// RegisterCustom adds a user-provided command without panicking. Unlike
// Register it refuses collisions with builtin names or aliases (builtins win)
// and reports the conflict as an error instead.
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
		Immediate: cmd.Immediate, Hidden: cmd.Hidden, Sensitive: cmd.Sensitive,
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

// Lookup finds a command by name or alias.
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

// List returns all registered commands, hidden ones included.
func (r *Registry) List() []Command {
	return r.commands
}

// HasHandler reports whether a command has a handler.
func (r *Registry) HasHandler(name string) bool {
	_, ok := r.handlers[name]
	return ok
}

// RemoveCustom unregisters all user-provided (Custom category) commands so
// they can be freshly reloaded from disk. Returns how many were removed.
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

// HeadlessCommands lists commands marked safe to run without a TUI. The
// headless entrypoint (planned slash support in `-p` mode) will consume this;
// keeping the filter here means new commands opt in at definition time.
func (r *Registry) HeadlessCommands() []Command {
	out := make([]Command, 0, len(r.commands))
	for _, cmd := range r.commands {
		if cmd.SupportsHeadless {
			out = append(out, cmd)
		}
	}
	return out
}

// Dispatch looks up and executes a command handler. Names resolve through
// aliases first. A registered but disabled command yields ErrCommandDisabled.
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

// PaletteItems returns visible commands as PaletteItems for the command
// palette, evaluated against host so Enabled/DisabledReason/Current decorations
// reflect live state. A nil host treats every command as enabled and inactive.
func (r *Registry) PaletteItems(host Host) []components.PaletteItem {
	items := make([]components.PaletteItem, 0, len(r.commands))
	for _, cmd := range r.commands {
		if cmd.Hidden {
			continue
		}
		item := components.PaletteItem{
			Label:       cmd.Name,
			Value:       cmd.Name,
			Description: cmd.Description,
			Icon:        cmd.Icon,
			Category:    cmd.Category,
			Hint:        cmd.ArgsHint,
			SearchTerms: strings.Join(append(append([]string{}, cmd.Aliases...), cmd.Description, cmd.Category), " "),
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
