package commands

import "strings"

// Parse parses a slash command input into command name and arguments.
// Returns empty name if input is not a slash command.
func Parse(input string) (name string, args []string) {
	input = strings.TrimSpace(input)
	if !strings.HasPrefix(input, "/") {
		return "", nil
	}
	parts := strings.Fields(input)
	if len(parts) == 0 {
		return "", nil
	}
	name = strings.TrimPrefix(parts[0], "/")
	if len(parts) > 1 {
		args = parts[1:]
	}
	return name, args
}

// ParseWithSubCommand parses input and resolves sub-commands.
// Returns (parentName, subName, args). If no sub-command is found, subName is empty.
// Example: "/mcp enable myserver" -> ("mcp", "enable", ["myserver"])
func ParseWithSubCommand(reg *Registry, input string) (parentName, subName string, args []string) {
	name, remaining := Parse(input)
	if name == "" {
		return "", "", nil
	}
	cmd, ok := reg.Lookup(name)
	if !ok || len(cmd.SubCommands) == 0 || len(remaining) == 0 {
		return name, "", remaining
	}
	// Check if the first argument matches a sub-command.
	subCandidate := remaining[0]
	for _, sub := range cmd.SubCommands {
		if sub.Name == subCandidate {
			return name, subCandidate, remaining[1:]
		}
	}
	return name, "", remaining
}
