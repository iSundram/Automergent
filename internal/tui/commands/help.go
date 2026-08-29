package commands

import (
	"sort"
	"strings"

	"github.com/iSundram/Automergent/internal/tui/components"
)

// HelpRows renders every visible command as a [key, description] row for the
// help overlay. Keys merge aliases and append the args hint, so documentation
// is always derived from — never duplicated against — the registry.
func (r *Registry) HelpRows() [][2]string {
	rows := make([][2]string, 0, len(r.commands))
	for _, cmd := range r.commands {
		if cmd.Hidden {
			continue
		}
		rows = append(rows, helpRow(cmd))
	}
	return rows
}

// HelpSections groups every visible command by category, tier-sorted within
// each category. Categories appear in registration order (stable across
// rebuilds), primary-tier commands first inside each group.
func (r *Registry) HelpSections() []components.HelpSection {
	var order []string
	byCategory := map[string][]Command{}
	for _, cmd := range r.commands {
		if cmd.Hidden {
			continue
		}
		if _, seen := byCategory[cmd.Category]; !seen {
			order = append(order, cmd.Category)
		}
		byCategory[cmd.Category] = append(byCategory[cmd.Category], cmd)
	}

	sections := make([]components.HelpSection, 0, len(order))
	for _, cat := range order {
		cmds := byCategory[cat]
		sort.SliceStable(cmds, func(i, j int) bool { return cmds[i].Tier < cmds[j].Tier })
		rows := make([][2]string, 0, len(cmds))
		for _, cmd := range cmds {
			rows = append(rows, helpRow(cmd))
		}
		sections = append(sections, components.HelpSection{Title: cat, Rows: rows})
	}
	return sections
}

// helpRow renders one command as a [key, description] pair. The key merges
// aliases and appends the args hint; prompt commands get a ↵ marker so help
// matches the palette's kind badges.
func helpRow(cmd Command) [2]string {
	names := make([]string, 0, 1+len(cmd.Aliases))
	for _, name := range append([]string{cmd.Name}, cmd.Aliases...) {
		names = append(names, "/"+name)
	}
	key := strings.Join(names, " or ")
	if cmd.ArgsHint != "" {
		key += " " + cmd.ArgsHint
	}
	switch cmd.Type {
	case CmdPrompt:
		key += "  ↵"
	case CmdFullPage:
		key += "  ⤢"
	}
	return [2]string{key, cmd.Description}
}
