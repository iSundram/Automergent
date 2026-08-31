package tips

// commands tip material (from the command registry).
func init() {
	register(CommandTip{
		Name:         "commands",
		Tip:          "list every registered command with metadata",
		Personalized: "",
		Body:         "# /commands\n\nLists the live command registry: every command with its category, aliases,\nargument hints and source (built-in or custom markdown command).\n\n## Usage\n- `/commands` — the full listing.\n- `/commands <filter>` — narrow by name or category.\n\n## Notes\n- The listing is the registry itself — the same source the palette, help\n  and dispatch use, so it can never drift from reality.\n- Custom commands from `.automergent/commands/` appear with their source\n  path.\n\n## Related\n- `/help` — the same commands with usage guidance.\n- `/skills` — the skill inventory.",
	})
}
