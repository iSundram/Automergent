package command

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/iSundram/Automergent/internal/tui/components"
)

// Command represents a slash command definition.
type Command struct {
	Name        string
	Aliases     []string
	Description string
	Category    string
	Icon        string
	Usage       string
	Immediate   bool
}

// Handler is a command handler function.
// It receives the host (App) and parsed arguments, returns a tea.Cmd.
type Handler func(Host, []string) tea.Cmd

// ErrUnknownCommand is returned when a command is not registered.
type ErrUnknownCommand string

func (e ErrUnknownCommand) Error() string {
	return "unknown command: " + string(e)
}

// ErrSessionOwned is returned when a command is handled by the session layer.
type ErrSessionOwned string

func (e ErrSessionOwned) Error() string {
	return "session-owned command: " + string(e)
}

// Registry manages command definitions and handlers.
type Registry struct {
	commands     []Command
	handlers     map[string]Handler
	sessionOwned map[string]bool
	aliases      map[string]string
}

// NewRegistry creates a new command registry.
func NewRegistry() *Registry {
	return &Registry{
		commands:     make([]Command, 0),
		handlers:     make(map[string]Handler),
		sessionOwned: make(map[string]bool),
		aliases:      make(map[string]string),
	}
}

// Register adds a command with its handler.
// If handler is nil, the command is metadata-only (e.g., session-owned).
func (r *Registry) Register(cmd Command, handler Handler) {
	for _, existing := range r.commands {
		if existing.Name == cmd.Name {
			panic(fmt.Sprintf("command %q already registered", cmd.Name))
		}
	}
	for _, alias := range cmd.Aliases {
		if existing, ok := r.aliases[alias]; ok {
			panic(fmt.Sprintf("alias %q already used by %q", alias, existing))
		}
		r.aliases[alias] = cmd.Name
	}
	r.commands = append(r.commands, cmd)
	if handler != nil {
		r.handlers[cmd.Name] = handler
	} else {
		r.sessionOwned[cmd.Name] = true
	}
}

// MustRegister registers and panics on duplicate.
func (r *Registry) MustRegister(cmd Command, handler Handler) {
	r.Register(cmd, handler)
}

// Lookup finds a command by name or alias.
func (r *Registry) Lookup(name string) (Command, bool) {
	if cmdName, ok := r.aliases[name]; ok {
		name = cmdName
	}
	for _, cmd := range r.commands {
		if cmd.Name == name {
			return cmd, true
		}
	}
	return Command{}, false
}

// List returns all registered commands.
func (r *Registry) List() []Command {
	return r.commands
}

// HasHandler reports whether a command has a handler.
func (r *Registry) HasHandler(name string) bool {
	_, ok := r.handlers[name]
	return ok
}

// IsSessionOwned reports whether a command is session-owned (no handler in this package).
func (r *Registry) IsSessionOwned(name string) bool {
	return r.sessionOwned[name]
}

// Dispatch looks up and executes a command handler.
func (r *Registry) Dispatch(host Host, name string, args []string) (tea.Cmd, error) {
	if cmdName, ok := r.aliases[name]; ok {
		name = cmdName
	}
	if r.IsSessionOwned(name) {
		return nil, ErrSessionOwned(name)
	}
	handler, ok := r.handlers[name]
	if !ok {
		return nil, ErrUnknownCommand(name)
	}
	return handler(host, args), nil
}

// PaletteItems returns all commands as PaletteItems for the command palette.
func (r *Registry) PaletteItems() []components.PaletteItem {
	items := make([]components.PaletteItem, 0, len(r.commands))
	for _, cmd := range r.commands {
		hint := ""
		if cmd.Usage != "" {
			hint = cmd.Usage
		}
		searchTerms := strings.Join(append(append([]string{}, cmd.Aliases...), cmd.Description, cmd.Category), " ")
		items = append(items, components.PaletteItem{
			Label:       cmd.Name,
			Value:       cmd.Name,
			Description: cmd.Description,
			Icon:        cmd.Icon,
			Category:    cmd.Category,
			Hint:        hint,
			SearchTerms: searchTerms,
		})
	}
	return items
}