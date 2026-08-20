package command

import (
	tea "charm.land/bubbletea/v2"

	"github.com/iSundram/Automergent/internal/tui/components"
)

// PaletteItems returns all commands as PaletteItems for the command palette.
// This is a convenience wrapper around Registry.PaletteItems().
func PaletteItems() []components.PaletteItem {
	return Default().PaletteItems()
}

// Lookup is a convenience wrapper around Registry.Lookup().
func Lookup(name string) (Command, bool) {
	return Default().Lookup(name)
}

// Dispatch is a convenience wrapper around Registry.Dispatch().
func Dispatch(host Host, input string) (tea.Cmd, error) {
	name, args := Parse(input)
	if name == "" {
		return nil, nil
	}
	return Default().Dispatch(host, name, args)
}