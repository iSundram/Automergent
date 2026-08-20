package command

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