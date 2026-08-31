package tips

// config tip material (from the command registry).
func init() {
	register(CommandTip{
		Name:         "config",
		Tip:          "open the settings picker — every persistent option",
		Personalized: "",
		Body:         "# /config\n\nOpens the settings picker: every persistent option (provider, model, theme,\nkeybindings, effort, security paths) with live values, plus paths to the\nglobal and project config files.\n\n## Usage\n- `/config` — open the picker.\n- Changes made through focused commands (`/theme`, `/effort`, ...) persist\n  to the same store.\n\n## Notes\n- Global config lives in `~/.automergent/config.yaml`; the project config\n  overrides it per workspace.\n- `/doctor` validates the merged result.\n\n## Related\n- `/doctor` — environment health check.\n- `/env` — the runtime environment view.",
	})
}
