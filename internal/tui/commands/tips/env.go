package tips

// env tip material (from the command registry).
func init() {
	register(CommandTip{
		Name:         "env",
		Tip:          "environment report — versions, terminal, capabilities",
		Personalized: "",
		Body:         "# /env\n\nPrints the runtime environment: application version, Go version, terminal\ncapabilities (truecolor, sync updates, keyboard protocol) and the\nresolution of key environment variables.\n\n## Usage\n- `/env` — the full report.\n\n## Notes\n- The capability lines explain why some visual features degrade on certain\n  terminals (tmux, screen).\n- API keys are shown as resolution sources, never values.\n\n## Related\n- `/doctor` — health checks on top of the environment.\n- `/version` — just the version.",
	})
}
