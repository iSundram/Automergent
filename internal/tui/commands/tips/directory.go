package tips

// directory tip material (from the command registry).
func init() {
	register(CommandTip{
		Name:         "directory",
		Tip:          "show the working directory and search roots",
		Personalized: "",
		Body:         "# /directory\n\nPrints the active working directory and any extra search roots added with\n`/add-dir`, plus the write-boundary policy (which paths are blocked or\nallowed by configuration).\n\n## Notes\n- The working directory is fixed at launch; extra roots are session state.\n- Writes outside allowed paths always ask first — the boundary is enforced\n  by the security layer, not by the model's goodwill.\n\n## Related\n- `/add-dir` — add a root.\n- `/doctor` — includes path-permission diagnostics.",
	})
}
