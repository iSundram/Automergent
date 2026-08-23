package commands

import "strings"

// HelpRows renders every visible command as a [key, description] row for the
// help overlay. Keys merge aliases and append the args hint, so documentation
// is always derived from — never duplicated against — the registry.
func (r *Registry) HelpRows() [][2]string {
	rows := make([][2]string, 0, len(r.commands))
	for _, cmd := range r.commands {
		if cmd.Hidden {
			continue
		}
		names := make([]string, 0, 1+len(cmd.Aliases))
		for _, name := range append([]string{cmd.Name}, cmd.Aliases...) {
			names = append(names, "/"+name)
		}
		key := strings.Join(names, " or ")
		if cmd.ArgsHint != "" {
			key += " " + cmd.ArgsHint
		}
		rows = append(rows, [2]string{key, cmd.Description})
	}
	return rows
}
