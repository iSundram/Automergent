package tips

// rename tip material (from the command registry).
func init() {
	register(CommandTip{
		Name:         "rename",
		Tip:          "rename the current session",
		Personalized: "",
		Body:         "# /rename\n\nSets the title of the current session. Titles appear in `/sessions`, the\nexit banner and resume matching; unnamed sessions fall back to their\nauto-generated title or first message.\n\n## Usage\n- `/rename <title>` — rename (empty titles are refused).\n\n## Notes\n- Renaming persists immediately to storage.\n- `/resume <title-substring>` matches the new name right away.\n- In the session picker, `ctrl+r` renames any listed session inline.\n\n## Related\n- `/sessions` — where titles are shown and searched.",
	})
}
