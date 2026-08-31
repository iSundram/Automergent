package tips

// version tip material (from the command registry).
func init() {
	register(CommandTip{
		Name:         "version",
		Tip:          "print the version",
		Personalized: "",
		Body:         "# /version\n\nPrints the application version (and the build's module version when it is\nan installed binary).\n\n## Usage\n- `/version` — version string.\n\n## Related\n- `/env` — version plus the full environment report.\n- `/doctor` — health check.",
	})
}
