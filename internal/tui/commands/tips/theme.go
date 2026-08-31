package tips

// theme tip material (from the command registry).
func init() {
	register(CommandTip{
		Name:         "theme",
		Tip:          "switch the color theme",
		Personalized: "",
		Body:         "# /theme\n\nSwitches the color theme (modern, catppuccin, dracula, ...). The change\napplies instantly across header, palette, browsers and diff views, and\npersists in the project config.\n\n## Usage\n- `/theme` — list themes and show the current one.\n- `/theme <name>` — switch.\n\n## Notes\n- Terminal color depth is respected; degraded palettes stay legible.\n- The change is persisted to the project config on success.\n\n## Related\n- `/config` — where the setting is stored.",
	})
}
