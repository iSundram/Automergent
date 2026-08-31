package tips

// keybindings tip material (from the command registry).
func init() {
	register(CommandTip{
		Name:         "keybindings",
		Tip:          "switch the keybinding scheme — default, vim, emacs",
		Personalized: "",
		Body:         "# /keybindings\n\nSwitches the keybinding scheme: default, vim or emacs navigation grammar\nacross the prompt, browsers and overlays.\n\n## Usage\n- `/keybindings` — list schemes and show the current one.\n- `/keybindings <scheme>` — switch.\n\n## Notes\n- The `?` help overlay reflects the active scheme's keys.\n- Persisted with the project config.\n\n## Related\n- `/config` — the stored settings view.",
	})
}
