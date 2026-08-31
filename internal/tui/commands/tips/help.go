package tips

// help tip material (from the command registry).
func init() {
	register(CommandTip{
		Name:         "help",
		Tip:          "the shortcut and command reference (? or /help)",
		Personalized: "",
		Body:         "# /help\n\nOpens the help overlay: keybindings for the active scheme, the full command\nreference grouped by category, and mode explanations.\n\n## Usage\n- `/help` or `?` — open.\n- `esc` or `?` — close.\n\n## Notes\n- The reference is generated from the live command registry, so custom\n  commands appear automatically.\n- Each command's comprehensive tips (this material) back its entry.\n\n## Related\n- `/commands` — the raw registry listing.",
	})
}
